-- PPT DeepSeek V4 Flash capability, price and provider-cost cutover.
BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '45s';

SELECT pg_advisory_xact_lock(hashtextextended('migration:107:ppt-deepseek-billing', 0));

-- Close the business-write race between the empty-active-task check and commit.
LOCK TABLE public.xz_ppt_tasks, public.xz_generation_tasks, public.xz_billing_events,
  public.xz_billing_rule_versions, public.xz_provider_costs
  IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE IF NOT EXISTS public.xz_migration_107_ppt_deepseek_billing_backup (
  id text PRIMARY KEY CHECK (id = 'cutover'),
  cutover_at timestamptz NOT NULL,
  capability_raw jsonb NOT NULL,
  capability_updated_at text,
  channel_raw jsonb NOT NULL,
  kimi_rule jsonb NOT NULL,
  target_capability_raw jsonb NOT NULL,
  target_channel_raw jsonb NOT NULL
);

DO $migration$
DECLARE
  canonical_channel_id CONSTANT text := 'channel_api_1785315792271635355';
  deepseek_model CONSTANT text := 'deepseek-v4-flash';
  capability_raw jsonb;
  channel_raw jsonb;
  target_capability_raw jsonb;
  target_channel_raw jsonb;
  target_modules jsonb;
  target_models jsonb;
  target_schemas jsonb;
  target_limits jsonb;
  target_rules jsonb;
  kimi_rule jsonb;
  backup_target_capability jsonb;
  backup_target_channel jsonb;
  backup_kimi_rule jsonb;
  capability_updated_at text;
  channel_status text;
  channel_protocol text;
  channel_base_url text;
  channel_raw_base_url text;
  cutover_at timestamptz := clock_timestamp();
  item_count integer;
  invalid_count integer;
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.xz_migration_107_ppt_deepseek_billing_backup WHERE id = 'cutover'
  ) THEN
    SELECT backup.target_capability_raw, backup.target_channel_raw, backup.kimi_rule, backup.cutover_at
      INTO backup_target_capability, backup_target_channel, backup_kimi_rule, cutover_at
    FROM public.xz_migration_107_ppt_deepseek_billing_backup backup
    WHERE backup.id = 'cutover'
    FOR UPDATE;

    SELECT raw INTO capability_raw
    FROM public.xz_system_settings
    WHERE id = 'ai_capability_config'
    FOR UPDATE;
    SELECT raw INTO channel_raw
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

    IF capability_raw IS DISTINCT FROM backup_target_capability
       OR channel_raw IS DISTINCT FROM backup_target_channel
       OR NOT EXISTS (
         SELECT 1 FROM public.xz_billing_rule_versions
         WHERE id = 'brv_billing_rule_ppt_deepseek_v4_flash_v1'
           AND rule_key = 'billing_rule_ppt_deepseek_v4_flash'
           AND legacy_rule_id = 'billing_rule_ppt_deepseek_v4_flash'
           AND model_name = 'DeepSeek V4 Flash'
           AND model_code = deepseek_model AND module_code = 'ppt_generation'
           AND billing_unit = 'PER_PAGE' AND base_price_points = 3
           AND minimum_charge_points = 3
           AND parameter_rules = '{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}'::jsonb
           AND rule_source = 'DATABASE' AND version = 1 AND status = 'PUBLISHED'
           AND effective_from = cutover_at AND published_at = cutover_at
           AND effective_to IS NULL AND tenant_id IS NULL AND plan_id IS NULL
           AND validation_result = '{"valid":true,"issues":[]}'::jsonb
       )
       OR NOT EXISTS (
         SELECT 1 FROM public.xz_provider_costs
         WHERE id = 'pcost_newapi_deepseek_v4_flash_per_page_v1'
           AND provider = 'NEWAPI' AND platform_model_code = deepseek_model
           AND upstream_model_name = 'deepseek-v4-flash-free'
           AND channel = canonical_channel_id AND billing_unit = 'PER_PAGE'
           AND parameter_range = '{}'::jsonb AND unit_cost = 0.02
           AND currency = 'CNY' AND status = 'ACTIVE' AND effective_from = cutover_at
           AND effective_to IS NULL
       )
       OR NOT EXISTS (
         SELECT 1 FROM public.xz_billing_rule_versions rule
         WHERE rule.id = 'brv_billing_rule_ppt_kimi_v1'
           AND (to_jsonb(rule) - ARRAY['status','effective_to','updated_at']::text[])
               = (backup_kimi_rule - ARRAY['status','effective_to','updated_at']::text[])
           AND rule_key = backup_kimi_rule->>'rule_key'
           AND legacy_rule_id = backup_kimi_rule->>'legacy_rule_id'
           AND model_name = backup_kimi_rule->>'model_name'
           AND model_code = backup_kimi_rule->>'model_code'
           AND module_code = backup_kimi_rule->>'module_code'
           AND billing_unit = backup_kimi_rule->>'billing_unit'
           AND base_price_points = (backup_kimi_rule->>'base_price_points')::numeric
           AND minimum_charge_points = (backup_kimi_rule->>'minimum_charge_points')::numeric
           AND parameter_rules = backup_kimi_rule->'parameter_rules'
           AND rule_source = backup_kimi_rule->>'rule_source'
           AND version = (backup_kimi_rule->>'version')::integer
           AND effective_from IS NOT DISTINCT FROM nullif(backup_kimi_rule->>'effective_from', '')::timestamptz
           AND validation_result = backup_kimi_rule->'validation_result'
           AND published_at IS NOT DISTINCT FROM nullif(backup_kimi_rule->>'published_at', '')::timestamptz
           AND status = 'ARCHIVED' AND effective_to = cutover_at
           AND updated_at = cutover_at
       )
       OR (SELECT count(*) FROM public.xz_billing_rule_versions
           WHERE model_code = deepseek_model
              OR id = 'brv_billing_rule_ppt_deepseek_v4_flash_v1') <> 1
       OR (SELECT count(*) FROM public.xz_provider_costs
           WHERE platform_model_code = deepseek_model
              OR id = 'pcost_newapi_deepseek_v4_flash_per_page_v1') <> 1
       OR (SELECT count(*) FROM public.xz_billing_rule_versions
           WHERE module_code = 'ppt_generation' AND status = 'PUBLISHED') <> 1
    THEN
      RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_PARTIAL_OR_DRIFTED_STATE';
    END IF;
    RETURN;
  END IF;

  SELECT raw, updated_at INTO capability_raw, capability_updated_at
  FROM public.xz_system_settings
  WHERE id = 'ai_capability_config'
  FOR UPDATE;
  IF NOT FOUND OR jsonb_typeof(capability_raw) <> 'object' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_CAPABILITY_MISSING';
  END IF;
  IF jsonb_typeof(capability_raw->'aiModules') <> 'array'
     OR jsonb_typeof(capability_raw->'aiModels') <> 'array'
     OR jsonb_typeof(capability_raw->'aiParameterSchemas') <> 'array'
     OR jsonb_typeof(capability_raw->'tenantModuleLimits') <> 'array'
     OR jsonb_typeof(capability_raw->'billingRules') <> 'array' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_CAPABILITY_ARRAYS_INVALID';
  END IF;

  SELECT raw, status, protocol, base_url,
         coalesce(raw->>'baseUrl', raw->>'base_url', '')
    INTO channel_raw, channel_status, channel_protocol, channel_base_url, channel_raw_base_url
  FROM public.xz_api_channels
  WHERE id = canonical_channel_id
  FOR UPDATE;
  IF NOT FOUND OR upper(btrim(coalesce(channel_status, ''))) <> 'ACTIVE'
     OR lower(btrim(coalesce(channel_protocol, ''))) NOT IN ('openai', 'openai-compatible', 'openai_compatible')
     OR rtrim(lower(btrim(coalesce(channel_base_url, ''))), '/') <> 'https://newapi.zs-kjhn.cn'
     OR rtrim(lower(btrim(coalesce(channel_raw_base_url, ''))), '/') <> rtrim(lower(btrim(coalesce(channel_base_url, ''))), '/')
     OR jsonb_typeof(channel_raw->'models') <> 'array' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_CHANNEL_MISMATCH';
  END IF;

  SELECT count(*) INTO invalid_count
  FROM public.xz_ppt_tasks
  WHERE upper(coalesce(stage, '')) IN ('DRAFT', 'OUTLINE_READY', 'GENERATING')
     OR upper(coalesce(status, '')) IN ('PENDING', 'PROCESSING', 'QUEUED', 'RUNNING');
  IF invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_ACTIVE_PPT_TASKS';
  END IF;

  SELECT count(*) INTO item_count
  FROM jsonb_array_elements(capability_raw->'aiModules') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation';
  SELECT count(*) INTO invalid_count
  FROM jsonb_array_elements(capability_raw->'aiModules') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
    AND (
      jsonb_typeof(item->'bound_models') IS DISTINCT FROM 'array'
      OR jsonb_array_length(item->'bound_models') <> 3
      OR NOT (item->'bound_models' @> '["deepseek-v4-flash","kimi-k2.6","ppt-text-model"]'::jsonb)
      OR item->>'default_schema_id' IS DISTINCT FROM 'schema_ppt_kimi'
    );
  IF item_count <> 1 OR invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_MODULE';
  END IF;

  SELECT count(*) INTO invalid_count
  FROM jsonb_array_elements(capability_raw->'aiModels') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation';
  SELECT count(*) INTO item_count
  FROM jsonb_array_elements(capability_raw->'aiModels') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
    AND coalesce(item->>'model_name', item->>'modelName') = deepseek_model;
  IF invalid_count <> 3 OR item_count <> 1
     OR (SELECT count(*) FROM jsonb_array_elements(capability_raw->'aiModels') item
         WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
           AND coalesce(item->>'model_name', item->>'modelName') = 'kimi-k2.6') <> 1
     OR (SELECT count(*) FROM jsonb_array_elements(capability_raw->'aiModels') item
         WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
           AND coalesce(item->>'model_name', item->>'modelName') = 'ppt-text-model') <> 1 THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_MODELS';
  END IF;

  SELECT count(*) INTO item_count
  FROM jsonb_array_elements(capability_raw->'aiParameterSchemas') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation';
  IF item_count <> 2
     OR (SELECT count(*) FROM jsonb_array_elements(capability_raw->'aiParameterSchemas') item
         WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
           AND coalesce(item->>'model_name', item->>'modelName') = deepseek_model) <> 1
     OR (SELECT count(*) FROM jsonb_array_elements(capability_raw->'aiParameterSchemas') item
         WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
           AND coalesce(item->>'model_name', item->>'modelName') = 'kimi-k2.6') <> 1
     OR NOT EXISTS (
    SELECT 1
    FROM jsonb_array_elements(capability_raw->'aiParameterSchemas') schema_item
    WHERE coalesce(schema_item->>'module_code', schema_item->>'moduleCode') = 'ppt_generation'
      AND coalesce(schema_item->>'model_name', schema_item->>'modelName') = deepseek_model
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(schema_item->'schema_json'->'fields') field WHERE field->>'key' = 'topic')
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(schema_item->'schema_json'->'fields') field WHERE field->>'key' = 'page_count')
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_SCHEMAS';
  END IF;

  SELECT count(*) INTO item_count
  FROM jsonb_array_elements(capability_raw->'tenantModuleLimits') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation';
  SELECT count(*) INTO invalid_count
  FROM jsonb_array_elements(capability_raw->'tenantModuleLimits') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
    AND (
      jsonb_typeof(item->'limit_json'->'models'->'allowed') IS DISTINCT FROM 'array'
      OR jsonb_array_length(item->'limit_json'->'models'->'allowed') <> 3
      OR NOT (item->'limit_json'->'models'->'allowed' @> '["deepseek-v4-flash","kimi-k2.6","ppt-text-model"]'::jsonb)
    );
  IF item_count <> 5 OR invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_LIMITS';
  END IF;

  SELECT count(*) INTO item_count
  FROM jsonb_array_elements(capability_raw->'billingRules') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
    AND coalesce(item->>'model_name', item->>'modelName') = 'kimi-k2.6';
  SELECT count(*) INTO invalid_count
  FROM jsonb_array_elements(capability_raw->'billingRules') item
  WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation';
  IF item_count <> 1 OR invalid_count <> 1
     OR EXISTS (
       SELECT 1 FROM jsonb_array_elements(capability_raw->'billingRules') item
       WHERE coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
         AND (
           coalesce((item->>'base_price')::numeric, -1) <> 1
           OR coalesce((item->>'minimum_charge')::numeric, (item->>'base_price')::numeric, -1) <> 1
           OR coalesce(item->>'billing_type', '') <> 'per_page'
           OR upper(coalesce(item->>'status', '')) <> 'ACTIVE'
           OR item->'parameter_multiplier' IS DISTINCT FROM '{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}'::jsonb
         )
     ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_UNEXPECTED_RAW_RULES';
  END IF;

  IF EXISTS (SELECT 1 FROM jsonb_array_elements_text(channel_raw->'models') model WHERE model = deepseek_model)
     OR EXISTS (SELECT 1 FROM public.xz_billing_rule_versions WHERE model_code = deepseek_model OR id = 'brv_billing_rule_ppt_deepseek_v4_flash_v1')
     OR EXISTS (SELECT 1 FROM public.xz_provider_costs WHERE platform_model_code = deepseek_model OR id = 'pcost_newapi_deepseek_v4_flash_per_page_v1')
     OR EXISTS (
       SELECT 1 FROM public.xz_billing_rule_versions
       WHERE module_code = 'ppt_generation' AND status = 'PUBLISHED'
         AND id <> 'brv_billing_rule_ppt_kimi_v1'
     ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_CONFLICTING_TARGET_ROWS';
  END IF;

  SELECT to_jsonb(rule) INTO kimi_rule
  FROM public.xz_billing_rule_versions rule
  WHERE id = 'brv_billing_rule_ppt_kimi_v1'
    AND model_code = 'kimi-k2.6' AND module_code = 'ppt_generation'
    AND billing_unit = 'PER_PAGE' AND base_price_points = 1
    AND minimum_charge_points = 1
    AND parameter_rules = '{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}'::jsonb
    AND rule_source = 'CODE_DEFAULT' AND version = 1 AND status = 'PUBLISHED'
    AND tenant_id IS NULL AND plan_id IS NULL AND created_by IS NULL
    AND effective_from IS NOT NULL AND effective_to IS NULL
    AND validation_result = '{"valid":true,"issues":[]}'::jsonb
    AND created_at IS NOT NULL AND published_at IS NOT NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_KIMI_RULE_MISMATCH';
  END IF;

  SELECT jsonb_agg(
           CASE WHEN coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
                THEN jsonb_set(jsonb_set(item, '{bound_models}', '["deepseek-v4-flash"]'::jsonb, true), '{default_schema_id}', '"schema_ppt_generation_default"'::jsonb, true)
                ELSE item END ORDER BY ord
         ) INTO target_modules
  FROM jsonb_array_elements(capability_raw->'aiModules') WITH ORDINALITY source(item, ord);

  SELECT jsonb_agg(
           CASE WHEN coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
                THEN item || jsonb_build_object(
                  'id', 'ai_model_deepseek_v4_flash', 'model_name', deepseek_model,
                  'provider', 'NewAPI', 'channel_id', canonical_channel_id,
                  'fallback_model', '', 'allow_fallback_switch', false,
                  'status', 'ACTIVE'
                ) ELSE item END ORDER BY ord
         ) INTO target_models
  FROM jsonb_array_elements(capability_raw->'aiModels') WITH ORDINALITY source(item, ord)
  WHERE coalesce(item->>'module_code', item->>'moduleCode') <> 'ppt_generation'
     OR coalesce(item->>'model_name', item->>'modelName') = deepseek_model;

  SELECT jsonb_agg(
           CASE WHEN coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
                THEN item || jsonb_build_object('id', 'schema_ppt_generation_default', 'model_name', deepseek_model)
                ELSE item END ORDER BY ord
         ) INTO target_schemas
  FROM jsonb_array_elements(capability_raw->'aiParameterSchemas') WITH ORDINALITY source(item, ord)
  WHERE coalesce(item->>'module_code', item->>'moduleCode') <> 'ppt_generation'
     OR coalesce(item->>'model_name', item->>'modelName') = deepseek_model;

  SELECT jsonb_agg(
           CASE WHEN coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
                THEN jsonb_set(item, '{limit_json,models,allowed}', '["deepseek-v4-flash"]'::jsonb, true)
                ELSE item END ORDER BY ord
         ) INTO target_limits
  FROM jsonb_array_elements(capability_raw->'tenantModuleLimits') WITH ORDINALITY source(item, ord);

  SELECT jsonb_agg(
           CASE WHEN coalesce(item->>'module_code', item->>'moduleCode') = 'ppt_generation'
                THEN item || jsonb_build_object(
                  'id', 'billing_rule_ppt_deepseek_v4_flash',
                  'model_name', deepseek_model, 'model_code', deepseek_model,
                  'billing_type', 'per_page', 'billing_unit', 'PER_PAGE',
                  'base_price', 3, 'minimum_charge', 3, 'cost_price', 2,
                  'currency_type', 'credit', 'status', 'ACTIVE',
                  'parameter_multiplier', jsonb_build_object(
                    'with_images', jsonb_build_object('true', 1, 'false', 1),
                    'uploaded_file', jsonb_build_object('true', 1, 'false', 1)
                  )
                ) ELSE item END ORDER BY ord
         ) INTO target_rules
  FROM jsonb_array_elements(capability_raw->'billingRules') WITH ORDINALITY source(item, ord);

  target_capability_raw := jsonb_set(
    jsonb_set(
      jsonb_set(
        jsonb_set(
          jsonb_set(capability_raw, '{aiModules}', target_modules, false),
          '{aiModels}', target_models, false
        ), '{aiParameterSchemas}', target_schemas, false
      ), '{tenantModuleLimits}', target_limits, false
    ), '{billingRules}', target_rules, false
  );
  target_channel_raw := jsonb_set(channel_raw, '{models}', (channel_raw->'models') || to_jsonb(deepseek_model), false);

  INSERT INTO public.xz_migration_107_ppt_deepseek_billing_backup(
    id, cutover_at, capability_raw, capability_updated_at, channel_raw, kimi_rule,
    target_capability_raw, target_channel_raw
  ) VALUES (
    'cutover', cutover_at, capability_raw, capability_updated_at, channel_raw, kimi_rule,
    target_capability_raw, target_channel_raw
  );

  UPDATE public.xz_system_settings
  SET raw = target_capability_raw, updated_at = cutover_at::text
  WHERE id = 'ai_capability_config';
  UPDATE public.xz_api_channels SET raw = target_channel_raw WHERE id = canonical_channel_id;
  UPDATE public.xz_billing_rule_versions
  SET status = 'ARCHIVED', effective_to = cutover_at, updated_at = cutover_at
  WHERE id = 'brv_billing_rule_ppt_kimi_v1';

  INSERT INTO public.xz_billing_rule_versions(
    id, rule_key, legacy_rule_id, model_name, model_code, module_code, billing_unit,
    base_price_points, minimum_charge_points, parameter_rules, rule_source, version,
    status, effective_from, validation_result, published_at, created_at, updated_at
  ) VALUES (
    'brv_billing_rule_ppt_deepseek_v4_flash_v1',
    'billing_rule_ppt_deepseek_v4_flash', 'billing_rule_ppt_deepseek_v4_flash',
    'DeepSeek V4 Flash', deepseek_model, 'ppt_generation', 'PER_PAGE',
    3, 3, '{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}'::jsonb,
    'DATABASE', 1, 'PUBLISHED', cutover_at, '{"valid":true,"issues":[]}'::jsonb,
    cutover_at, cutover_at, cutover_at
  );

  INSERT INTO public.xz_provider_costs(
    id, provider, channel, platform_model_code, upstream_model_name, billing_unit,
    parameter_range, unit_cost, currency, effective_from, status, created_at, updated_at
  ) VALUES (
    'pcost_newapi_deepseek_v4_flash_per_page_v1', 'NEWAPI', canonical_channel_id,
    deepseek_model, 'deepseek-v4-flash-free', 'PER_PAGE', '{}'::jsonb,
    0.02, 'CNY', cutover_at, 'ACTIVE', cutover_at, cutover_at
  );

  IF (SELECT raw FROM public.xz_system_settings WHERE id = 'ai_capability_config') IS DISTINCT FROM target_capability_raw
     OR (SELECT raw FROM public.xz_api_channels WHERE id = canonical_channel_id) IS DISTINCT FROM target_channel_raw THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'PPT_DEEPSEEK_BILLING_FINAL_ASSERTION_FAILED';
  END IF;
END
$migration$;

COMMIT;
