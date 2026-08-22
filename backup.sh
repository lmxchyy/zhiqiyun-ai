#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
BACKUP_DIR="backups/postgres"

json_escape() {
  # Minimal JSON string escape without jq.
  # Order matters: backslash first, then quotes/control chars.
  local s=$1
  local out="" i c hex
  local -i i_len=${#s}
  for ((i = 0; i < i_len; i++)); do
    c=${s:i:1}
    case "$c" in
      \\) out+='\\' ;;
      \") out+='\"' ;;
      $'\b') out+='\b' ;;
      $'\f') out+='\f' ;;
      $'\n') out+='\n' ;;
      $'\r') out+='\r' ;;
      $'\t') out+='\t' ;;
      *)
        # Escape other ASCII controls as \u00XX; pass through UTF-8 as-is.
        printf -v ord '%d' "'$c"
        if (( ord >= 0 && ord < 32 )); then
          printf -v hex '%02x' "$ord"
          out+="\\u00${hex}"
        else
          out+="$c"
        fi
        ;;
    esac
  done
  printf '%s' "$out"
}

sha256_file() {
  local file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    echo "[backup] ERROR: neither sha256sum nor shasum is available." >&2
    return 1
  fi
}

validate_meta_json() {
  local meta=$1
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$meta" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, encoding="utf-8") as f:
    data = json.load(f)
required = [
    "git_sha", "short_sha", "branch", "hostname",
    "started_at", "completed_at", "backup_file", "bytes", "sha256",
]
missing = [k for k in required if k not in data]
if missing:
    raise SystemExit(f"missing keys: {missing}")
if not isinstance(data["bytes"], int):
    raise SystemExit("bytes must be int")
if not isinstance(data["sha256"], str) or len(data["sha256"]) != 64:
    raise SystemExit("sha256 must be 64-char hex string")
PY
    return
  fi
  # Fallback without python: basic structural checks.
  test -s "$meta"
  grep -q '"git_sha"' "$meta"
  grep -q '"short_sha"' "$meta"
  grep -q '"branch"' "$meta"
  grep -q '"hostname"' "$meta"
  grep -q '"backup_file"' "$meta"
  grep -q '"bytes"' "$meta"
  grep -q '"sha256"' "$meta"
}

PART_FILE=""
META_PART_FILE=""
BACKUP_FILE=""
META_FILE=""
BACKUP_COMMITTED=0

cleanup_incomplete() {
  local ec=$?
  if [ -n "${PART_FILE:-}" ] && [ -e "$PART_FILE" ]; then
    rm -f -- "$PART_FILE"
  fi
  if [ -n "${META_PART_FILE:-}" ] && [ -e "$META_PART_FILE" ]; then
    rm -f -- "$META_PART_FILE"
  fi
  # Never leave a final dump without its metadata sidecar.
  if [ "${BACKUP_COMMITTED:-0}" -ne 1 ]; then
    if [ -n "${BACKUP_FILE:-}" ] && [ -e "$BACKUP_FILE" ]; then
      rm -f -- "$BACKUP_FILE"
    fi
    if [ -n "${META_FILE:-}" ] && [ -e "$META_FILE" ]; then
      rm -f -- "$META_FILE"
    fi
  fi
  exit "$ec"
}
trap cleanup_incomplete EXIT

printf '[backup] Project directory: %s\n' "$SCRIPT_DIR"
printf '[backup] Compose file: %s\n' "$COMPOSE_FILE"
printf '[backup] Env file: %s\n' "$ENV_FILE"

if [ ! -f "$ENV_FILE" ]; then
  echo "[backup] ERROR: $ENV_FILE does not exist. PostgreSQL credentials must be provided by .env." >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "[backup] ERROR: $COMPOSE_FILE does not exist." >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "[backup] ERROR: git is required to tag backup identity." >&2
  exit 1
fi

if ! command -v gzip >/dev/null 2>&1; then
  echo "[backup] ERROR: gzip is required to compress backups." >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "[backup] ERROR: docker is not installed." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

