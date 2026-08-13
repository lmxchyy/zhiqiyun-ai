#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai
if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi
bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
curl -fsS http://127.0.0.1:3100/api/v1/health; echo
docker inspect --format '{{.Name}} {{.State.Health.Status}}' zhiqiyun-ai-prod-xianzhi-ai-1
