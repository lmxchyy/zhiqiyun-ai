#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

if grep -q '^SMARTVIDEO_PLAN_TIMEOUT=' .env.production; then
  sed -i 's/^SMARTVIDEO_PLAN_TIMEOUT=.*/SMARTVIDEO_PLAN_TIMEOUT=180s/' .env.production
else
  echo 'SMARTVIDEO_PLAN_TIMEOUT=180s' >> .env.production
fi
if grep -q '^SMARTVIDEO_PLAN_MODEL=' .env.production; then
  sed -i 's/^SMARTVIDEO_PLAN_MODEL=.*/SMARTVIDEO_PLAN_MODEL=deepseek-v4-flash/' .env.production
else
  echo 'SMARTVIDEO_PLAN_MODEL=deepseek-v4-flash' >> .env.production
fi
grep -E '^(SMARTVIDEO_PLAN_|MODEL_PROVIDER_TIMEOUT|PPT_MODEL_CHAT)' .env.production | sed -E 's/(KEY|TOKEN|SECRET|PASSWORD)=.*/\1=***/'

if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi

bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"

docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv MODEL_PROVIDER_TIMEOUT_MS
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv SMARTVIDEO_PLAN_TIMEOUT
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv PPT_MODEL_CHAT_DISABLE_THINKING
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv SMARTVIDEO_PLAN_MODEL

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "UPDATE video_projects SET status='MATERIAL_READY', active_plan_task_id=NULL, error_stage=NULL, error_code=NULL, error_message=NULL, updated_at=now() WHERE id='vp_664248192f84dc96631df8cd';"

curl -fsS http://127.0.0.1:3100/api/v1/health; echo
docker inspect --format '{{.Name}} {{.State.Health.Status}}' zhiqiyun-ai-prod-xianzhi-ai-1 zhiqiyun-ai-prod-smartvideo-worker-1
