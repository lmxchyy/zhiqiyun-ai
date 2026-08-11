#!/bin/bash
set -euo pipefail
EVID=/tmp/priceplan-evidence-20260729
DB=zhiqiyun_rehearsal_202607290400
: > "$EVID/rehearsal/validate2.log"
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -AtF $'\t' -c \
  "SELECT conrelid::regclass::text, conname FROM pg_constraint WHERE contype='c' AND NOT convalidated AND conrelid::regclass::text LIKE 'xz_%' ORDER BY 1,2;" \
  | while IFS=$'\t' read -r tbl con; do
      echo "VALIDATING $tbl $con" | tee -a "$EVID/rehearsal/validate2.log"
      set +e
      docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -v ON_ERROR_STOP=1 \
        -c "ALTER TABLE ${tbl} VALIDATE CONSTRAINT ${con};" >> "$EVID/rehearsal/validate2.log" 2>&1
      EC=$?
      set -e
      echo "RESULT $tbl $con EXIT=$EC" | tee -a "$EVID/rehearsal/validate2.log"
    done
docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -c \
  "SELECT conrelid::regclass AS table_name, conname, convalidated FROM pg_constraint WHERE NOT convalidated AND conrelid::regclass::text LIKE 'xz_%' ORDER BY 1,2;" \
  | tee "$EVID/rehearsal/not-valid-final.txt"
COUNT=$(docker exec priceplan-rehearsal-pg psql -U rehearsal -d "$DB" -Atc "SELECT count(*) FROM pg_constraint WHERE contype IN ('c','f') AND NOT convalidated AND conrelid::regclass::text LIKE 'xz_%';")
echo "STILL_NOT_VALID=$COUNT" | tee "$EVID/rehearsal/still-not-valid-count.txt"
grep -E 'ERROR|RESULT|VALIDATE' "$EVID/rehearsal/validate2.log" | head -100
