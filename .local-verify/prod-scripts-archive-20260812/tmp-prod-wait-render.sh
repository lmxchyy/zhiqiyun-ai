#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
WK=zhiqiyun-ai-prod-smartvideo-worker-1
for i in 1 2 3 4 5 6 7 8 9 10 11 12; do
  row=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -t -A -F'|' -c \
    "SELECT status,stage,progress,coalesce(error_code,''),left(coalesce(error_message,''),120) FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
  echo "[$i] $row"
  status=${row%%|*}
  if [ "$status" = "SUCCEEDED" ] || [ "$status" = "FAILED" ]; then
    break
  fi
  sleep 15
done
echo "--- FINAL ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,progress,coalesce(work_id,'') AS work_id,coalesce(output_file_id,'') AS output_file_id,error_code,left(coalesce(error_message,''),160) AS err FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- LOGS ---"
docker logs --since 5m "$WK" 2>&1 | grep -iE 'smartvideo_render|speech|ffmpeg|complete|failed|error' | tail -40 || true
echo "DONE"
