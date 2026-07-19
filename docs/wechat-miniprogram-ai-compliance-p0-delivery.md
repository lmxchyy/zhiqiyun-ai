# 微信小程序 AI 生图合规上线 P0 交付记录

日期：2026-07-19

## 1. 项目现状检查

### 1.1 路径、技术栈和启动方式

- 小程序：`apps/user-uni`，Vue 3 + TypeScript + uni-app，状态以 composables/本地缓存为主并包含 Pinia；所有业务请求统一从 `src/api/client.ts` 进入。执行 `npm.cmd --prefix apps/user-uni run typecheck` 检查类型，执行 `npm.cmd --prefix apps/user-uni run build:mp-weixin` 生成 `apps/user-uni/dist/build/mp-weixin`，再由微信开发者工具导入运行。
- 后端：`backend-go`，Go 1.25 + Gin，PostgreSQL + Redis + RabbitMQ + S3/MinIO。开发启动为 `npm.cmd run dev:api` 或 `go run ./cmd/api`；完整本地栈为 `docker compose -f compose.yml up -d --build`，API 地址 `http://127.0.0.1:3100`。
- 管理端：`admin-vue`，Vue 3 + Pinia + Axios + Element Plus；`npm.cmd --prefix admin-vue run build`。
- 检查开始时 Git 分支为 `main`，工作区无未提交修改。本次仅新增/修改本文列出的 P0 文件，未删除模块、未修改 Logo、未提交、未推送。

### 1.2 当前 AI 生图完整链路

`MiniProgramRoleWorkbench / creation API -> apps/user-uni API Client -> POST /api/v1/generation-tasks -> 登录、租户、套餐、并发和 Token 预留 -> 微信输入文本/上传图片安全检查 -> AI Capability 模型和参数 Schema -> generation task -> image Provider Router -> OpenAI-compatible Adapter / 配置渠道 -> 输出审核状态 -> 私有对象存储 -> asset/作品中心 -> Token capture；失败进入 FailGenerationTask 并按幂等标记 release/refund。`

任务、作品、账本和供应商请求号分别落在 `generation_tasks` / `xz_generation_tasks`、`xz_assets`、billing/wallet ledger、任务 params/raw 快照中。异步图片入口位于 `backend-go/internal/httpserver/api.go`，能力中心位于 `ai_capability.go`，Provider 接口位于 `internal/provider/image/provider.go`，对象存储位于 `generation_storage.go`，Token 预留/扣除/退款在 `store.go` 与 `postgres_store.go`。

### 1.3 当前模型、渠道和供应商

代码默认能力模型：

- 图片：`mock-standard`（Local）、`gpt-image-2`（NewAPI 标签）。
- 视频：`mock-video`、`seedance-fast-2.0`（NewAPI 标签）、`doubao-seedance-2.0`（移动云）；历史运行数据还包含 `grok-video-image`。
- PPT 文本：`kimi-k2.6`（NewAPI 标签）、`ppt-text-model`（Local）。

运行库当前渠道：`api.zmoapi.cn`、`code.lai1758.dpdns.org`、本地 `uni-api`、`sub.xlcsh.top`、APIMart、CMECloud、ModelScope、OpenAI 官方、本地 ComfyUI；其状态包含 ACTIVE、CONFIGURABLE 和 DISABLED。公开运行模型接口当前返回 `mock-standard,gpt-image-2,sora-2,grok-image-video,grok-video-1.5,doubao-seedance-2.0`。

这些渠道是网关或请求通道，不等于算法备案主体。特别是 NewAPI 只作为模型网关；P0 新字段要求另行填写真实 `provider_company`、算法名称和备案编号。当前没有任何模型填完这些资料，因此小程序模型接口按 fail-closed 返回 0 个模型，PC/Web 仍返回原模型。

### 1.4 原有合规能力检查

