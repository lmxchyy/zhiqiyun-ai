begin;
drop index if exists uq_xz_assets_smartvideo_render_task;
drop index if exists idx_video_render_tasks_recovery;
alter table video_render_tasks drop constraint if exists chk_video_render_tasks_attempts;
alter table video_render_tasks drop constraint if exists chk_video_render_tasks_status;
update video_render_tasks set status='RUNNING' where status in ('PROCESSING','RENDERING');
alter table video_render_tasks add constraint chk_video_render_tasks_status check (
 status in ('CREATED','QUEUED','RUNNING','UPLOADING','SUCCEEDED','FAILED','CANCELLED')
);
alter table video_render_tasks drop column if exists output_metadata, drop column if exists cover_file_id,
 drop column if exists heartbeat_at, drop column if exists lease_expires_at, drop column if exists lease_owner,
 drop column if exists run_after, drop column if exists max_attempts, drop column if exists attempt_count, drop column if exists step;
commit;
