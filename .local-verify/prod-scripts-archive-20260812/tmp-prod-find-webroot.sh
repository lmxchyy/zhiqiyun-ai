#!/bin/bash
set -euo pipefail
nginx -T 2>/dev/null | awk '/server_name ai.zs-kjhn.cn/,/^[[:space:]]*server \{/{print}' | head -80
echo '==== ROOTS ===='
nginx -T 2>/dev/null | grep -nE 'root |alias ' | head -40
echo '==== LIVE INDEX ===='
# What does the live site serve?
curl -sI https://ai.zs-kjhn.cn/ | head -20
curl -s https://ai.zs-kjhn.cn/ | tr '"' '\n' | grep -E 'assets/index-|assets/App-' | head -20
echo '==== SEARCH MONTAGE IN WEBROOT ===='
# Common deploy targets
for d in /opt/zhiqiyun-ai/admin-dist /opt/zhiqiyun-ai/web /opt/zhiqiyun-ai/public /opt/zhiqiyun-ai/frontend /var/www/ai.zs-kjhn.cn /var/www/zhiqiyun /opt/zhiqiyun-ai/apps/admin-vue/dist; do
  [ -d "$d" ] && echo "exists $d" && ls "$d" | head -5
done
# find newest index-*.js outside node_modules modified after Aug 11
find /opt/zhiqiyun-ai /var/www -path '*/node_modules' -prune -o -name 'index-*.js' -mtime -3 -print 2>/dev/null | head -30
