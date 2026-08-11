#!/usr/bin/env bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
db=$(docker exec $PG_C printenv POSTGRES_DB)
u=$(docker exec $PG_C printenv POSTGRES_USER)
echo "=== xz_point_accounts cols ==="
docker exec $PG_C psql -U "$u" -d "$db" -c "SELECT column_name FROM information_schema.columns WHERE table_name='xz_point_accounts' ORDER BY ordinal_position;"
echo "=== xz_user_wallets cols ==="
docker exec $PG_C psql -U "$u" -d "$db" -c "SELECT column_name FROM information_schema.columns WHERE table_name='xz_user_wallets' ORDER BY ordinal_position;"
echo "=== token sample ==="
docker exec $PG_C psql -U "$u" -d "$db" -c "SELECT * FROM xz_point_accounts WHERE user_id='user_000002' LIMIT 1;" 2>&1 | head -20
docker exec $PG_C psql -U "$u" -d "$db" -c "SELECT id, user_id, token_balance FROM xz_user_wallets WHERE user_id='user_000002' LIMIT 1;" 2>&1 | head -20
echo "=== order product cols ==="
docker exec $PG_C psql -U "$u" -d "$db" -c "SELECT column_name FROM information_schema.columns WHERE table_name='xz_orders' AND (column_name LIKE '%product%' OR column_name LIKE '%wechat%') ORDER BY 1;"
echo "=== identity table ==="
docker exec $PG_C psql -U "$u" -d "$db" -c "SELECT count(*) FROM xz_identity_change_records WHERE user_id='user_000002';" 2>&1 | head -5
echo "=== ledger counts ==="
docker exec $PG_C psql -U "$u" -d "$db" -c "
SELECT o.order_no,
  (SELECT count(*) FROM xz_membership_entitlement_records m WHERE m.source_order_no=o.order_no) AS memb_n,
  (SELECT count(*) FROM xz_token_records t WHERE t.source_order_no=o.order_no OR t.order_id=o.order_no) AS tok_n
FROM xz_orders o WHERE o.order_no IN ('ZQY202607282159389857812495','ZQY20260728221656E339AB7A54');
"
