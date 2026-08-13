#!/usr/bin/env bash
set -euo pipefail
WK=zhiqiyun-ai-prod-smartvideo-worker-1
BASE=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_BASE_URL:-${MODEL_PROVIDER_URL:-}}"')
KEY=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_API_KEY:-${MODEL_PROVIDER_API_KEY:-}}"')
echo "GET $BASE/v1/models"
curl -sS -w "\nHTTP=%{http_code}\n" "${BASE%/}/v1/models" -H "Authorization: Bearer $KEY" -o /tmp/models.json
python3 -c '
import json,re
data=json.load(open("/tmp/models.json"))
items=data.get("data") or []
print("total", len(items))
for it in items:
  mid=str(it.get("id") or "")
  if re.search(r"tts|speech|audio|voice|cosy|fish|sambert|gpt-4o-audio|openai-audio", mid, re.I):
    print("HIT", mid)
print("--- all ids sample ---")
for mid in sorted(str(it.get("id") or "") for it in items)[:80]:
  print(mid)
'
