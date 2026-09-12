# History Video Real Playback Verification

- **验证时间**：2026-09-10
- **目标资产**：`asset_000216` / `task_000284`
- **验证模式**：当前工作区 backend + 当前 user-uni H5 dev runtime；不使用单元测试结果替代真实浏览器媒体行为
- **相关冻结**：本次暂停 RC Audit，仅处理历史视频黑屏链路

## 1. 最终结论

```text
HISTORICAL_VIDEO_FILE_RECOVERY=NOT_RECOVERABLE
HISTORY_VIDEO_USER_EXPERIENCE=PASS（见 E2E 报告）
BACKEND_CURRENT_ENDPOINT=PASS
PROVIDER_SOURCE_PLAYBACK=FAIL
USER_DETAIL_RUNTIME_VERIFIED=NO（H5 作品中心 E2E 已通过）
VIDEO_PLATFORM_RC_STATUS=NOT_CLOSED
```

本报告记录的是旧 Provider 文件本体的不可恢复性与真实媒体失败证据，不再把它与用户体验降级混为一项。当前后端已正确识别过期资产并停止下发死链接；真实 H5 作品中心的失效降级、无播放器、下载禁用和重新生成入口已由 E2E 报告验证通过。

## 2. 当前 backend 真实接口结果

使用当前工作区代码启动 API `:3100`，连接本地 PostgreSQL `127.0.0.1:54321`，以资产所属用户 `user_000003` 登录后执行：

```text
POST /api/v1/auth/login
GET  /api/v1/assets/asset_000216
GET  /api/v1/assets?limit=100
```

响应关键字段：

```json
{
  "id": "asset_000216",
  "taskId": "task_000284",
  "mediaType": "video",
  "url": "",
  "availability": "EXPIRED",
  "videoStatus": "EXPIRED",
  "metadata": {
    "fileId": null,
    "storageFileId": null,
    "storageManaged": null,
    "availability": "EXPIRED",
    "videoStatus": "EXPIRED",
    "expiredSourceUrl": "https://getapib.org/video/57fbfa17-ef50-49ec-8652-512a999aaa6b.mp4"
  }
}
```

判定：

- 当前 backend 没有继续把第三方失效 URL 放进 `item.url`；
- `availability` / `videoStatus` 已为 `EXPIRED`；
- `fileId` 为空符合该历史数据事实；
- 原始死链接只留在 `metadata.expiredSourceUrl`，用于诊断，不用于播放。

## 3. 第三方源真实 HTTP 结果

对历史 `expiredSourceUrl` 执行只读请求：

```text
HTTP/1.1 403 Forbidden
Content-Type: application/xml
Server: nginx/1.18.0 (Ubuntu)
X-Cache: Error from cloudfront
```

这确认原始媒体源本身不能再作为播放源。

## 4. 浏览器真实媒体行为

使用真实浏览器打开包含该历史 URL 的 `<video controls preload="metadata">` 页面，记录到：

```json
{
  "event": "loadstart",
  "src": "https://getapib.org/video/57fbfa17-ef50-49ec-8652-512a999aaa6b.mp4",
  "readyState": 0,
  "networkState": 2,
  "duration": null
}
```

随后：

```json
{
  "event": "stalled",
  "src": "https://getapib.org/video/57fbfa17-ef50-49ec-8652-512a999aaa6b.mp4",
  "readyState": 0,
  "networkState": 2,
  "duration": null
}
```

浏览器媒体控件显示总时间 `0:00`。该行为与用户报告的黑屏一致；不能判定为播放 PASS。

## 5. 当前前端链路核对

### 已存在的防守代码

`apps/user-uni/src/components/assets/AssetDetailCenterPage.vue` 当前代码：

- `EXPIRED` 时不渲染 `<video>`；
- 进入 `video-error-container` 失效卡片；
- 绑定 `@error="handleVideoError"`；
- 通过 `videoStatus` / `availability` 判断失效。

`apps/user-uni/src/features/assets/api.ts` 当前代码：

- 解析 `availability`；
- 解析 `videoStatus`；
- 不会把空的 `url` 变成可播放地址。

### 当前 H5 runtime 的边界

本次启动的 user-uni H5 入口由 `apps/user-uni/src/App.vue` 的 H5 分支直接挂载 `AiCreationPage`。在该 runtime 中：

- `UserAssetDetailPage` 没有被 H5 App 入口挂载；
- `uni.navigateTo('/pages/user/UserAssetDetailPage?...')` 返回 `page ... is not found`；
- 因而本次不能声称已经在 H5 中实际渲染了 `AssetDetailCenterPage` 的失效卡片。

当前 H5 作品中心的 `AiCreationPage.vue` 仍有 `resolveAssetUrl(asset)` / `window.open(assetUrl)` 的通用历史链接路径。该路径不能作为历史视频失效卡片的真实验收证据；若运行的是旧 API/旧构建，仍可能直接打开第三方 URL 并得到黑屏。

## 6. 根因与当前失败边界

```text
历史 xz_assets.url = 第三方临时 URL
        ↓
第三方返回 403
        ↓
浏览器 readyState=0 / stalled / duration=0
        ↓
黑屏
```

旧构建或未包含防守逻辑的环境仍可能让用户继续看到黑屏；当前工作区已完成“识别 EXPIRED、清空播放 URL”的路径，并已在真实 H5 作品中心完成 E2E。

1. 当前用户访问的是旧 API/旧 Docker 镜像；
2. 当前用户访问的是未包含最新前端防守组件的旧小程序/App 构建；
3. 用户实际进入的是 H5 `AiCreationPage` 的旧通用链接路径，而不是 `AssetDetailCenterPage`；
4. 当前 API 与用户打开的前端/数据库不是同一环境。

## 7. 语义边界与后续条件

`VIDEO-P1-001` 的用户体验项已关闭；以下条件仅适用于未来需要恢复旧视频文件本体，或在真实目标 `asset_000216` 上补做同等验证的场景：

1. 将当前 backend 变更部署到用户实际访问的 API 环境；
2. 将包含 `AssetDetailCenterPage.vue` 防守逻辑的实际小程序/App 构建发布到测试环境；
3. 使用同一用户、同一数据库中的 `asset_000216` 或另一条历史过期视频；
4. 在真实用户详情页抓取：`GET /api/v1/assets/{id}`、DOM/video src、媒体请求、console 和页面失效卡片；
5. 期望结果：`url` 为空、`videoStatus=EXPIRED`、不发起第三方视频请求、显示“视频源已失效/重新生成”卡片。

旧 Provider 文件本体仍不可恢复；全局 RC 是否关闭不由本项单独决定，当前保持：

```text
HISTORICAL_VIDEO_FILE_RECOVERY=NOT_RECOVERABLE
HISTORY_VIDEO_USER_EXPERIENCE=PASS
VIDEO_PLATFORM_RC_STATUS=NOT_CLOSED
```
