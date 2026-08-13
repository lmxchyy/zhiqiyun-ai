#!/usr/bin/env bash
set -euo pipefail
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c \
  "SELECT id, kind, asset_type, analysis_status, file_id FROM video_project_assets WHERE project_id='vp_664248192f84dc96631df8cd';"
docker logs --since 2h zhiqiyun-ai-prod-xianzhi-ai-1 2>&1 | grep -iE 'multipart|MinIO|storage|upload' | tail -50 || true
