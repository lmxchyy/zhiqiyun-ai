#!/usr/bin/env bash
set -euo pipefail
API=zhiqiyun-ai-prod-xianzhi-ai-1
PG=zhiqiyun-ai-prod-postgres-1

echo "--- index.html script refs ---"
docker exec "$API" sh -c 'head -c 2000 /app/admin-vue/dist/index.html; echo; grep -oE "assets/[^\"]+\.js" /app/admin-vue/dist/index.html | head -20'

echo "--- public site index ---"
curl -sS -D - https://ai.zs-kjhn.cn/app/works -o /tmp/works.html | head -40
grep -oE 'assets/[^"]+\.js' /tmp/works.html | head -20 || true
grep -oE 'index-[^"]+\.js' /tmp/works.html | head -10 || true

echo "--- check served JS for montage ---"
JS=$(grep -oE '/assets/index-[^"]+\.js' /tmp/works.html | head -1 || true)
if [ -n "$JS" ]; then
  echo "js=$JS"
  curl -sS "https://ai.zs-kjhn.cn$JS" -o /tmp/app.js
  grep -o 'montageWork\|SMART_VIDEO_MONTAGE\|AI自动混剪成片' /tmp/app.js | sort | uniq -c || echo "NO_MARKERS_IN_SERVED_JS"
  wc -c /tmp/app.js
fi

echo "--- users near render ---"
docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, username, display_name, phone, email FROM xz_users WHERE id IN ('user_000003') OR username ILIKE '%mosi%' OR phone IS NOT NULL ORDER BY id LIMIT 20;" 2>/dev/null \
|| docker exec "$PG" psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT table_name FROM information_schema.tables WHERE table_name ILIKE '%user%' ORDER BY 1 LIMIT 30;"

echo "DONE"
