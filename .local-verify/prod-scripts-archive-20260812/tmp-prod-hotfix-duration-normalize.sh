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
  "UPDATE video_projects SET status='MATERIAL_READY', active_plan_task_id=NULL, error_stage=NULL, error_code=NULL, error_message=NULL, updated_at=now() WHERE id='vp_664248192f84dc96631df8cd';"

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,left(coalesce(error_message,''),160) AS err,created_at,finished_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 3;"

curl -fsS http://127.0.0.1:3100/api/v1/health; echo
docker inspect --format '{{.Name}} {{.State.Health.Status}}' zhiqiyun-ai-prod-xianzhi-ai-1 zhiqiyun-ai-prod-smartvideo-worker-1
