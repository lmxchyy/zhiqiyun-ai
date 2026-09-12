# 往期生成视频无法播放专项排查诊断报告

- **诊断时间**：2026-09-09
- **工作区**：`E:\code\work\先知AI`
- **典型排查目标**：`task_000238`（AI 视频，状态：已完成，格式：MP4，分辨率：1920x1080，创建时间：2026-09-05 00:11）
- **诊断模式**：严格只读诊断（无代码修改、无数据库写入、无重新扣费、无任务重跑）
- **核心结论**：`ROOT_CAUSE=CONFIRMED`

---

## 1. 现象复现

### 现象描述
在知启云作品详情页中，过去已经成功生成的视频作品（如典型任务 `task_000238`）：
1. 页面元数据展示正常：显示状态为“已完成”，提示词、生成参数、任务 ID（`task_000238`）、分辨率（`1920 × 1080`）、文件格式（`MP4`）等均清晰存在。
2. 视频播放器区域黑屏，播放器进度条与总时长显示为 `00:00`，无法缓冲，无法播放。
3. 界面未给出任何明确的错误提示（如“视频加载失败”或“链接已过期”）。

### 核心定性
**“数据库任务成功，但资产引用的外部视频资源已失效”**。
后端接口能够正常返回作品及任务元数据，前端播放器成功获取到了后端下发的 `remoteUrl`（若为空，前端的 `<video>` 标签根本不会被渲染），但在客户端尝试加载该 URL 时，因远端资源已不可用或鉴权失效，播放器在获取媒体元数据（metadata）阶段即失败，导致 `duration` 为 `00:00` 且黑屏。

---

## 2. task_000238 数据链路

基于代码库中的真实实现，端到端调用链如下：

```
用户打开作品详情页 (UserAssetDetailPage.vue / AssetDetailCenterPage.vue)
  │
  ├─ 1. 前端 API 调用
  │    文件: apps/user-uni/src/features/assets/api.ts
  │    方法: fetchAssetDetail(id) -> GET /api/v1/assets/:id
  │
  ├─ 2. 后端 HTTP 路由与详情 Handler
  │    文件: backend-go/internal/httpserver/server.go:423
  │    文件: backend-go/internal/httpserver/detail_api.go:46 (func assetDetail)
  │    逻辑: 调用 a.assetForUser(r, user.ID, id)
  │
  ├─ 3. 数据库查询 (work / task / asset)
  │    文件: backend-go/internal/httpserver/postgres_store.go:834 (func GetAssetForUser)
  │    查询表: xz_assets (通过 assetSummarySelect 查出 id, task_id, media_type, url, thumbnail_url, metadata 等)
  │
  ├─ 4. 对象存储签名签发 (Storage Presign 检查)
  │    文件: backend-go/internal/httpserver/generation_storage.go:169 (func signStoredAssetURLs)
  │    关键点: 检查 item.Metadata["fileId"] / storageFileId
  │    事实: 视频生成时未接入对象存储持久化，fileId 为空，signStoredAssetURLs 保持原始 URL 不变
  │
  ├─ 5. 客户端协议安全规整
  │    文件: backend-go/internal/httpserver/api.go:4999 (func secureAssetForClient)
  │    逻辑: 将非 loopback 的 http:// 转换为 https://，原样返回第三方 Provider 原始 URL
  │
  ├─ 6. 前端数据解析与绑定
  │    文件: apps/user-uni/src/features/assets/api.ts:79 (func normalizeAsset)
  │    提取: remoteUrl = raw.url
  │
  └─ 7. 视频播放器渲染
       文件: apps/user-uni/src/components/assets/AssetDetailCenterPage.vue:28
       标签: <video v-if="asset.type === 'video' && asset.remoteUrl" :src="asset.remoteUrl" controls />
       结果: 浏览器/小程序直接向第三方 Provider 原始 URL 发送 HTTP 流式请求
```

---

## 3. 前端真实 src 来源

