#!/usr/bin/env bash
set -euo pipefail
echo "=== outbox ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,task_type,task_id,state,attempt_count,left(coalesce(last_error,''),160) AS err,created_at,updated_at FROM video_task_outbox WHERE task_id='svrender_044a0e2b4a72975352db68a7' OR payload::text LIKE '%svrender_044a0e2b4a72975352db68a7%' ORDER BY created_at DESC LIMIT 10;"

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT state, task_type, count(*) FROM video_task_outbox GROUP BY 1,2 ORDER BY 1,2;"

echo "=== render task detail ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,attempt_count,run_after,lease_owner,lease_expires_at,error_code,error_message FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"

echo "=== redis keys sample ==="
docker exec zhiqiyun-ai-prod-redis-1 redis-cli KEYS '*smart*' | head -50
docker exec zhiqiyun-ai-prod-redis-1 redis-cli KEYS '*render*' | head -50
docker exec zhiqiyun-ai-prod-redis-1 redis-cli KEYS '*video*' | head -50
