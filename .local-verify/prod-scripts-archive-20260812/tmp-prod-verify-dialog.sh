#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai
echo "COMMIT=$(git rev-parse --short HEAD)"
git status -sb
docker compose -f compose.prod.yml --env-file .env.production ps
echo "=== health ==="
curl -fsS http://127.0.0.1:8080/api/v1/health || curl -fsS http://127.0.0.1/api/v1/health || true
echo
echo "=== binary markers ==="
CID=$(docker compose -f compose.prod.yml --env-file .env.production ps -q xianzhi-ai | head -1)
docker exec "$CID" sh -c 'strings /opt/xianzhi-api | grep -F "1280x720" | head -2; strings /opt/xianzhi-api | grep -F "grok-imagine-video-1.5-preview" | head -2; strings /opt/xianzhi-api | grep -F "视频参考图参数格式错误" | head -1'