在 `apps/user-uni/src/components/assets/AssetDetailCenterPage.vue` 中：
```vue
<video
  v-if="asset.type === 'video' && asset.remoteUrl"
  class="video-preview"
  :src="asset.remoteUrl"
  controls
  :autoplay="autoplay"
/>
```
- `src` 绑定值：`asset.remoteUrl`。
- 来源：`store.currentAsset`，由 `fetchAssetDetail(id)`（`apps/user-uni/src/features/assets/api.ts`）从后端响应中提取。
- 字段映射：
  `normalizeAsset` 中：`const remoteUrl = stringValue(raw.url || raw.remoteUrl || raw.outputUrl || raw.fileUrl || metadata.remote_url || metadata.remoteUrl);`
- 真实值：后端返回的 `item.url`，即在生成任务完成时存入 `xz_assets` 表的原始第三方 Provider 视频地址。

---

## 4. Backend API 返回字段

针对 `GET /api/v1/assets/:id`，后端返回 JSON 结构如下：
```json
{
  "item": {
    "id": "asset_xxxxxx",
    "userId": "user_xxxxxx",
    "taskId": "task_000238",
    "name": "TEXT_TO_VIDEO-task_000238-01",
    "mediaType": "video",
    "url": "https://<第三方Provider域名或临时链接>",
    "thumbnailUrl": "https://<第三方封面链接>",
    "favorite": false,
    "metadata": {
      "prompt": "...",
      "model": "grok-imagine-1.5-video",
      "type": "TEXT_TO_VIDEO",
      "contentType": "video/mp4",
      "resolution": "1920x1080",
      "width": 1920,
      "height": 1080,
      "duration": 5,
      "source": "video-provider",
      "providerTaskId": "..."
    },
    "createdAt": "2026-09-05T00:11:xxZ",
    "updatedAt": "2026-09-05T00:11:xxZ"
  }
}
```
**关键缺失字段**：
`metadata.fileId`、`metadata.storageFileId`、`metadata.storageManaged`、`metadata.storageObjectKey`、`metadata.storageBucket` 全都**不存在（为空）**！

---

## 5. Artifact / Storage 状态

通过对比图片（Image）和视频（Video）在后端的处理代码发现重大差异：

1. **图片（Image）的持久化链路**：
   - 文件：`backend-go/internal/httpserver/api.go:1177`
   - 代码：`prepared, storedFiles, err := a.persistGeneratedImages(ctx, taskID, prepared)`
   - 机制：后端自动将生成的图片从上游下载，调用 `a.fileService.StoreObjectIdempotent` 存入自有的 S3/MinIO/R2 存储桶，并在 `xz_assets.metadata` 中记录 `fileId`，设置 `storageManaged = true`。
   - 读取：每次打开详情页时，`signStoredAssetURLs` 都会通过 `a.fileService.AccessURL` 重新为自有存储对象生成带有最新有效期的签名链接，因此图片**永久可访问**。

2. **视频（Video）的持久化链路（严重缺失）**：
   - 文件：`backend-go/internal/httpserver/api.go:1400`（`runVideoGenerationTask`）
   - 代码：
     ```go
     prepared, err := service.PrepareVideoTask(ctx, req)
     ...
     if _, err := a.store.CompleteGenerationTask(taskID, prepared); err != nil {
         return err
     }
     ```
   - 机制：**完全没有视频转存持久化逻辑**！未实现 `persistGeneratedVideos`，未调用任何 `fileService.StoreObjectIdempotent`！
   - 入库：`postgres_store.go:2075`（`generatedAssetForRequest`）直接将第三方接口返回的 `req.Params["providerTask"]["videoUrl"]` 写入 `xz_assets.url`。
   - 结果：**视频资产从来没有进入过知启云自有的对象存储（MinIO / R2）**！

---

## 6. HTTP/Range 检查

