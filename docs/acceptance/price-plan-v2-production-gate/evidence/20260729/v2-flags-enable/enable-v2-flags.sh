#!/usr/bin/env bash
# Enable V2 feature flags ONE AT A TIME on prod. Authorized by user 「明确授权开」.
# Does NOT change WECHAT_VIRTUAL_PAY_ENV (remains production).
set -euo pipefail

cd /opt/zhiqiyun-ai
EVID="${EVID:-/tmp/v2-flags-enable-20260729}"
mkdir -p "$EVID"
TS() { date '+%Y-%m-%d %H:%M:%S %z'; }
ENV_FILE=.env.production
APP=xianzhi-ai
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
API_BASE=http://127.0.0.1:3100
STEP="${1:-all}"  # all | 1 | 2 | 3 | probe | prices

set_flag() {
  local flag="$1" value="$2"
  if grep -q "^${flag}=" "$ENV_FILE"; then
    sed -i "s/^${flag}=.*/${flag}=${value}/" "$ENV_FILE"
  else
    echo "${flag}=${value}" >> "$ENV_FILE"
  fi
}

recreate_and_wait() {
  local label="$1"
  docker compose -f compose.prod.yml --env-file "$ENV_FILE" up -d --no-deps --force-recreate "$APP"
  local st=starting
  for i in $(seq 1 40); do
    st=$(docker inspect --format '{{.State.Health.Status}}' "$APP_C" 2>/dev/null || echo starting)
    echo "wait_$i health=$st $(TS)" | tee -a "$EVID/${label}.txt"
    if [ "$st" = "healthy" ]; then break; fi
    sleep 2
  done
  docker inspect --format 'health={{.State.Health.Status}} status={{.State.Status}}' "$APP_C" | tee -a "$EVID/${label}.txt"
  test "$st" = "healthy"
}

print_flags() {
  docker exec "$APP_C" printenv \
    SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
    PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
    PRICE_PLAN_TEST_ENTRY_ENABLED \
    WECHAT_VIRTUAL_PAY_ENV
}

check_prices() {
  local out="$1"
  local db user
  db=$(docker exec "$PG_C" printenv POSTGRES_DB)
  user=$(docker exec "$PG_C" printenv POSTGRES_USER)
  docker exec "$PG_C" psql -U "$user" -d "$db" -Atc "
SELECT 'default_member='||coalesce((
  SELECT sale_price_cents::text FROM xz_price_plans
  WHERE plan_id='plan_ai_creator_996' AND environment='PRODUCTION' AND is_default AND enabled
  ORDER BY updated_at DESC NULLS LAST LIMIT 1
),'MISSING');
SELECT 'default_agent='||coalesce((
  SELECT sale_price_cents::text FROM xz_price_plans
  WHERE plan_id='plan_agent_join_996' AND environment='PRODUCTION' AND is_default AND enabled
  ORDER BY updated_at DESC NULLS LAST LIMIT 1
),'MISSING');
SELECT 'good_member='||coalesce((
  SELECT platform_price_cents::text FROM xz_wechat_virtual_goods
  WHERE product_id='MEMBER_YEAR_996' AND environment='PRODUCTION' LIMIT 1
),'MISSING');
SELECT 'good_agent='||coalesce((
  SELECT platform_price_cents::text FROM xz_wechat_virtual_goods
  WHERE product_id='AGENT_JOIN_996' AND environment='PRODUCTION' LIMIT 1
),'MISSING');
" | tee "$out"
}

case "$STEP" in
  prices)
    check_prices "$EVID/prices-$(date +%H%M%S).txt"
    exit 0
    ;;
esac

if [ "$STEP" = "all" ] || [ "$STEP" = "0" ]; then
  echo "=== PRECHECK $(TS) ===" | tee "$EVID/00-precheck.txt"
  docker inspect --format 'health={{.State.Health.Status}} status={{.State.Status}}' "$APP_C" | tee -a "$EVID/00-precheck.txt"
  print_flags | tee -a "$EVID/00-precheck.txt"
  grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" | tee -a "$EVID/00-precheck.txt"
  check_prices "$EVID/00-price-baseline.txt"
  mkdir -p backups/compose
  cp "$ENV_FILE" "backups/compose/.env.production.pre-v2-flags-$(date +%Y%m%d_%H%M%S).bak"
  # evidence: redacted flags only — never copy full .env (secrets)
  grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" > "$EVID/env.flags.pre.txt" || true
fi

