#!/bin/bash
set -euo pipefail
cd /opt/zhiqiyun-ai
export GIT_BRANCH=codex/channel-ecosystem-v132-phase3
bash deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
curl -s http://127.0.0.1:3100/ | tr '"' '\n' | grep -E 'assets/index-.*\.js$' | head -3
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 sh -c "grep -o 'openWorksMine\|pendingWorksSourceTab\|isVideoWorkAsset' /app/admin-vue/dist/assets/index-*.js | sort | uniq -c"
