#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "SELECT m.file_name, m.file_id FROM xz_multipart_uploads m LEFT JOIN video_project_assets a ON a.file_id=m.file_id AND a.project_id='vp_664248192f84dc96631df8cd' WHERE m.created_at > now() - interval '6 hours' AND a.id IS NULL ORDER BY m.created_at;"
