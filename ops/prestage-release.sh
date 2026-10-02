#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
PRESTAGE_DIR="${PRESTAGE_DIR:-.prestage}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-}"
ROLLBACK_MANIFEST="${ROLLBACK_MANIFEST:-}"
RELEASE_REGISTRY="${RELEASE_REGISTRY:-}"
RELEASE_TRUST_KEY_FILE="${RELEASE_TRUST_KEY_FILE:-}"
RELEASE_TRUST_SECRET="${RELEASE_TRUST_SECRET:-}"
PRESTAGE_EXPIRY_HOURS="${PRESTAGE_EXPIRY_HOURS:-24}"
PRESTAGED_RELEASE_SHA="${PRESTAGED_RELEASE_SHA:-}"

OFFICIAL_API_BASE="https://api.github.com"
OFFICIAL_REPOSITORY="lmxchyy/zhiqiyun-ai"
OFFICIAL_WORKFLOW=".github/workflows/immutable-image-release.yml"

# Explicitly reject untrusted trust root overrides in environment
if [ -n "${GITHUB_API_BASE_URL:-}" ] && [ "$GITHUB_API_BASE_URL" != "$OFFICIAL_API_BASE" ]; then
  printf '[prestage] ERROR: PROVENANCE_FAILED: GITHUB_API_BASE_URL override rejected: %s. Official %s is strictly required.\n' "$GITHUB_API_BASE_URL" "$OFFICIAL_API_BASE" >&2
  exit 1
fi
if [ -n "${GITHUB_REPOSITORY:-}" ] && [ "$GITHUB_REPOSITORY" != "$OFFICIAL_REPOSITORY" ]; then
  printf '[prestage] ERROR: PROVENANCE_FAILED: GITHUB_REPOSITORY override rejected: %s. Only %s is permitted.\n' "$GITHUB_REPOSITORY" "$OFFICIAL_REPOSITORY" >&2
  exit 1
fi
if [ -n "${GITHUB_WORKFLOW:-}" ] && [ "$GITHUB_WORKFLOW" != "$OFFICIAL_WORKFLOW" ]; then
  printf '[prestage] ERROR: PROVENANCE_FAILED: GITHUB_WORKFLOW override rejected: %s. Only %s is permitted.\n' "$GITHUB_WORKFLOW" "$OFFICIAL_WORKFLOW" >&2
  exit 1
fi

LOCK_DIR="${PRESTAGE_DIR}/release.lock"
RECOVERY_LOCK="${PRESTAGE_DIR}/release.lock.recovering"
OWNER_TOKEN="${BASHPID:-$$}_$(date +%s%N 2>/dev/null || date +%s)_$RANDOM"
IS_LOCK_OWNER=0
TMP_PROOF=""

log() {
  printf '[prestage] %s\n' "$*"
}

fail() {
  printf '[prestage] ERROR: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [ -n "$TMP_PROOF" ] && [ -f "$TMP_PROOF" ]; then
    rm -f "$TMP_PROOF" 2>/dev/null || true
  fi
  if [ "$IS_LOCK_OWNER" = "1" ]; then
    if [ -f "${LOCK_DIR}/owner_token" ]; then
      local current_token
      current_token="$(cat "${LOCK_DIR}/owner_token" 2>/dev/null || true)"
      if [ "$current_token" = "$OWNER_TOKEN" ]; then
        rm -rf "$LOCK_DIR" 2>/dev/null || true
      fi
    fi
  fi
}

handle_signal() {
  local sig="$1"
  trap - "$sig" EXIT
  cleanup
  log "Terminated by signal $sig."
  case "$sig" in
    INT) exit 130 ;;
    TERM) exit 143 ;;
    *) exit 1 ;;
  esac
}
trap 'handle_signal INT' INT
trap 'handle_signal TERM' TERM
trap cleanup EXIT

# Parse command line flags
while [ $# -gt 0 ]; do
  case "$1" in
    -m|--manifest)
      RELEASE_MANIFEST="$2"
      shift 2
      ;;
    --rollback-manifest)
      ROLLBACK_MANIFEST="$2"
      shift 2
      ;;
    -r|--registry)
      RELEASE_REGISTRY="$2"
      shift 2
      ;;
    -c|--compose-file)
      COMPOSE_FILE="$2"
      shift 2
      ;;
    -e|--env-file)
      ENV_FILE="$2"
      shift 2
      ;;
    --prestage-dir)
      PRESTAGE_DIR="$2"
      shift 2
      ;;
    --trust-key-file)
      RELEASE_TRUST_KEY_FILE="$2"
      shift 2
      ;;
    --expiry-hours)
      PRESTAGE_EXPIRY_HOURS="$2"
      shift 2
      ;;
    --sha)
      PRESTAGED_RELEASE_SHA="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: ./ops/prestage-release.sh <PRESTAGED_RELEASE_SHA> [OPTIONS]"
      exit 0
      ;;
    *)
      if [ -z "$PRESTAGED_RELEASE_SHA" ] && [[ "$1" =~ ^[0-9a-f]{40}$ ]]; then
        PRESTAGED_RELEASE_SHA="$1"
        shift
      else
        fail "Unexpected argument: $1"
      fi
      ;;
  esac
done

[ -n "$PRESTAGED_RELEASE_SHA" ] || fail "PRESTAGED_RELEASE_SHA is required (40-char lowercase hex)."
[[ "$PRESTAGED_RELEASE_SHA" =~ ^[0-9a-f]{40}$ ]] || fail "PRESTAGED_RELEASE_SHA must be a 40-character lowercase hexadecimal SHA."

is_pid_alive() {
  local pid="$1"
  # Only ESRCH proves death; every other observation error fails closed.
  python3 -c 'import os,sys
try:
 p=int(sys.argv[1])
 if p<=0: sys.exit(0)
 if os.name=="nt":
  import ctypes
  k=ctypes.WinDLL("kernel32",use_last_error=True)
  k.OpenProcess.restype=ctypes.c_void_p
  k.CloseHandle.argtypes=[ctypes.c_void_p]
  h=k.OpenProcess(0x1000,False,p)
  if not h: sys.exit(1 if ctypes.get_last_error()==87 else 0)
  k.CloseHandle(h)
  sys.exit(0)
 os.kill(p,0)
except ProcessLookupError: sys.exit(1)
except Exception: sys.exit(0)' "$pid" 2>/dev/null
}

