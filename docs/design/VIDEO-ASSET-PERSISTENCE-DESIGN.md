# 视频资产持久化与历史播放兼容架构设计方案

- **文档状态**：APPROVED（进入 Autonomous Loop 执行）
- **对应缺陷**：`docs/acceptance/VIDEO-HISTORY-PLAYBACK-DIAGNOSIS.md`
- **定位评级**：`RC-Blocker`（数据资产生命周期不完整，生成成功 ≠ 资产可持久访问）
- **核心目标**：
  1. **短期降级防护（PR1）**：解决历史超期视频在前端黑屏并显示 `00:00` 的体验缺陷，提供明确的失效状态与“重新生成”入口。
  2. **长期彻底根治（PR2）**：补齐视频自有对象存储持久化链路（`persistGeneratedVideos`），实现视频物理资产与图片同等水准的永久可用性，并与 `VIDEO_ASYNC` 的 Capture/结算阶段对齐。

---

## 1. 当前视频资产问题剖析

### 1.1 现状调用链的断裂点
知启云系统的图片生成（Image）与视频生成（Video）在资产管理上存在严重不对称：

```
【图片链路 (已闭环)】
Model Provider ──> 生成图片 ──> 后端下载流 ──> 存入自有 MinIO/R2 ──> 产出 fileId ──> 写入 xz_assets (storageManaged=true)
                                                                            │
客户端请求作品 ────────────────── signStoredAssetURLs (动态签发900s预签名URL) ◄───┘ (永久可用)

【视频链路 (严重断裂)】
Model Provider ──> 生成视频 ──> 提取外部临时URL ──> 直接写入 xz_assets.url (无 fileId / storageManaged=false)
                                                                            │
客户端请求作品 ────────────────── signStoredAssetURLs (无 fileId, 跳过签发) ◄───┘
                                       │
                                  原样下发外部临时URL
                                       │
                               几天后第三方签名过期 (403/404)
                                       │
                              前端播放器黑屏，duration 显示 00:00 (UI 静默吞错)
```

### 1.2 核心症结清单
1. **作品库没有真实物理资产**：数据库 `xz_assets.url` 存放的仅仅是上游服务商（如 `newapi.zs-kjhn.cn` 或上游 S3/OSS）生成的**短期临时播放凭证**，而非知启云自持的数字化资产。
2. **时效性必定暴雷**：第三方预签名链接的 TTL 通常在 1 小时至 24 小时之间，临时文件通常在 3~7 天内被自动清理。因此视频在生成当天可播，几天后必然群体性失效。
3. **前端缺少防御性设计**：`AssetDetailCenterPage.vue` 的 `<video>` 组件未监听 `@error`，未实现失败占位与降级引导，真实 video 错误被完全吞掉。
4. **视频下载连带瘫痪**：`/api/v1/assets/:id/download` 会通过 `http.Get(item.URL)` 抓取原片，遇到超期链接直接向上游抛出 `502 Bad Gateway`。

---

## 2. 资产可用性（Asset Availability）状态模型

为避免简单粗暴的“无 fileId + 24小时 = expired”导致的误判，设计严密的 5 状态可用性模型：

```
                    ┌─────────────────────────┐
                    │      任务完成生成资产     │
                    └────────────┬────────────┘
                                 │
           ┌─────────────────────┴─────────────────────┐
           ▼                                           ▼
   【自有存储已托管】                            【未落入自有存储】
   (fileId != "")                               (fileId == "")
           │                                           │
           ▼                                           ├─────────────────────────┐
     ┌───────────┐                                     ▼                         ▼
     │ AVAILABLE │                             ┌───────────────┐           ┌───────────┐
     └───────────┘                             │ 无有效播放URL │           │ 有外部URL │
                                               └───────┬───────┘           └─────┬─────┘
                                                       │                         │
                                                       ▼                         ▼
                                                 ┌───────────┐            只读探活 / TTL判定
                                                 │  MISSING  │                   │
                                                 └───────────┘         ┌─────────┴─────────┐
                                                                       ▼                   ▼
                                                                  [探活成功/近期]       [探活403/404/超期]
                                                               ┌───────────────────┐ ┌───────────┐
                                                               │ PROVIDER_TEMP_URL │ │  EXPIRED  │
                                                               └─────────┬─────────┘ └───────────┘
                                                                         │
                                                                 触发异步转存修复
                                                                         │
                                                                         ▼
                                                                  ┌────────────┐
                                                                  │ PERSISTING │
                                                                  └────────────┘
```