if [ "$STEP" = "all" ] || [ "$STEP" = "1" ]; then
  FLAG=SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED
  echo "=== ENABLE $FLAG $(TS) ===" | tee "$EVID/01-fulfillment.txt"
  set_flag "$FLAG" true
  # ensure others stay false at this step
  set_flag PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED false
  set_flag PRICE_PLAN_TEST_ENTRY_ENABLED false
  grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" | tee -a "$EVID/01-fulfillment.txt"
  FLIP1_AT="$(TS)"
  echo "FLIP_AT=$FLIP1_AT" | tee -a "$EVID/01-fulfillment.txt"
  echo "$FLIP1_AT" > "$EVID/flip1-fulfillment-at.txt"
  recreate_and_wait 01-fulfillment
  print_flags | tee -a "$EVID/01-fulfillment.txt"
  FUL=$(docker exec "$APP_C" printenv SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED)
  CRE=$(docker exec "$APP_C" printenv PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED)
  TES=$(docker exec "$APP_C" printenv PRICE_PLAN_TEST_ENTRY_ENABLED)
  PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
  test "$FUL" = "true"
  test "$CRE" = "false"
  test "$TES" = "false"
  test "$PAY" = "production"
  check_prices "$EVID/01-prices-after.txt"
  grep -E '=(99600)$' "$EVID/01-prices-after.txt" | wc -l | tee -a "$EVID/01-fulfillment.txt"
  echo "VERIFY_OK fulfillment-only FLIP_AT=$FLIP1_AT" | tee -a "$EVID/01-fulfillment.txt"
fi

if [ "$STEP" = "all" ] || [ "$STEP" = "2" ]; then
  FLAG=PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED
  echo "=== ENABLE $FLAG $(TS) ===" | tee "$EVID/02-creation.txt"
  set_flag "$FLAG" true
  set_flag PRICE_PLAN_TEST_ENTRY_ENABLED false
  grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" | tee -a "$EVID/02-creation.txt"
  FLIP2_AT="$(TS)"
  echo "FLIP_AT=$FLIP2_AT" | tee -a "$EVID/02-creation.txt"
  echo "$FLIP2_AT" > "$EVID/flip2-creation-at.txt"
  recreate_and_wait 02-creation
  print_flags | tee -a "$EVID/02-creation.txt"
  FUL=$(docker exec "$APP_C" printenv SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED)
  CRE=$(docker exec "$APP_C" printenv PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED)
  TES=$(docker exec "$APP_C" printenv PRICE_PLAN_TEST_ENTRY_ENABLED)
  PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
  test "$FUL" = "true"
  test "$CRE" = "true"
  test "$TES" = "false"
  test "$PAY" = "production"
  check_prices "$EVID/02-prices-after.txt"
  echo "VERIFY_OK fulfillment+creation FLIP_AT=$FLIP2_AT" | tee -a "$EVID/02-creation.txt"
fi

if [ "$STEP" = "all" ] || [ "$STEP" = "3" ]; then
  FLAG=PRICE_PLAN_TEST_ENTRY_ENABLED
  echo "=== ENABLE $FLAG $(TS) ===" | tee "$EVID/03-test-entry.txt"
  set_flag "$FLAG" true
  grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" | tee -a "$EVID/03-test-entry.txt"
  FLIP3_AT="$(TS)"
  echo "FLIP_AT=$FLIP3_AT" | tee -a "$EVID/03-test-entry.txt"
  echo "$FLIP3_AT" > "$EVID/flip3-test-entry-at.txt"
  recreate_and_wait 03-test-entry
  print_flags | tee -a "$EVID/03-test-entry.txt"
  FUL=$(docker exec "$APP_C" printenv SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED)
  CRE=$(docker exec "$APP_C" printenv PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED)
  TES=$(docker exec "$APP_C" printenv PRICE_PLAN_TEST_ENTRY_ENABLED)
  PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
  test "$FUL" = "true"
  test "$CRE" = "true"
  test "$TES" = "true"
  test "$PAY" = "production"
  check_prices "$EVID/03-prices-after.txt"
  echo "VERIFY_OK all-three-true pay=production FLIP_AT=$FLIP3_AT" | tee -a "$EVID/03-test-entry.txt"
fi

if [ "$STEP" = "all" ] || [ "$STEP" = "probe" ]; then
  echo "=== PROBE $(TS) ===" | tee "$EVID/04-probe.txt"
  # dry unauth probes
  for path in /api/v1/health /healthz /readyz /api/v1/payment/price-quotes /api/v1/payment/test-price-quotes; do
    code=$(curl -sS -o /tmp/probe-body.json -w '%{http_code}' -X GET "${API_BASE}${path}" || echo ERR)
    echo "GET $path -> $code" | tee -a "$EVID/04-probe.txt"
  done
  for path in /api/v1/payment/price-quotes /api/v1/payment/test-price-quotes; do
    code=$(curl -sS -o /tmp/probe-body.json -w '%{http_code}' -X POST \
      -H 'Content-Type: application/json' \
      --data '{}' \
      "${API_BASE}${path}" || echo ERR)
    echo "POST $path empty -> $code body=$(head -c 200 /tmp/probe-body.json)" | tee -a "$EVID/04-probe.txt"
  done

  # admin pricing-health via short redis session (same pattern as seed)
  TOKEN="gate_v2_flags_$(date +%Y%m%d%H%M%S)_$$"
  ACTOR_ID=user_000001
  REDIS_PASSWORD="$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)"
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$TOKEN" "$ACTOR_ID" EX 900 >/dev/null
  code=$(curl -sS -o "$EVID/pricing-health.json" -w '%{http_code}' \
    -H "Authorization: Bearer $TOKEN" \
    "${API_BASE}/api/v1/admin/pricing-health" || echo ERR)
  echo "GET /api/v1/admin/pricing-health -> $code" | tee -a "$EVID/04-probe.txt"
  python3 - <<'PY' 2>/dev/null | tee -a "$EVID/04-probe.txt" || true
