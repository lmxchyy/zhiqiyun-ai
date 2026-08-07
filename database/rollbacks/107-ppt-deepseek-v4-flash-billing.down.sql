-- Roll back the PPT DeepSeek cutover only before any post-cutover usage exists.
BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '45s';

SELECT pg_advisory_xact_lock(hashtextextended('migration:107:ppt-deepseek-billing', 0));

-- Prevent new PPT tasks or billing evidence from appearing after the safety
-- checks but before the rollback commits.
LOCK TABLE public.xz_ppt_tasks, public.xz_generation_tasks, public.xz_billing_events,
  public.xz_billing_rule_versions, public.xz_provider_costs
  IN SHARE ROW EXCLUSIVE MODE;

DO $migration$
DECLARE
  canonical_channel_id CONSTANT text := 'channel_api_1785315792271635355';
  backup_record public.xz_migration_107_ppt_deepseek_billing_backup%ROWTYPE;
  current_capability jsonb;
  current_channel jsonb;
  invalid_count integer;
BEGIN
  SELECT * INTO backup_record
  FROM public.xz_migration_107_ppt_deepseek_billing_backup
  WHERE id = 'cutover'
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_DOWN_REQUIRES_EXACT_CUTOVER';
  END IF;

  SELECT raw INTO current_capability
  FROM public.xz_system_settings
  WHERE id = 'ai_capability_config'
  FOR UPDATE;
  SELECT raw INTO current_channel
  FROM public.xz_api_channels
  WHERE id = canonical_channel_id
  FOR UPDATE;

  PERFORM 1
  FROM public.xz_billing_rule_versions
  WHERE id IN (
    'brv_billing_rule_ppt_deepseek_v4_flash_v1',
    'brv_billing_rule_ppt_kimi_v1'
  )
  FOR UPDATE;
  PERFORM 1
  FROM public.xz_provider_costs
  WHERE id = 'pcost_newapi_deepseek_v4_flash_per_page_v1'
  FOR UPDATE;

  IF current_capability IS DISTINCT FROM backup_record.target_capability_raw
     OR current_channel IS DISTINCT FROM backup_record.target_channel_raw
     OR NOT EXISTS (
       SELECT 1 FROM public.xz_billing_rule_versions
       WHERE id = 'brv_billing_rule_ppt_deepseek_v4_flash_v1'
         AND rule_key = 'billing_rule_ppt_deepseek_v4_flash'
         AND legacy_rule_id = 'billing_rule_ppt_deepseek_v4_flash'
         AND model_name = 'DeepSeek V4 Flash'
         AND model_code = 'deepseek-v4-flash' AND module_code = 'ppt_generation'
         AND billing_unit = 'PER_PAGE' AND base_price_points = 3
         AND minimum_charge_points = 3
         AND parameter_rules = '{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}'::jsonb
         AND rule_source = 'DATABASE' AND version = 1 AND status = 'PUBLISHED'
         AND effective_from = backup_record.cutover_at
         AND effective_to IS NULL AND tenant_id IS NULL AND plan_id IS NULL
         AND validation_result = '{"valid":true,"issues":[]}'::jsonb
         AND published_at = backup_record.cutover_at
     )
     OR NOT EXISTS (
       SELECT 1 FROM public.xz_provider_costs
       WHERE id = 'pcost_newapi_deepseek_v4_flash_per_page_v1'
         AND provider = 'NEWAPI'
         AND platform_model_code = 'deepseek-v4-flash'
         AND upstream_model_name = 'deepseek-v4-flash-free'
         AND channel = canonical_channel_id AND billing_unit = 'PER_PAGE'
         AND parameter_range = '{}'::jsonb
         AND unit_cost = 0.02 AND currency = 'CNY' AND status = 'ACTIVE'
         AND effective_from = backup_record.cutover_at
         AND effective_to IS NULL
     )
     OR NOT EXISTS (
       SELECT 1 FROM public.xz_billing_rule_versions rule
       WHERE rule.id = 'brv_billing_rule_ppt_kimi_v1'
         AND (to_jsonb(rule) - ARRAY['status','effective_to','updated_at']::text[])
             = (backup_record.kimi_rule - ARRAY['status','effective_to','updated_at']::text[])
         AND rule_key = backup_record.kimi_rule->>'rule_key'
         AND legacy_rule_id = backup_record.kimi_rule->>'legacy_rule_id'
         AND model_name = backup_record.kimi_rule->>'model_name'
         AND model_code = backup_record.kimi_rule->>'model_code'
         AND module_code = backup_record.kimi_rule->>'module_code'
         AND billing_unit = backup_record.kimi_rule->>'billing_unit'
         AND base_price_points = (backup_record.kimi_rule->>'base_price_points')::numeric
         AND minimum_charge_points = (backup_record.kimi_rule->>'minimum_charge_points')::numeric
         AND parameter_rules = backup_record.kimi_rule->'parameter_rules'
         AND rule_source = backup_record.kimi_rule->>'rule_source'
         AND version = (backup_record.kimi_rule->>'version')::integer
         AND effective_from IS NOT DISTINCT FROM nullif(backup_record.kimi_rule->>'effective_from', '')::timestamptz
         AND validation_result = backup_record.kimi_rule->'validation_result'
         AND published_at IS NOT DISTINCT FROM nullif(backup_record.kimi_rule->>'published_at', '')::timestamptz
         AND status = 'ARCHIVED'
         AND effective_to = backup_record.cutover_at
         AND updated_at = backup_record.cutover_at
     )
     OR (SELECT count(*) FROM public.xz_billing_rule_versions
         WHERE model_code = 'deepseek-v4-flash'
            OR id = 'brv_billing_rule_ppt_deepseek_v4_flash_v1') <> 1
     OR (SELECT count(*) FROM public.xz_provider_costs
         WHERE platform_model_code = 'deepseek-v4-flash'
            OR id = 'pcost_newapi_deepseek_v4_flash_per_page_v1') <> 1
     OR (SELECT count(*) FROM public.xz_billing_rule_versions
         WHERE module_code = 'ppt_generation' AND status = 'PUBLISHED') <> 1
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_DOWN_REQUIRES_EXACT_CUTOVER';
  END IF;

  SELECT count(*) INTO invalid_count
  FROM public.xz_ppt_tasks
  WHERE upper(coalesce(stage, '')) IN ('DRAFT', 'OUTLINE_READY', 'GENERATING')
     OR upper(coalesce(status, '')) IN ('PENDING', 'PROCESSING', 'QUEUED', 'RUNNING');
  IF invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_DOWN_ACTIVE_PPT_TASKS';
  END IF;

  SELECT (
    (SELECT count(*) FROM public.xz_generation_tasks
     WHERE model = 'deepseek-v4-flash'
       AND created_at::timestamptz >= backup_record.cutover_at)
    +
    (SELECT count(*) FROM public.xz_billing_events
     WHERE model = 'deepseek-v4-flash' AND metric_code = 'ppt.generations'
       AND occurred_at::timestamptz >= backup_record.cutover_at)
  ) INTO invalid_count;
  IF invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_DOWN_POSTCUTOVER_USAGE';
  END IF;

  UPDATE public.xz_system_settings
  SET raw = backup_record.capability_raw,
      updated_at = backup_record.capability_updated_at
  WHERE id = 'ai_capability_config';
  UPDATE public.xz_api_channels
  SET raw = backup_record.channel_raw
  WHERE id = canonical_channel_id;

  UPDATE public.xz_billing_rule_versions
  SET status = backup_record.kimi_rule->>'status',
      effective_from = nullif(backup_record.kimi_rule->>'effective_from', '')::timestamptz,
      effective_to = nullif(backup_record.kimi_rule->>'effective_to', '')::timestamptz,
      validation_result = backup_record.kimi_rule->'validation_result',
      published_at = nullif(backup_record.kimi_rule->>'published_at', '')::timestamptz,
      updated_at = (backup_record.kimi_rule->>'updated_at')::timestamptz
  WHERE id = 'brv_billing_rule_ppt_kimi_v1';

  DELETE FROM public.xz_billing_rule_versions
  WHERE id = 'brv_billing_rule_ppt_deepseek_v4_flash_v1';
  DELETE FROM public.xz_provider_costs
  WHERE id = 'pcost_newapi_deepseek_v4_flash_per_page_v1';
  DELETE FROM public.xz_migration_107_ppt_deepseek_billing_backup WHERE id = 'cutover';
END
$migration$;

DROP TABLE public.xz_migration_107_ppt_deepseek_billing_backup;

COMMIT;
