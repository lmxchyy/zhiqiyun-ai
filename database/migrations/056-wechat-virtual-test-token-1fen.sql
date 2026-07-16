-- One-cent fixed Token product for low-value real WeChat virtual-payment
-- integration checks. Keep mappings disabled until the matching WeChat item
-- has been created and published.

INSERT INTO xz_plans(
  id, code, name, plan_type, product_type, payment_product_code,
  price_cents, grant_points, token_amount, token_rights_value_cents,
  duration_days, concurrency, active, entitlements, raw
)
VALUES (
  'recharge_test_1fen', 'test_1fen', 'Token支付联调1分',
  'TOKEN_RECHARGE', 'TOKEN_ONLY', 'TOKEN_TEST_1FEN',
  1, 1, 1, 1,
  0, 0, TRUE,
  '{"planType":"TOKEN_RECHARGE","productType":"TOKEN_ONLY","testOnly":true,"nonWithdrawable":true,"nonTransferable":true,"sort":9999}'::jsonb,
  '{"id":"recharge_test_1fen","code":"test_1fen","name":"Token支付联调1分","planType":"TOKEN_RECHARGE","productType":"TOKEN_ONLY","paymentProductCode":"TOKEN_TEST_1FEN","priceCents":1,"grantPoints":1,"tokenAmount":1,"testOnly":true,"active":true}'::jsonb
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
  ('wxvp_token_test_1fen_prod', 'recharge_test_1fen', 'TOKEN_TEST_1FEN', 'short_series_goods', 0, FALSE),
  ('wxvp_token_test_1fen_sandbox', 'recharge_test_1fen', 'TOKEN_TEST_1FEN', 'short_series_goods', 1, FALSE)
ON CONFLICT (plan_id, env) DO UPDATE SET
  wechat_product_id = EXCLUDED.wechat_product_id,
  mode = EXCLUDED.mode,
  enabled = EXCLUDED.enabled,
  updated_at = now();
