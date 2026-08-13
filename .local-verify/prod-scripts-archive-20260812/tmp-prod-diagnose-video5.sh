#!/usr/bin/env bash
set -uo pipefail
PSQL='docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off'
echo "=== tenant module limits video ==="
$PSQL -c "SELECT jsonb_pretty(jsonb_path_query_array(raw, '$.tenantModuleLimits[*] ? (@.moduleCode == \"video_generation\" || @.module_code == \"video_generation\")')) FROM xz_system_settings WHERE id='ai_capability_config';"
echo "=== billing rules grok ==="
$PSQL -c "SELECT jsonb_array_elements(COALESCE(raw->'billingRules', raw->'BillingRules', '[]'::jsonb)) FROM xz_system_settings WHERE id='ai_capability_config';" | grep -i grok || true
echo "=== live API module-schema for preview (no auth expect 401) ==="
curl -sS -o /tmp/ms.json -w "%{http_code}" "http://127.0.0.1:3100/api/v1/ai/module-schema?module_code=video_generation&model_name=grok-imagine-video-1.5-preview" || true
echo
head -c 300 /tmp/ms.json; echo
echo "=== access logs around 09:48 local = 01:48 UTC ==="
docker logs --since 2026-08-11T01:40:00Z --until 2026-08-11T02:00:00Z zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | tail -120 || true
