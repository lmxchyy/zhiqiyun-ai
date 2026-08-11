#!/usr/bin/env bash
# PRODUCTION dry quotes for NORMAL ¥996 + repurchase rule snapshot. No order create.
set -euo pipefail
EVID="${EVID:-/tmp/normal-996-20260729}"
mkdir -p "$EVID"
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
API_BASE=http://127.0.0.1:3100
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)

echo "=== FLAGS ===" | tee "$EVID/00-flags.txt"
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV | tee -a "$EVID/00-flags.txt"

echo "=== DEMO USER ===" | tee "$EVID/01-demo-user.txt"
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, email, name, member_level, agent_status,
       left(coalesce(subscription_expires_at::text,''), 32) AS expires
FROM xz_users WHERE id='user_000002';
" | tee -a "$EVID/01-demo-user.txt"

echo "=== PRODUCTION NORMAL defaults ===" | tee "$EVID/02-plans.txt"
docker exec "$PG_C" psql -U "$u" -d "$db" -c "
SELECT id, code, plan_id, environment, price_type, sale_price_cents, is_default, enabled, status, bonus_points
FROM xz_price_plans
WHERE environment='PRODUCTION' AND price_type='NORMAL'
ORDER BY plan_id;
" | tee -a "$EVID/02-plans.txt"

PP_MEMBER=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND price_type='NORMAL' AND is_default AND enabled LIMIT 1;")
PP_AGENT=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_agent_join_996' AND environment='PRODUCTION' AND price_type='NORMAL' AND is_default AND enabled LIMIT 1;")
echo "PP_MEMBER=$PP_MEMBER PP_AGENT=$PP_AGENT" | tee -a "$EVID/02-plans.txt"

REDIS_PASSWORD=$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)
UTOKEN="gate_normal996_quote_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN" "user_000002" EX 300 >/dev/null

quote() {
  local plan="$1" pp="$2" out="$3"
  local code
  code=$(curl -sS -o "$EVID/$out" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN" \
    -H 'Content-Type: application/json' \
    --data "{\"planId\":\"${plan}\",\"pricePlanId\":\"${pp}\"}" \
    "${API_BASE}/api/v1/payment/price-quotes")
  echo "POST price-quotes $plan -> $code" | tee -a "$EVID/03-quotes.txt"
  python3 - <<PY
import json,hashlib
p="$EVID/$out"
d=json.load(open(p))
qid=d.get("quoteId")
if qid:
  d["quoteId"]="sha256:"+hashlib.sha256(qid.encode()).hexdigest()[:16]
json.dump(d, open(p,"w"), ensure_ascii=False, indent=2)
print("amountCent=", d.get("amountCent"), "env=", d.get("environment"), "testOnly=", d.get("testOnly"))
PY
}

: > "$EVID/03-quotes.txt"
quote plan_ai_creator_996 "$PP_MEMBER" quote-member-normal.json
quote plan_agent_join_996 "$PP_AGENT" quote-agent-normal.json

docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN" >/dev/null
unset REDIS_PASSWORD
echo DONE | tee "$EVID/DONE.txt"
