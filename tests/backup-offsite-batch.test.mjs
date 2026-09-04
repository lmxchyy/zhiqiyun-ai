import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const harnessPath = path.join(repoRoot, "tests", "backup-offsite-batch.harness.sh");
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

function runHarness(scenario) {
  return spawnSync(bash, [harnessPath, scenario], {
    cwd: repoRoot,
    encoding: "utf8",
    timeout: 60000,
    env: { ...process.env, BASH_PATH: bash, PATH: `${process.env.PATH || ""}` }
  });
}

test("first invalid backup does not block second valid backup", () => {
  const result = runHarness("first-invalid-second-valid");
  // Per batch contract, invalid candidates must not block subsequent valid
  // candidates. The final exit code may be non-zero when invalid candidates
  // exist; what matters is that the second valid backup was processed.
  assert.match(result.stdout, /LOCAL_BACKUP_INVALID/);
  assert.match(result.stdout, /db_20260821_195734\.sql/);
  assert.match(result.stdout, /^TOTAL=/m);
  assert.match(result.stdout, /^INVALID=1/m);
  assert.match(result.stdout, /^UPLOADED=/m);
});

test("all invalid backups scan completely and return non-zero", () => {
  const result = runHarness("all-invalid");
  assert.notEqual(result.status, 0, `${result.stdout}\n${result.stderr}`);
  assert.match(result.stdout, /LOCAL_BACKUP_INVALID/);
  assert.match(result.stdout, /^TOTAL=/m);
  assert.match(result.stdout, /^INVALID=/m);
});
