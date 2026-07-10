-- Knowledge/RAG tenancy foundation.
-- Runtime identities in the current PostgreSQL primary store use text IDs in xz_* tables.

create table if not exists xz_tenants (
  id text primary key,
  tenant_type text not null check (tenant_type in ('PLATFORM', 'ENTERPRISE', 'PERSONAL')),
  enterprise_ref text,
  owner_user_id text references xz_users(id),
  name text not null,
  status text not null default 'ACTIVE',
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (id, tenant_type)
);

create unique index if not exists idx_xz_tenants_enterprise_ref
  on xz_tenants(enterprise_ref)
  where enterprise_ref is not null;

create table if not exists xz_tenant_members (
  id text primary key,
  tenant_id text not null references xz_tenants(id),
  user_id text not null references xz_users(id),
  role text not null check (role in ('PLATFORM_ADMIN', 'ENTERPRISE_ADMIN', 'MEMBER', 'GUEST')),
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (tenant_id, user_id),
  unique (tenant_id, id)
);

create table if not exists xz_organizations (
  id text primary key,
  tenant_id text not null references xz_tenants(id),
  parent_id text,
  organization_type text not null default 'DEPARTMENT',
  name text not null,
  status text not null default 'ACTIVE',
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (tenant_id, id),
  foreign key (tenant_id, parent_id) references xz_organizations(tenant_id, id)
);

create table if not exists xz_organization_members (
  id text primary key,
  tenant_id text not null,
  organization_id text not null,
  tenant_member_id text not null,
  role text not null default 'MEMBER',
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  unique (organization_id, tenant_member_id),
  foreign key (tenant_id, organization_id) references xz_organizations(tenant_id, id),
  foreign key (tenant_id, tenant_member_id) references xz_tenant_members(tenant_id, id)
);

create table if not exists xz_knowledge_categories (
  id text primary key,
  tenant_id text not null references xz_tenants(id),
  parent_id text,
  name text not null,
  sort_order integer not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (tenant_id, id),
  unique (tenant_id, parent_id, name),
  foreign key (tenant_id, parent_id) references xz_knowledge_categories(tenant_id, id)
);

create table if not exists xz_knowledge_bases (
  id text primary key,
  tenant_id text not null references xz_tenants(id),
  organization_id text,
  owner_user_id text not null references xz_users(id),
  category_id text,
  knowledge_type text not null check (knowledge_type in ('ENTERPRISE', 'DEPARTMENT', 'PERSONAL', 'AGENT')),
  name text not null,
  description text not null default '',
  logo_object_key text,
  visibility text not null default 'PRIVATE' check (visibility in ('PRIVATE', 'TENANT', 'ORGANIZATION', 'SHARED')),
  status text not null default 'ACTIVE',
  document_count bigint not null default 0,
  chunk_count bigint not null default 0,
  metadata jsonb not null default '{}'::jsonb,
  version bigint not null default 1,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id),
  foreign key (tenant_id, organization_id) references xz_organizations(tenant_id, id),
  foreign key (tenant_id, category_id) references xz_knowledge_categories(tenant_id, id)
);

create table if not exists xz_knowledge_tags (
  id text primary key,
  tenant_id text not null references xz_tenants(id),
  name text not null,
  color text not null default '',
  created_at timestamptz not null default now(),
  unique (tenant_id, id),
  unique (tenant_id, name)
);

create table if not exists xz_knowledge_base_tags (
  tenant_id text not null,
  knowledge_base_id text not null,
  tag_id text not null,
  created_at timestamptz not null default now(),
  primary key (knowledge_base_id, tag_id),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade,
  foreign key (tenant_id, tag_id) references xz_knowledge_tags(tenant_id, id) on delete cascade
);

create table if not exists xz_knowledge_base_acl (
  id text primary key,
  tenant_id text not null,
  knowledge_base_id text not null,
  subject_type text not null check (subject_type in ('USER', 'ROLE', 'ORGANIZATION', 'TENANT', 'SHARE')),
  subject_id text,
  permission text not null check (permission in ('VIEW', 'UPLOAD', 'EDIT', 'DELETE', 'SHARE', 'MANAGE')),
  effect text not null default 'ALLOW' check (effect in ('ALLOW', 'DENY')),
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade
);

create unique index if not exists idx_xz_knowledge_base_acl_rule
  on xz_knowledge_base_acl(knowledge_base_id, subject_type, coalesce(subject_id, ''), permission);

create table if not exists xz_knowledge_shares (
  id text primary key,
  tenant_id text not null,
  knowledge_base_id text not null,
  created_by text not null references xz_users(id),
  token_hash text not null unique,
  permissions jsonb not null default '["VIEW"]'::jsonb,
  password_hash text,
  status text not null default 'ACTIVE',
  max_calls bigint not null default 0,
  call_count bigint not null default 0,
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  foreign key (tenant_id, knowledge_base_id) references xz_knowledge_bases(tenant_id, id) on delete cascade
);

create index if not exists idx_xz_tenant_members_user on xz_tenant_members(user_id, status);
create index if not exists idx_xz_organizations_tenant_parent on xz_organizations(tenant_id, parent_id, status);
create index if not exists idx_xz_knowledge_bases_scope on xz_knowledge_bases(tenant_id, organization_id, status, updated_at desc);
create index if not exists idx_xz_knowledge_bases_owner on xz_knowledge_bases(owner_user_id, updated_at desc);
create index if not exists idx_xz_knowledge_base_acl_subject on xz_knowledge_base_acl(tenant_id, subject_type, subject_id, permission);

