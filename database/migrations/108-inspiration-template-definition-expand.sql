-- AI Inspiration Template Schema expand/backfill.
-- This migration is intentionally additive: legacy dynamic columns remain available
-- for migration verification but definition_json becomes the canonical target shape.

BEGIN;

ALTER TABLE inspiration_templates
  ADD COLUMN IF NOT EXISTS slug varchar(160),
  ADD COLUMN IF NOT EXISTS definition_json jsonb,
  ADD COLUMN IF NOT EXISTS published_at timestamptz;

UPDATE inspiration_templates
SET slug = id
WHERE slug IS NULL OR btrim(slug) = '';

UPDATE inspiration_templates
SET published_at = coalesce(updated_at, created_at, now())
WHERE status = 'PUBLISHED' AND published_at IS NULL;

UPDATE inspiration_templates
SET definition_json = jsonb_strip_nulls(jsonb_build_object(
  'schemaVersion', 1,
  'inputs', CASE
    WHEN input_requirements_json ?| ARRAY['referenceImageRequired', 'referenceImageMin', 'referenceImageMax'] THEN
      jsonb_build_array(jsonb_build_object(
        'key', 'referenceImages',
        'type', 'IMAGE',
        'label', 'Reference images',
        'required', coalesce((input_requirements_json->>'referenceImageRequired')::boolean, false),
        'helpText', 'Upload reference images required by this template',
        'validation', jsonb_strip_nulls(jsonb_build_object(
          'minItems', CASE
            WHEN input_requirements_json ? 'referenceImageMin' THEN (input_requirements_json->>'referenceImageMin')::integer
            WHEN coalesce((input_requirements_json->>'referenceImageRequired')::boolean, false) THEN 1
            ELSE NULL
          END,
          'maxItems', CASE
            WHEN input_requirements_json ? 'referenceImageMax' THEN (input_requirements_json->>'referenceImageMax')::integer
            WHEN input_requirements_json ? 'referenceImageMin' THEN (input_requirements_json->>'referenceImageMin')::integer
            WHEN coalesce((input_requirements_json->>'referenceImageRequired')::boolean, false) THEN 1
            ELSE NULL
          END,
          'accept', jsonb_build_array('image/*')
        ))
      ))
    ELSE '[]'::jsonb
  END,
  'prompt', jsonb_build_object(
    'template', prompt,
    'negativeTemplate', negative_prompt,
    'composer', jsonb_build_object('key', 'deterministic-template', 'version', 1)
  ),
  'bindings', '[]'::jsonb,
  'presets', jsonb_build_object(
    'inputDefaults', preset_config_json,
    'generationDefaults', parameters_json,
    'materials', reference_assets_json
  ),
  'presentation', display_config_json,
  'handoff', jsonb_strip_nulls(jsonb_build_object(
    'targetType', upper(content_type) || '_CREATION',
    'targetKey', CASE content_type
      WHEN 'image' THEN 'image.create'
      WHEN 'video' THEN 'video.create'
      WHEN 'ppt' THEN 'ppt.create'
      WHEN 'text' THEN 'text.create'
      WHEN 'agent' THEN 'agent.create'
      WHEN 'workflow' THEN 'workflow.create'
    END,
    'intentKey', nullif(scenario_code, '')
  )),
  'capability', jsonb_strip_nulls(jsonb_build_object(
    'capabilityKey', CASE content_type
      WHEN 'image' THEN 'image_generation'
      WHEN 'video' THEN 'video_generation'
      WHEN 'ppt' THEN 'ppt_generation'
      WHEN 'text' THEN 'text_generation'
      WHEN 'agent' THEN 'agent_execution'
      WHEN 'workflow' THEN 'workflow_execution'
    END,
    'modelHint', nullif(model_id, '')
  ))
))
WHERE definition_json IS NULL;

