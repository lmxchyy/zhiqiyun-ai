import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";

const repoRoot = path.resolve(import.meta.dirname, "..");
const dockerfile = path.join(repoRoot, "ops", "backup-uploader", "Dockerfile");
const requirements = path.join(repoRoot, "ops", "backup-uploader", "requirements.txt");
const pendingUpload = path.join(repoRoot, "ops", "backup-offsite-upload-pending.sh");
const retentionVerify = path.join(repoRoot, "ops", "backup-retention-verify-remote.sh");
const gitignore = readFileSync(path.join(repoRoot, ".gitignore"), "utf8");
const releaseWorkflow = readFileSync(path.join(repoRoot, ".github", "workflows", "backup-uploader-image.yml"), "utf8");
const compose = readFileSync(path.join(repoRoot, "compose.prod.yml"), "utf8");

test("production uploader packaging pins a non-host Python OBS runtime", () => {
  assert.equal(existsSync(dockerfile), true);
  assert.equal(existsSync(requirements), true);
  const dockerfileText = readFileSync(dockerfile, "utf8");
  const requirementsText = readFileSync(requirements, "utf8");
  assert.match(dockerfileText, /^FROM python:3\.11-slim-bookworm$/m);
  assert.match(dockerfileText, /ops\/backup-upload-object-storage\.sh/);
  assert.match(dockerfileText, /backup-uploader-db/);
  assert.match(dockerfileText, /backup-config-clone/);
  assert.match(requirementsText, /^esdk-obs-python==3\.26\.6$/m);
});

test("production uploader is opt-in, one-shot, and isolated from business services", () => {
  assert.match(compose, /backup-uploader:/);
  assert.match(compose, /profiles:\s*\["backup-uploader"\]/);
  assert.match(compose, /read_only:\s*true/);
  assert.match(compose, /\.\/backups\/postgres:\/var\/lib\/zhiqiyun\/backups\/postgres:rw/);
  assert.match(compose, /BACKUP_OBS_ENV_FILE:-\/dev\/null/);
  assert.match(compose, /BACKUP_STORAGE_CONFIG_ID/);
  assert.match(compose, /restart:\s*"no"/);
  assert.doesNotMatch(compose.slice(compose.indexOf("  backup-uploader:")), /\n\s+build:/);
  assert.doesNotMatch(compose, /DeleteObject|DELETE_OBJECT/i);
});

test("backup creation emits local integrity metadata and pending upload requires an immutable image", () => {
  assert.equal(existsSync(pendingUpload), true);
  assert.match(compose, /sha256=\"\$\$\(sha256sum/);
  assert.match(compose, /meta_file=\"\$\$file\.meta\.json\"/);
  const wrapper = readFileSync(pendingUpload, "utf8");
  assert.match(wrapper, /BACKUP_UPLOADER_IMAGE.*@sha256/);
  assert.match(wrapper, /--no-deps backup-uploader/);
  assert.doesNotMatch(wrapper, /docker compose.*--build/);
});

test("retention remote verification is read-only and uses the dedicated uploader", () => {
  const wrapper = readFileSync(retentionVerify, "utf8");
  assert.match(wrapper, /--verify-only/);
  assert.match(wrapper, /--remote-key/);
  assert.match(wrapper, /--expected-size/);
  assert.match(wrapper, /--expected-sha256/);
  assert.doesNotMatch(wrapper, /--upload|--download-to|DeleteObject|putObject|deleteObject/i);
});

test("tracked packaging contains no credential values", () => {
  const trackedFiles = [dockerfile, requirements, path.join(repoRoot, "compose.prod.yml")];
  for (const file of trackedFiles) {
    const text = readFileSync(file, "utf8");
    assert.doesNotMatch(text, /FAKE_SECRET_KEY|AKIA[0-9A-Z]{16}/);
    assert.doesNotMatch(text, /OBS_SECRET_ACCESS_KEY\s*[:=]\s*[^$\s}]+/);
  }
});

test("production OBS secret filename is ignored while the example remains tracked", () => {
  assert.match(gitignore, /\*\*\/backup-obs\.env/);
  assert.equal(existsSync(path.join(repoRoot, "ops", "backup-uploader", "backup-obs.env.example")), true);
});

test("backup uploader release workflow binds image to source SHA and publishes digests only after main push", () => {
  assert.match(releaseWorkflow, /pull_request:/);
  assert.match(releaseWorkflow, /branches: \[main, master\]/);
  assert.match(releaseWorkflow, /ops\/backup-uploader\/Dockerfile/);
  assert.match(releaseWorkflow, /IMAGE_TAG: git-\$\{\{ github\.sha \}\}/);
  assert.match(releaseWorkflow, /ghcr_digest/);
  assert.match(releaseWorkflow, /acr_digest/);
  assert.match(releaseWorkflow, /backup-uploader-release-manifest/);
  assert.doesNotMatch(releaseWorkflow, /backup-uploader:latest/);
});
