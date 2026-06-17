create table if not exists agent_versions (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id),
  version integer not null,
  actor_id uuid references users(id),
  reason varchar(100) not null,
  snapshot jsonb not null,
  created_at timestamptz not null default now(),
  unique (agent_id, version)
);

create table if not exists knowledge_bases (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  name varchar(150) not null,
  description text,
  document_count integer not null default 0,
  created_at timestamptz not null default now()
);

create table if not exists knowledge_documents (
  id uuid primary key default gen_random_uuid(),
  knowledge_base_id uuid not null references knowledge_bases(id),
  owner_id uuid not null references users(id),
  name varchar(255) not null,
  content text not null,
  chunks jsonb not null default '[]',
  created_at timestamptz not null default now()
);