# Acquire owner-only concurrency lock inside prestage dir with TOCTOU-safe atomic recovery
acquire_release_lock() {
  mkdir -p "$PRESTAGE_DIR"
  if mkdir "$LOCK_DIR" 2>/dev/null; then
    IS_LOCK_OWNER=1
    echo "$OWNER_TOKEN" > "$LOCK_DIR/owner_token"
    echo "${BASHPID:-$$}" > "$LOCK_DIR/owner_pid"
    date -u +%Y-%m-%dT%H:%M:%SZ > "$LOCK_DIR/created_at"
    return 0
  fi

  local existing_pid="" existing_token=""
  existing_pid="$(cat "$LOCK_DIR/owner_pid" 2>/dev/null || true)"
  existing_token="$(cat "$LOCK_DIR/owner_token" 2>/dev/null || true)"
  if [ -n "$existing_pid" ] && ! is_pid_alive "$existing_pid"; then
    log "Detected dead lock owner (PID $existing_pid). Attempting atomic stale lock recovery..."
    if mkdir "$RECOVERY_LOCK" 2>/dev/null; then
      local recheck_pid recheck_token
      recheck_pid="$(cat "$LOCK_DIR/owner_pid" 2>/dev/null || true)"
      recheck_token="$(cat "$LOCK_DIR/owner_token" 2>/dev/null || true)"
      if [ "$recheck_pid" = "$existing_pid" ] && [ "$recheck_token" = "$existing_token" ] && ! is_pid_alive "$recheck_pid"; then
        rm -rf "$LOCK_DIR" 2>/dev/null || true
        if mkdir "$LOCK_DIR" 2>/dev/null; then
          IS_LOCK_OWNER=1
          echo "$OWNER_TOKEN" > "$LOCK_DIR/owner_token"
          echo "${BASHPID:-$$}" > "$LOCK_DIR/owner_pid"
          date -u +%Y-%m-%dT%H:%M:%SZ > "$LOCK_DIR/created_at"
          rm -rf "$RECOVERY_LOCK" 2>/dev/null || true
          log "Stale lock safely recovered by PID ${BASHPID:-$$}."
          return 0
        fi
      fi
      rm -rf "$RECOVERY_LOCK" 2>/dev/null || true
    fi
  fi

  fail "CONCURRENCY_LOCKED: Another release process currently holds the lock."
}

acquire_release_lock

command -v git >/dev/null 2>&1 || fail "git is not installed."
command -v docker >/dev/null 2>&1 || fail "Docker is not installed."
command -v python3 >/dev/null 2>&1 || fail "python3 is required."
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is not available."

log "Prestaging release for commit: $PRESTAGED_RELEASE_SHA"
log "Compose file: $COMPOSE_FILE"
log "Env file: $ENV_FILE"

[ -f "$ENV_FILE" ] || fail "$ENV_FILE does not exist. Create it and fill in production values first."
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE does not exist."

# Working tree clean check: fail closed on dirty or untracked files
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  fail "Working tree contains uncommitted or untracked changes. Clean working tree before prestaging."
fi

# Ensure commit is in git database
if ! git cat-file -e "${PRESTAGED_RELEASE_SHA}^{commit}" 2>/dev/null; then
  log "Commit $PRESTAGED_RELEASE_SHA not found in local git object database. Fetching from $GIT_REMOTE..."
  git fetch "$GIT_REMOTE" "$PRESTAGED_RELEASE_SHA" 2>/dev/null || git fetch "$GIT_REMOTE" || fail "Failed to fetch commit $PRESTAGED_RELEASE_SHA from $GIT_REMOTE."
  git cat-file -e "${PRESTAGED_RELEASE_SHA}^{commit}" 2>/dev/null || fail "Commit $PRESTAGED_RELEASE_SHA still not found after fetch."
fi

# Prepare prestage target directory
PRESTAGE_TARGET_DIR="${PRESTAGE_DIR}/${PRESTAGED_RELEASE_SHA}"
mkdir -p "$PRESTAGE_TARGET_DIR"

# Stage 1: Official GitHub Provenance Chain Verification
log "Verifying official GitHub Actions provenance chain for commit $PRESTAGED_RELEASE_SHA..."
PROVENANCE_OUTPUT_JSON="${PRESTAGE_TARGET_DIR}/provenance-result.json"

GITHUB_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-${RELEASE_GITHUB_TOKEN:-}}}" \
python3 - "$PRESTAGED_RELEASE_SHA" "$RELEASE_MANIFEST" "$PRESTAGE_TARGET_DIR" "$RELEASE_REGISTRY" "$PROVENANCE_OUTPUT_JSON" <<'PY'
import hashlib
import io
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import zipfile

sha, user_manifest_path, target_dir, registry_choice, out_json_path = sys.argv[1:6]
token = os.environ.get("GITHUB_TOKEN", "").strip()

OFFICIAL_API_BASE = "https://api.github.com"
OFFICIAL_REPOSITORY = "lmxchyy/zhiqiyun-ai"
OFFICIAL_WORKFLOW = ".github/workflows/immutable-image-release.yml"

# Reject any environment attempt to override trust root
env_api = os.environ.get("GITHUB_API_BASE_URL", "").strip()
if env_api and env_api.rstrip("/") != OFFICIAL_API_BASE:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GITHUB_API_BASE_URL override rejected: {env_api}. Only {OFFICIAL_API_BASE} is allowed.\n")
    sys.exit(1)

env_repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
if env_repo and env_repo != OFFICIAL_REPOSITORY:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GITHUB_REPOSITORY override rejected: {env_repo}. Only {OFFICIAL_REPOSITORY} is allowed.\n")
    sys.exit(1)

env_wf = os.environ.get("GITHUB_WORKFLOW", "").strip()
if env_wf and env_wf != OFFICIAL_WORKFLOW:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GITHUB_WORKFLOW override rejected: {env_wf}. Only {OFFICIAL_WORKFLOW} is allowed.\n")
    sys.exit(1)

