#!/usr/bin/env bash
set -uo pipefail
IFS=$'\n\t'

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$REPO_ROOT/ops/backup-offsite-upload-pending.sh"
SCENARIO="${1:-invalid-valid-valid}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/backup-offsite-batch.XXXXXX")"
POSTGRES_ROOT="$ROOT/backups/postgres"
FAKE_ROOT="$ROOT/fake-object-store"
mkdir -p "$POSTGRES_ROOT" "$FAKE_ROOT"

# Create a mock docker that intercepts docker compose run backup-uploader
# and calls the uploader directly with the fake provider, mapping container
# paths to host paths.
# CRITICAL: The mock docker actively drains stdin (cat >/dev/null) by default
# to simulate an interactive / attached container session that consumes
# un-isolated stdin. If the caller does not redirect stdin (< /dev/null),
# this will drain the outer iterator stream and fail the test.
MOCK_BIN="$ROOT/bin"
mkdir -p "$MOCK_BIN"
cat > "$MOCK_BIN/docker" <<'DOCKEREOF'
#!/usr/bin/env bash
if [[ "$1" == "compose" ]]; then
  # Simulate container execution reading stdin unless isolated
  if [[ "${MOCK_DRAIN_STDIN:-1}" == "1" ]]; then
    cat >/dev/null 2>&1 || true
  fi

  FILE=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --file) FILE="$2"; shift 2;;
      *) shift;;
    esac
  done
  if [[ -z "$FILE" ]]; then echo "MISSING_FILE" >&2; exit 2; fi
  BASENAME="$(basename "$FILE")"
  HOST_FILE="$MOCK_HOST_ROOT/$BASENAME"
  HOST_ROOT="$MOCK_HOST_ROOT/.."

  # Check simulated uploader failure
  if [[ "$BASENAME" == *"simulated_fail"* ]]; then
    echo '{"error":"simulated uploader network failure","status":"OFFSITE_UPLOAD_FAILED"}' >&2
    exit 1
  fi

  BACKUP_OBS_FAKE=1 \
  BACKUP_OBS_BUCKET=fake-bucket \
  BACKUP_OBS_ENDPOINT=https://obs.example.invalid \
  BACKUP_OBS_REGION=cn-north-4 \
  BACKUP_OBJECT_FAKE_ROOT="$MOCK_FAKE_ROOT" \
  BACKUP_OBS_ENV_FILE="$MOCK_REPO_ROOT/ops/backup-uploader/backup-obs.env.example" \
  CONNECTOR_SECRET_ENCRYPTION_KEY=test-connector-secret-key-32bytes \
  "$MOCK_REPO_ROOT/ops/backup-upload-object-storage.sh" \
    --root "$HOST_ROOT" \
    --provider obs \
    --fake-root "$MOCK_FAKE_ROOT" \
    --file "$HOST_FILE" \
    --upload \
    --json
  exit $?
fi
echo "Unexpected docker command: $*" >&2
exit 1
DOCKEREOF
chmod +x "$MOCK_BIN/docker"

# Export paths for the mock docker script
export MOCK_HOST_ROOT="$POSTGRES_ROOT"
export MOCK_FAKE_ROOT="$FAKE_ROOT"
export MOCK_REPO_ROOT="$REPO_ROOT"
export PATH="$MOCK_BIN:$PATH"

create_valid_backup() {
  local name="$1"
  local file="$POSTGRES_ROOT/$name"
  local meta="$file.meta.json"
  printf 'valid payload for %s\n' "$name" | gzip -c >"$file"
  local bytes sha
  bytes="$(wc -c <"$file" | tr -d '[:space:]')"
  sha="$(sha256sum "$file" | awk '{print $1}')"
  printf '{\n  "git_sha": "test-sha",\n  "short_sha": "test",\n  "branch": "main",\n  "hostname": "fixture",\n  "started_at": "2026-08-22T15:52:44+08:00",\n  "completed_at": "2026-08-22T15:52:45+08:00",\n  "backup_file": "%s",\n  "bytes": %s,\n  "sha256": "%s"\n}\n' "$file" "$bytes" "$sha" >"$meta"
  touch -t 202608221552.44 "$file" "$meta"
}

create_invalid_no_meta() {
  local name="$1"
  local file="$POSTGRES_ROOT/$name"
  printf 'invalid payload without meta for %s\n' "$name" >"$file"
  touch -t 202608211957.34 "$file"
}

