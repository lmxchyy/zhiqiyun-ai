#!/bin/bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "\dt *user*"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, email, name from xz_users where email='agent1@xianzhi.ai' or id='user_000003';
"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "\d users" 2>/dev/null | head -30
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id::text, email, created_at from users where email ilike '%agent1%' or email ilike '%xianzhi%' limit 20;
" 2>/dev/null || true
# Who owns the video project?
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, user_id, tenant_id, title, status from video_projects where id='vp_664248192f84dc96631df8cd';
"
# Check if API can list the asset with a one-off query mimicking ListAssetsForUser scope
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, name from xz_assets
where user_id='user_000003' and deleted_at is null
  and (tenant_id is null or tenant_id='tenant_default')
order by created_at desc nulls last
limit 5;
"
