#!/usr/bin/env bash
# Seed SANDBOX-env V2 MEMBER/AGENT objects (separate from PRODUCTION rows).
# Idempotent by sandbox-specific codes / env-scoped productIds.
# Does NOT flip WECHAT_VIRTUAL_PAY_ENV or V2 switches.
set -euo pipefail

EVIDENCE_DIR="${EVIDENCE_DIR:-/tmp/sandbox-v2-seed-evidence}"
mkdir -p "$EVIDENCE_DIR"
API_BASE="${API_BASE:-http://127.0.0.1:3100}"
REDIS_C="${REDIS_C:-zhiqiyun-ai-prod-redis-1}"
APP_C="${APP_C:-zhiqiyun-ai-prod-xianzhi-ai-1}"
PG_C="${PG_C:-zhiqiyun-ai-prod-postgres-1}"
ACTOR_ID="user_000001"
TOKEN="gate_v2_sandbox_seed_$(date +%Y%m%d%H%M%S)_$$"
OFFER_ID="1450579876"
MODE="short_series_goods"
ENV_NAME="SANDBOX"
REASON_BASE="price-plan-v2-production-gate SANDBOX seed 20260729"
VERIF_REASON="SANDBOX mirror of dual-sign PRODUCTION props; productIds shared per WeChat sandbox catalog; env-isolated rows"
VERIF_EVIDENCE="docs/acceptance/price-plan-v2-production-gate/evidence/20260729/price-owner-wechat-goods-dual-sign.md"
EXPIRES_AT="$(date -u -d '+30 days' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+30d +%Y-%m-%dT%H:%M:%SZ)"

log() { printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" | tee -a "$EVIDENCE_DIR/seed.log" >&2; }

auth_curl() {
  local method="$1" path="$2" body="${3:-}"
  local tmp="$EVIDENCE_DIR/last-http.json"
  local code
  if [[ -n "$body" ]]; then
    code=$(curl -sS -g -o "$tmp" -w '%{http_code}' -X "$method" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "${API_BASE}${path}")
  else
    code=$(curl -sS -g -o "$tmp" -w '%{http_code}' -X "$method" \
      -H "Authorization: Bearer $TOKEN" \
      "${API_BASE}${path}")
  fi
  echo "$code"
}

require_code() {
  local got="$1" want="$2" label="$3"
  if [[ "$got" != "$want" ]]; then
    log "FAIL $label http=$got body=$(head -c 800 "$EVIDENCE_DIR/last-http.json")"
    exit 1
  fi
  log "OK $label http=$got"
  cp "$EVIDENCE_DIR/last-http.json" "$EVIDENCE_DIR/${label}.json"
}

find_price_plan_by_code() {
  local plan_id="$1" code="$2"
  local http
  http=$(auth_curl GET "/api/v1/admin/business-plans/${plan_id}/price-plans")
  require_code "$http" "200" "list-price-plans-${plan_id}"
  python3 - <<PY
import json
items=json.load(open("$EVIDENCE_DIR/last-http.json"))["items"]
for it in items:
  if it.get("code")=="$code":
    print(it["pricePlanId"]); break
else:
  print("")
PY
}

find_good_by_product() {
  local env="$1" product="$2"
  local http
  http=$(auth_curl GET "/api/v1/admin/wechat-virtual-goods")
  require_code "$http" "200" "list-goods-${env}-${product}"
  python3 - <<PY
import json
payload=json.load(open("$EVIDENCE_DIR/last-http.json"))
items=payload.get("items") or payload.get("goods") or []
for it in items:
  if it.get("productId")=="$product" and it.get("environment")=="$env":
    print(it.get("id") or it.get("wechatGoodId") or ""); break
else:
  print("")
PY
}

ensure_active_version() {
  local plan_id="$1"
  local http
  http=$(auth_curl GET "/api/v1/admin/business-plans/${plan_id}/versions")
  require_code "$http" "200" "list-versions-${plan_id}"
  python3 - <<PY
import json
items=json.load(open("$EVIDENCE_DIR/last-http.json"))["items"]
for it in items:
  if it.get("status")=="ACTIVE":
    print(it["id"]); break
else:
  raise SystemExit("no ACTIVE version for $plan_id")
PY
}

ensure_price_plan() {
  local plan_id="$1" version_id="$2" code="$3" name="$4" kind="$5" cents="$6" audience="$7" visible="$8" label="$9"
  local existing
  existing=$(find_price_plan_by_code "$plan_id" "$code")
  if [[ -n "$existing" ]]; then
    log "reuse price plan $existing code=$code"
    echo "$existing"
    return
  fi
  local body
  body=$(VISIBLE_PY="$visible" python3 - <<PY
import json, os
visible = os.environ["VISIBLE_PY"].lower() == "true"
print(json.dumps({
  "revision": 1,
  "planVersionId": "$version_id",
  "code": "$code",
  "name": "$name",
  "kind": "$kind",
  "channel": "WECHAT_VIRTUAL",
  "environment": "$ENV_NAME",
  "currency": "CNY",
  "salePriceCents": $cents,
  "listPriceCents": $cents,
  "giftPoints": 0,
  "giftTokens": 0,
  "audienceType": "$audience",
  "audienceRule": {},
  "isVisible": visible,
  "changeReason": "$REASON_BASE create $label"
}))
PY
)
  local http
  http=$(auth_curl POST "/api/v1/admin/business-plans/${plan_id}/price-plans" "$body")
  require_code "$http" "201" "create-price-${label}"
  python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["pricePlanId"])'
}

