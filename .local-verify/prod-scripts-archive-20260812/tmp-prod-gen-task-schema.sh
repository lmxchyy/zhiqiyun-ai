#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
echo "--- generation_tasks columns ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT column_name, data_type FROM information_schema.columns WHERE table_name='xz_generation_tasks' ORDER BY ordinal_position;"
echo "--- sample video task ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,user_id,type,status,name,point_cost,result_ids,left(coalesce(output_url,''),80),created_at FROM xz_generation_tasks WHERE user_id='user_000003' ORDER BY created_at DESC LIMIT 3;" 2>/dev/null \
|| docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,user_id,type,status,left(coalesce(name,''),40),created_at FROM xz_generation_tasks WHERE user_id='user_000003' ORDER BY created_at DESC LIMIT 3;"
echo DONE
