#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
OID=ZQY20260728221656E339AB7A54
TXN=4500000301202607297737425043
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
TS=$(date '+%Y-%m-%d %H:%M:%S %z')
echo "CHECKED_AT=$TS"
echo "ORDER=$OID TXN=$TXN"

echo "=== flags ==="
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV || true

echo
echo "=== order summary ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, status, fulfillment_status, entitlement_status,
       amount_cents, transaction_price_cents, wechat_goods_price_cents,
       wechat_product_id_snapshot, snapshot_version, payment_environment,
       wechat_transaction_id, price_plan_id, price_quote_id, plan_version_id,
       product_code, product_type, payment_mode,
       token_amount, token_grant_amount,
       paid_at, fulfilled_at, entitlement_granted_at, created_at,
       rights_snapshot, commission_rule_version_snapshot
FROM xz_orders WHERE id='$OID';
"

echo
echo "=== payment events ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, event_type, event_id, status, processing_status, transaction_id, amount_cents,
       verified, created_at, processed_at
FROM xz_payment_events
WHERE order_id='$OID' OR transaction_id='$TXN'
ORDER BY created_at;
"

echo
echo "=== payment records ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT payment_no, order_no, prepay_status, amount_cents,
       wechat_order_id, wechat_transaction_id, paid_at,
       callback_payload, notify_payload
FROM xz_payment_records WHERE order_no='$OID';
"

echo
echo "=== quote ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, plan_id, price_plan_id, entry_type, status,
       transaction_price_cents, wechat_product_id, environment, offer_id,
       payment_mode, whitelist_entry_id, created_at, consumed_at, consumed_order_no
FROM xz_order_price_quotes WHERE consumed_order_no='$OID';
"

echo
echo "=== user ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, name, role, member_level, agent_status, plan_id, subscription_expires_at, updated_at
FROM xz_users WHERE id='user_000002';
"

echo
echo "=== identities ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, identity_type, identity_status, commission_enabled, source_type, source_order_id,
       effective_at, expires_at, ended_at, created_at
FROM xz_user_business_identities WHERE user_id='user_000002' ORDER BY created_at;
"

echo
echo "=== channel agent ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, level, status, join_order_id, invite_code, token_rights_amount, created_at, updated_at
FROM xz_channel_agents WHERE user_id='user_000002';
"

echo
echo "=== agent profile ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, level, status, join_order_id, token_rights_amount, created_at
FROM xz_agent_profiles WHERE user_id='user_000002';
"

echo
echo "=== token grant ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, user_id, order_id, change_type, amount, balance_before, balance_after, remark, created_at
FROM xz_token_records WHERE order_id='$OID';
"

echo
echo "=== wallet ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT user_id, token_balance, total_token_granted, total_token_used, updated_at
FROM xz_user_wallets WHERE user_id='user_000002';
"

echo
echo "=== commissions ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, order_id, agent_id, amount_cents, rate, status, created_at
FROM xz_commissions WHERE order_id='$OID';
"

echo
echo "=== MEMBER+AGENT TEST orders ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, status, fulfillment_status, entitlement_status,
       wechat_product_id_snapshot, amount_cents, snapshot_version,
       wechat_transaction_id, paid_at, fulfilled_at
FROM xz_orders
WHERE user_id='user_000002'
  AND wechat_product_id_snapshot IN ('MEMBER_TEST_1YUAN','AGENT_TEST_1YUAN')
ORDER BY created_at;
"
