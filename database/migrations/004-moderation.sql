create table if not exists moderation_logs (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references users(id),
  content_type varchar(40) not null,
  status varchar(30) not null,
  matched_terms jsonb not null default '[]',
  created_at timestamptz not null default now()
);
