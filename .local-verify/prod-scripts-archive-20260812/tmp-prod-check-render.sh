#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
WK=zhiqiyun-ai-prod-smartvideo-worker-1
sleep 20
echo "--- TASK ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,attempt_count,lease_owner,error_code,left(coalesce(error_message,''),160) AS err,updated_at FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- OUTBOX ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,event_type,state,attempts,left(coalesce(last_error,''),120) AS err,available_at,published_at FROM video_task_outbox WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' ORDER BY id DESC LIMIT 5;"
echo "--- LOGS ---"
docker logs --since=3m "$WK" 2>&1 | grep -iE 'smartvideo_render|speech|ffmpeg|requeue|acquired|advance|failed|error|inconsistent' | tail -60 || true
echo "--- HEALTH ---"
docker inspect --format '{{.Name}} {{.State.Status}} {{.State.Health.Status}}' "$WK" || true
echo "CHECK_DONE"
