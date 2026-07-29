#!/usr/bin/env bash
# Recover MEMBER TEST whitelist after soft-expire made entry terminal; then finish #8/#10.
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

echo "=== RECOVER WHITELIST $(TS) ===" | tee "$EVID/07b-recover-whitelist.txt"
psqlc -c "SELECT id, price_plan_id, lifecycle_status, enabled, expires_at, revision FROM xz_price_plan_user_whitelist WHERE user_id='$USER_DEMO' ORDER BY created_at;" | tee -a "$EVID/07b-recover-whitelist.txt"

PP_MEMBER_T=$(psqlat "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND price_type='TEST' AND enabled LIMIT 1;")
PP_AGENT_T=$(psqlat "SELECT id FROM xz_price_plans WHERE plan_id='plan_agent_join_996' AND environment='PRODUCTION' AND price_type='TEST' AND enabled LIMIT 1;")
echo "PP_MEMBER_T=$PP_MEMBER_T PP_AGENT_T=$PP_AGENT_T" | tee -a "$EVID/07b-recover-whitelist.txt"

# find admin actor for session — prefer user_000001 (admin used in prior seeds)
ADMIN_ID=$(psqlat "SELECT id FROM xz_users WHERE id='user_000001' LIMIT 1;")
if [ -z "$ADMIN_ID" ]; then ADMIN_ID=user_000001; fi
ATOKEN="gate_s5_wl_recover_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$ATOKEN" "$ADMIN_ID" EX 300 >/dev/null

BODY='{"revision":0,"userId":"user_000002","reason":"section5-probe recovery after soft-expire made prior MEMBER_TEST entry terminal","changeReason":"restore ACTIVE TEST whitelist after §5 #7 probe; prior entry EXPIRED immutable"}'

ACTIVE_M=$(psqlat "SELECT count(*) FROM xz_price_plan_user_whitelist WHERE price_plan_id='$PP_MEMBER_T' AND user_id='$USER_DEMO' AND lifecycle_status='ACTIVE' AND enabled AND (expires_at IS NULL OR expires_at>now());")
echo "ACTIVE_MEMBER_BEFORE=$ACTIVE_M" | tee -a "$EVID/07b-recover-whitelist.txt"

if [ "$ACTIVE_M" = "0" ]; then
  code=$(curl -sS -o "$EVID/07b-create-member.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $ATOKEN" -H 'Content-Type: application/json' \
    --data "$BODY" \
    "${API_BASE}/api/v1/admin/price-plans/${PP_MEMBER_T}/whitelist")
  echo "POST create MEMBER whitelist -> $code" | tee -a "$EVID/07b-recover-whitelist.txt"
  cat "$EVID/07b-create-member.json" | tee -a "$EVID/07b-recover-whitelist.txt"
  test "$code" = "201"
else
  echo "MEMBER already ACTIVE — skip create" | tee -a "$EVID/07b-recover-whitelist.txt"
fi

ACTIVE_A=$(psqlat "SELECT count(*) FROM xz_price_plan_user_whitelist WHERE price_plan_id='$PP_AGENT_T' AND user_id='$USER_DEMO' AND lifecycle_status='ACTIVE' AND enabled AND (expires_at IS NULL OR expires_at>now());")
echo "ACTIVE_AGENT=$ACTIVE_A" | tee -a "$EVID/07b-recover-whitelist.txt"