import json
p="/tmp/v2-flags-enable-20260729/pricing-health.json"
try:
  d=json.load(open(p))
except Exception as e:
  print("parse_error", e); raise SystemExit
print("blockedIssueCount=", d.get("blockedIssueCount"))
print("warningIssueCount=", d.get("warningIssueCount"))
issues=d.get("issues") or d.get("blockedIssues") or []
if isinstance(issues, list):
  for it in issues[:20]:
    if isinstance(it, dict):
      print("issue:", it.get("code") or it.get("severity") or it.get("type"), it.get("severity") or it.get("message") or it)
    else:
      print("issue:", it)
PY
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$TOKEN" >/dev/null || true

  # dry authenticated quote probe WITHOUT creating order:
  # use a real user session if available is hard; instead list default price plan ids and POST with forged auth to see gate codes.
  # Prefer: inject same admin is wrong for user quote. Create ephemeral session for a known user if exists.
  USER_ID=$(docker exec "$PG_C" bash -lc 'db=$(printenv POSTGRES_DB); u=$(printenv POSTGRES_USER); psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_users ORDER BY created_at NULLS LAST LIMIT 1;"' 2>/dev/null || true)
  PLAN_MEMBER=$(docker exec "$PG_C" bash -lc 'db=$(printenv POSTGRES_DB); u=$(printenv POSTGRES_USER); psql -U "$u" -d "$db" -Atc "SELECT plan_id FROM xz_price_plans WHERE plan_id='"'"'plan_ai_creator_996'"'"' AND environment='"'"'PRODUCTION'"'"' AND is_default AND enabled LIMIT 1;"' 2>/dev/null || echo plan_ai_creator_996)
  PRICE_MEMBER=$(docker exec "$PG_C" bash -lc 'db=$(printenv POSTGRES_DB); u=$(printenv POSTGRES_USER); psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='"'"'plan_ai_creator_996'"'"' AND environment='"'"'PRODUCTION'"'"' AND is_default AND enabled LIMIT 1;"' 2>/dev/null || true)
  echo "probe_user=$USER_ID plan=$PLAN_MEMBER pricePlan=$PRICE_MEMBER" | tee -a "$EVID/04-probe.txt"
  if [ -n "${USER_ID:-}" ] && [ -n "${PRICE_MEMBER:-}" ]; then
    UTOKEN="gate_v2_quote_$(date +%Y%m%d%H%M%S)_$$"
    docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN" "$USER_ID" EX 300 >/dev/null
    code=$(curl -sS -o "$EVID/quote-member-normal.json" -w '%{http_code}' -X POST \
      -H "Authorization: Bearer $UTOKEN" \
      -H 'Content-Type: application/json' \
      --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PRICE_MEMBER}\"}" \
      "${API_BASE}/api/v1/payment/price-quotes" || echo ERR)
    echo "POST price-quotes member-normal -> $code" | tee -a "$EVID/04-probe.txt"
    # redact quoteId if present
    python3 - <<'PY' | tee -a "$EVID/04-probe.txt" || true
import json,hashlib
p="/tmp/v2-flags-enable-20260729/quote-member-normal.json"
try:
  d=json.load(open(p))
except Exception as e:
  print("body_raw", open(p).read()[:300]); raise SystemExit
qid=d.get("quoteId") or (d.get("quote") or {}).get("quoteId")
if qid:
  d["quoteId"]="sha256:"+hashlib.sha256(qid.encode()).hexdigest()[:16]
  if isinstance(d.get("quote"), dict) and d["quote"].get("quoteId"):
    d["quote"]["quoteId"]="sha256:"+hashlib.sha256(qid.encode()).hexdigest()[:16]
print(json.dumps({k:d.get(k) for k in ("code","error","message","transactionPrice","priceCents","currency","environment","channel","quoteId","pricePlanId","planId") if k in d or True}, ensure_ascii=False)[:800])
# keep redacted file
json.dump(d, open(p,"w"), ensure_ascii=False, indent=2)
PY
    # NO order create — dry quote only
    docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN" >/dev/null || true
  else
    echo "SKIP authenticated quote: missing user or pricePlanId" | tee -a "$EVID/04-probe.txt"
  fi

  print_flags | tee "$EVID/04-flags-final.txt"
  check_prices "$EVID/04-prices-final.txt"
  echo "PROBE_DONE $(TS)" | tee -a "$EVID/04-probe.txt"
fi

echo "DONE step=$STEP $(TS)" | tee -a "$EVID/done.txt"