def http_get(url):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https":
        sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: URL must be HTTPS: {url}\n")
        sys.exit(1)
    if parsed.hostname != "api.github.com":
        sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Untrusted host in initial request: {parsed.hostname}. Only api.github.com is permitted.\n")
        sys.exit(1)

    req = urllib.request.Request(url)
    req.add_header("User-Agent", "xianzhi-release-verifier")
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    if token and parsed.hostname == "api.github.com":
        req.add_header("Authorization", f"Bearer {token}")

    class SafeRedirectHandler(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            if not str(newurl).startswith("https://"):
                sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Insecure redirect to non-HTTPS URL rejected: {newurl}\n")
                sys.exit(1)
            new_req = super().redirect_request(req, fp, code, msg, headers, newurl)
            orig_host = urllib.parse.urlparse(req.full_url).hostname
            new_host = urllib.parse.urlparse(newurl).hostname
            if orig_host and new_host and orig_host.lower() != new_host.lower():
                new_req.headers.pop("Authorization", None)
                new_req.headers.pop("authorization", None)
            return new_req

    opener = urllib.request.build_opener(SafeRedirectHandler())
    try:
        with opener.open(req, timeout=30) as resp:
            return resp.read()
    except urllib.error.HTTPError as e:
        sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GitHub API request failed ({url}): HTTP {e.code} {e.reason}\n")
        sys.exit(1)
    except Exception as e:
        sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GitHub API request failed ({url}): {e}\n")
        sys.exit(1)

# 1. Query workflow runs for commit
runs_url = f"{OFFICIAL_API_BASE}/repos/{OFFICIAL_REPOSITORY}/actions/runs?head_sha={sha}&event=push"
runs_data = json.loads(http_get(runs_url).decode("utf-8"))
workflow_runs = runs_data.get("workflow_runs", [])

matching_run = None
for r in workflow_runs:
    r_sha = r.get("head_sha")
    r_event = r.get("event")
    r_branch = r.get("head_branch")
    r_path = r.get("path", "")
    r_status = r.get("status")
    r_conclusion = r.get("conclusion")
    if r_sha == sha and r_event == "push" and r_branch in ("main", "master") and r_status == "completed" and r_conclusion == "success":
        if r_path == OFFICIAL_WORKFLOW:
            matching_run = r
            break

if not matching_run:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: No successful push run of {OFFICIAL_WORKFLOW} found on main/master for commit {sha}\n")
    sys.exit(1)

run_id = matching_run["id"]
run_attempt = matching_run.get("run_attempt", 1)
run_branch = matching_run.get("head_branch")

# 2. Query artifacts for the matching run
artifacts_url = f"{OFFICIAL_API_BASE}/repos/{OFFICIAL_REPOSITORY}/actions/runs/{run_id}/artifacts"
artifacts_data = json.loads(http_get(artifacts_url).decode("utf-8"))
artifacts = artifacts_data.get("artifacts", [])

matching_artifact = None
expected_names = (f"release-manifest-{sha}", "release-manifest")
for a in artifacts:
    a_name = a.get("name")
    a_expired = a.get("expired", False)
    w_run = a.get("workflow_run", {})
    if a_name in expected_names and not a_expired:
        if not w_run or w_run.get("id") == run_id:
            matching_artifact = a
            break

if not matching_artifact:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Valid non-expired release-manifest artifact not found for run {run_id}\n")
    sys.exit(1)

artifact_id = matching_artifact["id"]
artifact_name = matching_artifact["name"]
download_url = f"{OFFICIAL_API_BASE}/repos/{OFFICIAL_REPOSITORY}/actions/artifacts/{artifact_id}/zip"

# 3. Download artifact zip and extract release-manifest.json
zip_bytes = http_get(download_url)
manifest_bytes = None
try:
    with zipfile.ZipFile(io.BytesIO(zip_bytes)) as zf:
        namelist = zf.namelist()
        manifest_filename = None
        for fn in (f"release-manifest-{sha}.json", "release-manifest.json"):
            if fn in namelist:
                manifest_filename = fn
                break
        if not manifest_filename:
            for fn in namelist:
                if fn.endswith(".json"):
                    manifest_filename = fn
                    break
        if not manifest_filename:
            sys.stderr.write("[prestage] ERROR: PROVENANCE_FAILED: Artifact zip does not contain a JSON manifest\n")
            sys.exit(1)
        manifest_bytes = zf.read(manifest_filename)
except Exception as e:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Failed to unpack artifact zip: {e}\n")
    sys.exit(1)

manifest_sha256 = hashlib.sha256(manifest_bytes).hexdigest()

# Validate manifest content
try:
    manifest_data = json.loads(manifest_bytes.decode("utf-8"))
except Exception as e:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Artifact manifest is invalid JSON: {e}\n")
    sys.exit(1)

if manifest_data.get("git_sha") != sha:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Manifest git_sha ({manifest_data.get('git_sha')}) does not match {sha}\n")
    sys.exit(1)

# Registry selection support (GHCR or aliyun_acr)
selected_registry = registry_choice.strip()
if selected_registry:
    reg_entry = manifest_data.get("registries", {}).get(selected_registry)
    if not isinstance(reg_entry, dict):
        sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Selected registry {selected_registry} not found in manifest\n")
        sys.exit(1)
    image_ref = reg_entry.get("image_reference", "")
    digest = reg_entry.get("digest", "")
else:
    image_ref = manifest_data.get("image_reference", "")
    digest = manifest_data.get("digest", "")

if not re.match(r"^sha256:[0-9a-f]{64}$", str(digest)):
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Manifest contains invalid digest: {digest}\n")
    sys.exit(1)

if not image_ref or not image_ref.endswith(f"@{digest}"):
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Manifest image_reference does not pin digest: {image_ref}\n")
    sys.exit(1)

if manifest_data.get("production_contract") != "passed":
    sys.stderr.write("[prestage] ERROR: PROVENANCE_FAILED: Manifest production_contract is not 'passed'\n")
    sys.exit(1)

# If user provided a local manifest file, verify byte equality
if user_manifest_path and os.path.isfile(user_manifest_path):
    with open(user_manifest_path, "rb") as uf:
        user_bytes = uf.read()
    user_sha256 = hashlib.sha256(user_bytes).hexdigest()
    if user_sha256 != manifest_sha256:
        sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: Local manifest bytes ({user_sha256}) do not match official GitHub artifact bytes ({manifest_sha256})\n")
        sys.exit(1)

# Save the verified official manifest
official_manifest_file = os.path.join(target_dir, "release-manifest.json")
with open(official_manifest_file, "wb") as mf:
    mf.write(manifest_bytes)

provenance = {
    "repository": OFFICIAL_REPOSITORY,
    "workflow": OFFICIAL_WORKFLOW,
    "run_id": run_id,
    "run_attempt": run_attempt,
    "branch": run_branch,
    "event": "push",
    "head_sha": sha,
    "artifact_id": artifact_id,
    "artifact_name": artifact_name,
    "artifact_expired": False,
    "manifest_bytes_sha256": manifest_sha256,
    "image_reference": image_ref,
    "image_digest": digest,
    "manifest_path": official_manifest_file
}

with open(out_json_path, "w", encoding="utf-8") as out:
    json.dump(provenance, out, indent=2, sort_keys=True)
PY

[ -f "$PROVENANCE_OUTPUT_JSON" ] || fail "Provenance verification did not produce expected output."

XIANZHI_IMAGE_REFERENCE="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d["image_reference"])' "$PROVENANCE_OUTPUT_JSON")"
export XIANZHI_IMAGE_REFERENCE
RELEASE_MANIFEST="${PRESTAGE_TARGET_DIR}/release-manifest.json"

case "$XIANZHI_IMAGE_REFERENCE" in
  *@sha256:*) ;;
  *) fail "Verified manifest did not provide a digest-pinned image reference: $XIANZHI_IMAGE_REFERENCE" ;;
