#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== recent worker plan logs ==="
docker logs --since 30m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'plan|timeout|deadline|newapi|provider|chat|deepseek' | tail -50 || true

echo "=== recent api plan logs ==="
docker logs --since 30m zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -iE 'plan|smartvideo|provider_unavailable|deadline' | tail -30 || true

echo "=== env relevant ==="
grep -E '^(SMARTVIDEO_|MODEL_PROVIDER_|PPT_|OPENAI_|CHAT_)' .env.production | sed -E 's/(KEY|TOKEN|SECRET|PASSWORD|API_KEY)=.*/\1=***/'

echo "=== project/plan tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,error_stage,error_code,left(coalesce(error_message,''),200) AS err FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,left(coalesce(error_message,''),220) AS err,created_at,updated_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 5;"

echo "=== resolve newapi from host ==="
getent hosts newapi.zs-kjhn.cn || true
dig +short newapi.zs-kjhn.cn || true

echo "=== curl newapi from host (auth header) ==="
# extract key without printing full value
KEY=$(grep -E '^(MODEL_PROVIDER_API_KEY|PPT_PROVIDER_API_KEY|OPENAI_API_KEY)=' .env.production | head -1 | cut -d= -f2-)
curl -sS -o /tmp/newapi_probe.out -w "http=%{http_code} time=%{time_total}\n" \
  --connect-timeout 10 --max-time 60 \
  -X POST "https://newapi.zs-kjhn.cn/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"鍙洖澶峯k"}],"max_tokens":8}' || echo "CURL_FAIL=$?"
head -c 500 /tmp/newapi_probe.out; echo

echo "=== curl newapi from worker container ==="
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 sh -c \
  'wget -qO- --timeout=20 https://newapi.zs-kjhn.cn/v1/models 2>&1 | head -c 200 || echo WGET_FAIL'
echo