### 2.1 状态枚举定义
| 状态值 | 描述 | 前端展示行为 | 播放行为 |
| :--- | :--- | :--- | :--- |
| `AVAILABLE` | 资产已在自有 MinIO/R2 托管，已生成动态 presigned URL | 正常展示作品 | 原生 `<video>` 流畅播放 |
| `PROVIDER_TEMP_URL` | 未落自有存储，但外部第三方链接经验证尚可访问（处于临时生命周期内） | 提示“临时资源，后台同步转存中” | 允许临时播放，后台触发转存 |
| `PERSISTING` | 正在从 Provider 执行大视频流式下载并推入自有存储中 | 显示“视频资产正在归档”加载态 | 等待转存完成或先走临时流 |
| `EXPIRED` | 第三方签名已过期（403）或文件已被清理（404），且无自有备份 | 隐藏黑屏播放器，显示失效卡片 | 禁用播放，引导用户“再次生成” |
| `MISSING` | 资产记录存在，但没有任何可用 URL 或物理对象不存在 | 显示“资产数据缺失” | 禁用播放 |

### 2.2 严禁规则
- **禁止**仅凭 `fileId为空 + 超过24小时` 静态一刀切判定为 `EXPIRED`；必须结合 URL 签名特征、只读 HEAD 探活、或上游网关特定规则。
- 若资产有本地有效路径或自有 storage 引用，状态必须为 `AVAILABLE`。

---

## 3. Artifact 架构层设计

建立明确的 `Generation -> Artifact -> Asset` 三层架构模型：

```
  +--------------------+
  |  Generation Task   |  (业务意图层: prompt, model, points, billing, user, task_id)
  +---------+----------+
            |
            v
  +--------------------+
  |      Artifact      |  (物理产物层: provider output, bytes stream, file_id, hash, mime)
  +---------+----------+
            |
            v
  +--------------------+
  |       Asset        |  (用户资产层: xz_assets, title, display, tags, availability)
  +--------------------+
```

### 3.1 Artifact 核心职责
1. **Provider Output Manifest**：保存上游模型返回的原始输出契约（包含 `providerTaskId`、原始 `videoUrl`、`width`、`height`、`duration` 等）。
2. **Storage Capture 承载物**：
   - 负责从 Provider 外部链接通过流式读写抓取物理字节流。
   - 负责写入底层对象存储（MinIO/R2），产出 `file_id`、`storage_key`、`sha256`、`file_size`。
3. **隔离底层协议与用户资产**：即使将来视频 Provider 更换（从 Grok 切换为 Seedance 或 Sora），上层 Asset 结构保持不变。

---

## 4. 视频资产持久化方案（PR2 核心）

视频文件相比图片有明显差异：体积大（10MB ~ 100MB）、下载耗时长、包含伴生封面图（Poster）。因此不能直接把全部字节读入内存，必须采用**流式分块传输与磁盘缓冲**。

### 4.1 核心流程设计

```
[Provider 执行成功] (返回外部 videoUrl 与可选的 thumbnailUrl)
         │
         ▼
[调用 persistGeneratedVideos]
         │
         ├─ 1. 幂等性检查 (FindActiveBusinessFile 检查 taskID 对应文件是否已在库)
         │      └─ 已存在直接复用 fileId，防止重试重复落库
         │
         ├─ 2. 视频流式落盘缓存 (Temp File Buffering)
         │      └─ 使用 os.CreateTemp 创建临时文件，LimitReader 限制最大 100MB
         │      └─ 避免多并发时 100MB []byte 导致 Go 堆内存 OOM
         │
         ├─ 3. 视频上传至自有存储 (MinIO/R2)
         │      └─ 使用 fileService.StoreObjectIdempotent 流式推入 S3/R2
         │      └─ BusinessType: "generation_result", BusinessID: taskID
         │      └─ 产出 videoFileId, storageObjectKey
         │
         ├─ 4. 伴生封面图转存 (Thumbnail / Poster)
         │      └─ 若 Provider 返回了 thumbnailUrl，同步转存至对象存储
         │      └─ 产出 coverFileId (供列表首屏快速渲染)
         │
         └─ 5. 组装 generated_storage_files 注入 req.Params
                └─ 注入 fileId, coverFileId, storageManaged: true
```

