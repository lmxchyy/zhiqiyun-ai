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
RELEASE_LEDGER_FILE="${RELEASE_LEDGER_FILE:-backups/release-ledger.json}"
COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
PRESTAGE_DIR="${PRESTAGE_DIR:-.prestage}"
PRESTAGE_PROOF_FILE="${PRESTAGE_PROOF_FILE:-}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

LOCK_DIR="${PRESTAGE_DIR}/release.lock"
RECOVERY_LOCK="${PRESTAGE_DIR}/release.lock.recovering"
OWNER_TOKEN="${BASHPID:-$$}_$(date +%s%N 2>/dev/null || date +%s)_$RANDOM"
IS_LOCK_OWNER=0
FIRST_UPGRADE_COLD=0
COLD_ARMED=0
COLD_STAGE=explicit-cold-rollback
COLD_TRIGGER=exit-failure

log() {
  printf '[rollback] %s\n' "$*"
}

fail() {
  printf '[rollback] ERROR: %s\n' "$*" >&2
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
    printf '%s\n' "[rollback] COLD_RECOVERY_UNKNOWN: hold retained; stop/audit not proven." >&2
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

while [ $# -gt 0 ]; do
  case "$1" in
    --first-upgrade-cold)
      FIRST_UPGRADE_COLD=1
      shift
      ;;
    --capability-proof)
      PRESTAGE_PROOF_FILE="$2"
      shift 2
      ;;
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

if [ -e "${PRESTAGE_DIR}/cold-recovery-required" ] || [ -L "${PRESTAGE_DIR}/cold-recovery-required" ]; then
  if [ "$FIRST_UPGRADE_COLD" = "1" ]; then
    acquire_release_lock
    python3 -B ops/first-upgrade-cold.py fence --prestage-dir "$PRESTAGE_DIR" --stage rollback-reentry --reason reentry || fail "COLD_RECOVERY_UNKNOWN: hold retained."
  fi
  fail "COLD_RECOVERY_REQUIRED: separate human authorization required; no rollback start."
fi
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
  env_backup_dir="${ENV_BACKUP_DIR:-backups/env}"
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

stop_services_fail_closed() {
  log "Capturing running container IDs before stop..."
  local pre_stop_cids=()
  for svc in xianzhi-ai smartvideo-worker; do
    local cids
    cids="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q "$svc" 2>/dev/null)" \
      || fail "FENCING_FAILED: Failed to query container id for service $svc before stop."
    if [ -n "$cids" ]; then
      while IFS= read -r c; do
        [ -n "$c" ] && pre_stop_cids+=("$c")
      done <<< "$cids"
    fi
  done

  log "Stopping running API and worker containers before rollback..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" stop -t 120 xianzhi-ai smartvideo-worker \
    || fail "CONTAINER_STOP_FAILED: Failed to initiate stop on API and worker containers."

  if [ "${#pre_stop_cids[@]}" -gt 0 ]; then
    for c in "${pre_stop_cids[@]}"; do
      local running status inspect_err=0
      running="$(docker inspect --format '{{.State.Running}}' "$c" 2>/dev/null)" || inspect_err=1
      status="$(docker inspect --format '{{.State.Status}}' "$c" 2>/dev/null)" || inspect_err=1
      [ "$inspect_err" -eq 0 ] || fail "FENCING_FAILED: Failed to inspect container $c after stop."
      [ "$running" = "false" ] || fail "FENCING_FAILED: Container $c is still running after stop (running=$running)."
      [ "$status" = "exited" ] || fail "FENCING_FAILED: Container $c did not reach exited state (status=$status)."
    done
  fi

  local running_count
  running_count="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps --status running -q xianzhi-ai smartvideo-worker 2>/dev/null)" \
    || fail "FENCING_FAILED: Failed to verify stopped status for containers."
  if [ -n "$running_count" ]; then
    local real_running
    real_running="$(printf '%s\n' "$running_count" | grep -v '^[[:space:]]*$' | grep -v 'NAME.*IMAGE.*STATUS' || true)"
    [ -z "$real_running" ] || fail "FENCING_FAILED: Unfenced containers detected prior to rollback."
  fi
}