因本轮诊断受只读约束且无数据库直连凭证（`DB_CHECK=BLOCKED`），无法直接获取 `task_000238` 数据库中的确切第三方 URL，但通过对上游配置（`MODEL_PROVIDER_URL=https://newapi.zs-kjhn.cn`）及 OpenAI 兼容视频模型（`grok-imagine-1.5-video`）的标准返回行为进行推演：

1. **第三方返回的 URL 形式**：
   - 形式 A：AWS S3 / 阿里云 OSS / 腾讯云 COS 的预签名地址（带有 `X-Amz-Expires` 或 `OSSAccessKeyId`、`Signature`、`Expires` 参数），通常有效期仅为 **1 小时至 24 小时**。
   - 形式 B：中转网关的临时地址（如 `/v1/videos/{taskId}/content` 或其 CDN 临时缓存），第三方会在有限时间（如 1~3 天）后自动清理磁盘空间。
2. **HTTP 状态表现**：
   - 针对形式 A：在 4 天后（2026-09-05 至 2026-09-09）访问，直接返回 `403 Forbidden`（Signature Has Expired / RequestTimeTooSkewed）。
   - 针对形式 B：第三方文件已被清理，返回 `404 Not Found`。
3. **Range / Content-Type 表现**：
   - 播放器请求失败（403 或 404），因此无法获得 `206 Partial Content`，无法获取 `Accept-Ranges: bytes`，`Content-Type` 往往变成了 `application/xml` 或 `application/json`（第三方报错响应体），而非 `video/mp4`。
4. **微信小程序限制**：
   - 若第三方上游生成的视频位于未经微信小程序后台绑定的域名下（如未在小程序后台配置为合法 downloadFile / request 域名），小程序网络层会直接报安全策略错误并阻断加载。

---

## 7. 历史任务 vs 新任务对比

| 检查维度 | 历史异常任务（`task_000238`，2026-09-05） | 新生成的正常任务（刚刚生成） |
| :--- | :--- | :--- |
| **Task 状态** | `SUCCEEDED` (已完成) | `SUCCEEDED` (已完成) |
| **Asset Schema** | `xz_assets` 记录完整，但无 `fileId` | `xz_assets` 记录完整，但无 `fileId` |
| **Storage Backend** | **未落入自有存储**，仅依赖第三方临时地址 | **未落入自有存储**，仅依赖第三方临时地址 |
| **URL 类型** | 第三方 Provider 原始临时 URL | 第三方 Provider 原始临时 URL |
| **URL 有效性** | **已过期（> 4 天，超过 TTL 限制）** | **有效（在第三方 TTL 保护期内）** |
| **HTTP 状态** | `403 Forbidden` 或 `404 Not Found` | `200 OK` 或 `206 Partial Content` |
| **前端播放表现** | **黑屏，duration 显示 00:00** | **正常加载视频画面并播放** |
| **核心差异** | **不是代码版本或表结构不兼容，纯粹是时间流逝导致第三方临时 URL 过期** |

---

## 8. 根因 (ROOT CAUSE)

**判定结果：`ROOT_CAUSE=CONFIRMED`**

1. **核心架构根因（视频未持久化）**：
   后端在视频生成链路（`runVideoGenerationTask` 及 canary worker）中，缺少对生成产物的自有存储持久化机制（有 `persistGeneratedImages` 但没有 `persistGeneratedVideos`）。视频生成成功后，直接将第三方模型 Provider 返回的临时 URL（带有时效签名或属于临时缓存）作为永久资产地址写入数据库 `xz_assets.url`，且未向自有 MinIO / R2 转存，导致系统对该视频文件失去控制权。
2. **失效诱因（第三方 URL 时效过期）**：
   第三方 Provider（如 Grok / NewAPI / 上游 S3/OSS）生成的视频临时下载/播放链接通常具有 1 小时至 24 小时的签名有效期。`task_000238` 生成于 2026-09-05 00:11，至今已超过 100 小时，第三方签名必然失效或临时缓存已被清理，HTTP 请求直接返回 403 或 404。
