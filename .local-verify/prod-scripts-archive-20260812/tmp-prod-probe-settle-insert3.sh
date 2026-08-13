#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1

USER_ID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT user_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
TENANT_ID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT coalesce(tenant_id,'') FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
VIDEO_ID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT output_file_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
COVER_ID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT cover_file_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
echo "user=$USER_ID tenant=$TENANT_ID video=$VIDEO_ID cover=$COVER_ID"

set +e
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "INSERT INTO xz_assets (id,user_id,tenant_id,organization_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,deleted_at,created_at,updated_at,raw) VALUES ('asset_probe_settle_test', '$USER_ID', nullif('$TENANT_ID',''), null, 'svrender_044a0e2b4a72975352db68a7', 'probe', 'video', 'storage://$VIDEO_ID', 'storage://$COVER_ID', false, '{\"type\":\"SMART_VIDEO_MONTAGE\",\"renderTaskId\":\"svrender_044a0e2b4a72975352db68a7\"}'::jsonb, null, now(), now(), '{}'::jsonb);"
echo "insert_now_exit=$?"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -c "DELETE FROM xz_assets WHERE id='asset_probe_settle_test';" >/dev/null

docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "INSERT INTO xz_assets (id,user_id,tenant_id,organization_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,deleted_at,created_at,updated_at,raw) VALUES ('asset_probe_settle_test', '$USER_ID', nullif('$TENANT_ID',''), null, 'svrender_044a0e2b4a72975352db68a7', 'probe', 'video', 'storage://$VIDEO_ID', 'storage://$COVER_ID', false, '{\"type\":\"SMART_VIDEO_MONTAGE\",\"renderTaskId\":\"svrender_044a0e2b4a72975352db68a7\"}'::jsonb, null, now()::text, now()::text, '{}'::jsonb);"
echo "insert_text_exit=$?"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -c "DELETE FROM xz_assets WHERE id='asset_probe_settle_test';" >/dev/null
set -e
echo DONE