preflight_check_quarantine_revocation() {
  local target_sha="$1"
  log "Preflight: Verifying database relation inspectability and backup directory for revocation..."
  python3 - "$COMPOSE_FILE" "$ENV_FILE" "$target_sha" <<'PY' || fail "QUARANTINE_REVOCATION_PREFLIGHT_FAILED: Database or backup directory preflight check failed before stop."
import os, stat, subprocess, sys, tempfile

sys.dont_write_bytecode = True

compose_file, env_file, target_sha = sys.argv[1:4]

child_env = os.environ.copy()
# Prevent leaking ambient host PGPASSWORD
child_env.pop("PGPASSWORD", None)

test_container = os.environ.get("XIANZHI_TEST_CONTAINER")
if test_container:
    psql_base = ["docker", "exec", "-i"]
    pw = os.environ.get("POSTGRES_PASSWORD")
    if pw:
        child_env["PGPASSWORD"] = pw
        psql_base.extend(["-e", "PGPASSWORD"])
    psql_base.extend([test_container, "psql", "-X", "-U", os.environ.get("POSTGRES_USER", "postgres"),
                     "-d", os.environ.get("POSTGRES_DB", "postgres"), "-v", "ON_ERROR_STOP=1", "-Atq"])
else:
    env_vars = {}
    if os.path.isfile(env_file):
        with open(env_file, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    env_vars[k.strip()] = v.strip().strip("'\"")
    db_user = os.environ.get("POSTGRES_USER") or env_vars.get("POSTGRES_USER", "xianzhi_prod")
    db_name = os.environ.get("POSTGRES_DB") or env_vars.get("POSTGRES_DB", "xianzhi")
    db_pass = os.environ.get("POSTGRES_PASSWORD") or env_vars.get("POSTGRES_PASSWORD", "")
    psql_base = ["docker", "compose", "-f", compose_file, "--env-file", env_file, "exec", "-T"]
    if db_pass:
        child_env["PGPASSWORD"] = db_pass
        psql_base.extend(["-e", "PGPASSWORD"])
    psql_base.extend(["postgres", "psql", "-X", "-U", db_user, "-d", db_name, "-v", "ON_ERROR_STOP=1", "-Atq"])

def run_sql(query):
    try:
        proc = subprocess.Popen(psql_base, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=child_env)
        out, _ = proc.communicate(input=query.encode("utf-8"), timeout=15)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.communicate()
        raise RuntimeError("Database query timed out")
    if proc.returncode != 0:
        # Never log raw DB stderr or credentials
        raise RuntimeError(f"Database query failed (exit code {proc.returncode})")
    return out.decode("utf-8", "replace").strip()

if os.environ.get("SIMULATE_PREFLIGHT_DB_INSPECTION_FAILURE") == "1":
    sys.stderr.write("[rollback] ERROR: Simulated database preflight inspection failure for test verification\n")
    sys.exit(1)

preflight_query = """BEGIN TRANSACTION READ ONLY;
SET LOCAL statement_timeout = '10s';
SELECT
  CASE WHEN to_regclass('public.provider_execution_quarantine') IS NOT NULL THEN 't' ELSE 'f' END,
  CASE WHEN to_regclass('public.provider_execution_quarantine') IS NOT NULL THEN
    CASE WHEN has_table_privilege(current_user, 'public.provider_execution_quarantine', 'SELECT')
              AND has_table_privilege(current_user, 'public.provider_execution_quarantine', 'TRUNCATE')
         THEN 't' ELSE 'f' END
    ELSE 't' END;
COMMIT;"""

try:
    res = run_sql(preflight_query)
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Failed to inspect database relations: {e}\n")
    sys.exit(1)

parts = res.split("|")
if len(parts) != 2 or parts[0].strip().lower() not in ("t", "f") or parts[1].strip().lower() not in ("t", "f"):
    sys.stderr.write(f"[rollback] ERROR: Unexpected relation inspection response\n")
    sys.exit(1)

tbl_exists = parts[0].strip().lower() == "t"
has_privs = parts[1].strip().lower() == "t"

if tbl_exists and not has_privs:
    sys.stderr.write("[rollback] ERROR: Insufficient privileges on public.provider_execution_quarantine (requires SELECT and TRUNCATE)\n")
    sys.exit(1)

if os.environ.get("SIMULATE_PREFLIGHT_BACKUP_DIR_FAILURE") == "1":
    sys.stderr.write("[rollback] ERROR: Simulated backup directory preflight failure for test verification\n")
    sys.exit(1)

backup_dir = os.environ.get("QUARANTINE_BACKUP_DIR") or "backups/quarantine"
if os.name == "nt" and len(backup_dir) >= 3 and backup_dir[0] == "/" and backup_dir[2] == "/":
    backup_dir = backup_dir[1] + ":" + backup_dir[2:]

probe_fd = None
probe_path = None
orig_dev = None
orig_ino = None
probe_token = None
cleanup_failed = False
cleanup_error_msg = None

try:
    os.makedirs(backup_dir, exist_ok=True)
    # Safely owned exclusive random temporary probe.
    # Never clobbers existing files or follows symlinks.
    probe_fd, probe_path = tempfile.mkstemp(prefix=".preflight_probe_", dir=backup_dir)
    probe_token = os.urandom(32).hex().encode("ascii") + b"\n"
    os.write(probe_fd, probe_token)
    st = os.fstat(probe_fd)
    orig_dev = st.st_dev
    orig_ino = st.st_ino
    if orig_dev is None or orig_ino is None or orig_ino == 0:
        raise RuntimeError("Unsupported identity verification: filesystem does not report device/inode")
    os.close(probe_fd)
    probe_fd = None
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Backup directory {backup_dir} is not writable or identity verification unsupported: {e}\n")
    sys.exit(1)
finally:
    if probe_fd is not None:
        try:
            os.close(probe_fd)
        except Exception:
            pass

    # Strictly verify identity before deletion: prove same created object via device/inode/file-id AND token content
    if probe_path is not None:
        try:
            if os.path.islink(probe_path):
                cleanup_failed = True
                cleanup_error_msg = f"Probe cleanup aborted: {probe_path} was substituted with a symlink"
            else:
                lst = os.lstat(probe_path)
                if not stat.S_ISREG(lst.st_mode):
                    cleanup_failed = True
                    cleanup_error_msg = f"Probe cleanup aborted: {probe_path} is not a regular file"
                elif lst.st_dev != orig_dev or lst.st_ino != orig_ino:
                    cleanup_failed = True
                    cleanup_error_msg = f"Probe cleanup aborted: file identity mismatch (expected dev={orig_dev}, ino={orig_ino}; got dev={lst.st_dev}, ino={lst.st_ino})"
                else:
                    with open(probe_path, "rb") as pf:
                        content = pf.read()
                    if content != probe_token:
                        cleanup_failed = True
                        cleanup_error_msg = f"Probe cleanup aborted: token/content mismatch"
                    else:
                        lst_after = os.lstat(probe_path)
                        if lst_after.st_dev != orig_dev or lst_after.st_ino != orig_ino or not stat.S_ISREG(lst_after.st_mode):
                            cleanup_failed = True
                            cleanup_error_msg = f"Probe cleanup aborted: file identity changed during content verification"
                        else:
                            os.remove(probe_path)
        except Exception as e:
            cleanup_failed = True
            cleanup_error_msg = f"Probe cleanup uncertainty or failure: {e}"

if cleanup_failed:
    sys.stderr.write(f"[rollback] ERROR: {cleanup_error_msg}\n")
    sys.exit(1)

sys.exit(0)
PY
}

cleanup_quarantine_for_rollback() {
  local target_version="$1"
  local target_capable="${2:-}"
  [ -n "$target_version" ] || fail "TARGET_CAPABILITY_UNKNOWN: Target commit version is empty."
  local target_sha
  target_sha="$(git rev-parse --verify "${target_version}^{commit}" 2>/dev/null || true)"
  if [ -z "$target_sha" ]; then
    fail "TARGET_CAPABILITY_UNKNOWN: Target commit for version '$target_version' cannot be resolved in git."
  fi

  [ -n "$target_capable" ] || fail "TARGET_CAPABILITY_UNKNOWN: Cleanup requires an explicit preverified runtime capability decision; migration presence is not capability."

  if [ "$target_capable" = "1" ]; then
    log "QUARANTINE_PRESERVED: Target $target_sha possesses authenticated packaged runtime barrier evidence. Active quarantine records retained."
    return 0
  fi

  # Legacy target lacks Issue #200 barrier
  if [ "${EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION:-0}" != "1" ]; then
    fail "LEGACY_TARGET_BARRIER_UNSUPPORTED: Target $target_sha lacks Issue #200 runtime quarantine barrier. Emergency return to legacy risk requires explicit emergency authorization (EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION=1). Truncating quarantine records without authorization is forbidden."
  fi

  log "Emergency authorization granted to revoke quarantine records for legacy target $target_sha..."
  python3 - "$COMPOSE_FILE" "$ENV_FILE" "$target_sha" "$TIMESTAMP" <<'PY' || fail "Failed to handle quarantine rollback cleanup"
import json, os, subprocess, sys

sys.dont_write_bytecode = True

compose_file, env_file, target_sha, ts = sys.argv[1:5]
child_env = os.environ.copy()
child_env.pop("PGPASSWORD", None)

test_container = os.environ.get("XIANZHI_TEST_CONTAINER")
if test_container:
    psql_base = ["docker", "exec", "-i"]
    pw = os.environ.get("POSTGRES_PASSWORD")
    if pw:
        child_env["PGPASSWORD"] = pw
        psql_base.extend(["-e", "PGPASSWORD"])
    psql_base.extend([test_container, "psql", "-X", "-U", os.environ.get("POSTGRES_USER", "postgres"),
                     "-d", os.environ.get("POSTGRES_DB", "postgres"), "-v", "ON_ERROR_STOP=1", "-Atq"])
else:
    env_vars = {}
    if os.path.isfile(env_file):
        with open(env_file, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    env_vars[k.strip()] = v.strip().strip("'\"")
    db_user = os.environ.get("POSTGRES_USER") or env_vars.get("POSTGRES_USER", "xianzhi_prod")
    db_name = os.environ.get("POSTGRES_DB") or env_vars.get("POSTGRES_DB", "xianzhi")
    db_pass = os.environ.get("POSTGRES_PASSWORD") or env_vars.get("POSTGRES_PASSWORD", "")
    psql_base = ["docker", "compose", "-f", compose_file, "--env-file", env_file, "exec", "-T"]
    if db_pass:
        child_env["PGPASSWORD"] = db_pass
        psql_base.extend(["-e", "PGPASSWORD"])
    psql_base.extend(["postgres", "psql", "-X", "-U", db_user, "-d", db_name, "-v", "ON_ERROR_STOP=1", "-Atq"])

def run_sql(query):
    proc = subprocess.Popen(psql_base, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=child_env)
    out, _ = proc.communicate(input=query.encode("utf-8"))
    if proc.returncode != 0:
        # Post-stop failures must not disclose credentials or raw database stderr either.
        raise RuntimeError(f"Database query failed (exit code {proc.returncode})")
    return out.decode("utf-8", "replace").strip()

try:
    tbl_ok = run_sql("SELECT to_regclass('public.provider_execution_quarantine') IS NOT NULL;")
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Failed to inspect database relations: {e}\n")
    sys.exit(1)

tbl_ok_clean = tbl_ok.strip().lower()
if tbl_ok_clean not in ("t", "f"):
    sys.stderr.write(f"[rollback] ERROR: Unexpected to_regclass response: {tbl_ok}\n")
    sys.exit(1)

if tbl_ok_clean == "f":
    print("[rollback] QUARANTINE_TABLE_NOT_FOUND: provider_execution_quarantine does not exist. Nothing to revoke.")
    sys.exit(0)

# Table exists; query active quarantine records
try:
    raw_rows = run_sql("SELECT coalesce(json_agg(row_to_json(q)), '[]'::json) FROM provider_execution_quarantine q;")
    rows = json.loads(raw_rows)
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Failed to query quarantine snapshot: {e}\n")
    sys.exit(1)

snap_file = None
if rows:
    backup_dir = os.environ.get("QUARANTINE_BACKUP_DIR") or "backups/quarantine"
    if os.name == "nt" and len(backup_dir) >= 3 and backup_dir[0] == "/" and backup_dir[2] == "/":
        backup_dir = backup_dir[1] + ":" + backup_dir[2:]
    os.makedirs(backup_dir, exist_ok=True)
    snap_file = os.path.join(backup_dir, f"revoked-{ts}-{target_sha}.json")
    snapshot_payload = json.dumps({
        "rolled_back_to": target_sha,
        "timestamp": ts,
        "count": len(rows),
        "records": rows
    }, indent=2).encode("utf-8")

    # Safe snapshot before TRUNCATE: write using exclusive creation (O_CREAT | O_EXCL), fsync
    try:
        if os.environ.get("SIMULATE_SNAPSHOT_FAILURE") == "1":
            raise IOError("Simulated snapshot write failure for test verification")

        flags = os.O_CREAT | os.O_EXCL | os.O_WRONLY
        if hasattr(os, "O_BINARY"):
            flags |= os.O_BINARY
        fd = os.open(snap_file, flags, 0o600)
        try:
            with os.fdopen(fd, "wb", closefd=False) as sf:
                sf.write(snapshot_payload)
                sf.flush()
            os.fsync(fd)
        finally:
            os.close(fd)
    except Exception as e:
        sys.stderr.write(f"[rollback] ERROR: SNAPSHOT_WRITE_FAILED: Failed to safely create/write snapshot file {snap_file}: {e}\n")
        sys.exit(1)

    # Re-read back the snapshot to verify file integrity BEFORE executing TRUNCATE
    try:
        with open(snap_file, "rb") as rf:
            readback_bytes = rf.read()
        if readback_bytes != snapshot_payload:
            sys.stderr.write(f"[rollback] ERROR: SNAPSHOT_INTEGRITY_MISMATCH: Snapshot readback bytes do not match expected payload\n")
            sys.exit(1)
        verified_obj = json.loads(readback_bytes.decode("utf-8"))
        if verified_obj.get("count") != len(rows) or len(verified_obj.get("records", [])) != len(rows):
            sys.stderr.write(f"[rollback] ERROR: SNAPSHOT_INTEGRITY_MISMATCH: Snapshot readback structure corrupt\n")
            sys.exit(1)
    except Exception as e:
        sys.stderr.write(f"[rollback] ERROR: SNAPSHOT_VERIFICATION_FAILED: Failed to verify snapshot {snap_file}: {e}\n")
        sys.exit(1)

# Snapshot successfully written and verified (or no rows to snapshot). Now execute TRUNCATE.
try:
    run_sql("TRUNCATE TABLE provider_execution_quarantine;")
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: TRUNCATE_FAILED: Failed to truncate provider_execution_quarantine: {e}\n")
    sys.exit(1)

# Post-truncate readback: assert strictly 0
try:
    if os.environ.get("SIMULATE_POST_TRUNCATE_READBACK_MISMATCH") == "1":
        rem = "1"
    else:
        rem = run_sql("SELECT count(*) FROM provider_execution_quarantine;")
    if rem.strip() != "0":
        sys.stderr.write(f"[rollback] ERROR: READBACK_VERIFICATION_FAILED: table still contains {rem} rows after truncate\n")
        sys.exit(1)
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Post-truncate readback verification failed: {e}\n")
    sys.exit(1)

if rows:
    print(f"[rollback] QUARANTINE_REVOKED: Emergency revoked and truncated provider_execution_quarantine ({len(rows)} records saved to {snap_file}). Target {target_sha} lacks runtime barrier.")
else:
    print(f"[rollback] QUARANTINE_REVOKED: Emergency revoked provider_execution_quarantine (0 records). Target {target_sha} lacks runtime barrier.")
PY
}

# Detect target type: rollback receipt, release manifest file, or explicit git commit/tag
target_git_sha=""
RECEIPT_PREV_GIT_SHA=""
MANIFEST_GIT_SHA=""
FROZEN_PREV_REF=""
FROZEN_PREV_ID=""
FROZEN_RB_MANIFEST=""
FROZEN_RB_HASH=""

if [ -n "$ROLLBACK_RECEIPT" ] || { [ -f "$TARGET_VERSION" ] && grep -Fq "receipt_version" "$TARGET_VERSION" 2>/dev/null; }; then
  [ -n "$ROLLBACK_RECEIPT" ] || ROLLBACK_RECEIPT="$TARGET_VERSION"
  [ -f "$ROLLBACK_RECEIPT" ] || fail "Rollback receipt specified does not exist on disk: $ROLLBACK_RECEIPT"

  # Bind manifest bytes being parsed to the hash being checked; freeze validated values
  # Avoid rereads and ensure strict identity parsing
  validated_receipt_env="$(python3 - "$ROLLBACK_RECEIPT" <<'PY' || fail "Rollback manifest sha256 mismatch."
import hashlib, json, os, shlex, sys

def normpath(p):
    if os.name == "nt" and len(p) >= 3 and p[0] == "/" and p[2] == "/":
        return p[1] + ":" + p[2:]
    return p

receipt_file = normpath(sys.argv[1])
if not os.path.isfile(receipt_file):
    sys.stderr.write(f"[rollback] ERROR: Rollback receipt does not exist: {receipt_file}\n")
    sys.exit(1)

try:
    with open(receipt_file, "rb") as rf:
        receipt_bytes = rf.read()
    receipt_data = json.loads(receipt_bytes.decode("utf-8"))
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Failed to parse rollback receipt: {e}\n")
    sys.exit(1)

prev_git_sha = receipt_data.get("previous_git_sha")
prev_ref = receipt_data.get("previous_image_reference")
prev_id = receipt_data.get("previous_image_id")
manifest_path = receipt_data.get("rollback_manifest_path")
manifest_hash = receipt_data.get("rollback_manifest_sha256")

if not prev_git_sha or not isinstance(prev_git_sha, str):
    sys.stderr.write("[rollback] ERROR: Rollback receipt contains no valid previous_git_sha\n")
    sys.exit(1)
if not prev_ref or prev_ref == "none" or not isinstance(prev_ref, str):
    sys.stderr.write("[rollback] ERROR: Rollback receipt contains no valid previous image reference.\n")
    sys.exit(1)
if not prev_id or prev_id == "none" or not isinstance(prev_id, str):
    sys.stderr.write("[rollback] ERROR: Rollback receipt contains no valid previous image ID.\n")
    sys.exit(1)
if not manifest_path or not isinstance(manifest_path, str):
    sys.stderr.write("[rollback] ERROR: Rollback receipt contains no rollback_manifest_path\n")
    sys.exit(1)
if not manifest_hash or not isinstance(manifest_hash, str):
    sys.stderr.write("[rollback] ERROR: Rollback receipt contains no rollback_manifest_sha256\n")
    sys.exit(1)

manifest_file = normpath(manifest_path)
if not os.path.isabs(manifest_file) and not os.path.exists(manifest_file):
    candidate = os.path.join(os.path.dirname(receipt_file), manifest_file)
    if os.path.exists(candidate):
        manifest_file = candidate

if not os.path.isfile(manifest_file):
    sys.stderr.write(f"[rollback] ERROR: Rollback manifest specified in receipt does not exist on disk: {manifest_file}\n")
    sys.exit(1)

try:
    with open(manifest_file, "rb") as mf:
        manifest_bytes = mf.read()
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Failed to read rollback manifest: {e}\n")
    sys.exit(1)

actual_hash = hashlib.sha256(manifest_bytes).hexdigest()
if actual_hash.lower() != manifest_hash.strip().lower():
    sys.stderr.write("[rollback] ERROR: Rollback manifest sha256 mismatch.\n")
    sys.exit(1)

try:
    manifest_data = json.loads(manifest_bytes.decode("utf-8"))
except Exception as e:
    sys.stderr.write(f"[rollback] ERROR: Failed to parse rollback manifest: {e}\n")
    sys.exit(1)

manifest_git_sha = manifest_data.get("git_sha")
if not manifest_git_sha or not isinstance(manifest_git_sha, str):
    sys.stderr.write("[rollback] ERROR: Rollback manifest contains no valid git_sha\n")
    sys.exit(1)

print(f"RECEIPT_PREV_GIT_SHA={shlex.quote(prev_git_sha.strip())}")
print(f"MANIFEST_GIT_SHA={shlex.quote(manifest_git_sha.strip())}")
print(f"FROZEN_PREV_REF={shlex.quote(prev_ref.strip())}")
print(f"FROZEN_PREV_ID={shlex.quote(prev_id.strip())}")
print(f"FROZEN_RB_MANIFEST={shlex.quote(manifest_file)}")
print(f"FROZEN_RB_HASH={shlex.quote(actual_hash)}")
print(f"FROZEN_RECEIPT_HASH={shlex.quote(hashlib.sha256(receipt_bytes).hexdigest())}")
PY
)"
  eval "$validated_receipt_env"
  if [ -z "$TARGET_VERSION" ] || [ "$TARGET_VERSION" = "$ROLLBACK_RECEIPT" ]; then
    target_git_sha="$RECEIPT_PREV_GIT_SHA"
  else
    target_git_sha="$TARGET_VERSION"
  fi
elif [ -f "$TARGET_VERSION" ]; then
  if [ -z "$RELEASE_MANIFEST" ]; then
    RELEASE_MANIFEST="$TARGET_VERSION"
  fi
  target_git_sha="$(python3 -c 'import json, sys, os; p = sys.argv[1]; p = (p[1] + ":" + p[2:]) if (os.name == "nt" and len(p) >= 3 and p[0] == "/" and p[2] == "/") else p; d=json.load(open(p, encoding="utf-8")); print(d.get("git_sha") or "")' "$TARGET_VERSION" 2>/dev/null || true)"
else
  target_git_sha="$TARGET_VERSION"
fi

[ -n "$target_git_sha" ] || fail "TARGET_CAPABILITY_UNKNOWN: Target git SHA could not be identified."

# Guard: Ensure rollback is not being misused as a forward release
is_ancestor=0
if git merge-base --is-ancestor "$target_git_sha" HEAD 2>/dev/null; then
  is_ancestor=1
fi
if [ "$is_ancestor" = "0" ] && [ -f "$RELEASE_LEDGER_FILE" ]; then
  if grep -Fq "$target_git_sha" "$RELEASE_LEDGER_FILE" 2>/dev/null; then
    is_ancestor=1
  fi
fi
if [ "$is_ancestor" = "0" ]; then
  fail "ROLLBACK_FORWARD_REJECTED: target version ($target_git_sha) is not an ancestor of current HEAD. rollback.sh cannot be used as an entry point for forward releases."
fi

# Preflight: Target Identity Resolution and Pinning
# Unknown target should be rejected before stop even if ledger allows ancestry.
resolved_target_sha="$(git rev-parse --verify "${target_git_sha}^{commit}" 2>/dev/null || true)"
if [ -z "$resolved_target_sha" ]; then
  fail "TARGET_CAPABILITY_UNKNOWN: Target commit for version '$target_git_sha' cannot be resolved in git."
fi
PINNED_TARGET_SHA="$resolved_target_sha"

# Preflight: Rollback Receipt, Manifest and Target Identity Strict Binding
if [ -n "$ROLLBACK_RECEIPT" ]; then
  resolved_receipt_sha="$(git rev-parse --verify "${RECEIPT_PREV_GIT_SHA}^{commit}" 2>/dev/null || true)"
  if [ -z "$resolved_receipt_sha" ]; then
    fail "TARGET_CAPABILITY_UNKNOWN: Target commit for receipt previous_git_sha '$RECEIPT_PREV_GIT_SHA' cannot be resolved in git."
  fi
  resolved_manifest_sha="$(git rev-parse --verify "${MANIFEST_GIT_SHA}^{commit}" 2>/dev/null || true)"
  if [ -z "$resolved_manifest_sha" ]; then
    fail "TARGET_CAPABILITY_UNKNOWN: Target commit for rollback manifest git_sha '$MANIFEST_GIT_SHA' cannot be resolved in git."
  fi

  if [ "$resolved_receipt_sha" != "$resolved_manifest_sha" ] || [ "$resolved_receipt_sha" != "$PINNED_TARGET_SHA" ] || [ "$resolved_manifest_sha" != "$PINNED_TARGET_SHA" ]; then
    fail "ROLLBACK_IDENTITY_MISMATCH: Rollback receipt previous_git_sha ($RECEIPT_PREV_GIT_SHA) does not match rollback manifest git_sha ($MANIFEST_GIT_SHA) or pinned target ($PINNED_TARGET_SHA)."
  fi

  if [ "$RECEIPT_PREV_GIT_SHA" != "$MANIFEST_GIT_SHA" ]; then
    case "$PINNED_TARGET_SHA" in
      "$RECEIPT_PREV_GIT_SHA"*) ;;
      *) fail "ROLLBACK_IDENTITY_MISMATCH: Rollback receipt previous_git_sha ($RECEIPT_PREV_GIT_SHA) does not match pinned target ($PINNED_TARGET_SHA)." ;;
    esac
    case "$PINNED_TARGET_SHA" in
      "$MANIFEST_GIT_SHA"*) ;;
      *) fail "ROLLBACK_IDENTITY_MISMATCH: Rollback manifest git_sha ($MANIFEST_GIT_SHA) does not match pinned target ($PINNED_TARGET_SHA)." ;;
    esac
  fi

  # Verify image exists in Docker before capability/stop
  expected_image_id="$(docker image inspect "$FROZEN_PREV_ID" --format '{{.Id}}' 2>/dev/null || true)"
  [ -n "$expected_image_id" ] || fail "ROLLBACK_IMAGE_MISSING: Previous image ($FROZEN_PREV_ID) has been pruned from local docker."
  [ "$expected_image_id" = "$FROZEN_PREV_ID" ] || fail "Previous image ID mismatch in local docker."

  IMMUTABLE_RELEASE=1
  OFFLINE_ROLLBACK=1
  XIANZHI_IMAGE_REFERENCE="$FROZEN_PREV_REF"
  export XIANZHI_IMAGE_REFERENCE
  log "Using verified rollback receipt: $ROLLBACK_RECEIPT (manifest: $FROZEN_RB_MANIFEST, sha256: $FROZEN_RB_HASH)"
