#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== plan tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,left(coalesce(error_message,''),220) AS err,created_at,finished_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 5;"

echo "=== DNS ==="
getent hosts newapi.zs-kjhn.cn || true

echo "=== curl models (quick) ==="
KEY=$(grep -E '^(MODEL_PROVIDER_API_KEY|PPT_MODEL_PROVIDER_API_KEY)=' .env.production | head -1 | cut -d= -f2-)
curl -sS -o /tmp/newapi_models.out -w "http=%{http_code} time=%{time_total}\n" \
  --connect-timeout 10 --max-time 30 \
  "https://newapi.zs-kjhn.cn/v1/models" \
  -H "Authorization: Bearer ${KEY}" || echo "CURL_FAIL=$?"
head -c 300 /tmp/newapi_models.out; echo

echo "=== curl chat short ==="
curl -sS -o /tmp/newapi_chat.out -w "http=%{http_code} time=%{time_total}\n" \
  --connect-timeout 10 --max-time 120 \
  -X POST "https://newapi.zs-kjhn.cn/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"鍙洖澶峯k"}],"max_tokens":8}' || echo "CURL_FAIL=$?"
head -c 500 /tmp/newapi_chat.out; echo

echo "=== curl chat with json_schema (plan-like) ==="
curl -sS -o /tmp/newapi_schema.out -w "http=%{http_code} time=%{time_total}\n" \
  --connect-timeout 10 --max-time 180 \
  -X POST "https://newapi.zs-kjhn.cn/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"system","content":"杩斿洖JSON"},{"role":"user","content":"鐢熸垚涓€涓彧鏈塼itle瀛楁鐨勫璞★紝title=娴嬭瘯"}],"max_tokens":256,"response_format":{"type":"json_object"}}' || echo "CURL_FAIL=$?"
head -c 500 /tmp/newapi_schema.out; echo

echo "=== SMARTVIDEO_PLAN_TIMEOUT env in containers ==="
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv SMARTVIDEO_PLAN_TIMEOUT || echo unset
docker exec zhiqiyun-ai-prod-smartvideo-worker-1 printenv MODEL_PROVIDER_TIMEOUT_MS || echo unset
