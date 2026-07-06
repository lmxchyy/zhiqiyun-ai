#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env}"
BACKUP_DIR="backups/postgres"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"
BACKUP_FILE="${BACKUP_DIR}/db_${TIMESTAMP}.sql"

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

set -a
. "$ENV_FILE"
set +a

if [ -z "${POSTGRES_DB:-}" ] || [ -z "${POSTGRES_USER:-}" ] || [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "[backup] ERROR: POSTGRES_DB, POSTGRES_USER, and POSTGRES_PASSWORD must be set in $ENV_FILE." >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"

echo "[backup] Creating PostgreSQL backup: $BACKUP_FILE"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  postgres pg_dump \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" \
  --clean --if-exists > "$BACKUP_FILE"

BACKUP_SIZE="$(ls -lh "$BACKUP_FILE" | awk '{print $5}')"
echo "[backup] Backup created: $BACKUP_FILE ($BACKUP_SIZE)"
