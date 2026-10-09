#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
GIT_BRANCH="${GIT_BRANCH:-}"
IMMUTABLE_RELEASE="${IMMUTABLE_RELEASE:-0}"
PRESTAGED_RELEASE="${PRESTAGED_RELEASE:-0}"
PRESTAGED_RELEASE_SHA="${PRESTAGED_RELEASE_SHA:-}"
PRESTAGE_PROOF_FILE="${PRESTAGE_PROOF_FILE:-}"
PRESTAGE_DIR="${PRESTAGE_DIR:-.prestage}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-}"
RELEASE_REGISTRY="${RELEASE_REGISTRY:-}"
RELEASE_TRUST_KEY_FILE="${RELEASE_TRUST_KEY_FILE:-}"
RELEASE_TRUST_SECRET="${RELEASE_TRUST_SECRET:-}"
RELEASE_LEDGER_FILE="${RELEASE_LEDGER_FILE:-backups/release-ledger.json}"
QUARANTINE_MANIFEST="${QUARANTINE_MANIFEST:-}"
SKIP_FETCH="${SKIP_FETCH:-0}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

LOCK_DIR="${PRESTAGE_DIR}/release.lock"
RECOVERY_LOCK="${PRESTAGE_DIR}/release.lock.recovering"
OWNER_TOKEN="${BASHPID:-$$}_$(date +%s%N 2>/dev/null || date +%s)_$RANDOM"
export OWNER_TOKEN
IS_LOCK_OWNER=0
FIRST_UPGRADE_COLD=0
COLD_ARMED=0
COLD_STAGE=preflight
COLD_TRIGGER=exit-failure
COLD_COMPOSE_FILE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --first-upgrade-cold)
      FIRST_UPGRADE_COLD=1
      shift
      ;;
    --prestaged)
      PRESTAGED_RELEASE=1
      IMMUTABLE_RELEASE=1
      if [ $# -gt 1 ] && [[ "$2" =~ ^[0-9a-f]{40}$ ]]; then
        PRESTAGED_RELEASE_SHA="$2"
        shift 2
      else
        shift 1
      fi
      ;;
    --sha)
      PRESTAGED_RELEASE_SHA="$2"
      shift 2
      ;;
    --proof)
      PRESTAGE_PROOF_FILE="$2"
      shift 2
      ;;
    --manifest)
      RELEASE_MANIFEST="$2"
      shift 2
      ;;
    --registry)
      RELEASE_REGISTRY="$2"
      shift 2
      ;;
    --compose-file)
      COMPOSE_FILE="$2"
      shift 2
      ;;
    --env-file)
      ENV_FILE="$2"
      shift 2
      ;;
    --quarantine-manifest)
      QUARANTINE_MANIFEST="$2"
      shift 2
      ;;
    --skip-fetch)
      SKIP_FETCH=1
      shift 1
      ;;
    *)
      break
      ;;
  esac
done

if [ "$SKIP_FETCH" = "1" ] && [ "$PRESTAGED_RELEASE" != "1" ]; then
  printf '[deploy] ERROR: %s\n' "Direct SKIP_FETCH without verified prestaged release proof is forbidden. Use --prestaged with a valid proof." >&2
  exit 1
fi

if [ -n "$QUARANTINE_MANIFEST" ] && [ ! -f "$QUARANTINE_MANIFEST" ]; then
  printf '[deploy] ERROR: %s\n' "Quarantine manifest specified but not found on disk: $QUARANTINE_MANIFEST" >&2
  exit 1
fi

log() {
  printf '[deploy] %s\n' "$*"
}

fail() {
  printf '[deploy] ERROR: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  local cold_failed=0
  if [ "${COLD_ARMED:-0}" = "1" ]; then
    python3 -B ops/first-upgrade-cold.py fence --prestage-dir "$PRESTAGE_DIR" --stage "$COLD_STAGE" --reason "$COLD_TRIGGER" --owner "$OWNER_TOKEN" --proof "$PRESTAGE_PROOF_FILE" || cold_failed=1
    COLD_ARMED=0
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
  if [ "$cold_failed" = "1" ]; then
    printf '%s\n' "[deploy] COLD_RECOVERY_UNKNOWN: hold retained; stop/audit not proven." >&2
    exit 1
  fi
}

