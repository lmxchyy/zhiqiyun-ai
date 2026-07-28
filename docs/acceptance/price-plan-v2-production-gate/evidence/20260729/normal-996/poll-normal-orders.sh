#!/usr/bin/env bash
# OPTIONAL diagnostics only — policy 2026-07-29: do NOT ask user to pay real Y996.
# Payment path ACCEPTED via Y1 TEST. This script only lists any incidental NORMAL orders.
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
SINCE="${1:-2026-07-29 00:00:00+08}"
echo "SINCE=$SINCE (read-only; no pay requested)"
docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id AS order_no, user_id, status, order_status, fulfillment_status, entitlement_status,
       amount_cents, wechat_product_id_snapshot, snapshot_version, payment_environment,
       price_plan_id, paid_at, fulfilled_at, created_at
FROM xz_orders
WHERE created_at::timestamptz >= '$SINCE'::timestamptz
  AND (
    wechat_product_id_snapshot IN ('MEMBER_YEAR_996','AGENT_JOIN_996')
    OR amount_cents = 99600
  )
ORDER BY created_at DESC
LIMIT 30;
"
