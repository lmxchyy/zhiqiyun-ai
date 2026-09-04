#!/usr/bin/env bash
set -uo pipefail
IFS=$'\n\t'

PROJECT_DIR="${PROJECT_DIR:-/opt/zhiqiyun-ai}"
BACKUP_ROOT="${BACKUP_ROOT:-$PROJECT_DIR/backups/postgres}"
COMPOSE_FILE="${COMPOSE_FILE:-$PROJECT_DIR/compose.prod.yml}"
ENV_FILE="${ENV_FILE:-$PROJECT_DIR/.env.production}"
BACKUP_OBS_ENV_FILE="${BACKUP_OBS_ENV_FILE:-$PROJECT_DIR/secrets/backup-obs.env}"
BACKUP_UPLOADER_IMAGE="${BACKUP_UPLOADER_IMAGE:-}"

[[ -d "$BACKUP_ROOT" ]] || { echo "BACKUP_ROOT_NOT_FOUND" >&2; exit 1; }
[[ -f "$COMPOSE_FILE" && -f "$ENV_FILE" ]] || { echo "COMPOSE_OR_ENV_NOT_FOUND" >&2; exit 1; }
[[ -f "$BACKUP_OBS_ENV_FILE" ]] || { echo "BACKUP_SECRET_FILE_NOT_FOUND" >&2; exit 1; }
[[ -r "$BACKUP_OBS_ENV_FILE" ]] || { echo "BACKUP_SECRET_FILE_NOT_READABLE" >&2; exit 1; }
[[ "$BACKUP_UPLOADER_IMAGE" == *@sha256:* ]] || { echo "IMMUTABLE_BACKUP_UPLOADER_IMAGE_REQUIRED" >&2; exit 1; }

export BACKUP_OBS_ENV_FILE BACKUP_UPLOADER_IMAGE
found=0
invalid_count=0
failed_count=0
uploaded_count=0
skipped_already_count=0
while IFS= read -r -d '' file; do
  found=1
  name="$(basename "$file")"
  echo "Uploading backup: $name"
  # NOTE: --root must be the backups parent (not the postgres dir): the uploader
  # derives the object key from the path relative to root and requires the
  # `postgres/` prefix for db_ files. Pointing root at the postgres dir makes
  # every db_ file fail with LOCAL_BACKUP_INVALID/unsupported backup category.
  upload_output="$(docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" --profile backup-uploader \
    run --rm --no-deps backup-uploader \
    --root /var/lib/zhiqiyun/backups \
    --file "/var/lib/zhiqiyun/backups/postgres/$name" \
    --upload --json 2>&1)" || true
  # Strip carriage returns so pattern matching works reliably across platforms.
  upload_output="$(printf '%s' "$upload_output" | tr -d '\r')"
  if [[ "$upload_output" == *'"status":"OFFSITE_VERIFIED"'* || "$upload_output" == *'"status":"ALREADY_OFFSITE_VERIFIED"'* ]]; then
    uploaded_count=$((uploaded_count + 1))
  elif [[ "$upload_output" == *'"status":"LOCAL_BACKUP_INVALID"'* ]]; then
    invalid_count=$((invalid_count + 1))
    echo "status=LOCAL_BACKUP_INVALID"
    echo "reason_code=metadata_missing_or_unsafe"
    echo "backup_name=$name"
    echo "$upload_output"
  else
    failed_count=$((failed_count + 1))
    echo "status=OFFSITE_UPLOAD_FAILED"
    echo "reason_code=uploader_failed"
    echo "backup_name=$name"
    echo "$upload_output"
  fi
done < <(find "$BACKUP_ROOT" -maxdepth 1 -type f \( -name 'db_*.sql' -o -name 'db_*.sql.gz' -o -name 'xianzhi-*.sql' -o -name 'xianzhi-*.sql.gz' \) -print0 | sort -z)

if [[ "$found" -eq 0 ]]; then
  echo "NO_BACKUP_FILES_FOUND"
fi

printf 'TOTAL=%s\nUPLOADED=%s\nSKIPPED_ALREADY_VERIFIED=%s\nINVALID=%s\nFAILED=%s\n' \
  "$found" "$uploaded_count" "$skipped_already_count" "$invalid_count" "$failed_count"

# Return non-zero when any candidate was invalid or failed, but only after every
# candidate has been scanned.
if [[ "$found" -gt 0 && "$failed_count" -gt 0 ]]; then
  exit 1
fi
