-- Versioned documents, normalized chunks, ingestion jobs and future source connectors.

create table if not exists xz_knowledge_sources (
  id text primary key,
  tenant_id text not null,
  knowledge_base_id text not null,
  source_type text not null,
  name text not null,
  credential_ref text,
  sync_schedule text,
  status text not null default 'ACTIVE',
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (tenant_id, id),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_sync_runs (
  id text primary key,
  tenant_id text not null,
  source_id text not null,
  status text not null default 'QUEUED',
  discovered_count bigint not null default 0,
  changed_count bigint not null default 0,
  failed_count bigint not null default 0,
  error_message text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  foreign key (tenant_id, source_id) references xz_knowledge_sources(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_documents (
  id text primary key,
  tenant_id text not null,
  knowledge_base_id text not null,
  source_id text,
  owner_user_id text not null references xz_users(id),
  latest_version_id text,
  name text not null,
  document_type text not null,
  mime_type text not null default 'text/plain',
  source_key text,
  status text not null default 'UPLOADED',
  metadata jsonb not null default '{}'::jsonb,
  version bigint not null default 1,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade,
  foreign key (tenant_id, source_id) references xz_knowledge_sources(tenant_id, id)
);

create table if not exists xz_knowledge_document_versions (
  id text primary key,
  tenant_id text not null,
  document_id text not null,
  version_no integer not null,
  original_object_key text,
  preview_object_key text,
  mime_type text not null,
  file_size bigint not null default 0,
  content_hash text not null,
  parse_status text not null default 'UPLOADED',
  parser_metadata jsonb not null default '{}'::jsonb,
  created_by text not null references xz_users(id),
  created_at timestamptz not null default now(),
  unique (tenant_id, id),
  unique (document_id, version_no),
  foreign key (tenant_id, document_id) references xz_knowledge_documents(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_document_units (
  id text primary key,
  tenant_id text not null,
  document_version_id text not null,
  unit_type text not null,
  unit_no integer not null,
  title text not null default '',
  content text not null,
  locator jsonb not null default '{}'::jsonb,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique (tenant_id, id),
  unique (document_version_id, unit_type, unit_no),
  foreign key (tenant_id, document_version_id) references xz_knowledge_document_versions(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_chunks (
  id text primary key,
  tenant_id text not null,
  knowledge_base_id text not null,
  document_id text not null,
  document_version_id text not null,
  sequence_no integer not null,
  chunk_key text not null,
  content text not null,
  token_count integer not null default 0,
  page_start integer,
  page_end integer,
  title text not null default '',
  title_path jsonb not null default '[]'::jsonb,
  source_locator jsonb not null default '{}'::jsonb,
  content_hash text not null,
  metadata jsonb not null default '{}'::jsonb,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id),
  unique (document_version_id, chunk_key),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade,
  foreign key (tenant_id, document_id) references xz_knowledge_documents(tenant_id, id) on delete cascade,
  foreign key (tenant_id, document_version_id) references xz_knowledge_document_versions(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_document_acl (
  id text primary key,
  tenant_id text not null,
  document_id text not null,
  subject_type text not null,
  subject_id text,
  permission text not null,
  effect text not null default 'ALLOW',
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  foreign key (tenant_id, document_id) references xz_knowledge_documents(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_document_tags (
  tenant_id text not null,
  document_id text not null,
  tag_id text not null,
  created_at timestamptz not null default now(),
  primary key (document_id, tag_id),
  foreign key (tenant_id, document_id) references xz_knowledge_documents(tenant_id, id) on delete cascade,
  foreign key (tenant_id, tag_id) references xz_knowledge_tags(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_chunk_tags (
  tenant_id text not null,
  chunk_id text not null,
  tag_id text not null,
  created_at timestamptz not null default now(),
  primary key (chunk_id, tag_id),
  foreign key (tenant_id, chunk_id) references xz_knowledge_chunks(tenant_id, id) on delete cascade,
  foreign key (tenant_id, tag_id) references xz_knowledge_tags(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_ingestion_jobs (
  id text primary key,
  tenant_id text not null,
  document_version_id text not null,
  ingestion_profile_id text,
  idempotency_key text not null,
  stage text not null default 'QUEUED',
  status text not null default 'QUEUED',
  attempt integer not null default 0,
  max_attempts integer not null default 3,
  progress integer not null default 0 check (progress between 0 and 100),
  config_snapshot jsonb not null default '{}'::jsonb,
  error_code text,
  error_message text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (tenant_id, id),
  unique (tenant_id, idempotency_key),
  foreign key (tenant_id, document_version_id) references xz_knowledge_document_versions(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_job_steps (
  id text primary key,
  tenant_id text not null,
  job_id text not null,
  step_type text not null,
  status text not null default 'WAITING',
  attempt integer not null default 0,
  input_snapshot jsonb not null default '{}'::jsonb,
  output_snapshot jsonb not null default '{}'::jsonb,
  error_message text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  foreign key (tenant_id, job_id) references xz_knowledge_ingestion_jobs(tenant_id, id) on delete cascade
);

create index if not exists idx_xz_knowledge_documents_kb on xz_knowledge_documents(tenant_id, knowledge_base_id, status, updated_at desc) where deleted_at is null;
create index if not exists idx_xz_knowledge_document_versions_document on xz_knowledge_document_versions(document_id, version_no desc);
create index if not exists idx_xz_knowledge_chunks_kb on xz_knowledge_chunks(tenant_id, knowledge_base_id, updated_at desc) where deleted_at is null;
create index if not exists idx_xz_knowledge_chunks_document on xz_knowledge_chunks(document_id, sequence_no) where deleted_at is null;
create index if not exists idx_xz_knowledge_jobs_status on xz_knowledge_ingestion_jobs(tenant_id, status, created_at desc);

