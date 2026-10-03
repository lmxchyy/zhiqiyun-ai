#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

TARGET_VERSION=""
IMMUTABLE_RELEASE="${IMMUTABLE_RELEASE:-0}"
OFFLINE_ROLLBACK="${OFFLINE_ROLLBACK:-0}"
ROLLBACK_RECEIPT="${ROLLBACK_RECEIPT:-}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-}"
RELEASE_REGISTRY="${RELEASE_REGISTRY:-}"
COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
PRESTAGE_DIR="${PRESTAGE_DIR:-.prestage}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

LOCK_DIR="${PRESTAGE_DIR}/release.lock"
RECOVERY_LOCK="${PRESTAGE_DIR}/release.lock.recovering"
OWNER_TOKEN="${BASHPID:-$$}_$(date +%s%N 2>/dev/null || date +%s)_$RANDOM"
IS_LOCK_OWNER=0

log() {
  printf '[rollback] %s\n' "$*"
}

fail() {
  printf '[rollback] ERROR: %s\n' "$*" >&2
  exit 1
}

cleanup() {
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

while [ $# -gt 0 ]; do
  case "$1" in
    --receipt)
      ROLLBACK_RECEIPT="$2"
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
    --offline)
      OFFLINE_ROLLBACK=1
      shift 1
      ;;
    *)
      if [ -z "$TARGET_VERSION" ]; then
        TARGET_VERSION="$1"
        shift 1
      else
        break
      fi
      ;;
  esac
done

if [ -z "$TARGET_VERSION" ] && [ -n "$ROLLBACK_RECEIPT" ]; then
  TARGET_VERSION="$ROLLBACK_RECEIPT"
elif [ -z "$TARGET_VERSION" ] && [ -n "$RELEASE_MANIFEST" ]; then
  TARGET_VERSION="$RELEASE_MANIFEST"
fi

[ -n "$TARGET_VERSION" ] || fail "TARGET_VERSION (or --receipt/--manifest) is required."

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
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is not available."

[ -f "$ENV_FILE" ] || fail "$ENV_FILE does not exist."
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE does not exist."

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
' "$XIANZHI_IMAGE_REFERENCE" <<< "$rendered" || fail "Compose desired state does not match the rollback target."
}

# Detect if target is a rollback receipt or release manifest file
target_git_sha=""
if [ -f "$TARGET_VERSION" ]; then
  if grep -Fq "receipt_version" "$TARGET_VERSION" 2>/dev/null; then
    ROLLBACK_RECEIPT="$TARGET_VERSION"
    target_git_sha="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d.get("previous_git_sha") or "")' "$TARGET_VERSION" 2>/dev/null || true)"
  else
    if [ -z "$RELEASE_MANIFEST" ]; then
      RELEASE_MANIFEST="$TARGET_VERSION"
    fi
    target_git_sha="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d.get("git_sha") or "")' "$TARGET_VERSION" 2>/dev/null || true)"
  fi
else
  target_git_sha="$TARGET_VERSION"
fi

# Guard: Ensure rollback is not being misused as a forward release
if [ -n "$target_git_sha" ] && [ "$target_git_sha" != "none" ]; then
  is_ancestor=0
  if git merge-base --is-ancestor "$target_git_sha" HEAD 2>/dev/null; then
    is_ancestor=1
  fi
  if [ "$is_ancestor" = "0" ] && [ -f "backups/release-ledger.json" ]; then
    if grep -Fq "$target_git_sha" "backups/release-ledger.json" 2>/dev/null; then
      is_ancestor=1
    fi
  fi
  if [ "$is_ancestor" = "0" ]; then
    fail "ROLLBACK_FORWARD_REJECTED: target version ($target_git_sha) is not an ancestor of current HEAD. rollback.sh cannot be used as an entry point for forward releases."
  fi
fi