handle_signal() {
  local sig="$1"
  trap - "$sig" EXIT
  COLD_TRIGGER="signal-$sig"
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

update_env_file_key() {
  local file="$1" key="$2" value="$3"
  local tmp env_backup_dir
  tmp="$(mktemp "${file}.tmp.XXXXXX")" || fail "Failed to create a temporary env file."
  env_backup_dir="backups/env"
  mkdir -p "$env_backup_dir"
  cp -p "$file" "$env_backup_dir/$(basename "$file").${TIMESTAMP}.bak"

  chmod --reference="$file" "$tmp" 2>/dev/null || chmod 600 "$tmp" || {
    rm -f "$tmp"
    fail "Failed to preserve permissions for $file."
  }

  awk -v k="$key" -v v="$value" '
    BEGIN { re = "^[#[:space:]]*" k "=" }
    $0 ~ re {
      if (!replaced) print k "=" v
      replaced = 1
      next
    }
    { print }
    END { if (!replaced) print k "=" v }
  ' "$file" > "$tmp"

  [ -s "$tmp" ] || { rm -f "$tmp"; fail "Failed to update $file: temporary file is empty."; }
  grep -Fqx "${key}=${value}" "$tmp" >/dev/null || {
    rm -f "$tmp"
    fail "Verification failed for $key in temporary env file."
  }
  chmod --reference="$file" "$tmp" 2>/dev/null || chmod 600 "$tmp" || {
    rm -f "$tmp"
    fail "Failed to preserve permissions for $file."
  }

  mv -f "$tmp" "$file"
}

validate_compose_desired_state() {
  local rendered="$1"
  python3 -c '
import json
import sys

expected, rendered = sys.argv[1], sys.stdin.read()
data = json.loads(rendered)
services = data.get("services", {})
for service in ("xianzhi-ai", "smartvideo-worker"):
    actual = services.get(service, {}).get("image")
    if actual != expected:
        raise SystemExit(f"Compose service {service} desired image mismatch: expected {expected}, got {actual}")
' "$XIANZHI_IMAGE_REFERENCE" <<< "$rendered" || fail "Compose desired state does not match the immutable release."
}

validate_video_storage_persistence() {
  local rendered="$1"
  python3 -c '
import json
import sys

rendered = sys.stdin.read()
data = json.loads(rendered)
service = data.get("services", {}).get("xianzhi-ai", {})
env = service.get("environment", {})
actual = env.get("VIDEO_STORAGE_PERSISTENCE_ENABLED") if isinstance(env, dict) else None
if str(actual).strip().lower() != "true":
    raise SystemExit("VIDEO_STORAGE_PERSISTENCE_ENABLED must resolve to true for xianzhi-ai")
' <<< "$rendered" || fail "Video storage persistence production gate failed."
}

check_safe_drain() {
  local drain_timeout="${DRAIN_TIMEOUT_SECONDS:-15}"
  if [ -n "$QUARANTINE_MANIFEST" ]; then
    [ -f "$QUARANTINE_MANIFEST" ] || fail "Quarantine manifest not found on disk: $QUARANTINE_MANIFEST"
    EXPECTED_QUARANTINE_MANIFEST_SHA256="$(sha256sum "$QUARANTINE_MANIFEST" | awk '{print $1}')"
    export EXPECTED_QUARANTINE_MANIFEST_SHA256
    log "Performing safe drain observation with approved quarantine exemptions (Release SHA: $PRESTAGED_RELEASE_SHA, Manifest SHA: $EXPECTED_QUARANTINE_MANIFEST_SHA256)..."
    python3 ops/verify-safe-drain.py "$COMPOSE_FILE" "$ENV_FILE" "$drain_timeout" \
      --manifest "$QUARANTINE_MANIFEST" \
      --release-sha "$PRESTAGED_RELEASE_SHA" \
      --expected-manifest-sha256 "$EXPECTED_QUARANTINE_MANIFEST_SHA256" \
      --prestage-proof "$PRESTAGE_PROOF_FILE"
  else
    log "Performing read-only safe drain observation; no quarantine exemptions..."
    python3 ops/verify-safe-drain.py "$COMPOSE_FILE" "$ENV_FILE" "$drain_timeout" --prestage-proof "$PRESTAGE_PROOF_FILE"
  fi
}

verify_health_and_readiness() {
  local expected_image_id="$1"
  local health_timeout="${HEALTH_CHECK_TIMEOUT_SECONDS:-60}"
  log "Verifying health, readiness, worker status, and RepoDigests after start..."
  python3 - "$COMPOSE_FILE" "$ENV_FILE" "$XIANZHI_IMAGE_REFERENCE" "$expected_image_id" "$health_timeout" <<'PY'
import json, os, shlex, shutil, subprocess, sys, time

compose_file, env_file, expected_ref, expected_img_id, timeout_str = sys.argv[1:6]

def fail(msg):
    sys.stderr.write(f"[deploy] ERROR: CUTOVER_GATE_FAILED: {msg}\n")
    sys.exit(1)

def run_cmd(args):
    if os.name == "nt":
        sh_bin = os.environ.get("SH_EXE") or shutil.which("sh") or "C:/Program Files/Git/bin/sh.exe"
        cmd_str = " ".join(shlex.quote(a) for a in args)
        return subprocess.check_output([sh_bin, "-c", cmd_str], stderr=subprocess.PIPE).decode("utf-8").strip()
    return subprocess.check_output(args, stderr=subprocess.PIPE).decode("utf-8").strip()

async_enabled = False
if os.path.isfile(env_file):
    with open(env_file, "r", encoding="utf-8") as ef:
        for line in ef:
            if line.strip().startswith("ASYNC_MESSAGING_ENABLED="):
                val = line.strip().split("=", 1)[1].strip("\"'").lower()
                if val == "true":
                    async_enabled = True

max_wait_seconds = float(timeout_str)
deadline = time.time() + max_wait_seconds
last_err = ""

while time.time() < deadline:
    try:
        # 1. Check containers running
        api_cid = run_cmd(["docker", "compose", "-f", compose_file, "--env-file", env_file, "ps", "-q", "xianzhi-ai"])
        worker_cid = run_cmd(["docker", "compose", "-f", compose_file, "--env-file", env_file, "ps", "-q", "smartvideo-worker"])
        if not api_cid or not worker_cid:
            last_err = "API or worker container not running"
            time.sleep(2)
            continue

        # 2. Check running image reference and image ID
        api_img_ref = run_cmd(["docker", "inspect", "--format", "{{.Config.Image}}", api_cid])
        api_img_id = run_cmd(["docker", "inspect", "--format", "{{.Image}}", api_cid])
        worker_img_ref = run_cmd(["docker", "inspect", "--format", "{{.Config.Image}}", worker_cid])
        worker_img_id = run_cmd(["docker", "inspect", "--format", "{{.Image}}", worker_cid])

        if api_img_ref != expected_ref or worker_img_ref != expected_ref:
            fail(f"PARTIAL_RELEASE_DETECTED: Config.Image mismatch (API={api_img_ref}, Worker={worker_img_ref}, Expected={expected_ref})")
        if api_img_id != expected_img_id or worker_img_id != expected_img_id:
            fail(f"PARTIAL_RELEASE_DETECTED: Image ID mismatch (API={api_img_id}, Worker={worker_img_id}, Expected={expected_img_id})")

        # 3. Check RepoDigests explicitly on the running image
        digests_out = run_cmd(["docker", "image", "inspect", api_img_id, "--format", "{{range .RepoDigests}}{{println .}}{{end}}"])
        repo_digests = [d.strip() for d in digests_out.splitlines() if d.strip()]
        if expected_ref not in repo_digests:
            fail(f"PARTIAL_RELEASE_DETECTED: running image RepoDigests do not contain {expected_ref} (digests: {repo_digests})")

        # 4. Check API health endpoint
        health_out = run_cmd([
            "docker", "compose", "-f", compose_file, "--env-file", env_file,
            "exec", "-T", "xianzhi-ai", "curl", "-fsS", "--max-time", "10", "http://127.0.0.1:3100/api/v1/health"
        ])

        if not health_out:
            last_err = "API health endpoint /api/v1/health not responding"
            time.sleep(2)
            continue

        h_data = json.loads(health_out)
        if h_data.get("status") != "ok":
            last_err = f"API health status is not 'ok': {health_out}"
            time.sleep(2)
            continue

        # 5. Check API ready endpoint
        ready_out = run_cmd([
            "docker", "compose", "-f", compose_file, "--env-file", env_file,
            "exec", "-T", "xianzhi-ai", "curl", "-fsS", "--max-time", "10", "http://127.0.0.1:3100/api/v1/ready"
        ])

        if not ready_out:
            last_err = "API readiness endpoint /api/v1/ready not responding"
            time.sleep(2)
            continue

        r_data = json.loads(ready_out)
        if str(r_data.get("ready", "")).lower() != "true":
            last_err = f"API readiness is not true: {ready_out}"
            time.sleep(2)
            continue

        if async_enabled:
            if r_data.get("asyncMessaging") != "READY":
                last_err = f"Async messaging status is not READY: {r_data.get('asyncMessaging')}"
                time.sleep(2)
                continue

        # 6. Check worker health status
        worker_health = run_cmd(["docker", "inspect", "--format", "{{.State.Health.Status}}", worker_cid])
        if worker_health != "healthy":
            last_err = f"smartvideo-worker container health status is not 'healthy' (status={worker_health})"
            time.sleep(2)
            continue

        # All checks passed!
        sys.exit(0)
    except Exception:
        fail("Runtime observation command failed or returned invalid data")

fail(f"Health and readiness verification timed out after {max_wait_seconds}s. Last error: {last_err}")
PY
}

if [ -e "${PRESTAGE_DIR}/cold-recovery-required" ] || [ -L "${PRESTAGE_DIR}/cold-recovery-required" ]; then
  fail "COLD_RECOVERY_REQUIRED: separate human authorization required; no deployment."
fi
[ "$FIRST_UPGRADE_COLD" != "1" ] || [ "$PRESTAGED_RELEASE" = "1" ] || fail "COLD_POLICY_REQUIRED: --first-upgrade-cold requires signed prestaged release."

command -v git >/dev/null 2>&1 || fail "git is not installed."
command -v docker >/dev/null 2>&1 || fail "Docker is not installed."
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is not available."

[ -f "$ENV_FILE" ] || fail "$ENV_FILE does not exist. Create it and fill in production values first."
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE does not exist."

# 磁盘剩余容量门禁
DISK_WARN_PERCENT="${DISK_WARN_PERCENT:-70}" \
DISK_CRITICAL_PERCENT="${DISK_CRITICAL_PERCENT:-80}" \
DISK_EMERGENCY_PERCENT="${DISK_EMERGENCY_PERCENT:-90}" \
DISK_MIN_FREE_BYTES="${DEPLOY_MIN_FREE_BYTES:-10737418240}" \
  sh ops/disk-guard.sh "$SCRIPT_DIR" || fail "Insufficient disk space for a safe deployment."

# Working tree clean check: fail closed on dirty or untracked files
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  fail "Working tree contains uncommitted or untracked changes. Commit/revert them before deploying."
fi

is_pid_alive() {
  local pid="$1"
  # Only ESRCH proves death. Permission/format/observation errors mean unknown/live.
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

if [ "$PRESTAGED_RELEASE" = "1" ]; then
  acquire_release_lock

  [ -n "$PRESTAGED_RELEASE_SHA" ] || fail "PRESTAGED_RELEASE_SHA is required for prestaged deployment."
  [[ "$PRESTAGED_RELEASE_SHA" =~ ^[0-9a-f]{40}$ ]] || fail "PRESTAGED_RELEASE_SHA must be a 40-character lowercase hexadecimal SHA."
  current_head="$(git rev-parse HEAD)"
  [ "$current_head" = "$PRESTAGED_RELEASE_SHA" ] || fail "Current HEAD ($current_head) does not match PRESTAGED_RELEASE_SHA ($PRESTAGED_RELEASE_SHA). Checkout target commit before cutover."
  echo "$PRESTAGED_RELEASE_SHA" > "$LOCK_DIR/release_sha"

  if [ -z "$PRESTAGE_PROOF_FILE" ]; then
    for candidate in \
      "${PRESTAGE_DIR}/${PRESTAGED_RELEASE_SHA}/prestage-proof.json" \
      "${PRESTAGE_DIR}/prestage-proof-${PRESTAGED_RELEASE_SHA}.json" \
      "prestage-proof-${PRESTAGED_RELEASE_SHA}.json"; do
      if [ -f "$candidate" ]; then
        PRESTAGE_PROOF_FILE="$candidate"
        break
      fi
    done
  fi
  { [ -n "$PRESTAGE_PROOF_FILE" ] && [ -f "$PRESTAGE_PROOF_FILE" ]; } || fail "Prestage proof not found for $PRESTAGED_RELEASE_SHA. Run ops/prestage-release.sh first."

  log "Verifying prestaged release proof: $PRESTAGE_PROOF_FILE"
  [ -x ops/verify-prestage-proof.sh ] || fail "ops/verify-prestage-proof.sh must be executable."
  if [ -n "${RELEASE_TRUST_KEY_FILE:-}" ]; then
    export RELEASE_TRUST_KEY_FILE
  fi
  if [ -n "${RELEASE_TRUST_SECRET:-}" ]; then
    export RELEASE_TRUST_SECRET
  fi
  XIANZHI_IMAGE_REFERENCE="$(bash ops/verify-prestage-proof.sh "$PRESTAGE_PROOF_FILE" "$PRESTAGED_RELEASE_SHA" "$COMPOSE_FILE" "$ENV_FILE" "${RELEASE_TRUST_KEY_FILE:-}" "$RELEASE_LEDGER_FILE")"
  export XIANZHI_IMAGE_REFERENCE
  proof_cold="$(python3 -c 'import json,sys; print(1 if json.load(open(sys.argv[1])).get("cold_recovery_policy") is not None else 0)' "$PRESTAGE_PROOF_FILE")"
  [ "$proof_cold" = "$FIRST_UPGRADE_COLD" ] || fail "COLD_POLICY_OPT_IN_MISMATCH: signed policy and --first-upgrade-cold must agree."
  case "$XIANZHI_IMAGE_REFERENCE" in
    *@sha256:*) ;;
    *) fail "Prestage proof did not provide a digest-pinned image reference." ;;
  esac
  log "Prestaged immutable image reference: $XIANZHI_IMAGE_REFERENCE"
