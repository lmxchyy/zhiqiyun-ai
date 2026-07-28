#!/usr/bin/env bash
# Temporary WECHAT_VIRTUAL_PAY_ENV=sandbox window, then restore production.
# Keeps V2 three flags true. Authorized by user 「都要继续」sandbox continuation.
set -euo pipefail

cd /opt/zhiqiyun-ai
EVID="${EVID:-/tmp/sandbox-runtime-20260729}"
mkdir -p "$EVID"
TS() { date '+%Y-%m-%d %H:%M:%S %z'; }
ENV_FILE=.env.production
APP=xianzhi-ai
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
REDIS_C=zhiqiyun-ai-prod-redis-1
PG_C=zhiqiyun-ai-prod-postgres-1
API_BASE=http://127.0.0.1:3100
ACTION="${1:-window}"  # window | restore-only | quotes-only

set_env_flag() {
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
  for i in $(seq 1 45); do
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

redact_quote() {
  local file="$1"
  python3 - <<PY
import json,hashlib,sys
p="$file"
try:
  d=json.load(open(p))
except Exception as e:
  print("parse_fail", e); sys.exit(0)
qid=d.get("quoteId")
if qid:
  d["quoteId"]="sha256:"+hashlib.sha256(qid.encode()).hexdigest()[:16]
json.dump(d, open(p,"w"), ensure_ascii=False, indent=2)
print("amountCent=", d.get("amountCent"), "env=", d.get("environment"), "testOnly=", d.get("testOnly"), "productId=", d.get("wechatProductId") or d.get("productId"))
PY
}

run_quotes() {
  local label="$1"
  local db u
  db=$(docker exec "$PG_C" printenv POSTGRES_DB)
  u=$(docker exec "$PG_C" printenv POSTGRES_USER)
  local pay_env
  pay_env=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
  local env_name
  if [ "$pay_env" = "sandbox" ]; then env_name=SANDBOX; else env_name=PRODUCTION; fi
  echo "=== QUOTES label=$label runtime=$pay_env env_name=$env_name $(TS) ===" | tee "$EVID/quotes-${label}.txt"

  local PP_MEMBER_N PP_AGENT_N PP_MEMBER_T PP_AGENT_T
  PP_MEMBER_N=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='$env_name' AND price_type='NORMAL' AND is_default AND enabled LIMIT 1;")
  PP_AGENT_N=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_agent_join_996' AND environment='$env_name' AND price_type='NORMAL' AND is_default AND enabled LIMIT 1;")
  PP_MEMBER_T=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_ai_creator_996' AND environment='$env_name' AND price_type='TEST' AND enabled LIMIT 1;")
  PP_AGENT_T=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT id FROM xz_price_plans WHERE plan_id='plan_agent_join_996' AND environment='$env_name' AND price_type='TEST' AND enabled LIMIT 1;")
  echo "PP_MEMBER_N=$PP_MEMBER_N PP_AGENT_N=$PP_AGENT_N PP_MEMBER_T=$PP_MEMBER_T PP_AGENT_T=$PP_AGENT_T" | tee -a "$EVID/quotes-${label}.txt"

  REDIS_PASSWORD=$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)
  UTOKEN="gate_sbx_quote_${label}_$$"
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$UTOKEN" "user_000002" EX 300 >/dev/null

  quote_one() {
    local path="$1" plan="$2" pp="$3" out="$4"
    local code
    code=$(curl -sS -o "$EVID/$out" -w '%{http_code}' -X POST \
      -H "Authorization: Bearer $UTOKEN" \
      -H 'Content-Type: application/json' \
      --data "{\"planId\":\"${plan}\",\"pricePlanId\":\"${pp}\"}" \
      "${API_BASE}${path}")
    echo "POST $path plan=$plan pp=$pp -> $code" | tee -a "$EVID/quotes-${label}.txt"
    redact_quote "$EVID/$out" | tee -a "$EVID/quotes-${label}.txt"
  }

  quote_one /api/v1/payment/price-quotes plan_ai_creator_996 "$PP_MEMBER_N" "quote-member-normal-${label}.json"
  quote_one /api/v1/payment/price-quotes plan_agent_join_996 "$PP_AGENT_N" "quote-agent-normal-${label}.json"
  if [ -n "$PP_MEMBER_T" ]; then
    quote_one /api/v1/payment/test-price-quotes plan_ai_creator_996 "$PP_MEMBER_T" "quote-member-test-${label}.json"
  fi
  if [ -n "$PP_AGENT_T" ]; then
    quote_one /api/v1/payment/test-price-quotes plan_agent_join_996 "$PP_AGENT_T" "quote-agent-test-${label}.json"
  fi

  # U0 denial smoke (admin user, not whitelisted for TEST)
  U0="gate_sbx_u0_${label}_$$"
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$U0" "user_000001" EX 120 >/dev/null
  if [ -n "$PP_MEMBER_T" ]; then
    code=$(curl -sS -o "$EVID/quote-u0-member-test-${label}.json" -w '%{http_code}' -X POST \
      -H "Authorization: Bearer $U0" -H 'Content-Type: application/json' \
      --data "{\"planId\":\"plan_ai_creator_996\",\"pricePlanId\":\"${PP_MEMBER_T}\"}" \
      "${API_BASE}/api/v1/payment/test-price-quotes")
    echo "U0 TEST member quote -> $code (expect 403)" | tee -a "$EVID/quotes-${label}.txt"
  fi
  docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$UTOKEN" "auth:session:$U0" >/dev/null || true
  unset REDIS_PASSWORD
}

