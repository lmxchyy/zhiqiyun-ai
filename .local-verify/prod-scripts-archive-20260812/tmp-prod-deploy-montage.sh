#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== PRE ==="
git rev-parse --abbrev-ref HEAD
git rev-parse --short HEAD
git status -sb

if ! git diff --quiet || ! git diff --cached --quiet; then
  TS=$(date +%Y-%m-%d_%H%M%S)
  mkdir -p "backups/dirty-tree"
  git status -sb > "backups/dirty-tree/status.${TS}.txt"
  git diff > "backups/dirty-tree/diff.${TS}.patch" || true
  git checkout -- .
  echo "DIRTY_CLEARED=${TS}"
fi

echo "=== BACKUP ==="
bash ./backup.sh

echo "=== DEPLOY ==="
GIT_REMOTE=origin \
GIT_BRANCH=codex/channel-ecosystem-v132-phase3 \
bash ./deploy.sh

echo "=== POST HEAD ==="
git rev-parse --short HEAD
git log -1 --oneline

echo "=== SERVICES ==="
docker compose -f compose.prod.yml --env-file .env.production ps

echo "=== WORKER ==="
docker compose -f compose.prod.yml --env-file .env.production ps smartvideo-worker || true
docker compose -f compose.prod.yml --env-file .env.production logs --tail=80 smartvideo-worker || true

echo "=== MIGRATION TABLES ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -Atc \
  "SELECT tablename FROM pg_tables WHERE tablename IN ('video_task_outbox','xz_multipart_uploads','video_project_versions','video_plan_tasks','video_render_tasks') ORDER BY 1;"

echo "=== HEALTH ==="
curl -fsS http://127.0.0.1:3100/api/v1/health || curl -fsS https://127.0.0.1/api/v1/health || true
echo
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
