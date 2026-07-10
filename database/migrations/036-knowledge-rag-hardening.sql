-- Align persisted ACL constraints with the knowledge authorization engine.

alter table xz_knowledge_base_acl
  drop constraint if exists xz_knowledge_base_acl_subject_type_check;

alter table xz_knowledge_base_acl
  add constraint xz_knowledge_base_acl_subject_type_check
  check (subject_type in ('USER', 'ROLE', 'ORGANIZATION', 'DEPARTMENT', 'TENANT', 'EVERYONE', 'GUEST', 'SHARE'));

alter table xz_knowledge_base_acl
  drop constraint if exists xz_knowledge_base_acl_permission_check;

alter table xz_knowledge_base_acl
  add constraint xz_knowledge_base_acl_permission_check
  check (permission in ('READ', 'VIEW', 'UPLOAD', 'EDIT', 'DELETE', 'SHARE', 'MANAGE'));

create index if not exists idx_xz_knowledge_vector_entries_metadata
  on xz_knowledge_vector_entries using gin(filter_metadata);

create index if not exists idx_xz_rag_runs_tenant_created
  on xz_rag_runs(tenant_id, created_at desc);