restore_production() {
  echo "=== RESTORE production $(TS) ===" | tee "$EVID/03-restore.txt"
  set_env_flag WECHAT_VIRTUAL_PAY_ENV production
  # ensure V2 flags stay true
  set_env_flag SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED true
  set_env_flag PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED true
  set_env_flag PRICE_PLAN_TEST_ENTRY_ENABLED true
  recreate_and_wait 03-restore
  print_flags | tee -a "$EVID/03-restore.txt"
  grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" | tee -a "$EVID/03-restore.txt"
  PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
  test "$PAY" = "production"
}

case "$ACTION" in
  restore-only)
    restore_production
    exit 0
    ;;
  quotes-only)
    run_quotes "only"
    exit 0
    ;;
esac

echo "=== PRECHECK $(TS) ===" | tee "$EVID/00-precheck.txt"
print_flags | tee -a "$EVID/00-precheck.txt"
grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" | tee -a "$EVID/00-precheck.txt"
mkdir -p backups/compose
cp "$ENV_FILE" "backups/compose/.env.production.pre-sandbox-window-$(date +%Y%m%d_%H%M%S).bak"
grep -E '^(SNAPSHOT_V2_|PRICE_PLAN_|WECHAT_VIRTUAL_PAY_ENV)=' "$ENV_FILE" > "$EVID/env.flags.pre.txt"

# baseline production quotes (before switch)
run_quotes "prod-before"

echo "=== SWITCH sandbox $(TS) ===" | tee "$EVID/01-switch-sandbox.txt"
set_env_flag WECHAT_VIRTUAL_PAY_ENV sandbox
set_env_flag SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED true
set_env_flag PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED true
set_env_flag PRICE_PLAN_TEST_ENTRY_ENABLED true
recreate_and_wait 01-switch-sandbox
print_flags | tee -a "$EVID/01-switch-sandbox.txt"
PAY=$(docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV)
test "$PAY" = "sandbox"

run_quotes "sandbox"

# pricing health under sandbox
REDIS_PASSWORD=$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)
ATOKEN="gate_sbx_health_$$"
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$ATOKEN" "user_000001" EX 120 >/dev/null
curl -sS -o "$EVID/pricing-health-sandbox.json" -H "Authorization: Bearer $ATOKEN" \
  "${API_BASE}/api/v1/admin/pricing/health" || true
docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$ATOKEN" >/dev/null || true
unset REDIS_PASSWORD
python3 - <<'PY' | tee -a "$EVID/01-switch-sandbox.txt" || true
import json
d=json.load(open("/tmp/sandbox-runtime-20260729/pricing-health-sandbox.json"))
print("health status=", d.get("status"), "blocked=", (d.get("summary") or {}).get("blockedIssueCount"))
PY

restore_production

# post-restore production quotes
run_quotes "prod-after"

echo "=== WINDOW DONE $(TS) ===" | tee "$EVID/DONE.txt"
print_flags | tee -a "$EVID/DONE.txt"
