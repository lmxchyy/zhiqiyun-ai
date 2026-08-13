#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c '\d video_task_outbox'
echo "=== outbox for render ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,aggregate_type,aggregate_id,event_type,state,attempts,left(coalesce(last_error,''),200) AS err,created_at,updated_at FROM video_task_outbox WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' OR aggregate_type='render' ORDER BY created_at DESC LIMIT 20;"
echo "=== outbox by state ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT state, aggregate_type, count(*) FROM video_task_outbox GROUP BY 1,2 ORDER BY 1,2;"
echo "=== redis ==="
docker exec zhiqiyun-ai-prod-redis-1 redis-cli KEYS 'xianzhi:smartvideo:*' | head -80
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LLEN xianzhi:smartvideo:render:pending || true
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LRANGE xianzhi:smartvideo:render:pending 0 20 || true
