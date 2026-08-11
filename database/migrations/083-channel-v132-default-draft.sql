-- Channel ecosystem V1.3.2 default configuration.
-- Everything remains DRAFT. Publishing requires the future admin workflow.

BEGIN;

INSERT INTO xz_commercial_rule_sets (
  id, tenant_id, rule_code, version, name, description, status,
  effective_start_at, config, created_by
) VALUES (
  'channel_rules_v132_default_v1', 'tenant_default', 'CHANNEL_ECOSYSTEM_V132', 1,
  'Channel Ecosystem V1.3.2 Default',
  'Default configurable rule set. Referral rewards are independent marketing expenses.',
  'DRAFT', '2026-07-26T00:00:00Z',
  jsonb_build_object(
    'directAgentCommissionLevels', 1,
    'allowAncestorAgentCommission', false,
    'operationCenterServiceReviewRequired', true,
    'operationCenterActiveRefundMode', 'FULL_ONLY_REVOKE_REVERSE_REFUND'
  ),
  'MIGRATION'
) ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

INSERT INTO xz_commercial_plan_versions (
  id, tenant_id, rule_set_id, plan_id, version, identity_type,
  price_cents, currency, token_grant_amount, token_rights_value_cents,
  duration_days, config
) VALUES
  ('channel_plan_member_996_v1', 'tenant_default', 'channel_rules_v132_default_v1',
   'plan_ai_creator_996', 1, 'MEMBER', 99600, 'CNY', 40000, 40000, 365,
   jsonb_build_object('scenarioCode', 'MEMBER_PURCHASE')),
  ('channel_plan_agent_996_v1', 'tenant_default', 'channel_rules_v132_default_v1',
   'plan_agent_join_996', 1, 'AGENT', 99600, 'CNY', 20000, 20000, 0,
   jsonb_build_object('scenarioCode', 'AGENT_JOIN')),
  ('channel_plan_operation_center_5000_v1', 'tenant_default', 'channel_rules_v132_default_v1',
   'plan_operation_center_service_5000', 1, 'OPERATION_CENTER', 500000, 'CNY', 0, 0, 0,
   jsonb_build_object('scenarioCode', 'OPERATION_CENTER_SERVICE', 'paymentState', 'REVIEW_REQUIRED'))
ON CONFLICT (rule_set_id, plan_id) DO NOTHING;

INSERT INTO xz_commission_rules (
  id, tenant_id, rule_code, rule_name, product_type, product_id,
  beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
  percentage_bps, calculation_config, priority, freeze_days, refund_policy,
  effective_start_at, version, status, commercial_rule_set_id, commercial_scenario_code
) VALUES
  ('channel_v132_member_agent_v1', 'tenant_default', 'CHANNEL_V132_MEMBER_DIRECT_AGENT', 'Member direct agent',
   'MEMBER_PURCHASE', NULL, 'AGENT', 1, 'FIXED_AMOUNT', 30000, 0, '{}'::jsonb, 10, 7,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'MEMBER_PURCHASE'),
  ('channel_v132_member_oc_v1', 'tenant_default', 'CHANNEL_V132_MEMBER_OPERATION_CENTER', 'Member operation center',
   'MEMBER_PURCHASE', NULL, 'OPERATION_CENTER', 1, 'FIXED_AMOUNT', 20000, 0, '{}'::jsonb, 20, 7,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'MEMBER_PURCHASE'),
  ('channel_v132_member_platform_v1', 'tenant_default', 'CHANNEL_V132_MEMBER_PLATFORM', 'Member platform remainder',
   'MEMBER_PURCHASE', NULL, 'PLATFORM', 0, 'REMAINDER_TO_PLATFORM', 0, 0, '{}'::jsonb, 1000, 0,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'MEMBER_PURCHASE'),
  ('channel_v132_agent_direct_v1', 'tenant_default', 'CHANNEL_V132_AGENT_DIRECT_AGENT', 'Agent join direct agent',
   'AGENT_JOIN', NULL, 'AGENT', 1, 'FIXED_AMOUNT', 30000, 0, '{}'::jsonb, 10, 7,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'AGENT_JOIN'),
  ('channel_v132_agent_oc_v1', 'tenant_default', 'CHANNEL_V132_AGENT_OPERATION_CENTER', 'Agent join operation center',
   'AGENT_JOIN', NULL, 'OPERATION_CENTER', 1, 'FIXED_AMOUNT', 20000, 0, '{}'::jsonb, 20, 7,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'AGENT_JOIN'),
  ('channel_v132_agent_platform_v1', 'tenant_default', 'CHANNEL_V132_AGENT_PLATFORM', 'Agent join platform remainder',
   'AGENT_JOIN', NULL, 'PLATFORM', 0, 'REMAINDER_TO_PLATFORM', 0, 0, '{}'::jsonb, 1000, 0,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'AGENT_JOIN'),
  ('channel_v132_oc_platform_v1', 'tenant_default', 'CHANNEL_V132_OC_PLATFORM', 'Operation center service platform income',
   'OPERATION_CENTER_SERVICE', NULL, 'PLATFORM', 0, 'REMAINDER_TO_PLATFORM', 0, 0, '{}'::jsonb, 1000, 0,
   'REVERSE_OR_RECOVER', '2026-07-26T00:00:00Z', 1, 'DRAFT', 'channel_rules_v132_default_v1', 'OPERATION_CENTER_SERVICE')
ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

INSERT INTO xz_referral_reward_rule_versions (
  id, tenant_id, rule_set_id, rule_code, version, referrer_type,
  beneficiary_type, beneficiary_relation, amount_cents, freeze_days, status, config
) VALUES
  ('channel_v132_ref_oc_to_oc_v1', 'tenant_default', 'channel_rules_v132_default_v1',
   'CHANNEL_V132_OC_REFERS_OC', 1, 'OPERATION_CENTER', 'OPERATION_CENTER', 'REFERRER', 300000, 7, 'DRAFT', '{}'::jsonb),
  ('channel_v132_ref_agent_agent_v1', 'tenant_default', 'channel_rules_v132_default_v1',
   'CHANNEL_V132_AGENT_REFERS_OC_AGENT', 1, 'AGENT', 'AGENT', 'REFERRER', 100000, 7, 'DRAFT', '{}'::jsonb),
  ('channel_v132_ref_agent_oc_v1', 'tenant_default', 'channel_rules_v132_default_v1',
   'CHANNEL_V132_AGENT_REFERS_OC_OWNING_OC', 1, 'AGENT', 'OPERATION_CENTER', 'REFERRER_OPERATION_CENTER', 200000, 7, 'DRAFT', '{}'::jsonb)
ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

COMMIT;