pre_upload_verified() {
  local name="$1"
  create_valid_backup "$name"
  local file="$POSTGRES_ROOT/$name"
  BACKUP_OBS_FAKE=1 \
  BACKUP_OBS_BUCKET=fake-bucket \
  BACKUP_OBS_ENDPOINT=https://obs.example.invalid \
  BACKUP_OBS_REGION=cn-north-4 \
  BACKUP_OBJECT_FAKE_ROOT="$FAKE_ROOT" \
  BACKUP_OBS_ENV_FILE="$REPO_ROOT/ops/backup-uploader/backup-obs.env.example" \
  "$REPO_ROOT/ops/backup-upload-object-storage.sh" \
    --root "$POSTGRES_ROOT/.." \
    --provider obs \
    --fake-root "$FAKE_ROOT" \
    --file "$file" \
    --upload \
    --json >/dev/null
}

run_batch() {
  BACKUP_ROOT="$POSTGRES_ROOT" \
  COMPOSE_FILE="$REPO_ROOT/compose.prod.yml" \
  ENV_FILE="$REPO_ROOT/.env.production.example" \
  BACKUP_OBS_ENV_FILE="$REPO_ROOT/ops/backup-uploader/backup-obs.env.example" \
  BACKUP_UPLOADER_IMAGE='xianzhi-ai-platform:test@sha256:000000000000000000000000000000000000000000000000000000000000' \
  "$SCRIPT" >"$ROOT/output.txt" 2>&1
  local rc=$?
  cat "$ROOT/output.txt"
  return "$rc"
}

