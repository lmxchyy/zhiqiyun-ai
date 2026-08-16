-- PPT Generation V2 Phase 3 Slice B: durable multi-page deck generation.

begin;

alter table xz_ppt_v2_agent_plans
  add column if not exists deck_state jsonb;

alter table xz_ppt_v2_generation_jobs
  drop constraint if exists chk_ppt_v2_generation_job_stage,
  drop constraint if exists chk_ppt_v2_generation_job_progress,
  drop constraint if exists chk_ppt_v2_generation_job_workflow;

alter table xz_ppt_v2_generation_jobs
  add constraint chk_ppt_v2_generation_job_stage check (
    stage in (
      'CREATED','TASK_LOADED','RENDERED','FILE_STORED','ASSET_CREATED','TASK_RELATED','COMPLETED',
      'INTENT_RESOLVED','RESEARCHED','STORYLINE_PLANNED','OUTLINE_PLANNED','OUTLINE_APPROVED',
      'CONTENT_READY','ASSETS_READY','LAYOUT_COMPILED','QUALITY_CHECKED'
    )
  ),
  add constraint chk_ppt_v2_generation_job_progress check (
    completed_work_units between 0 and total_work_units and
    ((workflow_type = 'RENDER' and total_work_units = 5) or
     (workflow_type = 'AGENT_OUTLINE' and total_work_units >= 3))
  ),
  add constraint chk_ppt_v2_generation_job_workflow check (
    (workflow_type = 'RENDER' and existing_task_id is not null and deck_job_id is not null and
      stage in ('CREATED','TASK_LOADED','RENDERED','FILE_STORED','ASSET_CREATED','TASK_RELATED','COMPLETED') and
      status <> 'WAITING_FOR_OUTLINE_APPROVAL')
    or
    (workflow_type = 'AGENT_OUTLINE' and slide_count between 6 and 12 and
      stage in (
        'CREATED','INTENT_RESOLVED','RESEARCHED','STORYLINE_PLANNED','OUTLINE_PLANNED','OUTLINE_APPROVED',
        'CONTENT_READY','ASSETS_READY','LAYOUT_COMPILED','QUALITY_CHECKED','RENDERED','FILE_STORED',
        'ASSET_CREATED','TASK_RELATED','COMPLETED'
      ) and
      (status <> 'WAITING_FOR_OUTLINE_APPROVAL' or stage = 'OUTLINE_PLANNED') and
      (stage in ('CREATED','INTENT_RESOLVED','RESEARCHED','STORYLINE_PLANNED','OUTLINE_PLANNED') or deck_job_id is not null))
  );

create unique index if not exists uq_ppt_v2_image_asset_per_intent
  on xz_file_objects(tenant_id,user_id,business_id)
  where business_type='ppt_v2_image_asset' and status='ACTIVE';

commit;
