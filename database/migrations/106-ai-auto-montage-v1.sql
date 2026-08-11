-- AI auto montage V1: domain model upgrade.
-- Replaces Smoke-era data model with immutable versions, plan tasks, outbox, and render pipeline stages.
-- Drop: video_storyboard_scenes (replaced by plan_snapshot jsonb in versions).

begin;

-- ============================================================
-- video_projects: new statuses, target_spec, version tracking
-- ============================================================
alter table video_projects drop constraint if exists chk_video_projects_status;
alter table video_projects add column if not exists target_spec jsonb;
alter table video_projects add column if not exists current_version_id text;
alter table video_projects add column if not exists confirmed_version_id text;
alter table video_projects add column if not exists active_analysis_task_id text;
alter table video_projects add column if not exists active_plan_task_id text;
alter table video_projects add column if not exists error_stage varchar(32);

alter table video_projects add constraint chk_video_projects_status check (
  status in ('DRAFT','ANALYZING','MATERIAL_READY','PLANNING','STORYBOARD_READY','CONFIRMED','RENDERING','COMPLETED','FAILED')
);

create index if not exists idx_video_projects_status_time
  on video_projects(tenant_id, status, updated_at desc)
  where deleted_at is null;

-- ============================================================
-- video_project_assets: analysis summary, content audit, duration
-- ============================================================
alter table video_project_assets
  add column if not exists kind varchar(16),
  add column if not exists order_index integer not null default 0,
  add column if not exists analysis_summary jsonb,
  add column if not exists representative_frame_file_ids jsonb,
  add column if not exists content_audit_status varchar(24) not null default 'pending',
  add column if not exists duration_ms bigint not null default 0,
  add column if not exists deleted_at timestamptz;

update video_project_assets set kind = lower(coalesce(asset_type,'image')) where kind is null;
update video_project_assets set order_index = sort_order where order_index = 0 and sort_order <> 0;
alter table video_project_assets alter column kind set not null;

alter table video_project_assets drop constraint if exists chk_video_project_assets_content_audit;
alter table video_project_assets add constraint chk_video_project_assets_content_audit
  check (content_audit_status in ('pending','passed','rejected'));

drop index if exists idx_video_project_assets_order;
create unique index if not exists uq_video_project_assets_order
  on video_project_assets(project_id, order_index) where deleted_at is null;

-- ============================================================
-- video_project_versions: immutable plan snapshots
-- ============================================================
drop table if exists video_storyboard_scenes;

alter table video_project_versions drop constraint if exists chk_video_project_versions_status;
alter table video_project_versions drop constraint if exists chk_video_project_versions_storyboard;
alter table video_project_versions
  add column if not exists source varchar(16) not null default 'ai',
  add column if not exists parent_version_id text,
  add column if not exists plan_schema_version integer,
  add column if not exists plan_snapshot jsonb,
  add column if not exists render_manifest jsonb,
  add column if not exists manifest_hash char(64),
  add column if not exists planner_model_key varchar(128),
  add column if not exists planner_request_id varchar(128),
  add column if not exists change_note text;

alter table video_project_versions add constraint chk_video_project_versions_source
  check (source in ('ai','user'));
alter table video_project_versions add constraint chk_video_project_versions_plan_schema
  check (plan_schema_version is null or plan_schema_version > 0);

-- ============================================================
-- video_plan_tasks: AI planning job table
-- ============================================================
create table if not exists video_plan_tasks (
  id text primary key,
  tenant_id text not null,
  project_id text not null references video_projects(id),
  user_id text not null,
  state varchar(24) not null default 'CREATED',
  instruction text not null default '',
  source_version_id text,
  output_version_id text,
  model_key varchar(128),
  provider_request_id varchar(128),
  attempt integer not null default 1,
  progress integer not null default 0,
  plan_snapshot jsonb,
  error_code text,
  error_message text,
  lease_owner text,
  lease_expires_at timestamptz,
  heartbeat_at timestamptz,
  idempotency_key varchar(128) not null,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  constraint chk_video_plan_tasks_state check (
    state in ('CREATED','QUEUED','PROCESSING','SUCCEEDED','FAILED')
  ),
  constraint chk_video_plan_tasks_progress check (progress between 0 and 100),
  constraint chk_video_plan_tasks_attempt check (attempt >= 1)
);

create unique index if not exists uq_video_plan_tasks_idempotency
  on video_plan_tasks(tenant_id, user_id, idempotency_key);
create index if not exists idx_video_plan_tasks_dispatch
  on video_plan_tasks(state, lease_expires_at, created_at)
  where state in ('CREATED','QUEUED','PROCESSING');
create index if not exists idx_video_plan_tasks_project
  on video_plan_tasks(tenant_id, user_id, project_id, created_at desc);

-- ============================================================
-- video_task_outbox: transactional job dispatch
-- ============================================================
create table if not exists video_task_outbox (
  id bigserial primary key,
  tenant_id text not null,
  aggregate_type varchar(32) not null,
  aggregate_id text not null,
  event_type varchar(64) not null,
  payload jsonb not null default '{}'::jsonb,
  state varchar(16) not null default 'pending',
  attempts integer not null default 0,
  available_at timestamptz not null default now(),
  published_at timestamptz,
  created_at timestamptz not null default now(),
  last_error text,
  constraint chk_video_task_outbox_aggregate check (
    aggregate_type in ('analysis','plan','render')
  ),
  constraint chk_video_task_outbox_state check (
    state in ('pending','published','failed')
  )
);

create unique index if not exists uq_video_task_outbox_event
  on video_task_outbox(aggregate_type, aggregate_id, event_type);
create index if not exists idx_video_task_outbox_dispatch
  on video_task_outbox(state, available_at)
  where state = 'pending';

-- ============================================================
-- video_render_tasks: new stages, points renaming, export fields
-- ============================================================
alter table video_render_tasks drop constraint if exists chk_video_render_tasks_status;

alter table video_render_tasks
  add column if not exists stage varchar(32),
  add column if not exists voice_file_id text,
  add column if not exists caption_file_id text,
  add column if not exists work_id text,
  add column if not exists billing_transaction_id text,
  add column if not exists cancel_requested_at timestamptz,
  add column if not exists retry_of_task_id text,
  add column if not exists attempt integer not null default 1,
  add column if not exists manifest_hash char(64);

alter table video_render_tasks
  add column if not exists quoted_points bigint not null default 0,
  add column if not exists reserved_points bigint not null default 0,
  add column if not exists captured_points bigint not null default 0,
  add column if not exists released_points bigint not null default 0;

update video_render_tasks set
  quoted_points = quoted_tokens,
  reserved_points = reserved_tokens,
  captured_points = captured_tokens,
  released_points = released_tokens;

alter table video_render_tasks add constraint chk_video_render_tasks_status check (
  status in ('CREATED','QUEUED','PROCESSING','SYNTHESIZING','RENDERING','UPLOADING','PUBLISHING','SUCCEEDED','FAILED','CANCELLED')
);

drop index if exists idx_video_render_tasks_recovery;
create index if not exists idx_video_render_tasks_recovery
  on video_render_tasks(status, run_after, lease_expires_at)
  where status in ('CREATED','QUEUED','PROCESSING','SYNTHESIZING','RENDERING','UPLOADING');

commit;
