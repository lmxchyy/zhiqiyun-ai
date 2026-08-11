#!/usr/bin/env bash
# §5 remaining probes: #5 idempotency, #7 whitelist-after-quote, #8 1-cent mismatch, #10 V1 regression
# Safe: no ¥996 charge; no permanent pay-env flip; restore whitelist/good prices; no --build/retag.
set -euo pipefail

EVID="${EVID:-/tmp/section5-probes-20260729}"
mkdir -p "$EVID"
TS() { date '+%Y-%m-%d %H:%M:%S %z'; }
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
API_BASE=http://127.0.0.1:3100
OID_MEMBER=ZQY202607282159389857812495
OID_AGENT=ZQY20260728221656E339AB7A54
USER_DEMO=user_000002

db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
REDIS_PASSWORD=$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)

psqlc() { docker exec "$PG_C" psql -U "$u" -d "$db" "$@"; }
psqlat() { docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "$1"; }

echo "=== PRECHECK $(TS) ===" | tee "$EVID/00-precheck.txt"
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV | tee -a "$EVID/00-precheck.txt"
docker inspect --format 'health={{.State.Health.Status}} image={{.Image}}' "$APP_C" | tee -a "$EVID/00-precheck.txt"

# ---------------------------------------------------------------------------
# #5 — Idempotency: ledger uniqueness + re-grant / re-sync must not double
# ---------------------------------------------------------------------------
echo "=== #5 IDEMPOTENCY BEFORE $(TS) ===" | tee "$EVID/05-idempotency.txt"
psqlc -c "
SELECT o.order_no, coalesce(o.wechat_product_id_snapshot,o.product_code,'') AS product,
  (SELECT count(*) FROM xz_membership_entitlement_records m WHERE m.source_order_no=o.order_no) AS memb_n,
  (SELECT count(DISTINCT idempotency_key) FROM xz_membership_entitlement_records m WHERE m.source_order_no=o.order_no) AS memb_keys,
  (SELECT count(*) FROM xz_token_records t WHERE t.source_order_no=o.order_no OR t.order_id=o.order_no) AS tok_n,
  (SELECT count(DISTINCT idempotency_key) FROM xz_token_records t WHERE t.source_order_no=o.order_no OR t.order_id=o.order_no) AS tok_keys,
  (SELECT count(*) FROM xz_identity_change_records i WHERE i.user_id=o.user_id AND (i.idempotency_key LIKE '%'||o.order_no||'%' OR CAST(i.reason AS text) LIKE '%'||o.order_no||'%')) AS ident_n,
  (SELECT count(*) FROM xz_payment_events e WHERE e.order_id=o.order_no AND e.event_type='query_order_paid') AS qop_n,
  (SELECT count(*) FROM xz_payment_events e WHERE e.order_id=o.order_no AND e.event_type='notify_provide_goods') AS npg_n,
  o.fulfillment_status, o.entitlement_status, o.snapshot_version
FROM xz_orders o
WHERE o.order_no IN ('$OID_MEMBER','$OID_AGENT')
ORDER BY o.order_no;
" | tee -a "$EVID/05-idempotency.txt"

# capture balances / row counts before re-sync
# MEMBER → 1 membership + 1 token; AGENT → 0 membership + 1 token (+ identity)
TOK_BEFORE=$(psqlat "SELECT coalesce(available,0) FROM xz_point_accounts WHERE user_id='$USER_DEMO' LIMIT 1;")
MEMB_M=$(psqlat "SELECT count(*) FROM xz_membership_entitlement_records WHERE source_order_no='$OID_MEMBER';")
MEMB_A=$(psqlat "SELECT count(*) FROM xz_membership_entitlement_records WHERE source_order_no='$OID_AGENT';")
TOK_M=$(psqlat "SELECT count(*) FROM xz_token_records WHERE source_order_no='$OID_MEMBER' OR order_id='$OID_MEMBER';")
TOK_A=$(psqlat "SELECT count(*) FROM xz_token_records WHERE source_order_no='$OID_AGENT' OR order_id='$OID_AGENT';")
echo "TOK_BEFORE=$TOK_BEFORE MEMB_M=$MEMB_M MEMB_A=$MEMB_A TOK_M=$TOK_M TOK_A=$TOK_A" | tee -a "$EVID/05-idempotency.txt"

