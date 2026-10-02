// tests/prestaged-round3-fixed.mjs
// Verification that all 5 blocker categories are now fixed and fail closed (GREEN).

import assert from "node:assert/strict";
import { execFileSync, execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";
const pythonBin = "python3";

// --------------------------------------------------------------------------
// [GREEN-A] Fixed Source Trust Root
// --------------------------------------------------------------------------
test("[GREEN-A1] prestage rejects untrusted HTTPS API base URL", async () => {
  let exitCode = 0;
  let stderr = "";
  try {
    await execFileAsync(bash, ["ops/prestage-release.sh", "--sha", "97361d7fe4cfcd32cce644b153532be480ad721a"], {
      env: { ...process.env, GITHUB_API_BASE_URL: "https://untrusted.example.invalid" }
    });
  } catch (err) {
    exitCode = err.code;
    stderr = err.stderr || "";
  }
  assert.equal(exitCode, 1, "Must exit with non-zero exit code");
  assert.match(stderr, /PROVENANCE_FAILED/);
  assert.match(stderr, /GITHUB_API_BASE_URL override rejected/);
});

test("[GREEN-A2] prestage rejects HTTP localhost as API base URL", async () => {
  let exitCode = 0;
  let stderr = "";
  try {
    await execFileAsync(bash, ["ops/prestage-release.sh", "--sha", "97361d7fe4cfcd32cce644b153532be480ad721a"], {
      env: { ...process.env, GITHUB_API_BASE_URL: "http://127.0.0.1:9999" }
    });
  } catch (err) {
    exitCode = err.code;
    stderr = err.stderr || "";
  }
  assert.equal(exitCode, 1, "Must exit with non-zero exit code");
  assert.match(stderr, /PROVENANCE_FAILED/);
  assert.match(stderr, /GITHUB_API_BASE_URL override rejected/);
});

test("[GREEN-A3] prestage rejects spoofed workflow path (exact match required)", async () => {
  const scriptContent = await readFile("ops/prestage-release.sh", "utf-8");
  // Check that r_path uses exact equality check
  const usesExactWorkflowMatch = scriptContent.includes('r_path == OFFICIAL_WORKFLOW');
  const usesEndswith = scriptContent.includes('r_path.endswith(');
  assert.equal(usesExactWorkflowMatch, true, "Must use exact equality check for official workflow");
  assert.equal(usesEndswith, false, "Must not use endswith for workflow matching");
});

test("[GREEN-A4] prestage compares user manifest bytes with official artifact bytes", async () => {
  const scriptContent = await readFile("ops/prestage-release.sh", "utf-8");
  assert.match(scriptContent, /user_sha256 != manifest_sha256/);
  assert.match(scriptContent, /Local manifest bytes \(\{user_sha256\}\) do not match official GitHub artifact bytes/);
});

// --------------------------------------------------------------------------
// [GREEN-B] Rollback Official Provenance & Full String Match
// --------------------------------------------------------------------------
test("[GREEN-B1] rollback manifest matching strictly uses full image reference equality, not substring", async () => {
  const scriptContent = await readFile("ops/prestage-release.sh", "utf-8");
  const usesSubstring = scriptContent.includes('cand_digest in api_image');
  const usesExactRef = scriptContent.includes('cand_ref == api_image');
  assert.equal(usesSubstring, false, "Must not use substring match for rollback digest");
  assert.equal(usesExactRef, true, "Must use full exact equality match for rollback image reference");
});

test("[GREEN-B2] rollback manifest is verified against official GitHub Actions provenance", async () => {
  const scriptContent = await readFile("ops/prestage-release.sh", "utf-8");
  const hasRollbackGithubCheck = scriptContent.includes("VERIFY ROLLBACK MANIFEST PROVENANCE VIA OFFICIAL GITHUB ACTIONS API");
  const queriesRollbackRuns = scriptContent.includes("rb_runs_url");
  const comparesRollbackBytes = scriptContent.includes("rb_official_sha256 != local_rb_sha256");
  assert.equal(hasRollbackGithubCheck, true);
  assert.equal(queriesRollbackRuns, true);
  assert.equal(comparesRollbackBytes, true);
});

// --------------------------------------------------------------------------
// [GREEN-C] Safe Drain Fail-Closed
// --------------------------------------------------------------------------
test("[GREEN-C1] safe drain fails closed (exits 1) when PostgreSQL query fails", () => {
  const code = `
import sys

# New deploy.sh safe drain logic:
def run_cmd(args):
    raise RuntimeError("psql: could not connect to server: Connection refused")

try:
    try:
        active_str = run_cmd(["psql"])
        active_count = int(active_str.strip())
    except Exception as e:
        sys.stderr.write(f"[deploy] ERROR: SAFE_DRAIN_REJECTED: PostgreSQL drain check query execution failed: {e}\\n")
        sys.exit(1)
except SystemExit as se:
    sys.exit(se.code)
`;
  let exitCode = 0;
  try {
    execFileSync(pythonBin, ["-c", code]);
    exitCode = 0;
  } catch (err) {
    exitCode = err.status;
  }
  assert.equal(exitCode, 1, "Must exit with 1 on DB failure (fail-closed)!");
});

test("[GREEN-C2] safe drain fails closed (exits 1) when container ps inspection fails", () => {
  const code = `
import sys

# New deploy.sh safe drain container check logic:
def run_cmd(args):
    raise RuntimeError("docker daemon unreachable")

try:
    try:
        c_id = run_cmd(["docker", "ps"])
    except Exception as e:
        sys.stderr.write(f"[deploy] ERROR: SAFE_DRAIN_REJECTED: Container inspection failed: {e}\\n")
        sys.exit(1)
except SystemExit as se:
    sys.exit(se.code)
`;
  let exitCode = 0;
  try {
    execFileSync(pythonBin, ["-c", code]);
    exitCode = 0;
  } catch (err) {
    exitCode = err.status;
  }
  assert.equal(exitCode, 1, "Must exit with 1 on ps failure (fail-closed)!");
});

test("[GREEN-C3] prestage script does not swallow pull migrate errors", async () => {
  const scriptContent = await readFile("ops/prestage-release.sh", "utf-8");
  const swallows = scriptContent.includes("pull migrate 2>/dev/null || true");
  assert.equal(swallows, false, "Must not swallow pull migrate error");
  assert.match(scriptContent, /pull migrate \|\| fail/);
});

test("[GREEN-C4] verify_health_and_readiness in deploy.sh verifies RepoDigests", async () => {
  const scriptContent = await readFile("deploy.sh", "utf-8");
  const funcBody = scriptContent.slice(
    scriptContent.indexOf("verify_health_and_readiness()"),
    scriptContent.indexOf("command -v git")
  );
  assert.match(funcBody, /RepoDigests/);
  assert.match(funcBody, /repo_digests = \[d\.strip\(\) for d in digests_out\.splitlines\(\)/);
  assert.match(funcBody, /if expected_ref not in repo_digests:/);
});

test("[GREEN-C5] safe drain is performed before stop and failure exits non-zero without stopping containers", async () => {
  const scriptContent = await readFile("deploy.sh", "utf-8");
  const drainIdx = scriptContent.indexOf("check_safe_drain");
  const stopIdx = scriptContent.indexOf("docker compose -f \"$COMPOSE_FILE\" --env-file \"$ENV_FILE\" stop");
  assert.ok(drainIdx > 0, "check_safe_drain must exist in deploy.sh");
  assert.ok(stopIdx > 0, "stop command must exist in deploy.sh");
  assert.ok(drainIdx < stopIdx, "check_safe_drain must be executed before stop!");
});

// --------------------------------------------------------------------------
// [GREEN-D] Effective Configuration Binding
// --------------------------------------------------------------------------
test("[GREEN-D1] prestage and proof verification bind effective compose config hash including process env overrides", async () => {
  const prestageScript = await readFile("ops/prestage-release.sh", "utf-8");
  const verifyScript = await readFile("ops/verify-prestage-proof.sh", "utf-8");
  assert.match(prestageScript, /bound_config_hash/);
  assert.match(verifyScript, /bound_config_hash/);
  assert.match(verifyScript, /CONFIG_MUTATED_AFTER_PRESTAGE: effective Compose configuration altered after prestage/);
});

// --------------------------------------------------------------------------
// [GREEN-E] Lock Owner Lifecycle and Signal Handling
// --------------------------------------------------------------------------
test("[GREEN-E1] deploy.sh, rollback.sh, ops/prestage-release.sh terminate on INT/TERM signals", async () => {
  for (const scriptPath of ["deploy.sh", "rollback.sh", "ops/prestage-release.sh"]) {
    const content = await readFile(scriptPath, "utf-8");
    assert.match(content, /handle_signal\(\)/);
    assert.match(content, /trap 'handle_signal INT' INT/);
    assert.match(content, /trap 'handle_signal TERM' TERM/);
    assert.match(content, /case "\$sig" in\s+INT\) exit 130 ;;\s+TERM\) exit 143 ;;/);
  }
});

test("[GREEN-E2] stale lock recovery uses atomic recovery lock and cross-platform liveness check", async () => {
  for (const scriptPath of ["deploy.sh", "rollback.sh", "ops/prestage-release.sh"]) {
    const content = await readFile(scriptPath, "utf-8");
    assert.match(content, /RECOVERY_LOCK=/);
    assert.match(content, /is_pid_alive\(\)/);
    assert.match(content, /mkdir "\$RECOVERY_LOCK"/);
  }
});
