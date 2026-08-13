#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT error_message FROM video_plan_tasks WHERE id='svplan_d6377eb8ab9676d91ef36333';"
