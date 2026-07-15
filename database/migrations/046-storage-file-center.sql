-- Platform object storage and file center (phase 1).
-- PostgreSQL is the current primary store; statements are safe to re-run.

create table if not exists xz_storage_configs (
  id text primary key,
  tenant_id text not null default 'platform',
  name text not null,
  provider text not null,
  endpoint text not null,
  signing_endpoint text,
  region text,
  bucket text not null,
  access_key_encrypted text,
  secret_key_encrypted text,
  session_token_encrypted text,
  public_domain text,
  cdn_domain text,
  use_ssl boolean not null default true,
  force_path_style boolean not null default false,
  is_default boolean not null default false,
  is_system boolean not null default false,
  status text not null default 'ENABLED',
  last_test_status text,
  last_test_message text,
  last_test_at timestamptz,
  created_by text,
  updated_by text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  constraint chk_xz_storage_configs_provider check (
    provider in ('minio', 's3', 'aliyun_oss', 'tencent_cos', 'huawei_obs', 'cloudflare_r2')
  ),
  constraint chk_xz_storage_configs_status check (status in ('ENABLED', 'DISABLED'))
);

alter table xz_storage_configs add column if not exists signing_endpoint text;

create unique index if not exists uk_xz_storage_configs_default
  on xz_storage_configs(tenant_id)
  where is_default = true and status = 'ENABLED' and deleted_at is null;
create index if not exists idx_xz_storage_configs_tenant
  on xz_storage_configs(tenant_id, status, provider)
  where deleted_at is null;

create table if not exists xz_file_objects (
  file_id text primary key,
  tenant_id text not null,
  user_id text not null,
  storage_config_id text not null,
  provider text not null,
  bucket text not null,
  object_key text not null,
  original_name text not null,
  stored_name text not null,
  extension text,
  mime_type text,
  file_size bigint not null default 0,
  reserved_size bigint not null default 0,
  file_hash text,
  hash_algorithm text,
  etag text,
  business_type text not null default 'uploads',
  business_id text,
  visibility text not null default 'PRIVATE',
  status text not null default 'PENDING_UPLOAD',
  is_temporary boolean not null default false,
  expires_at timestamptz,
  recycle_expires_at timestamptz,
  reference_count integer not null default 0,
  width integer,
  height integer,
  duration_ms bigint,
  page_count integer,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (storage_config_id, bucket, object_key),
  constraint chk_xz_file_objects_visibility check (
    visibility in ('PRIVATE', 'TENANT', 'SHARED', 'PUBLIC', 'SYSTEM')
  ),
  constraint chk_xz_file_objects_status check (
    status in (
      'PENDING_UPLOAD', 'UPLOADING', 'UPLOADED', 'PROCESSING', 'ACTIVE',
      'UPLOAD_FAILED', 'PROCESSING_FAILED', 'QUARANTINED',
      'DELETE_PENDING', 'DELETED', 'EXPIRED', 'MIGRATION_PENDING', 'MIGRATION_FAILED'
    )
  )
);

create index if not exists idx_xz_file_objects_tenant_status
  on xz_file_objects(tenant_id, status, created_at desc);
create index if not exists idx_xz_file_objects_user_created
  on xz_file_objects(tenant_id, user_id, created_at desc);
create index if not exists idx_xz_file_objects_business
  on xz_file_objects(tenant_id, business_type, business_id);
create index if not exists idx_xz_file_objects_expiration
  on xz_file_objects(is_temporary, expires_at)
  where status not in ('DELETED', 'DELETE_PENDING');
create index if not exists idx_xz_file_objects_recycle
  on xz_file_objects(recycle_expires_at)
  where status = 'DELETE_PENDING';

create table if not exists xz_file_relations (
  id text primary key,
  tenant_id text not null,
  source_file_id text not null references xz_file_objects(file_id),
  target_file_id text not null references xz_file_objects(file_id),
  relation_type text not null,
  created_at timestamptz not null default now(),
  unique (tenant_id, source_file_id, target_file_id, relation_type)
);

create table if not exists xz_tenant_storage_quotas (
  tenant_id text primary key,
  quota_bytes bigint not null default 0,
  used_bytes bigint not null default 0,
  reserved_bytes bigint not null default 0,
  file_count bigint not null default 0,
  warning_percent integer not null default 80,
  critical_percent integer not null default 95,
  updated_at timestamptz not null default now(),
  constraint chk_xz_tenant_storage_quota_nonnegative check (
    quota_bytes >= 0 and used_bytes >= 0 and reserved_bytes >= 0 and file_count >= 0
  )
);

create table if not exists xz_storage_jobs (
  id text primary key,
  tenant_id text not null,
  file_id text,
  job_type text not null,
  status text not null default 'PENDING',
  attempt integer not null default 0,
  max_attempts integer not null default 5,
  run_after timestamptz not null default now(),
  error_code text,
  error_message text,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists idx_xz_storage_jobs_pending
  on xz_storage_jobs(status, run_after)
  where status in ('PENDING', 'RETRY');

insert into xz_role_permissions(role, permission)
values
  ('SUPER_ADMIN', 'storage:view'),
  ('SUPER_ADMIN', 'storage:config:view'),
  ('SUPER_ADMIN', 'storage:config:create'),
  ('SUPER_ADMIN', 'storage:config:update'),
  ('SUPER_ADMIN', 'storage:config:delete'),
  ('SUPER_ADMIN', 'storage:config:test'),
  ('SUPER_ADMIN', 'storage:file:view'),
  ('SUPER_ADMIN', 'storage:file:download'),
  ('SUPER_ADMIN', 'storage:file:delete'),
  ('SUPER_ADMIN', 'storage:file:restore'),
  ('SUPER_ADMIN', 'storage:quota:view'),
  ('SUPER_ADMIN', 'storage:quota:update'),
  ('SUPER_ADMIN', 'storage:job:retry'),
  ('PLATFORM_ADMIN', 'storage:view'),
  ('PLATFORM_ADMIN', 'storage:config:view'),
  ('PLATFORM_ADMIN', 'storage:config:test'),
  ('PLATFORM_ADMIN', 'storage:file:view'),
  ('PLATFORM_ADMIN', 'storage:file:download'),
  ('PLATFORM_ADMIN', 'storage:quota:view')
on conflict do nothing;