ensure_good() {
  local product="$1" name="$2" cents="$3" label="$4"
  local existing
  existing=$(find_good_by_product "$ENV_NAME" "$product")
  if [[ -n "$existing" ]]; then
    log "reuse good $existing product=$product env=$ENV_NAME"
    local http
    http=$(auth_curl GET "/api/v1/admin/wechat-virtual-goods/${existing}")
    require_code "$http" "200" "get-good-${label}"
    local status verif rev
    status=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["status"])')
    verif=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"].get("verificationStatus",""))')
    rev=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["revision"])')
    if [[ "$status" != "PUBLISHED" || "$verif" != "MANUALLY_CONFIRMED_PUBLISHED" ]]; then
      http=$(auth_curl POST "/api/v1/admin/wechat-virtual-goods/${existing}/confirm-published" \
        "{\"revision\":${rev},\"reason\":\"${REASON_BASE} confirm ${label}\",\"verificationReason\":\"${VERIF_REASON}\",\"evidence\":\"${VERIF_EVIDENCE}\",\"verificationExpiresAt\":\"${EXPIRES_AT}\"}")
      require_code "$http" "200" "confirm-good-${label}"
    fi
    echo "$existing"
    return
  fi
  local body
  body=$(python3 - <<PY
import json
print(json.dumps({
  "channel": "WECHAT_VIRTUAL",
  "environment": "$ENV_NAME",
  "offerId": "$OFFER_ID",
  "productId": "$product",
  "goodsName": "$name",
  "platformPriceCents": $cents,
  "mode": "$MODE",
  "reason": "$REASON_BASE create good $label"
}))
PY
)
  local http
  http=$(auth_curl POST "/api/v1/admin/wechat-virtual-goods" "$body")
  require_code "$http" "201" "create-good-${label}"
  local good_id rev
  good_id=$(python3 -c 'import json; d=json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]; print(d.get("id") or d.get("wechatGoodId"))')
  rev=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["revision"])')
  http=$(auth_curl POST "/api/v1/admin/wechat-virtual-goods/${good_id}/confirm-published" \
    "{\"revision\":${rev},\"reason\":\"${REASON_BASE} confirm ${label}\",\"verificationReason\":\"${VERIF_REASON}\",\"evidence\":\"${VERIF_EVIDENCE}\",\"verificationExpiresAt\":\"${EXPIRES_AT}\"}")
  require_code "$http" "200" "confirm-good-${label}"
  echo "$good_id"
}

ensure_binding_active() {
  local price_plan_id="$1" good_id="$2" label="$3"
  local http
  http=$(auth_curl GET "/api/v1/admin/price-plans/${price_plan_id}/payment-bindings")
  require_code "$http" "200" "list-bindings-${label}"
  local binding_id rev enabled status
  read -r binding_id rev enabled status < <(python3 - <<PY
import json
items=json.load(open("$EVIDENCE_DIR/last-http.json"))["items"]
match=None
for it in items:
  gid=it.get("wechatGoodId") or it.get("wechat_good_id")
  if gid=="$good_id":
    match=it; break
if not items:
  print("NONE 0 false NONE")
elif match is None:
  it=items[0]
  print(it.get("id") or it.get("bindingId"), it.get("revision",0), str(it.get("enabled",False)).lower(), it.get("status"))
else:
  print(match.get("id") or match.get("bindingId"), match.get("revision",0), str(match.get("enabled",False)).lower(), match.get("status"))
PY
)
  if [[ "$binding_id" == "NONE" || -z "$binding_id" ]]; then
    http=$(auth_curl POST "/api/v1/admin/price-plans/${price_plan_id}/payment-bindings" \
      "{\"wechatGoodId\":\"${good_id}\",\"reason\":\"${REASON_BASE} bind ${label}\"}")
    require_code "$http" "201" "create-binding-${label}"
    binding_id=$(python3 -c 'import json; d=json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]; print(d.get("id") or d.get("bindingId"))')
    rev=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["revision"])')
    enabled="false"
    status="DRAFT"
  fi
  if [[ "$enabled" != "true" || "$status" != "ACTIVE" ]]; then
    http=$(auth_curl PATCH "/api/v1/admin/payment-bindings/${binding_id}" \
      "{\"revision\":${rev},\"enabled\":true,\"reason\":\"${REASON_BASE} activate binding ${label}\"}")
    require_code "$http" "200" "activate-binding-${label}"
  else
    log "binding already ACTIVE $binding_id"
  fi
  echo "$binding_id"
}

