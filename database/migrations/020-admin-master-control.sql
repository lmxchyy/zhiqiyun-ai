-- 主控 SaaS 首批表结构。
-- 金额使用分；配额、点数和用量使用整数；业务快照使用 jsonb 保留历史口径。

create table if not exists customers (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references users(id),
  enterprise_id uuid references enterprises(id),
  channel_agent_id uuid references channel_agents(id),
  name varchar(200) not null,
  contact_name varchar(100),
  contact_email varchar(255),
  contact_phone varchar(80),
  source varchar(80) not null default 'DIRECT',
  status varchar(30) not null default 'ACTIVE',
  owner_id uuid references users(id),
  tags jsonb not null default '[]',
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists customers_status_idx on customers(status);
create index if not exists customers_channel_agent_idx on customers(channel_agent_id);

create table if not exists products (
  id uuid primary key default gen_random_uuid(),
  code varchar(80) not null unique,
  name varchar(150) not null,
  type varchar(40) not null,
  status varchar(30) not null default 'ACTIVE',
  description text not null default '',
  config jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists product_entitlements (
  id uuid primary key default gen_random_uuid(),
  product_id uuid not null references products(id),
  code varchar(100) not null,
  name varchar(150) not null,
  unit varchar(40) not null,
  config jsonb not null default '{}',
  created_at timestamptz not null default now(),
  unique(product_id, code)
);

create table if not exists plan_products (
  id uuid primary key default gen_random_uuid(),
  plan_id uuid not null references membership_plans(id),
  product_id uuid not null references products(id),
  entitlement_snapshot jsonb not null default '{}',
  quota bigint not null default 0,
  status varchar(30) not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  unique(plan_id, product_id)
);

create table if not exists customer_groups (
  id uuid primary key default gen_random_uuid(),
  code varchar(80) not null unique,
  name varchar(120) not null,
  model_ratio numeric(10, 4) not null default 1,
  config jsonb not null default '{}',
  status varchar(30) not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists api_provider_channels (
  id uuid primary key default gen_random_uuid(),
  code varchar(80) not null unique,
  name varchar(150) not null,
  provider varchar(80) not null,
  base_url text not null,
  encrypted_api_key text,
  model_mapping jsonb not null default '{}',
  model_list jsonb not null default '[]',
  priority integer not null default 100,
  weight integer not null default 100,
  status varchar(30) not null default 'ACTIVE',
  last_checked_at timestamptz,
  health_snapshot jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists api_model_catalog (
  id uuid primary key default gen_random_uuid(),
  model_code varchar(120) not null unique,
  display_name varchar(150) not null,
  capability varchar(50) not null,
  billing_mode varchar(30) not null,
  model_ratio numeric(10, 4) not null default 1,
  completion_ratio numeric(10, 4) not null default 1,
  cache_ratio numeric(10, 4) not null default 1,
  fixed_quota bigint not null default 0,
  config jsonb not null default '{}',
  status varchar(30) not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists api_keys (
  id uuid primary key default gen_random_uuid(),
  customer_id uuid references customers(id),
  user_id uuid references users(id),
  name varchar(150) not null,
  key_hash varchar(128) not null unique,
  key_prefix varchar(40) not null,
  model_limits jsonb not null default '[]',
  ip_whitelist jsonb not null default '[]',
  quota_limit bigint not null default 0,
  quota_used bigint not null default 0,
  rate_limit_per_minute integer not null default 60,
  status varchar(30) not null default 'ACTIVE',
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists api_keys_user_idx on api_keys(user_id);
create index if not exists api_keys_customer_idx on api_keys(customer_id);

create table if not exists quota_accounts (
  id uuid primary key default gen_random_uuid(),
  customer_id uuid references customers(id),
  user_id uuid references users(id),
  available bigint not null default 0,
  frozen bigint not null default 0,
  version bigint not null default 0,
  updated_at timestamptz not null default now(),
  unique(customer_id, user_id)
);

create table if not exists quota_transactions (
  id uuid primary key default gen_random_uuid(),
  quota_account_id uuid references quota_accounts(id),
  user_id uuid references users(id),
  type varchar(40) not null,
  available_delta bigint not null default 0,
  frozen_delta bigint not null default 0,
  available_after bigint not null,
  frozen_after bigint not null,
  reference_type varchar(80) not null,
  reference_id varchar(120) not null,
  snapshot jsonb not null default '{}',
  created_at timestamptz not null default now()
);
create index if not exists quota_transactions_account_idx on quota_transactions(quota_account_id, created_at desc);

create table if not exists usage_daily_summaries (
  id uuid primary key default gen_random_uuid(),
  day date not null,
  customer_id uuid references customers(id),
  user_id uuid references users(id),
  product_code varchar(80) not null,
  metric varchar(80) not null,
  usage_count bigint not null default 0,
  quota_cost bigint not null default 0,
  cost_cents bigint not null default 0,
  revenue_cents bigint not null default 0,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now(),
  unique(day, customer_id, user_id, product_code, metric)
);

create table if not exists delivery_projects (
  id uuid primary key default gen_random_uuid(),
  customer_id uuid references customers(id),
  order_id uuid references orders(id),
  product_id uuid references products(id),
  name varchar(200) not null,
  type varchar(50) not null,
  status varchar(30) not null default 'PENDING',
  progress integer not null default 0 check (progress between 0 and 100),
  owner_id uuid references users(id),
  start_at timestamptz,
  due_at timestamptz,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists delivery_projects_status_idx on delivery_projects(status);

create table if not exists delivery_milestones (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references delivery_projects(id),
  name varchar(150) not null,
  status varchar(30) not null default 'PENDING',
  progress integer not null default 0 check (progress between 0 and 100),
  due_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists commission_rules (
  id uuid primary key default gen_random_uuid(),
  name varchar(150) not null,
  product_id uuid references products(id),
  plan_id uuid references membership_plans(id),
  channel_level integer,
  order_type varchar(50) not null default 'NEW',
  rate numeric(8, 5) not null default 0,
  fixed_amount_cents bigint not null default 0,
  status varchar(30) not null default 'ACTIVE',
  config jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists roles (
  id uuid primary key default gen_random_uuid(),
  code varchar(80) not null unique,
  name varchar(120) not null,
  description text not null default '',
  status varchar(30) not null default 'ACTIVE',
  created_at timestamptz not null default now()
);

create table if not exists permissions (
  id uuid primary key default gen_random_uuid(),
  code varchar(120) not null unique,
  name varchar(150) not null,
  module varchar(80) not null,
  action varchar(80) not null,
  created_at timestamptz not null default now()
);

create table if not exists role_permissions (
  role_id uuid not null references roles(id),
  permission_id uuid not null references permissions(id),
  created_at timestamptz not null default now(),
  primary key(role_id, permission_id)
);

create table if not exists user_roles (
  user_id uuid not null references users(id),
  role_id uuid not null references roles(id),
  created_at timestamptz not null default now(),
  primary key(user_id, role_id)
);

create table if not exists system_settings (
  key varchar(120) primary key,
  value jsonb not null default '{}',
  updated_by uuid references users(id),
  updated_at timestamptz not null default now()
);

create table if not exists brand_settings (
  id uuid primary key default gen_random_uuid(),
  code varchar(80) not null unique,
  name varchar(150) not null,
  logo_url text,
  primary_color varchar(40),
  domain text,
  config jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