if [ -n "$ROLLBACK_RECEIPT" ] && [ -f "$ROLLBACK_RECEIPT" ]; then
  log "Using verified rollback receipt: $ROLLBACK_RECEIPT"
  prev_ref="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d.get("previous_image_reference", ""))' "$ROLLBACK_RECEIPT" 2>/dev/null || true)"
  prev_id="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d.get("previous_image_id", ""))' "$ROLLBACK_RECEIPT" 2>/dev/null || true)"
  rb_manifest="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d.get("rollback_manifest_path", ""))' "$ROLLBACK_RECEIPT" 2>/dev/null || true)"
  rb_manifest_hash="$(python3 -c 'import json, sys; d=json.load(open(sys.argv[1])); print(d.get("rollback_manifest_sha256", ""))' "$ROLLBACK_RECEIPT" 2>/dev/null || true)"

  { [ -n "$prev_ref" ] && [ "$prev_ref" != "none" ]; } || fail "Rollback receipt contains no valid previous image reference."
  { [ -n "$prev_id" ] && [ "$prev_id" != "none" ]; } || fail "Rollback receipt contains no valid previous image ID."

  # Verify rollback manifest
  { [ -n "$rb_manifest" ] && [ -f "$rb_manifest" ]; } || fail "Rollback manifest specified in receipt does not exist on disk: $rb_manifest"
  actual_rb_hash="$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$rb_manifest")"
  [ "$actual_rb_hash" = "$rb_manifest_hash" ] || fail "Rollback manifest sha256 mismatch."

  # Verify image exists in Docker
  expected_image_id="$(docker image inspect "$prev_id" --format '{{.Id}}' 2>/dev/null || true)"
  [ -n "$expected_image_id" ] || fail "ROLLBACK_IMAGE_MISSING: Previous image ($prev_id) has been pruned from local docker."
  [ "$expected_image_id" = "$prev_id" ] || fail "Previous image ID mismatch in local docker."

  IMMUTABLE_RELEASE=1
  OFFLINE_ROLLBACK=1
  XIANZHI_IMAGE_REFERENCE="$prev_ref"
  export XIANZHI_IMAGE_REFERENCE
fi

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  if [ -z "${XIANZHI_IMAGE_REFERENCE:-}" ]; then
    [ -x ops/verify-release-manifest.sh ] || fail "manifest validator is not executable."
    [ -f "$RELEASE_MANIFEST" ] || fail "release manifest does not exist."
    raw_reference="$(bash ops/verify-release-manifest.sh "$RELEASE_MANIFEST" "" "" "$RELEASE_REGISTRY")"
    export XIANZHI_IMAGE_REFERENCE="$raw_reference"
  fi
  case "$XIANZHI_IMAGE_REFERENCE" in
    *@sha256:*) ;;
    *) fail "Release manifest did not provide a digest-pinned image reference." ;;
  esac
  log "Target immutable image reference: $XIANZHI_IMAGE_REFERENCE"

  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config >/dev/null
  compose_config="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" || fail "Failed to render Docker Compose configuration."
  validate_compose_desired_state "$compose_config"

cleanup_quarantine_for_rollback() {
  local target_sha="$1"
  # Quarantine table compatibility handling on rollback:
  # If rolling back to an older version that lacks the runtime quarantine barrier (such as b45e),
  # any enrolled quarantine rows cannot be safely enforced by the legacy runtime.
  # Truncate provider_execution_quarantine after taking an auditable backup snapshot.
  python3 - "$COMPOSE_FILE" "$ENV_FILE" "$target_sha" "$TIMESTAMP" <<'PY' || fail "Failed to handle quarantine rollback cleanup"
import json, os, shlex, subprocess, sys

compose_file, env_file, target_sha, ts = sys.argv[1:5]
cmd = ["docker", "compose", "-f", compose_file, "--env-file", env_file]

check_sql = "SELECT to_regclass('public.provider_execution_quarantine') IS NOT NULL;"
try:
    tbl_ok = subprocess.check_output(cmd + ["exec", "-T", "postgres", "sh", "-c",
        f'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -t -A -c "{check_sql}"'
    ], stderr=subprocess.PIPE).decode("utf-8").strip()
except Exception:
    tbl_ok = ""

if tbl_ok.lower() == "t":
    snap_sql = "SELECT coalesce(json_agg(row_to_json(q)), '[]'::json) FROM provider_execution_quarantine q;"
    try:
        raw_rows = subprocess.check_output(cmd + ["exec", "-T", "postgres", "sh", "-c",
            f'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -t -A -c "{snap_sql}"'
        ], stderr=subprocess.PIPE).decode("utf-8").strip()
        rows = json.loads(raw_rows)
    except Exception:
        rows = []
    if rows:
        os.makedirs("backups/quarantine", exist_ok=True)
        snap_file = f"backups/quarantine/revoked-{ts}.json"
        with open(snap_file, "w", encoding="utf-8") as sf:
            json.dump({"rolled_back_to": target_sha, "timestamp": ts, "count": len(rows), "records": rows}, sf, indent=2)
        res = subprocess.call(cmd + ["exec", "-T", "postgres", "sh", "-c",
            'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "TRUNCATE TABLE provider_execution_quarantine;"'
        ], stderr=subprocess.PIPE)
        if res == 0:
            print(f"[rollback] QUARANTINE_REVOKED: Truncated provider_execution_quarantine ({len(rows)} records saved to {snap_file}). Target {target_sha} lacks runtime barrier.")
        else:
            sys.stderr.write(f"[rollback] ERROR: Failed to truncate provider_execution_quarantine (exit code {res})\n")
            sys.exit(1)
PY
}

