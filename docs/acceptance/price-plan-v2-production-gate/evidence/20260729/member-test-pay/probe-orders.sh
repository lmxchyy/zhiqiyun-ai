#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
TS=$(date '+%Y-%m-%d %H:%M:%S %z')
echo "CHECKED_AT=$TS"
echo "db=$db user=$u"
echo "=== flags ==="
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV || true
echo
echo "=== created_at type ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_name='xz_orders'
  AND column_name IN ('created_at','updated_at','paid_at','fulfilled_at','status','order_status','provider_order_id','out_trade_no','wechat_transaction_id');
"
echo
echo "=== latest 12 orders for user_000002 ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, status, order_status, fulfillment_status, entitlement_status,
       amount_cents, transaction_price_cents, wechat_goods_price_cents,
       wechat_product_id_snapshot, snapshot_version, payment_environment,
       price_plan_id, price_quote_id, plan_version_id,
       order_type, business_order_type,
       paid_at, fulfilled_at, entitlement_granted_at,
       created_at, updated_at
FROM xz_orders
WHERE user_id = 'user_000002'
ORDER BY created_at DESC
LIMIT 12;
"
echo
echo "=== MEMBER_TEST / 100 fen last ~45m (cast timestamptz) ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, status, order_status, fulfillment_status, entitlement_status,
       amount_cents, transaction_price_cents, wechat_goods_price_cents,
       wechat_product_id_snapshot, snapshot_version, payment_environment,
       price_plan_id, price_quote_id, plan_version_id,
       order_type, business_order_type,
       paid_at, fulfilled_at, entitlement_granted_at,
       created_at, updated_at
FROM xz_orders
WHERE user_id = 'user_000002'
  AND created_at::timestamptz > now() - interval '45 minutes'
  AND (
    wechat_product_id_snapshot = 'MEMBER_TEST_1YUAN'
    OR price_plan_id = 'price_plan_20260728212634000000000_049a91b1'
    OR amount_cents = 100
    OR transaction_price_cents = 100
  )
ORDER BY created_at DESC;
"
echo
echo "=== all orders last 45m for user ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, status, order_status, fulfillment_status, entitlement_status,
       amount_cents, wechat_product_id_snapshot, snapshot_version,
       price_plan_id, created_at, paid_at, fulfilled_at
FROM xz_orders
WHERE user_id = 'user_000002'
  AND created_at::timestamptz > now() - interval '45 minutes'
ORDER BY created_at DESC;
"
echo
echo "=== AGENT_TEST last 2h ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, status, order_status, fulfillment_status, entitlement_status,
       amount_cents, wechat_product_id_snapshot, snapshot_version,
       price_plan_id, created_at, paid_at, fulfilled_at
FROM xz_orders
WHERE user_id = 'user_000002'
  AND created_at::timestamptz > now() - interval '2 hours'
  AND (
    wechat_product_id_snapshot = 'AGENT_TEST_1YUAN'
    OR price_plan_id = 'price_plan_20260728212634000000000_2ec1c485'
  )
ORDER BY created_at DESC;
"
echo
echo "=== user identity ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, email, name, role, status, agent_status, member_level, updated_at
FROM xz_users WHERE id = 'user_000002';
"
