create table if not exists payments (
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
