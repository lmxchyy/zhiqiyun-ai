#!/usr/bin/env bash
# Dry AGENT NORMAL quote only — no order create. Redact quoteId.
set -euo pipefail
EVID=/tmp/v2-flags-enable-20260729
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
API_BASE=http://127.0.0.1:3100
db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
USER_ID=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_users ORDER BY created_at NULLS LAST LIMIT 1;")
PRICE_AGENT=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_agent_join_996' AND environment='PRODUCTION' AND is_default AND enabled LIMIT 1;")
REDIS_PASSWORD=$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)
UTOKEN="gate_v2_agent_quote_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN" "$USER_ID" EX 180 >/dev/null
code=$(curl -sS -o "$EVID/quote-agent-normal.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN" \
  -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_agent_join_996\",\"pricePlanId\":\"${PRICE_AGENT}\"}" \
  "${API_BASE}/api/v1/payment/price-quotes")
echo "POST price-quotes agent-normal -> $code user=$USER_ID pricePlan=$PRICE_AGENT"
python3 - <<'PY'
import json,hashlib
p="/tmp/v2-flags-enable-20260729/quote-agent-normal.json"
d=json.load(open(p))
qid=d.get("quoteId")
if qid:
  d["quoteId"]="sha256:"+hashlib.sha256(qid.encode()).hexdigest()[:16]
json.dump(d, open(p,"w"), ensure_ascii=False, indent=2)
print("amountCent=", d.get("amountCent"), "env=", d.get("environment"), "testOnly=", d.get("testOnly"))
PY
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN" >/dev/null
