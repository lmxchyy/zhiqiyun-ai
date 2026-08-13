#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi

bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"

# Ensure disk headroom for health marker
df -h / | tail -1

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "UPDATE video_render_tasks
     SET status='QUEUED', stage='queued', step='queued', progress=5,
         attempt_count=0, attempt=0,
         lease_owner=NULL, lease_expires_at=NULL, heartbeat_at=NULL,
         error_code=NULL, error_message=NULL, finished_at=NULL,
         run_after=now(), updated_at=now()
   WHERE id='svrender_044a0e2b4a72975352db68a7';
   UPDATE video_projects
     SET status='RENDERING', active_render_task_id='svrender_044a0e2b4a72975352db68a7',
         error_code=NULL, error_message=NULL, updated_at=now()
   WHERE id=(SELECT project_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7');
   UPDATE video_task_outbox
     SET state='pending', attempts=0, available_at=now(), published_at=NULL, last_error=NULL
   WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' AND event_type='enqueue_requested';"

sleep 25
echo "--- TASK ---"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,attempt_count,error_code,left(coalesce(error_message,''),160) AS err,updated_at FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- LOGS ---"
docker logs --since 2m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'smartvideo_render|speech|ffmpeg|acquired|advance|skip|failed|complete' | tail -50 || true
docker inspect --format '{{.Name}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' zhiqiyun-ai-prod-smartvideo-worker-1
