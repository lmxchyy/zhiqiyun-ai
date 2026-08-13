#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<'SQL'
\d xz_file_objects
SQL
echo '==== by mtime ===='
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<'SQL'
SELECT *
FROM xz_file_objects
ORDER BY 1 DESC
LIMIT 1;
SQL
echo '==== count recent ===='
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -Atc \
  "SELECT count(*) FROM xz_file_objects;"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<'SQL'
SELECT m.file_name, m.file_id AS mpu_file_id, m.state AS mpu_state, f.file_id AS obj_id, f.status
FROM xz_multipart_uploads m
LEFT JOIN xz_file_objects f ON f.file_id = m.file_id
ORDER BY m.created_at DESC
LIMIT 10;
SQL
