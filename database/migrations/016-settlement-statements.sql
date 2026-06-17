create table if not exists settlement_statements (
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

alter table withdrawal_requests
  add column if not exists settlement_statement_id uuid references settlement_statements(id);