fi

# Production cutover must be offline and authenticated. Legacy build/pull
# entrypoints cannot establish actual packaged runtime capability.
[ "$PRESTAGED_RELEASE" = "1" ] || fail "RUNTIME_CAPABILITY_PROOF_REQUIRED: Use fresh Prestage proof before production cutover."

if [ "$PRESTAGED_RELEASE" != "1" ]; then
  if [ -z "$GIT_BRANCH" ]; then
    GIT_BRANCH="$(git symbolic-ref --quiet --short HEAD)" \
      || fail "Cannot determine the current Git branch. Set GIT_BRANCH explicitly."
  fi

  mkdir -p backups/compose
  cp "$COMPOSE_FILE" "backups/compose/$(basename "$COMPOSE_FILE").${TIMESTAMP}.bak"
  log "Backed up $COMPOSE_FILE."

  log "Fetching GitHub source: $GIT_REMOTE/$GIT_BRANCH"
  git fetch --prune "$GIT_REMOTE" "$GIT_BRANCH"
  git pull --ff-only "$GIT_REMOTE" "$GIT_BRANCH"
  log "Current commit: $(git rev-parse --short HEAD)"

  [ -f "$ENV_FILE" ] || fail "$ENV_FILE is missing after the Git update."
  [ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE is missing after the Git update."
else
  mkdir -p backups/compose
  cp "$COMPOSE_FILE" "backups/compose/$(basename "$COMPOSE_FILE").${TIMESTAMP}.bak"
  log "Backed up $COMPOSE_FILE."
  log "Prestaged mode: zero remote Git operations (completely offline cutover)."
  log "Current verified commit: $(git rev-parse HEAD)"
fi

if [ "$IMMUTABLE_RELEASE" = "1" ] && [ "$PRESTAGED_RELEASE" != "1" ]; then
  [ -n "$RELEASE_MANIFEST" ] || fail "RELEASE_MANIFEST is required for immutable release."
  [ -x ops/verify-release-manifest.sh ] || fail "ops/verify-release-manifest.sh must be executable."
  raw_reference="$(bash ops/verify-release-manifest.sh "$RELEASE_MANIFEST" "$(git rev-parse HEAD)" "" "$RELEASE_REGISTRY")"
  export XIANZHI_IMAGE_REFERENCE="$raw_reference"
  case "$XIANZHI_IMAGE_REFERENCE" in
    *@sha256:*) ;;
    *) fail "Release manifest did not provide a digest-pinned image reference." ;;
  esac
  log "Verified immutable image reference: $XIANZHI_IMAGE_REFERENCE"
