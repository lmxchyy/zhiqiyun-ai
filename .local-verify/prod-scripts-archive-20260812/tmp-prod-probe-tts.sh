#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
WK=zhiqiyun-ai-prod-smartvideo-worker-1

echo "--- PLAN VOICE ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -t -A -c \
  "SELECT left(specification::text, 2500) FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';" | head -c 3000
echo
echo "--- VOICE JSON ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT specification->'voice' AS voice, jsonb_array_length(coalesce(specification->'scenes','[]'::jsonb)) AS scenes FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"

echo "--- WORKER SPEECH ENV ---"
docker exec "$WK" sh -c 'env | grep -iE "SPEECH|MODEL_PROVIDER|TTS|OPENAI" | sed "s/\(KEY=\).*/\1***/" | sort'

echo "--- PROBE TTS ---"
# load API key from container env without printing it
BASE=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_BASE_URL:-${MODEL_PROVIDER_URL:-}}"')
KEY=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_API_KEY:-${MODEL_PROVIDER_API_KEY:-}}"')
MODEL=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_MODEL:-tts-1}"')
echo "base=$BASE model=$MODEL"
if [ -n "$BASE" ] && [ -n "$KEY" ]; then
  CODE=$(curl -sS -o /tmp/tts_probe_body -w '%{http_code}' \
    -X POST "${BASE%/}/v1/audio/speech" \
    -H "Authorization: Bearer $KEY" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"tts-1\",\"input\":\"开业促销短视频配音测试\",\"voice\":\"alloy\",\"speed\":1,\"response_format\":\"mp3\"}" || echo curl_fail)
  echo "http=$CODE bytes=$(wc -c </tmp/tts_probe_body)"
  head -c 200 /tmp/tts_probe_body; echo
  CODE2=$(curl -sS -o /tmp/tts_probe_body2 -w '%{http_code}' \
    -X POST "${BASE%/}/v1/audio/speech" \
    -H "Authorization: Bearer $KEY" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"smart-video-speech\",\"input\":\"开业促销短视频配音测试\",\"voice\":\"alloy\",\"speed\":1,\"response_format\":\"mp3\"}" || echo curl_fail)
  echo "http_alias=$CODE2 bytes=$(wc -c </tmp/tts_probe_body2)"
  head -c 200 /tmp/tts_probe_body2; echo
fi
echo "DONE"
