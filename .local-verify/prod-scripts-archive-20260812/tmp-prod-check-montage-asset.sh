#!/bin/bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id,user_id,task_id,name,media_type,left(url,80) as url,left(coalesce(thumbnail_url,''),80) as thumb,metadata->>'type' as meta_type,created_at
from xz_assets
where id='asset_fbc47867bd25964261f742fd' or task_id like 'svrender_%' or name like '%混剪%'
order by created_at desc limit 10;
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, username, email from xz_users where id='user_000003' limit 5;
"
# Check if admin bundle on disk has montageWork
grep -o 'montageWork\|SMART_VIDEO_MONTAGE\|AI自动混剪' /opt/zhiqiyun-ai/admin-vue/dist/assets/*.js 2>/dev/null | head -20 || true
ls -lt /opt/zhiqiyun-ai/admin-vue/dist/assets/index-*.js 2>/dev/null | head -5 || ls -lt /opt/zhiqiyun-ai/deploy/admin/assets/index-*.js 2>/dev/null | head -5 || find /opt/zhiqiyun-ai -name 'index-*.js' -path '*/assets/*' 2>/dev/null | head -10
