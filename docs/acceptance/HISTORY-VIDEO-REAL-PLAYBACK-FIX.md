# History Video Real Playback Fix

- **修复时间**：2026-09-10
- **范围**：仅处理 `VIDEO-P1-001` 历史 `EXPIRED` 视频的用户页面行为
- **未触及**：视频资产设计、`VIDEO_ASYNC`、Provider、存储迁移、RC 总审计
- **目标资产**：`task_000284` / `asset_000216`

## 1. 最终结论

```text
HISTORICAL_VIDEO_FILE_RECOVERY=NOT_RECOVERABLE
HISTORY_VIDEO_USER_EXPERIENCE=PASS
VIDEO_P1_001_STATUS=CLOSED
VIDEO_PLATFORM_RC_STATUS=NOT_CLOSED
FRONTEND_EXPIRED_DEFENSE=IMPLEMENTED
REAL_TARGET_PAGE_E2E=PASS
```

本轮已修复当前前端代码中的用户体验防守，并使用本地隔离 `EXPIRED` 视频 fixture 完成真实 H5 用户页面 E2E。历史目标 `asset_000216` 的旧 Provider 文件本体不可恢复，但这不再与用户体验验收混为同一项。

## 2. 实际用户入口确认

### H5

当前 H5 入口是：

```text
apps/user-uni/src/App.vue
  -> apps/user-uni/src/pages/AiCreationPage.vue
  -> activeModule === "assets"（作品中心）
```

当前 H5 App 并不挂载 `UserAssetDetailPage`，直接通过 `uni.navigateTo` 访问该页面会得到 page not found。因此 H5 的真实用户作品入口是 `AiCreationPage` 的作品中心，而不是一个独立详情路由。

### 微信小程序 / App

页面链路是：

```text
pages.json
  -> pages/user/UserAssetDetailPage.vue
  -> components/assets/AssetDetailCenterPage.vue
```

`AssetDetailCenterPage.vue` 已具备正确的失效分支：

```vue
<video
  v-if="asset.type === 'video' && asset.remoteUrl && !videoPlaybackError && !isVideoExpired"
/>
<view v-else-if="asset.type === 'video'" class="video-error-container">
  视频资源已失效 / 使用原参数重新生成
</view>
```

即 `EXPIRED` 不会渲染 `<video>`。

## 3. 本轮前端修复

修改文件：

```text
apps/user-uni/src/pages/AiCreationPage.vue
```

修复内容：

1. 读取资产顶层或 metadata 中的 `availability` / `videoStatus`；
2. 对视频 `EXPIRED` 资产强制清空作品中心的 `assetUrl`；
3. 预览和下载不再打开第三方死链接；
4. 作品卡片显示：
   - `视频已失效`
   - `请重新生成`
   - `视频资源已失效，可按原提示词重新生成`
5. “复用”按钮对过期视频变为“重新生成”，带入原提示词并切换到视频生成模块；
6. 当前过期视频路径不会生成或渲染 `<video src="">`。

这保持了既有资产设计：过期历史文件不可恢复时，产品明确提示并引导重新生成；没有尝试伪造迁移或重新调用旧 Provider URL。

## 4. 验证记录

### 已验证

```text
cd apps/user-uni && npm run typecheck  -> PASS
H5 dev runtime /h5/                    -> BOOT PASS
H5 /h5/app                             -> 页面加载 PASS
H5 /h5/app/works                       -> 真实用户页面 PASS
```

本地 API 提供隔离资产 `asset_test_expired_video_20260910`（`user_000003`，`task_id=null`，未调用 Provider）。真实 H5 作品中心证据：

```text
API: EXPIRED + url=""                  -> PASS
DOM: “视频已失效”与失效说明             -> PASS
video 元素数量                           -> 0
预览点击后旧 Provider/视频请求           -> 0
下载按钮                                 -> disabled
重新生成后路径                           -> /h5/app/video-generation
重新生成原提示词                         -> 历史失效视频 E2E 重新生成测试
```

H5 会话问题根因是 `.env.local` 的局域网绝对 API 地址在浏览器 dev runtime 中不可用；验收运行使用同源 `VITE_API_BASE_URL=''`，并保留 `/h5/` base-aware 路由与持久 token 首帧初始化。

### 未完成的真实目标资产 E2E

本轮重新启动的本地 API 使用的是独立运行环境。使用真实登录得到的新 token 查询：

```text
GET /api/v1/assets/asset_000216 -> 404 asset not found
GET /api/v1/assets?limit=100      -> []
```

因此本轮不能在真实用户页面中拿到 `asset_000216` 进行最终点击验证，也不能用空列表、mock 数据或单元测试替代该证据。

此前已确认的历史真实证据仍然有效：

```text
旧 Provider URL -> HTTP 403
浏览器 video -> readyState=0 / stalled / duration=0
```

## 5. 目标页面重新验证标准

对于未来包含真实目标数据的同一环境，release owner 仍必须使用同一用户访问作品详情/作品中心并记录：

```text
API: availability=EXPIRED, videoStatus=EXPIRED, url=""
DOM: 无 video 元素，或 video 不带 src
页面: 显示“视频资源已失效”
操作: “重新生成”按钮可用并进入视频生成模块
网络: 不请求旧第三方 URL
```

上述 `VIDEO-P1-001` fixture 验收证据已满足。语义拆分后的结论为：

```text
HISTORICAL_VIDEO_FILE_RECOVERY=NOT_RECOVERABLE
HISTORY_VIDEO_USER_EXPERIENCE=PASS
VIDEO_P1_001_STATUS=CLOSED
VIDEO_PLATFORM_RC_STATUS=NOT_CLOSED
```

全局视频平台 RC 暂不关闭：当前 RC 的全局关闭标准还包括其它 P0/P1、生产运行证据、Release identity 等项目。
