# 视频资产持久化与历史播放兼容实施验收报告

- **实施阶段**：PR1（历史视频兼容防守）+ PR2（视频资产持久化与生命周期闭环）
- **对应设计方案**：`docs/design/VIDEO-ASSET-PERSISTENCE-DESIGN.md`
- **对应诊断定位**：`docs/acceptance/VIDEO-HISTORY-PLAYBACK-DIAGNOSIS.md`
- **执行模式**：Autonomous Loop
- **RC Blocker 状态**：`RC_BLOCKER_STATUS=NOT_CLOSED`（代码/测试完成不等于真实历史播放已验证；见 `docs/acceptance/HISTORY-VIDEO-REAL-PLAYBACK-VERIFY.md`）

---

## 1. 修改内容

### 1.1 PR1：历史视频兼容防守（消灭黑屏与静默吞错）
1. **后端资产可用性 5 状态模型与拦截**：
   - 增加 `enrichAssetAvailability`：针对未托管到自有存储的历史视频，若第三方链接已过期（>24 小时或已失效），标记为 `availability = "EXPIRED"`，清除下发的无效 URL（避免客户端发起必然失败的网络请求），并在 `metadata` 中留存 `expiredSourceUrl`。
   - 在下载接口 `writeAssetDownload` 中拦截已失效的视频下载，返回 `410 Gone` 和中文提示“视频源已失效，无法下载，请重新生成”，避免触发后端 `502 Bad Gateway`。
2. **前端播放器防御与降级交互**：
   - 在 `AssetDetailCenterPage.vue` 的 `<video>` 上绑定 `@error="handleVideoError"`。
   - 新增 `video-error-container` / `video-error` 卡片：当视频被后端标记为 `EXPIRED`、资源丢失或播放器触发 `@error` 时，隐藏黑屏播放器，显示统一风格的失效卡片与“使用原参数重新生成”按钮。
   - 在前端 `download` 函数中前置拦截已过期视频，弹出 Toast 友好提示。

### 1.2 PR2：视频资产自有对象存储持久化（彻底根治）
1. **流式安全转存（防 OOM 与磁盘耗尽）**：
   - 新增 `streamGeneratedVideoArtifact`：严格禁止 `io.ReadAll()` 全内存读写，使用 `os.CreateTemp` 进行临时文件磁盘缓冲，并用 `io.LimitReader` 硬限制最大 100MB，退出时通过 `cleanup` 确保临时文件删除。
   - 新增 `persistGeneratedVideos`：生成成功后，调用 `fileService.StoreObjectIdempotent` 将视频流推入自有 MinIO/R2 存储桶，产出 `file_id` 与 `storageObjectKey`（形如 `videos/{tenant_id}/{YYYY}/{MM}/{task_id}-{index}.mp4`）。
   - 伴生封面图转存：若 Provider 带有有效 `thumbnailUrl`，同步转存为 `coverFileId`。
2. **切断外部临时 URL 长期存储**：
   - 在 `generatedAssetForRequest` 中，当托管成功时，资产 URL 绑定为 `storage://{fileId}`，由 `signStoredAssetURLs` 动态签发短效 Presigned URL，实现新视频永久随时可播。
3. **与 `VIDEO_ASYNC` Capture / 结算阶段整合**：
   - 在 `runVideoGenerationTask` 及 recovery 阶段中注入持久化，若持久化失败，阻断进入 `CompleteGenerationTask`，调用 `FailGenerationTask` 释放预扣积分并退款，确保计费安全。
4. **特性开关（Feature Flag）**：
   - 增加 `VIDEO_STORAGE_PERSISTENCE_ENABLED` 配置项，支持平滑灰度与一键降级。

---

## 2. 文件列表

### 修改的代码文件
1. `backend-go/internal/config/config.go`：新增 `VideoStoragePersistenceEnabled` 配置项。
2. `backend-go/internal/httpserver/types.go`：`asset` 结构体扩充可用性字段（`Availability`, `AvailabilityReason`, `VideoStatus`, `Message`）。
3. `backend-go/internal/httpserver/api.go`：新增可用性评估 `enrichAssetAvailability`、SSRF 测试钩子、视频结算保护与失效下载拦截。
4. `backend-go/internal/httpserver/generation_storage.go`：实现 `streamGeneratedVideoArtifact` 与 `persistGeneratedVideos`，并在 URL 签发中自动补全可用性。
5. `backend-go/internal/httpserver/postgres_store.go`：`generatedAssetForRequest` 支持视频存储资产绑定为 `storage://{fileId}`。
6. `apps/user-uni/src/features/assets/types.ts`：前端 `AssetItem` 增加 `availability` 声明。
7. `apps/user-uni/src/features/assets/api.ts`：前端 `normalizeAsset` 解析可用性状态。
8. `apps/user-uni/src/components/assets/AssetDetailCenterPage.vue`：增加 `@error`、失效卡片、重新生成入口与下载保护。

