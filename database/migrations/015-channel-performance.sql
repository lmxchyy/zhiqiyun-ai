create table if not exists channel_performance_snapshots (
  id uuid primary key default gen_random_uuid(),
  period varchar(30) not null,
  totals jsonb not null default '{}',
  rankings jsonb not null default '[]',
  created_by uuid references users(id),
  created_at timestamptz not null default now()
);

create index if not exists channel_performance_period_idx
  on channel_performance_snapshots(period, created_at desc);
