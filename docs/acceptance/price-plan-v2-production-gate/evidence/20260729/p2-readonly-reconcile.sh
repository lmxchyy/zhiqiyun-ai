#!/usr/bin/env bash
# P2 production read-only reconcile — NO migrate, NO deploy, NO flag changes, NO wechat ops.
# Run on the production host (or jump host with docker + psql to prod).
# Usage:
#   EVIDENCE_DIR=/tmp/p2-reconcile-20260729 bash p2-readonly-reconcile.sh
#   # if app DB is reached via docker exec:
#   PSQL='docker exec -i <pg-container> psql -U <user> -d <db> -v ON_ERROR_STOP=1'
set -euo pipefail

EVIDENCE_DIR="${EVIDENCE_DIR:-/tmp/p2-reconcile-$(date +%Y%m%d%H%M%S)}"
mkdir -p "$EVIDENCE_DIR"
OUT="$EVIDENCE_DIR/p2-readonly.txt"
: > "$OUT"

EXPECTED_IMAGE_ID="sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32"
EXPECTED_GIT_SHA="a39485ef159dabf348a71059a0e922af4894ab5a"

log() { echo "$*" | tee -a "$OUT"; }

log "=== P2 READ-ONLY START $(date -Iseconds) ==="
log "HOSTNAME=$(hostname)"
log "FORBIDDEN: migrate / deploy / compose up --build / flag flip / wechat console"
log

# --- A. Running image identity ---
CID=$(docker ps -q --filter name=xianzhi-ai | head -1 || true)
log "A_CID=${CID:-EMPTY}"
if [[ -z "${CID}" ]]; then
  log "A_RESULT=FAIL no running xianzhi-ai container"
else
  IMG=$(docker inspect -f '{{.Image}}' "$CID")
  RUNNING_IMAGE_ID=$(docker inspect -f '{{.Id}}' "$IMG")
  CONFIG_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$CID")
  # Do NOT dump Config.Env (P0). Only print selected non-secret flags via docker exec printenv.
  log "A_CONFIG_IMAGE=${CONFIG_IMAGE}"
  log "A_RUNNING_IMAGE_ID=${RUNNING_IMAGE_ID}"
  log "A_EXPECTED_IMAGE_ID=${EXPECTED_IMAGE_ID}"
  if [[ "$RUNNING_IMAGE_ID" == "$EXPECTED_IMAGE_ID" ]]; then
    log "A_IMAGE_MATCH=EXACT"
  else
    log "A_IMAGE_MATCH=DIVERGED"
  fi
  log "A_GIT_EXPECTED=${EXPECTED_GIT_SHA}"
fi
log

# --- B. Runtime flags (container printenv; values are booleans / enum, not secrets) ---
if [[ -n "${CID}" ]]; then
  for k in \
    SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
    PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
    PRICE_PLAN_TEST_ENTRY_ENABLED \
    WECHAT_VIRTUAL_PAY_ENV
  do
    v=$(docker exec "$CID" printenv "$k" 2>/dev/null || echo MISSING)
    log "B_${k}=${v}"
  done
  if docker exec "$CID" printenv SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED 2>/dev/null | grep -qx true \
    && docker exec "$CID" printenv PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED 2>/dev/null | grep -qx true \
    && docker exec "$CID" printenv PRICE_PLAN_TEST_ENTRY_ENABLED 2>/dev/null | grep -qx true \
    && docker exec "$CID" printenv WECHAT_VIRTUAL_PAY_ENV 2>/dev/null | grep -qx production; then
    log "B_FLAGS_RESULT=PASS expected true/true/true + production"
  else
    log "B_FLAGS_RESULT=CHECK — compare with evidence package; do not flip flags in this step"
  fi
fi
log

# --- C. DB schema / constraints (SELECT-only) ---
# Prefer explicit PSQL wrapper; else try common compose service name.
if [[ -z "${PSQL:-}" ]]; then
  PG_CID=$(docker ps -q --filter name=postgres | head -1 || true)
  if [[ -n "${PG_CID}" ]]; then
    PSQL="docker exec -i ${PG_CID} psql -U postgres -d zhiqiyun -v ON_ERROR_STOP=1"
    log "C_PSQL_AUTO=${PSQL}"
  else
    log "C_RESULT=SKIP set PSQL=... to run DB checks"
    PSQL=""
  fi
fi

if [[ -n "${PSQL}" ]]; then
  SQL_FILE="$EVIDENCE_DIR/p2-readonly.sql"
  cat > "$SQL_FILE" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT current_database() AS db, current_user AS db_user,
       pg_is_in_recovery() AS is_replica,
       current_setting('transaction_read_only') AS txn_ro;

-- 097 core tables present?
SELECT relname, to_regclass('public.'||relname) IS NOT NULL AS present
FROM (VALUES
  ('xz_price_plans'),
  ('xz_price_plan_versions'),
  ('xz_price_plan_payment_bindings'),
  ('xz_wechat_virtual_goods'),
  ('xz_price_plan_user_whitelist'),
  ('xz_order_price_quotes')
) AS t(relname);

-- constraints named *_097..*_100 that are NOT validated
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

-- agent NORMAL local price snapshot (no secrets)
SELECT code, status, enabled, is_default, price_cents, test_only
FROM xz_price_plans
WHERE code IN ('pp_agent_normal_prod_996','pp_agent_test_prod_1')
   OR code ILIKE '%agent%996%'
   OR code ILIKE '%agent%test%'
ORDER BY code
LIMIT 20;

SELECT g.product_id, g.platform_price_cents, g.status, g.environment
FROM xz_wechat_virtual_goods g
WHERE g.product_id IN ('AGENT_JOIN_996','AGENT_TEST_1YUAN','MEMBER_YEAR_996','MEMBER_TEST_1YUAN')
ORDER BY g.product_id, g.environment;

ROLLBACK;
SQL
  log "=== C_DB_OUTPUT ==="
  # shellcheck disable=SC2086
  $PSQL -f "$SQL_FILE" 2>&1 | tee -a "$OUT" | tee "$EVIDENCE_DIR/p2-db.out" >/dev/null || {
    log "C_RESULT=FAIL psql exited non-zero (still read-only attempt; inspect p2-db.out)"
  }
  if grep -q 'still_not_valid_097_100' "$EVIDENCE_DIR/p2-db.out" 2>/dev/null; then
    snv=$(awk '/still_not_valid_097_100/{getline; print; exit}' "$EVIDENCE_DIR/p2-db.out" 2>/dev/null || true)
    log "C_STILL_NOT_VALID_LINE=${snv:-see_p2-db.out}"
  fi
fi

log
log "=== P2 READ-ONLY END $(date -Iseconds) ==="
log "EVIDENCE_DIR=${EVIDENCE_DIR}"
log "PASS_HINT: A_IMAGE_MATCH=EXACT; B_FLAGS=true/true/true+production; C still_not_valid_097_100=0; AGENT_JOIN_996@99600 present"
log "NEXT_IF_PASS: P3/P4 human dual-sign + wechat console check → P6 GO"
echo "Wrote ${OUT}"
