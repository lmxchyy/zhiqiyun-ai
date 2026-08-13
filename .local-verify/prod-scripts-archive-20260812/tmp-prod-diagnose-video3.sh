#!/usr/bin/env bash
set -uo pipefail
PSQL='docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off'
echo "=== task counts ==="
$PSQL -c "SELECT count(*) AS total, count(*) FILTER (WHERE created_at::timestamptz > now() - interval '1 day') AS last_day FROM xz_generation_tasks;"
echo "=== recent video-ish tasks ==="
$PSQL -c "SELECT id, user_id, type, model, status, terminal, left(coalesce(error::text,''),160) AS err, left(prompt,40) AS prompt, created_at FROM xz_generation_tasks WHERE created_at > to_char(now() - interval '12 hours', 'YYYY-MM-DD\"T\"HH24:MI:SS') OR created_at::timestamptz > now() - interval '12 hours' ORDER BY created_at DESC LIMIT 25;"
echo "=== fallback recent any ==="
$PSQL -c "SELECT id, model, status, type, terminal, left(coalesce(error::text,''),160) AS err, created_at FROM xz_generation_tasks ORDER BY created_at DESC LIMIT 15;"
echo "=== system settings preview presence ==="
$PSQL -c "SELECT id, updated_at, (raw::text LIKE '%grok-imagine-video-1.5-preview%') AS has_preview, (raw::text LIKE '%grok-imagine-1.5-video%') AS has_grok15 FROM xz_system_settings;"
echo "=== newapi channel models ==="
$PSQL -c "SELECT id, name, base_url, models::text, status FROM xz_api_channels WHERE id ILIKE '%newapi%' OR models::text ILIKE '%grok%' OR models::text ILIKE '%seedance%';"
echo "=== docker logs last 200 matching ==="
docker logs --tail 2000 zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -iE 'error|fail|403|preview|grok|seedance|not allowed|forbidden|permission' | tail -80 || true
