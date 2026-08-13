#!/usr/bin/env bash
set -euo pipefail
API=zhiqiyun-ai-prod-xianzhi-ai-1
echo "--- find js with montage markers ---"
docker exec "$API" sh -c 'grep -R -l "montageWork\|SMART_VIDEO_MONTAGE\|AI自动混剪成片\|isMontageWork" /app/admin-vue/dist 2>/dev/null | head -20'
docker exec "$API" sh -c 'ls -la /app/admin-vue/dist/assets 2>/dev/null | head -30'
docker exec "$API" sh -c 'ls -la /app/user-h5 2>/dev/null | head -20'
# Also check nginx root / what compose mounts
docker inspect --format '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{"\n"}}{{end}}' "$API" | head -40
echo "--- compose service volumes/command ---"
cd /opt/zhiqiyun-ai
grep -A40 'xianzhi-ai:' compose.prod.yml | head -50
echo "DONE"