fi

compose_config="$(docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  config --format json 2>/dev/null)" || fail "Failed to render Docker Compose configuration."

log "Verifying rendered video storage persistence gate..."
validate_video_storage_persistence "$compose_config"

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  log "Validating Docker Compose desired state matches manifest..."
  validate_compose_desired_state "$compose_config"
fi

if [ "$PRESTAGED_RELEASE" = "1" ]; then
  # Pre-flight check: verify all required images exist locally before touching running services
  docker image inspect "$XIANZHI_IMAGE_REFERENCE" >/dev/null 2>&1 \
    || fail "Target release image $XIANZHI_IMAGE_REFERENCE is missing from local Docker."

  # Verify running services have not drifted from the prestaged rollback snapshot
  receipt_path="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("rollback_receipt_path", ""))' "$PRESTAGE_PROOF_FILE" 2>/dev/null || true)"
  { [ -n "$receipt_path" ] && [ -f "$receipt_path" ]; } || fail "ROLLBACK_RECEIPT_MISSING: rollback receipt missing from prestage proof."

  prev_expected_ref="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("previous_image_reference", ""))' "$receipt_path")"
  prev_expected_id="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("previous_image_id", ""))' "$receipt_path")"
  { [ -n "$prev_expected_ref" ] && [ "$prev_expected_ref" != "none" ]; } || fail "ROLLBACK_RECEIPT_INVALID: previous image reference missing in receipt."
  { [ -n "$prev_expected_id" ] && [ "$prev_expected_id" != "none" ]; } || fail "ROLLBACK_RECEIPT_INVALID: previous image ID missing in receipt."

  cur_api_cid="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q xianzhi-ai)" || fail "SAFE_DRAIN_REJECTED: API discovery failed."
  cur_worker_cid="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q smartvideo-worker)" || fail "SAFE_DRAIN_REJECTED: Worker discovery failed."
  { [ -n "$cur_api_cid" ] && [ -n "$cur_worker_cid" ]; } || fail "Required old containers missing."
  if [ -n "$cur_api_cid" ]; then
    cur_api_img="$(docker inspect --format '{{.Config.Image}}' "$cur_api_cid")" || fail "API image query failed."
    cur_api_id="$(docker inspect --format '{{.Image}}' "$cur_api_cid")" || fail "API image-ID query failed."
    [ "$cur_api_img" = "$prev_expected_ref" ] || fail "RUNNING_IDENTITY_DRIFT: running API image drifted from prestaged snapshot ($cur_api_img != $prev_expected_ref)."
    [ "$cur_api_id" = "$prev_expected_id" ] || fail "RUNNING_IDENTITY_DRIFT: running API image ID drifted from prestaged snapshot ($cur_api_id != $prev_expected_id)."
  fi
  if [ -n "$cur_worker_cid" ]; then
    cur_worker_img="$(docker inspect --format '{{.Config.Image}}' "$cur_worker_cid")" || fail "Worker image query failed."
    cur_worker_id="$(docker inspect --format '{{.Image}}' "$cur_worker_cid")" || fail "Worker image-ID query failed."
    [ "$cur_worker_img" = "$prev_expected_ref" ] || fail "RUNNING_IDENTITY_DRIFT: running worker image drifted from prestaged snapshot ($cur_worker_img != $prev_expected_ref)."
    [ "$cur_worker_id" = "$prev_expected_id" ] || fail "RUNNING_IDENTITY_DRIFT: running worker image ID drifted from prestaged snapshot ($cur_worker_id != $prev_expected_id)."
  fi
  docker image inspect "$prev_expected_id" >/dev/null 2>&1 \
    || fail "ROLLBACK_IMAGE_MISSING: previous image $prev_expected_id is missing from local Docker."

  # Explicit scheduler diagnostics / RabbitMQ queue observations, not HTTP liveness alone.
  python3 ops/verify-release-runtime.py pre "$COMPOSE_FILE" "$ENV_FILE"
  # Pre-stop gate: Safe Drain Check (Read-only observation before stopping services)
  check_safe_drain

  # Record old container IDs before stopping
  old_api_cid="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q xianzhi-ai)" || fail "Old API status query failed."
  old_worker_cid="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q smartvideo-worker)" || fail "Old worker status query failed."
  { [ -n "$old_api_cid" ] && [ "$old_api_cid" = "$cur_api_cid" ]; } || fail "Old API identity changed."
  { [ -n "$old_worker_cid" ] && [ "$old_worker_cid" = "$cur_worker_cid" ]; } || fail "Old worker identity changed."

  # Final offline identity/expiry/policy recheck immediately BEFORE first stop.
  capability_args=(--verify-proof "$PRESTAGE_PROOF_FILE" --image "$XIANZHI_IMAGE_REFERENCE" --release-sha "$PRESTAGED_RELEASE_SHA" --compose-file "$COMPOSE_FILE" --env-file "$ENV_FILE")
  python3 -B ops/verify-image-quarantine-capability.py "${capability_args[@]}" >/dev/null || fail "RUNTIME_CAPABILITY_INVALID: Restage before stop."

  if [ "$FIRST_UPGRADE_COLD" = "1" ]; then
    COLD_STAGE=first-stop
    COLD_ARMED=1
    COLD_COMPOSE_FILE="$(python3 -B ops/first-upgrade-cold.py arm --proof "$PRESTAGE_PROOF_FILE" --compose-file "$COMPOSE_FILE" --env-file "$ENV_FILE" --prestage-dir "$PRESTAGE_DIR" --owner "$OWNER_TOKEN" --stage "$COLD_STAGE")" || fail "COLD_ARM_FAILED: no cold cutover authorized."
    # Disable every relevant OLD restart policy before stopping. Arm remains
    # distinct from a recovery receipt so only this in-flight success can finish.
    python3 -B ops/first-upgrade-cold.py prepare --owner "$OWNER_TOKEN" --proof "$PRESTAGE_PROOF_FILE" --prestage-dir "$PRESTAGE_DIR" --stage "$COLD_STAGE" --reason exit-failure >/dev/null || fail "COLD_FENCE_FAILED: stopped state unknown."
  fi

  # Stop old API and worker services safely, verifying zero running owner processes
  log "Safely stopping old API and worker containers..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" stop -t 120 xianzhi-ai smartvideo-worker || fail "Failed to stop old API/worker containers."

  for old_cid in "$old_api_cid" "$old_worker_cid"; do
    if [ -n "$old_cid" ]; then
      cid_running="$(docker inspect --format '{{.State.Running}}' "$old_cid")" || fail "Old container running-state query failed."
      [ "$cid_running" = "false" ] || fail "Old container $old_cid failed to exit after stop."
      cid_state="$(docker inspect --format '{{.State.Status}}' "$old_cid")" || fail "Old container status query failed."
      [ "$cid_state" = "exited" ] || fail "Old container did not reach exited state."
      cid_exit="$(docker inspect --format '{{.State.ExitCode}}' "$old_cid")" || fail "Old container exit-code query failed."
      case "$cid_exit" in
        0|143) ;;
        *) fail "Old container $old_cid exited uncleanly (exit code $cid_exit)." ;;
      esac
    fi
  done

  running_old="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps --status running -q xianzhi-ai smartvideo-worker)" || fail "Post-stop status query failed."
  [ -z "$running_old" ] || fail "Old API/worker containers failed to stop completely. Parallel old+new forbidden."

  # Execute database migration and verify actual exit code
  COLD_STAGE=migration
  log "Executing database migration..."
  docker compose \
    -f "$COMPOSE_FILE" \
    --env-file "$ENV_FILE" \
    rm -f migrate >/dev/null 2>&1 || true

  log "Running migration container in detached mode (--pull never)..."
  docker compose -f "${COLD_COMPOSE_FILE:-$COMPOSE_FILE}" --env-file "$ENV_FILE" up -d --no-build --pull never migrate

  migrate_cid="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -a -q migrate)" || fail "Migration status query failed."
  [ -n "$migrate_cid" ] || fail "Failed to inspect migrate container ID."

  max_migrate_wait="${MIGRATE_TIMEOUT_SECONDS:-60}"
  migrate_done=0
  for _ in $(seq 1 "$max_migrate_wait"); do
    m_status="$(docker inspect --format '{{.State.Status}}' "$migrate_cid")" || fail "Migration runtime status query failed."
    if [ "$m_status" = "exited" ]; then
      migrate_done=1
      break
    fi
    sleep 1
  done
  [ "$migrate_done" = "1" ] || fail "Database migration timed out after ${max_migrate_wait}s."

  migrate_exit="$(docker inspect --format '{{.State.ExitCode}}' "$migrate_cid")" || fail "Migration exit-code query failed."
  if [ "$migrate_exit" != "0" ]; then
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs migrate 2>/dev/null || true
    fail "MIGRATION_FAILED: Migration container exited with non-zero exit code ($migrate_exit)."
  fi
  log "Database migration completed successfully (exit code 0)."

  if [ -n "$QUARANTINE_MANIFEST" ]; then
    [ -f "$QUARANTINE_MANIFEST" ] || fail "Quarantine manifest not found on disk: $QUARANTINE_MANIFEST"
    current_manifest_sha="$(sha256sum "$QUARANTINE_MANIFEST" | awk '{print $1}')"
    [ "$current_manifest_sha" = "$EXPECTED_QUARANTINE_MANIFEST_SHA256" ] \
      || fail "MANIFEST_MUTATED: Quarantine manifest bytes changed between safe drain and enrollment."
    log "Enrolling approved quarantine records during zero-activity window..."
    python3 ops/enroll-quarantine.py "$COMPOSE_FILE" "$ENV_FILE" "$QUARANTINE_MANIFEST" "$PRESTAGED_RELEASE_SHA" \
      --release-lock-dir "$LOCK_DIR" \
      --expected-manifest-sha256 "$EXPECTED_QUARANTINE_MANIFEST_SHA256" \
      --prestage-proof "$PRESTAGE_PROOF_FILE"
  fi

  COLD_STAGE=target-start
  log "Starting immutable production services from prestaged images (zero pull)..."
  docker compose -f "${COLD_COMPOSE_FILE:-$COMPOSE_FILE}" --env-file "$ENV_FILE" up -d --no-build --remove-orphans --pull never
