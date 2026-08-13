#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,aggregate_type,aggregate_id,event_type,state,attempts,left(coalesce(last_error,''),200) AS err,available_at,published_at,created_at FROM video_task_outbox WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' OR aggregate_type='render' ORDER BY created_at DESC LIMIT 20;"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT state, aggregate_type, count(*) FROM video_task_outbox GROUP BY 1,2 ORDER BY 1,2;"
docker exec zhiqiyun-ai-prod-redis-1 redis-cli KEYS 'xianzhi:smartvideo:*'
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LLEN xianzhi:smartvideo:render:pending
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LLEN xianzhi:smartvideo:render:working
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LLEN xianzhi:smartvideo:render:dead
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LRANGE xianzhi:smartvideo:render:pending 0 20
docker exec zhiqiyun-ai-prod-redis-1 redis-cli LRANGE xianzhi:smartvideo:render:dead 0 20
