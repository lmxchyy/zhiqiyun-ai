#!/bin/bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
docker exec "$PG" psql -U xianzhi -d xianzhi -c "
SELECT model_code, billing_unit, base_price_points, status, parameter_multipliers
FROM xz_billing_rule_versions
WHERE module_code ILIKE '%video%' OR model_code ILIKE '%grok%' OR model_code ILIKE '%seedance%'
ORDER BY model_code, created_at DESC
LIMIT 40;
" 2>/dev/null || docker exec "$PG" psql -U xianzhi -d xianzhi -c "\dt *billing*"

docker exec "$PG" psql -U xianzhi -d xianzhi -Atc "
SELECT jsonb_pretty(jsonb_path_query_array(state, '$.billingRules[*] ? (@.moduleCode == \"video_generation\" || @.module_code == \"video_generation\")'))
FROM platform_state LIMIT 1;
" 2>/dev/null | head -c 8000 || true

docker exec "$PG" psql -U xianzhi -d xianzhi -Atc "
SELECT elem->>'modelName' || '|' || coalesce(elem->>'status','') || '|' || coalesce(elem->>'moduleCode', elem->>'module_code','')
FROM platform_state, jsonb_array_elements(state->'aiModels') elem
WHERE (elem->>'modelType' = 'video' OR elem->>'model_type' = 'video'
   OR elem->>'moduleCode' ILIKE '%video%' OR elem->>'module_code' ILIKE '%video%')
ORDER BY 1;
" 2>/dev/null || true
