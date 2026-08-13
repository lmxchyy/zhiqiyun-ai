#!/usr/bin/env bash
set -euo pipefail
echo "=== API TAIL ==="
docker logs --since 3h zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | tail -250
echo "=== PROJECTS ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, title, status, active_analysis_task_id, error_stage, updated_at FROM video_projects ORDER BY updated_at DESC LIMIT 8;"
echo "=== ASSETS ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, project_id, kind, content_audit_status, file_id, created_at FROM video_project_assets ORDER BY created_at DESC LIMIT 20;"
echo "=== OUTBOX ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, aggregate_type, event_type, state, last_error, created_at FROM video_task_outbox ORDER BY created_at DESC LIMIT 15;"
echo "=== COLUMNS assets ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "\d video_project_assets"
