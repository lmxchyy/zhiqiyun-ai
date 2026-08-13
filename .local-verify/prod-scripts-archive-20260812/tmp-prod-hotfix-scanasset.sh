#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai
if ! git diff --quiet || ! git diff --cached --quiet; then
  git checkout -- .
fi
bash ./backup.sh
GIT_REMOTE=origin GIT_BRANCH=codex/channel-ecosystem-v132-phase3 bash ./deploy.sh
echo "DEPLOY_DONE=$(git rev-parse --short HEAD)"
curl -fsS http://127.0.0.1:3100/api/v1/health; echo

# Attach already-uploaded ACTIVE files that failed addAsset due to scan bug.
PROJECT_ID=vp_664248192f84dc96631df8cd
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 <<SQL
WITH project AS (
  SELECT id, tenant_id, user_id FROM video_projects WHERE id='${PROJECT_ID}'
),
existing AS (
  SELECT file_id FROM video_project_assets WHERE project_id='${PROJECT_ID}' AND deleted_at IS NULL
),
candidates AS (
  SELECT m.file_id, m.file_name, f.object_key, f.mime_type, f.file_size, f.tenant_id, f.user_id,
         CASE WHEN lower(coalesce(f.mime_type,'')) LIKE 'video/%' OR lower(m.file_name) LIKE '%.mp4' THEN 'VIDEO' ELSE 'IMAGE' END AS asset_type,
         row_number() OVER (ORDER BY m.created_at) AS rn
  FROM xz_multipart_uploads m
  JOIN xz_file_objects f ON f.file_id = m.file_id
  JOIN project p ON p.tenant_id = f.tenant_id AND p.user_id = f.user_id
  WHERE m.state='completed'
    AND f.status='ACTIVE'
    AND m.created_at > now() - interval '6 hours'
    AND NOT EXISTS (SELECT 1 FROM existing e WHERE e.file_id = m.file_id)
),
base_order AS (
  SELECT coalesce(max(order_index), -1) AS max_order FROM video_project_assets WHERE project_id='${PROJECT_ID}' AND deleted_at IS NULL
)
INSERT INTO video_project_assets (
  id, project_id, tenant_id, user_id, file_id, storage_key, asset_type, kind,
  sort_order, order_index, metadata, analysis_status, content_audit_status, duration_ms, created_at, updated_at
)
SELECT
  'vpa_' || substr(md5(c.file_id || ':backfill'), 1, 24),
  '${PROJECT_ID}',
  c.tenant_id,
  c.user_id,
  c.file_id,
  c.object_key,
  c.asset_type,
  lower(c.asset_type),
  (b.max_order + c.rn)::int,
  (b.max_order + c.rn)::int,
  jsonb_build_object('originalName', c.file_name, 'mimeType', c.mime_type, 'fileSize', c.file_size),
  'PENDING',
  'pending',
  0,
  now(),
  now()
FROM candidates c CROSS JOIN base_order b;

SELECT id, kind, asset_type, analysis_status, file_id FROM video_project_assets WHERE project_id='${PROJECT_ID}' AND deleted_at IS NULL ORDER BY order_index;
SQL

docker compose -f compose.prod.yml --env-file .env.production ps xianzhi-ai smartvideo-worker
