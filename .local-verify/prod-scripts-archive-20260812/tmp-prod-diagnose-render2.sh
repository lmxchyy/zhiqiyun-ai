#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "\d video_render_tasks"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,progress,version_id,left(coalesce(error_message,''),220) AS err,created_at,finished_at FROM video_render_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 5;"
docker logs --since 20m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'render|speech|error|fail|SMARTVIDEO' | tail -50 || true
