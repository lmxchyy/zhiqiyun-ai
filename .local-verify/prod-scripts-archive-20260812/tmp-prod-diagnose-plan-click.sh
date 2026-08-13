#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== project ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,active_plan_task_id,error_stage,error_code,left(coalesce(error_message,''),180) AS err,updated_at FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"

echo "=== recent plan tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,left(coalesce(error_message,''),180) AS err,created_at,finished_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 8;"

echo "=== assets analysis ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT analysis_status, count(*) FROM video_project_assets WHERE project_id='vp_664248192f84dc96631df8cd' GROUP BY 1;"

echo "=== api logs (5m) ==="
docker logs --since 5m zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -iE 'plan|smart-video|video-projects|422|400|500' | tail -40 || true

echo "=== worker logs (5m) ==="
docker logs --since 5m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'plan|timeout|error|fail|provider|chat' | tail -40 || true

echo "=== worker metrics tail ==="
docker logs --since 5m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep smartvideo_metrics | tail -10 || true
