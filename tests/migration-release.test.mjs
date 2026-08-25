import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import test from "node:test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const harness = "tests/migration-release.harness.sh";
// Pin Git Bash explicitly: bare "bash" resolves to the WSL launcher in System32
// when tests execute under the Windows self-hosted runner service account.
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

test("release migration runner selects 109, excludes down files, and is idempotent", () => {
  const output = execFileSync(bash, [harness], { cwd: root, encoding: "utf8" });
  assert.match(output, /migration harness PASS/);
});

test("migration failure blocks application startup through compose dependency", async () => {
  const compose = await readFile(path.join(root, "compose.prod.yml"), "utf8");
  const runner = await readFile(path.join(root, "ops", "run-migrations.sh"), "utf8");
  assert.match(compose, /xianzhi-ai:[\s\S]*depends_on:[\s\S]*migrate:[\s\S]*condition:\s*service_completed_successfully/);
  assert.match(compose, /smartvideo-worker:[\s\S]*depends_on:[\s\S]*migrate:[\s\S]*condition:\s*service_completed_successfully/);
  assert.match(runner, /pg_advisory_lock\(hashtext\('xianzhi:schema_migrations'\)\)/);
  assert.match(runner, /SELECT EXISTS \(SELECT 1 FROM schema_migrations/);
  assert.match(runner, /MIGRATION_FILES baseline is required/);
});
