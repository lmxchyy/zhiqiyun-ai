#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== project ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,error_stage,error_code,left(coalesce(error_message,''),300) AS err,updated_at FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"

echo "=== latest plan tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,error_message,created_at,finished_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 3;"

echo "=== assets ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,asset_type,analysis_status,duration_ms FROM video_project_assets WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY sort_order;"