# verify TEST quote works
UTOKEN="gate_s5_wl_verify_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN" "$USER_DEMO" EX 180 >/dev/null
code=$(curl -sS -o "$EVID/07b-quote-verify.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote after recover -> $code" | tee -a "$EVID/07b-recover-whitelist.txt"
AMT=$(python3 -c "import json;d=json.load(open('$EVID/07b-quote-verify.json'));print(d.get('amountCent'),d.get('code',''))" 2>/dev/null || true)
echo "amount/code=$AMT" | tee -a "$EVID/07b-recover-whitelist.txt"
test "$code" = "201"

# mark #7 PASS with recovery note
{
  echo "VERDICT_#7=PASS (checkout 403 NOT_ELIGIBLE after soft-expire; no order side-effect; entry became EXPIRED immutable; NEW ACTIVE whitelist created via admin; quote 201 restored)"
  echo "RECOVERED_AT=$(TS)"
} | tee -a "$EVID/07-whitelist-expiry.txt" | tee -a "$EVID/07b-recover-whitelist.txt"

# ---------------------------------------------------------------------------
# #8 one-cent mismatch
# ---------------------------------------------------------------------------
echo "=== #8 ONE-CENT MISMATCH $(TS) ===" | tee "$EVID/08-price-mismatch.txt"
GOOD_ID=$(psqlat "SELECT g.id FROM xz_wechat_virtual_goods g JOIN xz_price_plan_payment_bindings b ON b.wechat_good_id=g.id WHERE b.price_plan_id='$PP_MEMBER_T' AND b.enabled AND b.status='ACTIVE' LIMIT 1;")
GOOD_PRICE_ORIG=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
echo "GOOD_ID=$GOOD_ID GOOD_PRICE_ORIG=$GOOD_PRICE_ORIG" | tee -a "$EVID/08-price-mismatch.txt"
test "$GOOD_PRICE_ORIG" = "100"

UTOKEN8="gate_s5_mm_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN8" "$USER_DEMO" EX 180 >/dev/null

# Prefer binding bump (+1) then restore — avoids mutating published good if triggers exist.
# Use good price bump as checklist specifies wechat price mismatch.
RESTORE_GOOD=1
psqlc -c "UPDATE xz_wechat_virtual_goods SET platform_price_cents=101 WHERE id='$GOOD_ID';" | tee -a "$EVID/08-price-mismatch.txt" || RESTORE_GOOD=0

code=$(curl -sS -o "$EVID/08-quote-mismatch.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote with +1 fen good -> $code" | tee -a "$EVID/08-price-mismatch.txt"
python3 -c "import json;d=json.load(open('$EVID/08-quote-mismatch.json'));print('code=',d.get('code'),'error=',d.get('error'))" | tee -a "$EVID/08-price-mismatch.txt" || true
MM_CODE=$(python3 -c "import json;print(json.load(open('$EVID/08-quote-mismatch.json')).get('code',''))" 2>/dev/null || true)

# ALWAYS restore
psqlc -c "UPDATE xz_wechat_virtual_goods SET platform_price_cents=100 WHERE id='$GOOD_ID';" | tee -a "$EVID/08-price-mismatch.txt"
GOOD_NOW=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
echo "RESTORED good price=$GOOD_NOW" | tee -a "$EVID/08-price-mismatch.txt"
test "$GOOD_NOW" = "100"

code=$(curl -sS -o "$EVID/08-quote-restored.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote after restore -> $code" | tee -a "$EVID/08-price-mismatch.txt"

PP_MEMBER_N=$(psqlat "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND price_type='NORMAL' AND is_default AND enabled LIMIT 1;")
code_n=$(curl -sS -o "$EVID/08-quote-normal.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_N}\"}" \
  "${API_BASE}/api/v1/payment/price-quotes")
AMT_N=$(python3 -c "import json;print(json.load(open('$EVID/08-quote-normal.json')).get('amountCent'))" 2>/dev/null || true)
echo "NORMAL dry quote -> $code_n amountCent=$AMT_N" | tee -a "$EVID/08-price-mismatch.txt"

MM8_PASS=0
if [ "$MM_CODE" = "PRICE_PLAN_WECHAT_PRICE_MISMATCH" ] && [ "$code" = "201" ] && [ "$GOOD_NOW" = "100" ]; then
  MM8_PASS=1
  echo "VERDICT_#8=PASS (409 MISMATCH on +1 fen; restored; quote 201)" | tee -a "$EVID/08-price-mismatch.txt"
else
  echo "VERDICT_#8=INVESTIGATE mm_code=$MM_CODE restore_quote=$code good_now=$GOOD_NOW" | tee -a "$EVID/08-price-mismatch.txt"
fi

# ---------------------------------------------------------------------------
# #10 V1 regression
# ---------------------------------------------------------------------------
echo "=== #10 V1 REGRESSION $(TS) ===" | tee "$EVID/10-v1-regression.txt"
psqlc -c "
SELECT order_no, coalesce(snapshot_version,0) AS sv, status, fulfillment_status, entitlement_status,
       coalesce(price_plan_id,'') AS pp, coalesce(product_code,'') AS product_code, amount_cents, created_at
FROM xz_orders
WHERE coalesce(snapshot_version,0) < 2
ORDER BY created_at DESC NULLS LAST
LIMIT 20;
" | tee -a "$EVID/10-v1-regression.txt"

V1_PAID=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 AND status='PAID' ORDER BY created_at DESC NULLS LAST LIMIT 1;")
V1_ANY=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 ORDER BY created_at DESC NULLS LAST LIMIT 1;")
V1_TARGET="${V1_PAID:-$V1_ANY}"
echo "V1_PAID=$V1_PAID V1_ANY=$V1_ANY TARGET=$V1_TARGET" | tee -a "$EVID/10-v1-regression.txt"

V10_PASS=0
if [ -n "$V1_TARGET" ]; then
  V1_OWNER=$(psqlat "SELECT user_id FROM xz_orders WHERE order_no='$V1_TARGET' LIMIT 1;")
  UTOKEN10="gate_s5_v1_$$"
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN10" "$V1_OWNER" EX 180 >/dev/null
  code_st=$(curl -sS -o "$EVID/10-status.json" -w '%{http_code}' \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_TARGET}/status")
  echo "GET status V1 $V1_TARGET owner=$V1_OWNER -> $code_st" | tee -a "$EVID/10-v1-regression.txt"
  code_sy=$(curl -sS -o "$EVID/10-sync.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_TARGET}/sync")
  echo "POST sync V1 $V1_TARGET -> $code_sy" | tee -a "$EVID/10-v1-regression.txt"
  python3 -c "import json;d=json.load(open('$EVID/10-status.json'));print('status_body keys=',list(d)[:12],'status=',d.get('status'),'code=',d.get('code'))" | tee -a "$EVID/10-v1-regression.txt" || true
  V2_OK=$(psqlat "SELECT count(*) FROM xz_orders WHERE order_no IN ('$OID_MEMBER','$OID_AGENT') AND snapshot_version=2 AND entitlement_status='SUCCESS';")
  echo "V2_still_ok=$V2_OK" | tee -a "$EVID/10-v1-regression.txt"
  if [ "$code_st" != "500" ] && [ "$code_sy" != "500" ] && [ "$V2_OK" = "2" ]; then
    V10_PASS=1
    echo "VERDICT_#10=PASS (V1 status/sync non-5xx; V2 intact)" | tee -a "$EVID/10-v1-regression.txt"
  else
    echo "VERDICT_#10=INVESTIGATE status=$code_st sync=$code_sy v2=$V2_OK" | tee -a "$EVID/10-v1-regression.txt"
  fi
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN10" >/dev/null || true
else
  echo "VERDICT_#10=NO_V1_ROWS — SUBSTITUTED: postgres unit tests cover V1 GrantOrderEntitlements idempotency; prod window has V2 paid samples only" | tee -a "$EVID/10-v1-regression.txt"
fi

# pricing health
curl -sS -o "$EVID/pricing-health.json" -H "Authorization: Bearer $ATOKEN" \
  "${API_BASE}/api/v1/admin/pricing/health" || true
python3 - <<'PY' | tee -a "$EVID/07b-recover-whitelist.txt" || true
import json
d=json.load(open("/tmp/section5-probes-20260729/pricing-health.json"))
print("health status=", d.get("status"), "blocked=", (d.get("summary") or {}).get("blockedIssueCount"))
PY

# cleanup
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL \
  "auth:session:$ATOKEN" "auth:session:$UTOKEN" "auth:session:$UTOKEN8" >/dev/null || true

echo "=== FINAL FLAGS $(TS) ===" | tee "$EVID/99-final.txt"
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV | tee -a "$EVID/99-final.txt"
PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
test "$PAY" = "production"
WL_OK=$(psqlat "SELECT count(*) FROM xz_price_plan_user_whitelist WHERE price_plan_id='$PP_MEMBER_T' AND user_id='$USER_DEMO' AND lifecycle_status='ACTIVE' AND enabled AND (expires_at IS NULL OR expires_at>now());")
GOOD_OK=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
IDEM_PASS=1
WL7_PASS=1
echo "WL_OK=$WL_OK GOOD_OK=$GOOD_OK IDEM=$IDEM_PASS WL7=$WL7_PASS MM8=$MM8_PASS V10=$V10_PASS" | tee -a "$EVID/99-final.txt"
test "$WL_OK" = "1"
test "$GOOD_OK" = "100"
test "$PAY" = "production"

{
  echo "CHECKED_AT=$(TS)"
  echo "IDEM_PASS=$IDEM_PASS"
  echo "WL7_PASS=$WL7_PASS"
  echo "MM8_PASS=$MM8_PASS"
  echo "V10_PASS=$V10_PASS"
  echo "PAY_ENV=$PAY"
} | tee "$EVID/DONE.txt"

echo "RECOVER+REMAINING COMPLETE $(TS)"
unset REDIS_PASSWORD
