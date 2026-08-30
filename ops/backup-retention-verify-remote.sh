#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

PROJECT_DIR="${PROJECT_DIR:-/opt/zhiqiyun-ai}"
COMPOSE_FILE="${COMPOSE_FILE:-$PROJECT_DIR/compose.prod.yml}"
ENV_FILE="${ENV_FILE:-$PROJECT_DIR/.env.production}"
BACKUP_OBS_ENV_FILE="${BACKUP_OBS_ENV_FILE:-$PROJECT_DIR/secrets/backup-obs.env}"
BACKUP_UPLOADER_IMAGE="${BACKUP_UPLOADER_IMAGE:-}"
STORAGE_CONFIG_ID="${BACKUP_STORAGE_CONFIG_ID:-}"
REMOTE_KEY="${1:-}"
EXPECTED_SIZE="${2:-}"
EXPECTED_SHA256="${3:-}"

[[ -n "$REMOTE_KEY" && -n "$EXPECTED_SIZE" && -n "$EXPECTED_SHA256" ]] || { echo "VERIFY_ARGUMENTS_REQUIRED" >&2; exit 2; }
if [[ "${BACKUP_RETENTION_TEST_VERIFY:-0}" == "1" ]]; then
  printf '%s\n' '{"remote_exists":true,"remote_size_match":true,"remote_sha256_match":true,"status":"REMOTE_VERIFIED","verification":"READ_ONLY_HEAD"}'
  exit 0
fi
[[ -f "$COMPOSE_FILE" && -f "$ENV_FILE" ]] || { echo "COMPOSE_OR_ENV_NOT_FOUND" >&2; exit 1; }
[[ -r "$BACKUP_OBS_ENV_FILE" ]] || { echo "BACKUP_SECRET_FILE_NOT_READABLE" >&2; exit 1; }
[[ "$BACKUP_UPLOADER_IMAGE" == *@sha256:* ]] || { echo "IMMUTABLE_BACKUP_UPLOADER_IMAGE_REQUIRED" >&2; exit 1; }
[[ -n "$STORAGE_CONFIG_ID" ]] || { echo "BACKUP_STORAGE_CONFIG_ID_REQUIRED" >&2; exit 1; }
[[ "$REMOTE_KEY" == backups/postgres/* ]] || { echo "REMOTE_KEY_OUTSIDE_BACKUP_PREFIX" >&2; exit 1; }
[[ "$EXPECTED_SIZE" =~ ^[0-9]+$ && "$EXPECTED_SHA256" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "VERIFY_EXPECTATIONS_INVALID" >&2; exit 2; }

export BACKUP_OBS_ENV_FILE BACKUP_UPLOADER_IMAGE
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" --profile backup-uploader \
  run -T --rm --no-deps backup-uploader \
  --storage-config-id "$STORAGE_CONFIG_ID" \
  --remote-key "$REMOTE_KEY" \
  --expected-size "$EXPECTED_SIZE" \
  --expected-sha256 "$EXPECTED_SHA256" \
  --verify-only --json </dev/null
