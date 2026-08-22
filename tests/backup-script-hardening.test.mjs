import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const backupPath = path.join(repoRoot, "backup.sh");
const harnessPath = path.join(repoRoot, "tests", "backup-script-hardening.harness.sh");

function findBash() {
  const candidates = [
    process.env.BASH_PATH,
    "bash",
    "C:\\Program Files\\Git\\bin\\bash.exe",
    "C:\\Program Files\\Git\\usr\\bin\\bash.exe",
    "/usr/bin/bash",
    "/bin/bash"
  ].filter(Boolean);

  for (const candidate of candidates) {
    const probe = spawnSync(candidate, ["-c", "echo ok"], { encoding: "utf8" });
    if (probe.status === 0 && String(probe.stdout || "").includes("ok")) {
      return candidate;
    }
  }
  return null;
}

function gitLsFilesMode(relPath) {
  const probe = spawnSync("git", ["ls-files", "-s", "--", relPath], {
    cwd: repoRoot,
    encoding: "utf8"
  });
  assert.equal(probe.status, 0, `git ls-files failed: ${probe.stderr}`);
  const line = String(probe.stdout || "").trim();
  assert.ok(line, `git ls-files returned empty for ${relPath}`);
  return line.split(/\s+/)[0];
}

test("backup.sh hardens deploy snapshots (source + mode + behavior)", () => {
  const source = readFileSync(backupPath, "utf8");

  assert.match(source, /set -euo pipefail/, "must enable pipefail-safe shell mode");
  assert.match(source, /\.sql\.gz/, "final artifact must be .sql.gz");
  assert.match(source, /\.part/, "must use .part temporary file");
  assert.match(source, /gzip -t/, "must validate gzip archive");
  assert.match(source, /test -s/, "must reject empty dumps");
  assert.match(source, /sha256/i, "must record sha256");
  assert.match(source, /meta\.json/, "must write metadata sidecar");
  assert.match(source, /git rev-parse HEAD/, "must capture full git sha");
  assert.match(source, /--short=10/, "must capture short=10 git sha");
  assert.doesNotMatch(
    source,
    />\s*"\$\{?BACKUP_FILE\}?"/,
    "must not redirect pg_dump directly into the final backup path"
  );
  assert.match(
    source,
    /pg_dump[\s\S]*\|\s*gzip/,
    "pg_dump must be piped through gzip"
  );
  assert.match(source, /--clean/, "must keep --clean");
  assert.match(source, /--if-exists/, "must keep --if-exists");
  assert.doesNotMatch(source, /\brm\s+-rf\s+backups/, "must not wipe backups tree");
  assert.doesNotMatch(
    source,
    /find\s+.*backups\/postgres.*-delete/,
    "must not auto-delete historical postgres backups"
  );

  assert.equal(gitLsFilesMode("backup.sh"), "100755", "backup.sh must be Git mode 100755");

  const bash = findBash();
  assert.ok(bash, "bash is required to run backup.sh behavioral harness");
  assert.ok(existsSync(harnessPath), "harness script missing");

  const syntax = spawnSync(bash, ["-n", backupPath], { encoding: "utf8" });
  assert.equal(syntax.status, 0, `bash -n backup.sh failed: ${syntax.stderr}`);

  const harness = spawnSync(bash, [harnessPath], {
    cwd: repoRoot,
    encoding: "utf8",
    env: { ...process.env, BACKUP_SRC: backupPath }
  });
  if (harness.status !== 0) {
    const detail = `${harness.stdout || ""}\n${harness.stderr || ""}`.trim();
    assert.fail(`backup harness failed (exit ${harness.status}):\n${detail}`);
  }
});