- 输入审核：已有微信 `msg_sec_check` 和 `img_sec_check`，只对小程序请求触发；未配置 AppID/Secret 时 fail-closed。P0 增加统一审核状态快照。
- 输出审核：原来没有正式统一输出审核；P0 增加状态机和 Mock 标识，Mock 不会在上线检查中被报告为“正式启用”。
- AI 内容标识：前端原已有 `AiGeneratedContentNotice`；P0 增加任务/作品隐式元数据和受控下载。SVG、JPEG、PNG 下载会生成带“AI生成”的衍生内容并记录原文件关联；其他未支持格式保持 fail-closed，不能返回无标识原图。
- 用户协议/隐私政策：登录页已有勾选与占位文案，但原来没有版本化发布记录。P0 增加版本表、接受记录表、公开读取和管理端读写 API；企业正式正文仍待录入。
- 投诉举报：原来没有完整入口。P0 增加统一“协议与安全”页、投诉/侵权入口字段和投诉数据表；实际 URL 待企业配置。
- 操作日志：已有 `insertAuditLog`、任务日志、账本生命周期和支付审计。P0 将终端、模型合规快照、输入/输出审核、AI 标识写入任务 params 和作品 metadata。
- 模型终端权限：原来没有模型级终端合规门；P0 已增加并在列表、调用和后台保存三处校验。

### 1.5 小程序四个首屏页面

- 首页：游客可浏览首页内容、案例和会员介绍，已有 AI 生成提示。
- 创作：已有图片生成/图生图、任务提交、参考图上传和登录保护；P0 由模型合规门决定实际可调用模型。视频、PPT、智能体等原页面保留，没有删除。
- 作品：已有作品列表、详情、任务进度、下载、删除、收藏、归档、再次创作；P0 下载增加输出审核与 AI 标识门。
- 我的：已有会员/Token、订单、设置等入口；新增“协议、AI规范与投诉”。
- 游客与登录：原有 `features/auth/gate.ts` 保存 pending action，`src/api/client.ts` 统一处理 401、刷新 Token 和登录跳转。本次保留并复用，没有在页面散写 `uni.request`。

## 2. P0 实施内容

- 扩展现有 `adminAIModel` 和管理端模型编辑器：技术提供方、技术主体、算法名称/备案号/类型、合同状态和到期时间、合规状态、允许终端/能力、小程序开关、备注、模型版本。
- 后台勾选小程序开关时立即强校验；不完整或过期配置不能保存。
- `/api/v1/public/models` 根据请求终端过滤；小程序只返回四项门槛全部满足且备案字段完整的模型。
- 任务提交和再次创作在后端再次校验模型，并把真实主体、算法、备案号和模型版本做不可变快照。伪造 `model_id/model` 无法绕过。
- 保留现有统一 Provider Adapter，未写死腾讯、百度或其他新供应商。
- 增加 input/output 审核状态机、Mock 输出审核、拒绝/人工复核状态、审核请求号/原因字段；失败任务复用现有幂等退款路径。
- 增加 AI 标识元数据、原始文件/下载衍生文件表和下载 fail-closed 逻辑。
- 增加协议版本、用户同意、投诉、内容审核、AI 标识配置和下载衍生物数据库表。
- 首次小程序生成前由后端校验三份必要协议的当前发布版本；缺少发布版本或用户未确认时返回 428，前端保留创作草稿并进入协议页，确认后返回原操作。
- 内容审核结果写入 `xz_content_audits`，管理端可查询待人工复核记录并将其复核为通过或拒绝。
- 增加只读上线检查 API，列出企业资料、可用模型、正式审核、显隐标识、协议、投诉、违规开启模型和 30 天内到期协议。
- 管理端模型列表展示“小程序合规”状态，编辑弹窗可维护全部合规字段。

## 3. 修改文件清单

- 后端：`backend-go/internal/httpserver/miniprogram_compliance.go`、`miniprogram_compliance_test.go`、`admin_types.go`、`ai_capability.go`、`api.go`、`asset_center_api.go`、`store.go`、`postgres_store.go`、`server.go`。
- 数据库：`database/migrations/060-miniprogram-ai-compliance-p0.sql`。
- 管理端：`admin-vue/src/App.vue`、`admin-vue/src/components/ai/AiCapabilityDomain.vue`。
- 小程序：`apps/user-uni/src/pages/user/ComplianceCenterPage.vue`、`UserSettingsPage.vue`、`pages.json`。
- 部署/配置：`.env.example`、`.env.production.example`、`compose.yml`、`compose.prod.yml`。

