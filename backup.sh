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
  local s=$1
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\t'/\\t}
  s=${s//$'\r'/\\r}
  s=${s//$'\n'/\\n}
  printf '%s' "$s"
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

PART_FILE=""
cleanup_part() {
  if [ -n "${PART_FILE:-}" ] && [ -e "$PART_FILE" ]; then
    rm -f -- "$PART_FILE"
  fi
}
trap cleanup_part EXIT

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
HOSTNAME_VALUE="$(hostname 2>/dev/null || printf 'unknown')"
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

rm -f -- "$PART_FILE" "$META_PART_FILE"

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

mv -- "$PART_FILE" "$BACKUP_FILE"
PART_FILE=""
mv -- "$META_PART_FILE" "$META_FILE"

BACKUP_SIZE="$(ls -lh "$BACKUP_FILE" | awk '{print $5}')"
echo "[backup] Backup created: $BACKUP_FILE ($BACKUP_SIZE)"
echo "[backup] Metadata: $META_FILE"
echo "[backup] sha256: $SHA256"
