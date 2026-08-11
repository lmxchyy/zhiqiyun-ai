#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
echo "db=$db user=$u"
echo "=== columns xz_price_plans ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "\d xz_price_plans"
echo "=== all price plans ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, code, plan_id, environment, kind, sale_price_cents, is_default, status,
       coalesce(is_enabled::text, enabled::text) AS enabled_flag, test_only, gift_points
FROM xz_price_plans
ORDER BY environment, plan_id, kind;
" 2>&1 || docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, code, plan_id, environment, kind, sale_price_cents, is_default, status, gift_points
FROM xz_price_plans ORDER BY environment, plan_id, kind;
"
echo "=== demo / alt ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, email, member_level, agent_status
FROM xz_users
WHERE email ILIKE '%demo%' OR id IN ('user_000010','user_000019','user_000015','user_000011','user_000002');
"
echo "=== recent virtual pay users ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT user_id, count(*) AS n, max(created_at) AS last_at
FROM xz_orders
WHERE wechat_product_id_snapshot IS NOT NULL OR payment_channel ILIKE '%virtual%'
GROUP BY user_id
ORDER BY max(created_at) DESC
LIMIT 15;
"
echo "=== DONE ==="
