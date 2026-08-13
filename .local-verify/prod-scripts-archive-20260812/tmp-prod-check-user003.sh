#!/bin/bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "\d xz_users" | head -40
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, phone, email, display_name, created_at from xz_users where id='user_000003';
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select count(*) as asset_count,
       sum(case when id='asset_fbc47867bd25964261f742fd' then 1 else 0 end) as has_target,
       max(case when id='asset_fbc47867bd25964261f742fd' then rn end) as target_rank
from (
  select id, row_number() over (order by created_at desc) as rn
  from xz_assets
  where user_id='user_000003' and deleted_at is null
) t;
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, media_type, name, created_at
from xz_assets
where user_id='user_000003' and deleted_at is null
order by created_at desc
limit 15;
"
# locate current admin js
find /opt/zhiqiyun-ai -name 'index-*.js' \( -path '*/admin*' -o -path '*/dist*' \) 2>/dev/null | head -20
