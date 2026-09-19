import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const harness = fileURLToPath(new URL("./backup-retention-dry-run.harness.sh", import.meta.url));
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

test("scheduled retention dry-run reports candidates and preserves the latest backup", () => {
  const output = execFileSync(bash, [harness], { cwd: root, encoding: "utf8" });
  assert.match(output, /^RETENTION_DRY_RUN=PASS$/m);
  assert.match(output, /^RETENTION_TOTAL_COUNT=1$/m);
  assert.match(output, /^RETENTION_CANDIDATE_COUNT=0$/m);
  assert.match(output, /^RETENTION_LATEST_BACKUP_PRESERVED=yes$/m);
});
