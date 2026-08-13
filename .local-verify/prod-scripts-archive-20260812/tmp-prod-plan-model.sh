#!/usr/bin/env bash
set -euo pipefail
echo "=== chat models ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, left(raw::text,200) FROM xz_system_settings WHERE id LIKE '%ai%' OR id LIKE '%capability%' OR id LIKE '%model%' LIMIT 20;" 2>/dev/null || true
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 printenv | grep -iE 'SMARTVIDEO_PLAN|CHAT_|OPENAI|LLM|MODEL' | sed 's/\(KEY\|SECRET\|TOKEN\)=.*/\1=***/' | head -40
echo "=== plan tasks table ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT column_name FROM information_schema.columns WHERE table_name='video_plan_tasks' ORDER BY ordinal_position;"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, project_id, state, model_key, left(coalesce(error_code,''),40) code, left(coalesce(error_message,''),160) msg, created_at FROM video_plan_tasks ORDER BY created_at DESC LIMIT 10;"
