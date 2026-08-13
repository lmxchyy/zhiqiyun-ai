#!/bin/bash
set -euo pipefail
JS=/app/admin-vue/dist/assets/index-DFhJ-FhG.js
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 sh -c "
ls -l $JS
grep -o 'montageWork\|SMART_VIDEO_MONTAGE\|AI自动混剪\|svrender_' $JS | sort | uniq -c
echo '--- sample around montage ---'
grep -o '.{0,40}montageWork.{0,80}' $JS | head -5 || true
grep -o '.{0,40}SMART_VIDEO_MONTAGE.{0,80}' $JS | head -5 || true
"
# Also verify API returns asset for user - login as agent1 if we can get token from sessions
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id, user_id, left(token,20) as token_prefix, expires_at, created_at
from xz_sessions
where user_id='user_000003'
order by created_at desc nulls last
limit 5;
" 2>/dev/null || docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "\dt *session*"
