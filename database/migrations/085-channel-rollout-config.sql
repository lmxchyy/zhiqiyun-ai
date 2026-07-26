-- Channel ecosystem V1.3.2: pinned shadow rules and future canary configuration.
-- Runtime settlement remains legacy. real_switch_enabled defaults to false.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_channel_rollout_configs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  config_version INT NOT NULL DEFAULT 1 CHECK (config_version > 0),
  mode TEXT NOT NULL DEFAULT 'SHADOW' CHECK (mode IN ('LEGACY', 'SHADOW', 'V132_CANARY', 'V132_FULL')),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  pinned_rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  pinned_rule_set_version INT NOT NULL CHECK (pinned_rule_set_version > 0),
  canary_basis_points INT NOT NULL DEFAULT 0 CHECK (canary_basis_points BETWEEN 0 AND 10000),
  hash_salt TEXT NOT NULL DEFAULT '',
  allow_order_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allow_order_ids) = 'array'),
  allow_user_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allow_user_ids) = 'array'),
  deny_order_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(deny_order_ids) = 'array'),
  deny_user_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(deny_user_ids) = 'array'),
  real_switch_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  change_reason TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT 'MIGRATION',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id),
  CHECK (mode <> 'SHADOW' OR canary_basis_points = 0),
  CHECK (mode <> 'V132_CANARY' OR (canary_basis_points BETWEEN 1 AND 9999 AND hash_salt <> '')),
  CHECK (mode NOT IN ('V132_CANARY', 'V132_FULL') OR real_switch_enabled = TRUE)
);

CREATE TABLE IF NOT EXISTS xz_channel_rollout_config_history (
  id BIGSERIAL PRIMARY KEY,
  rollout_config_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  config_version INT NOT NULL,
  config_snapshot JSONB NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  changed_by TEXT NOT NULL,
  change_reason TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_xz_channel_rollout_config_history_tenant
  ON xz_channel_rollout_config_history(tenant_id, config_version DESC);

CREATE OR REPLACE FUNCTION xz_version_channel_rollout_config()
RETURNS trigger AS $$
BEGIN
  INSERT INTO xz_channel_rollout_config_history (
    rollout_config_id, tenant_id, config_version, config_snapshot, changed_by, change_reason
  ) VALUES (
    OLD.id, OLD.tenant_id, OLD.config_version, to_jsonb(OLD), NEW.updated_by, NEW.change_reason
  );
  NEW.config_version := OLD.config_version + 1;
  NEW.updated_at := now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_channel_rollout_config_version ON xz_channel_rollout_configs;
CREATE TRIGGER trg_xz_channel_rollout_config_version
BEFORE UPDATE ON xz_channel_rollout_configs
FOR EACH ROW EXECUTE FUNCTION xz_version_channel_rollout_config();

INSERT INTO xz_channel_rollout_configs (
  id, tenant_id, mode, enabled, pinned_rule_set_id, pinned_rule_set_version,
  canary_basis_points, hash_salt, real_switch_enabled, change_reason, updated_by
) VALUES (
  'channel_rollout_tenant_default', 'tenant_default', 'SHADOW', TRUE,
  'channel_rules_v132_default_v1', 1, 0, 'channel-v132-tenant-default-v1', FALSE,
  'Pin the accepted V1.3.2 shadow rule version without switching settlement', 'MIGRATION'
) ON CONFLICT (tenant_id) DO NOTHING;

CREATE OR REPLACE FUNCTION xz_protect_pinned_channel_rule_value()
RETURNS trigger AS $$
DECLARE
  bound_rule_set_id TEXT;
  old_value JSONB;
  new_value JSONB;
BEGIN
  IF TG_TABLE_NAME = 'xz_commercial_rule_sets' THEN
    bound_rule_set_id := OLD.id;
  ELSIF TG_TABLE_NAME = 'xz_commission_rules' THEN
    bound_rule_set_id := OLD.commercial_rule_set_id;
  ELSE
    bound_rule_set_id := OLD.rule_set_id;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM xz_channel_rollout_configs
    WHERE enabled = TRUE AND pinned_rule_set_id = bound_rule_set_id
  ) THEN
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'pinned channel rule values cannot be deleted';
  END IF;
  old_value := to_jsonb(OLD);
  new_value := to_jsonb(NEW);
  IF TG_TABLE_NAME = 'xz_commercial_rule_sets' THEN
    old_value := old_value - ARRAY['status','published_by','published_at','updated_at','effective_end_at'];
    new_value := new_value - ARRAY['status','published_by','published_at','updated_at','effective_end_at'];
  ELSIF TG_TABLE_NAME IN ('xz_commission_rules', 'xz_referral_reward_rule_versions') THEN
    old_value := old_value - ARRAY['status','updated_at'];
    new_value := new_value - ARRAY['status','updated_at'];
  ELSE
    old_value := old_value - 'updated_at';
    new_value := new_value - 'updated_at';
  END IF;
  IF old_value <> new_value THEN
    RAISE EXCEPTION 'pinned channel rule values are immutable; create and bind a new version';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_pinned_rule_set_value ON xz_commercial_rule_sets;
CREATE TRIGGER trg_xz_pinned_rule_set_value
BEFORE UPDATE OR DELETE ON xz_commercial_rule_sets
FOR EACH ROW EXECUTE FUNCTION xz_protect_pinned_channel_rule_value();

DROP TRIGGER IF EXISTS trg_xz_pinned_plan_value ON xz_commercial_plan_versions;
CREATE TRIGGER trg_xz_pinned_plan_value
BEFORE UPDATE OR DELETE ON xz_commercial_plan_versions
FOR EACH ROW EXECUTE FUNCTION xz_protect_pinned_channel_rule_value();

DROP TRIGGER IF EXISTS trg_xz_pinned_commission_rule_value ON xz_commission_rules;
CREATE TRIGGER trg_xz_pinned_commission_rule_value
BEFORE UPDATE OR DELETE ON xz_commission_rules
FOR EACH ROW WHEN (OLD.commercial_rule_set_id IS NOT NULL)
EXECUTE FUNCTION xz_protect_pinned_channel_rule_value();

DROP TRIGGER IF EXISTS trg_xz_pinned_referral_rule_value ON xz_referral_reward_rule_versions;
CREATE TRIGGER trg_xz_pinned_referral_rule_value
BEFORE UPDATE OR DELETE ON xz_referral_reward_rule_versions
FOR EACH ROW EXECUTE FUNCTION xz_protect_pinned_channel_rule_value();

COMMIT;
