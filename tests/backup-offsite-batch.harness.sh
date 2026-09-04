#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$REPO_ROOT/ops/backup-offsite-upload-pending.sh"
SCENARIO="${1:-first-invalid-second-valid}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/backup-offsite-batch.XXXXXX")"
POSTGRES_ROOT="$ROOT/backups/postgres"
FAKE_ROOT="$ROOT/fake-object-store"
mkdir -p "$POSTGRES_ROOT" "$FAKE_ROOT"

MOCK_BIN="$ROOT/bin"
mkdir -p "$MOCK_BIN"
cat > "$MOCK_BIN/docker" <<'DOCKEREOF'
#!/usr/bin/env bash
if [[ "$1" == "compose" ]]; then
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
  BACKUP_OBS_FAKE=1 \
  BACKUP_OBS_BUCKET=fake-bucket \
  BACKUP_OBS_ENDPOINT=https://obs.example.invalid \
  BACKUP_OBS_REGION=cn-north-4 \
  BACKUP_OBJECT_FAKE_ROOT="$MOCK_FAKE_ROOT" \
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

export MOCK_HOST_ROOT="$POSTGRES_ROOT"
export MOCK_FAKE_ROOT="$FAKE_ROOT"
export MOCK_REPO_ROOT="$REPO_ROOT"
export PATH="$MOCK_BIN:$PATH"

set_file() {
  FILE="$POSTGRES_ROOT/$1"
  META="$FILE.meta.json"
}

write_meta() {
  local bytes sha
  bytes="$(wc -c <"$FILE" | tr -d '[:space:]')"
  sha="$(sha256sum "$FILE" | awk '{print $1}')"
  printf '{\n  "git_sha": "test-git-sha",\n  "short_sha": "test-git",\n  "branch": "main",\n  "hostname": "fixture",\n  "started_at": "2026-08-22T15:52:44+08:00",\n  "completed_at": "2026-08-22T15:52:45+08:00",\n  "backup_file": "%s",\n  "bytes": %s,\n  "sha256": "%s"\n}\n' "$FILE" "$bytes" "$sha" >"$META"
}

make_valid() {
  set_file "db_20260822_155244_abcdef1234.sql.gz"
  printf 'fixture sql payload\n' | gzip -c >"$FILE"
  write_meta
  touch -t 202608221552.44 "$FILE" "$META"
}

make_invalid_no_meta() {
  set_file "db_20260821_195734.sql"
  printf 'fixture sql payload without meta\n' >"$FILE"
  touch -t 202608211957.34 "$FILE"
}

make_second_valid() {
  set_file "db_20260823_160000_ffffff0000.sql.gz"
  printf 'second valid sql payload\n' | gzip -c >"$FILE"
  write_meta
  touch -t 202608231600.00 "$FILE" "$META"
}

case "$SCENARIO" in
  first-invalid-second-valid)
    make_invalid_no_meta
    make_second_valid
    BACKUP_ROOT="$POSTGRES_ROOT" \
    COMPOSE_FILE="$REPO_ROOT/compose.prod.yml" \
    ENV_FILE="$REPO_ROOT/.env.production.example" \
    BACKUP_OBS_ENV_FILE="$REPO_ROOT/ops/backup-uploader/backup-obs.env.example" \
    BACKUP_UPLOADER_IMAGE='xianzhi-ai-platform:test@sha256:0000000000000000000000000000000000000000000000000000000000000000' \
    "$SCRIPT" 2>&1 || true
    ;;
  all-invalid)
    make_invalid_no_meta
    BACKUP_ROOT="$POSTGRES_ROOT" \
    COMPOSE_FILE="$REPO_ROOT/compose.prod.yml" \
    ENV_FILE="$REPO_ROOT/.env.production.example" \
    BACKUP_OBS_ENV_FILE="$REPO_ROOT/ops/backup-uploader/backup-obs.env.example" \
    BACKUP_UPLOADER_IMAGE='xianzhi-ai-platform:test@sha256:0000000000000000000000000000000000000000000000000000000000000000' \
    "$SCRIPT" 2>&1 || true
    # Verify no offsite markers were created for invalid backup
    ! find "$POSTGRES_ROOT" -name "*.offsite.json" -print | grep -q .
    ;;
  *)
    printf 'unknown scenario: %s\n' "$SCENARIO" >&2
    exit 2
    ;;
esac
