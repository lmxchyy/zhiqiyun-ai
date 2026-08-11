-- CR-2026-OC-008 production change-window seed
-- Run ONLY after 079-088 and 089-096 are applied on production.
-- Keeps rollout in SHADOW with empty whitelists. Does not enable schedulers.
BEGIN;

-- 083 inserts DRAFT v1 with FULL_ONLY_* already in config.
UPDATE xz_commercial_rule_sets
SET status = 'PUBLISHED',
    published_by = 'CR-2026-OC-008',
    published_at = COALESCE(published_at, now()),
    updated_at = now()
WHERE id = 'channel_rules_v132_default_v1'
  AND status <> 'PUBLISHED';

-- Immutable pin protection blocks config/rbacRole edits on pinned rows.
-- Publish a new version with complete OPERATION_CENTER plan and re-pin.
INSERT INTO xz_commercial_rule_sets (
  id, tenant_id, rule_code, version, name, description, status,
  effective_start_at, config, created_by, published_by, published_at
)
SELECT
  'channel_rules_v132_default_v2',
  tenant_id,
  rule_code,
  2,
  name || ' (CR-2026-OC-008)',
  description,
  'PUBLISHED',
  now(),
  config,
  'CR-2026-OC-008',
  'CR-2026-OC-008',
  now()
FROM xz_commercial_rule_sets
WHERE id = 'channel_rules_v132_default_v1'
ON CONFLICT (tenant_id, rule_code, version) DO UPDATE
SET status = 'PUBLISHED',
    published_by = 'CR-2026-OC-008',
    published_at = COALESCE(xz_commercial_rule_sets.published_at, now()),
    updated_at = now();

INSERT INTO xz_commercial_plan_versions (
  id, tenant_id, rule_set_id, plan_id, version, identity_type,
  price_cents, currency, token_grant_amount, token_rights_value_cents,
  duration_days, config
)
SELECT
  id || '_v2',
  tenant_id,
  'channel_rules_v132_default_v2',
  plan_id,
  2,
  identity_type,
  price_cents,
  currency,
  token_grant_amount,
  token_rights_value_cents,
  duration_days,
  CASE
    WHEN identity_type = 'OPERATION_CENTER' THEN
      COALESCE(config, '{}'::jsonb) || jsonb_build_object(
        'scenarioCode', 'OPERATION_CENTER_SERVICE',
        'rbacRole', 'OPERATION',
        'paymentState', 'REVIEW_REQUIRED'
      )
    ELSE config
  END
FROM xz_commercial_plan_versions
WHERE rule_set_id = 'channel_rules_v132_default_v1'
ON CONFLICT (rule_set_id, plan_id) DO NOTHING;

INSERT INTO xz_commission_rules (
  id, tenant_id, rule_code, rule_name, product_type, product_id,
  beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
  percentage_bps, calculation_config, priority, freeze_days, refund_policy,
  effective_start_at, version, status, commercial_rule_set_id, commercial_scenario_code
)
SELECT
  id || '_v2',
  tenant_id,
  rule_code || '_V2',
  rule_name,
  product_type,
  product_id,
  beneficiary_role,
  relationship_level,
  calculation_type,
  fixed_amount_cents,
  percentage_bps,
  calculation_config,
  priority,
  freeze_days,
  refund_policy,
  effective_start_at,
  2,
  'ACTIVE',
  'channel_rules_v132_default_v2',
  commercial_scenario_code
FROM xz_commission_rules
WHERE commercial_rule_set_id = 'channel_rules_v132_default_v1'
ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

INSERT INTO xz_referral_reward_rule_versions (
  id, tenant_id, rule_set_id, rule_code, version, referrer_type,
  beneficiary_type, beneficiary_relation, amount_cents, freeze_days, status, config
)
SELECT
  id || '_v2',
  tenant_id,
  'channel_rules_v132_default_v2',
  rule_code || '_V2',
  2,
  referrer_type,
  beneficiary_type,
  beneficiary_relation,
  amount_cents,
  freeze_days,
  'PUBLISHED',
  config
FROM xz_referral_reward_rule_versions
WHERE rule_set_id = 'channel_rules_v132_default_v1'
ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

UPDATE xz_channel_rollout_configs
SET pinned_rule_set_id = 'channel_rules_v132_default_v2',
    pinned_rule_set_version = 2,
    mode = 'SHADOW',
    enabled = TRUE,
    real_switch_enabled = FALSE,
    percentage_rollout_enabled = FALSE,
    canary_basis_points = 0,
    allow_order_ids = '[]'::jsonb,
    allow_user_ids = '[]'::jsonb,
    allow_plan_ids = '[]'::jsonb,
    allow_tenant_ids = '[]'::jsonb,
    deny_order_ids = '[]'::jsonb,
    deny_user_ids = '[]'::jsonb,
    change_reason = 'CR-2026-OC-008 pin published v2; keep SHADOW safe defaults',
    updated_by = 'CR-2026-OC-008',
    updated_at = now()
WHERE tenant_id = 'tenant_default';

COMMIT;

SELECT id, version, status, config->>'operationCenterActiveRefundMode' AS refund_mode
FROM xz_commercial_rule_sets
ORDER BY version;

SELECT id, identity_type, config->>'rbacRole' AS rbac_role, config->>'scenarioCode' AS scenario
FROM xz_commercial_plan_versions
WHERE rule_set_id = 'channel_rules_v132_default_v2'
  AND identity_type = 'OPERATION_CENTER';

SELECT pinned_rule_set_id, pinned_rule_set_version, mode,
       real_switch_enabled, percentage_rollout_enabled, canary_basis_points
FROM xz_channel_rollout_configs
WHERE tenant_id = 'tenant_default';
