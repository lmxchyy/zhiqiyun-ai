#!/usr/bin/env bash
set -uo pipefail
PSQL='docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off'
echo "=== recent video failures ==="
$PSQL -c "SELECT id, model, status, left(coalesce(error->>'message', error::text, ''),140) AS err, created_at FROM xz_generation_tasks WHERE type ILIKE '%VIDEO%' ORDER BY created_at DESC LIMIT 12;"
echo "=== seedance-fast channel presence ==="
$PSQL -c "SELECT id, name, status, raw->>'models' AS models, base_url FROM xz_api_channels WHERE raw::text ILIKE '%seedance%' OR id ILIKE '%seedance%' OR id ILIKE '%newapi%' OR base_url ILIKE '%newapi%';"
echo "=== model channel_id for seedance/grok ==="
$PSQL -c "SELECT jsonb_array_elements(raw->'aiModels')->>'model_name' AS model, jsonb_array_elements(raw->'aiModels')->>'channel_id' AS channel_id, jsonb_array_elements(raw->'aiModels')->>'status' AS status FROM xz_system_settings WHERE id='ai_capability_config';" | grep -Ei 'seedance|grok|video' || true
echo "=== published billing v1 seedance/grok ==="
$PSQL -c "\dt *billing*" | head -40
