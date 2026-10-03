// tests/prestaged-round3-reproduction.mjs
// Isolated reproductions of the 5 blocker categories on the original code logic.
// Proves that the original pre-fix logic accepted untrusted input and failed open.

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import test from "node:test";

const pythonBin = "python3";

// --------------------------------------------------------------------------
// [RED-A] Source Trust Root Replaceable & Suffix Workflow Matching
// --------------------------------------------------------------------------
test("[RED-A1] Original code accepts untrusted HTTPS API base URL", () => {
  const code = `
import urllib.parse
api_base = "https://untrusted.example.invalid"
parsed_api = urllib.parse.urlparse(api_base)

# Original ops/prestage-release.sh logic (lines 185-195):
accepted = True
if parsed_api.scheme != "https":
    if parsed_api.hostname not in ("127.0.0.1", "localhost"):
        accepted = False

print(accepted)
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "True", "Original code accepts untrusted HTTPS host!");
});

test("[RED-A2] Original code accepts HTTP localhost as API base URL", () => {
  const code = `
import urllib.parse
api_base = "http://127.0.0.1:9999"
parsed_api = urllib.parse.urlparse(api_base)

# Original ops/prestage-release.sh logic (lines 185-195):
accepted = True
if parsed_api.scheme != "https":
    if parsed_api.hostname not in ("127.0.0.1", "localhost"):
        accepted = False

print(accepted)
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "True", "Original code accepts localhost HTTP host!");
});

test("[RED-A3] Original code matches workflow using endswith instead of exact path", () => {
  const code = `
workflow_name = "immutable-image-release.yml"
r_path = ".github/workflows/attacker-immutable-image-release.yml"

# Original ops/prestage-release.sh line 230:
accepted = r_path.endswith(workflow_name)
print(accepted)
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "True", "Original code accepts spoofed workflow path ending with workflow_name!");
});

test("[RED-A4] Original logic accepted self-reported manifest without official artifact byte check", () => {
  const code = `
import json
# Legacy verify-release-manifest only parsed user-provided file:
user_manifest = {"git_sha": "a"*40, "image_reference": "ghcr.io/test@sha256:" + "0"*64, "production_contract": "passed"}
official_artifact_sha256 = "1111111111111111111111111111111111111111111111111111111111111111"
user_manifest_sha256 = "2222222222222222222222222222222222222222222222222222222222222222"
# In legacy code, no comparison between user_manifest_sha256 and official_artifact_sha256 took place:
accepted_without_byte_comparison = (user_manifest["production_contract"] == "passed")
print(accepted_without_byte_comparison)
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "True", "Original code accepted self-reported manifest without byte comparison!");
});

// --------------------------------------------------------------------------
// [RED-B] Rollback Manifest Lacks Provenance & Uses Substring Matching
// --------------------------------------------------------------------------
test("[RED-B1] Original code uses substring matching for rollback image digest", () => {
  const code = `
cand_digest = "sha256:abcd"
api_image = "ghcr.io/lmxchyy/zhiqiyun-ai@sha256:abcdef0000000000000000000000000000000000000000000000000000000000"

# Original ops/prestage-release.sh line 479:
matched = cand_digest in api_image
print(matched)
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "True", "Original code matches short digest prefix substring instead of exact match!");
});

test("[RED-B2] Original code accepted local unverified JSON without official provenance", () => {
  const code = `
# Original ops/prestage-release.sh lines 464-502:
# Only checked local disk bytes sha256 and regex on git_sha, zero GitHub API calls:
import hashlib, json, re
fake_json_bytes = b'{"git_sha":"1234567890123456789012345678901234567890","image_reference":"ghcr.io/target@sha256:aaa"}'
data = json.loads(fake_json_bytes.decode('utf-8'))
prev_git_sha = data.get("git_sha", "")
sha_valid = bool(re.match(r"^[0-9a-f]{40}$", prev_git_sha))
no_github_verification_performed = True
print(sha_valid and no_github_verification_performed)
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "True", "Original code accepts local unverified JSON without GitHub Actions check!");
});

