-- Integration-only virtual products must never be purchasable in production.
-- Keep their sandbox mappings untouched for future payment diagnostics.
UPDATE xz_wechat_virtual_product_mappings AS mapping
SET enabled = FALSE,
    updated_at = now()
FROM xz_plans AS plan
WHERE mapping.plan_id = plan.id
  AND mapping.env = 0
  AND lower(coalesce(plan.entitlements->>'testOnly', plan.raw->>'testOnly', 'false')) = 'true'
  AND mapping.enabled = TRUE;
