#!/usr/bin/env bash
set -euo pipefail
echo "=== PROJECT ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,title,status,requirement,active_analysis_task_id,active_plan_task_id,error_stage,error_code,error_message,updated_at FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"
echo "=== ASSETS ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,kind,analysis_status,attempt_count,error_code,left(coalesce(sanitized_error_message,''),80) err,updated_at FROM video_project_assets WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY order_index;"
echo "=== ANALYSIS TASKS ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,asset_id,status,attempt_count,left(coalesce(last_error,''),120) err,created_at,updated_at FROM video_asset_analysis_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 20;" 2>/dev/null || \
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "\dt *analysis*"
echo "=== OUTBOX ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,aggregate_type,event_type,state,attempts,left(coalesce(last_error,''),160) err,created_at FROM video_task_outbox ORDER BY created_at DESC LIMIT 20;"
echo "=== PLAN TASKS ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,left(coalesce(error_message,''),120) err,created_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 10;" 2>/dev/null || echo no_plan_tasks
echo "=== WORKER ==="
docker logs --since 30m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | tail -80
echo "=== API HEAD ==="
docker logs --since 30m zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -iE 'smart|video-project|plan|analysis|invalid|panic|ERROR|500|400' | tail -60 || true
