#!/bin/bash
set -euo pipefail
echo '==== PORT 3100 ===='
ss -lptn | grep 3100 || netstat -lptn | grep 3100 || true
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}' | head -40
echo '==== COMPOSE ===='
ls /opt/zhiqiyun-ai/*.yml /opt/zhiqiyun-ai/docker-compose*.yml /opt/zhiqiyun-ai/deploy/*.yml 2>/dev/null || true
echo '==== CURL LOCAL ===='
curl -s http://127.0.0.1:3100/ | tr '"' '\n' | grep -E 'assets/|/admin/' | head -30
echo '==== CONTAINER FS ===='
# Find which container listens 3100
CID=$(docker ps -q --filter publish=3100)
if [ -z "$CID" ]; then
  CID=$(docker ps --format '{{.ID}} {{.Ports}}' | awk '/3100/{print $1; exit}')
fi
echo "CID=$CID"
if [ -n "$CID" ]; then
  docker inspect "$CID" --format '{{.Name}} {{range .Mounts}}{{.Source}}->{{.Destination}}; {{end}}'
  docker exec "$CID" sh -c 'ls -lt /usr/share/nginx/html/assets/index-*.js 2>/dev/null | head -5; ls -lt /app/dist/assets/index-*.js 2>/dev/null | head -5; ls -lt /var/www/html/assets/index-*.js 2>/dev/null | head -5; find / -name "index-*.js" 2>/dev/null | grep -v node_modules | head -20'
fi
