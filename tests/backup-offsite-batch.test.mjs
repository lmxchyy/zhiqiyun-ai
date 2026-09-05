import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const harnessPath = path.join(repoRoot, "tests", "backup-offsite-batch.harness.sh");
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

function runHarness(scenario, envOverrides = {}) {
  return spawnSync(bash, [harnessPath, scenario], {
    cwd: repoRoot,
    encoding: "utf8",
    timeout: 60000,
    env: { ...process.env, BASH_PATH: bash, PATH: `${process.env.PATH || ""}`, ...envOverrides }
  });
}

test("A: invalid + valid + valid - invalid does not block subsequent candidates", () => {
  const result = runHarness("invalid-valid-valid");
  assert.notEqual(result.status, 0, "batch must exit non-zero when invalid candidate exists");
  assert.match(result.stdout, /LOCAL_BACKUP_INVALID/);
  assert.match(result.stdout, /db_20260821_195734\.sql/);
  assert.match(result.stdout, /db_20260822_155244_aaaa000001\.sql\.gz/);
  assert.match(result.stdout, /db_20260823_160000_bbbb000002\.sql\.gz/);
  assert.match(result.stdout, /^TOTAL=3/m);
  assert.match(result.stdout, /^INVALID=1/m);
  assert.match(result.stdout, /^UPLOADED=2/m);
  assert.match(result.stdout, /^FAILED=0/m);
});

test("B: regression - child consumes stdin without breaking outer iterator", () => {
  // The mock uploader in the harness actively drains stdin (cat >/dev/null).
  // With stdin isolation (< /dev/null and fd 9), all 3 candidates must be scanned.
  const result = runHarness("child-consumes-stdin", { MOCK_DRAIN_STDIN: "1" });
  assert.notEqual(result.status, 0, "batch must exit non-zero when invalid candidate exists");
  assert.match(result.stdout, /^TOTAL=3/m, "all 3 candidates must be counted even if child drains stdin");
  assert.match(result.stdout, /^INVALID=1/m);
  assert.match(result.stdout, /^UPLOADED=2/m);
  assert.match(result.stdout, /db_20260822_155244_aaaa000001\.sql\.gz/);
  assert.match(result.stdout, /db_20260823_160000_bbbb000002\.sql\.gz/);
});

test("C: invalid + already verified + valid - all 3 scanned with proper status", () => {
  const result = runHarness("invalid-verified-valid");
  assert.notEqual(result.status, 0, "batch must exit non-zero when invalid candidate exists");
  assert.match(result.stdout, /^TOTAL=3/m);
  assert.match(result.stdout, /^INVALID=1/m);
  assert.match(result.stdout, /^SKIPPED_ALREADY_VERIFIED=1/m);
  assert.match(result.stdout, /^UPLOADED=1/m);
  assert.match(result.stdout, /^FAILED=0/m);
});

test("D: valid + uploader failure + valid - failure does not block subsequent candidate", () => {
  const result = runHarness("valid-failed-valid");
  assert.notEqual(result.status, 0, "batch must exit non-zero when failed candidate exists");
  assert.match(result.stdout, /^TOTAL=3/m);
  assert.match(result.stdout, /^UPLOADED=2/m);
  assert.match(result.stdout, /^FAILED=1/m);
  assert.match(result.stdout, /^INVALID=0/m);
  assert.match(result.stdout, /db_20260824_170000_cccc000003\.sql\.gz/, "third candidate must be scanned");
});

test("E: all valid - complete scan returns exit 0", () => {
  const result = runHarness("all-valid");
  assert.equal(result.status, 0, `all valid batch must exit 0: ${result.stdout}\n${result.stderr}`);
  assert.match(result.stdout, /^TOTAL=2/m);
  assert.match(result.stdout, /^UPLOADED=2/m);
  assert.match(result.stdout, /^INVALID=0/m);
  assert.match(result.stdout, /^FAILED=0/m);
});

test("F: all invalid - complete scan returns non-zero", () => {
  const result = runHarness("all-invalid");
  assert.notEqual(result.status, 0, "all invalid batch must exit non-zero");
  assert.match(result.stdout, /^TOTAL=2/m);
  assert.match(result.stdout, /^INVALID=2/m);
  assert.match(result.stdout, /^UPLOADED=0/m);
  assert.match(result.stdout, /^FAILED=0/m);
});

test("G: filename safety - handles spaces safely via null delimiters", () => {
  const result = runHarness("filename-safety");
  assert.equal(result.status, 0, `safe filename batch must exit 0: ${result.stdout}\n${result.stderr}`);
  assert.match(result.stdout, /^TOTAL=1/m);
  assert.match(result.stdout, /^UPLOADED=1/m);
  assert.match(result.stdout, /safe space/);
});

test("H: no secret leakage - secrets never appear in batch stdout/stderr", () => {
  const result = runHarness("secret-leakage");
  assert.equal(result.status, 0);
  assert.doesNotMatch(result.stdout, /OBS_SECRET_ACCESS_KEY/);
  assert.doesNotMatch(result.stdout, /test-connector-secret-key/);
  assert.doesNotMatch(result.stderr, /OBS_SECRET_ACCESS_KEY/);
  assert.doesNotMatch(result.stderr, /test-connector-secret-key/);
});
