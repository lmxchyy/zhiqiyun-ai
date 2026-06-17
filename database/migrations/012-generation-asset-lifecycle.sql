alter table generation_tasks
  add column if not exists retry_of_task_id uuid references generation_tasks(id);

alter table assets
  add column if not exists deleted_at timestamptz;

create index if not exists assets_user_active_idx
  on assets(user_id, created_at desc)
  where deleted_at is null;
