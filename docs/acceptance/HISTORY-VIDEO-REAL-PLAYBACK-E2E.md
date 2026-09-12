# History Video Real Playback E2E

- **验证时间**：2026-09-10
- **范围**：仅验证 `VIDEO-P1-001` 的隔离 `EXPIRED` 视频用户页面行为
- **未执行**：RC Audit、Secret、Payment、Backup、VIDEO_ASYNC 改动
- **代码状态**：仅修复 H5 开发环境的同源 API/认证会话连续性与持久 token 首帧初始化；未修改视频业务、Asset 状态、VIDEO_ASYNC 或 RC Audit

## 1. 最终结论

```text
HISTORY_VIDEO_REAL_PLAYBACK_E2E=PASS
HISTORICAL_VIDEO_FILE_RECOVERY=NOT_RECOVERABLE
HISTORY_VIDEO_USER_EXPERIENCE=PASS
VIDEO_P1_001_STATUS=CLOSED
VIDEO_PLATFORM_RC_STATUS=NOT_CLOSED
FIXTURE_CREATED=PASS
ASSET_API_EXPIRED_RESPONSE=PASS
REAL_USER_PAGE_CLICKTHROUGH=PASS
AUTH_E2E_ENVIRONMENT=PASS
```

视频 E2E 已完成；视频平台 RC 仍保持 NOT_CLOSED，因为其它 RC/P0/P1 阻断未处理。本报告只关闭 VIDEO-P1-001 的真实页面验收，不代表整个视频平台 RC 关闭。

## 2. 隔离测试资产

已在本地 Docker PostgreSQL `ai-postgres-1` 的 `xianzhi` 数据库中，以事务方式写入唯一测试资产：

```text
asset_id = asset_test_expired_video_20260910
user_id  = user_000003
task_id  = null（asset-only 隔离 fixture，避免依赖不存在的任务投影）
media    = video
fixture  = VIDEO-P1-001
isolated = true
```

写入值：

```text
url            = https://expired-provider.example/video.mp4
fileId         = null
availability   = EXPIRED
videoStatus    = EXPIRED
```

该资产没有写入生产环境，也没有调用 Provider 或生成模型。

## 3. API 验证

使用真实登录接口取得 `user_000003` token 后请求：

```text
GET /api/v1/assets/asset_test_expired_video_20260910
```

结果：HTTP 200，backend 正确返回：

```json
{
  "mediaType": "video",
  "url": "",
  "availability": "EXPIRED",
  "videoStatus": "EXPIRED",
  "metadata": {
    "fileId": null,
    "availability": "EXPIRED",
    "videoStatus": "EXPIRED",
    "expiredSourceUrl": "https://expired-provider.example/video.mp4"
  }
}
```

判定：`ASSET_API_EXPIRED_RESPONSE=PASS`。

## 4. 真实 H5 页面验收

当前 H5 入口：

```text
App.vue -> AiCreationPage.vue -> 作品中心
```

运行配置使用 H5 同源 API（`VITE_API_BASE_URL=''`），避免 `.env.local` 中面向手机局域网的 `192.168.1.12:3100` 地址在浏览器中失效。代码同时加入了 H5 `BASE_URL=/h5/` 感知路由及持久 token 首帧初始化。

真实页面地址：

```text
http://127.0.0.1:5173/h5/app/works
```

验收证据：

1. 页面使用真实登录 token 加载，`GET /api/v1/auth/me`、`/api/v1/assets`、`/api/v1/generation-tasks`、`/api/v1/models`、`/api/v1/points/account` 均返回 HTTP 200。
2. 作品中心真实 DOM 出现隔离资产“历史失效视频 E2E 测试资产”，并显示：`视频已失效`、`视频资源已失效，可按原提示词重新生成`、按钮“重新生成”。
3. 该资产 DOM 没有 `<video>` 元素；页面中 `video` 数量为 `0`。
4. 点击真实卡片“预览”后仍无 `<video>`，资源列表未出现 `expired-provider.example`、`.mp4` 或 `.m4v` 请求。
5. 下载按钮为 disabled，失效资产的 `assetUrl` 为空，未打开旧 Provider URL。
6. 点击真实卡片“重新生成”后，浏览器路径进入 `/h5/app/video-generation`，模块状态为 `videoGeneration`，原提示词为 `历史失效视频 E2E 重新生成测试`。

判定：`REAL_USER_PAGE_CLICKTHROUGH=PASS`，`AUTH_E2E_ENVIRONMENT=PASS`。本次没有使用空列表、mock DOM、Provider 调用或伪造播放成功。

## 5. 当前前端代码的目标行为

`apps/user-uni/src/pages/AiCreationPage.vue` 已实现：

```text
availability/videoStatus=EXPIRED
        ↓
assetUrl=""
        ↓
作品卡片显示“视频已失效”
        ↓
预览/下载不打开死链接
        ↓
“重新生成”带入原提示词并进入视频生成模块
```

小程序/App 的 `AssetDetailCenterPage.vue` 已有同等失效卡片分支：`EXPIRED` 时不渲染 `<video>`。

以上代码路径已由本次真实 H5 用户页面 fixture E2E 验证；历史第三方源本身的播放失败结论仍由 VERIFY 报告保留。

## 6. 结论边界

本报告关闭的是 `VIDEO-P1-001` 的真实 H5 用户体验验收：失效资源不再黑屏或触发死链接，并可带原提示词重新生成。

验收语义拆分为：

```text
HISTORICAL_VIDEO_FILE_RECOVERY=NOT_RECOVERABLE
HISTORY_VIDEO_USER_EXPERIENCE=PASS
VIDEO_P1_001_STATUS=CLOSED
```

`HISTORY_VIDEO_REAL_PLAYBACK` 作为旧的混合命名不再用于判定；旧 Provider 文件本体不可恢复，不阻塞用户体验修复。全局 `VIDEO_PLATFORM_RC_STATUS` 仍为 `NOT_CLOSED`，因为当前 RC 还有其它 P0/P1 与生产证据 blocker，尚未满足全局关闭标准。
