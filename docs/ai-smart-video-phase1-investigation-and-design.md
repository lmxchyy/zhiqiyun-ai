# AI 智能成片第一阶段调查与设计

## 1. 工程现状与可复用模块

真实目录：

- Go 后端：`backend-go`，入口为 `backend-go/cmd/api/main.go`，HTTP 路由集中在 `backend-go/internal/httpserver/server.go`。
- 电脑端 SaaS 管理端：`admin-vue`，Vue 3 + Pinia + Axios + Element Plus，统一请求入口为 `admin-vue/src/api/client.ts`。
- 用户端、小程序与 App 共用端：`apps/user-uni`，Vue 3 + TypeScript + uni-app，统一请求入口为 `apps/user-uni/src/api/client.ts`。
- 共享前端包：`packages/api-client`、`packages/shared-auth`、`packages/shared-types` 等。
- 数据库迁移：`database/migrations`，按递增序号执行 additive SQL；回滚脚本位于 `database/rollbacks`。

可复用能力：

- 文生视频、图生视频：`backend-go/internal/app/generation/service.go`、`backend-go/internal/provider/video`、`backend-go/internal/httpserver/api.go` 的 `createGenerationTask` 和 `runVideoGenerationTask`。
- 统一生成任务：`backend-go/internal/httpserver/types.go` 的 `generationTask`，持久化和计费事务位于 `backend-go/internal/httpserver/postgres_store.go`。
- 作品中心：`backend-go/internal/httpserver/asset_center_api.go`、`recent_works_api.go`，路由为 `/api/v1/assets`、`/api/v1/works/recent`。
- 文件上传和对象存储：`backend-go/internal/storage`、`backend-go/internal/httpserver/file_center_api.go`、迁移 `046-storage-file-center.sql`。文件中心保存 `file_id`、`object_key`，通过短期签名 URL 访问。
- Redis 和异步能力：`backend-go/internal/infra/clients.go`；可靠队列样例为 `backend-go/internal/httpserver/connector_queue.go` 的 pending/working Redis list；当前普通视频生成仍使用进程内 goroutine。
- 模型调用封装：`backend-go/internal/provider/{chat,image,video}` 与 `backend-go/internal/app/generation`，动态模型/渠道路由位于 `backend-go/internal/httpserver/api.go`。
- Token 冻结、确认扣费、释放：`backend-go/internal/httpserver/postgres_store.go` 的 generation reserve/capture/release 事务；状态与账本类型在 `billing_v1_types.go`。
- 内容审核：`backend-go/internal/httpserver/wechat_content_security.go`、`miniprogram_compliance.go`，生成入口已执行输入审核、输出审核和审计记录。
- 租户、用户和权限：`auth_api.go`、`user_rbac.go`、`user_rbac_store.go`、`enterprise_api.go`；文件中心的 `identity` 是可复用的用户/租户上下文解析方式。
- 任务进度：生成任务提供 GET 查询和前端轮询；RAG 单独提供流式 run 接口。未发现统一 WebSocket 任务总线。
- 日志和请求追踪：`server.go` 注入并回传 `X-Request-Id`，PostgreSQL 模式启用 audit middleware；后台 worker 使用标准库 `log`。

调查期间在源码、Go 依赖、Docker 编排中未发现 FFmpeg、FFprobe、Remotion 或独立视频渲染服务的实际集成。现有“视频生成”是上游模型生成，不是多素材剪辑合成。

## 2. 需求差距

- 现有生成任务只表达一次模型请求，缺少“项目—素材—脚本版本—分镜—渲染任务”的长期编辑模型。
- 文件中心有视频/图片对象，但没有项目内引用、排序和素材分析元数据。
- 没有素材探测、镜头切分、音频/字幕分析和结构化脚本生成流程。
- 没有可恢复的多阶段渲染队列、worker 心跳、租约和渲染服务协议。
- 现有视频生成 goroutine 不适合长时、可恢复的合成任务。
- 现有作品中心能承接最终视频，但需增加来源项目、来源版本和渲染任务关联元数据。
- 计费已有可靠事务，但智能成片需要“分析预估 + 渲染预估 + 实际确认”的复合计价规则。
- 小程序上传能力受包体、内存、后台存活时间和大文件限制，不能承担完整时间线编辑和本地渲染。

## 3. 推荐模块边界

`internal/app/smartvideo` 作为平台无关领域：

- Project Service：草稿、版本、确认和删除规则。
- Asset Service：只管理文件引用、顺序和分析状态，不上传或删除底层对象。
- Storyboard Service：结构化脚本与分镜，调用模型只能经过既有 provider/generation 抽象。
- Render Orchestrator：只负责编排、幂等、状态、租约、错误和补偿。
- Renderer Port：FFmpeg/云渲染/Remotion 等实现放在 provider/worker 边界，领域层不感知 SDK。
- Billing Port：桥接现有 generation wallet ledger，不建立第二套钱包。
- Work Center Publisher：成功后用既有资产写入路径创建视频作品。

HTTP 层只做认证、租户上下文、参数解析和错误映射；对象存储仍由文件中心负责。

## 4. 数据表

第一阶段新增：

- `video_projects`：租户、用户、标题、文字要求、领域状态、当前版本、最终作品、活跃渲染任务、错误码与软删除。
- `video_project_assets`：项目、文件中心 `file_id`、`storage_key`、素材类型、顺序、强类型素材元数据。删除该记录不删除文件对象。
- `video_project_versions`：不可变版本号、文字要求、强类型 script JSON、storyboard snapshot JSON、确认状态。
- `video_storyboard_scenes`：场次序号、旁白、视觉要求、时长、素材 ID 数组和强类型转场 JSON。
- `video_render_tasks`：版本、幂等键、状态、进度、强类型渲染规格、Token 生命周期字段、输出文件/作品、错误码和时间。

