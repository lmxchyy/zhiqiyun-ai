#!/usr/bin/env bash
set -euo pipefail
PSQL=(docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off)
"${PSQL[@]}" -c "SELECT column_name, data_type FROM information_schema.columns WHERE table_name='xz_file_objects' ORDER BY ordinal_position;"
"${PSQL[@]}" -c "SELECT m.file_name, m.file_id AS mpu_file, m.state, f.file_id AS obj, f.status FROM xz_multipart_uploads m LEFT JOIN xz_file_objects f ON f.file_id=m.file_id ORDER BY m.created_at DESC LIMIT 10;"
"${PSQL[@]}" -c "SELECT file_id, status FROM xz_file_objects WHERE file_id='file_8f7500955c16de30ba50fb7f';"
