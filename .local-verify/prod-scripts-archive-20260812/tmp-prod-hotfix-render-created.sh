#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi

bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"

# Re-dispatch the stuck render task that was dropped while status stayed CREATED.
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "UPDATE video_render_tasks SET status='CREATED', stage='created', step='created', progress=0, lease_owner=NULL, lease_expires_at=NULL, error_code=NULL, error_message=NULL, run_after=now(), updated_at=now() WHERE id='svrender_044a0e2b4a72975352db68a7' AND status IN ('CREATED','QUEUED','FAILED');
   UPDATE video_task_outbox SET state='pending', attempts=0, available_at=now(), published_at=NULL, last_error=NULL WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' AND event_type='enqueue_requested';"

sleep 8
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,error_code,left(coalesce(error_message,''),160) AS err FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,attempts,published_at,left(coalesce(last_error,''),120) AS err FROM video_task_outbox WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7';"

curl -fsS http://127.0.0.1:3100/api/v1/health; echo
docker inspect --format '{{.Name}} {{.State.Health.Status}}' zhiqiyun-ai-prod-xianzhi-ai-1 zhiqiyun-ai-prod-smartvideo-worker-1
