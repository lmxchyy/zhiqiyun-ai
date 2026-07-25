-- AI smart-video media analysis and preprocessing, phase 2.
-- Analysis tasks are intentionally separate from render tasks: they have
-- independent source fingerprints, leases, retries and derived-file outputs.

begin;

alter table video_project_assets
  add column if not exists analysis_status text not null default 'PENDING',
  add column if not exists source_fingerprint text not null default '',
  add column if not exists normalized_metadata jsonb,
  add column if not exists filtered_probe_result jsonb,
  add column if not exists thumbnail_file_id text references xz_file_objects(file_id),
  add column if not exists proxy_file_id text references xz_file_objects(file_id),
  add column if not exists attempt_count integer not null default 0,
  add column if not exists error_code text,
  add column if not exists sanitized_error_message text,
  add column if not exists analyzer_version text,
  add column if not exists analysis_started_at timestamptz,
  add column if not exists analysis_finished_at timestamptz;

alter table video_project_assets drop constraint if exists chk_video_project_assets_analysis_status;
alter table video_project_assets add constraint chk_video_project_assets_analysis_status
  check (analysis_status in ('PENDING','QUEUED','RUNNING','SUCCEEDED','FAILED'));
alter table video_project_assets drop constraint if exists chk_video_project_assets_normalized_metadata;
alter table video_project_assets add constraint chk_video_project_assets_normalized_metadata
  check (normalized_metadata is null or jsonb_typeof(normalized_metadata) = 'object');
alter table video_project_assets drop constraint if exists chk_video_project_assets_filtered_probe;
alter table video_project_assets add constraint chk_video_project_assets_filtered_probe
  check (filtered_probe_result is null or jsonb_typeof(filtered_probe_result) = 'object');
alter table video_project_assets drop constraint if exists chk_video_project_assets_attempt_count;
alter table video_project_assets add constraint chk_video_project_assets_attempt_count
  check (attempt_count >= 0);

create index if not exists idx_video_project_assets_analysis
  on video_project_assets(tenant_id,user_id,project_id,analysis_status,updated_at);

create table if not exists video_asset_analysis_tasks (
  id text primary key,
  project_id text not null references video_projects(id),
  asset_id text not null references video_project_assets(id),
  tenant_id text not null,
  user_id text not null,
  source_file_id text not null references xz_file_objects(file_id),
  source_fingerprint text not null,
  client_request_id text not null default '',
  status text not null default 'PENDING',
  attempt_count integer not null default 0,
  max_attempts integer not null default 3,
  run_after timestamptz not null default now(),
  lease_owner text,
  lease_expires_at timestamptz,
  heartbeat_at timestamptz,
  analyzer_version text,
  error_code text,
  sanitized_error_message text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  unique(asset_id,source_fingerprint),
  constraint chk_video_asset_analysis_tasks_status check (
    status in ('PENDING','QUEUED','RUNNING','SUCCEEDED','FAILED')
  ),
  constraint chk_video_asset_analysis_tasks_attempts check (
    attempt_count >= 0 and max_attempts > 0 and attempt_count <= max_attempts
  )
);

create unique index if not exists uk_video_asset_analysis_request
  on video_asset_analysis_tasks(tenant_id,user_id,client_request_id,asset_id)
  where client_request_id <> '';
create index if not exists idx_video_asset_analysis_dispatch
  on video_asset_analysis_tasks(status,run_after,lease_expires_at)
  where status in ('PENDING','QUEUED','RUNNING');
create index if not exists idx_video_asset_analysis_project
  on video_asset_analysis_tasks(tenant_id,user_id,project_id,created_at desc);

commit;
