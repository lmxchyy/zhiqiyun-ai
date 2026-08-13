#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
WK=zhiqiyun-ai-prod-smartvideo-worker-1
API=zhiqiyun-ai-prod-xianzhi-ai-1

echo "--- TASK FULL ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,progress,attempt_count,quoted_points,reserved_points,captured_points,released_points,billing_transaction_id,output_file_id,cover_file_id,work_id,error_code,error_message FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"

echo "--- RECENT API/SETTLE LOGS ---"
docker logs --since 10m "$WK" 2>&1 | grep -iE 'settle|publish|billing|points|work|SMARTVIDEO_RENDER_PUBLISH|error|panic' | tail -60 || true
docker logs --since 10m "$API" 2>&1 | grep -iE 'settle|publish|billing|smartvideo|render|points|work' | tail -40 || true

echo "--- PG ERRORS ---"
docker logs --since 15m "$PG" 2>&1 | grep -iE 'ERROR|DETAIL|STATEMENT' | tail -40 || true

echo "--- POINTS TX ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT table_name FROM information_schema.tables WHERE table_name ILIKE '%point%' OR table_name ILIKE '%ledger%' OR table_name ILIKE '%work%' ORDER BY 1 LIMIT 40;"

echo "DONE"
