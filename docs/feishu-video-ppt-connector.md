# 飞书视频与 PPT Connector 接入交付说明

## 1. 实施前架构分析

1. 消息入口：`POST /api/open/connectors/feishu/events/:connectorKey`；只由随机 `connectorKey` 解析企业，消息体不能指定租户。
2. 生图链路：事件校验 → 入站消息幂等 → Redis/本地队列 → 用户绑定 → 意图 → `CreatePendingGenerationTask` → 模型中心 → 私有存储 → `xz_assets` → Capture → 飞书投递。
3. 改图上下文：按当前 Connector、群/单聊和成员查找最后一张成功图片；参考作品查询同时约束企业、Connector、绑定和 chat，禁止跨用户/跨企业。
4. 账号绑定：`connector_user_bindings` 关联飞书用户和企业内部用户；未绑定用户按企业默认组织创建隔离影子成员，管理员可改绑真实账号。
5. 消息幂等：`connector_messages(platform, external_message_id)`、`connector_ai_tasks(platform, external_message_id)` 和业务 `client_request_id=feishu:<message_id>` 构成幂等链。
6. 视频入口：复用 `generation.Service.PrepareVideoTask`，不在 Connector 内实现供应商调用。
7. 视频任务：复用 `xz_generation_tasks` 的 `QUEUED/PROCESSING/SUCCEEDED/FAILED`、Provider 轮询/超时和账单状态映射。
8. 视频作品：下载供应商结果后写入文件中心 PRIVATE 对象，再由现有 `CompleteGenerationTask` 创建作品和结算。
9. PPT 入口：复用 `internal/app/ppt.Service.GenerateWithConcurrency`、现有大纲模型和 PPT 能力配置。
10. PPT 渲染导出：复用现有大纲、内容、自动配图、`buildPPTX` 与模板/主题流程。
11. PPT 作品：PPTX 先写入 PRIVATE 文件中心，再写入 `xz_assets`；飞书只获得文件或短期签名链接。
12. 账单：图片/视频沿用统一生成任务 Reserve/Capture/Release；飞书 PPT 新增兼容的统一生成账单任务，在 PPT 引擎执行前 Reserve，作品落库后 Capture，任一步失败 Release。
13. 统一状态：Connector 使用 `created/queued/validating/reserved/processing/rendering/uploading/completed/delivery_failed/failed/cancelled/refunded`，不暴露供应商状态。
14. 存储：复用文件中心 `StoreObject` 与 `AccessURL`；PRIVATE 文件按当前内部用户签发短期地址。
15. 飞书发送：适配器统一支持文本、图片、文件和互动卡片；单文件上限 100 MiB，并有 HTTP 超时和有限重试。
16. 复用服务：企业 RBAC、Connector 密钥加密、用户绑定、模型中心、生成任务、Billing、PPT 引擎、文件中心、作品中心、审计日志和 API Client。
17. 主要改动：统一命令/Handler、意图路由、视频/PPT 适配服务、会话上下文、任务字段与迁移、SendFile、管理页配置/筛选/重投、测试和文档。
18. 风险：生产模型和存储必须可达；飞书文件权限需随应用版本发布；大文件会降级为短期链接；卡片 action 目前只预留数据，未开放回调；迁移必须先于新 API 容器启动。

## 2. 统一路由

```text
FeishuEventHandler
  -> MessageNormalizer
  -> RuleIntentRouter
  -> CapabilityRouter
  -> CapabilityHandler
  -> ConnectorTask
  -> Existing Image/Video/PPT Service
  -> Private Asset Service
  -> Existing Billing Service
  -> FeishuResultSender
```

`AICommand` 统一携带企业、内部/外部用户、Connector、会话、外部消息 ID、原文、意图、参数、参考作品和会话上下文。Handler 接口包含 `CanHandle`、`Validate`、`EstimateCost`、`Execute`、`QueryStatus`、`BuildResult`。图片原功能已经适配到同一入口，没有保留第二套飞书业务分支。

## 3. 视频流程

