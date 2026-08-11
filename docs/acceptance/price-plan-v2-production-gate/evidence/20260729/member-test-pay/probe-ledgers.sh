#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
OID=ZQY202607282159389857812495
TXN=4500000265202607293548593890
QID=quote_cc2367c46d222553b85a53c5
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
TS=$(date '+%Y-%m-%d %H:%M:%S %z')
echo "CHECKED_AT=$TS"
echo "ORDER_ID=$OID TXN=$TXN"

echo "=== xz_order_price_quotes ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, price_plan_id, plan_version_id, sale_price_cents, environment, status,
       wechat_product_id, snapshot_version, created_at, expires_at, consumed_at
FROM xz_order_price_quotes WHERE id='$QID';
" 2>&1 || docker exec "$PG_C" psql -U "$u" -d "$db" -c "\d xz_order_price_quotes"

echo
echo "=== quote columns + row ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "
SELECT column_name FROM information_schema.columns WHERE table_name='xz_order_price_quotes' ORDER BY ordinal_position;
"
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "SELECT * FROM xz_order_price_quotes WHERE id='$QID';" 2>&1 | head -120

echo
echo "=== xz_payment_events for order/txn ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT * FROM xz_payment_events
WHERE CAST(row_to_json(xz_payment_events) AS text) LIKE '%$OID%'
   OR CAST(row_to_json(xz_payment_events) AS text) LIKE '%$TXN%'
ORDER BY 1 DESC LIMIT 10;
" 2>&1 | head -160

echo
echo "=== xz_payment_records ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT * FROM xz_payment_records
WHERE CAST(row_to_json(xz_payment_records) AS text) LIKE '%$OID%'
   OR CAST(row_to_json(xz_payment_records) AS text) LIKE '%$TXN%'
ORDER BY 1 DESC LIMIT 10;
" 2>&1 | head -160

echo
echo "=== payment_events (legacy) ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT * FROM payment_events
WHERE CAST(row_to_json(payment_events) AS text) LIKE '%$OID%'
   OR CAST(row_to_json(payment_events) AS text) LIKE '%$TXN%'
ORDER BY 1 DESC LIMIT 5;
" 2>&1 | head -80

echo
echo "=== xz_membership_entitlement_records ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT * FROM xz_membership_entitlement_records
WHERE user_id='user_000002' OR source_order_no='$OID' OR CAST(row_to_json(xz_membership_entitlement_records) AS text) LIKE '%$OID%'
ORDER BY 1 DESC LIMIT 5;
" 2>&1 | head -120

echo
echo "=== xz_token_records for order ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, user_id, order_id, change_type, amount, balance_before, balance_after, remark, created_at, idempotency_key, source_order_no
FROM xz_token_records
WHERE order_id='$OID' OR source_order_no='$OID' OR user_id='user_000002'
ORDER BY created_at DESC LIMIT 8;
" 2>&1 | head -80

echo
echo "=== xz_wallet_ledger for order ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT * FROM xz_wallet_ledger
WHERE CAST(row_to_json(xz_wallet_ledger) AS text) LIKE '%$OID%'
ORDER BY 1 DESC LIMIT 8;
" 2>&1 | head -100

echo
echo "=== xz_compute_ledger_entries for order ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT * FROM xz_compute_ledger_entries
WHERE CAST(row_to_json(xz_compute_ledger_entries) AS text) LIKE '%$OID%'
ORDER BY 1 DESC LIMIT 8;
" 2>&1 | head -100

echo
echo "=== product_entitlements ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT * FROM product_entitlements
WHERE CAST(row_to_json(product_entitlements) AS text) LIKE '%$OID%'
   OR CAST(row_to_json(product_entitlements) AS text) LIKE '%user_000002%'
ORDER BY 1 DESC LIMIT 5;
" 2>&1 | head -80

echo
echo "=== identity change records for order ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, user_id, change_type, source_type, reason, created_at, idempotency_key
FROM xz_identity_change_records
WHERE user_id='user_000002' OR CAST(row_to_json(xz_identity_change_records) AS text) LIKE '%$OID%'
ORDER BY created_at DESC LIMIT 5;
" 2>&1 | head -60

echo
echo "=== token balance / wallet snapshot ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT * FROM xz_user_wallets WHERE user_id='user_000002' LIMIT 3;
" 2>&1 | head -40
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, user_id, available, frozen FROM xz_point_accounts WHERE user_id='user_000002' LIMIT 3;
" 2>&1 | head -20
