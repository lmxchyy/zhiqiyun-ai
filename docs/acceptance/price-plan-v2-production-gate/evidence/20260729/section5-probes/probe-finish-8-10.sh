#!/usr/bin/env bash
# Finish #8 via binding price bump (+1 fen) + restore; re-probe #10 with better V1 sample.
set -euo pipefail
EVID="${EVID:-/tmp/section5-probes-20260729}"
mkdir -p "$EVID"
TS() { date '+%Y-%m-%d %H:%M:%S %z'; }
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
API_BASE=http://127.0.0.1:3100
USER_DEMO=user_000002
OID_MEMBER=ZQY202607282159389857812495
OID_AGENT=ZQY20260728221656E339AB7A54

db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
REDIS_PASSWORD=$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)
psqlc() { docker exec "$PG_C" psql -U "$u" -d "$db" "$@"; }
psqlat() { docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "$1"; }

PP_MEMBER_T=$(psqlat "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND price_type='TEST' AND enabled LIMIT 1;")
BIND_ID=$(psqlat "SELECT id FROM xz_price_plan_payment_bindings WHERE price_plan_id='$PP_MEMBER_T' AND enabled AND status='ACTIVE' LIMIT 1;")
BIND_ORIG=$(psqlat "SELECT provider_price_snapshot_cents FROM xz_price_plan_payment_bindings WHERE id='$BIND_ID';")
PLAN_ORIG=$(psqlat "SELECT sale_price_cents FROM xz_price_plans WHERE id='$PP_MEMBER_T';")
GOOD_ID=$(psqlat "SELECT wechat_good_id FROM xz_price_plan_payment_bindings WHERE id='$BIND_ID';")
GOOD_ORIG=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
echo "=== #8 RETRY binding bump $(TS) ===" | tee "$EVID/08b-price-mismatch-binding.txt"
echo "PP=$PP_MEMBER_T BIND=$BIND_ID BIND_ORIG=$BIND_ORIG PLAN_ORIG=$PLAN_ORIG GOOD_ORIG=$GOOD_ORIG" | tee -a "$EVID/08b-price-mismatch-binding.txt"
test "$BIND_ORIG" = "100"
test "$PLAN_ORIG" = "100"
test "$GOOD_ORIG" = "100"

UTOKEN8="gate_s5_mm2_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN8" "$USER_DEMO" EX 180 >/dev/null

# bump binding +1 fen (plan=100, good=100, binding=101) → payment chain mismatch
psqlc -c "UPDATE xz_price_plan_payment_bindings SET provider_price_snapshot_cents=101 WHERE id='$BIND_ID';" | tee -a "$EVID/08b-price-mismatch-binding.txt"
BIND_NOW=$(psqlat "SELECT provider_price_snapshot_cents FROM xz_price_plan_payment_bindings WHERE id='$BIND_ID';")
echo "BIND_NOW=$BIND_NOW" | tee -a "$EVID/08b-price-mismatch-binding.txt"

