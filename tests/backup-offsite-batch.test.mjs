import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const harnessPath = new URL("backup-offsite-batch.harness.sh", import.meta.url);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

function runHarness(scenario) {
  return spawnSync(bash, [harnessPath.pathname, scenario], {
    cwd: repoRoot,
    encoding: "utf8",
    timeout: 30000,
    env: { ...process.env, BASH_PATH: bash, PATH: `${process.env.PATH || ""}` }
  });
}

test("first invalid backup does not block a second valid backup", () => {
  const result = runHarness("first-invalid-second-valid");
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  assert.match(result.stderr, /LOCAL_BACKUP_INVALID/);
  assert.match(result.stderr, /db_20260821_195734\.sql/);
});

test("all invalid backups scan completely and return non-zero", () => {
  const result = runHarness("all-invalid");
  assert.notEqual(result.status, 0, `${result.stdout}\n${result.stderr}`);
  assert.match(result.stderr, /LOCAL_BACKUP_INVALID/);
});