## 4. 数据库迁移

执行：`docker compose -f compose.yml run --rm migrate`。

新增/扩展：`ai_models` 合规字段；`generation_tasks`/`xz_generation_tasks` 审核与标识字段；`xz_content_audits`、`xz_ai_download_derivatives`、`xz_legal_documents`、`xz_user_agreement_acceptances`、`xz_complaints`、`xz_ai_label_settings`。所有新增开关默认关闭，不会自动放开旧模型。

## 5. 新增环境变量

`CONTENT_AUDIT_OUTPUT_MODE`、`ENTERPRISE_LEGAL_NAME`、五类协议的 `*_VERSION`/`*_CONTENT`、`COMPLAINT_ENTRY_URL`、`INFRINGEMENT_COMPLAINT_URL`。P0 即使误设 `CONTENT_AUDIT_OUTPUT_MODE=formal` 也会进入 `manual_review/formal-unconfigured` 并保持上线阻断；必须实现并验证正式 Provider Adapter 和健康检查后才能改变该门禁。

## 6. API 变更

- `GET /api/v1/public/models`：增加小程序终端合规过滤。
- `POST /api/v1/generation-tasks`、`POST /api/v1/generation-tasks/:id/retry`：增加终端、模型合规和审核快照强校验。
- `GET /api/v1/public/legal-documents`：公开已发布协议及投诉入口。
- `GET /api/v1/legal/acceptance-status`：查询当前用户对必要协议最新发布版本的确认状态。
- `POST /api/v1/legal/acceptances`：服务端原子记录当前已发布必要协议版本，客户端不能指定或伪造版本。
- `GET /api/v1/admin/compliance/miniprogram-launch-check`：只读上线检查。
- `GET /api/v1/admin/compliance/legal-documents`：协议版本列表。
- `PUT /api/v1/admin/compliance/legal-documents/:code`：新建/更新并发布协议。
- `GET /api/v1/admin/compliance/content-audits?status=manual_review`：查询待人工复核内容。
- `PATCH /api/v1/admin/compliance/content-audits/:id`：人工复核为 `approved` 或 `rejected`。
- `POST/PATCH /api/v1/admin/ai/models[/:id]`：支持合规字段；违规开启返回 400。
- `GET /api/v1/assets/:id/download`：输出未通过时拒绝；AI 结果不得返回无标识原件。

## 7. 管理端配置

进入“AI 能力中心 -> 模型管理 -> 编辑”。先填写真实技术主体、算法名称、备案编号、合同状态/到期日、合规状态、允许终端和允许能力，最后再勾选“小程序使用”。NewAPI 只能留在上游/网关字段，不能填入技术主体。

调用上线检查 API 确认 `blocked=false` 后才可提交微信审核。协议通过管理端 API 写入并将状态设为 `PUBLISHED`；后续可在现有管理端上补可视化协议编辑器。

## 8. 小程序测试步骤

1. 导入 `apps/user-uni/dist/build/mp-weixin`，清空登录态，确认首页可浏览。
2. 点击生成/上传/作品/购买，确认登录页出现；完成登录后确认 pending action 恢复。
3. 在模型未配置资质时请求模型列表，应为空；PC/Web 模型列表保持不变。
4. 管理端尝试只勾小程序开关，应保存失败；完整填写未来有效合同和备案资料后才能保存。
5. 修改请求中的模型为未合规模型，后端应返回 403。
6. 发布三份必要协议后首次生成应返回 428 并进入协议页；确认后返回创作页再次提交，服务端记录准确版本。更新任一协议版本后应再次要求确认。
7. 使用测试提示词 `[audit-reject]` 验证输出被拒绝；使用 `[audit-review]` 验证管理端人工复核列表可查询，不得展示或下载。
8. 对通过审核的 SVG/JPEG/PNG 作品下载，确认可见“AI生成”标识、响应包含原内容关联且数据库存在衍生记录；不支持格式应返回 503 而不是原图。
9. 打开“我的 -> 设置 -> 协议、AI规范与投诉”，核对五类协议和两个投诉入口。

