#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== APPLY 106/107 ==="
ls -la database/migrations/106-ai-auto-montage-v1.sql database/migrations/107-storage-multipart-upload.sql

docker compose -f compose.prod.yml --env-file .env.production rm -f migrate >/dev/null 2>&1 || true

MIGRATION_FILES="106-ai-auto-montage-v1.sql 107-storage-multipart-upload.sql" \
docker compose -f compose.prod.yml --env-file .env.production run --rm \
  -e MIGRATION_FILES="106-ai-auto-montage-v1.sql 107-storage-multipart-upload.sql" \
  migrate

echo "=== TABLES AFTER ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -Atc \
  "SELECT tablename FROM pg_tables WHERE tablename IN ('video_task_outbox','xz_multipart_uploads','xz_multipart_upload_parts','video_plan_tasks','video_project_versions','video_render_tasks') ORDER BY 1;"

echo "=== RESTART WORKER ==="
docker compose -f compose.prod.yml --env-file .env.production up -d --no-deps smartvideo-worker xianzhi-ai
sleep 8
docker compose -f compose.prod.yml --env-file .env.production ps smartvideo-worker xianzhi-ai
docker compose -f compose.prod.yml --env-file .env.production logs --tail=40 smartvideo-worker

echo "=== HEALTH ==="
curl -fsS http://127.0.0.1:3100/api/v1/health
echo
echo "MIGRATE_DONE=$(git rev-parse --short HEAD)"
