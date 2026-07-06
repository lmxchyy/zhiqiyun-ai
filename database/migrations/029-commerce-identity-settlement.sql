-- Commerce identity settlement module.
-- Adds separated product, member, agent, operation-center and settlement fields.
-- Amounts use integer cents; token/point amounts use integers.

alter table if exists users
  add column if not exists member_level text not null default 'FREE',
  add column if not exists agent_status text not null default 'NONE',
  add column if not exists operation_center_status text not null default 'NONE';

alter table if exists membership_plans
  add column if not exists plan_type text,
  add column if not exists token_amount bigint not null default 0,
  add column if not exists token_rights_value_cents bigint not null default 0,
  add column if not exists member_level text,
  add column if not exists agent_level text;

alter table if exists orders
  add column if not exists order_no text,
  add column if not exists order_type text,
  add column if not exists direct_agent_id uuid,
  add column if not exists parent_agent_id uuid,
  add column if not exists operation_center_id text,
  add column if not exists token_grant_amount bigint not null default 0,
  add column if not exists token_grant_value_cents bigint not null default 0,
  add column if not exists platform_income_cents bigint not null default 0,
  add column if not exists fulfillment_status text not null default 'PENDING',
  add column if not exists fulfilled_at timestamptz;

alter table if exists channel_agents
  add column if not exists operation_center_id text,
  add column if not exists join_order_id uuid,
  add column if not exists join_fee_cents bigint not null default 0,
  add column if not exists token_rights_amount bigint not null default 0;

create table if not exists operation_centers (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null unique references users(id),
  name text not null,
  region text,
  invite_code text not null unique,
  status text not null default 'ACTIVE',
  join_order_id uuid references orders(id),
  join_fee_cents bigint not null default 0,
  approved_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists customer_relations (
  id uuid primary key default gen_random_uuid(),
  customer_user_id uuid not null references users(id),
  direct_agent_id uuid references channel_agents(id),
  parent_agent_id uuid references channel_agents(id),
  operation_center_id uuid references operation_centers(id),
  bind_type text not null,
  bind_start_at timestamptz not null default now(),
  bind_end_at timestamptz,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists token_records (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id),
  order_id uuid references orders(id),
  change_type text not null,
  amount bigint not null,
  balance_after bigint not null,
  remark text,
  created_at timestamptz not null default now(),
  unique(order_id, change_type)
);

alter table if exists commissions
  add column if not exists receiver_type text not null default 'AGENT',
  add column if not exists receiver_id text,
  add column if not exists commission_type text,
  add column if not exists settle_status text not null default 'UNSETTLED',
  add column if not exists updated_at timestamptz;

alter table xz_users
  add column if not exists member_level text,
  add column if not exists agent_status text,
  add column if not exists operation_center_status text;

alter table xz_plans
  add column if not exists plan_type text,
  add column if not exists token_amount bigint not null default 0,
  add column if not exists token_rights_value_cents bigint not null default 0,
  add column if not exists member_level text,
  add column if not exists agent_level text;

alter table xz_orders
  add column if not exists order_no text,
  add column if not exists order_type text,
  add column if not exists direct_agent_id text,
  add column if not exists parent_agent_id text,
  add column if not exists operation_center_id text,
  add column if not exists token_grant_amount bigint not null default 0,
  add column if not exists platform_income_cents bigint not null default 0,
  add column if not exists fulfillment_status text not null default 'PENDING',
  add column if not exists fulfilled_at text;

alter table xz_channel_agents
  add column if not exists operation_center_id text,
  add column if not exists join_order_id text,
  add column if not exists join_fee_cents bigint not null default 0,
  add column if not exists token_rights_amount bigint not null default 0;

create table if not exists xz_operation_centers (
  id text primary key,
  user_id text unique,
  name text,
  region text,
  invite_code text unique,
  status text,
  join_order_id text,
  join_fee_cents bigint not null default 0,
  approved_at text,
  created_at text,
  updated_at text,
  raw jsonb not null default '{}'::jsonb
);

create table if not exists xz_token_records (
  id text primary key,
  user_id text not null,
  order_id text,
  change_type text not null,
  amount bigint not null default 0,
  balance_after bigint not null default 0,
  remark text,
  created_at text,
  raw jsonb not null default '{}'::jsonb,
  unique(order_id, change_type)
);

alter table xz_commissions
  add column if not exists receiver_type text,
  add column if not exists receiver_id text,
  add column if not exists commission_type text,
  add column if not exists settle_status text,
  add column if not exists updated_at text;

create index if not exists idx_xz_operation_centers_user_id on xz_operation_centers(user_id);
create index if not exists idx_xz_token_records_user_id on xz_token_records(user_id);
create index if not exists idx_xz_orders_order_type on xz_orders(order_type);

insert into xz_plans (id, code, name, price_cents, grant_points, duration_days, concurrency, active, entitlements, raw)
values
  ('plan_ai_creator_996', 'ai_creator_996', '996 AI 创作会员包', 99600, 40000, 365, 8, true,
   '{"planType":"MEMBER_PACKAGE","productType":"MEMBER_PACKAGE","displayPrice":"996 元","tokenRightsValueCents":40000,"tokenGrantAmount":40000,"memberLevel":"PRO","newapiGroup":"pro_group","businessDescription":"获得 400 元 AI 点数 / Token，不包含代理商资格","sort":1}'::jsonb,
   '{"id":"plan_ai_creator_996","code":"ai_creator_996","name":"996 AI 创作会员包","planType":"MEMBER_PACKAGE","priceCents":99600,"grantPoints":40000,"tokenAmount":40000,"memberLevel":"PRO","durationDays":365,"concurrency":8,"active":true}'::jsonb),
  ('plan_agent_join_996', 'agent_join_996', '996 代理商开通包', 99600, 20000, 365, 0, true,
   '{"planType":"AGENT_JOIN_PACKAGE","productType":"AGENT_JOIN_PACKAGE","displayPrice":"996 元","tokenRightsValueCents":20000,"tokenGrantAmount":20000,"opensAgent":true,"agentLevel":"AGENT","businessDescription":"开通代理商身份，并获得 200 元 AI 点数 / Token","sort":2}'::jsonb,
   '{"id":"plan_agent_join_996","code":"agent_join_996","name":"996 代理商开通包","planType":"AGENT_JOIN_PACKAGE","priceCents":99600,"grantPoints":20000,"tokenAmount":20000,"agentLevel":"AGENT","durationDays":365,"active":true}'::jsonb),
  ('plan_operation_center_5000', 'operation_center_5000', '5000 运营中心开通包', 500000, 0, 365, 0, true,
   '{"planType":"OPERATION_CENTER_PACKAGE","productType":"OPERATION_CENTER_PACKAGE","displayPrice":"5000 元","tokenRightsValueCents":0,"tokenGrantAmount":0,"opensOperationCenter":true,"businessDescription":"开通运营中心身份，默认平台收入 5000 元","sort":3}'::jsonb,
   '{"id":"plan_operation_center_5000","code":"operation_center_5000","name":"5000 运营中心开通包","planType":"OPERATION_CENTER_PACKAGE","priceCents":500000,"grantPoints":0,"tokenAmount":0,"durationDays":365,"active":true}'::jsonb)
on conflict (id) do update set
  code = excluded.code,
  name = excluded.name,
  price_cents = excluded.price_cents,
  grant_points = excluded.grant_points,
  duration_days = excluded.duration_days,
  concurrency = excluded.concurrency,
  active = excluded.active,
  entitlements = excluded.entitlements,
  raw = excluded.raw;