UTOKEN="gate_s5_sync_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN" "$USER_DEMO" EX 180 >/dev/null

for OID in "$OID_MEMBER" "$OID_AGENT"; do
  code=$(curl -sS -o "$EVID/05-sync-$OID.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN" \
    "${API_BASE}/api/v1/payment/orders/${OID}/sync")
  echo "POST sync $OID -> $code" | tee -a "$EVID/05-idempotency.txt"
  # second sync (concurrent-ish sequential)
  code2=$(curl -sS -o "$EVID/05-sync2-$OID.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN" \
    "${API_BASE}/api/v1/payment/orders/${OID}/sync")
  echo "POST sync2 $OID -> $code2" | tee -a "$EVID/05-idempotency.txt"
done

sleep 1
echo "=== #5 IDEMPOTENCY AFTER $(TS) ===" | tee -a "$EVID/05-idempotency.txt"
psqlc -c "
SELECT o.order_no,
  (SELECT count(*) FROM xz_membership_entitlement_records m WHERE m.source_order_no=o.order_no) AS memb_n,
  (SELECT count(*) FROM xz_token_records t WHERE t.source_order_no=o.order_no OR t.order_id=o.order_no) AS tok_n,
  (SELECT count(*) FROM xz_payment_events e WHERE e.order_id=o.order_no AND e.event_type='query_order_paid') AS qop_n,
  o.fulfillment_status, o.entitlement_status,
  (SELECT available FROM xz_point_accounts WHERE user_id=o.user_id LIMIT 1) AS token_bal
FROM xz_orders o
WHERE o.order_no IN ('$OID_MEMBER','$OID_AGENT')
ORDER BY o.order_no;
" | tee -a "$EVID/05-idempotency.txt"

TOK_AFTER=$(psqlat "SELECT coalesce(available,0) FROM xz_point_accounts WHERE user_id='$USER_DEMO' LIMIT 1;")
MEMB_M2=$(psqlat "SELECT count(*) FROM xz_membership_entitlement_records WHERE source_order_no='$OID_MEMBER';")
MEMB_A2=$(psqlat "SELECT count(*) FROM xz_membership_entitlement_records WHERE source_order_no='$OID_AGENT';")
TOK_M2=$(psqlat "SELECT count(*) FROM xz_token_records WHERE source_order_no='$OID_MEMBER' OR order_id='$OID_MEMBER';")
TOK_A2=$(psqlat "SELECT count(*) FROM xz_token_records WHERE source_order_no='$OID_AGENT' OR order_id='$OID_AGENT';")
echo "TOK_AFTER=$TOK_AFTER MEMB_M2=$MEMB_M2 MEMB_A2=$MEMB_A2 TOK_M2=$TOK_M2 TOK_A2=$TOK_A2" | tee -a "$EVID/05-idempotency.txt"

IDEM_PASS=0
if [ "$TOK_BEFORE" = "$TOK_AFTER" ] \
   && [ "$MEMB_M" = "$MEMB_M2" ] && [ "$MEMB_A" = "$MEMB_A2" ] \
   && [ "$TOK_M" = "$TOK_M2" ] && [ "$TOK_A" = "$TOK_A2" ] \
   && [ "$MEMB_M" = "1" ] && [ "$MEMB_A" = "0" ] \
   && [ "$TOK_M" = "1" ] && [ "$TOK_A" = "1" ]; then
  IDEM_PASS=1
  echo "VERDICT_#5=PASS (re-sync×2 no double grant; MEMBER memb=1/tok=1; AGENT memb=0/tok=1)" | tee -a "$EVID/05-idempotency.txt"
else
  echo "VERDICT_#5=FAIL_OR_INVESTIGATE tok=$TOK_BEFORE/$TOK_AFTER membM=$MEMB_M/$MEMB_M2 membA=$MEMB_A/$MEMB_A2 tokM=$TOK_M/$TOK_M2 tokA=$TOK_A/$TOK_A2" | tee -a "$EVID/05-idempotency.txt"
fi

