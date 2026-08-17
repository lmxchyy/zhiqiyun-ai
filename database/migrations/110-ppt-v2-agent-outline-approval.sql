-- PPT Generation V2 Phase 3 Slice A: durable agent planning and outline approval.

begin;

alter table xz_ppt_v2_generation_jobs
  add column if not exists workflow_type text not null default 'RENDER';

alter table xz_ppt_v2_generation_jobs
  alter column existing_task_id drop not null,
  alter column deck_job_id drop not null;

alter table xz_ppt_v2_generation_jobs
  drop constraint if exists chk_ppt_v2_generation_job_status,
  drop constraint if exists chk_ppt_v2_generation_job_stage,
  drop constraint if exists chk_ppt_v2_generation_job_progress,
  drop constraint if exists chk_ppt_v2_generation_job_workflow;

alter table xz_ppt_v2_generation_jobs
  add constraint chk_ppt_v2_generation_job_status check (
    status in ('QUEUED','RUNNING','RETRY_WAIT','WAITING_FOR_OUTLINE_APPROVAL','SUCCEEDED','FAILED','CANCELLED')
  ),
  add constraint chk_ppt_v2_generation_job_stage check (
    stage in (
      'CREATED','TASK_LOADED','RENDERED','FILE_STORED','ASSET_CREATED','TASK_RELATED','COMPLETED',
      'INTENT_RESOLVED','RESEARCHED','STORYLINE_PLANNED','OUTLINE_PLANNED','OUTLINE_APPROVED'
    )
  ),
  add constraint chk_ppt_v2_generation_job_progress check (
    completed_work_units between 0 and total_work_units and
    ((workflow_type = 'RENDER' and total_work_units = 5) or
     (workflow_type = 'AGENT_OUTLINE' and total_work_units = 3))
  ),
  add constraint chk_ppt_v2_generation_job_workflow check (
    (workflow_type = 'RENDER' and existing_task_id is not null and deck_job_id is not null and
      stage in ('CREATED','TASK_LOADED','RENDERED','FILE_STORED','ASSET_CREATED','TASK_RELATED','COMPLETED') and
      status <> 'WAITING_FOR_OUTLINE_APPROVAL')
    or
    (workflow_type = 'AGENT_OUTLINE' and existing_task_id is null and deck_job_id is null and
      slide_count between 6 and 12 and
      stage in ('CREATED','INTENT_RESOLVED','RESEARCHED','STORYLINE_PLANNED','OUTLINE_PLANNED','OUTLINE_APPROVED') and
      (status <> 'WAITING_FOR_OUTLINE_APPROVAL' or stage = 'OUTLINE_PLANNED'))
  );

create table if not exists xz_ppt_v2_agent_plans (
  generation_job_id text primary key references xz_ppt_v2_generation_jobs(id) on delete cascade,
  intent jsonb,
  research jsonb,
  storyline jsonb,
  current_outline_revision integer,
  approved_outline_revision integer,
  research_execution_count integer not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint chk_ppt_v2_agent_plan_revisions check (
    (current_outline_revision is null or current_outline_revision > 0) and
    (approved_outline_revision is null or approved_outline_revision > 0) and
    (approved_outline_revision is null or approved_outline_revision = current_outline_revision)
  ),
  constraint chk_ppt_v2_agent_plan_research_count check (research_execution_count >= 0)
);

create table if not exists xz_ppt_v2_outline_revisions (
  id bigserial primary key,
  generation_job_id text not null references xz_ppt_v2_generation_jobs(id) on delete cascade,
  revision integer not null,
  parent_revision integer,
  outline jsonb not null,
  approved_at timestamptz,
  created_at timestamptz not null default now(),
  unique(generation_job_id,revision),
  constraint chk_ppt_v2_outline_revision check (
    revision > 0 and (parent_revision is null or parent_revision > 0)
  )
);

create index if not exists idx_ppt_v2_outline_revisions_job_created
  on xz_ppt_v2_outline_revisions(generation_job_id,created_at desc);

commit;