1. 从企业配置填充默认模型、时长、比例和分辨率；解析用户显式参数。
2. 校验企业/Connector/成员/群聊权限、图生视频开关、最大时长、最大分辨率和单次/日/月积分。
3. 图生视频只读取当前成员当前会话的最后图片；没有可用图片时明确提示。
4. 通过既有模型中心估价并创建 `xz_generation_tasks`，冻结企业积分。
5. 调用现有 `PrepareVideoTask`，完成后下载受控结果并保存 PRIVATE 文件与作品。
6. 成功 Capture；失败由 `FailGenerationTask` Release。
7. 小于 100 MiB 时尝试上传飞书文件；同时发送包含参数、模型、积分和短期作品地址的完成卡片。
8. 投递失败只更新 Connector 任务为 `delivery_failed`；后台“重发作品”重新签发链接并发卡片，不调用模型、不扣费。

## 4. PPT 流程

1. 从企业配置填充默认页数、模板、主题、语言、企业 Logo/知识库开关；校验最大页数和成员限制。
2. 复用 PPT 能力模型与价格配置估价，创建幂等账单生成任务并 Reserve。
3. 复用现有大纲、内容、配图、渲染和 `buildPPTX` 导出流程。
4. PPTX 写入 PRIVATE 文件中心并进入 `xz_assets` 后 Capture；失败 Release。
5. 小文件上传飞书为 PPT 文件；完成卡片包含主题、页数、模板、模型、文件名、大小、积分和短期下载地址。

## 5. 权限、额度与上下文

- 企业视频：启用、默认模型/时长/比例/分辨率、图生视频、最大时长/分辨率、单次/日/月积分、deny/allow/approval。
- 企业 PPT：启用、默认模板/页数/最大页数、Logo/知识库、单次/日/月积分、deny/allow/approval。
- 成员：`videoGenerate`、`pptGenerate`、视频/PPT 日月额度、`maxVideoDuration`、`maxPptPages`；复用 permission JSON 避免重复表字段。
- 上下文：`connector_session_contexts` 保存最后意图、任务、作品、主题、参数和提示词，唯一键为企业 + Connector + chat + 外部用户，默认 24 小时过期。

## 6. 数据库迁移

执行 `database/migrations/062-feishu-video-ppt-capabilities.sql`：

- 扩展 `connector_ai_tasks` 的统一状态、内部阶段、投递状态/次数、预计积分和完成投递时间。
- 新建 `connector_session_contexts`。
- 为 `xz_ppt_tasks.client_request_id` 增加幂等字段和唯一索引。
- 为已有飞书成员补充视频/PPT 权限键，不修改现有图片权限。

迁移是幂等 SQL，生产必须先运行 migrate，再启动使用新字段的 API。

## 7. 测试与部署

本地：

```powershell
cd backend-go
go test ./internal/connector/... ./internal/httpserver/... -count=1
npm.cmd --prefix ../admin-vue run build
```

生产：

```bash
cd /opt/zhiqiyun-ai
git fetch gitee main
git pull --ff-only gitee main
docker compose --env-file .env.production -f compose.prod.yml run --rm migrate
docker compose --env-file .env.production -f compose.prod.yml up -d --build
```

部署后依次验证健康检查、迁移版本、后台飞书配置页、机器人生图/视频/PPT、任务查询、同一消息幂等、失败 Release 和后台重投。

## 8. 故障排查

- “没有权限”：检查企业能力开关、permission mode、成员 permission JSON 和企业角色权限。
- “上一张图片不存在”：确认图片由同一 Connector、同一 chat、同一飞书成员成功生成，且会话未过期。
- “余额不足/额度已用完”：检查企业算力余额、单次/日/月上限和成员覆盖值。
- “生成成功但未收到文件”：在任务记录筛选 `delivery_failed`，点击“重发作品”；该操作不会重新生成或扣费。
- “文件上传失败”：确认飞书应用已发布上传文件权限；大于 100 MiB 的文件应使用短期下载链接。
- “PPT 超时”：检查大纲模型、图片模型、对象存储和 PPT 配置；内部错误不会原样返回飞书。
