#!/usr/bin/env bash
set -euo pipefail
WK=zhiqiyun-ai-prod-smartvideo-worker-1
echo "--- INSPECT ---"
docker inspect --format 'status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{end}} restarts={{.RestartCount}} exit={{.State.ExitCode}} err={{.State.Error}} oom={{.State.OOMKilled}}' "$WK"
echo "--- LAST LOGS ---"
docker logs --tail=120 "$WK" 2>&1
echo "--- EVENTS ---"
docker events --since 5m --until 0s --filter container="$WK" 2>&1 | tail -20 || true
echo "DONE"
