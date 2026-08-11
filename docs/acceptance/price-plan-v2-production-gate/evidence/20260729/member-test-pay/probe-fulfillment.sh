#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
OID=ZQY202607282159389857812495
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
TS=$(date '+%Y-%m-%d %H:%M:%S %z')
echo "CHECKED_AT=$TS"
echo "ORDER_ID=$OID"
echo
echo "=== full order row (key fields) ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, user_id, status, fulfillment_status, entitlement_status, entitlement_error,
       amount_cents, transaction_price_cents, wechat_goods_price_cents, payable_amount_cents,
       wechat_product_id_snapshot, snapshot_version, payment_environment,
       price_plan_id, price_quote_id, plan_version_id,
       wechat_transaction_id, token_amount, token_grant_amount,
       paid_at, fulfilled_at, entitlement_started_at, entitlement_granted_at,
       created_at, updated_at,
       rights_snapshot, commission_snapshot_v2, commission_rule_version_snapshot,
       price_snapshot
FROM xz_orders WHERE id = '$OID';
"
echo
echo "=== quote row ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT * FROM xz_price_quotes WHERE id = 'quote_cc2367c46d222553b85a53c5';
" 2>&1 || docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT table_name FROM information_schema.tables WHERE table_name LIKE '%quote%';
"
echo
echo "=== ledger / entitlement related tables ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "
SELECT table_name FROM information_schema.tables
WHERE table_schema='public' AND (
  table_name LIKE '%ledger%' OR table_name LIKE '%entitle%' OR table_name LIKE '%wallet%'
  OR table_name LIKE '%grant%' OR table_name LIKE '%commission%' OR table_name LIKE '%payment%'
  OR table_name LIKE '%callback%' OR table_name LIKE '%wechat%pay%' OR table_name LIKE '%virtual%pay%'
)
ORDER BY 1;
"
echo
echo "=== membership / identity after pay ==="
docker exec "$PG_C" psql -U "$u" -d "$db" -x -c "
SELECT id, email, role, member_level, agent_status, status, updated_at
FROM xz_users WHERE id='user_000002';
"
echo
echo "=== search order id in likely audit/ledger tables (best effort) ==="
for t in xz_wallet_ledgers xz_token_ledgers xz_point_ledgers xz_entitlement_grants xz_order_entitlements xz_payment_callbacks xz_wechat_payment_events xz_commerce_ledgers xz_audit_logs xz_identity_events; do
  exists=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT to_regclass('$t') IS NOT NULL")
  echo "-- $t exists=$exists"
  if [ "$exists" = "t" ]; then
    docker exec "$PG_C" psql -U "$u" -d "$db" -c "
      SELECT * FROM $t
      WHERE CAST(row_to_json($t) AS text) LIKE '%$OID%'
         OR CAST(row_to_json($t) AS text) LIKE '%user_000002%'
      ORDER BY 1 DESC NULLS LAST
      LIMIT 5;
    " 2>&1 | head -80 || true
  fi
done