elif [ "$IMMUTABLE_RELEASE" = "1" ]; then
  log "Preparing database migration..."
  docker compose \
    -f "$COMPOSE_FILE" \
    --env-file "$ENV_FILE" \
    rm -f migrate >/dev/null 2>&1 || true

  log "Pulling and starting immutable production services..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull xianzhi-ai smartvideo-worker
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --no-build --remove-orphans
else
  log "Preparing database migration..."
  docker compose \
    -f "$COMPOSE_FILE" \
    --env-file "$ENV_FILE" \
    rm -f migrate >/dev/null 2>&1 || true

  log "Building and starting production services..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --build --remove-orphans
fi

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  expected_image_id="$(docker image inspect "$XIANZHI_IMAGE_REFERENCE" --format '{{.Id}}')" \
    || fail "Cannot inspect the manifest image locally."
  [ -n "$expected_image_id" ] || fail "Manifest image has no local image ID."

  if [ "$PRESTAGED_RELEASE" = "1" ]; then
    # Full health & readiness gating verification (including RepoDigests, API health/ready, worker healthy)
    COLD_STAGE=target-health
    verify_health_and_readiness "$expected_image_id"
    python3 -B ops/verify-image-quarantine-capability.py "${capability_args[@]}" --post-start >/dev/null || fail "RUNTIME_CAPABILITY_POST_START_MISMATCH: Actual process policy failed."
    python3 ops/verify-release-runtime.py post "$COMPOSE_FILE" "$ENV_FILE"
  else
    for service in xianzhi-ai smartvideo-worker; do
      container_id="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q "$service")"
      [ -n "$container_id" ] || fail "No running container found for $service."

      configured_image="$(docker inspect --format '{{.Config.Image}}' "$container_id")" \
        || fail "Cannot inspect the configured image for $service."
      [ "$configured_image" = "$XIANZHI_IMAGE_REFERENCE" ] \
        || fail "PARTIAL_RELEASE_DETECTED: $service Config.Image is not the manifest reference."

      running_image_id="$(docker inspect --format '{{.Image}}' "$container_id")" \
        || fail "Cannot inspect the running image for $service."
      [ "$running_image_id" = "$expected_image_id" ] \
        || fail "PARTIAL_RELEASE_DETECTED: $service is not running the manifest image digest."

      repo_digests="$(docker image inspect "$running_image_id" --format '{{range .RepoDigests}}{{println .}}{{end}}')" \
        || fail "Cannot inspect RepoDigests for $service."
      printf '%s\n' "$repo_digests" | grep -Fqx -- "$XIANZHI_IMAGE_REFERENCE" \
        || fail "PARTIAL_RELEASE_DETECTED: $service RepoDigests do not contain the manifest reference."
    done
    log "Running API and worker match the immutable release digest."
  fi

  COLD_STAGE=persist-release
  update_env_file_key "$ENV_FILE" "XIANZHI_IMAGE_REFERENCE" "$XIANZHI_IMAGE_REFERENCE"
  log "Persisted XIANZHI_IMAGE_REFERENCE to $ENV_FILE."

  log "Rechecking fresh Compose resolution from the persisted env file..."
  fresh_compose_config="$(env -u XIANZHI_IMAGE_REFERENCE docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" \
    || fail "Failed to render fresh Docker Compose configuration."
  validate_compose_desired_state "$fresh_compose_config"
  validate_video_storage_persistence "$fresh_compose_config"

  if [ "$PRESTAGED_RELEASE" = "1" ]; then
    log "Recording successful deployment in release ledger..."
    python3 - "$PRESTAGE_PROOF_FILE" "$RELEASE_LEDGER_FILE" <<'PY'
