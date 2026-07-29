#!/usr/bin/env bash
set -euo pipefail
EVID=/tmp/section5-probes-20260729
PG_C=zhiqiyun-ai-prod-postgres-1
REDIS_C=zhiqiyun-ai-prod-redis-1
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
API_BASE=http://127.0.0.1:3100
db=$(docker exec $PG_C printenv POSTGRES_DB)
u=$(docker exec $PG_C printenv POSTGRES_USER)
REDIS_PASSWORD=$(docker exec $REDIS_C printenv REDIS_PASSWORD)
ATOKEN=gate_s5_health_final_$$
docker exec $REDIS_C redis-cli -a "$REDIS_PASSWORD" --no-auth-warning SET "auth:session:$ATOKEN" "user_000001" EX 120 >/dev/null
curl -sS -o "$EVID/pricing-health-final.json" -H "Authorization: Bearer $ATOKEN" "${API_BASE}/api/v1/admin/pricing/health"
python3 - <<'PY'
import json
d=json.load(open("/tmp/section5-probes-20260729/pricing-health-final.json"))
print("status=", d.get("status"))
print("blocked=", (d.get("summary") or {}).get("blockedIssueCount"))
print("keys=", list(d)[:20])
issues=d.get("issues") or d.get("blockedIssues") or []
if isinstance(issues, list):
  for i in issues[:10]:
    print("issue=", i.get("code") if isinstance(i,dict) else i)
PY
docker exec $PG_C psql -U "$u" -d "$db" -c "
SELECT id, lifecycle_status, enabled, expires_at>now() AS not_expired, revision
FROM xz_price_plan_user_whitelist
WHERE user_id='user_000002' AND price_plan_id='price_plan_20260728212634000000000_049a91b1'
ORDER BY created_at;
"
docker exec $APP_C printenv SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED PRICE_PLAN_TEST_ENTRY_ENABLED WECHAT_VIRTUAL_PAY_ENV
docker exec $REDIS_C redis-cli -a "$REDIS_PASSWORD" --no-auth-warning DEL "auth:session:$ATOKEN" >/dev/null
# redact secrets in evidence json
python3 - <<'PY'
import json,hashlib,glob,os
evid="/tmp/section5-probes-20260729"
for path in glob.glob(evid+"/*.json"):
  try:
    d=json.load(open(path))
  except Exception:
    continue
  changed=False
  for k in ("quoteId","paySig","signature","signData","sessionKey","wxLoginCode"):
    if k in d and d[k]:
      if k=="quoteId":
        d[k]="sha256:"+hashlib.sha256(str(d[k]).encode()).hexdigest()[:16]
      else:
        d[k]="[REDACTED]"
      changed=True
  if "item" in d and isinstance(d["item"], dict):
    for k in ("paySig","signature","signData"):
      if k in d["item"]:
        d["item"][k]="[REDACTED]"; changed=True
  if changed:
    json.dump(d, open(path,"w"), ensure_ascii=False, indent=2)
    print("redacted", os.path.basename(path))
print("DONE redact")
PY
ls -la "$EVID" | head -60
unset REDIS_PASSWORD
