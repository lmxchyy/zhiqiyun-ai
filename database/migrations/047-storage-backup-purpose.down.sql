drop index if exists idx_xz_storage_configs_purpose;
alter table xz_storage_configs drop column if exists object_prefix;
alter table xz_storage_configs drop column if exists purpose;
