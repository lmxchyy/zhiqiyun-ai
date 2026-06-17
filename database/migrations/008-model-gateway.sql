create table if not exists model_call_logs (
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
