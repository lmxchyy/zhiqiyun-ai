-- AI smart video foundation, phase 1.
-- Additive only: existing generation_tasks, assets and file center remain authoritative.

begin;

create table if not exists video_projects (
  id text primary key,
  tenant_id text not null,
  user_id text not null,
  title text not null,
  requirement text not null default '',
  status text not null default 'DRAFT',
  current_version integer not null default 0,
  output_asset_id text,
  active_render_task_id text,
  error_code text,
  error_message text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  constraint chk_video_projects_status check (
    status in ('DRAFT','ANALYZING','STORYBOARD_READY','CONFIRMED','RENDERING','COMPLETED','FAILED')
  ),
  constraint chk_video_projects_version check (current_version >= 0)
);

create index if not exists idx_video_projects_owner
  on video_projects(tenant_id,user_id,updated_at desc)
  where deleted_at is null;

create table if not exists video_project_assets (
  id text primary key,
  project_id text not null references video_projects(id),
  tenant_id text not null,
  user_id text not null,
  file_id text not null references xz_file_objects(file_id),
  storage_key text not null,
  asset_type text not null,
  sort_order integer not null default 0,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(project_id,file_id),
  constraint chk_video_project_assets_type check (asset_type in ('VIDEO','IMAGE')),
  constraint chk_video_project_assets_metadata check (jsonb_typeof(metadata) = 'object')
);

create index if not exists idx_video_project_assets_order
  on video_project_assets(tenant_id,user_id,project_id,sort_order,created_at);

create table if not exists video_project_versions (
  id text primary key,
  project_id text not null references video_projects(id),
  tenant_id text not null,
  version_number integer not null,
  status text not null default 'DRAFT',
  requirement text not null default '',
  script jsonb not null default '{"title":"","summary":"","language":"","estimatedLengthMs":0,"sections":[]}'::jsonb,
  storyboard_snapshot jsonb not null default '[]'::jsonb,
  created_by text not null,
  created_at timestamptz not null default now(),
  unique(project_id,version_number),
  constraint chk_video_project_versions_status check (
    status in ('DRAFT','GENERATED','CONFIRMED','SUPERSEDED')
  ),
  constraint chk_video_project_versions_script check (jsonb_typeof(script) = 'object'),
  constraint chk_video_project_versions_storyboard check (jsonb_typeof(storyboard_snapshot) = 'array')
);

create table if not exists video_storyboard_scenes (
  id text primary key,
  project_id text not null references video_projects(id),
  version_id text not null references video_project_versions(id),
  scene_index integer not null,
  title text not null default '',
  narration text not null default '',
  visual_prompt text not null default '',
  duration_ms bigint not null default 0,
  source_asset_ids jsonb not null default '[]'::jsonb,
  transition jsonb not null default '{"type":"cut","durationMs":0}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(version_id,scene_index),
  constraint chk_video_storyboard_scene_index check (scene_index >= 0),
  constraint chk_video_storyboard_duration check (duration_ms >= 0),
  constraint chk_video_storyboard_sources check (jsonb_typeof(source_asset_ids) = 'array'),
  constraint chk_video_storyboard_transition check (jsonb_typeof(transition) = 'object')
);

create table if not exists video_render_tasks (
  id text primary key,
  project_id text not null references video_projects(id),
  version_id text references video_project_versions(id),
  tenant_id text not null,
  user_id text not null,
  client_request_id text not null,
  status text not null default 'CREATED',
  progress integer not null default 0,
  specification jsonb not null,
  quoted_tokens bigint not null default 0,
  reserved_tokens bigint not null default 0,
  captured_tokens bigint not null default 0,
  released_tokens bigint not null default 0,
  output_file_id text references xz_file_objects(file_id),
  output_asset_id text,
  error_code text,
  error_message text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  unique(tenant_id,user_id,client_request_id),
  constraint chk_video_render_tasks_status check (
    status in ('CREATED','QUEUED','RUNNING','UPLOADING','SUCCEEDED','FAILED','CANCELLED')
  ),
  constraint chk_video_render_tasks_progress check (progress between 0 and 100),
  constraint chk_video_render_tasks_specification check (jsonb_typeof(specification) = 'object'),
  constraint chk_video_render_tasks_tokens check (
    quoted_tokens >= 0 and reserved_tokens >= 0 and captured_tokens >= 0 and released_tokens >= 0
  )
);

create index if not exists idx_video_render_tasks_owner
  on video_render_tasks(tenant_id,user_id,project_id,created_at desc);
create index if not exists idx_video_render_tasks_dispatch
  on video_render_tasks(status,created_at)
  where status in ('CREATED','QUEUED','RUNNING','UPLOADING');

commit;
