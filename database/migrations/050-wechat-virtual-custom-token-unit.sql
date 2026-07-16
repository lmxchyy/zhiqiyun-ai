-- Ten-cent base unit for server-priced custom Token recharge.
-- The client requests a quantity; unit price and Token grant remain server-owned.

INSERT INTO xz_plans(
  id, code, name, plan_type, product_type, payment_product_code,
  price_cents, grant_points, token_amount, token_rights_value_cents,
  duration_days, concurrency, active, entitlements, raw
)
VALUES (
  'recharge_custom_unit_1yuan', 'custom_unit_1yuan', '自定义金额充值',
  'TOKEN_RECHARGE', 'TOKEN_ONLY', 'TOKEN_CUSTOM_1YUAN',
  10, 10, 10, 10,
  0, 0, TRUE,
  '{"planType":"TOKEN_RECHARGE","productType":"TOKEN_ONLY","customQuantity":true,"minQuantity":1,"maxQuantity":5000,"unitPriceCents":10,"unitTokenAmount":10,"nonWithdrawable":true,"nonTransferable":true,"sort":90}'::jsonb,
  '{"id":"recharge_custom_unit_1yuan","code":"custom_unit_1yuan","name":"自定义金额充值","planType":"TOKEN_RECHARGE","productType":"TOKEN_ONLY","paymentProductCode":"TOKEN_CUSTOM_1YUAN","priceCents":10,"grantPoints":10,"tokenAmount":10,"customQuantity":true,"minQuantity":1,"maxQuantity":5000,"active":true}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  plan_type = EXCLUDED.plan_type,
  product_type = EXCLUDED.product_type,
  payment_product_code = EXCLUDED.payment_product_code,
  price_cents = EXCLUDED.price_cents,
  grant_points = EXCLUDED.grant_points,
  token_amount = EXCLUDED.token_amount,
  token_rights_value_cents = EXCLUDED.token_rights_value_cents,
  duration_days = EXCLUDED.duration_days,
  active = EXCLUDED.active,
  entitlements = EXCLUDED.entitlements,
  raw = EXCLUDED.raw;

INSERT INTO xz_wechat_virtual_product_mappings(
  id, plan_id, wechat_product_id, mode, env, enabled
)
VALUES
  ('wxvp_token_custom_1yuan_prod', 'recharge_custom_unit_1yuan', 'TOKEN_CUSTOM_1YUAN', 'short_series_goods', 0, TRUE),
  ('wxvp_token_custom_1yuan_sandbox', 'recharge_custom_unit_1yuan', 'TOKEN_CUSTOM_1YUAN', 'short_series_goods', 1, TRUE)
ON CONFLICT (plan_id, env) DO UPDATE SET
  wechat_product_id = EXCLUDED.wechat_product_id,
  mode = EXCLUDED.mode,
  enabled = EXCLUDED.enabled,
  updated_at = now();
