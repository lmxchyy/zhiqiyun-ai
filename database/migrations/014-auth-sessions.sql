create table if not exists auth_sessions (
  access_token_hash varchar(64) primary key,
  user_id uuid not null references users(id),
  refresh_token_hash varchar(64) not null unique,
  access_expires_at timestamptz not null,
  refresh_expires_at timestamptz not null,
  revoked_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists auth_sessions_user_active_idx
  on auth_sessions(user_id, access_expires_at desc)
  where revoked_at is null;
