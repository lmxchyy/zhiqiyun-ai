-- Normalize the first agent-commerce virtual products for WeChat virtual payment.
-- Prices and entitlements remain server-owned in xz_plans; clients only submit payment_product_code.

UPDATE xz_plans
SET plan_type = 'TOKEN_RECHARGE',
    product_type = 'TOKEN_ONLY',
    payment_product_code = CASE id
      WHEN 'recharge_small' THEN 'TOKEN_SMALL_2500'
      WHEN 'recharge_standard' THEN 'TOKEN_STANDARD_15000'
      WHEN 'recharge_100' THEN 'TOKEN_10000'
      WHEN 'recharge_business' THEN 'TOKEN_BUSINESS_50000'
      WHEN 'recharge_400' THEN 'TOKEN_40000'
      WHEN 'recharge_enterprise' THEN 'TOKEN_ENTERPRISE_200000'
    END,
    token_amount = grant_points,
    token_rights_value_cents = price_cents,
    entitlements = coalesce(entitlements, '{}'::jsonb) || jsonb_build_object(
      'planType', 'TOKEN_RECHARGE',
      'productType', 'TOKEN_ONLY',
      'tokenGrantAmount', grant_points,
      'nonWithdrawable', true,
      'nonTransferable', true
    ),
    raw = coalesce(raw, '{}'::jsonb) || jsonb_build_object(
      'planType', 'TOKEN_RECHARGE',
      'productType', 'TOKEN_ONLY',
      'paymentProductCode', CASE id
        WHEN 'recharge_small' THEN 'TOKEN_SMALL_2500'
        WHEN 'recharge_standard' THEN 'TOKEN_STANDARD_15000'
        WHEN 'recharge_100' THEN 'TOKEN_10000'
        WHEN 'recharge_business' THEN 'TOKEN_BUSINESS_50000'
        WHEN 'recharge_400' THEN 'TOKEN_40000'
        WHEN 'recharge_enterprise' THEN 'TOKEN_ENTERPRISE_200000'
      END,
      'tokenAmount', grant_points
    )
WHERE id IN (
  'recharge_small', 'recharge_standard', 'recharge_100',
  'recharge_business', 'recharge_400', 'recharge_enterprise'
);

UPDATE xz_plans
SET plan_type = CASE id
      WHEN 'plan_ai_creator_996' THEN 'MEMBER_PACKAGE'
      WHEN 'plan_agent_join_996' THEN 'AGENT_JOIN_PACKAGE'
    END,
    product_type = 'TOKEN_UPGRADE',
    payment_product_code = CASE id
      WHEN 'plan_ai_creator_996' THEN 'MEMBER_YEAR_996'
      WHEN 'plan_agent_join_996' THEN 'AGENT_JOIN_996'
    END,
    token_amount = grant_points,
    token_rights_value_cents = CASE id
      WHEN 'plan_ai_creator_996' THEN 40000
      WHEN 'plan_agent_join_996' THEN 20000
    END,
    member_level = CASE WHEN id = 'plan_ai_creator_996' THEN 'PRO' ELSE member_level END,
    agent_level = CASE WHEN id = 'plan_agent_join_996' THEN 'AGENT' ELSE agent_level END,
    entitlements = coalesce(entitlements, '{}'::jsonb) || jsonb_build_object(
      'productType', 'TOKEN_UPGRADE',
      'tokenGrantAmount', grant_points,
      'memberLevel', CASE WHEN id = 'plan_ai_creator_996' THEN 'PRO' ELSE '' END,
      'agentLevel', CASE WHEN id = 'plan_agent_join_996' THEN 'AGENT' ELSE '' END,
      'nonWithdrawable', true,
      'nonTransferable', true
    ),
    raw = coalesce(raw, '{}'::jsonb) || jsonb_build_object(
      'productType', 'TOKEN_UPGRADE',
      'paymentProductCode', CASE id
        WHEN 'plan_ai_creator_996' THEN 'MEMBER_YEAR_996'
        WHEN 'plan_agent_join_996' THEN 'AGENT_JOIN_996'
      END,
      'tokenAmount', grant_points,
      'memberLevel', CASE WHEN id = 'plan_ai_creator_996' THEN 'PRO' ELSE '' END,
      'agentLevel', CASE WHEN id = 'plan_agent_join_996' THEN 'AGENT' ELSE '' END
    )
WHERE id IN ('plan_ai_creator_996', 'plan_agent_join_996');

INSERT INTO xz_wechat_virtual_product_mappings(
  id, plan_id, wechat_product_id, mode, env, enabled
)
SELECT
  'wxvp_' || replace(plan.id, 'plan_', '') || CASE env.value WHEN 0 THEN '_prod' ELSE '_sandbox' END,
  plan.id,
  plan.payment_product_code,
  'short_series_goods',
  env.value,
  TRUE
FROM xz_plans plan
CROSS JOIN (VALUES (0), (1)) AS env(value)
WHERE plan.id IN (
  'recharge_small', 'recharge_standard', 'recharge_100',
  'recharge_business', 'recharge_400', 'recharge_enterprise',
  'plan_ai_creator_996', 'plan_agent_join_996'
)
ON CONFLICT (plan_id, env) DO UPDATE SET
  wechat_product_id = EXCLUDED.wechat_product_id,
  mode = EXCLUDED.mode,
  enabled = EXCLUDED.enabled,
  updated_at = now();
