#!/bin/bash
set -euo pipefail
ASSET_ID=asset_fbc47867bd25964261f742fd
RENDER_ID=svrender_044a0e2b4a72975352db68a7
USER_ID=user_000003
TASK_ID=task_${RENDER_ID}
NOW=$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -v ON_ERROR_STOP=1 <<SQL
WITH asset AS (
  SELECT id, user_id, tenant_id, name, url, thumbnail_url, metadata, created_at, updated_at
  FROM xz_assets WHERE id='${ASSET_ID}'
),
upsert AS (
  INSERT INTO xz_generation_tasks (
    id,user_id,tenant_id,organization_id,billing_account_type,billing_account_id,module_code,type,model,billing_type,
    status,progress,point_cost,prompt,params,result_ids,error,created_at,updated_at,worker_finished_at,
    client_request_id,task_status,billing_status,raw
  )
  SELECT
    '${TASK_ID}',
    a.user_id,
    a.tenant_id,
    null,
    'PERSONAL',
    null,
    'smart_video_montage',
    'SMART_VIDEO_MONTAGE',
    'AI自动混剪',
    'POINTS',
    'SUCCEEDED',
    100,
    0,
    a.name,
    jsonb_build_object(
      'moduleCode','smart_video_montage',
      'renderTaskId','${RENDER_ID}',
      'mediaType','video',
      'fileId', a.metadata->>'fileId',
      'coverFileId', a.metadata->>'coverFileId'
    ),
    jsonb_build_array(a.id),
    'null'::jsonb,
    coalesce(a.created_at, '${NOW}'),
    coalesce(a.updated_at, '${NOW}'),
    coalesce(a.updated_at, '${NOW}'),
    '${RENDER_ID}',
    'SUCCEEDED',
    'CAPTURED',
    jsonb_build_object(
      'id','${TASK_ID}',
      'userId', a.user_id,
      'tenantId', a.tenant_id,
      'clientRequestId','${RENDER_ID}',
      'moduleCode','smart_video_montage',
      'type','SMART_VIDEO_MONTAGE',
      'prompt', a.name,
      'model','AI自动混剪',
      'status','SUCCEEDED',
      'progress',100,
      'pointCost',0,
      'resultIds', jsonb_build_array(a.id),
      'imageUrl', a.url,
      'outputUrl', a.url,
      'resultUrl', a.url,
      'thumbnailUrl', a.thumbnail_url,
      'mediaType','video',
      'name', a.name,
      'createdAt', a.created_at,
      'updatedAt', a.updated_at
    )
  FROM asset a
  WHERE NOT EXISTS (
    SELECT 1 FROM xz_generation_tasks t
    WHERE t.user_id=a.user_id AND coalesce(t.client_request_id,'')='${RENDER_ID}'
  )
  ON CONFLICT (id) DO NOTHING
  RETURNING id
)
SELECT 'inserted_or_exists' AS status, coalesce((SELECT id FROM upsert), (SELECT id FROM xz_generation_tasks WHERE client_request_id='${RENDER_ID}' LIMIT 1)) AS task_id;
SQL

docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -c "
select id,type,status,client_request_id,result_ids,left(prompt,60) as prompt
from xz_generation_tasks
where id='${TASK_ID}' or client_request_id='${RENDER_ID}';
"
