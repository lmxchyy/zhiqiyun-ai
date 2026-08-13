#!/bin/bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, name, email, mobile, role, status from xz_users where id='user_000003';
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, user_id, tenant_id, organization_id, media_type, name, deleted_at, created_at
from xz_assets where id='asset_fbc47867bd25964261f742fd';
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select count(*) as asset_count from xz_assets where user_id='user_000003' and deleted_at is null;
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, tenant_id, media_type, left(name,40) as name, created_at
from xz_assets
where user_id='user_000003' and deleted_at is null
order by created_at desc nulls last
limit 12;
"
# Check deployed frontend for montageWork
WEBROOT=$(grep -E 'ADMIN_DIST|WEB_ROOT|nginx' /opt/zhiqiyun-ai/deploy.sh 2>/dev/null | head -5 || true)
echo "---"
find /var/www /opt/zhiqiyun-ai /usr/share/nginx -name 'index-*.js' 2>/dev/null | head -20