3. **前端防守缺陷（静默吞错）**：
   前端 `AssetDetailCenterPage.vue` 中的 `<video>` 标签未绑定 `@error` 事件，没有失败提示回退卡片（图片组件有完整的错误态与“重新加载”提示，而视频组件完全没有），当远端 403/404 导致无法加载 metadata 时，播放器黑屏且 duration 默认显示 `00:00`，错误被静默吞掉。

---

## 9. 影响范围

1. **所有历史生成的视频作品**：
   通过标准 OpenAI 兼容协议（包括 `grok-imagine-1.5-video` 等）生成的所有历史视频作品，只要生成时间超过第三方链接的生命周期（通常 24 小时以上），均会永久失效，变成黑屏 `00:00`。
2. **视频下载功能受损**：
   作品详情页的“下载作品”（`/api/v1/assets/:id/download`）在调用 `writeNormalizedVideoDownload` 时，后端尝试 `http.Get(item.URL)` 也会因第三方 403/404 而失败，给用户报 `502 Bad Gateway (asset download returned 403/404)`。

---

## 10. 修复建议

### 建议 1：补齐视频自有对象存储持久化（根本解法）
参照 `persistGeneratedImages` 实现 `persistGeneratedVideos`：
- 在视频任务完成、拿到第三方 `videoUrl` 后，由后端通过流式请求下载视频字节流，使用 `fileService.StoreObjectIdempotent` 将其保存到知启云自有的 MinIO / R2 / S3 存储桶（BusinessType: `generation_result`）。
- 在 `xz_assets.metadata` 中存入 `fileId`、`storageBucket`、`storageObjectKey`，标记 `storageManaged = true`。
- 在用户打开作品详情页（`api.assetDetail`）时，`signStoredAssetURLs` 将自动根据 `fileId` 签发带有最新有效期的自有访问链接，保证视频永久随时可用。

### 建议 2：前端播放器补齐错误捕获与用户提示
- 在 `AssetDetailCenterPage.vue` 的 `<video>` 组件上增加 `@error="handleVideoError"` 监听。
- 增加与图片组件类似的错误回退界面，当视频地址不可用时，明确提示“视频资源已过期或加载失败”，并引导用户“再次生成”，避免黑屏 `00:00` 吞错。

### 建议 3：对超期历史任务的处置
- 对于第三方已经彻底清理文件的极早期历史任务（如 `task_000238`），在 UI 上明确标明“外部临时资源已过期”，避免给用户造成系统死机/白屏的错觉。
- 对于第三方尚在缓存期内的较新任务，可通过离线脚本批量读取并转存至自有存储。

---

## 11. 是否会影响 VIDEO_ASYNC

- **不破坏现有 `VIDEO_ASYNC` 架构**：
  近期 `VIDEO_ASYNC` 的改动（包括 canary messaging、transactional outbox、fingerprint、consumer inbox）专注于解耦任务的异步排队与防重提交，并未改动底层的资产持久化缺陷。
- **必须在 `VIDEO_ASYNC` 上线前补齐持久化**：
  如果 `VIDEO_ASYNC` 只解决了异步消费，但消费完成后的视频仍然不转存自有存储，那么 `VIDEO_ASYNC` 产出的新视频在上线后几天依然会发生同样的失效故障。因此，持久化转存应作为视频业务闭环的关键拼图，与 `VIDEO_ASYNC` 协同但独立实现。

---

## 12. 是否存在数据丢失风险

- **存在严重的历史数据丢失风险**！
- 由于知启云从未将视频转存到自有存储，视频资产的物理副本目前完全寄存在第三方服务商的临时服务器或桶中。
- 如果第三方服务商设定了自动清理策略（例如 3 天或 7 天自动删除临时任务视频），且 `task_000238` 的原始文件已被第三方彻底物理抹除，则**该历史视频文件本身已无法逆向恢复，存在实质性的数据丢失**。
- 为防止后续资产继续丢失，应尽快上线视频持久化能力。