fi

if [ "$FIRST_UPGRADE_COLD" = "1" ]; then
  [ -n "$ROLLBACK_RECEIPT" ] && [ -n "$PRESTAGE_PROOF_FILE" ] || fail "COLD_POLICY_REQUIRED: actual receipt and signed Proof required."
  # Arm validates target capability, protected bytes, official exact unsupported
  # rollback identity and signed no-restart policy. This branch NEVER runs SQL.
  COLD_ARMED=1
  python3 -B ops/first-upgrade-cold.py arm --proof "$PRESTAGE_PROOF_FILE" --receipt "$ROLLBACK_RECEIPT" --compose-file "$COMPOSE_FILE" --env-file "$ENV_FILE" --prestage-dir "$PRESTAGE_DIR" --owner "$OWNER_TOKEN" --stage explicit-cold-rollback >/dev/null || fail "COLD_POLICY_INVALID: no recovery authorized."
  python3 -B ops/first-upgrade-cold.py fence --prestage-dir "$PRESTAGE_DIR" --stage explicit-cold-rollback --reason explicit-cold-rollback --owner "$OWNER_TOKEN" --proof "$PRESTAGE_PROOF_FILE" || fail "COLD_RECOVERY_UNKNOWN: hold retained."
  COLD_ARMED=0
  fail "STOPPED_RECOVERY_REQUIRED: no old business image started; separate human authorization required."
