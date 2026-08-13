#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1

UID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT user_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
TID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT coalesce(tenant_id,'') FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
VID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT output_file_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
CID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "SELECT cover_file_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
echo "uid=$UID tid=$TID vid=$VID cid=$CID"

docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "INSERT INTO xz_assets (id,user_id,tenant_id,organization_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,deleted_at,created_at,updated_at,raw) VALUES ('asset_probe_settle_test', '$UID', nullif('$TID',''), null, 'svrender_044a0e2b4a72975352db68a7', 'probe', 'video', 'storage://$VID', 'storage://$CID', false, '{\"type\":\"SMART_VIDEO_MONTAGE\",\"renderTaskId\":\"svrender_044a0e2b4a72975352db68a7\"}'::jsonb, null, to_char(now(),'YYYY-MM-DD\"T\"HH24:MI:SS.MS\"Z\"'), to_char(now(),'YYYY-MM-DD\"T\"HH24:MI:SS.MS\"Z\"'), '{}'::jsonb);" \
  && echo INSERT_OK || echo INSERT_FAIL

docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -c "DELETE FROM xz_assets WHERE id='asset_probe_settle_test';" || true

# Also try with timestamptz now() into text columns
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "INSERT INTO xz_assets (id,user_id,tenant_id,organization_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,deleted_at,created_at,updated_at,raw) VALUES ('asset_probe_settle_test2', '$UID', nullif('$TID',''), null, 'svrender_044a0e2b4a72975352db68a7', 'probe2', 'video', 'storage://$VID', 'storage://$CID', false, '{\"type\":\"SMART_VIDEO_MONTAGE\",\"renderTaskId\":\"svrender_044a0e2b4a72975352db68a7\"}'::jsonb, null, now()::text, now()::text, '{}'::jsonb);" \
  && echo INSERT2_OK || echo INSERT2_FAIL
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -c "DELETE FROM xz_assets WHERE id like 'asset_probe_settle_test%';" || true

echo "--- existing asset for task ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -c \
  "SELECT id, name, left(metadata::text,120) FROM xz_assets WHERE coalesce(metadata->>'renderTaskId','')='svrender_044a0e2b4a72975352db68a7' OR task_id='svrender_044a0e2b4a72975352db68a7';"

echo "DONE"