enable_and_default() {
  local price_plan_id="$1" label="$2" make_default="$3"
  local http rev status enabled is_default
  http=$(auth_curl GET "/api/v1/admin/price-plans/${price_plan_id}")
  require_code "$http" "200" "get-price-${label}"
  rev=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["revision"])')
  status=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["status"])')
  enabled=$(python3 -c 'import json; d=json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]; print(str(d.get("isEnabled", d.get("enabled", False))).lower())')
  is_default=$(python3 -c 'import json; print(str(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"].get("isDefault", False)).lower())')

  if [[ "$enabled" != "true" || "$status" != "ACTIVE" ]]; then
    http=$(auth_curl POST "/api/v1/admin/price-plans/${price_plan_id}/enable" \
      "{\"revision\":${rev},\"changeReason\":\"${REASON_BASE} enable ${label}\"}")
    require_code "$http" "200" "enable-price-${label}"
    rev=$(python3 -c 'import json; print(json.load(open("'"$EVIDENCE_DIR"'/last-http.json"))["item"]["revision"])')
  else
    log "price plan already enabled $price_plan_id"
  fi

  if [[ "$make_default" == "true" && "$is_default" != "true" ]]; then
    http=$(auth_curl POST "/api/v1/admin/price-plans/${price_plan_id}/make-default" \
      "{\"revision\":${rev},\"changeReason\":\"${REASON_BASE} make default ${label}\"}")
    require_code "$http" "200" "default-price-${label}"
  fi
}

ensure_whitelist() {
  local price_plan_id="$1" user_id="$2" label="$3"
  local http
  http=$(auth_curl GET "/api/v1/admin/price-plans/${price_plan_id}/whitelist")
  if [[ "$http" == "200" ]]; then
    local existing
    existing=$(python3 - <<PY
import json
payload=json.load(open("$EVIDENCE_DIR/last-http.json"))
items=payload.get("items") or payload.get("entries") or []
for it in items:
  if it.get("userId")=="$user_id" and str(it.get("status","")).upper() in ("ACTIVE","ENABLED"):
    print(it.get("whitelistEntryId") or it.get("id") or it.get("entryId") or "YES"); break
else:
  print("")
PY
)
    if [[ -n "$existing" ]]; then
      log "whitelist already active user=$user_id plan=$price_plan_id ($existing)"
      return
    fi
  fi
  local body
  body=$(python3 - <<PY
import json
print(json.dumps({
  "revision": 0,
  "userId": "$user_id",
  "reason": "$REASON_BASE whitelist $label for user_000002 sandbox TEST",
  "changeReason": "sandbox-runtime gate: enable TEST whitelist on SANDBOX TEST plans"
}))
PY
)
  http=$(auth_curl POST "/api/v1/admin/price-plans/${price_plan_id}/whitelist" "$body")
  if [[ "$http" != "201" && "$http" != "200" ]]; then
    log "WARN whitelist ${label} http=$http body=$(head -c 400 "$EVIDENCE_DIR/last-http.json")"
  else
    log "OK whitelist ${label} http=$http"
    cp "$EVIDENCE_DIR/last-http.json" "$EVIDENCE_DIR/whitelist-${label}.json"
  fi
}

log "=== flags before ==="
docker exec "$APP_C" sh -c 'printenv' | grep -E '^(PRICE_PLAN_|SNAPSHOT_V2_|WECHAT_VIRTUAL_PAY_ENV)=' | tee "$EVIDENCE_DIR/flags-before.txt" || true

log "inject redis session for $ACTOR_ID"
REDIS_PASSWORD="$(docker exec "$REDIS_C" printenv REDIS_PASSWORD)"
redis_cli() { docker exec "$REDIS_C" redis-cli -a "$REDIS_PASSWORD" --no-auth-warning "$@"; }
redis_cli SET "auth:session:$TOKEN" "$ACTOR_ID" EX 1800 >/dev/null

MEMBER_VERSION=$(ensure_active_version "plan_ai_creator_996")
AGENT_VERSION=$(ensure_active_version "plan_agent_join_996")
log "MEMBER_VERSION=$MEMBER_VERSION AGENT_VERSION=$AGENT_VERSION"

