#!/usr/bin/env bash
set -euo pipefail
echo "=== BEFORE ==="
df -h /
df -h /var/lib/docker 2>/dev/null || true
du -sh /opt/zhiqiyun-ai/backups/* 2>/dev/null | sort -hr | head -20 || true
docker system df || true

echo "=== CLEAN ==="
# prune unused docker data (keep running containers/images in use)
docker container prune -f || true
docker image prune -af || true
docker builder prune -af || true
docker volume prune -f || true
# keep newest 3 postgres backups only
if [ -d /opt/zhiqiyun-ai/backups/postgres ]; then
  cd /opt/zhiqiyun-ai/backups/postgres
  ls -1t *.sql 2>/dev/null | tail -n +4 | xargs -r rm -f
  ls -1t *.sql.gz 2>/dev/null | tail -n +4 | xargs -r rm -f
fi
# clear leftover render temps if any on host
rm -rf /tmp/smartvideo/* 2>/dev/null || true
journalctl --vacuum-size=200M >/dev/null 2>&1 || true

echo "=== AFTER ==="
df -h /
docker system df || true

echo "=== RESTART WORKER ==="
cd /opt/zhiqiyun-ai
# use deploy-auto or compose with env file
if [ -f .env.production ]; then
  set -a; source .env.production; set +a
fi
export TARGET_PLATFORM="${TARGET_PLATFORM:-linux/amd64}"
docker compose -f compose.prod.yml --env-file .env.production up -d --no-deps smartvideo-worker
sleep 8
docker inspect --format 'status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{end}} restarts={{.RestartCount}}' zhiqiyun-ai-prod-smartvideo-worker-1
docker logs --tail=30 zhiqiyun-ai-prod-smartvideo-worker-1 2>&1

# requeue stuck render
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 -c \
  "UPDATE video_render_tasks
     SET status='QUEUED', stage='queued', step='queued', progress=5,
         lease_owner=NULL, lease_expires_at=NULL, heartbeat_at=NULL,
         error_code=NULL, error_message=NULL, run_after=now(), updated_at=now()
   WHERE id='svrender_044a0e2b4a72975352db68a7';
   UPDATE video_task_outbox
     SET state='pending', attempts=0, available_at=now(), published_at=NULL, last_error=NULL
   WHERE aggregate_id='svrender_044a0e2b4a72975352db68a7' AND event_type='enqueue_requested';"

sleep 20
echo "--- TASK ---"
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,stage,step,progress,lease_owner,error_code,left(coalesce(error_message,''),160) AS err,updated_at FROM video_render_tasks WHERE id='svrender_044a0e2b4a72975352db68a7';"
echo "--- LOGS ---"
docker logs --since 1m zhiqiyun-ai-prod-smartvideo-worker-1 2>&1 | grep -iE 'smartvideo_render|speech|acquired|advance|failed|error|inconsistent|no space' | tail -40 || true
echo "CLEAN_DONE"
