#!/usr/bin/env bash
set -euo pipefail
cd /opt/zhiqiyun-ai

echo "=== project ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,status,current_version,current_version_id,confirmed_version_id,error_stage,error_code,left(coalesce(error_message,''),200) AS err,updated_at FROM video_projects WHERE id='vp_664248192f84dc96631df8cd';"

echo "=== versions ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,version_number,status,source,parent_version_id,manifest_hash IS NOT NULL AS has_manifest,created_at FROM video_project_versions WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY version_number DESC LIMIT 10;"

echo "=== recent plan tasks ==="
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id,state,left(coalesce(error_message,''),180) AS err,output_version_id,created_at,finished_at FROM video_plan_tasks WHERE project_id='vp_664248192f84dc96631df8cd' ORDER BY created_at DESC LIMIT 5;"

echo "=== api logs 10m confirm ==="
docker logs --since 10m zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -iE 'confirm|versions|smart-video|video-projects|invalid_plan|duration|422|400|500|panic' | tail -60 || true