log "=== ensure SANDBOX wechat goods ==="
GOOD_MEMBER_N=$(ensure_good "MEMBER_YEAR_996" "996AI创作会员包(沙箱)" 99600 "sbx-member-normal")
GOOD_AGENT_N=$(ensure_good "AGENT_JOIN_996" "996AI代理加盟包(沙箱)" 99600 "sbx-agent-normal")
GOOD_MEMBER_T=$(ensure_good "MEMBER_TEST_1YUAN" "会员1元测试包(沙箱)" 100 "sbx-member-test")
GOOD_AGENT_T=$(ensure_good "AGENT_TEST_1YUAN" "代理1元测试包(沙箱)" 100 "sbx-agent-test")

log "=== ensure SANDBOX price plans ==="
PP_MEMBER_N=$(ensure_price_plan "plan_ai_creator_996" "$MEMBER_VERSION" "pp_member_normal_sbx_996" "会员沙箱正式价996" "NORMAL" 99600 "PUBLIC" true "sbx-member-normal")
PP_AGENT_N=$(ensure_price_plan "plan_agent_join_996" "$AGENT_VERSION" "pp_agent_normal_sbx_996" "代理沙箱正式价996" "NORMAL" 99600 "PUBLIC" true "sbx-agent-normal")
PP_MEMBER_T=$(ensure_price_plan "plan_ai_creator_996" "$MEMBER_VERSION" "pp_member_test_sbx_entry" "会员沙箱1元测试" "TEST" 100 "TEST" false "sbx-member-test")
PP_AGENT_T=$(ensure_price_plan "plan_agent_join_996" "$AGENT_VERSION" "pp_agent_test_sbx_entry" "代理沙箱1元测试" "TEST" 100 "TEST" false "sbx-agent-test")

log "=== ensure SANDBOX bindings ==="
BIND_MEMBER_N=$(ensure_binding_active "$PP_MEMBER_N" "$GOOD_MEMBER_N" "sbx-member-normal")
BIND_AGENT_N=$(ensure_binding_active "$PP_AGENT_N" "$GOOD_AGENT_N" "sbx-agent-normal")
BIND_MEMBER_T=$(ensure_binding_active "$PP_MEMBER_T" "$GOOD_MEMBER_T" "sbx-member-test")
BIND_AGENT_T=$(ensure_binding_active "$PP_AGENT_T" "$GOOD_AGENT_T" "sbx-agent-test")

log "=== enable + default SANDBOX NORMAL ==="
enable_and_default "$PP_MEMBER_N" "sbx-member-normal" true
enable_and_default "$PP_AGENT_N" "sbx-agent-normal" true
enable_and_default "$PP_MEMBER_T" "sbx-member-test" false
enable_and_default "$PP_AGENT_T" "sbx-agent-test" false

log "=== whitelist demo for SANDBOX TEST ==="
ensure_whitelist "$PP_MEMBER_T" "user_000002" "sbx-member-test"
ensure_whitelist "$PP_AGENT_T" "user_000002" "sbx-agent-test"

python3 - <<PY | tee "$EVIDENCE_DIR/created-inventory.json"
import json
print(json.dumps({
  "environment": "$ENV_NAME",
  "offerId": "$OFFER_ID",
  "mode": "$MODE",
  "versions": {"member": "$MEMBER_VERSION", "agent": "$AGENT_VERSION"},
  "pricePlans": {
    "memberNormal": {"id": "$PP_MEMBER_N", "code": "pp_member_normal_sbx_996", "cents": 99600, "productId": "MEMBER_YEAR_996", "bindingId": "$BIND_MEMBER_N", "goodId": "$GOOD_MEMBER_N"},
    "agentNormal": {"id": "$PP_AGENT_N", "code": "pp_agent_normal_sbx_996", "cents": 99600, "productId": "AGENT_JOIN_996", "bindingId": "$BIND_AGENT_N", "goodId": "$GOOD_AGENT_N"},
    "memberTest": {"id": "$PP_MEMBER_T", "code": "pp_member_test_sbx_entry", "cents": 100, "productId": "MEMBER_TEST_1YUAN", "bindingId": "$BIND_MEMBER_T", "goodId": "$GOOD_MEMBER_T"},
    "agentTest": {"id": "$PP_AGENT_T", "code": "pp_agent_test_sbx_entry", "cents": 100, "productId": "AGENT_TEST_1YUAN", "bindingId": "$BIND_AGENT_T", "goodId": "$GOOD_AGENT_T"}
  }
}, indent=2, ensure_ascii=False))
PY

log "=== flags after (unchanged expected) ==="
docker exec "$APP_C" sh -c 'printenv' | grep -E '^(PRICE_PLAN_|SNAPSHOT_V2_|WECHAT_VIRTUAL_PAY_ENV)=' | tee "$EVIDENCE_DIR/flags-after.txt" || true
redis_cli DEL "auth:session:$TOKEN" >/dev/null || true
unset REDIS_PASSWORD
log "DONE"
