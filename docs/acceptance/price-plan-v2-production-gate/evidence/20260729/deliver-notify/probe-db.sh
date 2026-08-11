#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
DB=$(docker exec "$PG_C" printenv POSTGRES_DB)
USER=$(docker exec "$PG_C" printenv POSTGRES_USER)
echo "=== orders ==="
docker exec -i "$PG_C" psql -U "$USER" -d "$DB" -c \
"select order_no, status, entitlement_status, wechat_openid_hash, wechat_order_id, wechat_transaction_id
 from xz_orders
 where order_no in ('ZQY202607282159389857812495','ZQY20260728221656E339AB7A54');"
echo "=== payment events ==="
docker exec -i "$PG_C" psql -U "$USER" -d "$DB" -c \
"select order_id, event_type, processing_status, error_message, processed_at
 from xz_payment_events
 where order_id in ('ZQY202607282159389857812495','ZQY20260728221656E339AB7A54')
 order by created_at;"
echo "=== redis wechat keys sample ==="
docker exec "$REDIS_C" redis-cli --scan --pattern '*wechat*' | head -30 || true
echo "=== oneshot json ==="
cat /tmp/deliver-notify-oneshot/oneshot-1785277952.json
