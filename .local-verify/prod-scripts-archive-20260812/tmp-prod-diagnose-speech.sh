#!/usr/bin/env bash
set -euo pipefail
PG=zhiqiyun-ai-prod-postgres-1
WK=zhiqiyun-ai-prod-smartvideo-worker-1
echo "--- TASK ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,progress,attempt_count,error_code,error_message,updated_at FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- FULL SPEECH LOGS ---"
docker logs --since 5m "$WK" 2>&1 | grep -iE 'speech|tts|audio|SMARTVIDEO_SPEECH|newapi|openai|prepare|voice|caption|fail|error' | tail -80 || true
echo "--- TTS MODEL CONFIG ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, model_key, provider, enabled, left(coalesce(base_url,''),80) AS base_url FROM ai_models WHERE model_key ILIKE '%tts%' OR model_key ILIKE '%speech%' OR display_name ILIKE '%tts%' OR display_name ILIKE '%语音%' ORDER BY id LIMIT 20;" 2>/dev/null \
  || docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT table_name FROM information_schema.tables WHERE table_name ILIKE '%model%' OR table_name ILIKE '%channel%' ORDER BY 1;"
echo "DONE"