### 4.2 视频流式读取函数实现规范
在 `generation_storage.go` 中新增安全流式读取器：

```go
func streamGeneratedVideoArtifact(ctx context.Context, rawURL string) (*os.File, int64, string, func(), error) {
    remoteURL, err := validateRemoteDownloadURL(rawURL)
    if err != nil {
        return nil, 0, "", nil, err
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL.String(), nil)
    if err != nil {
        return nil, 0, "", nil, err
    }
    res, err := remoteDownloadHTTPClient().Do(req)
    if err != nil {
        return nil, 0, "", nil, err
    }
    defer res.Body.Close()
    if res.StatusCode < 200 || res.StatusCode >= 300 {
        return nil, 0, "", nil, fmt.Errorf("video upstream returned %d", res.StatusCode)
    }

    tempFile, err := os.CreateTemp("", "video-persist-*")
    if err != nil {
        return nil, 0, "", nil, fmt.Errorf("create temp video file: %w", err)
    }
    cleanup := func() {
        _ = tempFile.Close()
        _ = os.Remove(tempFile.Name())
    }

    written, err := io.Copy(tempFile, io.LimitReader(res.Body, maxGeneratedVideoBytes+1))
    if err != nil {
        cleanup()
        return nil, 0, "", nil, fmt.Errorf("buffer video stream: %w", err)
    }
    if written > maxGeneratedVideoBytes {
        cleanup()
        return nil, 0, "", nil, errors.New("video file exceeds maximum allowed size (100MB)")
    }
    if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
        cleanup()
        return nil, 0, "", nil, err
    }

    contentType := res.Header.Get("Content-Type")
    if !strings.Contains(strings.ToLower(contentType), "video") {
        contentType = "video/mp4"
    }
    return tempFile, written, contentType, cleanup, nil
}
```

---

## 5. Artifact / file_id / storage_key 规范

### 5.1 命名规范
- **S3 Bucket**：复用配置中的 `${S3_BUCKET}`（如 `xianzhi-assets`）
- **Object Key 规范**：
  ```
  videos/{tenant_id}/{YYYY}/{MM}/{task_id}-{index}.mp4
  ```
  示例：`videos/tenant_default/2026/09/task_000238-01.mp4`
- **伴生封面 Object Key 规范**：
  ```
  videos/{tenant_id}/{YYYY}/{MM}/{task_id}-{index}-cover.jpg
  ```

### 5.2 资产元数据结构（`xz_assets.metadata` JSONB）
持久化成功后，`xz_assets` 的 `metadata` 必须固化以下字段：
```json
{
  "fileId": "file_000982",
  "storageFileId": "file_000982",
  "storageTenantId": "tenant_default",
  "storageProvider": "minio",
  "storageBucket": "xianzhi-assets",
  "storageObjectKey": "videos/tenant_default/2026/09/task_000238-01.mp4",
  "coverFileId": "file_000983",
  "storageManaged": true,
  "fileSize": 18452190,
  "fileSizeBytes": 18452190,
  "contentType": "video/mp4",
  "sourceUrl": "https://provider-temp-url.example/video.mp4",
  "duration": 5,
  "resolution": "1920x1080",
  "width": 1920,
  "height": 1080
}
```

---

## 6. 历史视频资产修复策略（`video_asset_repair_job`）

针对已经存在但未落自有存储的历史视频记录，设计后台修复作业：

