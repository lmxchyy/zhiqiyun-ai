#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
echo "db=$db user=$u"
echo "=== FLAGS ==="
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV
echo "=== DEMO USER ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, email, name, member_level, agent_status,
       left(coalesce(subscription_expires_at::text,''), 32) AS expires,
       status
FROM xz_users
WHERE id = 'user_000002' OR email ILIKE '%demo@xianzhi.ai%'
ORDER BY id;
"
echo "=== WECHAT IDENTITY TABLES ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT table_name FROM information_schema.tables
WHERE table_schema='public' AND table_name ILIKE '%wechat%'
ORDER BY 1;
"
echo "=== ALT CANDIDATES (ACTIVE users, prefer wechat-bound) ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT u.id, u.email, u.name, u.member_level, u.agent_status,
       left(coalesce(u.subscription_expires_at::text,''), 32) AS expires,
       CASE WHEN nullif(btrim(coalesce(u.wechat_union_id,'')),'') IS NOT NULL THEN 'HAS_UNION' ELSE 'NO_UNION' END AS wechat_union,
       CASE WHEN exists(
         SELECT 1 FROM xz_orders o
         WHERE o.user_id = u.id AND coalesce(o.wechat_openid_hash,'') <> ''
       ) THEN 'HAS_PAY_OPENID' ELSE 'NO_PAY_OPENID' END AS pay_openid
FROM xz_users u
WHERE u.status = 'ACTIVE'
  AND u.id <> 'user_000002'
ORDER BY
  (nullif(btrim(coalesce(u.wechat_union_id,'')),'') IS NOT NULL) DESC,
  (exists(SELECT 1 FROM xz_orders o WHERE o.user_id=u.id AND coalesce(o.wechat_openid_hash,'')<>'')) DESC,
  u.created_at DESC
LIMIT 40;
"
echo "=== V2 MATRIX COUNTS BY ENV ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT 'price_plans' AS t, environment, count(*) FROM xz_price_plans GROUP BY environment
UNION ALL
SELECT 'goods', environment, count(*) FROM xz_wechat_virtual_goods GROUP BY environment
UNION ALL
SELECT 'bindings', environment, count(*) FROM xz_price_plan_payment_bindings GROUP BY environment
ORDER BY 1,2;
"
echo "=== PRODUCTION NORMAL plans ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, code, plan_id, environment, mode, sale_price_cents, is_default, status, enabled, test_only, gift_points
FROM xz_price_plans
WHERE environment='PRODUCTION' AND mode='NORMAL'
ORDER BY plan_id;
"
echo "=== DONE ==="
