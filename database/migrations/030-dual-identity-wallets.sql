-- Dual identity commerce model.
-- users remains the only login subject. Agent identity, user wallet and agent wallet are separate ledgers.

alter table if exists orders
  add column if not exists buyer_user_id uuid,
  add column if not exists business_order_type text,
  add column if not exists token_amount bigint not null default 0,
  add column if not exists reward_snapshot jsonb not null default '{}'::jsonb;

update orders
set buyer_user_id = user_id
where buyer_user_id is null;

create table if not exists agent_profiles (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null unique references users(id),
  parent_agent_id uuid references agent_profiles(id),
  operation_center_id uuid references operation_centers(id),
  level integer not null default 2,
  status varchar(20) not null default 'ACTIVE',
  invite_code varchar(50) not null unique,
  join_order_id uuid references orders(id),
  join_fee_cents bigint not null default 0,
  token_rights_amount bigint not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

insert into agent_profiles (id, user_id, parent_agent_id, operation_center_id, level, status, invite_code, join_order_id, join_fee_cents, token_rights_amount, created_at, updated_at)
select
  ca.id,
  ca.user_id,
  ca.parent_id,
  null,
  ca.level,
  ca.status,
  ca.invite_code,
  ca.join_order_id,
  ca.join_fee_cents,
  ca.token_rights_amount,
  ca.created_at,
  coalesce(ca.created_at, now())
from channel_agents ca
on conflict (id) do update set
  user_id = excluded.user_id,
  parent_agent_id = excluded.parent_agent_id,
  level = excluded.level,
  status = excluded.status,
  invite_code = excluded.invite_code,
  join_order_id = excluded.join_order_id,
  join_fee_cents = excluded.join_fee_cents,
  token_rights_amount = excluded.token_rights_amount,
  updated_at = now();

create table if not exists user_wallets (
  user_id uuid primary key references users(id),
  token_balance bigint not null default 0,
  cash_balance_cents bigint not null default 0,
  frozen_token bigint not null default 0,
  total_token_granted bigint not null default 0,
  total_token_used bigint not null default 0,
  updated_at timestamptz not null default now()
);

insert into user_wallets (user_id, token_balance, frozen_token, total_token_granted, total_token_used)
select user_id, available, frozen, 0, 0
from point_accounts
on conflict (user_id) do update set
  token_balance = excluded.token_balance,
  frozen_token = excluded.frozen_token,
  total_token_granted = excluded.total_token_granted,
  total_token_used = excluded.total_token_used,
  updated_at = now();

create table if not exists agent_wallets (
  agent_id uuid primary key references agent_profiles(id),
  user_id uuid not null references users(id),
  commission_balance_cents bigint not null default 0,
  withdrawable_balance_cents bigint not null default 0,
  frozen_commission_cents bigint not null default 0,
  total_commission_cents bigint not null default 0,
  total_withdrawn_cents bigint not null default 0,
  updated_at timestamptz not null default now()
);

with commission_totals as (
  select
    agent_id,
    coalesce(sum(amount_cents) filter (where upper(coalesce(status, '')) not in ('CANCELED', 'CANCELLED', 'VOID')), 0) as total_commission_cents
  from commissions
  group by agent_id
),
withdrawal_totals as (
  select
    agent_id,
    coalesce(sum(amount_cents) filter (where upper(coalesce(status, '')) in ('PENDING', 'REVIEWING', 'FROZEN')), 0) as frozen_commission_cents,
    coalesce(sum(amount_cents) filter (where upper(coalesce(status, '')) in ('APPROVED', 'PAID', 'SETTLED', 'SUCCEEDED')), 0) as total_withdrawn_cents
  from withdrawal_requests
  group by agent_id
)
insert into agent_wallets (agent_id, user_id, commission_balance_cents, withdrawable_balance_cents, frozen_commission_cents, total_commission_cents, total_withdrawn_cents)
select
  ap.id,
  ap.user_id,
  coalesce(ct.total_commission_cents, 0) as commission_balance_cents,
  greatest(
    coalesce(ct.total_commission_cents, 0)
    - coalesce(wt.frozen_commission_cents, 0)
    - coalesce(wt.total_withdrawn_cents, 0),
    0
  ) as withdrawable_balance_cents,
  coalesce(wt.frozen_commission_cents, 0) as frozen_commission_cents,
  coalesce(ct.total_commission_cents, 0) as total_commission_cents,
  coalesce(wt.total_withdrawn_cents, 0) as total_withdrawn_cents
from agent_profiles ap
left join commission_totals ct on ct.agent_id = ap.id
left join withdrawal_totals wt on wt.agent_id = ap.id
on conflict (agent_id) do update set
  user_id = excluded.user_id,
  commission_balance_cents = excluded.commission_balance_cents,
  withdrawable_balance_cents = excluded.withdrawable_balance_cents,
  frozen_commission_cents = excluded.frozen_commission_cents,
  total_commission_cents = excluded.total_commission_cents,
  total_withdrawn_cents = excluded.total_withdrawn_cents,
  updated_at = now();

alter table if exists xz_orders
  add column if not exists buyer_user_id text,
  add column if not exists business_order_type text,
  add column if not exists token_amount bigint not null default 0,
  add column if not exists token_grant_value_cents bigint not null default 0,
  add column if not exists reward_snapshot jsonb not null default '{}'::jsonb;

create table if not exists xz_agent_profiles (
  id text primary key,
  user_id text not null unique,
  parent_agent_id text,
  operation_center_id text,
  level int not null default 2,
  status text not null default 'ACTIVE',
  invite_code text not null,
  join_order_id text,
  join_fee_cents bigint not null default 0,
  token_rights_amount bigint not null default 0,
  created_at text,
  updated_at text,
  raw jsonb not null default '{}'::jsonb
);

insert into xz_agent_profiles (id, user_id, parent_agent_id, operation_center_id, level, status, invite_code, join_order_id, join_fee_cents, token_rights_amount, created_at, updated_at, raw)
select id, user_id, parent_id, operation_center_id, level, status, invite_code, join_order_id, join_fee_cents, token_rights_amount, created_at, updated_at, raw
from xz_channel_agents
on conflict (id) do update set
  user_id = excluded.user_id,
  parent_agent_id = excluded.parent_agent_id,
  operation_center_id = excluded.operation_center_id,
  level = excluded.level,
  status = excluded.status,
  invite_code = excluded.invite_code,
  join_order_id = excluded.join_order_id,
  join_fee_cents = excluded.join_fee_cents,
  token_rights_amount = excluded.token_rights_amount,
  updated_at = excluded.updated_at,
  raw = excluded.raw;

create table if not exists xz_user_wallets (
  user_id text primary key,
  token_balance bigint not null default 0,
  cash_balance_cents bigint not null default 0,
  frozen_token bigint not null default 0,
  total_token_granted bigint not null default 0,
  total_token_used bigint not null default 0,
  updated_at timestamptz not null default now(),
  raw jsonb not null default '{}'::jsonb
);

create table if not exists xz_agent_wallets (
  agent_id text primary key,
  user_id text not null,
  commission_balance_cents bigint not null default 0,
  withdrawable_balance_cents bigint not null default 0,
  frozen_commission_cents bigint not null default 0,
  total_commission_cents bigint not null default 0,
  total_withdrawn_cents bigint not null default 0,
  updated_at timestamptz not null default now(),
  raw jsonb not null default '{}'::jsonb
);

create index if not exists idx_agent_profiles_user_id on agent_profiles(user_id);
create index if not exists idx_orders_buyer_user_id on orders(buyer_user_id);
create index if not exists idx_xz_agent_profiles_user_id on xz_agent_profiles(user_id);
create index if not exists idx_xz_orders_buyer_user_id on xz_orders(buyer_user_id);
