# AI 智能成片渲染 Worker 最小闭环

本阶段只支持固定场景 `ZhiqiyunSmartVideoSmoke`：1080×1920、30fps、5 秒、H.264/AAC MP4。任务费用字段保持为 0，不接入 Token 冻结、结算或释放。

## 启动

```powershell
docker compose --profile smartvideo up -d --build migrate xianzhi-ai smartvideo-worker
docker compose ps
docker compose logs -f smartvideo-worker
```

Worker 不开放端口。健康检查只验证 FFmpeg/FFprobe 启动探测已通过且 worker 主循环正在运行。

## 创建数据库 smoke 任务

先替换下面的 `tenant_id`、`user_id`；对应租户必须已经配置可用的对象存储。

```sql
begin;
insert into video_projects
  (id,tenant_id,user_id,title,requirement,status,current_version,created_at,updated_at)
values
  ('vp_smoke','tenant_default','user_smoke','ZhiqiyunSmartVideoSmoke','固定渲染冒烟','CONFIRMED',0,now(),now())
on conflict (id) do nothing;

insert into video_render_tasks
  (id,project_id,tenant_id,user_id,client_request_id,status,progress,step,max_attempts,run_after,specification,created_at,updated_at)
values
  ('vrt_smoke','vp_smoke','tenant_default','user_smoke','ZhiqiyunSmartVideoSmoke-v1','QUEUED',5,'queued',3,now(),
   '{"width":1080,"height":1920,"frameRate":30,"format":"mp4","videoCodec":"h264","audioCodec":"aac","durationMs":5000}'::jsonb,
   now(),now())
on conflict (tenant_id,user_id,client_request_id) do nothing;
commit;
```

```powershell
docker compose exec redis redis-cli LPUSH xianzhi:smartvideo:render:pending '{"taskId":"vrt_smoke"}'
```

任务详情：`GET /api/v1/video-projects/vp_smoke/render-tasks/vrt_smoke`。作品中心：`GET /api/v1/assets`。

## 重试与排障

失败任务使用认证 API 重新入队：

```text
POST /api/v1/video-projects/{projectId}/render-tasks/{taskId}/retry
```

Redis 队列键：

- `xianzhi:smartvideo:render:pending`
- `xianzhi:smartvideo:render:working`
- `xianzhi:smartvideo:render:delayed`
- `xianzhi:smartvideo:render:dead`

验证生成文件：

```powershell
ffprobe -v error -show_entries stream=codec_type,codec_name,width,height,r_frame_rate,pix_fmt -show_entries format=duration,size -of json ZhiqiyunSmartVideoSmoke.mp4
```
