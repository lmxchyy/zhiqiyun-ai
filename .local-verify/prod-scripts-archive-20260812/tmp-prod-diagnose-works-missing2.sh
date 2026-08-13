#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
API=zhiqiyun-ai-prod-xianzhi-ai-1

echo "HEAD=$(cd /opt/zhiqiyun-ai && git rev-parse --short HEAD)"
echo "--- ASSET ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,user_id,tenant_id,media_type,deleted_at,created_at,left(url,50) AS url,left(thumbnail_url,50) AS thumb,metadata->>'type' AS typ FROM xz_assets WHERE id='asset_fbc47867bd25964261f742fd';"

echo "--- USER ASSET COUNTS ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT user_id, count(*) FILTER (WHERE deleted_at is null) AS alive, count(*) FILTER (WHERE deleted_at is null AND metadata->>'type'='SMART_VIDEO_MONTAGE') AS montage FROM xz_assets GROUP BY user_id ORDER BY alive DESC LIMIT 10;"

echo "--- LISTASSETSFORUSER ORDER TOP 5 for user_000003 ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,name,media_type,metadata->>'type' AS typ,created_at FROM xz_assets WHERE user_id='user_000003' AND deleted_at IS NULL ORDER BY created_at DESC NULLS LAST LIMIT 8;"

echo "--- FILE OBJECTS ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT column_name FROM information_schema.columns WHERE table_name='xz_file_objects' ORDER BY 1;"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT * FROM xz_file_objects WHERE file_id IN ('file_451ba340d17175e3c2e8cfe0','file_6edd1723394f45f198dc2d9c')\gx" 2>/dev/null | head -80 || \
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT file_id,user_id,tenant_id,original_name,mime_type,size_bytes,visibility FROM xz_file_objects WHERE file_id IN ('file_451ba340d17175e3c2e8cfe0','file_6edd1723394f45f198dc2d9c');" 2>/dev/null || true

echo "--- FRONT BUNDLE CONTAINS montageWork? ---"
docker exec "$API" sh -c 'grep -R "montageWork\|SMART_VIDEO_MONTAGE\|AI自动混剪" /app/dist /usr/share/nginx/html /var/www 2>/dev/null | head -20' || true
# find static path
docker exec "$API" sh -c 'find / -name "index.html" 2>/dev/null | head -20'
echo "DONE"
