create table if not exists agent_shares (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  owner_id uuid not null references users(id),
  token varchar(100) not null unique,
  status varchar(30) not null default 'ACTIVE',
  calls bigint not null default 0,
  last_called_at timestamptz,
  created_at timestamptz not null default now()
);

create table if not exists agent_feedback (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  call_id uuid not null references agent_call_logs(id),
  user_id uuid not null references users(id),
  rating integer not null check (rating between 1 and 5),
  comment text not null default '',
  created_at timestamptz not null default now(),
  unique(call_id, user_id)
);

alter table agent_call_logs
  add column if not exists channel varchar(30) not null default 'AUTHENTICATED';
