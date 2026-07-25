begin;
alter table video_render_tasks
  add column if not exists step text not null default 'created',
  add column if not exists attempt_count integer not null default 0,
  add column if not exists max_attempts integer not null default 3,
  add column if not exists run_after timestamptz not null default now(),
  add column if not exists lease_owner text,
  add column if not exists lease_expires_at timestamptz,
  add column if not exists heartbeat_at timestamptz,
  add column if not exists cover_file_id text references xz_file_objects(file_id),
  add column if not exists output_metadata jsonb not null default '{}'::jsonb;
update video_render_tasks set status='PROCESSING' where status='RUNNING';
alter table video_render_tasks drop constraint if exists chk_video_render_tasks_status;
alter table video_render_tasks add constraint chk_video_render_tasks_status check (
 status in ('CREATED','QUEUED','PROCESSING','RENDERING','UPLOADING','SUCCEEDED','FAILED','CANCELLED')
);
alter table video_render_tasks add constraint chk_video_render_tasks_attempts check (attempt_count >= 0 and max_attempts between 1 and 20);
create index if not exists idx_video_render_tasks_recovery on video_render_tasks(status,run_after,lease_expires_at)
 where status in ('CREATED','QUEUED','PROCESSING','RENDERING','UPLOADING');
create unique index if not exists uq_xz_assets_smartvideo_render_task
 on xz_assets(task_id) where metadata->>'source'='smart_video' and deleted_at is null;
commit;
