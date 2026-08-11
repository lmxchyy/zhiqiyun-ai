#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
OID=ZQY202607282159389857812495
TXN=4500000265202607293548593890
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
TS=$(date '+%Y-%m-%d %H:%M:%S %z')
echo "CHECKED_AT=$TS"
echo "ORDER=$OID TXN=$TXN"

echo "=== order summary ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, status, fulfillment_status, entitlement_status,
       amount_cents, wechat_product_id_snapshot, snapshot_version, payment_environment,
       wechat_transaction_id, price_plan_id, price_quote_id,
       paid_at, fulfilled_at, entitlement_granted_at,
       token_amount, token_grant_amount,
       rights_snapshot
FROM xz_orders WHERE id='$OID';
"

echo
echo "=== payment events (all types for order) ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, event_type, event_id, status, processing_status, transaction_id, amount_cents,
       verified, created_at, processed_at, left(raw_body::text, 200) AS raw_body_prefix
FROM xz_payment_events
WHERE order_id='$OID' OR transaction_id='$TXN'
ORDER BY created_at;
"

echo
echo "=== payment records ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT payment_no, order_no, prepay_status, amount_cents,
       wechat_order_id, wechat_transaction_id, paid_at,
       callback_payload, notify_payload,
       request_payload, response_payload
FROM xz_payment_records WHERE order_no='$OID';
"

echo
echo "=== membership entitlement ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, member_level, effective_at, expires_at, source_order_no, idempotency_key, metadata, created_at
FROM xz_membership_entitlement_records WHERE source_order_no='$OID';
"

echo
echo "=== token grant ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, user_id, order_id, change_type, amount, balance_before, balance_after, remark, created_at
FROM xz_token_records WHERE order_id='$OID';
"

echo
echo "=== wallet ledger ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, entry_type, points, available_before, available_after, idempotency_key, reference_id, created_at
FROM xz_wallet_ledger
WHERE reference_id LIKE '%$OID%' OR idempotency_key LIKE '%$OID%';
"

echo
echo "=== user after grant ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, member_level, role, agent_status, updated_at FROM xz_users WHERE id='user_000002';
SELECT user_id, token_balance, total_token_granted, total_token_used, updated_at FROM xz_user_wallets WHERE user_id='user_000002';
"

echo
echo "=== AGENT_TEST still absent? ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, status, wechat_product_id_snapshot, amount_cents, created_at
FROM xz_orders
WHERE user_id='user_000002'
  AND (wechat_product_id_snapshot='AGENT_TEST_1YUAN' OR price_plan_id='price_plan_20260728212634000000000_2ec1c485')
ORDER BY created_at DESC LIMIT 5;
"

echo
echo "=== any deliver/ship related payment event types globally recent ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT event_type, count(*) FROM xz_payment_events
WHERE created_at > now() - interval '2 days'
GROUP BY 1 ORDER BY 2 DESC;
"