# ---------------------------------------------------------------------------
# #7 — quote then whitelist soft-expire → checkout reject; restore whitelist
# ---------------------------------------------------------------------------
echo "=== #7 WHITELIST EXPIRY AFTER QUOTE $(TS) ===" | tee "$EVID/07-whitelist-expiry.txt"
PP_MEMBER_T=$(psqlat "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND price_type='TEST' AND enabled LIMIT 1;")
WL_ID=$(psqlat "SELECT id FROM xz_price_plan_user_whitelist WHERE price_plan_id='$PP_MEMBER_T' AND user_id='$USER_DEMO' AND lifecycle_status='ACTIVE' ORDER BY created_at DESC LIMIT 1;")
WL_EXPIRES_ORIG=$(psqlat "SELECT coalesce(to_char(expires_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"'),'') FROM xz_price_plan_user_whitelist WHERE id='$WL_ID';")
WL_REV=$(psqlat "SELECT revision FROM xz_price_plan_user_whitelist WHERE id='$WL_ID';")
echo "PP_MEMBER_T=$PP_MEMBER_T WL_ID=$WL_ID WL_EXPIRES_ORIG=$WL_EXPIRES_ORIG WL_REV=$WL_REV" | tee -a "$EVID/07-whitelist-expiry.txt"

# plant auth + fake wechat session so createOrder passes session gate
UTOKEN7="gate_s5_wl_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN7" "$USER_DEMO" EX 300 >/dev/null
# fake openid/sessionKey — order creation signs locally; we expect reject BEFORE insert
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET \
  "auth:wechat-session:$USER_DEMO" \
  '{"openid":"probe_s5_fake_openid","sessionKey":"probe_s5_fake_session_key"}' EX 300 >/dev/null

# issue TEST quote while whitelist ACTIVE
code=$(curl -sS -o "$EVID/07-quote-before.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN7" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote while ACTIVE -> $code" | tee -a "$EVID/07-whitelist-expiry.txt"
QID=$(python3 -c "import json; print(json.load(open('$EVID/07-quote-before.json')).get('quoteId',''))" 2>/dev/null || true)
AMOUNT=$(python3 -c "import json; d=json.load(open('$EVID/07-quote-before.json')); print(d.get('amountCent'), d.get('testOnly'))" 2>/dev/null || true)
echo "quoteId_len=${#QID} amount/testOnly=$AMOUNT" | tee -a "$EVID/07-whitelist-expiry.txt"
test "$code" = "201"
test -n "$QID"

ORDERS_BEFORE=$(psqlat "SELECT count(*) FROM xz_orders WHERE user_id='$USER_DEMO';")

# soft-expire whitelist (MUST restore)
psqlc -c "UPDATE xz_price_plan_user_whitelist SET expires_at=now()-interval '2 seconds' WHERE id='$WL_ID';" | tee -a "$EVID/07-whitelist-expiry.txt"
echo "soft-expired WL_ID=$WL_ID" | tee -a "$EVID/07-whitelist-expiry.txt"

# checkout with pinned quote → expect 403 PRICE_PLAN_NOT_ELIGIBLE
code=$(curl -sS -o "$EVID/07-order-after-expire.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN7" -H 'Content-Type: application/json' \
  --data "{\"quoteId\":\"${QID}\"}" \
  "${API_BASE}/api/v1/payment/wechat-virtual/orders")
echo "checkout after expire -> $code" | tee -a "$EVID/07-whitelist-expiry.txt"
python3 - <<PY | tee -a "$EVID/07-whitelist-expiry.txt" || true
import json
d=json.load(open("$EVID/07-order-after-expire.json"))
print("code=", d.get("code"), "error=", d.get("error") or d.get("message"))
PY

ORDERS_AFTER=$(psqlat "SELECT count(*) FROM xz_orders WHERE user_id='$USER_DEMO';")
QSTATUS=$(psqlat "SELECT status FROM xz_order_price_quotes WHERE quote_token_hash=encode(digest(convert_to('$QID','UTF8'),'sha256'),'hex') LIMIT 1;" 2>/dev/null || \
  psqlat "SELECT status FROM xz_order_price_quotes WHERE id LIKE 'quote_%' AND user_id='$USER_DEMO' AND status='AVAILABLE' AND entry_type='TEST' ORDER BY created_at DESC LIMIT 1;")
# better: lookup via recent AVAILABLE TEST quote for user
QSTATUS=$(psqlat "SELECT status FROM xz_order_price_quotes WHERE user_id='$USER_DEMO' AND entry_type='TEST' AND created_at > now()-interval '10 minutes' ORDER BY created_at DESC LIMIT 1;")
echo "orders_before=$ORDERS_BEFORE orders_after=$ORDERS_AFTER recent_quote_status=$QSTATUS" | tee -a "$EVID/07-whitelist-expiry.txt"

# new TEST quote while expired → also 403
code=$(curl -sS -o "$EVID/07-quote-while-expired.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN7" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote while EXPIRED -> $code" | tee -a "$EVID/07-whitelist-expiry.txt"
python3 -c "import json;d=json.load(open('$EVID/07-quote-while-expired.json'));print('code=',d.get('code'))" | tee -a "$EVID/07-whitelist-expiry.txt" || true

# RESTORE whitelist expires_at
if [ -n "$WL_EXPIRES_ORIG" ]; then
  psqlc -c "UPDATE xz_price_plan_user_whitelist SET expires_at='$WL_EXPIRES_ORIG'::timestamptz WHERE id='$WL_ID';" | tee -a "$EVID/07-whitelist-expiry.txt"
else
  psqlc -c "UPDATE xz_price_plan_user_whitelist SET expires_at=now()+interval '365 days' WHERE id='$WL_ID';" | tee -a "$EVID/07-whitelist-expiry.txt"
fi
WL_EXPIRES_NOW=$(psqlat "SELECT expires_at, lifecycle_status, enabled FROM xz_price_plan_user_whitelist WHERE id='$WL_ID';")
echo "RESTORED whitelist: $WL_EXPIRES_NOW" | tee -a "$EVID/07-whitelist-expiry.txt"

# verify quote works again after restore
code=$(curl -sS -o "$EVID/07-quote-after-restore.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN7" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote after restore -> $code" | tee -a "$EVID/07-whitelist-expiry.txt"

WL7_PASS=0
ORDER_CODE=$(python3 -c "import json;print(json.load(open('$EVID/07-order-after-expire.json')).get('code',''))" 2>/dev/null || true)
if [ "$ORDER_CODE" = "PRICE_PLAN_NOT_ELIGIBLE" ] && [ "$ORDERS_BEFORE" = "$ORDERS_AFTER" ] && [ "$code" = "201" ]; then
  WL7_PASS=1
  echo "VERDICT_#7=PASS (checkout 403 NOT_ELIGIBLE; no order side-effect; whitelist restored; quote 201)" | tee -a "$EVID/07-whitelist-expiry.txt"
else
  echo "VERDICT_#7=INVESTIGATE order_code=$ORDER_CODE orders=$ORDERS_BEFORE/$ORDERS_AFTER restore_quote=$code" | tee -a "$EVID/07-whitelist-expiry.txt"
fi

# cleanup sessions
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL \
  "auth:session:$UTOKEN" "auth:session:$UTOKEN7" "auth:wechat-session:$USER_DEMO" >/dev/null || true

# ---------------------------------------------------------------------------
# #8 — 1-cent mismatch on TEST good → PRICE_PLAN_WECHAT_PRICE_MISMATCH; restore
# ---------------------------------------------------------------------------
echo "=== #8 ONE-CENT MISMATCH $(TS) ===" | tee "$EVID/08-price-mismatch.txt"
GOOD_ID=$(psqlat "SELECT g.id FROM xz_wechat_virtual_goods g JOIN xz_price_plan_payment_bindings b ON b.wechat_good_id=g.id WHERE b.price_plan_id='$PP_MEMBER_T' AND b.enabled AND b.status='ACTIVE' LIMIT 1;")
GOOD_PRICE_ORIG=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
BIND_PRICE=$(psqlat "SELECT provider_price_snapshot_cents FROM xz_price_plan_payment_bindings WHERE price_plan_id='$PP_MEMBER_T' AND enabled AND status='ACTIVE' LIMIT 1;")
PLAN_PRICE=$(psqlat "SELECT sale_price_cents FROM xz_price_plans WHERE id='$PP_MEMBER_T';")
echo "GOOD_ID=$GOOD_ID GOOD_PRICE_ORIG=$GOOD_PRICE_ORIG BIND=$BIND_PRICE PLAN=$PLAN_PRICE" | tee -a "$EVID/08-price-mismatch.txt"
test "$GOOD_PRICE_ORIG" = "100"

UTOKEN8="gate_s5_mm_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN8" "$USER_DEMO" EX 180 >/dev/null

# bump good +1 fen
psqlc -c "UPDATE xz_wechat_virtual_goods SET platform_price_cents=101 WHERE id='$GOOD_ID';" | tee -a "$EVID/08-price-mismatch.txt"

code=$(curl -sS -o "$EVID/08-quote-mismatch.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote with +1 fen good -> $code" | tee -a "$EVID/08-price-mismatch.txt"
python3 -c "import json;d=json.load(open('$EVID/08-quote-mismatch.json'));print('code=',d.get('code'),'error=',d.get('error'))" | tee -a "$EVID/08-price-mismatch.txt" || true
MM_CODE=$(python3 -c "import json;print(json.load(open('$EVID/08-quote-mismatch.json')).get('code',''))" 2>/dev/null || true)

# RESTORE immediately
psqlc -c "UPDATE xz_wechat_virtual_goods SET platform_price_cents=$GOOD_PRICE_ORIG WHERE id='$GOOD_ID';" | tee -a "$EVID/08-price-mismatch.txt"
GOOD_NOW=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
echo "RESTORED good price=$GOOD_NOW" | tee -a "$EVID/08-price-mismatch.txt"
test "$GOOD_NOW" = "$GOOD_PRICE_ORIG"

# verify quote OK after restore
code=$(curl -sS -o "$EVID/08-quote-restored.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
  "${API_BASE}/api/v1/payment/test-price-quotes")
echo "TEST quote after restore -> $code" | tee -a "$EVID/08-price-mismatch.txt"

# also confirm NORMAL dry quote still 99600 (no charge)
PP_MEMBER_N=$(psqlat "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND price_type='NORMAL' AND is_default AND enabled LIMIT 1;")
code_n=$(curl -sS -o "$EVID/08-quote-normal.json" -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $UTOKEN8" -H 'Content-Type: application/json' \
  --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_N}\"}" \
  "${API_BASE}/api/v1/payment/price-quotes")
AMT_N=$(python3 -c "import json;print(json.load(open('$EVID/08-quote-normal.json')).get('amountCent'))" 2>/dev/null || true)
echo "NORMAL dry quote -> $code_n amountCent=$AMT_N" | tee -a "$EVID/08-price-mismatch.txt"

docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN8" >/dev/null || true

MM8_PASS=0
if [ "$MM_CODE" = "PRICE_PLAN_WECHAT_PRICE_MISMATCH" ] && [ "$code" = "201" ] && [ "$GOOD_NOW" = "100" ]; then
  MM8_PASS=1
  echo "VERDICT_#8=PASS (409 MISMATCH on +1 fen; restored; quote 201)" | tee -a "$EVID/08-price-mismatch.txt"
else
  echo "VERDICT_#8=INVESTIGATE mm_code=$MM_CODE restore_quote=$code good_now=$GOOD_NOW" | tee -a "$EVID/08-price-mismatch.txt"
fi

# ---------------------------------------------------------------------------
# #10 — V1 historical orders still readable / syncable without mutating V2
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

V1_SAMPLE=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 AND status IN ('PAID','FULFILLED','SUCCESS','COMPLETED','pending','PENDING','PAID_PENDING') ORDER BY created_at DESC NULLS LAST LIMIT 1;")
# prefer a paid V1 virtual order if any
V1_PAID=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 AND status='PAID' ORDER BY created_at DESC NULLS LAST LIMIT 1;")
V1_ANY=$(psqlat "SELECT order_no FROM xz_orders WHERE coalesce(snapshot_version,0) < 2 ORDER BY created_at DESC NULLS LAST LIMIT 1;")
echo "V1_PAID=$V1_PAID V1_ANY=$V1_ANY" | tee -a "$EVID/10-v1-regression.txt"

UTOKEN10="gate_s5_v1_$$"
# use order owner if possible
V1_TARGET="${V1_PAID:-$V1_ANY}"
V1_OWNER=$(psqlat "SELECT user_id FROM xz_orders WHERE order_no='$V1_TARGET' LIMIT 1;" 2>/dev/null || true)
if [ -z "$V1_OWNER" ]; then V1_OWNER=$USER_DEMO; fi
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN10" "$V1_OWNER" EX 180 >/dev/null

V10_PASS=0
if [ -n "$V1_TARGET" ]; then
  code_st=$(curl -sS -o "$EVID/10-status-$V1_TARGET.json" -w '%{http_code}' \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_TARGET}/status")
  echo "GET status V1 $V1_TARGET owner=$V1_OWNER -> $code_st" | tee -a "$EVID/10-v1-regression.txt"
  code_sy=$(curl -sS -o "$EVID/10-sync-$V1_TARGET.json" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $UTOKEN10" \
    "${API_BASE}/api/v1/payment/orders/${V1_TARGET}/sync")
  echo "POST sync V1 $V1_TARGET -> $code_sy" | tee -a "$EVID/10-v1-regression.txt"
  # V2 orders still intact
  V2_OK=$(psqlat "SELECT count(*) FROM xz_orders WHERE order_no IN ('$OID_MEMBER','$OID_AGENT') AND snapshot_version=2 AND entitlement_status='SUCCESS';")
  echo "V2_still_ok=$V2_OK" | tee -a "$EVID/10-v1-regression.txt"
  # productCode legacy path for NON-managed product should still be accepted or clear error (not crash)
  # Use a known V1 token product if present; else just record status/sync evidence
  if [ "$code_st" = "200" ] || [ "$code_st" = "404" ] || [ "$code_sy" = "200" ] || [ "$code_sy" = "409" ] || [ "$code_sy" = "400" ]; then
    # 200 status/sync = usable; non-5xx = branch alive
    if [ "$code_st" != "500" ] && [ "$code_sy" != "500" ] && [ "$V2_OK" = "2" ]; then
      V10_PASS=1
      echo "VERDICT_#10=PASS (V1 order status/sync reachable non-5xx; V2 rows intact)" | tee -a "$EVID/10-v1-regression.txt"
    fi
  fi
  if [ "$V10_PASS" != "1" ]; then
    echo "VERDICT_#10=INVESTIGATE status=$code_st sync=$code_sy v2=$V2_OK" | tee -a "$EVID/10-v1-regression.txt"
  fi
else
  echo "VERDICT_#10=NO_V1_ROWS — mark SUBSTITUTED: unit tests cover V1 grant path; prod has only V2 paid samples in window" | tee -a "$EVID/10-v1-regression.txt"
  V10_PASS=0
fi

docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN10" >/dev/null || true

# final flags check — must still be production
echo "=== FINAL FLAGS $(TS) ===" | tee "$EVID/99-final.txt"
docker exec "$APP_C" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV | tee -a "$EVID/99-final.txt"
PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
test "$PAY" = "production"

# restore sanity: TEST whitelist ACTIVE + good=100
WL_OK=$(psqlat "SELECT count(*) FROM xz_price_plan_user_whitelist WHERE id='$WL_ID' AND lifecycle_status='ACTIVE' AND enabled AND expires_at>now();")
GOOD_OK=$(psqlat "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE id='$GOOD_ID';")
echo "WL_OK=$WL_OK GOOD_OK=$GOOD_OK IDEM=$IDEM_PASS WL7=$WL7_PASS MM8=$MM8_PASS V10=$V10_PASS" | tee -a "$EVID/99-final.txt"
test "$WL_OK" = "1"
test "$GOOD_OK" = "100"

{
  echo "CHECKED_AT=$(TS)"
  echo "IDEM_PASS=$IDEM_PASS"
  echo "WL7_PASS=$WL7_PASS"
  echo "MM8_PASS=$MM8_PASS"
  echo "V10_PASS=$V10_PASS"
  echo "PAY_ENV=$PAY"
} | tee "$EVID/DONE.txt"

echo "ALL PROBES COMPLETED $(TS)"
unset REDIS_PASSWORD
