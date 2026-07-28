#!/bin/bash
set -euo pipefail
EVID=/tmp/priceplan-evidence-20260729
BACKUP=/opt/zhiqiyun-ai/backups/postgres/db_2026-07-25_231939.sql
DB=zhiqiyun_rehearsal_202607290400
mkdir -p "$EVID/rehearsal" "$EVID/migrations"
docker rm -f priceplan-rehearsal-pg >/dev/null 2>&1 || true
docker run -d --name priceplan-rehearsal-pg \
  -e POSTGRES_PASSWORD=rehearsal \
  -e POSTGRES_USER=rehearsal \
  -e POSTGRES_DB="$DB" \
  pgvector/pgvector:pg16 >/dev/null
for i in $(seq 1 60); do
  if docker exec priceplan-rehearsal-pg pg_isready -U rehearsal >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
docker exec priceplan-rehearsal-pg pg_isready -U rehearsal
echo "RESTORE_START $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee "$EVID/rehearsal/restore.meta"
START=$(date +%s)
set +e
docker exec -i priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -v ON_ERROR_STOP=1 < "$BACKUP" > "$EVID/rehearsal/restore.log" 2>&1
EC=$?
set -e
END=$(date +%s)
echo "RESTORE_EXIT=$EC RESTORE_SEC=$((END-START))" | tee -a "$EVID/rehearsal/restore.meta"
tail -30 "$EVID/rehearsal/restore.log"
if [ "$EC" -ne 0 ]; then
  echo "RESTORE_FAILED" | tee -a "$EVID/rehearsal/restore.meta"
  exit "$EC"
fi
echo "RESTORE_OK" | tee -a "$EVID/rehearsal/restore.meta"
