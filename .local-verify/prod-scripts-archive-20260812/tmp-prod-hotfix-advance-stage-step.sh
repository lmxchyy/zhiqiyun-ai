#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi

bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "UPDATE video_render_tasks
     SET status='QUEUED', stage='queued', step='queued', progress=5,
         lease_owner=NULL, lease_expires_at=NULL, heartbeat_at=NULL,
         error_code=NULL, error_message=NULL, run_after=now(), updated_at=now()
   WHERE id='svrender_044a0e2b4a72975352db68a7';
   UPDATE video_task_outbox
     SET state='pending', attempts=0, available_at=now(), published_at=NULL, last_error=NULL
   WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' AND event_type='enqueue_requested';"

sleep 15
echo "--- TASK AFTER RESET ---"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,lease_owner,error_code,left(coalesce(error_message,''),180) AS err,updated_at FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- WORKER LOGS ---"
docker logs --since 2m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'smartvideo_render|speech|ffmpeg|requeue|acquired|advance' | tail -40 || true

curl -fsS http://127.0.0.1:3100/api/v1/health; echo
docker inspect --format '{{.Name}} {{.State.Health.Status}}' zhiqiyun-ai-prod-xianzhi-ai-1 zhiqiyun-ai-prod-smartvideo-worker-1