fi

# Legacy emergency entry contract intentionally changed: no stop/revoke/build.
# Emergency authorization never substitutes for actual target runtime evidence.
TARGET_CAPABLE=0
if [ "$IMMUTABLE_RELEASE" != "1" ] || [ -z "$PRESTAGE_PROOF_FILE" ] || [ ! -f "$PRESTAGE_PROOF_FILE" ]; then
  # Preserve the original read-only emergency preflight security surface; even
  # a successful preflight is now followed by unconditional pre-stop rejection.
  if [ "${EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION:-0}" = "1" ]; then
    preflight_check_quarantine_revocation "$PINNED_TARGET_SHA"
  fi
  fail "LEGACY_TARGET_BARRIER_UNSUPPORTED: RUNTIME_CAPABILITY_PROOF_MISSING: Actual immutable target and authenticated runtime Proof are required before stop, including emergency rollback."
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

  capability_args=(--verify-proof "$PRESTAGE_PROOF_FILE" --image "$XIANZHI_IMAGE_REFERENCE" --release-sha "$PINNED_TARGET_SHA" --compose-file "$COMPOSE_FILE" --env-file "$ENV_FILE")
  if [ -n "$ROLLBACK_RECEIPT" ]; then
    capability_args+=(--rollback --receipt-sha256 "$FROZEN_RECEIPT_HASH")
  fi
  verified_capability_id="$(python3 -B ops/verify-image-quarantine-capability.py "${capability_args[@]}")" || fail "RUNTIME_CAPABILITY_INVALID: Actual rollback image proof failed before stop."
  if [ -n "$ROLLBACK_RECEIPT" ]; then
    [ "$verified_capability_id" = "$FROZEN_PREV_ID" ] || fail "RUNTIME_CAPABILITY_IMAGE_MISMATCH: Frozen receipt ID differs."
  fi
  expected_image_id="$verified_capability_id"
  TARGET_CAPABLE=1
  OFFLINE_ROLLBACK=1

  stop_services_fail_closed
  cleanup_quarantine_for_rollback "$PINNED_TARGET_SHA" "$TARGET_CAPABLE"

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
  python3 -B ops/verify-image-quarantine-capability.py "${capability_args[@]}" --post-start >/dev/null || fail "RUNTIME_CAPABILITY_POST_START_MISMATCH: Actual rollback process policy failed."
  log "Running services match the rollback release digest and authenticated process policy."

  update_env_file_key "$ENV_FILE" "XIANZHI_IMAGE_REFERENCE" "$XIANZHI_IMAGE_REFERENCE"
  log "Persisted XIANZHI_IMAGE_REFERENCE to $ENV_FILE."

  log "Rechecking fresh Compose resolution from the persisted env file..."
  fresh_compose_config="$( (unset XIANZHI_IMAGE_REFERENCE && docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null) )" \
    || fail "Failed to render fresh Docker Compose configuration."
  validate_compose_desired_state "$fresh_compose_config"

  log "Pruning dangling images..."
  docker image prune -f >/dev/null 2>&1 || true

  log "Rollback to $TARGET_VERSION completed successfully."
  exit 0
fi

# Legacy mutable rollback fallback
stop_services_fail_closed
cleanup_quarantine_for_rollback "$PINNED_TARGET_SHA" "$TARGET_CAPABLE"
git checkout "$PINNED_TARGET_SHA"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --build --remove-orphans
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps
log "Legacy rollback to $TARGET_VERSION completed."
