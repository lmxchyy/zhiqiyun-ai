#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
echo "--- xz_file_objects cols ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT column_name FROM information_schema.columns WHERE table_name='xz_file_objects' ORDER BY ordinal_position;"
echo "--- FILES ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT * FROM xz_file_objects WHERE file_id IN ('file_451ba340d17175e3c2e8cfe0','file_6edd1723394f45f198dc2d9c') LIMIT 5;"
echo "DONE"
