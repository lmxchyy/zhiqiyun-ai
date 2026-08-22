import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const harnessPath = path.join(repoRoot, "tests", "backup-offsite-upload.harness.sh");
const uploaderSource = readFileSync(path.join(repoRoot, "ops", "backup-upload-object-storage.sh"), "utf8");

test("uploader source stays compatible with production Python 3.6", () => {
  assert.doesNotMatch(uploaderSource, /datetime\.fromisoformat|zoneinfo|dataclasses|capture_output|gzip\.BadGzipFile/);
});

function findBash() {
  for (const candidate of [process.env.BASH_PATH, "C:\\Program Files\\Git\\bin\\bash.exe", "C:\\Program Files\\Git\\usr\\bin\\bash.exe", "bash"]) {
    if (!candidate) continue;
    const probe = spawnSync(candidate, ["-c", "echo ok"], { encoding: "utf8", timeout: 5000 });
    if (probe.status === 0 && probe.stdout.trim() === "ok") return candidate;
  }
  return null;
}

function runHarness(scenario) {
  const bash = findBash();
  assert.ok(bash, "bash is required");
  return spawnSync(bash, [harnessPath, scenario], {
    cwd: repoRoot,
    encoding: "utf8",
    env: { ...process.env, BASH_PATH: bash, PATH: `C:\\Program Files\\Git\\usr\\bin;${process.env.PATH || ""}` }
  });
}

function expectFailure(scenario, message) {
  const result = runHarness(scenario);
  if (result.stderr.includes("SYMLINK_FIXTURE_UNAVAILABLE")) return;
  assert.notEqual(result.status, 0, `${scenario} unexpectedly succeeded\n${result.stdout}\n${result.stderr}`);
  assert.match(`${result.stdout}\n${result.stderr}`, message);
}

test("valid backup dry-run produces a deterministic deploy object key without uploading", () => {
  const result = runHarness("valid-dry-run");
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  const report = JSON.parse(result.stdout);
  assert.equal(report.status, "DRY_RUN");
  assert.equal(report.object_key, "zhiqiyun-ai/postgres/deploy/2026/08/db_20260822_155244_abcdef1234.sql.gz");
  assert.equal(report.uploaded, false);
});

test("valid fake upload writes OFFSITE_VERIFIED metadata", () => {
  const result = runHarness("valid-upload");
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  const report = JSON.parse(result.stdout);
  assert.equal(report.status, "OFFSITE_VERIFIED");
  assert.equal(report.verification, "OFFSITE_VERIFIED");
  assert.equal(report.local_bytes, report.remote_bytes);
  assert.equal(report.local_sha256, report.remote_sha256);
  assert.match(report.offsite_path, /\.offsite\.json$/);
});

test("identical existing remote object is idempotent", () => {
  const result = runHarness("idempotent");
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  assert.equal(JSON.parse(result.stdout).status, "ALREADY_OFFSITE_VERIFIED");
});

test("conflicting existing remote object is never overwritten", () => {
  expectFailure("remote-conflict", /REMOTE_CONFLICT/);
});

test("remote size mismatch cannot produce verification metadata", () => {
  expectFailure("remote-size-mismatch", /REMOTE_SIZE_MISMATCH/);
});

test("remote checksum mismatch cannot produce verification metadata", () => {
  expectFailure("remote-checksum-mismatch", /REMOTE_CHECKSUM_MISMATCH/);
});

test("missing metadata is rejected before upload", () => {
  expectFailure("missing-meta", /LOCAL_BACKUP_INVALID/);
});

test("sha256 mismatch is rejected before upload", () => {
  expectFailure("sha-mismatch", /LOCAL_BACKUP_INVALID/);
});

test("byte count mismatch is rejected before upload", () => {
  expectFailure("bytes-mismatch", /LOCAL_BACKUP_INVALID/);
});

test("invalid gzip is rejected before upload", () => {
  expectFailure("bad-gzip", /LOCAL_BACKUP_INVALID/);
});

test("symlink input is rejected", () => {
  const result = runHarness("symlink");
  if (result.stderr.includes("SYMLINK_FIXTURE_UNAVAILABLE")) return;
  assert.notEqual(result.status, 0, `${result.stdout}\n${result.stderr}`);
  assert.match(`${result.stdout}\n${result.stderr}`, /LOCAL_BACKUP_INVALID/);
});

test("path outside backup root is rejected", () => {
  expectFailure("outside-root", /LOCAL_BACKUP_INVALID/);
});

test("missing COS configuration never uploads", () => {
  expectFailure("credentials-missing", /UPLOAD_NOT_CONFIGURED/);
});

test("spaces, brackets, and double hyphens remain safe in object keys", () => {
  if (process.platform === "win32") return;
  const result = runHarness("filename-safe");
  if (result.stderr.includes("SPECIAL_FILENAME_FIXTURE_UNAVAILABLE")) return;
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  const report = JSON.parse(result.stdout);
  assert.equal(report.status, "DRY_RUN");
  assert.match(report.object_key, /db_20260822_155244_abcdef \[x\] --\.sql\.gz$/);
  assert.doesNotMatch(report.object_key, /\.\./);
});
