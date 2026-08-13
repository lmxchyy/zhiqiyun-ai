#!/bin/bash
set -euo pipefail
cd /opt/zhiqiyun-ai
# Prefer gitee for production pull
if git remote get-url gitee >/dev/null 2>&1; then
  export GIT_REMOTE=gitee
elif git remote get-url origin >/dev/null 2>&1; then
  # if origin is gitee use it
  :
fi
# Show remotes
git remote -v
export GIT_BRANCH=codex/channel-ecosystem-v132-phase3
# Discard any local dirty tracked files that would block deploy
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "WARNING: dirty tree, resetting tracked files to HEAD before deploy"
  git checkout -- .
  git clean -fd --exclude=.env.production --exclude=backups --exclude=data 2>/dev/null || true
fi
bash deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
# Verify new bundle
curl -s http://127.0.0.1:3100/ | tr '"' '\n' | grep -E 'assets/index-' | head -5
JS=$(curl -s http://127.0.0.1:3100/ | tr '"' '\n' | grep -E 'assets/index-.*\.js$' | head -1)
echo "JS=$JS"
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 sh -c "grep -o 'openWorksMine\|pendingWorksSourceTab\|isVideoWorkAsset\|SMART_VIDEO_MONTAGE' /app/admin-vue/dist/${JS#/admin/} /app/admin-vue/dist/assets/*.js 2>/dev/null | sort | uniq -c | head -20"
