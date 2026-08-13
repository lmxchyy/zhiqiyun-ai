import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);

async function source(path) {
  return (await readFile(new URL(path, root), "utf8")).replaceAll("\r\n", "\n");
}

test("production compose caps container logs for every service", async () => {
  const compose = await source("compose.prod.yml");
  const serviceNames = [
    "xianzhi-ai",
    "smartvideo-worker",
    "postgres",
    "postgres-backup",
    "migrate",
    "redis",
    "rabbitmq",
    "minio",
    "disk-monitor"
  ];

  for (const service of serviceNames) {
    const start = compose.indexOf(`\n  ${service}:\n`);
    assert.ok(start >= 0, `${service} service is missing`);
    const rest = compose.slice(start + 1);
    const next = rest.search(/\n  [a-z0-9-]+:\n/);
    const block = next >= 0 ? rest.slice(0, next) : rest;
    assert.match(block, /^\s{4}logging:\n\s{6}driver: "local"\n\s{6}options:\n\s{8}max-size: "\$\{DOCKER_LOG_MAX_SIZE:-20m\}"\n\s{8}max-file: "\$\{DOCKER_LOG_MAX_FILE:-5\}"/m);
  }
});

test("automatic postgres backups are compressed, validated, atomic, and report expired files without deleting them", async () => {
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

test("disk monitor delegates threshold evaluation to the shared disk guard", async () => {
  const compose = await source("compose.prod.yml");
  const guard = await source("ops/disk-guard.sh");
  const monitor = await source("ops/disk-monitor.sh");
  const start = compose.indexOf("\n  disk-monitor:\n");
  const end = compose.indexOf("\nvolumes:\n", start);
  const block = compose.slice(start, end);

  assert.match(block, /DISK_WARN_PERCENT: "\$\{DISK_WARN_PERCENT:-70\}"/);
  assert.match(block, /DISK_CRITICAL_PERCENT: "\$\{DISK_CRITICAL_PERCENT:-80\}"/);
  assert.match(block, /DISK_EMERGENCY_PERCENT: "\$\{DISK_EMERGENCY_PERCENT:-90\}"/);
  assert.match(block, /DISK_MIN_FREE_BYTES: "\$\{DISK_MIN_FREE_BYTES:-10737418240\}"/);
  assert.match(block, /ops\/disk-guard\.sh:\/usr\/local\/bin\/disk-guard\.sh:ro/);
  assert.match(block, /ops\/disk-monitor\.sh:\/usr\/local\/bin\/disk-monitor\.sh:ro/);
  assert.match(block, /grep -Eq '\^\(OK\|WARNING\|CRITICAL\)\$'/);
  assert.match(guard, /state=EMERGENCY/);
  assert.match(guard, /state=CRITICAL/);
  assert.match(guard, /state=WARNING/);
  assert.match(guard, /state=OK/);
  assert.match(monitor, /sh \/usr\/local\/bin\/disk-guard\.sh/);
});

test("rabbitmq persists a production disk watermark", async () => {
  const compose = await source("compose.prod.yml");
  const rabbitmq = await source("ops/rabbitmq/rabbitmq.conf");
  assert.match(compose, /\.\/ops\/rabbitmq\/rabbitmq\.conf:\/etc\/rabbitmq\/rabbitmq\.conf:ro/);
  assert.match(rabbitmq, /^disk_free_limit\.absolute = 2GB$/m);
});

test("deploy stops before backup and build when free disk is below the hard floor", async () => {
  const deploy = await source("deploy.sh");
  const check = deploy.indexOf("ops/disk-guard.sh");
  const composeBackup = deploy.indexOf("mkdir -p backups/compose");
  const build = deploy.indexOf("up -d --build --remove-orphans");

  assert.ok(check >= 0 && check < composeBackup, "disk gate must run before creating deployment backup");
  assert.ok(check < build, "disk gate must run before Docker build");
  assert.match(deploy, /DISK_MIN_FREE_BYTES="\$\{DEPLOY_MIN_FREE_BYTES:-10737418240\}"/);
  assert.match(deploy, /fail "Insufficient disk space for a safe deployment/);
});

test("production environment example exposes storage guardrails", async () => {
  const env = await source(".env.production.example");
  for (const line of [
    "BACKUP_RETENTION_DAYS=30",
    "BACKUP_MIN_FREE_BYTES=10737418240",
    "DOCKER_LOG_MAX_SIZE=20m",
    "DOCKER_LOG_MAX_FILE=5",
    "DEPLOY_MIN_FREE_BYTES=10737418240",
    "DISK_WARN_PERCENT=70",
    "DISK_CRITICAL_PERCENT=80",
    "DISK_EMERGENCY_PERCENT=90",
    "DISK_MIN_FREE_BYTES=10737418240",
    "DISK_MONITOR_INTERVAL_SECONDS=60"
  ]) {
    assert.match(env, new RegExp(`^${line}$`, "m"), `${line} is missing`);
  }
});