log "Stopping running API and worker containers before rollback..."
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" stop xianzhi-ai smartvideo-worker 2>/dev/null || true

cleanup_quarantine_for_rollback "$target_git_sha"

  if [ "$OFFLINE_ROLLBACK" = "1" ]; then
    log "Starting rollback production services (offline, --pull never)..."
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" rm -f migrate >/dev/null 2>&1 || true
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --no-build --remove-orphans --pull never
  else
    log "Pulling exact immutable image: $XIANZHI_IMAGE_REFERENCE"
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull xianzhi-ai smartvideo-worker
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" rm -f migrate >/dev/null 2>&1 || true
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --no-build --remove-orphans
  fi
  expected_image_id="$(docker image inspect "$XIANZHI_IMAGE_REFERENCE" --format '{{.Id}}')" \
    || fail "Cannot inspect the manifest image locally."
  [ -n "$expected_image_id" ] || fail "Manifest image has no local image ID."

  for service in xianzhi-ai smartvideo-worker; do
    container_id="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q "$service")"
    [ -n "$container_id" ] || fail "No running container found for $service."

    configured_image="$(docker inspect --format '{{.Config.Image}}' "$container_id")" \
      || fail "Cannot inspect the configured image for $service."
    [ "$configured_image" = "$XIANZHI_IMAGE_REFERENCE" ] \
      || fail "PARTIAL_ROLLBACK_DETECTED: $service Config.Image is not the target rollback reference."

    running_image_id="$(docker inspect --format '{{.Image}}' "$container_id")" \
      || fail "Cannot inspect the running image for $service."
    [ "$running_image_id" = "$expected_image_id" ] \
      || fail "PARTIAL_ROLLBACK_DETECTED: $service is not running the rollback image digest."

    repo_digests="$(docker image inspect "$running_image_id" --format '{{range .RepoDigests}}{{println .}}{{end}}')" \
      || fail "Cannot inspect RepoDigests for $service."
    printf '%s\n' "$repo_digests" | grep -Fqx -- "$XIANZHI_IMAGE_REFERENCE" \
      || fail "PARTIAL_ROLLBACK_DETECTED: $service RepoDigests do not contain the rollback reference."
  done
  log "Running services match the rollback release digest."

  update_env_file_key "$ENV_FILE" "XIANZHI_IMAGE_REFERENCE" "$XIANZHI_IMAGE_REFERENCE"
  log "Persisted XIANZHI_IMAGE_REFERENCE to $ENV_FILE."

  log "Rechecking fresh Compose resolution from the persisted env file..."
  fresh_compose_config="$(env -u XIANZHI_IMAGE_REFERENCE docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" \
    || fail "Failed to render fresh Docker Compose configuration."
  validate_compose_desired_state "$fresh_compose_config"

  log "Pruning dangling images..."
  docker image prune -f >/dev/null 2>&1 || true

  log "Rollback to $TARGET_VERSION completed successfully."
  exit 0
fi

# Legacy mutable rollback fallback
cleanup_quarantine_for_rollback "$TARGET_VERSION"
git checkout "$TARGET_VERSION"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --build --remove-orphans
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps
log "Legacy rollback to $TARGET_VERSION completed."
