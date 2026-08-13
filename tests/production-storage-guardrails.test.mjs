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

test("automatic postgres backups are compressed, validated, atomic, and rotate one file at a time", async () => {
  const compose = await source("compose.prod.yml");
  const start = compose.indexOf("\n  postgres-backup:\n");
  const end = compose.indexOf("\n  migrate:\n", start);
  const block = compose.slice(start, end);

  assert.match(block, /BACKUP_RETENTION_DAYS: "\$\{BACKUP_RETENTION_DAYS:-30\}"/);
  assert.match(block, /BACKUP_MIN_FREE_BYTES: "\$\{BACKUP_MIN_FREE_BYTES:-10737418240\}"/);
  assert.match(block, /pg_dump[\s\S]*gzip -c > "\$\$temp_file"/);
  assert.match(block, /gzip -t "\$\$temp_file"/);
  assert.match(block, /mv "\$\$temp_file" "\$\$file"/);
  assert.match(block, /find \/backups[^\n]+-print -quit/);
  assert.doesNotMatch(block, /find \/backups[^\n]+-delete/);
});

test("disk monitor enforces warning, action, emergency, and recovery thresholds", async () => {
  const compose = await source("compose.prod.yml");
  const start = compose.indexOf("\n  disk-monitor:\n");
  const end = compose.indexOf("\nvolumes:\n", start);
  const block = compose.slice(start, end);

  assert.match(block, /DISK_WARN_PERCENT: "\$\{DISK_WARN_PERCENT:-70\}"/);
  assert.match(block, /DISK_CRITICAL_PERCENT: "\$\{DISK_CRITICAL_PERCENT:-80\}"/);
  assert.match(block, /DISK_EMERGENCY_PERCENT: "\$\{DISK_EMERGENCY_PERCENT:-90\}"/);
  assert.match(block, /DISK_MIN_FREE_BYTES: "\$\{DISK_MIN_FREE_BYTES:-10737418240\}"/);
  assert.match(block, /disk_state=EMERGENCY/);
  assert.match(block, /disk_state=CRITICAL/);
  assert.match(block, /disk_state=WARNING/);
  assert.match(block, /disk_state=OK/);
});

test("rabbitmq persists a production disk watermark", async () => {
  const compose = await source("compose.prod.yml");
  const rabbitmq = await source("ops/rabbitmq/rabbitmq.conf");
  assert.match(compose, /RABBITMQ_DISK_FREE_LIMIT: "\$\{RABBITMQ_DISK_FREE_LIMIT:-2GB\}"/);
  assert.match(compose, /\.\/ops\/rabbitmq\/rabbitmq\.conf:\/etc\/rabbitmq\/rabbitmq\.conf:ro/);
  assert.match(rabbitmq, /^disk_free_limit\.absolute = \$\(RABBITMQ_DISK_FREE_LIMIT\)$/m);
});

test("deploy stops before backup and build when free disk is below the hard floor", async () => {
  const deploy = await source("deploy.sh");
  const check = deploy.indexOf("DEPLOY_MIN_FREE_BYTES");
  const composeBackup = deploy.indexOf("mkdir -p backups/compose");
  const build = deploy.indexOf("up -d --build --remove-orphans");

  assert.ok(check >= 0 && check < composeBackup, "disk gate must run before creating deployment backup");
  assert.ok(check < build, "disk gate must run before Docker build");
  assert.match(deploy, /DEPLOY_MIN_FREE_BYTES_FROM_FILE=.*DEPLOY_MIN_FREE_BYTES.*ENV_FILE/s);
  assert.match(deploy, /DEPLOY_MIN_FREE_BYTES" -gt 0/);
  assert.match(deploy, /fail "Insufficient disk space/);
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
    "DISK_MONITOR_INTERVAL_SECONDS=60",
    "RABBITMQ_DISK_FREE_LIMIT=2GB"
  ]) {
    assert.match(env, new RegExp(`^${line}$`, "m"), `${line} is missing`);
  }
});
