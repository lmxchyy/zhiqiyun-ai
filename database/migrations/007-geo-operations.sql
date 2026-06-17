alter table geo_monitor_tasks
  add column if not exists schedule_id uuid,
  add column if not exists updated_at timestamptz not null default now();

create table if not exists geo_schedules (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  brand_id uuid not null references geo_brands(id),
  question text not null,
  platform varchar(100) not null,
  frequency varchar(30) not null,
  status varchar(30) not null default 'ACTIVE',
  next_run_at timestamptz not null,
  last_run_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists geo_reports (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  brand_id uuid not null references geo_brands(id),
  period varchar(30) not null,
  task_count integer not null,
  metrics jsonb not null default '{}',
  recommendations jsonb not null default '[]',
  created_at timestamptz not null default now()
);

create table if not exists geo_contents (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  brand_id uuid not null references geo_brands(id),
  type varchar(30) not null,
  title text not null,
  content text not null,
  status varchar(30) not null default 'DRAFT',
  created_at timestamptz not null default now()
);
