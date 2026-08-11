-- Rollback 106: AI auto montage V1.
-- Note: This rollback is provided for development environments only.
-- Production rollbacks should not drop user data; use feature flags instead.

begin;

-- Restore old check constraint
alter table video_render_tasks drop constraint if exists chk_video_render_tasks_status;
alter table video_render_tasks add constraint chk_video_render_tasks_status check (
  status in ('CREATED','QUEUED','PROCESSING','RENDERING','UPLOADING','SUCCEEDED','FAILED','CANCELLED')
);

alter table video_render_tasks
  drop column if exists stage,
  drop column if exists voice_file_id,
  drop column if exists caption_file_id,
  drop column if exists work_id,
  drop column if exists billing_transaction_id,
  drop column if exists cancel_requested_at,
  drop column if exists retry_of_task_id,
  drop column if exists attempt,
  drop column if exists manifest_hash,
  drop column if exists quoted_points,
  drop column if exists reserved_points,
  drop column if exists captured_points,
  drop column if exists released_points;

drop table if exists video_task_outbox;
drop table if exists video_plan_tasks;

alter table video_project_versions
  drop column if exists source,
  drop column if exists parent_version_id,
  drop column if exists plan_schema_version,
  drop column if exists plan_snapshot,
  drop column if exists render_manifest,
  drop column if exists manifest_hash,
  drop column if exists planner_model_key,
  drop column if exists planner_request_id,
  drop column if exists change_note;

alter table video_project_assets
  drop column if exists kind,
  drop column if exists order_index,
  drop column if exists analysis_summary,
  drop column if exists representative_frame_file_ids,
  drop column if exists content_audit_status,
  drop column if exists duration_ms,
  drop column if exists deleted_at;

alter table video_projects drop constraint if exists chk_video_projects_status;
alter table video_projects
  drop column if exists target_spec,
  drop column if exists current_version_id,
  drop column if exists confirmed_version_id,
  drop column if exists active_analysis_task_id,
  drop column if exists active_plan_task_id,
  drop column if exists error_stage;
alter table video_projects add constraint chk_video_projects_status check (
  status in ('DRAFT','ANALYZING','STORYBOARD_READY','CONFIRMED','RENDERING','COMPLETED','FAILED')
);

commit;
