#!/usr/bin/env bash
set -euo pipefail

echo "=== render tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,version_id,left(coalesce(error_message,''),200) AS err,created_at,finished_at FROM video_render_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 5;"

echo "=== worker metrics/logs ==="
docker logs --since 15m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'render|speech|error|fail|plan_provider|smartvideo_metrics' | tail -40 || true
