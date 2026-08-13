#!/usr/bin/env bash
set -euo pipefail

echo "=== render task ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,attempt,attempt_count,lease_owner,lease_expires_at,error_code,left(coalesce(error_message,''),220) AS err,started_at,finished_at,updated_at FROM video_render_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 3;"

echo "=== project ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,active_render_task_id,confirmed_version_id,updated_at FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"

echo "=== worker logs 15m ==="
docker logs --since 15m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'render|speech|ffmpeg|error|fail|panic|svrender_|SMARTVIDEO' | tail -80 || true

echo "=== metrics ==="
docker logs --since 10m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep smartvideo_metrics | tail -15 || true

echo "=== speech env ==="
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv SMARTVIDEO_SPEECH_BASE_URL || true
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv SMARTVIDEO_SPEECH_MODEL || true
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv speech_provider 2>/dev/null || true
