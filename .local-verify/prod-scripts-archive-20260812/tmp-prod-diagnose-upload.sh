#!/usr/bin/env bash
set -euo pipefail
echo "=== multipart tables ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT state, count(*) FROM xz_multipart_uploads GROUP BY 1 ORDER BY 1;"
echo "=== recent multipart ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, state, file_name, total_size, left(coalesce(provider_upload_id,''),40) provider, created_at FROM xz_multipart_uploads ORDER BY created_at DESC LIMIT 10;"
echo "=== minio ping ==="
docker exec zhiqiyun-ai-prod-minio-1 mc --version 2>/dev/null | head -1 || true
docker inspect --format '{{.State.Health.Status}}' zhiqiyun-ai-prod-minio-1
echo "=== api err sample via recreate request log? ==="
# gin may not log; check if SMARTVIDEO / storage env present
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 printenv | grep -iE 'MINIO|S3|STORAGE|MULTIPART|SMARTVIDEO' | sed 's/=.*/=***/' | head -40
