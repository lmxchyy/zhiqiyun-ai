import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);

test("production compose passes personal point expiry worker configuration", async () => {
  const compose = await readFile(new URL("compose.prod.yml", root), "utf8");
  for (const name of [
    "XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED",
    "XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL",
    "XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH"
  ]) {
    assert.match(compose, new RegExp(`^\\s+${name}:`, "m"), `${name} is not passed to xianzhi-ai`);
  }
});

test("production environment example declares explicit expiry worker settings", async () => {
  const env = await readFile(new URL(".env.production.example", root), "utf8");
  assert.match(env, /^XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED=true$/m);
  assert.match(env, /^XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL=1m$/m);
  assert.match(env, /^XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH=100$/m);
});

test("migration one-off fails immediately when a selected SQL file fails", async () => {
  const compose = await readFile(new URL("compose.prod.yml", root), "utf8");
  const normalizedCompose = compose.replaceAll("\r\n", "\n");
  const migrateCommand = normalizedCompose.slice(
    normalizedCompose.indexOf("\n  migrate:\n"),
    normalizedCompose.indexOf("\n  redis:\n")
  );
  const failFastAt = migrateCommand.indexOf("set -eu");
  const loopAt = migrateCommand.indexOf("for name in");
  assert.ok(failFastAt >= 0 && failFastAt < loopAt, "migrate command must enable set -eu before the file loop");
});