if [ -z "${POSTGRES_DB:-}" ] || [ -z "${POSTGRES_USER:-}" ] || [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "[backup] ERROR: POSTGRES_DB, POSTGRES_USER, and POSTGRES_PASSWORD must be set in $ENV_FILE." >&2
  exit 1
fi

GIT_SHA="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short=10 HEAD)"
BRANCH="$(git branch --show-current 2>/dev/null || true)"
if [ -z "$BRANCH" ]; then
  BRANCH="HEAD/detached"
fi
# Optional override for tests / controlled metadata; unset in normal ops.
HOSTNAME_VALUE="${BACKUP_HOSTNAME:-$(hostname 2>/dev/null || printf 'unknown')}"
STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"

BACKUP_BASENAME="db_${TIMESTAMP}_${SHORT_SHA}.sql.gz"
BACKUP_FILE="${BACKUP_DIR}/${BACKUP_BASENAME}"
PART_FILE="${BACKUP_FILE}.part"
META_FILE="${BACKUP_FILE}.meta.json"
META_PART_FILE="${META_FILE}.part"

mkdir -p "$BACKUP_DIR"

echo "[backup] Creating PostgreSQL backup: $BACKUP_FILE"
echo "[backup] Git SHA: $GIT_SHA ($SHORT_SHA) branch=$BRANCH"

rm -f -- "$PART_FILE" "$META_PART_FILE" "$BACKUP_FILE" "$META_FILE"

docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  postgres pg_dump \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" \
  --clean --if-exists \
  | gzip -c >"$PART_FILE"

test -s "$PART_FILE"
gzip -t "$PART_FILE"

BYTES="$(wc -c <"$PART_FILE" | tr -d '[:space:]')"
SHA256="$(sha256_file "$PART_FILE")"
COMPLETED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Test-only fault injection: fail before metadata exists (no finals yet).
if [ "${BACKUP_TEST_INJECT_META_FAIL:-0}" = "1" ]; then
  echo "[backup] ERROR: injected metadata failure before meta write." >&2
  exit 1
fi

{
  printf '{\n'
  printf '  "git_sha": "%s",\n' "$(json_escape "$GIT_SHA")"
  printf '  "short_sha": "%s",\n' "$(json_escape "$SHORT_SHA")"
  printf '  "branch": "%s",\n' "$(json_escape "$BRANCH")"
  printf '  "hostname": "%s",\n' "$(json_escape "$HOSTNAME_VALUE")"
  printf '  "started_at": "%s",\n' "$(json_escape "$STARTED_AT")"
  printf '  "completed_at": "%s",\n' "$(json_escape "$COMPLETED_AT")"
  printf '  "backup_file": "%s",\n' "$(json_escape "$BACKUP_FILE")"
  printf '  "bytes": %s,\n' "$BYTES"
  printf '  "sha256": "%s"\n' "$(json_escape "$SHA256")"
  printf '}\n'
} >"$META_PART_FILE"

test -s "$META_PART_FILE"
validate_meta_json "$META_PART_FILE"

# Both artifacts validated as .part — promote only after that.
mv -- "$PART_FILE" "$BACKUP_FILE"
PART_FILE=""

# Test-only: fail after dump final exists but before meta final.
if [ "${BACKUP_TEST_INJECT_AFTER_GZ_MV:-0}" = "1" ]; then
  echo "[backup] ERROR: injected failure after dump rename (meta not committed)." >&2
  exit 1
fi

mv -- "$META_PART_FILE" "$META_FILE"
META_PART_FILE=""

# Final consistency gate: both must exist before declaring success.
test -f "$BACKUP_FILE"
test -f "$META_FILE"
test -s "$BACKUP_FILE"
gzip -t "$BACKUP_FILE"
validate_meta_json "$META_FILE"
BACKUP_COMMITTED=1

BACKUP_SIZE="$(ls -lh "$BACKUP_FILE" | awk '{print $5}')"
echo "[backup] Backup created: $BACKUP_FILE ($BACKUP_SIZE)"
echo "[backup] Metadata: $META_FILE"
echo "[backup] sha256: $SHA256"
