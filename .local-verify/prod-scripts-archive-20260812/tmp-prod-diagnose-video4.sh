#!/usr/bin/env bash
set -uo pipefail
PSQL='docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off'
echo "=== api channels cols ==="
$PSQL -c "SELECT column_name FROM information_schema.columns WHERE table_name='xz_api_channels' ORDER BY ordinal_position;"
echo "=== newapi channel raw models ==="
$PSQL -c "SELECT id, name, base_url, status, raw->'models' AS models FROM xz_api_channels WHERE id ILIKE '%newapi%' OR raw::text ILIKE '%grok%' OR raw::text ILIKE '%seedance%';"
echo "=== ai_capability bound models / limits snippet ==="
$PSQL -c "SELECT jsonb_pretty(jsonb_path_query_array(raw, '$.aiModules[*] ? (@.moduleCode == \"video_generation\" || @.module_code == \"video_generation\")')) FROM xz_system_settings WHERE id='ai_capability_config';" | head -80
echo "=== model names containing grok/seedance ==="
$PSQL -c "SELECT jsonb_array_elements(COALESCE(raw->'aiModels','[]'::jsonb))->>'modelName' AS model_name FROM xz_system_settings WHERE id='ai_capability_config';" | grep -Ei 'grok|seedance|video' || true
$PSQL -c "SELECT jsonb_array_elements(COALESCE(raw->'aiModels','[]'::jsonb))->>'model_name' AS model_name FROM xz_system_settings WHERE id='ai_capability_config';" | grep -Ei 'grok|seedance|video' || true
