create table if not exists platform_state (
  id varchar(50) primary key,
  state jsonb not null,
  version bigint not null default 0,
  updated_at timestamptz not null default now()
);
