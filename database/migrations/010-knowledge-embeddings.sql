alter table knowledge_documents
  add column if not exists embeddings jsonb not null default '[]';
