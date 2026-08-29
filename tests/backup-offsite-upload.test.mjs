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

test("large OBS backups use resumable multipart upload", () => {
  assert.match(uploaderSource, /size >= 64 \* 1024 \* 1024/);
  assert.match(uploaderSource, /uploadFile\(self\.bucket, key, source, 16 \* 1024 \* 1024, 1, True/);
});

test("OBS metadata extraction accepts SDK header mapping and preserves fail-closed checks", () => {
  assert.match(uploaderSource, /def response_value\(response, name, default=None\)/);
  assert.match(uploaderSource, /for name, value in mapping_items\(response_value\(response, "header", \[\]\)\)/);
  assert.match(uploaderSource, /normalized_name in \("x-obs-meta-sha256", "sha256"\)/);
  assert.match(uploaderSource, /remote\.get\("sha256"\) != local\["sha256"\]/);
});

test("auxiliary objects use their own content checksums and repair only identical content", () => {
  assert.match(uploaderSource, /digest = hashlib\.sha256\(content\.encode\("utf-8"\)\)\.hexdigest\(\)/);
  assert.match(uploaderSource, /metadata=\{\"x-obs-meta-sha256\": digest\}/);
  assert.match(uploaderSource, /current = self\.get_text\(key\)/);
  assert.match(uploaderSource, /current != content/);
  assert.match(uploaderSource, /fail\("REMOTE_CONFLICT", "OBS sidecar exists with different content"\)/);
  assert.match(uploaderSource, /main_uploaded = False/);
  assert.match(uploaderSource, /main_uploaded or meta_uploaded or checksum_uploaded/);
});

test("OBS sidecar reads support callable and in-memory SDK body forms", () => {
  assert.match(uploaderSource, /reader = getattr\(body, "read", None\)/);
  assert.match(uploaderSource, /if callable\(reader\):/);
  assert.match(uploaderSource, /body = reader\(\)/);
  assert.match(uploaderSource, /body = response_value\(body, "content"\)/);
  assert.match(uploaderSource, /body = response_value\(body, "buffer"\)/);
  assert.match(uploaderSource, /OBS sidecar response body format is unsupported/);
});

test("multipart diagnostics expose only safe lifecycle and part fields", () => {
  for (const event of [
    "MULTIPART_INIT_STARTED",
    "MULTIPART_INIT_COMPLETED",
    "PART_STARTED",
    "PART_COMPLETED",
    "PART_FAILED",
    "MULTIPART_COMPLETE_STARTED",
    "MULTIPART_COMPLETE_COMPLETED",
  ]) {
    assert.match(uploaderSource, new RegExp(event));
  }
  assert.match(uploaderSource, /upload_id_hash/);
  assert.doesNotMatch(uploaderSource, /print\(.*access_key|print\(.*secret_access_key/);
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
  assert.equal(report.object_key, "backups/postgres/deploy/2026/08/db_20260822_155244_abcdef1234.sql.gz");
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

test("OBS fake provider uploads under the fixed postgres prefix", () => {
  const result = runHarness("obs-valid-upload");
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  const report = JSON.parse(result.stdout);
  assert.equal(report.provider, "obs");
  assert.equal(report.status, "OFFSITE_VERIFIED");
  assert.match(report.object_key, /^backups\/postgres\/deploy\/2026\/08\//);
  assert.equal(report.remote_sha256, report.local_sha256);
});

test("OBS callers cannot inject a business prefix", () => {
  const result = runHarness("obs-prefix-injection");
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  assert.match(JSON.parse(result.stdout).object_key, /^backups\/postgres\//);
  assert.doesNotMatch(result.stdout, /images\//);
});

test("OBS idempotency and conflict protection are preserved", () => {
  const identical = runHarness("obs-existing-identical");
  assert.equal(identical.status, 0, `${identical.stdout}\n${identical.stderr}`);
  assert.equal(JSON.parse(identical.stdout).status, "ALREADY_OFFSITE_VERIFIED");
  expectFailure("obs-remote-conflict", /REMOTE_CONFLICT/);
});

for (const scenario of ["obs-auth-failure", "obs-timeout", "obs-network-failure", "obs-partial-upload", "obs-meta-failure", "obs-sha-failure"]) {
  test(`OBS ${scenario} never verifies a partial upload`, () => {
    expectFailure(scenario, /OFFSITE/);
  });
}

test("OBS configuration failures are explicit", () => {
  expectFailure("obs-config-missing", /CONFIG_REQUIRED/);
  for (const scenario of ["obs-bucket-missing", "obs-endpoint-missing", "obs-region-missing"]) {
    expectFailure(scenario, /CONFIG_REQUIRED/);
  }
});

test("OBS provider errors redact credentials", () => {
  const result = runHarness("obs-secret-redaction");
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, /FAKE_ACCESS_KEY|FAKE_SECRET_KEY/);
});

test("database-backed backup config mode is explicit and fail-closed", () => {
  assert.match(uploaderSource, /--storage-config-id/);
  assert.match(uploaderSource, /BACKUP_STORAGE_CONFIG_NOT_FOUND/);
  assert.match(uploaderSource, /backup-uploader-db/);
  assert.match(uploaderSource, /PROVIDER.*obs/);
});
