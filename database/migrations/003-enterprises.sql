create table if not exists enterprises (
  id uuid primary key default gen_random_uuid(),
  name varchar(200) not null,
  owner_id uuid not null references users(id),
  status varchar(30) not null default 'ACTIVE',
  total_quota bigint not null default 0,
  available_quota bigint not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists enterprise_members (
  id uuid primary key default gen_random_uuid(),
  enterprise_id uuid not null references enterprises(id),
  user_id uuid not null references users(id),
  role varchar(40) not null,
  status varchar(30) not null default 'ACTIVE',
  quota_limit bigint not null default 0,
  quota_used bigint not null default 0,
  created_at timestamptz not null default now(),
  unique (enterprise_id, user_id)
);

create table if not exists enterprise_quota_transactions (
  id uuid primary key default gen_random_uuid(),
  enterprise_id uuid not null references enterprises(id),
  member_id uuid not null references enterprise_members(id),
  type varchar(30) not null,
  amount bigint not null,
  available_after bigint not null,
  actor_id uuid references users(id),
  created_at timestamptz not null default now()
);
