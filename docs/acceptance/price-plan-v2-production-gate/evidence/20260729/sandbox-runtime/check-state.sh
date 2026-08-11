#!/bin/bash
set -euo pipefail
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
PG_C=zhiqiyun-ai-prod-postgres-1
echo "=== FLAGS ==="
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV
echo "=== HEALTH ==="
docker inspect --format 'health={{.State.Health.Status}}' "$APP_C"
echo "=== ENV FILE ==="
grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' /opt/zhiqiyun-ai/.env.production
echo "=== SANDBOX EVID ==="
ls -la /tmp/sandbox-runtime-20260729 2>/dev/null || echo none
ls -la /tmp/sandbox-v2-seed-evidence 2>/dev/null | sed -n '1,40p' || echo no-seed
ls -la /tmp/normal-996-20260729 2>/dev/null || echo no-normal
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
echo "=== SANDBOX ROWS ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "SELECT environment, count(*) FROM xz_price_plans GROUP BY 1 ORDER BY 1;"
docker exec "$PG_C" psql -U "$u" -d "$db" -c "SELECT environment, count(*) FROM xz_wechat_virtual_goods GROUP BY 1 ORDER BY 1;"
docker exec "$PG_C" psql -U "$u" -d "$db" -c "SELECT environment, count(*) FROM xz_price_plan_payment_bindings GROUP BY 1 ORDER BY 1;"
echo DONE