UPDATE inspiration_template_versions
SET snapshot_json = snapshot_json || jsonb_build_object(
  'definition', jsonb_strip_nulls(jsonb_build_object(
    'schemaVersion', 1,
    'inputs', CASE
      WHEN coalesce(snapshot_json->'inputRequirements', '{}'::jsonb) ?| ARRAY['referenceImageRequired', 'referenceImageMin', 'referenceImageMax'] THEN
        jsonb_build_array(jsonb_build_object(
          'key', 'referenceImages',
          'type', 'IMAGE',
          'label', 'Reference images',
          'required', coalesce((snapshot_json->'inputRequirements'->>'referenceImageRequired')::boolean, false),
          'helpText', 'Upload reference images required by this template',
          'validation', jsonb_strip_nulls(jsonb_build_object(
            'minItems', CASE
              WHEN coalesce(snapshot_json->'inputRequirements', '{}'::jsonb) ? 'referenceImageMin'
                THEN (snapshot_json->'inputRequirements'->>'referenceImageMin')::integer
              WHEN coalesce((snapshot_json->'inputRequirements'->>'referenceImageRequired')::boolean, false) THEN 1
              ELSE NULL
            END,
            'maxItems', CASE
              WHEN coalesce(snapshot_json->'inputRequirements', '{}'::jsonb) ? 'referenceImageMax'
                THEN (snapshot_json->'inputRequirements'->>'referenceImageMax')::integer
              WHEN coalesce(snapshot_json->'inputRequirements', '{}'::jsonb) ? 'referenceImageMin'
                THEN (snapshot_json->'inputRequirements'->>'referenceImageMin')::integer
              WHEN coalesce((snapshot_json->'inputRequirements'->>'referenceImageRequired')::boolean, false) THEN 1
              ELSE NULL
            END,
            'accept', jsonb_build_array('image/*')
          ))
        ))
      ELSE '[]'::jsonb
    END,
    'prompt', jsonb_build_object(
      'template', coalesce(snapshot_json->>'prompt', ''),
      'negativeTemplate', coalesce(snapshot_json->>'negativePrompt', ''),
      'composer', jsonb_build_object('key', 'deterministic-template', 'version', 1)
    ),
    'bindings', '[]'::jsonb,
    'presets', jsonb_build_object(
      'inputDefaults', coalesce(snapshot_json->'presetConfig', '{}'::jsonb),
      'generationDefaults', coalesce(snapshot_json->'parameters', '{}'::jsonb),
      'materials', coalesce(snapshot_json->'referenceAssets', '[]'::jsonb)
    ),
    'presentation', coalesce(snapshot_json->'displayConfig', '{}'::jsonb),
    'handoff', jsonb_strip_nulls(jsonb_build_object(
      'targetType', upper(coalesce(snapshot_json->>'contentType', '')) || '_CREATION',
      'targetKey', CASE snapshot_json->>'contentType'
        WHEN 'image' THEN 'image.create'
        WHEN 'video' THEN 'video.create'
        WHEN 'ppt' THEN 'ppt.create'
        WHEN 'text' THEN 'text.create'
        WHEN 'agent' THEN 'agent.create'
        WHEN 'workflow' THEN 'workflow.create'
      END,
      'intentKey', nullif(coalesce(snapshot_json->>'scenarioCode', ''), '')
    )),
    'capability', jsonb_strip_nulls(jsonb_build_object(
      'capabilityKey', CASE snapshot_json->>'contentType'
        WHEN 'image' THEN 'image_generation'
        WHEN 'video' THEN 'video_generation'
        WHEN 'ppt' THEN 'ppt_generation'
        WHEN 'text' THEN 'text_generation'
        WHEN 'agent' THEN 'agent_execution'
        WHEN 'workflow' THEN 'workflow_execution'
      END,
      'modelHint', nullif(coalesce(snapshot_json->>'modelId', ''), '')
    ))
  ))
)
WHERE NOT (snapshot_json ? 'definition');

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM inspiration_templates
    WHERE slug IS NULL OR btrim(slug) = ''
  ) THEN
    RAISE EXCEPTION 'migration 108 failed: template slug backfill is incomplete';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM inspiration_templates
    WHERE definition_json IS NULL
       OR jsonb_typeof(definition_json) <> 'object'
       OR definition_json->>'schemaVersion' <> '1'
       OR jsonb_typeof(definition_json->'inputs') <> 'array'
       OR jsonb_typeof(definition_json->'prompt') <> 'object'
       OR jsonb_typeof(definition_json->'bindings') <> 'array'
       OR jsonb_typeof(definition_json->'presets') <> 'object'
       OR jsonb_typeof(definition_json->'presentation') <> 'object'
       OR jsonb_typeof(definition_json->'handoff') <> 'object'
       OR jsonb_typeof(definition_json->'capability') <> 'object'
  ) THEN
    RAISE EXCEPTION 'migration 108 failed: template definition_json backfill is invalid';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM inspiration_template_versions
    WHERE NOT (snapshot_json ? 'definition')
       OR jsonb_typeof(snapshot_json->'definition') <> 'object'
       OR snapshot_json->'definition'->>'schemaVersion' <> '1'
  ) THEN
    RAISE EXCEPTION 'migration 108 failed: version definition backfill is invalid';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_inspiration_templates_tenant_slug
  ON inspiration_templates (tenant_id, slug)
  WHERE deleted_at IS NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'inspiration_templates_content_type_check'
      AND conrelid = 'inspiration_templates'::regclass
      AND pg_get_constraintdef(oid) LIKE '%workflow%'
  ) THEN
    ALTER TABLE inspiration_templates
      DROP CONSTRAINT IF EXISTS inspiration_templates_content_type_check;
    ALTER TABLE inspiration_templates
      ADD CONSTRAINT inspiration_templates_content_type_check
      CHECK (content_type IN ('image', 'video', 'ppt', 'text', 'agent', 'workflow')) NOT VALID;
  END IF;
END $$;

ALTER TABLE inspiration_templates
  VALIDATE CONSTRAINT inspiration_templates_content_type_check;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'inspiration_templates_definition_json_check'
      AND conrelid = 'inspiration_templates'::regclass
  ) THEN
    ALTER TABLE inspiration_templates
      ADD CONSTRAINT inspiration_templates_definition_json_check
      CHECK (
        definition_json IS NULL OR (
          jsonb_typeof(definition_json) = 'object'
          AND definition_json->>'schemaVersion' = '1'
        )
      ) NOT VALID;
  END IF;
END $$;

ALTER TABLE inspiration_templates
  VALIDATE CONSTRAINT inspiration_templates_definition_json_check;

COMMIT;
