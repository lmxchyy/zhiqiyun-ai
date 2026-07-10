-- Configurable providers, pgvector indices and AI Agent RAG runtime.

create extension if not exists vector;

create table if not exists xz_knowledge_embedding_profiles (
  id text primary key,
  tenant_id text references xz_tenants(id),
  name text not null,
  provider_channel_id text,
  provider_key text not null,
  model_name text not null,
  dimension integer not null check (dimension > 0),
  batch_size integer not null default 32,
  normalized boolean not null default true,
  status text not null default 'ACTIVE',
  version bigint not null default 1,
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists xz_knowledge_vector_store_profiles (
  id text primary key,
  tenant_id text references xz_tenants(id),
  name text not null,
  provider_key text not null,
  endpoint text,
  credential_ref text,
  collection_prefix text not null default 'xianzhi_kb',
  distance_metric text not null default 'COSINE',
  status text not null default 'ACTIVE',
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists xz_knowledge_rerank_profiles (
  id text primary key,
  tenant_id text references xz_tenants(id),
  name text not null,
  provider_channel_id text,
  provider_key text not null,
  model_name text not null,
  candidate_limit integer not null default 50,
  status text not null default 'ACTIVE',
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists xz_knowledge_ingestion_profiles (
  id text primary key,
  tenant_id text references xz_tenants(id),
  embedding_profile_id text not null references xz_knowledge_embedding_profiles(id),
  vector_store_profile_id text not null references xz_knowledge_vector_store_profiles(id),
  name text not null,
  parser_key text not null default 'plain_text',
  ocr_provider_key text,
  chunker_key text not null default 'fixed',
  chunk_size integer not null default 800,
  overlap integer not null default 120,
  min_tokens integer not null default 40,
  max_tokens integer not null default 1200,
  version bigint not null default 1,
  status text not null default 'ACTIVE',
  cleaning_config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists xz_knowledge_retrieval_profiles (
  id text primary key,
  tenant_id text references xz_tenants(id),
  rerank_profile_id text references xz_knowledge_rerank_profiles(id),
  name text not null,
  search_mode text not null default 'HYBRID',
  top_k integer not null default 8,
  threshold numeric(8,6) not null default 0.55,
  vector_weight numeric(8,6) not null default 0.7,
  keyword_weight numeric(8,6) not null default 0.3,
  context_token_limit integer not null default 6000,
  query_rewrite_enabled boolean not null default true,
  metadata_filter_enabled boolean not null default true,
  status text not null default 'ACTIVE',
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

insert into xz_knowledge_embedding_profiles (
  id, tenant_id, name, provider_key, model_name, dimension, batch_size, normalized, status, version, config
) values (
  'embedding_deterministic_default', null, '内置确定性 Embedding', 'deterministic', 'xianzhi-hash-embedding-v1', 256, 64, true, 'ACTIVE', 1, '{"developmentFallback":true}'::jsonb
) on conflict (id) do nothing;

insert into xz_knowledge_vector_store_profiles (
  id, tenant_id, name, provider_key, collection_prefix, distance_metric, status, config
) values (
  'vector_pgvector_default', null, '默认 pgvector', 'pgvector', 'xianzhi_kb', 'COSINE', 'ACTIVE', '{}'::jsonb
) on conflict (id) do nothing;

insert into xz_knowledge_ingestion_profiles (
  id, tenant_id, embedding_profile_id, vector_store_profile_id, name, parser_key, chunker_key,
  chunk_size, overlap, min_tokens, max_tokens, version, status, cleaning_config
) values (
  'ingestion_default', null, 'embedding_deterministic_default', 'vector_pgvector_default', '默认解析配置',
  'auto', 'fixed', 800, 120, 40, 1200, 1, 'ACTIVE', '{}'::jsonb
) on conflict (id) do nothing;

insert into xz_knowledge_retrieval_profiles (
  id, tenant_id, name, search_mode, top_k, threshold, vector_weight, keyword_weight,
  context_token_limit, query_rewrite_enabled, metadata_filter_enabled, status, config
) values (
  'retrieval_default', null, '默认 Hybrid Search', 'HYBRID', 8, 0.20, 0.7, 0.3,
  6000, false, true, 'ACTIVE', '{}'::jsonb
) on conflict (id) do nothing;

alter table xz_knowledge_bases add column if not exists ingestion_profile_id text references xz_knowledge_ingestion_profiles(id);
alter table xz_knowledge_bases add column if not exists retrieval_profile_id text references xz_knowledge_retrieval_profiles(id);

create table if not exists xz_knowledge_vector_indices (
  id text primary key,
  tenant_id text not null,
  knowledge_base_id text not null,
  embedding_profile_id text not null references xz_knowledge_embedding_profiles(id),
  vector_store_profile_id text not null references xz_knowledge_vector_store_profiles(id),
  revision integer not null,
  dimension integer not null,
  physical_index_name text not null,
  status text not null default 'BUILDING',
  is_active boolean not null default false,
  indexed_chunk_count bigint not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  activated_at timestamptz,
  unique (tenant_id, id),
  unique (knowledge_base_id, revision),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade
);

create unique index if not exists idx_xz_knowledge_vector_indices_active
  on xz_knowledge_vector_indices(knowledge_base_id)
  where is_active = true;

create table if not exists xz_knowledge_vector_entries (
  id text primary key,
  tenant_id text not null,
  vector_index_id text not null,
  chunk_id text not null,
  embedding vector,
  search_text text not null default '',
  search_vector tsvector generated always as (to_tsvector('simple', coalesce(search_text, ''))) stored,
  external_vector_id text,
  embedding_hash text not null,
  filter_metadata jsonb not null default '{}'::jsonb,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (vector_index_id, chunk_id),
  foreign key (tenant_id, vector_index_id) references xz_knowledge_vector_indices(tenant_id, id) on delete cascade,
  foreign key (tenant_id, chunk_id) references xz_knowledge_chunks(tenant_id, id) on delete cascade
);

create index if not exists idx_xz_knowledge_vector_entries_search on xz_knowledge_vector_entries using gin(search_vector);
create index if not exists idx_xz_knowledge_vector_entries_metadata on xz_knowledge_vector_entries using gin(filter_metadata);

create table if not exists xz_ai_agents (
  id text primary key,
  tenant_id text not null references xz_tenants(id),
  owner_user_id text not null references xz_users(id),
  name text not null,
  description text not null default '',
  model_name text not null default '',
  system_prompt text not null default '',
  status text not null default 'DRAFT',
  version bigint not null default 1,
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id)
);

create table if not exists xz_agent_knowledge_bindings (
  id text primary key,
  tenant_id text not null,
  agent_id text not null,
  knowledge_base_id text not null,
  retrieval_profile_id text references xz_knowledge_retrieval_profiles(id),
  priority integer not null default 100,
  weight numeric(8,6) not null default 1,
  enabled boolean not null default true,
  retrieval_overrides jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (agent_id, knowledge_base_id),
  foreign key (tenant_id, agent_id) references xz_ai_agents(tenant_id, id) on delete cascade,
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade
);

create table if not exists xz_ai_agent_conversations (
  id text primary key,
  tenant_id text not null,
  organization_id text,
  agent_id text not null,
  user_id text not null references xz_users(id),
  title text not null default '新对话',
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id),
  foreign key (tenant_id, agent_id) references xz_ai_agents(tenant_id, id),
  foreign key (tenant_id, organization_id) references xz_organizations(tenant_id, id)
);

create table if not exists xz_ai_agent_messages (
  id text primary key,
  tenant_id text not null,
  conversation_id text not null,
  parent_message_id text,
  role text not null check (role in ('system', 'user', 'assistant', 'tool')),
  content text not null,
  status text not null default 'COMPLETED',
  input_tokens integer not null default 0,
  output_tokens integer not null default 0,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique (tenant_id, id),
  foreign key (tenant_id, conversation_id) references xz_ai_agent_conversations(tenant_id, id) on delete cascade,
  foreign key (tenant_id, parent_message_id) references xz_ai_agent_messages(tenant_id, id)
);

create table if not exists xz_rag_runs (
  id text primary key,
  tenant_id text not null,
  conversation_id text not null,
  user_message_id text not null,
  assistant_message_id text,
  agent_id text not null,
  retry_of_run_id text,
  original_query text not null,
  rewritten_query text not null default '',
  status text not null default 'QUEUED',
  retrieval_latency_ms bigint not null default 0,
  generation_latency_ms bigint not null default 0,
  input_tokens integer not null default 0,
  output_tokens integer not null default 0,
  point_cost bigint not null default 0,
  billing_event_id text,
  binding_snapshot jsonb not null default '[]'::jsonb,
  retrieval_snapshot jsonb not null default '{}'::jsonb,
  error_code text,
  error_message text,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  updated_at timestamptz not null default now(),
  unique (tenant_id, id),
  foreign key (tenant_id, conversation_id) references xz_ai_agent_conversations(tenant_id, id) on delete cascade,
  foreign key (tenant_id, user_message_id) references xz_ai_agent_messages(tenant_id, id),
  foreign key (tenant_id, assistant_message_id) references xz_ai_agent_messages(tenant_id, id),
  foreign key (tenant_id, agent_id) references xz_ai_agents(tenant_id, id),
  foreign key (tenant_id, retry_of_run_id) references xz_rag_runs(tenant_id, id)
);

create table if not exists xz_rag_retrieval_hits (
  id text primary key,
  tenant_id text not null,
  rag_run_id text not null,
  knowledge_base_id text not null,
  chunk_id text not null,
  initial_rank integer not null,
  final_rank integer not null,
  vector_score numeric(10,8),
  keyword_score numeric(10,8),
  rerank_score numeric(10,8),
  final_score numeric(10,8) not null,
  metadata_snapshot jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  foreign key (tenant_id, rag_run_id) references xz_rag_runs(tenant_id, id) on delete cascade,
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id),
  foreign key (tenant_id, chunk_id) references xz_knowledge_chunks(tenant_id, id)
);

create table if not exists xz_rag_citations (
  id text primary key,
  tenant_id text not null,
  rag_run_id text not null,
  assistant_message_id text not null,
  document_id text not null,
  document_version_id text not null,
  chunk_id text not null,
  citation_order integer not null,
  document_name_snapshot text not null,
  quote_snapshot text not null,
  locator_snapshot jsonb not null default '{}'::jsonb,
  similarity_score numeric(10,8),
  created_at timestamptz not null default now(),
  unique (rag_run_id, citation_order),
  foreign key (tenant_id, rag_run_id) references xz_rag_runs(tenant_id, id) on delete cascade,
  foreign key (tenant_id, assistant_message_id) references xz_ai_agent_messages(tenant_id, id),
  foreign key (tenant_id, document_id) references xz_knowledge_documents(tenant_id, id),
  foreign key (tenant_id, document_version_id) references xz_knowledge_document_versions(tenant_id, id),
  foreign key (tenant_id, chunk_id) references xz_knowledge_chunks(tenant_id, id)
);

create table if not exists xz_rag_run_events (
  id text primary key,
  tenant_id text not null,
  rag_run_id text not null,
  sequence_no bigint not null,
  event_type text not null,
  payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique (rag_run_id, sequence_no),
  foreign key (tenant_id, rag_run_id) references xz_rag_runs(tenant_id, id) on delete cascade
);

create table if not exists xz_ai_agent_message_feedback (
  id text primary key,
  tenant_id text not null,
  message_id text not null,
  user_id text not null references xz_users(id),
  rating integer not null check (rating in (-1, 1)),
  reason text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (message_id, user_id),
  foreign key (tenant_id, message_id) references xz_ai_agent_messages(tenant_id, id) on delete cascade
);

create index if not exists idx_xz_ai_agents_owner on xz_ai_agents(tenant_id, owner_user_id, status, updated_at desc) where deleted_at is null;
create index if not exists idx_xz_agent_bindings_agent on xz_agent_knowledge_bindings(tenant_id, agent_id, enabled, priority desc);
create index if not exists idx_xz_agent_conversations_user on xz_ai_agent_conversations(tenant_id, user_id, updated_at desc) where deleted_at is null;
create index if not exists idx_xz_agent_messages_conversation on xz_ai_agent_messages(conversation_id, created_at, id);
create index if not exists idx_xz_rag_runs_conversation on xz_rag_runs(conversation_id, created_at desc);
create index if not exists idx_xz_rag_runs_status on xz_rag_runs(tenant_id, status, created_at desc);
create index if not exists idx_xz_rag_hits_run_rank on xz_rag_retrieval_hits(rag_run_id, final_rank);
create index if not exists idx_xz_rag_events_run_sequence on xz_rag_run_events(rag_run_id, sequence_no);
