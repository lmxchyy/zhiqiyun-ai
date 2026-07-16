-- TOKEN_TEST_1FEN was published in the WeChat virtual-payment console on
-- 2026-07-16. Enable the matching production and sandbox mappings only after
-- that external publish step has completed.

UPDATE xz_plans
SET active = TRUE
WHERE id = 'recharge_test_1fen'
  AND payment_product_code = 'TOKEN_TEST_1FEN'
  AND price_cents = 1
  AND token_amount = 1;

UPDATE xz_wechat_virtual_product_mappings
SET enabled = TRUE,
    updated_at = now()
WHERE plan_id = 'recharge_test_1fen'
  AND wechat_product_id = 'TOKEN_TEST_1FEN'
  AND env IN (0, 1);