case "$SCENARIO" in
  invalid-valid-valid|first-invalid-second-valid|child-consumes-stdin)
    create_invalid_no_meta "db_20260821_195734.sql"
    create_valid_backup "db_20260822_155244_aaaa000001.sql.gz"
    create_valid_backup "db_20260823_160000_bbbb000002.sql.gz"
    run_batch
    rc=$?
    grep -q "LOCAL_BACKUP_INVALID" "$ROOT/output.txt" || { echo "MISSING_LOCAL_BACKUP_INVALID" >&2; exit 1; }
    grep -q "db_20260821_195734.sql" "$ROOT/output.txt" || { echo "MISSING_INVALID_NAME" >&2; exit 1; }
    grep -q "db_20260822_155244_aaaa000001.sql.gz" "$ROOT/output.txt" || { echo "MISSING_VALID_1" >&2; exit 1; }
    grep -q "db_20260823_160000_bbbb000002.sql.gz" "$ROOT/output.txt" || { echo "MISSING_VALID_2" >&2; exit 1; }
    grep -q "^TOTAL=3" "$ROOT/output.txt" || { echo "EXPECTED_TOTAL_3" >&2; exit 1; }
    grep -q "^INVALID=1" "$ROOT/output.txt" || { echo "EXPECTED_INVALID_1" >&2; exit 1; }
    grep -q "^UPLOADED=2" "$ROOT/output.txt" || { echo "EXPECTED_UPLOADED_2" >&2; exit 1; }
    grep -q "^FAILED=0" "$ROOT/output.txt" || { echo "EXPECTED_FAILED_0" >&2; exit 1; }
    exit "$rc"
    ;;
  invalid-verified-valid)
    create_invalid_no_meta "db_20260821_195734.sql"
    pre_upload_verified "db_20260822_155244_aaaa000001.sql.gz"
    create_valid_backup "db_20260823_160000_bbbb000002.sql.gz"
    run_batch
    rc=$?
    grep -q "^TOTAL=3" "$ROOT/output.txt" || { echo "EXPECTED_TOTAL_3" >&2; exit 1; }
    grep -q "^INVALID=1" "$ROOT/output.txt" || { echo "EXPECTED_INVALID_1" >&2; exit 1; }
    grep -q "^SKIPPED_ALREADY_VERIFIED=1" "$ROOT/output.txt" || { echo "EXPECTED_SKIPPED_1" >&2; exit 1; }
    grep -q "^UPLOADED=1" "$ROOT/output.txt" || { echo "EXPECTED_UPLOADED_1" >&2; exit 1; }
    grep -q "^FAILED=0" "$ROOT/output.txt" || { echo "EXPECTED_FAILED_0" >&2; exit 1; }
    exit "$rc"
    ;;
  valid-failed-valid)
    create_valid_backup "db_20260822_155244_aaaa000001.sql.gz"
    create_valid_backup "db_20260823_simulated_fail_bbbb.sql.gz"
    create_valid_backup "db_20260824_170000_cccc000003.sql.gz"
    run_batch
    rc=$?
    grep -q "^TOTAL=3" "$ROOT/output.txt" || { echo "EXPECTED_TOTAL_3" >&2; exit 1; }
    grep -q "^UPLOADED=2" "$ROOT/output.txt" || { echo "EXPECTED_UPLOADED_2" >&2; exit 1; }
    grep -q "^FAILED=1" "$ROOT/output.txt" || { echo "EXPECTED_FAILED_1" >&2; exit 1; }
    grep -q "^INVALID=0" "$ROOT/output.txt" || { echo "EXPECTED_INVALID_0" >&2; exit 1; }
    # Verify the 3rd valid file was actually processed
    grep -q "db_20260824_170000_cccc000003.sql.gz" "$ROOT/output.txt" || { echo "MISSING_THIRD_FILE" >&2; exit 1; }
    exit "$rc"
    ;;
  all-valid)
    create_valid_backup "db_20260822_155244_aaaa000001.sql.gz"
    create_valid_backup "db_20260823_160000_bbbb000002.sql.gz"
    run_batch
    rc=$?
    grep -q "^TOTAL=2" "$ROOT/output.txt" || { echo "EXPECTED_TOTAL_2" >&2; exit 1; }
    grep -q "^UPLOADED=2" "$ROOT/output.txt" || { echo "EXPECTED_UPLOADED_2" >&2; exit 1; }
    grep -q "^INVALID=0" "$ROOT/output.txt" || { echo "EXPECTED_INVALID_0" >&2; exit 1; }
    grep -q "^FAILED=0" "$ROOT/output.txt" || { echo "EXPECTED_FAILED_0" >&2; exit 1; }
    exit "$rc"
    ;;
  all-invalid)
    create_invalid_no_meta "db_20260821_195734.sql"
    create_invalid_no_meta "db_20260821_212820.sql"
    run_batch
    rc=$?
    grep -q "^TOTAL=2" "$ROOT/output.txt" || { echo "EXPECTED_TOTAL_2" >&2; exit 1; }
    grep -q "^INVALID=2" "$ROOT/output.txt" || { echo "EXPECTED_INVALID_2" >&2; exit 1; }
    grep -q "^UPLOADED=0" "$ROOT/output.txt" || { echo "EXPECTED_UPLOADED_0" >&2; exit 1; }
    grep -q "^FAILED=0" "$ROOT/output.txt" || { echo "EXPECTED_FAILED_0" >&2; exit 1; }
    # No offsite markers should exist for invalid backups
    if find "$POSTGRES_ROOT" -name "*.offsite.json" -print | grep -q .; then
      echo "UNEXPECTED_OFFSITE_MARKER_FOUND" >&2
      exit 1
    fi
    exit "$rc"
    ;;
  filename-safety)
    create_valid_backup "db_20260822_155244_safe space.sql.gz"
    run_batch
    rc=$?
    grep -q "^TOTAL=1" "$ROOT/output.txt" || { echo "EXPECTED_TOTAL_1" >&2; exit 1; }
    grep -q "^UPLOADED=1" "$ROOT/output.txt" || { echo "EXPECTED_UPLOADED_1" >&2; exit 1; }
    grep -q "^INVALID=0" "$ROOT/output.txt" || { echo "EXPECTED_INVALID_0" >&2; exit 1; }
    grep -q "^FAILED=0" "$ROOT/output.txt" || { echo "EXPECTED_FAILED_0" >&2; exit 1; }
    exit "$rc"
    ;;
  secret-leakage)
    create_valid_backup "db_20260822_155244_aaaa000001.sql.gz"
    run_batch
    rc=$?
    # Verify no credentials or env secrets appear in stdout
    if grep -E -q "OBS_SECRET_ACCESS_KEY|test-connector-secret-key" "$ROOT/output.txt"; then
      echo "SECRET_LEAKAGE_DETECTED" >&2
      exit 1
    fi
    exit "$rc"
    ;;
  *)
    printf 'unknown scenario: %s\n' "$SCENARIO" >&2
    exit 2
    ;;
esac
