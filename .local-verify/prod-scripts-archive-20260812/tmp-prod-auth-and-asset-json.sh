#!/bin/bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "\d auth_sessions"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, user_id, left(token,24) as tok, expires_at, created_at
from auth_sessions
where user_id='user_000003'
order by created_at desc
limit 5;
"
# Check asset JSON shape as returned by a quick Go-less SQL simulation of metadata
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -t -A -c "
select json_build_object(
  'id', id,
  'userId', user_id,
  'tenantId', tenant_id,
  'taskId', task_id,
  'name', name,
  'mediaType', media_type,
  'url', url,
  'thumbnailUrl', thumbnail_url,
  'metadata', metadata,
  'createdAt', created_at
)::text
from xz_assets where id='asset_fbc47867bd25964261f742fd';
" | head -c 2000
echo