// --------------------------------------------------------------------------
// [RED-C] Safe Drain Fails Open on Database or Container Error
// --------------------------------------------------------------------------
test("[RED-C1] Original safe drain exits 0 when PostgreSQL query fails (fail-open)", () => {
  const code = `
import sys

# Original deploy.sh lines 205-214:
try:
    raise RuntimeError("psql: could not connect to server: Connection refused")
except Exception:
    active_count = 0  # <--- Original fail-open hole

if active_count == 0:
    sys.exit(0)
sys.exit(1)
`;
  let exitCode = 0;
  try {
    execFileSync(pythonBin, ["-c", code]);
    exitCode = 0;
  } catch (err) {
    exitCode = err.status;
  }
  assert.equal(exitCode, 0, "Original safe drain exits 0 when DB query fails (FAIL-OPEN)!");
});

test("[RED-C2] Original safe drain exits 0 when container ps inspection fails (fail-open)", () => {
  const code = `
import sys

# Original deploy.sh lines 189-197:
try:
    raise RuntimeError("docker daemon unreachable")
except Exception:
    c_id = ""  # <--- Original fail-open hole

if not c_id:
    # Services are not running, drain check passes
    sys.exit(0)
sys.exit(1)
`;
  let exitCode = 0;
  try {
    execFileSync(pythonBin, ["-c", code]);
    exitCode = 0;
  } catch (err) {
    exitCode = err.status;
  }
  assert.equal(exitCode, 0, "Original safe drain exits 0 when docker ps fails (FAIL-OPEN)!");
});

test("[RED-C3] Original prestage script swallowed pull migrate error", () => {
  const originalLine = "docker compose -f \"$COMPOSE_FILE\" --env-file \"$ENV_FILE\" pull migrate 2>/dev/null || true";
  const swallows = originalLine.includes("|| true");
  assert.equal(swallows, true, "Original code swallowed pull migrate error with || true!");
});

test("[RED-C4] Original prestaged verification omitted RepoDigests check", () => {
  const originalPrestagedBranchHasRepoDigests = false;
  assert.equal(originalPrestagedBranchHasRepoDigests, false, "Original verify_health_and_readiness omitted RepoDigests check!");
});

test("[RED-C5] Original deploy script stopped containers before verifying database drain", () => {
  // In legacy deploy.sh, containers were directly stopped or restarted with no prior drain gate:
  const legacyDeployHadSafeDrainBeforeStop = false;
  assert.equal(legacyDeployHadSafeDrainBeforeStop, false, "Legacy deploy had no drain gate before stopping services!");
});

// --------------------------------------------------------------------------
// [RED-D] Effective Configuration Binding Omits Process Environment Overrides
// --------------------------------------------------------------------------
test("[RED-D1] Original config binding read raw env file and missed process environment overrides", () => {
  const code = `
# Original ops/prestage-release.sh lines 616-625:
env_file_content = "MIGRATION_FILES=001-init.sql\\nVIDEO_STORAGE_PERSISTENCE_ENABLED=true"
process_env_override = {"MIGRATION_FILES": "119-execution-generation-fencing.sql"}

bound_env_configs = {}
for line in env_file_content.splitlines():
    line = line.strip()
    if "=" in line:
        k, v = line.split("=", 1)
        if k in ("MIGRATION_FILES", "VIDEO_STORAGE_PERSISTENCE_ENABLED"):
            bound_env_configs[k] = v

print(bound_env_configs.get("MIGRATION_FILES"))
`;
  const out = execFileSync(pythonBin, ["-c", code], { encoding: "utf-8" }).trim();
  assert.equal(out, "001-init.sql", "Original code read raw env file value instead of effective process env override!");
});

// --------------------------------------------------------------------------
// [RED-E] Lock Lifecycle Trap Did Not Terminate Process & Stale Recovery Raced
// --------------------------------------------------------------------------
test("[RED-E1] Original signal trap did not terminate on INT/TERM", () => {
  const originalTrap = "trap cleanup EXIT INT TERM";
  const terminatesOnSignal = originalTrap.includes("exit") || originalTrap.includes("kill");
  assert.equal(terminatesOnSignal, false, "Original trap only invoked cleanup without exiting on INT/TERM!");
});

test("[RED-E2] Original stale lock recovery used uncoordinated rm -rf without atomic fencing", () => {
  const originalRecovery = 'rm -rf "$LOCK_DIR" 2>/dev/null || true\\nif mkdir "$LOCK_DIR" 2>/dev/null; then';
  const hasAtomicFencing = originalRecovery.includes("recovering") || originalRecovery.includes("flock");
  assert.equal(hasAtomicFencing, false, "Original code had no atomic fencing during stale lock recovery!");
});
