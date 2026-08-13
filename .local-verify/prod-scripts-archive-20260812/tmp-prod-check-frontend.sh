#!/bin/bash
set -euo pipefail
# Find nginx root and check if montageWork is in served JS
nginx -T 2>/dev/null | grep -E 'root |server_name|location /' | head -40
echo '===='
for d in /opt/zhiqiyun-ai/admin-vue/dist /opt/zhiqiyun-ai/dist/admin /opt/zhiqiyun-ai/deploy/admin /var/www/html /opt/zhiqiyun-ai/apps/admin-vue/dist; do
  if [ -d "$d" ]; then
    echo "DIR $d"
    ls -lt "$d"/assets/index-*.js 2>/dev/null | head -3 || ls -lt "$d"/assets/*.js 2>/dev/null | head -3
  fi
done
echo '===='
# git log on server
cd /opt/zhiqiyun-ai && git log -5 --oneline && git rev-parse HEAD
echo '===='
# search deployed assets for montage markers
find /opt/zhiqiyun-ai -path '*/node_modules' -prune -o -name 'index-*.js' -print 2>/dev/null | while read f; do
  if grep -q 'montageWork\|SMART_VIDEO_MONTAGE' "$f" 2>/dev/null; then
    echo "HIT $f"
    ls -l "$f"
  fi
done | head -20
