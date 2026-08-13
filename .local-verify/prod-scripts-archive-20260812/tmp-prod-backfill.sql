INSERT INTO video_project_assets (
  id, project_id, tenant_id, user_id, file_id, storage_key, asset_type, kind,
  sort_order, order_index, metadata, analysis_status, content_audit_status, duration_ms, created_at, updated_at
)
SELECT
  'vpa_' || substr(md5(f.file_id || ':backfill2'), 1, 24),
  'vp_664248192f84dc96631df8cd',
  f.tenant_id,
  f.user_id,
  f.file_id,
  f.object_key,
  CASE WHEN lower(coalesce(f.mime_type,'')) LIKE 'video/%' OR lower(m.file_name) LIKE '%.mp4' THEN 'VIDEO' ELSE 'IMAGE' END,
  CASE WHEN lower(coalesce(f.mime_type,'')) LIKE 'video/%' OR lower(m.file_name) LIKE '%.mp4' THEN 'video' ELSE 'image' END,
  (SELECT coalesce(max(order_index), -1) FROM video_project_assets WHERE project_id='vp_664248192f84dc96631df8cd' AND deleted_at IS NULL)
    + row_number() OVER (ORDER BY m.created_at),
  (SELECT coalesce(max(order_index), -1) FROM video_project_assets WHERE project_id='vp_664248192f84dc96631df8cd' AND deleted_at IS NULL)
    + row_number() OVER (ORDER BY m.created_at),
  jsonb_build_object('originalName', m.file_name, 'mimeType', f.mime_type, 'fileSize', f.file_size),
  'PENDING',
  'pending',
  0,
  now(),
  now()
FROM xz_multipart_uploads m
JOIN xz_file_objects f ON f.file_id = m.file_id
LEFT JOIN video_project_assets a
  ON a.file_id = m.file_id AND a.project_id = 'vp_664248192f84dc96631df8cd' AND a.deleted_at IS NULL
WHERE m.created_at > now() - interval '6 hours'
  AND f.status = 'ACTIVE'
  AND a.id IS NULL;

SELECT id, kind, asset_type, analysis_status, metadata->>'originalName' AS name
FROM video_project_assets
WHERE project_id='vp_664248192f84dc96631df8cd' AND deleted_at IS NULL
ORDER BY order_index;
