#!/usr/bin/env bash
set -euo pipefail
WK=zhiqiyun-ai-prod-smartvideo-worker-1
PG=zhiqiyun-ai-prod-postgres-1
BASE=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_BASE_URL:-${MODEL_PROVIDER_URL:-}}"')
KEY=$(docker exec "$WK" sh -c 'printf %s "${SMARTVIDEO_SPEECH_API_KEY:-${MODEL_PROVIDER_API_KEY:-}}"')

echo "--- LIST MODELS (filter tts/speech/audio) ---"
curl -sS "${BASE%/}/v1/models" -H "Authorization: Bearer $KEY" | python3 - <<'PY' || true
import sys,json,re
raw=sys.stdin.read()
try:
  data=json.loads(raw)
except Exception as e:
  print('parse_fail', e, raw[:300]); raise SystemExit
items=data.get('data') or data.get('models') or []
print('total', len(items))
keys=[]
for it in items:
  mid=it.get('id') or it.get('model') or ''
  keys.append(mid)
  if re.search(r'tts|speech|audio|voice|cosy|fish|openai-audio', mid, re.I):
    print('HIT', mid)
print('sample', keys[:30])
PY

echo "--- PROJECT PLAN VOICE ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT p.id, p.status, left(coalesce(v.plan_snapshot::text,''),200) FROM video_projects p LEFT JOIN video_project_versions v ON v.project_id=p.id AND v.id=(SELECT version_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7') WHERE p.id=(SELECT project_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7');"

docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT version_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"

VID=$(docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -t -A -c "SELECT version_id FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';")
echo "version=$VID"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT plan_snapshot->'voice' AS voice, jsonb_typeof(plan_snapshot->'scenes') AS scenes_type, coalesce(jsonb_array_length(plan_snapshot->'scenes'),0) AS scenes FROM video_project_versions WHERE id='$VID';" 2>/dev/null \
|| docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT table_name FROM information_schema.tables WHERE table_name ILIKE '%version%' OR table_name ILIKE '%plan%' ORDER BY 1;"

echo "DONE"
