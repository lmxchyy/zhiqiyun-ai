-- V1.3.2 Canary remains whitelist-only. This migration does not enable real settlement.
BEGIN;

ALTER TABLE xz_channel_rollout_configs
  ADD COLUMN IF NOT EXISTS allow_tenant_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE xz_channel_rollout_configs
  DROP CONSTRAINT IF EXISTS ck_xz_channel_rollout_allow_tenant_ids;
ALTER TABLE xz_channel_rollout_configs
  ADD CONSTRAINT ck_xz_channel_rollout_allow_tenant_ids
  CHECK (jsonb_typeof(allow_tenant_ids)='array');

COMMIT;
