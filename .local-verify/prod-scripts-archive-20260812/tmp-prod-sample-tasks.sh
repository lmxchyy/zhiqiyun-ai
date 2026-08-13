#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,user_id,type,status,model,point_cost,result_ids,left(coalesce(prompt,''),40),created_at FROM xz_generation_tasks WHERE user_id='user_000003' ORDER BY created_at DESC LIMIT 3;"
