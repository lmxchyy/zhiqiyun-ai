import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";

const repoRoot = path.resolve(import.meta.dirname, "..");
const dockerfile = path.join(repoRoot, "ops", "backup-uploader", "Dockerfile");
const requirements = path.join(repoRoot, "ops", "backup-uploader", "requirements.txt");
const compose = readFileSync(path.join(repoRoot, "compose.prod.yml"), "utf8");

test("production uploader packaging pins a non-host Python OBS runtime", () => {
  assert.equal(existsSync(dockerfile), true);
  assert.equal(existsSync(requirements), true);
  const dockerfileText = readFileSync(dockerfile, "utf8");
  const requirementsText = readFileSync(requirements, "utf8");
  assert.match(dockerfileText, /^FROM python:3\.11-slim-bookworm$/m);
  assert.match(dockerfileText, /ops\/backup-upload-object-storage\.sh/);
  assert.match(dockerfileText, /backup-uploader-db/);
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
  assert.doesNotMatch(compose, /DeleteObject|DELETE_OBJECT/i);
});

test("tracked packaging contains no credential values", () => {
  const trackedFiles = [dockerfile, requirements, path.join(repoRoot, "compose.prod.yml")];
  for (const file of trackedFiles) {
    const text = readFileSync(file, "utf8");
    assert.doesNotMatch(text, /FAKE_SECRET_KEY|AKIA[0-9A-Z]{16}/);
    assert.doesNotMatch(text, /OBS_SECRET_ACCESS_KEY\s*[:=]\s*[^$\s}]+/);
  }
});