### 6.1 作业流程设计
```
[扫描任务] 遍历 xz_assets 表: media_type = 'video' AND (metadata->>'fileId' IS NULL OR metadata->>'fileId' = '')
    │
    ▼
[只读可用性判断]
    ├─ A. 检查当前 url 是否有效:
    │     发送 HTTP HEAD / Range 请求测试响应码 (200 / 206)
    │
    ├─ B. 若当前 url 失效 (403/404):
    │     根据 task_id 查 xz_generation_tasks 提取 provider_task_id
    │     尝试向上游 Provider 发起只读查询 (Query Task) 获取是否刷新了 URL
    │
    ▼
[分类处置]
    ├─ 情况 1: URL 仍存活 / 成功刷新新 URL
    │     执行 streamGeneratedVideoArtifact 下载并转存至自有存储
    │     更新 xz_assets: metadata 补充 fileId、storageManaged: true
    │     修复成功！
    │
    └─ 情况 2: 上游资源已永久删除 (404/410/Token 无效且上游无任务记录)
          标记 metadata["availability"] = "EXPIRED"
          记录审计日志: repair_failed, reason: "upstream_resource_purged"
          前端将直接呈现友好降级卡片，消灭黑屏 00:00
```

---

## 7. 前端播放器防御与 UI 改造（`AssetDetailCenterPage.vue`）

1. **绑定 `@error` 事件**：
   ```vue
   <video
     v-if="asset.type === 'video' && asset.remoteUrl && !videoPlaybackFailed"
     class="video-preview"
     :src="asset.remoteUrl"
     controls
     :autoplay="autoplay"
     @error="handleVideoError"
   />
   ```
2. **新增失效占位卡片（彻底消灭黑屏 00:00）**：
   ```vue
   <view v-if="asset.type === 'video' && (videoPlaybackFailed || asset.availability === 'EXPIRED')" class="video-expired-card">
     <text class="expired-symbol">⚠️</text>
     <text class="expired-title">视频资源已过期</text>
     <text class="expired-desc">早期生成的临时视频文件已超过云端保留期限。</text>
     <button class="expired-regenerate-btn" @click="regenerate">使用原参数再次生成</button>
   </view>
   ```
3. **下载按钮安全拦截**：
   若检测到资源处于 `EXPIRED` 状态，点击“保存到相册”直接弹出 Toast 提示“资源已过期无法下载，请重新生成”，避免触发无效的后端 502 请求。

---

## 8. 失败补偿与重试策略

- **计费安全铁律**：若 Provider 已成功返回且模型扣费已发生，**严禁**因为转存失败直接给用户退款或将任务标记为 `FAILED` 并重退积分。
- **降级容错**：
  1. 任务正常进入 `SUCCEEDED`。
  2. 若 `persistGeneratedVideos` 出现临时性错误（如超时），将第三方原始 URL 临时降级写入，并在 `metadata` 记录 `persistStatus = "PENDING_RETRY"`。
  3. 后台守护协程在 1 小时内针对 `PENDING_RETRY` 的视频资产重新执行后台转存，拉取成功后再补写 `fileId`。

---

## 9. 与 VIDEO_ASYNC Capture 阶段整合方案

在 `RunGenerationVideoCanaryWorker` -> `runVideoGenerationTask` 中：
```
[Worker 消费 RabbitMQ 消息]
           │
           ▼
[调用 PrepareVideoTask / 轮询 Provider 得到 SUCCEEDED]
           │
           ▼
[执行 persistGeneratedVideos] ◄─── 新增注入点
     ├─ 成功: 产出 fileId，注入 Params
     └─ 降级: 记录告警日志，保全原始 URL
           │
           ▼
[调用 a.store.CompleteGenerationTask] (原子结算点)
     ├─ 更新 xz_generation_tasks 状态为 SUCCEEDED
     ├─ 结算扣减预扣积分 (Capture Points)
     ├─ 写入 xz_assets (带 fileId)
     └─ 提交 Inbox/Outbox 事务
```
- **绝不在事务锁内下载大视频**：持久化在 `CompleteGenerationTask` 事务开启前完成，确保数据库事务极短（毫秒级），杜绝数据库行锁持有时间过长。

---

## 10. 特性开关（Feature Flag）与回滚

引入配置项：
```bash
VIDEO_STORAGE_PERSISTENCE_ENABLED=false # 默认安全关闭，测试与灰度时开启
```
- `false`：沿用原有的快速流式降级模式。
- `true`：执行全链路自有对象存储持久化。
- 回滚：纯配置热切换，零破坏性数据库 DDL，秒级生效。
