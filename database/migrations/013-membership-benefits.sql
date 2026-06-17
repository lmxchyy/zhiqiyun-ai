create table if not exists coupons (
  id uuid primary key default gen_random_uuid(),
  code varchar(100) not null unique,
  name varchar(150) not null,
  type varchar(30) not null,
  value bigint not null,
  min_amount bigint not null default 0,
  max_uses integer not null default 1,
  uses_count integer not null default 0,
  status varchar(30) not null default 'ACTIVE',
  expires_at timestamptz not null,
  created_by uuid references users(id),
  created_at timestamptz not null default now()
);

create table if not exists user_coupons (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  coupon_id uuid not null references coupons(id),
  status varchar(30) not null default 'AVAILABLE',
  order_id uuid references orders(id),
  created_at timestamptz not null default now(),
  unique(user_id, coupon_id)
);

alter table orders
  add column if not exists original_amount_cents bigint,
  add column if not exists discount_amount_cents bigint not null default 0,
  add column if not exists coupon_id uuid references coupons(id);

create table if not exists redemption_codes (
  id uuid primary key default gen_random_uuid(),
  code varchar(100) not null unique,
  type varchar(30) not null,
  points bigint not null default 0,
  plan_id uuid references membership_plans(id),
  max_uses integer not null default 1,
  uses_count integer not null default 0,
  status varchar(30) not null default 'ACTIVE',
  expires_at timestamptz not null,
  created_by uuid references users(id),
  channel_agent_id uuid references channel_agents(id),
  created_at timestamptz not null default now()
);

create table if not exists redemption_uses (
  id uuid primary key default gen_random_uuid(),
  code_id uuid not null references redemption_codes(id),
  user_id uuid not null references users(id),
  benefit_snapshot jsonb not null default '{}',
  created_at timestamptz not null default now(),
  unique(code_id, user_id)
);
