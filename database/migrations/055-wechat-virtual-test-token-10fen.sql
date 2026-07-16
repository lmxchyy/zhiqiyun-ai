-- Reconcile the existing WeChat custom Token product with its ten-cent
-- console price. The stable WeChat product ID is intentionally retained.

UPDATE xz_plans
SET price_cents = 10,
    grant_points = 10,
    token_amount = 10,
    token_rights_value_cents = 10,
    entitlements = COALESCE(entitlements, '{}'::jsonb) ||
      '{"customQuantity":true,"minQuantity":1,"maxQuantity":5000,"unitPriceCents":10,"unitTokenAmount":10}'::jsonb,
    raw = COALESCE(raw, '{}'::jsonb) ||
      '{"priceCents":10,"grantPoints":10,"tokenAmount":10,"customQuantity":true,"minQuantity":1,"maxQuantity":5000,"active":true}'::jsonb,
    active = TRUE
WHERE id = 'recharge_custom_unit_1yuan'
  AND payment_product_code = 'TOKEN_CUSTOM_1YUAN';

UPDATE xz_wechat_virtual_product_mappings
SET enabled = TRUE,
    updated_at = now()
WHERE plan_id = 'recharge_custom_unit_1yuan'
  AND wechat_product_id = 'TOKEN_CUSTOM_1YUAN'
  AND env IN (0, 1);

-- A short-lived alternate test product may exist on machines where an older
-- draft of this migration ran. Keep its audit data but make it unavailable.
UPDATE xz_plans
SET active = FALSE
WHERE id = 'recharge_test_10fen';

UPDATE xz_wechat_virtual_product_mappings
SET enabled = FALSE,
    updated_at = now()
WHERE plan_id = 'recharge_test_10fen';
