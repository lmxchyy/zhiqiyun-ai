create table if not exists model_pricing_rules (
  id uuid primary key default gen_random_uuid(),
  model_code varchar(100) not null,
  capability varchar(40) not null,
  point_cost bigint not null check (point_cost > 0),
  status varchar(30) not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  unique(model_code, capability)
);

alter table model_definitions
  add column if not exists provider_code varchar(100),
  add column if not exists capabilities jsonb not null default '[]',
  add column if not exists tier varchar(30) not null default 'PAID',
  add column if not exists point_costs jsonb not null default '{}',
  add column if not exists status varchar(30) not null default 'ACTIVE';
