create table if not exists geo_content_publications (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id),
  brand_id uuid not null references geo_brands(id),
  content_id uuid not null references geo_contents(id),
  platform varchar(100) not null,
  url text not null,
  status varchar(30) not null default 'PUBLISHED',
  published_at timestamptz not null,
  metrics_history jsonb not null default '[]',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(content_id, url)
);

alter table geo_contents
  add column if not exists publications integer not null default 0,
  add column if not exists last_published_at timestamptz;
