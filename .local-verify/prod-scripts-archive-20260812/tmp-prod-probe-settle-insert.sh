#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1

echo "--- xz_assets columns ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name='xz_assets' ORDER BY ordinal_position;"

echo "--- try insert like publisher ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE
  uid text;
  tid text;
  vid text;
  cid text;
BEGIN
  SELECT user_id, tenant_id, output_file_id, cover_file_id
    INTO uid, tid, vid, cid
    FROM video_render_tasks
   WHERE id='svrender_044a0e2b4a72975352db68a7';
  RAISE NOTICE 'user=% tenant=% video=% cover=%', uid, tid, vid, cid;
  BEGIN
    INSERT INTO xz_assets
      (id,user_id,tenant_id,organization_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,deleted_at,created_at,updated_at,raw)
    VALUES
      ('asset_probe_settle_test', uid, nullif(tid,''), null, 'svrender_044a0e2b4a72975352db68a7',
       'probe', 'video', 'storage://'||vid, 'storage://'||cid, false,
       '{"type":"SMART_VIDEO_MONTAGE","renderTaskId":"svrender_044a0e2b4a72975352db68a7"}'::jsonb,
       null, now(), now(), '{}'::jsonb);
    RAISE NOTICE 'insert_ok';
    DELETE FROM xz_assets WHERE id='asset_probe_settle_test';
  EXCEPTION WHEN others THEN
    RAISE NOTICE 'insert_fail: %', SQLERRM;
  END;
END $$;
SQL

echo "--- reservation ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT * FROM xz_personal_point_reservations WHERE id='reservation_6fbbe9cbd268d3a83d4fdef0c8d6458a' OR business_id='svrender_044a0e2b4a72975352db68a7' LIMIT 5;"

echo "DONE"
