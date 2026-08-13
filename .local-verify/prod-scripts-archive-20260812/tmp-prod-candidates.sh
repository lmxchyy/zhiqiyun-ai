#!/usr/bin/env bash
set -euo pipefail
PROJECT_ID=vp_664248192f84dc96631df8cd
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off <<SQL
WITH project AS (
  SELECT id, tenant_id, user_id FROM video_projects WHERE id='${PROJECT_ID}'
),
existing AS (
  SELECT file_id FROM video_project_assets WHERE project_id='${PROJECT_ID}' AND deleted_at IS NULL
)
SELECT m.file_name, m.file_id, f.status,
       CASE WHEN lower(coalesce(f.mime_type,'')) LIKE 'video/%' OR lower(m.file_name) LIKE '%.mp4' THEN 'VIDEO' ELSE 'IMAGE' END AS asset_type
FROM xz_multipart_uploads m
JOIN xz_file_objects f ON f.file_id = m.file_id
JOIN project p ON p.tenant_id = f.tenant_id AND p.user_id = f.user_id
WHERE m.state='completed'
  AND f.status='ACTIVE'
  AND m.created_at > now() - interval '6 hours'
  AND NOT EXISTS (SELECT 1 FROM existing e WHERE e.file_id = m.file_id);
SQL
