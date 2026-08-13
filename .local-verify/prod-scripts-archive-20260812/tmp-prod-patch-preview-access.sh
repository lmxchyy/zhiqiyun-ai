#!/usr/bin/env bash
set -euo pipefail
PSQL='docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -P pager=off'
echo "=== backup ai capability ==="
$PSQL -c "CREATE TABLE IF NOT EXISTS xz_ops_config_backups (id text PRIMARY KEY, source_table text NOT NULL, source_id text NOT NULL, raw jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());"
$PSQL -c "INSERT INTO xz_ops_config_backups(id, source_table, source_id, raw)
SELECT 'ai_capability_config_' || to_char(now(), 'YYYYMMDDHH24MISS'), 'xz_system_settings', id, raw
FROM xz_system_settings WHERE id='ai_capability_config';"
$PSQL -c "INSERT INTO xz_ops_config_backups(id, source_table, source_id, raw)
SELECT 'api_channel_' || id || '_' || to_char(now(), 'YYYYMMDDHH24MISS'), 'xz_api_channels', id, raw
FROM xz_api_channels WHERE id IN ('channel_newapi_grok_imagine','channel_newapi_gateway') OR raw::text ILIKE '%newapi%';"

echo "=== patch video limits allowed models ==="
$PSQL <<'SQL'
UPDATE xz_system_settings
SET raw = jsonb_set(
  raw,
  '{tenantModuleLimits}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN COALESCE(elem->>'module_code', elem->>'moduleCode') = 'video_generation'
          AND COALESCE(elem->'limit_json'->'models'->'allowed', elem->'limitJson'->'models'->'allowed') IS NOT NULL
        THEN
          CASE
            WHEN elem ? 'limit_json' THEN jsonb_set(
              elem,
              '{limit_json,models,allowed}',
              (
                SELECT jsonb_agg(to_jsonb(x) ORDER BY x)
                FROM (
                  SELECT DISTINCT trim(both '"' from value::text) AS x
                  FROM jsonb_array_elements(COALESCE(elem->'limit_json'->'models'->'allowed','[]'::jsonb))
                  UNION
                  SELECT unnest(ARRAY['grok-imagine-video-1.5-preview','grok-imagine-1.5-video'])
                ) s
                WHERE x <> ''
              )
            )
            ELSE jsonb_set(
              elem,
              '{limitJson,models,allowed}',
              (
                SELECT jsonb_agg(to_jsonb(x) ORDER BY x)
                FROM (
                  SELECT DISTINCT trim(both '"' from value::text) AS x
                  FROM jsonb_array_elements(COALESCE(elem->'limitJson'->'models'->'allowed','[]'::jsonb))
                  UNION
                  SELECT unnest(ARRAY['grok-imagine-video-1.5-preview','grok-imagine-1.5-video'])
                ) s
                WHERE x <> ''
              )
            )
          END
        ELSE elem
      END
    )
    FROM jsonb_array_elements(COALESCE(raw->'tenantModuleLimits','[]'::jsonb)) elem
  ),
  true
),
updated_at = now()
WHERE id='ai_capability_config';
SQL

echo "=== patch bound models ==="
$PSQL <<'SQL'
UPDATE xz_system_settings
SET raw = jsonb_set(
  raw,
  '{aiModules}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN COALESCE(elem->>'module_code', elem->>'moduleCode') = 'video_generation' THEN
          CASE
            WHEN elem ? 'bound_models' THEN jsonb_set(
              elem, '{bound_models}',
              (
                SELECT jsonb_agg(to_jsonb(x) ORDER BY x)
                FROM (
                  SELECT DISTINCT trim(both '"' from value::text) AS x
                  FROM jsonb_array_elements(COALESCE(elem->'bound_models','[]'::jsonb))
                  UNION SELECT unnest(ARRAY['grok-imagine-video-1.5-preview','grok-imagine-1.5-video'])
                ) s WHERE x <> ''
              )
            )
            ELSE jsonb_set(
              elem, '{boundModels}',
              (
                SELECT jsonb_agg(to_jsonb(x) ORDER BY x)
                FROM (
                  SELECT DISTINCT trim(both '"' from value::text) AS x
                  FROM jsonb_array_elements(COALESCE(elem->'boundModels','[]'::jsonb))
                  UNION SELECT unnest(ARRAY['grok-imagine-video-1.5-preview','grok-imagine-1.5-video'])
                ) s WHERE x <> ''
              )
            )
          END
        ELSE elem
      END
    )
    FROM jsonb_array_elements(COALESCE(raw->'aiModules','[]'::jsonb)) elem
  ),
  true
),
updated_at = now()
WHERE id='ai_capability_config';
SQL

echo "=== patch newapi channel models ==="
$PSQL <<'SQL'
UPDATE xz_api_channels
SET raw = jsonb_set(
  COALESCE(raw,'{}'::jsonb),
  '{models}',
  (
    SELECT jsonb_agg(to_jsonb(x) ORDER BY x)
    FROM (
      SELECT DISTINCT trim(both '"' from value::text) AS x
      FROM jsonb_array_elements(COALESCE(raw->'models','[]'::jsonb))
      UNION SELECT unnest(ARRAY['grok-imagine-video-1.5-preview','grok-imagine-1.5-video'])
    ) s WHERE x <> ''
  ),
  true
)
WHERE id='channel_newapi_grok_imagine'
   OR (base_url ILIKE '%newapi%' AND COALESCE(raw->>'protocol','') IN ('','openai'));
SQL

echo "=== verify ==="
$PSQL -c "SELECT (raw::text LIKE '%grok-imagine-video-1.5-preview%') AS has_preview FROM xz_system_settings WHERE id='ai_capability_config';"
$PSQL -c "SELECT id, raw->'models' AS models FROM xz_api_channels WHERE id='channel_newapi_grok_imagine' OR base_url ILIKE '%newapi%';"
$PSQL -c "SELECT jsonb_pretty(jsonb_path_query_array(raw, '$.tenantModuleLimits[*].limit_json.models.allowed')) FROM xz_system_settings WHERE id='ai_capability_config';"
echo "DONE"
