#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai
if ! git diff --quiet || ! git diff --cached --quiet; then
  TS=$(date +%Y-%m-%d_%H%M%S)
  mkdir -p "backups/dirty-tree"
  git status -sb > "backups/dirty-tree/status.${TS}.txt"
  git diff > "backups/dirty-tree/diff.${TS}.patch" || true
  git checkout -- .
fi
bash ./backup.sh
GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
curl -fsS http://127.0.0.1:3100/api/v1/health
echo
