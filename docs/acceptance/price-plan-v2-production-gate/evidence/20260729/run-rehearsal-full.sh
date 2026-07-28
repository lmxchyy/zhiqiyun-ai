#!/bin/bash
set -euo pipefail
EVID=/tmp/priceplan-evidence-20260729
BACKUP=/opt/zhiqiyun-ai/backups/postgres/db_2026-07-25_231939.sql
DB=zhiqiyun_rehearsal_202607290400
MIG=/tmp/priceplan-migrations
mkdir -p "$EVID/rehearsal"
docker rm -f priceplan-rehearsal-pg >/dev/null 2>&1 || true
docker run -d --name priceplan-rehearsal-pg \
  -e POSTGRES_PASSWORD=rehearsal \
  -e POSTGRES_USER=rehearsal \
  -e POSTGRES_DB="$DB" \
  pgvector/pgvector:pg16 >/dev/null
for i in $(seq 1 60); do
  docker exec priceplan-rehearsal-pg pg_isready -U rehearsal >/dev/null 2>&1 && break
  sleep 2
done
docker exec priceplan-rehearsal-pg pg_isready -U rehearsal
# Create roles referenced by dump owners/ACLs
docker exec priceplan-rehearsal-pg psql -U rehearsal -d postgres -v ON_ERROR_STOP=1 <<'SQL'
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'zhiqiyun_prod') THEN
    CREATE ROLE zhiqiyun_prod LOGIN PASSWORD 'rehearsal';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'readonly') THEN
    CREATE ROLE readonly NOLOGIN;
  END IF;
END $$;
SQL
echo "RESTORE_START $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee "$EVID/rehearsal/restore.meta"
START=$(date +%s)
set +e
docker exec -i priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -v ON_ERROR_STOP=0 < "$BACKUP" > "$EVID/rehearsal/restore.log" 2>&1
EC=$?
set -e
END=$(date +%s)
ERRS=$(grep -c '^ERROR:' "$EVID/rehearsal/restore.log" || true)
echo "RESTORE_EXIT=$EC RESTORE_SEC=$((END-START)) ERROR_LINES=$ERRS" | tee -a "$EVID/rehearsal/restore.meta"
grep '^ERROR:' "$EVID/rehearsal/restore.log" | head -40 | tee "$EVID/rehearsal/restore-errors.txt" || true
# Baseline counts
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -Atc "select 'orders='||count(*) from xz_orders; select 'plans='||count(*) from xz_plans; select 'amount_sum='||coalesce(sum(amount_cents),0) from xz_orders;" | tee "$EVID/rehearsal/baseline-before.txt"
# Apply migrations with timing
: > "$EVID/rehearsal/migrate.log"
for f in 097-member-agent-price-plan-v2.sql 098-price-plan-admin-governance.sql 099-price-plan-default-switch.sql 100-price-plan-test-whitelist-audit.sql; do
  echo "MIGRATE_START $f $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$EVID/rehearsal/migrate.meta"
  MS=$(date +%s)
  set +e
  docker exec -i priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -v ON_ERROR_STOP=1 < "$MIG/$f" >> "$EVID/rehearsal/migrate.log" 2>&1
  MEC=$?
  set -e
  ME=$(date +%s)
  echo "MIGRATE_END $f EXIT=$MEC SEC=$((ME-MS))" | tee -a "$EVID/rehearsal/migrate.meta"
  if [ "$MEC" -ne 0 ]; then
    echo "MIGRATE_FAILED $f" | tee -a "$EVID/rehearsal/migrate.meta"
    tail -40 "$EVID/rehearsal/migrate.log"
    exit "$MEC"
  fi
done
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -Atc "select 'orders='||count(*) from xz_orders; select 'plans='||count(*) from xz_plans; select 'amount_sum='||coalesce(sum(amount_cents),0) from xz_orders; select 'price_plans='||count(*) from xz_price_plans; select 'goods='||count(*) from xz_wechat_virtual_goods;" | tee "$EVID/rehearsal/baseline-after.txt"
# NOT VALID constraints
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -c "SELECT conrelid::regclass AS table_name, conname, convalidated FROM pg_constraint WHERE contype='c' AND NOT convalidated AND conrelid::regclass::text LIKE 'xz_%' ORDER BY 1,2;" | tee "$EVID/rehearsal/not-valid-before-validate.txt"
# Validate
: > "$EVID/rehearsal/validate.log"
VS=$(date +%s)
set +e
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -v ON_ERROR_STOP=1 <<'SQL' >> "$EVID/rehearsal/validate.log" 2>&1
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT conrelid::regclass AS tbl, conname
    FROM pg_constraint
    WHERE contype = 'c' AND NOT convalidated
      AND conrelid::regclass::text LIKE 'xz_%'
    ORDER BY 1, 2
  LOOP
    EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', r.tbl, r.conname);
    RAISE NOTICE 'validated %.%', r.tbl, r.conname;
  END LOOP;
END $$;
SQL
VEC=$?
set -e
VE=$(date +%s)
echo "VALIDATE_EXIT=$VEC SEC=$((VE-VS))" | tee -a "$EVID/rehearsal/migrate.meta"
tail -50 "$EVID/rehearsal/validate.log"
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -c "SELECT count(*) AS still_not_valid FROM pg_constraint WHERE contype='c' AND NOT convalidated AND conrelid::regclass::text LIKE 'xz_%';" | tee "$EVID/rehearsal/not-valid-after-validate.txt"
# Second restore rehearsal proof: recreate empty DB and restore again briefly timed
docker exec priceplan-rehearsal-pg psql -U rehearsal -d postgres -v ON_ERROR_STOP=1 -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='${DB}_copy' AND pid <> pg_backend_pid();" >/dev/null 2>&1 || true
docker exec priceplan-rehearsal-pg psql -U rehearsal -d postgres -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS ${DB}_copy;"
docker exec priceplan-rehearsal-pg psql -U rehearsal -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE ${DB}_copy OWNER rehearsal;"
RS2=$(date +%s)
set +e
docker exec -i priceplan-rehearsal-pg psql -U rehearsal -d "${DB}_copy" -v ON_ERROR_STOP=0 < "$BACKUP" > "$EVID/rehearsal/restore-second.log" 2>&1
EC2=$?
set -e
RE2=$(date +%s)
echo "SECOND_RESTORE_EXIT=$EC2 SEC=$((RE2-RS2)) ERR=$(grep -c '^ERROR:' "$EVID/rehearsal/restore-second.log" || true)" | tee -a "$EVID/rehearsal/restore.meta"
echo DONE
