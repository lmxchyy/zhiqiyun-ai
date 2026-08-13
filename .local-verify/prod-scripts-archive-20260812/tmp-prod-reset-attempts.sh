#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
WK=zhiqiyun-ai-prod-smartvideo-worker-1
echo "--- TASK DETAIL ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,progress,attempt,attempt_count,max_attempts,run_after,lease_owner,lease_expires_at,now() AS now FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- RESET WITH ATTEMPTS ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "UPDATE video_render_tasks
     SET status='QUEUED', stage='queued', step='queued', progress=5,
         attempt_count=0, attempt=0,
         lease_owner=NULL, lease_expires_at=NULL, heartbeat_at=NULL,
         error_code=NULL, error_message=NULL, run_after=now(), updated_at=now()
   WHERE id='svrender_044a0e2b4a72975352db68a7';
   UPDATE video_task_outbox
     SET state='pending', attempts=0, available_at=now(), published_at=NULL, last_error=NULL
   WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' AND event_type='enqueue_requested';"
sleep 25
echo "--- TASK AFTER ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,attempt_count,max_attempts,lease_owner,error_code,left(coalesce(error_message,''),160) AS err,updated_at FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- LOGS ---"
docker logs --since 1m "$WK" 2>&1 | grep -iE 'smartvideo_render|speech|acquired|advance|failed|error|inconsistent|ffmpeg' | tail -50 || true
echo "DONE"