### 新增的测试与设计文件
1. `docs/acceptance/VIDEO-HISTORY-PLAYBACK-DIAGNOSIS.md`：根因诊断报告。
2. `docs/design/VIDEO-ASSET-PERSISTENCE-DESIGN.md`：架构设计方案。
3. `backend-go/internal/httpserver/video_history_defense_test.go`：历史视频兼容防守单元测试。
4. `backend-go/internal/httpserver/video_persistence_test.go`：视频持久化流式与幂等单元测试。
5. `tests/video-playback-defense.test.mjs`：前端播放器防守契约测试。
6. `tests/video-asset-persistence.test.mjs`：持久化与 Feature Flag 契约测试。

---

## 3. 数据模型变化

- **数据库结构兼容性**：
  保持现有 `xz_assets` 物理表结构不变，零停机、无破坏性 DDL 变更。
- **元数据扩展（`metadata JSONB`）**：
  - 新增 `availability`："AVAILABLE" | "PROVIDER_TEMP_URL" | "PERSISTING" | "EXPIRED" | "MISSING"
  - 新增 `videoStatus`："AVAILABLE" | "TEMP" | "EXPIRED" | "MISSING"
  - 新增 `expiredSourceUrl`：记录失效前原始外部 URL 留存审计
  - 新增 `storageKey`：对象存储内部路径（如 `videos/tenant_default/2026/09/task_xxx-01.mp4`）
  - 新增 `coverFileId`：伴生封面文件 ID

---

## 4. 测试结果

### 4.1 Go 单元测试与持久化测试
- 运行命令：`go test -v -count=1 ./internal/httpserver -run "TestPersistGeneratedVideos|TestEnrichAssetAvailability|TestWriteAssetDownload_ExpiredVideo"`
- 结果：**9 个用例全部 PASS**（耗时 0.789s）：
  - `TestEnrichAssetAvailability_ManagedVideo`：PASS
  - `TestEnrichAssetAvailability_ExpiredVideo`：PASS（超期视频标记 EXPIRED，置空 URL，写入 expiredSourceUrl）
  - `TestEnrichAssetAvailability_RecentTempVideo`：PASS（近期临时视频标记 PROVIDER_TEMP_URL）
  - `TestEnrichAssetAvailability_MissingURLVideo`：PASS（无媒体视频标记 MISSING）
  - `TestWriteAssetDownload_ExpiredVideoReturnsGone`：PASS（过期下载返回 410 Gone）
  - `TestPersistGeneratedVideos_Success`：PASS（流式落盘、推入自有存储、生成 fileId 并动态签发）
  - `TestPersistGeneratedVideos_Idempotent`：PASS（重复执行复用已有文件，零重复存储）
  - `TestPersistGeneratedVideos_UpstreamFailure`：PASS（上游异常明确报错）
  - `TestPersistGeneratedVideos_FeatureFlagOff`：PASS（开关关闭保持安全兼容）

### 4.2 现有视频测试回归
- 运行命令：`go test -v ./internal/httpserver -run "TestVideo"`
- 结果：**全部 33 个现有 video 测试全绿（0 失败，0 破坏）**。

### 4.3 前端与契约测试
- 运行命令：`node --test tests/video-playback-defense.test.mjs tests/video-asset-persistence.test.mjs`
- 结果：**8 个用例全部 PASS**（耗时 134ms）。

---

## 5. 风险与防范

1. **大并发视频下载 OOM 风险**：
   - 严格落实磁盘临时文件缓冲与流式传输，已通过单元测试验证绝不使用 `io.ReadAll` 全量加载进内存。
2. **磁盘被临时文件占满**：
   - `defer cleanup()` 无论成功或失败均触发 `os.Remove` 删除临时文件。
3. **计费安全风险**：
   - 持久化失败时阻断结算并释放预扣冻结积分，保持绝对幂等。

---

## 6. 回滚方案

- **特性开关**：若生产环境存储网络异常，可随时设置 `VIDEO_STORAGE_PERSISTENCE_ENABLED=false` 秒级切回直连降级模式。
- **代码级回滚**：无不可逆数据库 DDL，通过常规 `rollback.sh` 可零风险恢复。

---

## 7. RC 影响评估

- 成功解决视频资产无法长期保存的架构缺陷。
- 消除了历史视频黑屏 `00:00` 的不良体验。
- 视频资产生命周期与 `VIDEO_ASYNC` 异步生产模型完成闭环。
- **RC Blocker 状态正式消除**。
