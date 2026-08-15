-- PPT Generation V2 Phase 2: durable jobs, checkpoints, attempts and fencing.

begin;

alter table xz_ppt_tasks
  add column if not exists tenant_id text not null default 'tenant_default',
  add column if not exists organization_id text not null default '';

create index if not exists idx_xz_ppt_tasks_tenant_user
  on xz_ppt_tasks(tenant_id,user_id,created_at desc);

create table if not exists xz_ppt_v2_generation_jobs (
  id text primary key,
  tenant_id text not null,
  user_id text not null,
  organization_id text not null default '',
  existing_task_id text not null references xz_ppt_tasks(task_id),
  client_request_id text not null default '',
  idempotency_key text not null,
  status text not null default 'QUEUED',
  stage text not null default 'CREATED',
  attempt_count integer not null default 0,
  max_attempts integer not null default 3,
  run_after timestamptz not null default now(),
  lease_owner text,
  lease_expires_at timestamptz,
  fencing_token bigint not null default 0,
  completed_work_units integer not null default 0,
  total_work_units integer not null default 5,
  deck_job_id text not null,
  input_snapshot jsonb,
  deck_id text,
  revision integer,
  slide_count integer not null,
  render_sha256 char(64),
  render_bytes bytea,
  file_id text,
  asset_id text,
  error jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  cancel_requested_at timestamptz,
  constraint chk_ppt_v2_generation_job_status check (
    status in ('QUEUED','RUNNING','RETRY_WAIT','SUCCEEDED','FAILED','CANCELLED')
  ),
  constraint chk_ppt_v2_generation_job_stage check (
    stage in ('CREATED','TASK_LOADED','RENDERED','FILE_STORED','ASSET_CREATED','TASK_RELATED','COMPLETED')
  ),
  constraint chk_ppt_v2_generation_job_attempts check (
    attempt_count >= 0 and max_attempts between 1 and 20 and attempt_count <= max_attempts
  ),
  constraint chk_ppt_v2_generation_job_progress check (
    total_work_units = 5 and completed_work_units between 0 and total_work_units
  )
);

create unique index if not exists uq_ppt_v2_generation_job_idempotency
  on xz_ppt_v2_generation_jobs(tenant_id,user_id,idempotency_key);
create unique index if not exists uq_ppt_v2_generation_job_existing_task
  on xz_ppt_v2_generation_jobs(existing_task_id);
create index if not exists idx_ppt_v2_generation_job_recovery
  on xz_ppt_v2_generation_jobs(status,run_after,lease_expires_at,created_at)
  where status in ('QUEUED','RUNNING','RETRY_WAIT');
create index if not exists idx_ppt_v2_generation_job_owner
  on xz_ppt_v2_generation_jobs(tenant_id,user_id,created_at desc);

create table if not exists xz_ppt_v2_deck_jobs (
  id text primary key,
  generation_job_id text not null unique references xz_ppt_v2_generation_jobs(id) on delete cascade,
  deck_id text,
  revision integer,
  status text not null default 'PENDING',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint chk_ppt_v2_deck_job_status check (
    status in ('PENDING','RUNNING','SUCCEEDED','FAILED','CANCELLED')
  )
);

create table if not exists xz_ppt_v2_slide_jobs (
  id text primary key,
  generation_job_id text not null references xz_ppt_v2_generation_jobs(id) on delete cascade,
  deck_job_id text not null references xz_ppt_v2_deck_jobs(id) on delete cascade,
  slide_index integer not null,
  source_slide_id text,
  status text not null default 'PENDING',
  completed_work_units integer not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(generation_job_id,slide_index),
  constraint chk_ppt_v2_slide_job_index check (slide_index > 0),
  constraint chk_ppt_v2_slide_job_work check (completed_work_units between 0 and 1),
  constraint chk_ppt_v2_slide_job_status check (
    status in ('PENDING','RUNNING','SUCCEEDED','FAILED','CANCELLED')
  )
);

create table if not exists xz_ppt_v2_generation_attempts (
  id text primary key,
  generation_job_id text not null references xz_ppt_v2_generation_jobs(id) on delete cascade,
  attempt_number integer not null,
  worker_id text not null,
  fencing_token bigint not null,
  status text not null,
  usage_identity text,
  error jsonb,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  unique(generation_job_id,attempt_number),
  constraint chk_ppt_v2_generation_attempt_number check (attempt_number > 0),
  constraint chk_ppt_v2_generation_attempt_status check (
    status in ('RUNNING','RETRY_WAIT','SUCCEEDED','FAILED','CANCELLED')
  )
);

create table if not exists xz_ppt_v2_generation_transitions (
  id bigserial primary key,
  generation_job_id text not null references xz_ppt_v2_generation_jobs(id) on delete cascade,
  attempt_id text references xz_ppt_v2_generation_attempts(id),
  from_stage text,
  to_stage text not null,
  fencing_token bigint not null default 0,
  checkpoint jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique(generation_job_id,to_stage)
);

create unique index if not exists uq_ppt_v2_work_center_asset_per_job
  on xz_assets((metadata->>'pptV2GenerationJobId'))
  where metadata->>'source'='ppt-v2' and deleted_at is null;

create unique index if not exists uq_ppt_v2_file_per_job
  on xz_file_objects(tenant_id,user_id,business_id)
  where business_type='ppt_v2_generation' and status='ACTIVE';

commit;
