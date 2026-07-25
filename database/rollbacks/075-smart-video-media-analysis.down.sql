begin;

drop table if exists video_asset_analysis_tasks;
drop index if exists idx_video_project_assets_analysis;

alter table video_project_assets
  drop constraint if exists chk_video_project_assets_attempt_count,
  drop constraint if exists chk_video_project_assets_filtered_probe,
  drop constraint if exists chk_video_project_assets_normalized_metadata,
  drop constraint if exists chk_video_project_assets_analysis_status,
  drop column if exists analysis_finished_at,
  drop column if exists analysis_started_at,
  drop column if exists analyzer_version,
  drop column if exists sanitized_error_message,
  drop column if exists error_code,
  drop column if exists attempt_count,
  drop column if exists proxy_file_id,
  drop column if exists thumbnail_file_id,
  drop column if exists filtered_probe_result,
  drop column if exists normalized_metadata,
  drop column if exists source_fingerprint,
  drop column if exists analysis_status;

commit;
