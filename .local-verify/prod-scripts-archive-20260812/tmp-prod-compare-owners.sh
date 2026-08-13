#!/usr/bin/env bash
set -euo pipefail
PSQL=(docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off)
"${PSQL[@]}" -c "SELECT id, tenant_id, user_id, title FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"
"${PSQL[@]}" -c "SELECT m.file_name, m.file_id, f.tenant_id, f.user_id, f.status, m.created_at FROM xz_multipart_uploads m JOIN xz_file_objects f ON f.file_id=m.file_id ORDER BY m.created_at DESC LIMIT 8;"
