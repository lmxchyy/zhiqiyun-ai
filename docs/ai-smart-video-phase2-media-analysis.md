# AI 智能成片第二阶段：素材分析与预处理

## 范围

本阶段只建立智能成片素材的异步分析和预处理基础设施，不生成 AI 脚本、AI 分镜或正式成片，也不扣减 Token、不发布作品中心、不增加前端页面。

素材入口仍是现有文件中心。客户端只能提交 `fileId` 和素材类型，不能提交本地路径或远程 URL。项目绑定时验证文件属于当前租户和用户且状态为 `ACTIVE`；Worker 执行时通过文件中心再次鉴权并读取对象。

## 任务流程

1. 用户通过既有文件中心上传视频或图片。
2. `POST /api/v1/video-projects/:id/assets` 只绑定 `fileId`，素材状态为 `PENDING`。
3. `POST /api/v1/video-projects/:id/analyze` 要求 `Idempotency-Key`，按 `fileId + storageKey + hash + MIME + size` 计算源指纹，为每个需要分析的素材创建或复用独立分析任务。
4. API 把仅含任务、项目和素材 ID 的消息写入 Redis 可靠队列并立即返回汇总状态。
5. Worker 通过 PostgreSQL 租约领取任务，定时心跳；进程重启后 Redis working 队列会恢复，数据库过期租约可被重新领取。
6. Worker 通过 `internal/storage.Service.OpenObject` 读取已鉴权的私有对象，下载到随机受控临时目录，并限制最大字节数。
7. FFprobe 适配器输出强类型视频或图片元数据；FFmpeg 生成缩略图，视频额外生成低清 H.264/AAC MP4 代理。
8. 派生文件通过 `internal/storage.Service.StoreObject` 写回既有文件中心，标记为 `smart_video_thumbnail` 或 `smart_video_proxy`，数据库仅保存文件 ID。
9. 成功写入标准化元数据和派生文件引用；失败写入稳定 `error_code` 和经过净化的展示消息。可重试错误按指数退避，达到上限进入死信。

## 模块边界

- `internal/app/smartvideo/media.go`：领域强类型、媒体 Port、队列 Port、分析状态。
- `internal/app/smartvideo/analysis_service.go`：请求幂等、源指纹、汇总与显式重试。
- `internal/app/smartvideo/analysis_worker.go`：租约、心跳、超时、退避与最终失败。
- `internal/provider/media`：安全命令执行、FFprobe/FFmpeg 适配和临时文件预处理。
- `internal/infra/smartvideo_queue.go`：Redis pending/working/delayed/dead 队列。
- `internal/storage`：复用文件中心鉴权、对象流读取和服务端派生文件写入。
- `internal/smartvideoruntime`、`cmd/smartvideo-worker`：独立 Worker 运行入口。

领域层不依赖 `os/exec`、FFprobe JSON、对象存储 SDK 或签名 URL。

## 数据库

迁移 `075-smart-video-media-analysis.sql` 在 `video_project_assets` 增加分析快照字段，并新增 `video_asset_analysis_tasks`。

分析任务不能复用 `video_render_tasks`：两者的源指纹、租约、重试、派生文件、超时和生命周期相互独立。唯一约束 `(asset_id, source_fingerprint)` 保证同一素材版本只有一个任务；按租户、用户、幂等键和素材建立部分唯一索引，防止重复请求。

状态转换：

```text
PENDING -> QUEUED -> RUNNING -> SUCCEEDED
                         \----> FAILED -> PENDING（显式重试）
RUNNING -> QUEUED（可重试失败及退避）
```

`SUCCEEDED` 且源指纹未变化时不会重复分析。`RUNNING` 由数据库租约阻止并发执行；最后一次尝试的租约过期会落为 `FAILED`，不会永久卡住。

## API

实际路由遵循项目既有 `/api/v1` 前缀：

- `POST /api/v1/video-projects/:id/analyze`
  - Header：`Idempotency-Key: <client-generated-key>`
  - 返回 `202` 和项目分析汇总。
- `GET /api/v1/video-projects/:id/analysis`
  - 返回总数、等待/运行/成功/失败计数和每个素材的类型化元数据、派生文件 ID、错误码。
- `POST /api/v1/video-projects/:id/assets/:assetId/retry-analysis`
  - 只允许显式重试 `FAILED` 素材，返回 `202`。

所有接口复用现有登录中间件、租户/用户范围和统一 JSON 响应；分析结果不返回内部路径、对象存储凭证或永久公开 URL。

## 配置

配置统一从 `internal/config.Load` 读取：

- `SMARTVIDEO_ANALYSIS_ENABLED`
- `SMARTVIDEO_FFPROBE_PATH`
- `SMARTVIDEO_FFMPEG_PATH`
- `SMARTVIDEO_PROBE_TIMEOUT`
- `SMARTVIDEO_PROCESS_TIMEOUT`
- `SMARTVIDEO_MAX_FILE_BYTES`
- `SMARTVIDEO_MAX_VIDEO_DURATION`
- `SMARTVIDEO_MAX_VIDEO_PIXELS`
- `SMARTVIDEO_MAX_IMAGE_PIXELS`
- `SMARTVIDEO_PROXY_MAX_WIDTH`
- `SMARTVIDEO_PROXY_VIDEO_BITRATE`
- `SMARTVIDEO_PROXY_AUDIO_BITRATE`
- `SMARTVIDEO_ANALYSIS_MAX_ATTEMPTS`
- `SMARTVIDEO_ANALYSIS_WORKER_CONCURRENCY`
- `SMARTVIDEO_TEMP_DIR`

默认分析开关关闭。开发 Compose 的 Worker 位于 `smartvideo` profile，默认不启动；生产 Compose 未接入 Worker，待独立评审资源、容量和发布策略后再启用。

## 运行环境

后端镜像基于 Alpine 3.20，构建时固定安装 `ffmpeg=6.1.1-r8`，同时提供 FFprobe 6.1.1；运行时不联网下载。Worker 启动时执行工具版本检查并记录版本，工具缺失或不可执行会使 Worker 启动失败。

开发 Worker 使用独立进程、2 CPU、2 GiB 内存、256 PID 和 2 GiB tmpfs，健康检查基于 Worker 成功启动后写入的标记文件，并支持信号优雅退出。

## 已知边界

- 本阶段没有 AI 脚本、AI 分镜或正式视频渲染实现。
- 没有 Token 预估、冻结、扣费或退款。
- 派生文件进入文件中心，但尚未发布到作品中心。
- 生产部署尚未增加 Worker；`SMARTVIDEO_ANALYSIS_ENABLED` 在生产批准前应保持关闭。
- 首版代理参数支持配置，但编码队列、硬件加速和不同终端自适应规格留待容量测试后确定。