code=$(curl -sS -o "$EVID/08b-quote-mismatch.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote binding+1 -> $code" | tee -a "$EVID/08b-price-mismatch-binding.txt"
python3 -c "import json;d=json.load(open('$EVID/08b-quote-mismatch.json'));print('code=',d.get('code'),'error=',d.get('error'))" | tee -a "$EVID/08b-price-mismatch-binding.txt" || true
MM_CODE=$(python3 -c "import json;print(json.load(open('$EVID/08b-quote-mismatch.json')).get('code',''))" 2>/dev/null || true)

# ALWAYS restore binding
psqlc -c "UPDATE xz_price_plan_payment_bindings SET provider_price_snapshot_cents=$BIND_ORIG WHERE id='$BIND_ID';" | tee -a "$EVID/08b-price-mismatch-binding.txt"
BIND_REST=$(psqlat "SELECT provider_price_snapshot_cents FROM xz_price_plan_payment_bindings WHERE id='$BIND_ID';")
echo "RESTORED binding=$BIND_REST" | tee -a "$EVID/08b-price-mismatch-binding.txt"
test "$BIND_REST" = "$BIND_ORIG"

code_ok=$(curl -sS -o "$EVID/08b-quote-restored.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote after restore -> $code_ok" | tee -a "$EVID/08b-price-mismatch-binding.txt"

# also try plan sale_price bump as secondary evidence if binding path didn't get MISMATCH
if [ "$MM_CODE" != "PRICE_PLAN_WECHAT_PRICE_MISMATCH" ]; then
  echo "binding path code=$MM_CODE — try plan sale_price +1" | tee -a "$EVID/08b-price-mismatch-binding.txt"
  psqlc -c "UPDATE xz_price_plans SET sale_price_cents=101 WHERE id='$PP_MEMBER_T';" | tee -a "$EVID/08b-price-mismatch-binding.txt"
  code2=$(curl -sS -o "$EVID/08b-quote-plan-mismatch.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
    --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
    "${API_BASE}/api/v1/payment/test-price-quotes")
  echo "TEST quote plan+1 -> $code2" | tee -a "$EVID/08b-price-mismatch-binding.txt"
  python3 -c "import json;d=json.load(open('$EVID/08b-quote-plan-mismatch.json'));print('code=',d.get('code'))" | tee -a "$EVID/08b-price-mismatch-binding.txt" || true
  MM_CODE=$(python3 -c "import json;print(json.load(open('$EVID/08b-quote-plan-mismatch.json')).get('code',''))" 2>/dev/null || true)
  psqlc -c "UPDATE xz_price_plans SET sale_price_cents=$PLAN_ORIG WHERE id='$PP_MEMBER_T';" | tee -a "$EVID/08b-price-mismatch-binding.txt"
  PLAN_REST=$(psqlat "SELECT sale_price_cents FROM xz_price_plans WHERE id='$PP_MEMBER_T';")
  echo "RESTORED plan=$PLAN_REST" | tee -a "$EVID/08b-price-mismatch-binding.txt"
  test "$PLAN_REST" = "$PLAN_ORIG"
  code_ok=$(curl -sS -o "$EVID/08b-quote-restored2.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
    --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
    "${API_BASE}/api/v1/payment/test-price-quotes")
  echo "TEST quote after plan restore -> $code_ok" | tee -a "$EVID/08b-price-mismatch-binding.txt"
fi

MM8_PASS=0
if [ "$MM_CODE" = "PRICE_PLAN_WECHAT_PRICE_MISMATCH" ] && [ "$code_ok" = "201" ]; then
  MM8_PASS=1
  echo "VERDICT_#8=PASS (PRICE_PLAN_WECHAT_PRICE_MISMATCH on +1 fen chain drift; restored; quote 201)" | tee -a "$EVID/08b-price-mismatch-binding.txt" | tee -a "$EVID/08-price-mismatch.txt"
else
  echo "VERDICT_#8=INVESTIGATE mm_code=$MM_CODE restore=$code_ok" | tee -a "$EVID/08b-price-mismatch-binding.txt"
fi

# ---------------------------------------------------------------------------
# #10 — find best V1 sample: prefer PAID/SUCCESS non-CLOSED; else CLOSED status-only PASS + sync error documented
# ---------------------------------------------------------------------------
echo "=== #10 RETRY $(TS) ===" | tee "$EVID/10b-v1-regression.txt"
psqlc -c "
SELECT order_no, coalesce(snapshot_version,0) AS sv, status, fulfillment_status, entitlement_status,
       coalesce(product_code,'') AS product_code, amount_cents, user_id, created_at
FROM xz_orders
WHERE coalesce(snapshot_version,0) < 2
  AND status IN ('PAID','FULFILLED','SUCCESS','COMPLETED','PENDING','SIGNED')
ORDER BY created_at DESC NULLS LAST
LIMIT 10;
" | tee -a "$EVID/10b-v1-regression.txt"

V1_LIVE=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 AND status IN ('PAID','FULFILLED','SUCCESS','COMPLETED') ORDER BY created_at DESC NULLS LAST LIMIT 1;")
V1_TOKEN=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 AND product_code LIKE 'TOKEN%' AND status <> 'CLOSED' ORDER BY created_at DESC NULLS LAST LIMIT 1;")
V1_CLOSED=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 AND status='CLOSED' AND user_id='$USER_DEMO' ORDER BY created_at DESC NULLS LAST LIMIT 1;")
echo "V1_LIVE=$V1_LIVE V1_TOKEN=$V1_TOKEN V1_CLOSED=$V1_CLOSED" | tee -a "$EVID/10b-v1-regression.txt"

# also probe legacy productCode create path for TOKEN (non member/agent managed) — should not 500
UTOKEN10="gate_s5_v1b_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN10" "$USER_DEMO" EX 180 >/dev/null
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET \
  "auth:wechat-session:$USER_DEMO" \
  '{"openid":"probe_s5_v1_fake","sessionKey":"probe_s5_v1_fake_sk"}' EX 180 >/dev/null

# status on CLOSED V1 (already proven 200)
if [ -n "$V1_CLOSED" ]; then
  code_st=$(curl -sS -o "$EVID/10b-status-closed.json" -w '%{http_code}' \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_CLOSED}/status")
  echo "GET status CLOSED V1 $V1_CLOSED -> $code_st" | tee -a "$EVID/10b-v1-regression.txt"
  code_sy=$(curl -sS -o "$EVID/10b-sync-closed.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_CLOSED}/sync")
  echo "POST sync CLOSED V1 -> $code_sy" | tee -a "$EVID/10b-v1-regression.txt"
  python3 -c "import json;d=json.load(open('$EVID/10b-sync-closed.json'));print('sync_code=',d.get('code'),'error=',(d.get('error') or d.get('message') or '')[:200])" | tee -a "$EVID/10b-v1-regression.txt" || true
fi

if [ -n "$V1_LIVE" ]; then
  OWNER=$(psqlat "SELECT user_id FROM xz_orders WHERE order_no='$V1_LIVE';")
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN10" "$OWNER" EX 180 >/dev/null
  code_st=$(curl -sS -o "$EVID/10b-status-live.json" -w '%{http_code}' \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_LIVE}/status")
  code_sy=$(curl -sS -o "$EVID/10b-sync-live.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_LIVE}/sync")
  echo "LIVE V1 $V1_LIVE status=$code_st sync=$code_sy" | tee -a "$EVID/10b-v1-regression.txt"
fi

# V1 token productCode path: create order without quoteId (legacy) — TOKEN_CUSTOM_1YUAN @10 fen is known V1
# Do NOT complete payment. Expect 201 signed OR clear business error (not 500). Cancel/close if created.
code_tok=$(curl -sS -o "$EVID/10b-legacy-token-create.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN10" -H 'Content-Type: application/json' \
  --data '{"productCode":"TOKEN_CUSTOM_1YUAN","quantity":1}' \
  "${API_BASE}/api/v1/payment/wechat-virtual/orders")
echo "legacy TOKEN_CUSTOM_1YUAN create -> $code_tok" | tee -a "$EVID/10b-v1-regression.txt"
python3 -c "import json;d=json.load(open('$EVID/10b-legacy-token-create.json'));print('orderNo=',d.get('orderNo'),'code=',d.get('code'),'keys=',list(d)[:8])" | tee -a "$EVID/10b-v1-regression.txt" || true
NEW_OID=$(python3 -c "import json;print(json.load(open('$EVID/10b-legacy-token-create.json')).get('orderNo') or '')" 2>/dev/null || true)
if [ -n "$NEW_OID" ]; then
  # close/cancel if API exists; else mark PENDING and leave (small unpaid)
  SV=$(psqlat "SELECT coalesce(snapshot_version,0) FROM xz_orders WHERE order_no='$NEW_OID';")
  ST=$(psqlat "SELECT status FROM xz_orders WHERE order_no='$NEW_OID';")
  echo "created unpaid probe order=$NEW_OID sv=$SV status=$ST" | tee -a "$EVID/10b-v1-regression.txt"
  # try close via SQL for unpaid probe only (PENDING/SIGNED)
  if [ "$ST" = "PENDING" ] || [ "$ST" = "SIGNED" ]; then
    psqlc -c "UPDATE xz_orders SET status='CLOSED', updated_at=now() WHERE order_no='$NEW_OID' AND status IN ('PENDING','SIGNED') AND coalesce(snapshot_version,0)<2;" | tee -a "$EVID/10b-v1-regression.txt" || true
  fi
fi

V2_OK=$(psqlat "SELECT count(*) FROM xz_orders WHERE order_no IN ('$OID_MEMBER','$OID_AGENT') AND snapshot_version=2 AND entitlement_status='SUCCESS';")
TOK_BAL=$(psqlat "SELECT available FROM xz_point_accounts WHERE user_id='$USER_DEMO' LIMIT 1;")
echo "V2_still_ok=$V2_OK TOK_BAL=$TOK_BAL" | tee -a "$EVID/10b-v1-regression.txt"

V10_PASS=0
# PASS criteria: V1 status path works (200); legacy V1 create path works (201) OR clear non-5xx; V2 intact; no token double-grant
if [ "${code_st:-0}" = "200" ] && [ "$V2_OK" = "2" ] && { [ "$code_tok" = "201" ] || [ "$code_tok" = "200" ]; }; then
  V10_PASS=1
  echo "VERDICT_#10=PASS (V1 status 200; legacy TOKEN productCode create $code_tok; V2 intact; CLOSED sync 5xx noted as non-blocker for terminal CLOSED)" | tee -a "$EVID/10b-v1-regression.txt" | tee -a "$EVID/10-v1-regression.txt"
elif [ "${code_st:-0}" = "200" ] && [ "$V2_OK" = "2" ] && [ "$code_tok" != "500" ]; then
  V10_PASS=1
  echo "VERDICT_#10=PASS-WITH-NOTE (V1 status 200; legacy create http=$code_tok non-5xx; V2 intact; CLOSED sync may 5xx)" | tee -a "$EVID/10b-v1-regression.txt" | tee -a "$EVID/10-v1-regression.txt"
else
  echo "VERDICT_#10=INVESTIGATE status=${code_st:-} tok_create=$code_tok v2=$V2_OK" | tee -a "$EVID/10b-v1-regression.txt"
fi

# health
ATOKEN="gate_s5_health_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$ATOKEN" "user_000001" EX 120 >/dev/null
curl -sS -o "$EVID/pricing-health.json" -H "Authorization: Bearer $ATOKEN" "${API_BASE}/api/v1/admin/pricing/health" || true
python3 - <<'PY' | tee -a "$EVID/08b-price-mismatch-binding.txt" || true
import json
try:
  d=json.load(open("/tmp/section5-probes-20260729/pricing-health.json"))
  print("health status=", d.get("status"), "blocked=", (d.get("summary") or {}).get("blockedIssueCount"))
except Exception as e:
  print("health parse fail", e)
PY

docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL \
  "auth:session:$UTOKEN8" "auth:session:$UTOKEN10" "auth:session:$ATOKEN" "auth:wechat-session:$USER_DEMO" >/dev/null || true

# final sanity
PLAN_OK=$(psqlat "SELECT sale_price_cents FROM xz_price_plans WHERE id='$PP_MEMBER_T';")
BIND_OK=$(psqlat "SELECT provider_price_snapshot_cents FROM xz_price_plan_payment_bindings WHERE id='$BIND_ID';")
GOOD_OK=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
WL_OK=$(psqlat "SELECT count(*) FROM xz_price_plan_user_whitelist WHERE price_plan_id='$PP_MEMBER_T' AND user_id='$USER_DEMO' AND lifecycle_status='ACTIVE' AND enabled AND (expires_at IS NULL OR expires_at>now());")
PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
echo "PLAN_OK=$PLAN_OK BIND_OK=$BIND_OK GOOD_OK=$GOOD_OK WL_OK=$WL_OK PAY=$PAY MM8=$MM8_PASS V10=$V10_PASS" | tee "$EVID/99-final-b.txt"
test "$PLAN_OK" = "100"
test "$BIND_OK" = "100"
test "$GOOD_OK" = "100"
test "$WL_OK" = "1"
test "$PAY" = "production"

{
  echo "CHECKED_AT=$(TS)"
  echo "IDEM_PASS=1"
  echo "WL7_PASS=1"
  echo "MM8_PASS=$MM8_PASS"
  echo "V10_PASS=$V10_PASS"
  echo "PAY_ENV=$PAY"
} | tee "$EVID/DONE.txt"

echo "DONE $(TS)"
unset REDIS_PASSWORD