esac
log "Verified immutable image reference: $XIANZHI_IMAGE_REFERENCE"

# Disk capacity gate
log "Checking disk capacity..."
DISK_WARN_PERCENT="${DISK_WARN_PERCENT:-70}" \
DISK_CRITICAL_PERCENT="${DISK_CRITICAL_PERCENT:-80}" \
DISK_EMERGENCY_PERCENT="${DISK_EMERGENCY_PERCENT:-90}" \
DISK_MIN_FREE_BYTES="${DEPLOY_MIN_FREE_BYTES:-10737418240}" \
  sh ops/disk-guard.sh "$SCRIPT_DIR" || fail "Insufficient disk space for prestaging."

# Pre-pull immutable image
log "Pre-pulling immutable image: $XIANZHI_IMAGE_REFERENCE"
docker pull "$XIANZHI_IMAGE_REFERENCE" || fail "Failed to pull immutable image $XIANZHI_IMAGE_REFERENCE."

local_image_id="$(docker image inspect "$XIANZHI_IMAGE_REFERENCE" --format '{{.Id}}' 2>/dev/null)" || fail "Cannot inspect image $XIANZHI_IMAGE_REFERENCE locally."
[ -n "$local_image_id" ] || fail "Image $XIANZHI_IMAGE_REFERENCE has no local image ID."

# Pre-pull dependent images from compose (e.g. migrate runner) - fail closed on error
log "Pre-pulling compose dependencies (migrate runner)..."
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull migrate || fail "Failed to pull compose dependency (migrate)."

# Preflight render & validate compose
log "Validating Compose configuration and runtime gates..."
compose_config="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" \
  || fail "Failed to render Docker Compose configuration."

python3 -c '
import json, sys
data = json.loads(sys.stdin.read())
service = data.get("services", {}).get("xianzhi-ai", {})
env = service.get("environment", {})
actual = env.get("VIDEO_STORAGE_PERSISTENCE_ENABLED") if isinstance(env, dict) else None
if str(actual).strip().lower() != "true":
    raise SystemExit("VIDEO_STORAGE_PERSISTENCE_ENABLED must resolve to true for xianzhi-ai")
' <<< "$compose_config" || fail "Video storage persistence gate failed."

# Rollback Receipt & Running Digest Binding (Verified Official Provenance for Rollback)
ROLLBACK_RECEIPT_PATH="${PRESTAGE_TARGET_DIR}/rollback-receipt.json"
log "Preparing and verifying rollback receipt with official provenance..."
GITHUB_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-${RELEASE_GITHUB_TOKEN:-}}}" \
python3 - "$ROLLBACK_RECEIPT_PATH" "$PRESTAGED_RELEASE_SHA" "$COMPOSE_FILE" "$ENV_FILE" "$ROLLBACK_MANIFEST" "$PRESTAGE_DIR" "$RELEASE_REGISTRY" "$PRESTAGE_TARGET_DIR" <<'PY'
import datetime, hashlib, io, json, os, re, shlex, shutil, subprocess, sys, urllib.error, urllib.parse, urllib.request, zipfile

(
    receipt_path,
    target_sha,
    compose_file,
    env_file,
    user_rollback_manifest,
    prestage_dir,
    registry_choice,
    target_dir
) = sys.argv[1:9]

token = os.environ.get("GITHUB_TOKEN", "").strip()
OFFICIAL_API_BASE = "https://api.github.com"
OFFICIAL_REPOSITORY = "lmxchyy/zhiqiyun-ai"
OFFICIAL_WORKFLOW = ".github/workflows/immutable-image-release.yml"

# Reject any environment attempt to override trust root in rollback verification
env_api = os.environ.get("GITHUB_API_BASE_URL", "").strip()
if env_api and env_api.rstrip("/") != OFFICIAL_API_BASE:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GITHUB_API_BASE_URL override rejected: {env_api}. Only {OFFICIAL_API_BASE} is allowed.\n")
    sys.exit(1)

env_repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
if env_repo and env_repo != OFFICIAL_REPOSITORY:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GITHUB_REPOSITORY override rejected: {env_repo}. Only {OFFICIAL_REPOSITORY} is allowed.\n")
    sys.exit(1)

