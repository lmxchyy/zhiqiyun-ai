-- Explicit purpose and prefix boundaries for non-business storage configs.
alter table xz_storage_configs add column if not exists purpose text not null default '';
alter table xz_storage_configs add column if not exists object_prefix text not null default '';

create index if not exists idx_xz_storage_configs_purpose
  on xz_storage_configs(purpose, status)
  where deleted_at is null;
