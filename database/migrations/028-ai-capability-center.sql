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

create index if not exists idx_ai_models_module_status on ai_models(module_code, status);
create index if not exists idx_ai_parameter_schemas_module_model on ai_parameter_schemas(module_code, model_name);
create index if not exists idx_tenant_module_limits_scope on tenant_module_limits(module_code, tenant_id, agent_id, package_id, model_name);
create index if not exists idx_billing_rules_module_model on billing_rules(module_code, model_name, status);

alter table generation_tasks add column if not exists tenant_id text;
alter table generation_tasks add column if not exists agent_id text;
alter table generation_tasks add column if not exists operation_center_id text;
alter table generation_tasks add column if not exists module_code text;
alter table generation_tasks add column if not exists billing_type text;
alter table generation_tasks add column if not exists final_schema_snapshot jsonb not null default '{}'::jsonb;
alter table generation_tasks add column if not exists limit_snapshot jsonb not null default '{}'::jsonb;
alter table generation_tasks add column if not exists upstream_provider text;
alter table generation_tasks add column if not exists upstream_request_id text;
alter table generation_tasks add column if not exists user_charge_amount bigint not null default 0;
alter table generation_tasks add column if not exists upstream_cost bigint not null default 0;
alter table generation_tasks add column if not exists platform_profit bigint not null default 0;
alter table generation_tasks add column if not exists agent_commission bigint not null default 0;
alter table generation_tasks add column if not exists operation_center_commission bigint not null default 0;
alter table generation_tasks add column if not exists failure_reason text;

alter table xz_generation_tasks add column if not exists module_code text;
alter table xz_generation_tasks add column if not exists billing_type text;

alter table xz_billing_events add column if not exists tenant_id text;
alter table xz_billing_events add column if not exists operation_center_id text;
alter table xz_billing_events add column if not exists module_code text;

create index if not exists idx_generation_tasks_module_code on generation_tasks(module_code);
create index if not exists idx_xz_generation_tasks_module_code on xz_generation_tasks(module_code);
create index if not exists idx_xz_billing_events_module_code on xz_billing_events(module_code);
