create table if not exists invoices (
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