## 9. 微信审核前人工材料清单

- 企业营业执照、主体名称、联系地址、客服电话/邮箱（本项目未虚构，当前待配置）。
- 每个开放模型的真实技术提供方、主体公司全称、算法名称、算法备案编号和公开查询证明。
- 有效合作协议、授权范围、终端范围、能力范围和到期时间。
- 正式输入/输出内容审核服务合同、接口证明、人工复核流程和应急联系人。
- 用户协议、隐私政策、AI 内容规范、平台公约、未成年人保护说明的法务定稿与版本号。
- 投诉举报和侵权投诉的受理页面、处理时限、联系人和留痕流程。
- AI 显式/隐式标识策略、下载样例、对象存储保留策略和审计留存周期。
- 微信小程序类目、隐私保护指引、用户生成内容/深度合成相关平台材料。

## 10. 尚未接通的外部服务与上线阻塞项

- 正式输出内容审核供应商尚未接通；当前是明确标记的 Mock 状态。
- SVG/JPEG/PNG 已支持同步生成显式标识下载版；WebP、GIF 和视频标识 worker 尚未接通，不支持格式继续拒绝下载原件。
- 企业主体、所有算法备案资料和合作协议尚未录入，所以小程序可用模型数为 0，这是预期阻断状态。
- 正式协议正文、版本号、投诉/侵权 URL 尚未配置。
- 协议确认 API 和首次生成前逐版本确认交互已实现；因正式法务正文未发布，仍需录入后进行微信端联调。
- 管理端人工复核查询/处理 API 已实现；专用可视化复核工作台尚未实现。
- 图片隐式标识目前保存业务元数据；尚未接通第三方不可见水印/内容凭证服务。

## 11. 风险与回滚

- 风险控制：所有旧模型默认 `miniprogram_enabled=false`；小程序筛选和调用均 fail-closed；PC/Web 不走小程序合规门。
- 代码回滚：恢复本次文件修改即可；不需要删除旧模块。
- 数据回滚：迁移只增表/增列，保留不会影响旧代码。若必须物理回滚，应先备份，再由 DBA 单独逐项处理；本次未提供破坏性 DROP 脚本。
- 临时运营回滚：将目标模型 `miniprogram_enabled=false` 或从 `allowed_terminals` 移除 `miniprogram`，无需影响 PC/Web。

## 12. 实际验证结果

- `go test ./...`：通过。
- P0 专项测试（过期合同、伪造模型、输出审核、SVG 与 raster AI 标识下载）：通过。
- `npm.cmd --prefix apps/user-uni run typecheck`：通过。
- `npm.cmd --prefix apps/user-uni run build:mp-weixin`：通过，总包 2.05 MB，主包 1.42 MB。
- `npm.cmd --prefix admin-vue run build`：通过。
- `docker compose -f compose.yml run --rm migrate`：通过；已确认新增表和 `ai_models` 合规列存在。
- `docker compose -f compose.yml up -d --build xianzhi-ai`：通过，容器 healthy。
- 实际接口：`/api/v1/health=ok`；Web 返回 6 个运行模型；小程序返回 0 个模型；公开协议返回 5 个待配置条目。
- `npm.cmd run test:pc-web:smoke`：5/5 通过，确认 PC/Web 游客、登录恢复和作品入口未破坏。
- `npm.cmd run test:user-h5:smoke`：mobile-h5 7/7 通过；desktop-h5 仅基础壳和公开刷新 2/7 通过，另外 5 个用例仍按移动访客 DOM 选择器断言桌面版页面而失败。独立 PC/Web 套件 5/5 通过，失败未指向本次 P0 代码，但该测试配置需后续拆分或修正。
- `npm.cmd run verify:api-boundaries`、`npm.cmd run verify:admin-navigation`：通过。
- `git diff --check`：无补丁错误，仅有 Windows LF/CRLF 提示。