JSON 字段均限定顶层类型，Go 侧均有具体 struct；不以无约束 `map[string]any` 作为领域合同。

## 5. API 建议

第一阶段已定义：

- `GET /api/v1/video-projects`
- `POST /api/v1/video-projects`
- `GET /api/v1/video-projects/:id`
- `PATCH /api/v1/video-projects/:id`
- `DELETE /api/v1/video-projects/:id`
- `GET /api/v1/video-projects/:id/assets`
- `POST /api/v1/video-projects/:id/assets`
- `PUT /api/v1/video-projects/:id/assets/order`
- `DELETE /api/v1/video-projects/:id/assets/:assetId`
- `POST /api/v1/video-projects/:id/render-tasks`

上传继续调用 `/api/v1/files/upload/init` 和 `/api/v1/files/upload/complete`，业务类型建议为 `smart_video_source`，完成后再把 `fileId` 绑定到项目。

下一阶段建议：

- `POST /video-projects/:id/analysis-tasks`
- `GET /video-projects/:id/versions`
- `POST /video-projects/:id/versions/:versionId/confirm`
- `GET/PATCH /video-projects/:id/storyboard-scenes`
- `GET /video-projects/:id/render-tasks/:taskId`
- `POST /video-projects/:id/render-tasks/:taskId/cancel`

分析与渲染任务的创建接口均必须要求 `Idempotency-Key`，并保存请求摘要；相同键、不同请求体必须冲突。

## 6. 异步状态机

项目：

`DRAFT -> ANALYZING -> STORYBOARD_READY -> CONFIRMED -> RENDERING -> COMPLETED`

分析或渲染失败进入 `FAILED`，用户修改后回到 `DRAFT`；渲染中的项目不能删除。

渲染任务：

`CREATED -> QUEUED -> RUNNING -> UPLOADING -> SUCCEEDED`

`CREATED/QUEUED/RUNNING -> CANCELLED`；非终态可进入 `FAILED`。终态不可逆。所有失败写 `error_code`、安全的 `error_message`，详细上下文写带 request ID/task ID 的服务端日志。

生产 worker 应复用 Redis pending/working 可靠队列模式，并增加数据库租约、heartbeat、attempt、max_attempts、run_after；不能沿用单进程 goroutine 作为最终方案。

## 7. Token 流程

1. 分镜确认后，服务端按素材时长、输出时长、分辨率、模型分析量和渲染档位计算报价，客户端不能决定单价。
2. 创建渲染任务的数据库事务内写任务并冻结 `quoted_tokens`，幂等键同时覆盖任务和账本。
3. worker 领取后不再次冻结。
4. 成功上传且作品中心写入成功后，以实际量确认扣费：冻结额内 capture；少用部分 release；如规则允许超额，必须走显式补冻结或失败，不能形成负余额。
5. 分析/渲染/上传/作品写入失败时 release；同一失败重试不能重复释放。
6. 成功后投递或前端通知失败只重试通知，不重新渲染、不重复扣费。

第一阶段只保留强类型 Port 和任务 Token 字段，尚未把新任务接入真实钱包事务，避免伪造已完成计费。

## 8. 作品中心关联

渲染输出先进入 `xz_file_objects`，保存私有 `file_id/object_key`；随后通过既有资产创建链路生成视频作品。作品 metadata 建议记录：

- `source=smart_video`
- `videoProjectId`
- `videoProjectVersionId`
- `videoRenderTaskId`
- `outputFileId`

`video_projects.output_asset_id` 只保存作品 ID。项目/素材引用删除不得永久删除 `xz_file_objects`；实际文件回收由文件中心 `reference_count` 和回收策略决定。

## 9. 电脑端与小程序边界

电脑端：

- 多文件拖拽、大素材管理、脚本/分镜编辑、时间线预览、版本比较和渲染参数。
- 通过 `admin-vue/src/api/client.ts` 或用户 Web 端共享 API Client 接口，不散写请求。

小程序端：

- 新建草稿、少量素材选择/上传、文字要求、脚本与分镜确认、任务状态查看、作品播放。
- 不实现复杂时间线和本地渲染；大文件上传需分片/断点续传能力成熟后再开放。
- 继续使用 `apps/user-uni/src/api/client.ts`，页面禁止直接 `uni.request`，不引入 Axios。

## 10. 分阶段计划与风险

1. 第一阶段：本文调查、五表迁移、领域模型、Repository/Service/HTTP 骨架、草稿和素材 CRUD、状态校验、租户隔离与幂等测试。
2. 第二阶段：素材探测 worker（FFprobe）、内容审核、分析任务、结构化脚本/分镜、版本确认和分析计费。
3. 第三阶段：独立渲染 worker、Redis 可靠队列、租约/重试/取消、FFmpeg 或云渲染 PoC、真实冻结/确认/释放。
4. 第四阶段：作品中心发布、电脑端完整编辑器、小程序轻量确认流、监控告警和容量治理。

主要风险：

- 大文件上传、跨区域对象存储带宽和临时磁盘容量。
- 编解码器许可、字体/音乐版权、硬件加速和不同素材格式兼容。
- 长任务的进程重启恢复、重复消费、取消时资源清理和进度准确性。
- AI 分析 JSON 稳定性，需要严格 schema 校验和版本迁移。
- 内容审核需要覆盖输入文本、原素材、脚本、分镜和最终视频。
- 计费预估与实际资源消耗偏差。
- 最终资产写入成功但状态/计费更新失败的分布式一致性，应采用 outbox/补偿任务。