env_wf = os.environ.get("GITHUB_WORKFLOW", "").strip()
if env_wf and env_wf != OFFICIAL_WORKFLOW:
    sys.stderr.write(f"[prestage] ERROR: PROVENANCE_FAILED: GITHUB_WORKFLOW override rejected: {env_wf}. Only {OFFICIAL_WORKFLOW} is allowed.\n")
    sys.exit(1)

def fail(msg):
    sys.stderr.write(f"[prestage] ERROR: ROLLBACK_RECEIPT_ERROR: {msg}\n")
    sys.exit(1)

def run_cmd(args, env_override=None, env_remove=None):
    cmd_env = dict(os.environ)
    if env_remove:
        for k in env_remove:
            cmd_env.pop(k, None)
    if env_override:
        cmd_env.update(env_override)
    if os.name == "nt":
        sh_bin = os.environ.get("SH_EXE") or shutil.which("sh") or "C:/Program Files/Git/bin/sh.exe"
        cmd_str = " ".join(shlex.quote(a) for a in args)
        return subprocess.check_output([sh_bin, "-c", cmd_str], stderr=subprocess.PIPE, env=cmd_env).decode("utf-8").strip()
    return subprocess.check_output(args, stderr=subprocess.PIPE, env=cmd_env).decode("utf-8").strip()

