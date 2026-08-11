#!/bin/bash
set -euo pipefail
PG_C=zhiqiyun-ai-prod-postgres-1
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT pp.environment, pp.code, pp.price_type, pp.sale_price_cents, pp.is_default, pp.enabled,
       g.product_id, g.platform_price_cents, g.offer_id, g.mode, b.status AS binding_status, b.enabled AS binding_enabled
FROM xz_price_plans pp
JOIN xz_price_plan_payment_bindings b ON b.price_plan_id = pp.id
JOIN xz_wechat_virtual_goods g ON g.id = b.wechat_good_id
WHERE pp.price_type='NORMAL'
ORDER BY pp.environment, pp.plan_id;
"
