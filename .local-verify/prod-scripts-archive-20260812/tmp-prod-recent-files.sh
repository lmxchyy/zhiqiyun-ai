#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<'SQL'
SELECT column_name FROM information_schema.columns WHERE table_name='xz_file_objects' ORDER BY 1;
SQL
echo '==== recent files ===='
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<'SQL'
SELECT file_id, coalesce(original_filename, file_name, '') AS name, status, size_bytes, content_type, created_at
FROM xz_file_objects
WHERE created_at > now() - interval '3 hours'
ORDER BY created_at DESC
LIMIT 20;
SQL
