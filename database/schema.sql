-- 先知 AI 平台 PostgreSQL 核心表结构
-- 金额使用分，积分使用整数；业务流水只新增，不覆盖历史记录。

create extension if not exists pgcrypto;

-- 迁移过渡状态表：用于在领域表逐步规范化期间，让 API 与 Worker 通过
-- PostgreSQL 共享完整业务状态，替代跨容器 JSON 文件共享。
create table if not exists platform_state (
  id varchar(50) primary key,
  state jsonb not null,
  version bigint not null default 0,
  updated_at timestamptz not null default now()
);

create table users (
  id uuid primary key default gen_random_uuid(),
  email varchar(255) not null unique,
  password_hash text not null,
  name varchar(100) not null,
  role varchar(40) not null default 'MEMBER',
  status varchar(20) not null default 'ACTIVE',
  plan_id uuid,
  referred_by uuid references users(id),
  subscription_expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table enterprises (
  id uuid primary key default gen_random_uuid(),
  name varchar(200) not null,
  owner_id uuid not null references users(id),
  status varchar(30) not null default 'ACTIVE',
  total_quota bigint not null default 0,
  available_quota bigint not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table enterprise_members (
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

create table enterprise_quota_transactions (
  id uuid primary key default gen_random_uuid(),
  enterprise_id uuid not null references enterprises(id),
  member_id uuid not null references enterprise_members(id),
  type varchar(30) not null,
    amount bigint not null,
    available_after bigint not null,
    actor_id uuid references users(id),
    reference_type varchar(50),
    reference_id varchar(100),
    created_at timestamptz not null default now()
);

create table membership_plans (
  id uuid primary key default gen_random_uuid(),
  code varchar(50) not null unique,
  name varchar(100) not null,
  price_cents bigint not null check (price_cents >= 0),
  grant_points bigint not null check (grant_points >= 0),
  duration_days integer not null,
  concurrency integer not null default 1,
  entitlements jsonb not null default '{}',
  active boolean not null default true,
  created_at timestamptz not null default now()
);

alter table users add constraint users_plan_fk foreign key (plan_id) references membership_plans(id);

create table auth_sessions (
  access_token_hash varchar(64) primary key,
  user_id uuid not null references users(id),
  refresh_token_hash varchar(64) not null unique,
  access_expires_at timestamptz not null,
  refresh_expires_at timestamptz not null,
  revoked_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index auth_sessions_user_active_idx on auth_sessions(user_id, access_expires_at desc) where revoked_at is null;

create table point_accounts (
  user_id uuid primary key references users(id),
  available bigint not null default 0 check (available >= 0),
  frozen bigint not null default 0 check (frozen >= 0),
  version bigint not null default 0,
  updated_at timestamptz not null default now()
);

create table point_transactions (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  type varchar(40) not null,
  available_delta bigint not null default 0,
  frozen_delta bigint not null default 0,
  available_after bigint not null,
  frozen_after bigint not null,
  reference_type varchar(50) not null,
  reference_id varchar(100) not null,
  created_at timestamptz not null default now()
);
create index point_transactions_user_idx on point_transactions(user_id, created_at desc);

create table model_providers (
  id uuid primary key default gen_random_uuid(),
  code varchar(50) not null unique,
  name varchar(100) not null,
  encrypted_api_key text,
  base_url text,
  status varchar(20) not null default 'ACTIVE',
  config jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create table model_definitions (
  id uuid primary key default gen_random_uuid(),
  provider_id uuid not null references model_providers(id),
  code varchar(100) not null unique,
  name varchar(100) not null,
  capability varchar(40) not null,
    point_cost bigint not null,
    billing_source varchar(30),
    enterprise_member_id uuid references enterprise_members(id),
    billing_transaction_id uuid references enterprise_quota_transactions(id),
  config jsonb not null default '{}',
  active boolean not null default true,
  provider_code varchar(100),
  capabilities jsonb not null default '[]',
  tier varchar(30) not null default 'PAID',
  point_costs jsonb not null default '{}',
  status varchar(30) not null default 'ACTIVE'
);

create table model_pricing_rules (
  id uuid primary key default gen_random_uuid(),
  model_code varchar(100) not null,
  capability varchar(40) not null,
  point_cost bigint not null check (point_cost > 0),
  status varchar(30) not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  unique(model_code, capability)
);

create table if not exists ai_modules (
  id text primary key,
  module_code text not null unique,
  name text not null,
  description text not null default '',
  status text not null default 'ACTIVE',
  open_tenant_ids jsonb not null default '[]'::jsonb,
  open_package_ids jsonb not null default '[]'::jsonb,
  bound_models jsonb not null default '[]'::jsonb,
  default_schema_id text,
  allow_agents boolean not null default true,
  allow_end_users boolean not null default true,
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists ai_models (
  id text primary key,
  model_name text not null,
  model_type text not null,
  provider text not null,
  capability_code jsonb not null default '[]'::jsonb,
  module_code text not null references ai_modules(module_code),
  status text not null default 'ACTIVE',
  fallback_model text,
  sort_weight integer not null default 0,
  allow_fallback_switch boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (module_code, model_name)
);

create table if not exists ai_parameter_schemas (
  id text primary key,
  module_code text not null references ai_modules(module_code),
  model_name text,
  schema_json jsonb not null default '{"fields":[]}'::jsonb,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists tenant_module_limits (
  id text primary key,
  tenant_id text not null default 'default',
  agent_id text,
  package_id text,
  module_code text not null references ai_modules(module_code),
  model_name text,
  limit_json jsonb not null default '{}'::jsonb,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists billing_rules (
  id text primary key,
  module_code text not null references ai_modules(module_code),
  model_name text,
  billing_type text not null default 'per_request',
  base_price numeric(12,4) not null default 1,
  cost_price numeric(12,4) not null default 0,
  currency_type text not null default 'credit',
  parameter_multiplier jsonb not null default '{}'::jsonb,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table generation_tasks (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  tenant_id text,
  agent_id text,
  operation_center_id text,
  module_code text,
  type varchar(40) not null,
  model_code varchar(100) not null,
  prompt text not null,
  params jsonb not null default '{}',
  billing_type text,
  final_schema_snapshot jsonb not null default '{}'::jsonb,
  limit_snapshot jsonb not null default '{}'::jsonb,
  upstream_provider text,
  upstream_request_id text,
  user_charge_amount bigint not null default 0,
  upstream_cost bigint not null default 0,
  platform_profit bigint not null default 0,
  agent_commission bigint not null default 0,
  operation_center_commission bigint not null default 0,
  status varchar(30) not null,
  progress integer not null default 0 check (progress between 0 and 100),
  point_cost bigint not null,
  idempotency_key varchar(100) unique,
  retry_of_task_id uuid references generation_tasks(id),
  error text,
  failure_reason text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index generation_tasks_user_idx on generation_tasks(user_id, created_at desc);
create index generation_tasks_module_code_idx on generation_tasks(module_code);

create table generation_task_attempts (
  id uuid primary key default gen_random_uuid(),
  task_id uuid not null references generation_tasks(id),
  attempt integer not null,
  provider_request_id varchar(200),
  status varchar(30) not null,
  request_snapshot jsonb not null default '{}',
  response_snapshot jsonb not null default '{}',
  cost_cents bigint not null default 0,
  started_at timestamptz not null default now(),
  finished_at timestamptz
);

create table model_call_logs (
    id uuid primary key default gen_random_uuid(),
    task_id uuid references generation_tasks(id),
    provider_code varchar(100) not null,
    model_code varchar(100) not null,
    status varchar(30) not null,
    latency_ms bigint not null default 0,
    cost_cents bigint not null default 0,
    provider_request_id varchar(200),
    error text,
    created_at timestamptz not null default now()
);

create table assets (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  task_id uuid references generation_tasks(id),
  name varchar(255) not null,
  media_type varchar(30) not null,
  storage_key text not null,
  metadata jsonb not null default '{}',
  favorite boolean not null default false,
  deleted_at timestamptz,
  created_at timestamptz not null default now()
);

create table moderation_logs (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references users(id),
  content_type varchar(40) not null,
  status varchar(30) not null,
  matched_terms jsonb not null default '[]',
  created_at timestamptz not null default now()
);

create table orders (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  plan_id uuid references membership_plans(id),
  amount_cents bigint not null,
  status varchar(30) not null,
  price_snapshot jsonb not null,
  idempotency_key varchar(100) unique,
  paid_at timestamptz,
  created_at timestamptz not null default now()
);

create table payment_events (
  id uuid primary key default gen_random_uuid(),
  event_id varchar(200) not null unique,
  order_id uuid not null references orders(id),
  channel varchar(30) not null,
  payload jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create table payments (
    id uuid primary key default gen_random_uuid(),
    channel varchar(30) not null,
    event_id varchar(200) not null,
    order_id uuid not null references orders(id),
    amount bigint not null,
    status varchar(30) not null,
    provider_transaction_id varchar(200),
    created_at timestamptz not null default now(),
    unique (channel, event_id)
);

create table invoices (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id),
    order_id uuid not null references orders(id),
    amount bigint not null,
    title varchar(255) not null,
    tax_number varchar(100),
    email varchar(255),
    status varchar(30) not null default 'PENDING',
    invoice_number varchar(100),
    issued_at timestamptz,
    created_at timestamptz not null default now(),
    unique (order_id)
);

create table channel_agents (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null unique references users(id),
  parent_id uuid references channel_agents(id),
  level integer not null check (level in (1, 2)),
  status varchar(20) not null,
  invite_code varchar(50) not null unique,
  created_at timestamptz not null default now()
);

create table commissions (
  id uuid primary key default gen_random_uuid(),
  order_id uuid not null references orders(id),
  agent_id uuid not null references channel_agents(id),
  amount_cents bigint not null,
  rate numeric(6, 5) not null,
  status varchar(30) not null,
  rule_snapshot jsonb not null,
  created_at timestamptz not null default now()
);

create table withdrawal_requests (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references channel_agents(id),
  amount_cents bigint not null check (amount_cents > 0),
  status varchar(30) not null default 'PENDING',
  reviewed_by uuid references users(id),
  reviewed_at timestamptz,
  created_at timestamptz not null default now()
);

create table presentations (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  topic varchar(255) not null,
  theme varchar(100) not null,
  status varchar(30) not null,
  slides jsonb not null default '[]',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table agents (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  name varchar(150) not null,
  description text,
  status varchar(30) not null,
  version integer not null default 1,
  workflow jsonb not null default '[]',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table agent_versions (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  version integer not null,
  actor_id uuid references users(id),
  reason varchar(100) not null,
  snapshot jsonb not null,
  created_at timestamptz not null default now(),
  unique (agent_id, version)
);

create table knowledge_bases (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  name varchar(150) not null,
  description text,
  document_count integer not null default 0,
  created_at timestamptz not null default now()
);

create table knowledge_documents (
  id uuid primary key default gen_random_uuid(),
  knowledge_base_id uuid not null references knowledge_bases(id),
  owner_id uuid not null references users(id),
  name varchar(255) not null,
    content text not null,
    chunks jsonb not null default '[]',
    embeddings jsonb not null default '[]',
    created_at timestamptz not null default now()
);

create table agent_call_logs (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  user_id uuid references users(id),
  input jsonb not null,
  output jsonb not null,
  token_usage bigint not null default 0,
  cost_cents bigint not null default 0,
  latency_ms bigint not null default 0,
  channel varchar(30) not null default 'AUTHENTICATED',
  created_at timestamptz not null default now()
);

create table agent_shares (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  owner_id uuid not null references users(id),
  token varchar(100) not null unique,
  status varchar(30) not null default 'ACTIVE',
  calls bigint not null default 0,
  last_called_at timestamptz,
  created_at timestamptz not null default now()
);

create table agent_feedback (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  call_id uuid not null references agent_call_logs(id),
  user_id uuid not null references users(id),
  rating integer not null check (rating between 1 and 5),
  comment text not null default '',
  created_at timestamptz not null default now(),
  unique(call_id, user_id)
);

create table geo_brands (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  name varchar(150) not null,
  competitors jsonb not null default '[]',
  keywords jsonb not null default '[]',
  created_at timestamptz not null default now()
);

create table geo_monitor_tasks (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  brand_id uuid not null references geo_brands(id),
  question text not null,
  platform varchar(100) not null,
  status varchar(30) not null,
    result jsonb not null default '{}',
    created_at timestamptz not null default now()
);

create table geo_schedules (
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

create table geo_reports (
    id uuid primary key default gen_random_uuid(),
    owner_id uuid not null references users(id),
    brand_id uuid not null references geo_brands(id),
    period varchar(30) not null,
    task_count integer not null,
    metrics jsonb not null default '{}',
    recommendations jsonb not null default '[]',
    created_at timestamptz not null default now()
);

create table geo_contents (
    id uuid primary key default gen_random_uuid(),
    owner_id uuid not null references users(id),
    brand_id uuid not null references geo_brands(id),
    type varchar(30) not null,
    title text not null,
    content text not null,
    status varchar(30) not null default 'DRAFT',
    publications integer not null default 0,
    last_published_at timestamptz,
    created_at timestamptz not null default now()
);

create table geo_content_publications (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  brand_id uuid not null references geo_brands(id),
  content_id uuid not null references geo_contents(id),
  platform varchar(100) not null,
  url text not null,
  status varchar(30) not null default 'PUBLISHED',
  published_at timestamptz not null,
  metrics_history jsonb not null default '[]',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(content_id, url)
);

create table coupons (
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

create table user_coupons (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  coupon_id uuid not null references coupons(id),
  status varchar(30) not null default 'AVAILABLE',
  order_id uuid references orders(id),
  created_at timestamptz not null default now(),
  unique(user_id, coupon_id)
);

alter table orders
  add column original_amount_cents bigint,
  add column discount_amount_cents bigint not null default 0,
  add column coupon_id uuid references coupons(id);

create table redemption_codes (
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

create table redemption_uses (
  id uuid primary key default gen_random_uuid(),
  code_id uuid not null references redemption_codes(id),
  user_id uuid not null references users(id),
  benefit_snapshot jsonb not null default '{}',
  created_at timestamptz not null default now(),
  unique(code_id, user_id)
);

create table channel_performance_snapshots (
  id uuid primary key default gen_random_uuid(),
  period varchar(30) not null,
  totals jsonb not null default '{}',
  rankings jsonb not null default '[]',
  created_by uuid references users(id),
  created_at timestamptz not null default now()
);
create index channel_performance_period_idx on channel_performance_snapshots(period, created_at desc);

create table settlement_statements (
  id uuid primary key default gen_random_uuid(),
  statement_number varchar(100) not null unique,
  agent_id uuid not null references channel_agents(id),
  withdrawal_id uuid not null references withdrawal_requests(id),
  amount bigint not null,
  commission_ids jsonb not null default '[]',
  status varchar(30) not null,
  period_start timestamptz not null,
  period_end timestamptz not null,
  reviewed_by uuid references users(id),
  paid_at timestamptz,
  created_at timestamptz not null default now()
);

alter table withdrawal_requests add column settlement_statement_id uuid references settlement_statements(id);

create table audit_logs (
  id uuid primary key default gen_random_uuid(),
  actor_id uuid references users(id),
  action varchar(100) not null,
  target_type varchar(100) not null,
  target_id varchar(100) not null,
  detail jsonb not null default '{}',
  created_at timestamptz not null default now()
);
create index audit_logs_actor_idx on audit_logs(actor_id, created_at desc);
