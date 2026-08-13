#!/usr/bin/env bash
set -euo pipefail
# Check if worker process still holds anything; inspect full recent worker stderr
docker logs --since 20m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | tail -100
echo "===="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT now() AS db_now, id,status,stage,progress,lease_owner,lease_expires_at,heartbeat_at,attempt,attempt_count FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
# Check redis via worker env
REDIS_URL=$(grep '^REDIS_URL=' /opt/zhiqiyun-ai/.env.production | head -1 | cut -d= -f2-)
python3 - <<'PY' "$REDIS_URL" 2>/dev/null || true
import os,sys
try:
  import redis
except Exception as e:
  print('no redis py', e)
  sys.exit(0)
url=sys.argv[1]
r=redis.Redis.from_url(url, decode_responses=True)
for k in ['xianzhi:smartvideo:render:pending','xianzhi:smartvideo:render:working','xianzhi:smartvideo:render:dead']:
  try:
    print(k, r.llen(k), r.lrange(k,0,5))
  except Exception as e:
    print(k, e)
try:
  print('delayed', r.zcard('xianzhi:smartvideo:render:delayed'), r.zrange('xianzhi:smartvideo:render:delayed',0,5,withscores=True))
except Exception as e:
  print('delayed', e)
PY
