#!/usr/bin/env bash
set -uo pipefail
echo "=== verify limits contain preview ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "SELECT COALESCE(elem->>'package_id', elem->>'packageId', '<default>') AS package_id, elem->'limit_json'->'models'->'allowed' AS allowed FROM xz_system_settings, jsonb_array_elements(raw->'tenantModuleLimits') elem WHERE id='ai_capability_config' AND COALESCE(elem->>'module_code', elem->>'moduleCode')='video_generation';"
echo "=== restart api to clear any memory ==="
cd /opt/zhiqiyun-ai
docker compose -f compose.prod.yml --env-file .env.production restart xianzhi-ai
sleep 5
curl -fsS http://127.0.0.1:3100/api/v1/health
echo
