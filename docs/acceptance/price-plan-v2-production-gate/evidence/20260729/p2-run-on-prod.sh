#!/usr/bin/env bash
set -euo pipefail
EVIDENCE_DIR=/tmp/p2-reconcile-20260729
mkdir -p "$EVIDENCE_DIR"
OUT="$EVIDENCE_DIR/p2-readonly.txt"
APP_C=zhiqiyun-ai-prod-xianzhi-ai-1
PG_C=zhiqiyun-ai-prod-postgres-1
EXPECTED_IMAGE_ID="sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32"
EXPECTED_GIT_SHA="a39485ef159dabf348a71059a0e922af4894ab5a"

: > "$OUT"
log() { echo "$*" | tee -a "$OUT"; }

log "=== P2 READ-ONLY START $(date -Iseconds) ==="
log "HOSTNAME=$(hostname)"
log "FORBIDDEN: migrate / deploy / compose up --build / flag flip / wechat console"
log

log "A_CID=$(docker inspect -f '{{.Id}}' "$APP_C" | cut -c1-12)"
IMG=$(docker inspect -f '{{.Image}}' "$APP_C")
RUNNING_IMAGE_ID=$(docker inspect -f '{{.Id}}' "$IMG")
CONFIG_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$APP_C")
log "A_CONFIG_IMAGE=${CONFIG_IMAGE}"
log "A_RUNNING_IMAGE_ID=${RUNNING_IMAGE_ID}"
log "A_EXPECTED_IMAGE_ID=${EXPECTED_IMAGE_ID}"
if [[ "$RUNNING_IMAGE_ID" == "$EXPECTED_IMAGE_ID" ]]; then
  log "A_IMAGE_MATCH=EXACT"
else
  log "A_IMAGE_MATCH=DIVERGED"
fi
log "A_GIT_EXPECTED=${EXPECTED_GIT_SHA}"
log

for k in SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED PRICE_PLAN_TEST_ENTRY_ENABLED WECHAT_VIRTUAL_PAY_ENV; do
  v=$(docker exec "$APP_C" printenv "$k" 2>/dev/null || echo MISSING)
  log "B_${k}=${v}"
done
if docker exec "$APP_C" printenv SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED | grep -qx true \
  && docker exec "$APP_C" printenv PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED | grep -qx true \
  && docker exec "$APP_C" printenv PRICE_PLAN_TEST_ENTRY_ENABLED | grep -qx true \
  && docker exec "$APP_C" printenv WECHAT_VIRTUAL_PAY_ENV | grep -qx production; then
  log "B_FLAGS_RESULT=PASS"
else
  log "B_FLAGS_RESULT=CHECK"
fi
log

db=$(docker exec "$PG_C" printenv POSTGRES_DB)
u=$(docker exec "$PG_C" printenv POSTGRES_USER)
log "C_DB_NAME=${db}"
log "C_DB_USER=${u}"

docker exec -i "$PG_C" psql -U "$u" -d "$db" -v ON_ERROR_STOP=1 <<'SQL' > "$EVIDENCE_DIR/p2-db.out" 2>&1
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT current_database() AS db, current_user AS db_user,
       pg_is_in_recovery() AS is_replica,
       current_setting('transaction_read_only') AS txn_ro;

SELECT relname, to_regclass('public.'||relname) IS NOT NULL AS present
FROM (VALUES
  ('xz_price_plans'),
  ('xz_price_plan_versions'),
  ('xz_price_plan_payment_bindings'),
  ('xz_wechat_virtual_goods'),
  ('xz_price_plan_user_whitelist'),
  ('xz_order_price_quotes')
) AS t(relname);

SELECT count(*) AS still_not_valid_097_100
FROM pg_constraint
WHERE contype IN ('c','f')
  AND NOT convalidated
  AND conname ~ '_(097|098|099|100)$';

SELECT conrelid::regclass::text AS table_name, conname, contype, convalidated
FROM pg_constraint
WHERE contype IN ('c','f')
  AND conname ~ '_(097|098|099|100)$'
ORDER BY 1, 2;

SELECT code, status, enabled, is_default, sale_price_cents, price_type, environment
FROM xz_price_plans
WHERE environment='PRODUCTION'
  AND (code ILIKE '%agent%' OR plan_id ILIKE '%agent%')
ORDER BY code
LIMIT 30;

SELECT g.product_id, g.platform_price_cents, g.status, g.environment
FROM xz_wechat_virtual_goods g
WHERE g.product_id IN ('AGENT_JOIN_996','AGENT_TEST_1YUAN','MEMBER_YEAR_996','MEMBER_TEST_1YUAN')
ORDER BY g.product_id, g.environment;

ROLLBACK;
SQL

cat "$EVIDENCE_DIR/p2-db.out" >> "$OUT"
SNV=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT count(*) FROM pg_constraint WHERE contype IN ('c','f') AND NOT convalidated AND conname ~ '_(097|098|099|100)$';")
AGENT996=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE product_id='AGENT_JOIN_996' AND environment='PRODUCTION' ORDER BY 1 DESC LIMIT 1;")
AGENT1=$(docker exec "$PG_C" psql -U "$u" -d "$db" -Atc "SELECT platform_price_cents FROM xz_wechat_virtual_goods WHERE product_id='AGENT_TEST_1YUAN' AND environment='PRODUCTION' ORDER BY 1 DESC LIMIT 1;")
log "C_STILL_NOT_VALID_097_100=${SNV}"
log "C_AGENT_JOIN_996_CENTS=${AGENT996}"
log "C_AGENT_TEST_1YUAN_CENTS=${AGENT1}"
if [[ "$SNV" == "0" && "$AGENT996" == "99600" && "$AGENT1" == "100" ]]; then
  log "C_RESULT=PASS"
else
  log "C_RESULT=CHECK"
fi

A_OK=0; B_OK=0; C_OK=0
grep -q 'A_IMAGE_MATCH=EXACT' "$OUT" && A_OK=1
grep -q 'B_FLAGS_RESULT=PASS' "$OUT" && B_OK=1
grep -q 'C_RESULT=PASS' "$OUT" && C_OK=1
if [[ "$A_OK" == 1 && "$B_OK" == 1 && "$C_OK" == 1 ]]; then
  log "P2_OVERALL=PASS"
else
  log "P2_OVERALL=FAIL_OR_CHECK A=$A_OK B=$B_OK C=$C_OK"
fi
log "=== P2 READ-ONLY END $(date -Iseconds) ==="
log "EVIDENCE_DIR=${EVIDENCE_DIR}"
