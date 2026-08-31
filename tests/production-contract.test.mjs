import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { readFileSync } from "node:fs";

const root = new URL("../", import.meta.url);
const harnessPath = new URL("production-contract.harness.sh", new URL("../tests/", import.meta.url));
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

test("production contract harness exists and passes", async () => {
  await access(harnessPath);
  const harness = await readFile(harnessPath, "utf8");
  assert.match(harness, /psql -U contract -d xianzhi_contract -Atqc 'SELECT 1'/);
  const output = execFileSync(bash, ["tests/production-contract.harness.sh"], {
    cwd: fileURLToPath(root),
    encoding: "utf8"
  });
  assert.match(output, /production contract harness PASS/);
});

test("production contract documents the runtime and migration gates", async () => {
  const workflow = await readFile(new URL(".github/workflows/user-core.yml", root), "utf8");
  const docs = await readFile(new URL("../docs/architecture/production-contract-ci.md", import.meta.url), "utf8");
  assert.match(workflow, /^  production-contract:\s*$/m);
  assert.match(docs, /PRODUCTION_CONTRACT_CI_READY/);
  assert.match(docs, /service_completed_successfully/);
});

test("automatic retention requires explicit enablement and remains bounded", () => {
  const source = readFileSync(new URL("../ops/backup-retention-scheduler.sh", import.meta.url), "utf8");
  assert.match(source, /RETENTION_AUTO_APPLY_ENABLED:-false/);
  assert.match(source, /RETENTION_AUTO_MAX_COUNT:-4/);
  assert.match(source, /RETENTION_AUTO_MAX_BYTES:-1073741824/);
  assert.match(source, /MAX_COUNT.*-le 5/);
  assert.match(source, /MAX_BYTES.*-le 1073741824/);
  assert.match(source, /RETENTION_APPLY=NOT_RUN/);
  assert.match(source, /RETENTION_APPLY=FAILED/);
  assert.match(source, /RETENTION_SCHEDULER_LOCK_BUSY|FLOCK_UNAVAILABLE/);
  assert.match(source, /backup-offsite-upload-pending\.sh/);
  assert.match(source, /backup-retention\.sh/);
  assert.doesNotMatch(source, /deleteObject|delete_object|find\s+.*-delete|xargs\s+rm/);
});
