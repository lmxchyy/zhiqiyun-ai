#!/usr/bin/env bash
set -uo pipefail
cd /opt/zhiqiyun-ai
echo "=== commit ==="
git rev-parse --short HEAD
echo "=== recent logs ==="
docker logs --since 3h zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -E 'permission|not allowed|preview|403|FORBIDDEN|grok-imagine|PrepareVideo|generation task|FailGeneration|video provider|NewAPI' | tail -120 || true
echo "=== tables ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "\dt xz_*" | grep -Ei 'generation|system|limit|model|ai_|channel' || true
echo "=== recent video tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<'SQL'
SELECT column_name FROM information_schema.columns
WHERE table_name='xz_generation_tasks' ORDER BY ordinal_position;
SQL
