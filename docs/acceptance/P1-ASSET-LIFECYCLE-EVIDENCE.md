# P1 Asset Lifecycle Evidence

- **采集时间**：2026-09-09
- **结论**：`PARTIAL / NOT VERIFIED`

## 视频

承接既有 `VIDEO_ASYNC` 专项验收：视频资产持久化、历史播放防守、签名访问和幂等已有 acceptance 记录，本文件不重新打开已关闭的视频 RC 项。

## 图片

本轮启动本地 API 后，通过 API 创建了白名单 mock 图片任务并验证：

```text
task_id: task_000296
asset_id: asset_000223
file_id: file_b0b9d6e3c326991bcee9da07
status: SUCCEEDED
billing_status: CAPTURED
asset download: HTTP 200
content type: image/png
bytes: 1,795,927
storageManaged: true
```

该任务使用数据库中现有的 R2 storage config，不能作为 MinIO 或生产 Provider Canary。测试资产已执行 soft delete + permanent delete；File Object 进入 `DELETED` recycle 状态。

## PPTX

代码已实现 `PPTX -> File Object -> storage:// reference -> signed URL`，但当前未执行真实 API + PostgreSQL + MinIO/R2 PPTX 导出闭环。

## 智能混剪

代码和 Node/Go 套件通过；当前没有真实 Worker render、视频对象、signed download、Capture/Release 对账证据。

## 缺失证据

```text
Generation -> Artifact -> File Object -> Asset
```

需要对图片、PPTX、视频、混剪分别提供真实对象 key、file_id、asset_id、HTTP 访问和重复执行幂等记录，并证明长期不保存 Provider URL。

## 状态

`PARTIAL / PPTX_AND_WORKER_ASSET_E2E_NOT_VERIFIED`