import datetime, json, os, sys
proof_file, ledger_file = sys.argv[1:3]
with open(proof_file, "r", encoding="utf-8") as pf:
    proof = json.load(pf)
nonce = proof.get("proof_nonce")
git_sha = proof.get("git_sha")
image_ref = proof.get("image_reference")
ledger = {"ledger_version": "1.0", "entries": []}
if os.path.isfile(ledger_file):
    try:
        with open(ledger_file, "r", encoding="utf-8") as lf:
            ledger = json.load(lf)
    except Exception:
        pass
entries = ledger.get("entries", [])
entries.append({
    "nonce": nonce,
    "git_sha": git_sha,
    "image_reference": image_ref,
    "status": "CONSUMED",
    "consumed_at": datetime.datetime.now(datetime.timezone.utc).isoformat()
})
ledger["entries"] = entries
os.makedirs(os.path.dirname(ledger_file) or ".", exist_ok=True)
tmp = f"{ledger_file}.tmp"
with open(tmp, "w", encoding="utf-8") as lf:
    json.dump(ledger, lf, indent=2, sort_keys=True)
    lf.write("\n")
os.replace(tmp, ledger_file)
PY
  fi
fi

if [ "$COLD_ARMED" = "1" ]; then
  COLD_STAGE=final-audit
  python3 -B ops/first-upgrade-cold.py complete --proof "$PRESTAGE_PROOF_FILE" --compose-file "$COMPOSE_FILE" --env-file "$ENV_FILE" --prestage-dir "$PRESTAGE_DIR" --owner "$OWNER_TOKEN" --ledger "$RELEASE_LEDGER_FILE" || fail "COLD_COMPLETION_FAILED: recovery required."
  COLD_ARMED=0
fi

log "Pruning dangling images..."
docker image prune -f >/dev/null 2>&1 || true

log "Deployment completed successfully."
