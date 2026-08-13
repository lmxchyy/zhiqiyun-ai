#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai
TS=$(date +%Y-%m-%d_%H%M%S)
mkdir -p "backups/dirty-tree"
git status -sb > "backups/dirty-tree/status.${TS}.txt"
git diff > "backups/dirty-tree/diff.${TS}.patch" || true
cp -a backup.sh deploy.sh rollback.sh "backups/dirty-tree/"
echo "BACKUP_DIRTY_OK=${TS}"
git checkout -- backup.sh deploy.sh rollback.sh
git status -sb
echo "START_DEPLOY"
GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
