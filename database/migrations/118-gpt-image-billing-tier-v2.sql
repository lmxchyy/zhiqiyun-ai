-- Migration 118: GPT Image 2K/4K billing tier normalization and quote fix (Issue #124)
-- Scope: Normalize GPT Image billing size multipliers to tier_1k=1.0, tier_2k=1.5, tier_4k=2.0.
-- Eliminate legacy 1024x1536/1.2 fallback and ensure idempotent repeatable migration.

BEGIN;

-- 1. Archive legacy published version 1 for billing_rule_image_gpt
UPDATE xz_billing_rule_versions
SET status = 'ARCHIVED', effective_to = now(), updated_at = now()
WHERE rule_key = 'billing_rule_image_gpt' AND status = 'PUBLISHED' AND version < 2;

-- 2. Insert or update published version 2 with normalized tier-based size rules
INSERT INTO xz_billing_rule_versions(
  id, rule_key, legacy_rule_id, model_name, model_code, module_code, billing_unit,
  base_price_points, minimum_charge_points, parameter_rules, rule_source, version,
  status, effective_from, validation_result, published_at, created_at, updated_at
)
VALUES (
  'brv_billing_rule_image_gpt_v2',
  'billing_rule_image_gpt',
  'billing_rule_image_gpt',
  'GPT Image 2',
  'gpt-image-2',
  'image_generation',
  'PER_IMAGE',
  10,
  1,
  '{"quality":{"auto":1,"low":1,"normal":1.2,"medium":1.2,"high":1.5,"standard":1},"size":{"auto":1,"tier_720p":1,"tier_1k":1,"tier_2k":1.5,"tier_4k":2,"1024x1024":1,"1024x1536":1,"1536x1024":1,"1280x720":1,"720x1280":1,"2048x2048":1.5,"2048x1152":1.5,"3840x2160":2,"2160x3840":2}}',
  'CODE_DEFAULT',
  2,
  'PUBLISHED',
  now(),
  '{"valid":true,"issues":[]}',
  now(),
  now(),
  now()
)
ON CONFLICT (rule_key, version) DO UPDATE SET
  parameter_rules = EXCLUDED.parameter_rules,
  status = EXCLUDED.status,
  updated_at = now();

COMMIT;
