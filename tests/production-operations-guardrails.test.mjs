import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import test from "node:test";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const root = new URL("../", import.meta.url);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

function posixPath(path) {
  return path.replaceAll("\\", "/");
}

async function runDiskGuard(dfOutput, env) {
  const directory = await mkdtemp(fileURLToPath(new URL("../.codex-tmp/disk-guard-test-", import.meta.url)));
  const fakeBin = join(directory, "bin");
  await mkdir(fakeBin);
  await writeFile(join(fakeBin, "df"), `#!/bin/sh\nprintf '%b\\n' '${dfOutput}'\n`, { mode: 0o755 });
  const script = posixPath(fileURLToPath(new URL("../ops/disk-guard.sh", import.meta.url)));
  const path = `/${fakeBin[0].toLowerCase()}${posixPath(fakeBin.slice(2))}`;
  return execFileAsync(bash, ["-c", `export PATH='${path}':\"$PATH\"; '${script}' /srv`], {
    env: { ...process.env, ...env }
  });
}

async function source(path) {
  return (await readFile(new URL(path, root), "utf8")).replaceAll("\r\n", "\n");
}

test("disk guard rejects an unsafe filesystem before deployment work starts", async () => {
  await assert.rejects(
    runDiskGuard("Filesystem 1024-blocks Used Available Capacity Mounted on\\n/dev/test 100 95 5 95% /srv", {
      DISK_WARN_PERCENT: "70",
      DISK_CRITICAL_PERCENT: "80",
      DISK_EMERGENCY_PERCENT: "90",
      DISK_MIN_FREE_BYTES: "10240"
    }),
    (error) => error.code === 2 && /disk_state=EMERGENCY/.test(error.stderr)
  );
});

test("disk guard reports warning without blocking service operation", async () => {
  const { stdout } = await runDiskGuard("Filesystem 1024-blocks Used Available Capacity Mounted on\\n/dev/test 100 75 25 75% /srv", {
    DISK_WARN_PERCENT: "70",
    DISK_CRITICAL_PERCENT: "80",
    DISK_EMERGENCY_PERCENT: "90",
    DISK_MIN_FREE_BYTES: "1024"
  });

  assert.match(stdout, /disk_state=WARNING/);
});

test("production compose bounds logs and exposes a persistent disk monitor", async () => {
  const compose = await source("compose.prod.yml");
  const monitor = await source("ops/disk-monitor.sh");
  for (const service of ["xianzhi-ai", "smartvideo-worker", "postgres", "postgres-backup", "migrate", "redis", "rabbitmq", "minio", "disk-monitor"]) {
    const start = compose.indexOf(`\n  ${service}:\n`);
    assert.ok(start >= 0, `${service} service is missing`);
    const remainder = compose.slice(start + 1);
    const nextService = remainder.search(/\n  [a-z0-9-]+:\n/);
    const block = nextService >= 0 ? remainder.slice(0, nextService) : remainder;
    assert.match(block, /\n    logging:\n      driver: "local"\n      options:\n        max-size: "\$\{DOCKER_LOG_MAX_SIZE:-20m\}"\n        max-file: "\$\{DOCKER_LOG_MAX_FILE:-5\}"/);
  }
  assert.match(compose, /\n  disk-monitor:\n[\s\S]*ops\/disk-monitor\.sh:\/usr\/local\/bin\/disk-monitor\.sh:ro/);
  assert.match(compose, /- disk-monitor-probe:\/host-volume:ro/);
  assert.doesNotMatch(compose, /- \/:\/host-volume/);
  assert.match(compose, /test: \["CMD-SHELL", "grep -Eq '\^\(OK\|WARNING\|CRITICAL\)\$' \/tmp\/disk-state"\]/);
  assert.match(monitor, /result="\$\(sh \/usr\/local\/bin\/disk-guard\.sh "\$target" 2>&1\)"/);
});

test("automatic backups are compressed, validated, atomic, and report expired files without deleting them", async () => {
  const compose = await source("compose.prod.yml");
  const start = compose.indexOf("\n  postgres-backup:\n");
  const end = compose.indexOf("\n  migrate:\n", start);
  const block = compose.slice(start, end);

  assert.match(block, /BACKUP_RETENTION_DAYS: "\$\{BACKUP_RETENTION_DAYS:-30\}"/);
  assert.match(block, /BACKUP_MIN_FREE_BYTES: "\$\{BACKUP_MIN_FREE_BYTES:-10737418240\}"/);
  assert.match(block, /pg_dump[\s\S]*gzip -c > "\$\$temp_file"/);
  assert.match(block, /gzip -t "\$\$temp_file"/);
  assert.match(block, /mv "\$\$temp_file" "\$\$file"/);
  assert.match(block, /expired_backup_count=/);
  assert.match(block, /find \/backups[^\n]+-mtime/);
  assert.doesNotMatch(block, /find \/backups[^\n]+-delete/);
  assert.doesNotMatch(block, /rm [^\n]*xianzhi-/);
});

test("RabbitMQ keeps a production disk reserve and deploy enforces the same hard floor", async () => {
  const compose = await source("compose.prod.yml");
  const rabbitmq = await source("ops/rabbitmq/rabbitmq.conf");
  const deploy = await source("deploy.sh");

  assert.match(compose, /\.\/ops\/rabbitmq\/rabbitmq\.conf:\/etc\/rabbitmq\/rabbitmq\.conf:ro/);
  assert.match(rabbitmq, /^disk_free_limit\.absolute = 2GB$/m);
  assert.match(deploy, /ops\/disk-guard\.sh/);
  assert.ok(deploy.indexOf("ops/disk-guard.sh") < deploy.indexOf("mkdir -p backups/compose"));
});