def http_get(url):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https":
        fail(f"URL must be HTTPS: {url}")
    if parsed.hostname != "api.github.com":
        fail(f"Untrusted host in initial request: {parsed.hostname}. Only api.github.com is permitted.")
    req = urllib.request.Request(url)
    req.add_header("User-Agent", "xianzhi-release-verifier")
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    if token and parsed.hostname == "api.github.com":
        req.add_header("Authorization", f"Bearer {token}")

    class SafeRedirectHandler(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            if not str(newurl).startswith("https://"):
                fail(f"Insecure redirect to non-HTTPS URL: {newurl}")
            new_req = super().redirect_request(req, fp, code, msg, headers, newurl)
            orig_host = urllib.parse.urlparse(req.full_url).hostname
            new_host = urllib.parse.urlparse(newurl).hostname
            if orig_host and new_host and orig_host.lower() != new_host.lower():
                new_req.headers.pop("Authorization", None)
                new_req.headers.pop("authorization", None)
            return new_req

    opener = urllib.request.build_opener(SafeRedirectHandler())
    try:
        with opener.open(req, timeout=30) as resp:
            return resp.read()
    except Exception as e:
        fail(f"GitHub API request failed for rollback provenance ({url}): {e}")

# 1. Query currently running API and worker containers
try:
    api_cid = run_cmd(["docker", "compose", "-f", compose_file, "--env-file", env_file, "ps", "-q", "xianzhi-ai"])
    worker_cid = run_cmd(["docker", "compose", "-f", compose_file, "--env-file", env_file, "ps", "-q", "smartvideo-worker"])
except Exception as e:
    fail(f"Cannot inspect running containers: {e}")

if not api_cid:
    fail("No running container found for xianzhi-ai. Running production services must be active to snapshot rollback state.")
if not worker_cid:
    fail("No running container found for smartvideo-worker. Running production services must be active to snapshot rollback state.")

api_image = run_cmd(["docker", "inspect", "--format", "{{.Config.Image}}", api_cid])
api_id = run_cmd(["docker", "inspect", "--format", "{{.Image}}", api_cid])
worker_image = run_cmd(["docker", "inspect", "--format", "{{.Config.Image}}", worker_cid])
worker_id = run_cmd(["docker", "inspect", "--format", "{{.Image}}", worker_cid])

if not api_id or not api_image:
    fail("Cannot inspect running xianzhi-ai image reference or ID.")
if api_id != worker_id:
    fail(f"PARTIAL_RELEASE_DETECTED: running API image ID ({api_id}) does not match worker image ID ({worker_id})")
if api_image != worker_image:
    fail(f"PARTIAL_RELEASE_DETECTED: running API Config.Image ({api_image}) does not match worker Config.Image ({worker_image})")

if not re.match(r"^.+@sha256:[0-9a-f]{64}$", api_image):
    fail(f"Running API image reference is not digest-pinned: {api_image}")

# Verify running image exists locally in Docker and get repo digests
try:
    digests_out = run_cmd(["docker", "image", "inspect", api_id, "--format", "{{range .RepoDigests}}{{println .}}{{end}}"])
    repo_digests = [d.strip() for d in digests_out.splitlines() if d.strip()]
except Exception as e:
    fail(f"Cannot inspect RepoDigests for running image {api_id}: {e}")

if not repo_digests:
    fail(f"Running image {api_id} has no RepoDigests. It must be a digest-pinned immutable image.")

if api_image not in repo_digests:
    fail(f"Running image {api_image} is not in RepoDigests: {repo_digests}")

# Verify current old Compose desired state (must NOT be overridden by target XIANZHI_IMAGE_REFERENCE in env)
try:
    # Unset XIANZHI_IMAGE_REFERENCE in the subshell to resolve the actual old compose configuration from env_file
    old_compose_raw = run_cmd(
        ["docker", "compose", "-f", compose_file, "--env-file", env_file, "config", "--format", "json"],
        env_remove=["XIANZHI_IMAGE_REFERENCE"]
    )
    old_compose_data = json.loads(old_compose_raw)
    old_api_target = old_compose_data.get("services", {}).get("xianzhi-ai", {}).get("image")
    old_worker_target = old_compose_data.get("services", {}).get("smartvideo-worker", {}).get("image")
    if old_api_target != api_image:
        fail(f"Current old compose desired image ({old_api_target}) does not match running API image ({api_image})")
    if old_worker_target != api_image:
        fail(f"Current old compose desired image ({old_worker_target}) does not match running worker image ({api_image})")
except Exception as e:
    if "ROLLBACK_RECEIPT_ERROR" in str(e):
        raise
    fail(f"Failed to verify old compose desired state: {e}")

# 2. Locate rollback manifest candidate
rollback_manifest_path = ""
if user_rollback_manifest:
    if not os.path.isfile(user_rollback_manifest) or os.path.getsize(user_rollback_manifest) == 0:
        fail(f"Specified --rollback-manifest not found or empty: {user_rollback_manifest}")
    rollback_manifest_path = user_rollback_manifest
else:
    candidates = [
        os.path.join("backups", "release-manifest.json"),
        "release-manifest.json"
    ]
    if os.path.isdir(prestage_dir):
        for sub in sorted(os.listdir(prestage_dir), reverse=True):
            m_cand = os.path.join(prestage_dir, sub, "release-manifest.json")
            if os.path.isfile(m_cand):
                candidates.append(m_cand)

    for cand in candidates:
        if os.path.isfile(cand):
            try:
                with open(cand, "r", encoding="utf-8") as f:
                    data = json.load(f)
                cand_ref = data.get("image_reference", "")
                if registry_choice:
                    reg_entry = data.get("registries", {}).get(registry_choice)
                    if isinstance(reg_entry, dict) and reg_entry.get("image_reference"):
                        cand_ref = reg_entry.get("image_reference")
                if cand_ref == api_image:
                    rollback_manifest_path = cand
                    break
            except Exception:
                continue

if not rollback_manifest_path:
    fail("Rollback manifest matching the currently running container image digest was not found. Specify --rollback-manifest explicitly.")

# Read local rollback manifest candidate
with open(rollback_manifest_path, "rb") as rf:
    local_rb_bytes = rf.read()
local_rb_sha256 = hashlib.sha256(local_rb_bytes).hexdigest()
try:
    rb_data = json.loads(local_rb_bytes.decode("utf-8"))
except Exception as e:
    fail(f"Rollback manifest is invalid JSON: {e}")

prev_git_sha = rb_data.get("git_sha", "")
if not prev_git_sha or not re.match(r"^[0-9a-f]{40}$", prev_git_sha):
    fail(f"Rollback manifest {rollback_manifest_path} contains invalid git_sha ({prev_git_sha}). Cannot use local HEAD.")

# 3. VERIFY ROLLBACK MANIFEST PROVENANCE VIA OFFICIAL GITHUB ACTIONS API
rb_runs_url = f"{OFFICIAL_API_BASE}/repos/{OFFICIAL_REPOSITORY}/actions/runs?head_sha={prev_git_sha}&event=push"
rb_runs_data = json.loads(http_get(rb_runs_url).decode("utf-8"))
rb_matching_run = None
for r in rb_runs_data.get("workflow_runs", []):
    if (r.get("head_sha") == prev_git_sha and
        r.get("event") == "push" and
        r.get("head_branch") in ("main", "master") and
        r.get("status") == "completed" and
        r.get("conclusion") == "success" and
        r.get("path") == OFFICIAL_WORKFLOW):
        rb_matching_run = r
        break

if not rb_matching_run:
    fail(f"No official successful push run found on GitHub for rollback commit {prev_git_sha}")

rb_run_id = rb_matching_run["id"]
rb_artifacts_url = f"{OFFICIAL_API_BASE}/repos/{OFFICIAL_REPOSITORY}/actions/runs/{rb_run_id}/artifacts"
rb_artifacts_data = json.loads(http_get(rb_artifacts_url).decode("utf-8"))

rb_matching_artifact = None
expected_rb_names = (f"release-manifest-{prev_git_sha}", "release-manifest")
for a in rb_artifacts_data.get("artifacts", []):
    if a.get("name") in expected_rb_names and not a.get("expired", False):
        rb_matching_artifact = a
        break

if not rb_matching_artifact:
    fail(f"No valid non-expired release-manifest artifact found for rollback run {rb_run_id}")

rb_artifact_id = rb_matching_artifact["id"]
rb_download_url = f"{OFFICIAL_API_BASE}/repos/{OFFICIAL_REPOSITORY}/actions/artifacts/{rb_artifact_id}/zip"
rb_zip_bytes = http_get(rb_download_url)

rb_official_manifest_bytes = None
try:
    with zipfile.ZipFile(io.BytesIO(rb_zip_bytes)) as zf:
        namelist = zf.namelist()
        manifest_filename = None
        for fn in (f"release-manifest-{prev_git_sha}.json", "release-manifest.json"):
            if fn in namelist:
                manifest_filename = fn
                break
        if not manifest_filename:
            for fn in namelist:
                if fn.endswith(".json"):
                    manifest_filename = fn
                    break
        if not manifest_filename:
            fail("Rollback artifact zip contains no JSON manifest")
        rb_official_manifest_bytes = zf.read(manifest_filename)
except Exception as e:
    fail(f"Failed to unpack rollback artifact zip: {e}")

rb_official_sha256 = hashlib.sha256(rb_official_manifest_bytes).hexdigest()
if rb_official_sha256 != local_rb_sha256:
    fail(f"Local rollback manifest bytes ({local_rb_sha256}) do not match official GitHub Actions artifact bytes ({rb_official_sha256})")

# Verify verified rollback manifest content pins running image
rb_verified_data = json.loads(rb_official_manifest_bytes.decode("utf-8"))
rb_resolved_ref = rb_verified_data.get("image_reference", "")
if registry_choice:
    reg_entry = rb_verified_data.get("registries", {}).get(registry_choice)
    if isinstance(reg_entry, dict) and reg_entry.get("image_reference"):
        rb_resolved_ref = reg_entry.get("image_reference")

if rb_resolved_ref != api_image:
    fail(f"Verified rollback manifest image reference ({rb_resolved_ref}) does not match running API image ({api_image})")

# Write verified rollback manifest to target dir
verified_rb_file = os.path.join(target_dir, "rollback-manifest.json")
with open(verified_rb_file, "wb") as rbf:
    rbf.write(rb_official_manifest_bytes)

with open(compose_file, "rb") as cf:
    prev_compose_hash = hashlib.sha256(cf.read()).hexdigest()
with open(env_file, "rb") as ef:
    prev_env_hash = hashlib.sha256(ef.read()).hexdigest()

receipt = {
    "receipt_version": "1.0",
    "target_git_sha": target_sha,
    "previous_git_sha": prev_git_sha,
    "previous_image_reference": api_image,
    "previous_image_id": api_id,
    "previous_repo_digests": repo_digests,
    "rollback_manifest_path": verified_rb_file,
    "rollback_manifest_sha256": rb_official_sha256,
    "previous_compose_hash": prev_compose_hash,
    "previous_env_hash": prev_env_hash,
    "selected_registry": registry_choice,
    "rollback_provenance": {
        "repository": OFFICIAL_REPOSITORY,
        "workflow": OFFICIAL_WORKFLOW,
        "run_id": rb_run_id,
        "artifact_id": rb_artifact_id,
        "head_sha": prev_git_sha
    },
    "created_at": datetime.datetime.now(datetime.timezone.utc).isoformat()
}

with open(receipt_path, "w", encoding="utf-8") as f:
    json.dump(receipt, f, indent=2, sort_keys=True)
    f.write("\n")
PY

# Trust Key handling
log "Resolving release trust key..."
TRUST_KEY=""
if [ -n "$RELEASE_TRUST_SECRET" ]; then
  TRUST_KEY="$RELEASE_TRUST_SECRET"
elif [ -n "$RELEASE_TRUST_KEY_FILE" ] && [ -f "$RELEASE_TRUST_KEY_FILE" ]; then
  TRUST_KEY="$(cat "$RELEASE_TRUST_KEY_FILE")"
elif [ -f "/etc/zhiqiyun/release-trust.key" ]; then
  TRUST_KEY="$(cat "/etc/zhiqiyun/release-trust.key")"
elif [ -f "${PRESTAGE_DIR}/release-trust.key" ]; then
  TRUST_KEY="$(cat "${PRESTAGE_DIR}/release-trust.key")"
else
  mkdir -p "$PRESTAGE_DIR"
  python3 -c 'import secrets; print(secrets.token_hex(32))' > "${PRESTAGE_DIR}/release-trust.key"
  chmod 600 "${PRESTAGE_DIR}/release-trust.key" 2>/dev/null || true
  TRUST_KEY="$(cat "${PRESTAGE_DIR}/release-trust.key")"
  log "Generated new local release trust key at ${PRESTAGE_DIR}/release-trust.key"
fi

# Atomic proof generation
FINAL_PROOF="${PRESTAGE_TARGET_DIR}/prestage-proof.json"
TMP_PROOF="$(mktemp "${PRESTAGE_TARGET_DIR}/prestage-proof.tmp.XXXXXX")"

log "Computing cryptographic digests and signing proof (fail closed on missing protected files)..."
RELEASE_TRUST_SECRET="$TRUST_KEY" python3 - "$TMP_PROOF" "$PRESTAGED_RELEASE_SHA" "$XIANZHI_IMAGE_REFERENCE" "$local_image_id" "$RELEASE_MANIFEST" "$COMPOSE_FILE" "$ENV_FILE" "$ROLLBACK_RECEIPT_PATH" "$PRESTAGE_EXPIRY_HOURS" "$PROVENANCE_OUTPUT_JSON" "$RELEASE_REGISTRY" <<'PY'
import datetime
import hashlib
import hmac
import json
import os
import shlex
import shutil
import subprocess
import sys
import uuid

(
    tmp_proof_path,
    git_sha,
    image_ref,
    local_img_id,
    manifest_path,
    compose_file,
    env_file,
    rollback_receipt_path,
    expiry_hours_str,
    provenance_json_path,
    registry_choice
) = sys.argv[1:12]

trust_key = os.environ.get("RELEASE_TRUST_SECRET", "").strip()

def fail(msg):
    sys.stderr.write(f"[prestage] ERROR: {msg}\n")
    sys.exit(1)

def run_cmd(args):
    if os.name == "nt":
        sh_bin = os.environ.get("SH_EXE") or shutil.which("sh") or "C:/Program Files/Git/bin/sh.exe"
        cmd_str = " ".join(shlex.quote(a) for a in args)
        return subprocess.check_output([sh_bin, "-c", cmd_str], stderr=subprocess.PIPE).decode("utf-8").strip()
    return subprocess.check_output(args, stderr=subprocess.PIPE).decode("utf-8").strip()

expiry_hours = float(expiry_hours_str)

# 1. Manifest sha256 (Fail Closed)
if not os.path.isfile(manifest_path) or os.path.getsize(manifest_path) == 0:
    fail(f"PROTECTED_FILE_MISSING: manifest file missing or empty: {manifest_path}")
with open(manifest_path, "rb") as f:
    manifest_bytes = f.read()
    manifest_sha256 = hashlib.sha256(manifest_bytes).hexdigest()

digest = image_ref.split("@", 1)[1] if "@" in image_ref else ""

# 2. Compose sha256 (Fail Closed)
if not os.path.isfile(compose_file) or os.path.getsize(compose_file) == 0:
    fail(f"PROTECTED_FILE_MISSING: compose file missing or empty: {compose_file}")
with open(compose_file, "rb") as f:
    compose_hash = hashlib.sha256(f.read()).hexdigest()

# 3. Env sha256 & Effective Compose Configuration Binding (Fail Closed)
if not os.path.isfile(env_file) or os.path.getsize(env_file) == 0:
    fail(f"PROTECTED_FILE_MISSING: env file missing or empty: {env_file}")
with open(env_file, "rb") as f:
    env_hash = hashlib.sha256(f.read()).hexdigest()

# Render effective Compose config incorporating env file and process environment overrides
try:
    effective_compose_raw = run_cmd(["docker", "compose", "-f", compose_file, "--env-file", env_file, "config", "--format", "json"])
    effective_data = json.loads(effective_compose_raw)
except Exception as e:
    fail(f"Failed to render effective Docker Compose configuration: {e}")

services_data = effective_data.get("services", {})
for s_name in ("xianzhi-ai", "smartvideo-worker", "migrate"):
    if not services_data.get(s_name):
        fail("CRITICAL_SERVICE_MISSING: " + s_name)
# Bind the WHOLE rendered model: all services/env, DB/broker/OBS credentials and
# endpoints, allowlists, mount sources/options, networks and Compose project.
# Persist only its hash. Never write the rendered secret values into the proof.
bound_config_str = json.dumps(effective_data, sort_keys=True, separators=(',', ':'))
bound_config_hash = hashlib.sha256(bound_config_str.encode("utf-8")).hexdigest()

# 4. Migrations directory & actual SQL content (Fail Closed)
migrations_dir = os.path.join("database", "migrations")
if not os.path.isdir(migrations_dir):
    fail(f"PROTECTED_FILE_MISSING: database/migrations directory missing")

sql_files = sorted([f for f in os.listdir(migrations_dir) if f.endswith(".sql") and not f.endswith(".down.sql")])
if not sql_files:
    fail("PROTECTED_FILE_MISSING: database/migrations directory contains no valid SQL migration files")

dir_hasher = hashlib.sha256()
migrations_files_map = {}
for fn in sql_files:
    fp = os.path.join(migrations_dir, fn)
    if not os.path.isfile(fp) or os.path.getsize(fp) == 0:
        fail(f"PROTECTED_FILE_MISSING: migration SQL file empty or missing: {fn}")
    with open(fp, "rb") as mf:
        content = mf.read()
    f_hash = hashlib.sha256(content).hexdigest()
    migrations_files_map[fn] = f_hash
    dir_hasher.update(f"{fn}:{f_hash}\n".encode("utf-8"))
migrations_content_hash = dir_hasher.hexdigest()

migrations_tree_hash = ""
try:
    migrations_tree_hash = run_cmd(["git", "rev-parse", f"{git_sha}:database/migrations"])
except Exception:
    try:
        migrations_tree_hash = run_cmd(["git", "rev-parse", "HEAD:database/migrations"])
    except Exception:
        pass

# 5. Deploy & ops scripts hash (Fail Closed)
deploy_scripts = [
    "deploy.sh",
    "rollback.sh",
    "ops/verify-release-manifest.sh",
    "ops/disk-guard.sh",
    "ops/run-migrations.sh",
    "ops/prestage-release.sh",
    "ops/verify-prestage-proof.sh",
    "ops/verify-release-runtime.py",
]
deploy_scripts_hash = {}
for s in deploy_scripts:
    if not os.path.isfile(s) or os.path.getsize(s) == 0:
        fail(f"PROTECTED_FILE_MISSING: deployment script missing or empty: {s}")
    with open(s, "rb") as sf:
        deploy_scripts_hash[s] = hashlib.sha256(sf.read()).hexdigest()

# 6. Rollback receipt hash (Fail Closed)
if not os.path.isfile(rollback_receipt_path) or os.path.getsize(rollback_receipt_path) == 0:
    fail(f"PROTECTED_FILE_MISSING: rollback receipt missing or empty: {rollback_receipt_path}")
with open(rollback_receipt_path, "rb") as rf:
    rollback_receipt_hash = hashlib.sha256(rf.read()).hexdigest()

# 7. Official Provenance from Step 1
with open(provenance_json_path, "r", encoding="utf-8") as pvf:
    gov = json.load(pvf)

now = datetime.datetime.now(datetime.timezone.utc)
expires_at = now + datetime.timedelta(hours=expiry_hours)
proof_nonce = str(uuid.uuid4())

proof_payload = {
    "proof_version": "1.0",
    "git_sha": git_sha,
    "manifest_sha256": manifest_sha256,
    "manifest_path": manifest_path,
    "image_reference": image_ref,
    "image_digest": digest,
    "selected_registry": registry_choice,
    "local_image_id": local_img_id,
    "compose_hash": compose_hash,
    "bound_config_hash": bound_config_hash,
    "migrations_tree_hash": migrations_tree_hash,
    "migrations_content_hash": migrations_content_hash,
    "migrations_files_count": len(sql_files),
    "deploy_scripts_hash": deploy_scripts_hash,
    "env_hash": env_hash,
    "config_binding_version": 2,
    "rollback_receipt_hash": rollback_receipt_hash,
    "rollback_receipt_path": rollback_receipt_path,
    "runtime_gates": {
        "disk_guard": "PASS",
        "compose_desired_state": "PASS",
        "video_storage_persistence": "PASS"
    },
    "github_provenance": gov,
    "created_at": now.isoformat(),
    "expires_at": expires_at.isoformat(),
    "proof_nonce": proof_nonce,
    "signature_algorithm": "HMAC-SHA256"
}

canonical_str = json.dumps(proof_payload, sort_keys=True, separators=(',', ':'))
sig = hmac.new(trust_key.strip().encode("utf-8"), canonical_str.encode("utf-8"), hashlib.sha256).hexdigest()
proof_payload["signature"] = sig

with open(tmp_proof_path, "w", encoding="utf-8") as pf:
    json.dump(proof_payload, pf, indent=2, sort_keys=True)
    pf.write("\n")
PY

# Verify that the generated proof passes self-verification
log "Self-verifying generated proof before final installation..."
if [ -n "${RELEASE_TRUST_KEY_FILE:-}" ]; then
  export RELEASE_TRUST_KEY_FILE
fi
if [ -n "$TRUST_KEY" ]; then
  export RELEASE_TRUST_SECRET="$TRUST_KEY"
fi
bash ops/verify-prestage-proof.sh "$TMP_PROOF" "$PRESTAGED_RELEASE_SHA" "$COMPOSE_FILE" "$ENV_FILE" "" >/dev/null \
  || fail "Internal self-verification of the generated proof failed."

mv -f "$TMP_PROOF" "$FINAL_PROOF"
cp -f "$FINAL_PROOF" "${PRESTAGE_DIR}/prestage-proof-${PRESTAGED_RELEASE_SHA}.json"
chmod 600 "$FINAL_PROOF" 2>/dev/null || true
chmod 600 "${PRESTAGE_DIR}/prestage-proof-${PRESTAGED_RELEASE_SHA}.json" 2>/dev/null || true

# Clear TMP_PROOF so exit trap doesn't delete the final proof
TMP_PROOF=""

log "Prestage proof atomically created at: $FINAL_PROOF"
log "Prestage preparation complete for commit $PRESTAGED_RELEASE_SHA. Ready for offline cutover."
