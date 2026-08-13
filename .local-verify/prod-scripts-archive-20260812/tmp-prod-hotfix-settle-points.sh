#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi

# free a bit of space if needed
df -h / | tail -1
docker builder prune -af >/dev/null 2>&1 || true

bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"

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
         error_code=NULL, error_message=NULL, error_stage=NULL, updated_at=now()
   WHERE id=(SELECT project_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7');
   UPDATE video_task_outbox
     SET state='pending', attempts=0, available_at=now(), published_at=NULL, last_error=NULL
   WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' AND event_type='enqueue_requested';"

# poll up to ~4 minutes
for i in $(seq 1 16); do
  row=$(docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -t -A -F'|' -c \
    "SELECT status,stage,progress,coalesce(work_id,''),coalesce(error_code,''),left(coalesce(error_message,''),120) FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
  echo "[$i] $row"
  status=${row%%|*}
  if [ "$status" = "SUCCEEDED" ] || [ "$status" = "FAILED" ]; then
    break
  fi
  sleep 15
done

echo "--- LOGS ---"
docker logs --since 5m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'smartvideo_render|speech|settle|publish|capture|complete|failed|error' | tail -60 || true
echo "FINAL_DONE"
