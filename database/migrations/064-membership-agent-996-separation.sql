-- Separate the two CNY 996 products while reusing the existing payment,
-- identity, entitlement and commission tables.

BEGIN;

UPDATE xz_plans
SET name = '知启云AI Pro年度会员',
    plan_type = 'MEMBER_PACKAGE',
    product_type = 'MEMBERSHIP',
    payment_product_code = 'MEMBER_PRO_YEAR_996',
    price_cents = 99600,
    grant_points = 40000,
    token_amount = 40000,
    token_rights_value_cents = 40000,
    member_level = 'PRO',
    agent_level = NULL,
    duration_days = 365,
    active = TRUE,
    entitlements = coalesce(entitlements, '{}'::jsonb) || jsonb_build_object(
      'category', 'MEMBERSHIP',
      'productType', 'MEMBERSHIP',
      'planType', 'MEMBER_PACKAGE',
      'memberLevel', 'PRO',
      'memberDays', 365,
      'creditUnits', 40000,
      'commissionTemplateCode', 'COMMISSION_996_STANDARD',
      'benefits', jsonb_build_array('MEMBER_MODELS', 'MEMBER_PRICING', 'PRIORITY_QUEUE'),
      'testOnly', false,
      'nonWithdrawable', true,
      'nonTransferable', true
    ),
    raw = coalesce(raw, '{}'::jsonb) || jsonb_build_object(
      'name', '知启云AI Pro年度会员',
      'paymentProductCode', 'MEMBER_PRO_YEAR_996',
      'productType', 'MEMBERSHIP',
      'planType', 'MEMBER_PACKAGE',
      'priceCents', 99600,
      'grantPoints', 40000,
      'memberLevel', 'PRO',
      'durationDays', 365,
      'commissionTemplateCode', 'COMMISSION_996_STANDARD',
      'active', true
    )
WHERE id = 'plan_ai_creator_996';

UPDATE xz_plans
SET name = '知启云AI官方代理商',
    plan_type = 'AGENT_JOIN_PACKAGE',
    product_type = 'IDENTITY',
    payment_product_code = 'AGENT_STANDARD_996',
    price_cents = 99600,
    grant_points = 20000,
    token_amount = 20000,
    token_rights_value_cents = 20000,
    member_level = NULL,
    agent_level = 'AGENT',
    duration_days = 0,
    active = TRUE,
    entitlements = coalesce(entitlements, '{}'::jsonb) || jsonb_build_object(
      'category', 'IDENTITY',
      'productType', 'IDENTITY',
      'planType', 'AGENT_JOIN_PACKAGE',
      'agentLevel', 'AGENT',
      'creditUnits', 20000,
      'commissionTemplateCode', 'COMMISSION_996_STANDARD',
      'benefits', jsonb_build_array('AGENT_CENTER', 'INVITE_CODE', 'PROMOTION', 'COMMISSION'),
      'testOnly', false,
      'nonWithdrawable', true,
      'nonTransferable', true
    ),
    raw = coalesce(raw, '{}'::jsonb) || jsonb_build_object(
      'name', '知启云AI官方代理商',
      'paymentProductCode', 'AGENT_STANDARD_996',
      'productType', 'IDENTITY',
      'planType', 'AGENT_JOIN_PACKAGE',
      'priceCents', 99600,
      'grantPoints', 20000,
      'agentLevel', 'AGENT',
      'commissionTemplateCode', 'COMMISSION_996_STANDARD',
      'active', true
    )
WHERE id = 'plan_agent_join_996';

UPDATE xz_wechat_virtual_product_mappings mapping
SET wechat_product_id = CASE mapping.plan_id
      WHEN 'plan_ai_creator_996' THEN 'MEMBER_PRO_YEAR_996'
      WHEN 'plan_agent_join_996' THEN 'AGENT_STANDARD_996'
    END,
    updated_at = now()
WHERE mapping.plan_id IN ('plan_ai_creator_996', 'plan_agent_join_996');

UPDATE xz_commission_rules
SET status = 'INACTIVE', updated_at = now()
WHERE tenant_id = 'tenant_default'
  AND rule_code IN ('AGENT_996_DIRECT_AGENT', 'AGENT_996_OPERATION_CENTER', 'AGENT_996_PLATFORM_REMAINDER')
  AND status = 'ACTIVE';

INSERT INTO xz_commission_rules (
  id, tenant_id, rule_code, rule_name, product_type, product_id,
  beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
  percentage_bps, priority, freeze_days, refund_policy, effective_start_at,
  version, status, calculation_config
)
VALUES
  ('commission_rule_996_standard_agent_v1', 'tenant_default',
   'COMMISSION_996_STANDARD_AGENT', '996标准模板直属代理分润',
   'COMMISSION_TEMPLATE', 'COMMISSION_996_STANDARD', 'AGENT', 1,
   'FIXED_AMOUNT', 30000, 0, 10, 7, 'REVERSE_OR_RECOVER',
   '2020-01-01T00:00:00Z', 1, 'ACTIVE', '{}'::jsonb),
  ('commission_rule_996_standard_operation_v1', 'tenant_default',
   'COMMISSION_996_STANDARD_OPERATION_CENTER', '996标准模板运营中心分润',
   'COMMISSION_TEMPLATE', 'COMMISSION_996_STANDARD', 'OPERATION_CENTER', 1,
   'FIXED_AMOUNT', 20000, 0, 20, 7, 'REVERSE_OR_RECOVER',
   '2020-01-01T00:00:00Z', 1, 'ACTIVE', '{}'::jsonb),
  ('commission_rule_996_standard_platform_v1', 'tenant_default',
   'COMMISSION_996_STANDARD_PLATFORM', '996标准模板平台留存',
   'COMMISSION_TEMPLATE', 'COMMISSION_996_STANDARD', 'PLATFORM', 0,
   'REMAINDER_TO_PLATFORM', 0, 0, 1000, 0, 'REVERSE_OR_RECOVER',
   '2020-01-01T00:00:00Z', 1, 'ACTIVE', '{}'::jsonb)
ON CONFLICT (tenant_id, rule_code, version) DO UPDATE SET
  rule_name = EXCLUDED.rule_name,
  product_type = EXCLUDED.product_type,
  product_id = EXCLUDED.product_id,
  beneficiary_role = EXCLUDED.beneficiary_role,
  relationship_level = EXCLUDED.relationship_level,
  calculation_type = EXCLUDED.calculation_type,
  fixed_amount_cents = EXCLUDED.fixed_amount_cents,
  percentage_bps = EXCLUDED.percentage_bps,
  priority = EXCLUDED.priority,
  freeze_days = EXCLUDED.freeze_days,
  refund_policy = EXCLUDED.refund_policy,
  status = 'ACTIVE',
  updated_at = now();

COMMIT;
