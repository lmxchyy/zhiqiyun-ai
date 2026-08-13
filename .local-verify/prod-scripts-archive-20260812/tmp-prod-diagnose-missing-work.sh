#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
TASK=svrender_044a0e2b4a72975352db68a7
echo "--- RENDER TASK ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,progress,user_id,tenant_id,work_id,output_asset_id,output_file_id,cover_file_id,captured_points,updated_at FROM video_render_tasks WHERE id='$TASK';"
echo "--- PROJECT ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,user_id,tenant_id,output_asset_id,active_render_task_id FROM video_projects WHERE id=(SELECT project_id FROM video_render_tasks WHERE id='$TASK');"
echo "--- ASSET BY WORK_ID ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,user_id,tenant_id,task_id,name,media_type,url,thumbnail_url,favorite,deleted_at,created_at,left(metadata::text,300) AS meta FROM xz_assets WHERE id='asset_fbc47867bd25964261f742fd' OR task_id='$TASK' OR coalesce(metadata->>'renderTaskId','')='$TASK';"
echo "--- RECENT ASSETS FOR SAME USER ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,name,media_type,deleted_at,created_at,left(coalesce(metadata->>'type',''),40) AS typ,left(coalesce(url,''),60) AS url FROM xz_assets WHERE user_id=(SELECT user_id FROM video_render_tasks WHERE id='$TASK') ORDER BY created_at DESC LIMIT 15;"
echo "--- FILE ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,user_id,tenant_id,file_name,mime_type,file_size,visibility,business_type,deleted_at FROM files WHERE id IN (SELECT output_file_id FROM video_render_tasks WHERE id='$TASK' UNION SELECT cover_file_id FROM video_render_tasks WHERE id='$TASK');" 2>/dev/null \
|| docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT table_name FROM information_schema.tables WHERE table_name ILIKE '%file%' ORDER BY 1 LIMIT 30;"
echo "DONE"
