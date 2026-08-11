-- Storage multipart upload sessions for resumable large-file uploads.

begin;

create table if not exists xz_multipart_uploads (
  id text primary key,
  tenant_id text not null,
  owner_user_id text not null,
  file_id text not null,
  provider_upload_id text not null,
  object_key text not null,
  file_name text not null,
  content_type text not null default '',
  total_size bigint not null,
  part_size bigint not null,
  total_parts integer not null,
  state text not null,
  idempotency_key text,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  constraint chk_xz_multipart_uploads_state check (
    state in ('initialized','uploading','completing','completed','aborted','expired')
  ),
  constraint chk_xz_multipart_uploads_parts check (total_parts > 0 and total_parts <= 10000),
  constraint chk_xz_multipart_uploads_sizes check (total_size > 0 and part_size > 0)
);

create unique index if not exists uq_xz_multipart_uploads_idempotency
  on xz_multipart_uploads(tenant_id, owner_user_id, idempotency_key)
  where idempotency_key is not null and idempotency_key <> '';

create index if not exists idx_xz_multipart_uploads_file
  on xz_multipart_uploads(file_id);

create index if not exists idx_xz_multipart_uploads_expiry
  on xz_multipart_uploads(state, expires_at)
  where state in ('initialized','uploading','completing');

create table if not exists xz_multipart_upload_parts (
  upload_id text not null references xz_multipart_uploads(id) on delete cascade,
  part_number integer not null,
  etag text not null,
  size_bytes bigint not null default 0,
  completed_at timestamptz not null default now(),
  primary key (upload_id, part_number),
  constraint chk_xz_multipart_upload_parts_number check (part_number >= 1 and part_number <= 10000)
);

commit;
