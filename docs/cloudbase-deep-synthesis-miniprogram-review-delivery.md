# CloudBase 第三方深度合成资质接入与小程序审核版交付记录

> 更新说明（2026-07-22）：本文中的 CloudBase 强制路由方案已废止。小程序合规模型有 `channel_id` 时优先使用绑定通道，未绑定时按模型支持情况和通道优先级自动选择；CloudBase Adapter 保留为可选通道，不再是统一前置条件。

日期：2026-07-19

## 1. 项目检查结论

- 小程序位于 `apps/user-uni`，技术栈为 Vue 3 + TypeScript + uni-app，请求统一由 `src/api/client.ts` 发出。`npm.cmd run typecheck` 做类型检查，`npm.cmd run build:mp-weixin` 生成微信包。
- Go 后端位于 `backend-go`，技术栈为 Go + Gin；项目实际数据库是 PostgreSQL（不是 MySQL），并使用 Redis、RabbitMQ 和 S3/MinIO 兼容对象存储。可执行 `go run ./cmd/api`，完整本地栈使用 `docker compose -f compose.yml up -d --build`。
- 管理端位于 `admin-vue`，技术栈为 Vue 3 + Pinia + Axios + Element Plus，使用 `npm.cmd run build` 构建。
- 当前生图链路是：小程序统一 API Client -> `POST /api/v1/generation-tasks` -> 身份/租户/套餐/并发/Token 预留 -> 输入审核 -> 模型配置与小程序合规门 -> generation task -> image Provider -> 输出审核 -> 对象存储 -> 作品中心 -> Token capture；失败进入原有幂等 release/refund。
- 模型配置位于现有 AI 能力中心与 `ai_models`；渠道位于管理端渠道配置和 `configuredGenerationChannels`；任务、计费退款、对象存储分别位于 `api.go`、`store.go`/`postgres_store.go`、`generation_storage.go`。NewAPI/OpenAI-compatible 地址只作为网关，不能作为算法备案主体。
- P0 已具备：小程序输入审核、明确标为 Mock 的输出审核状态机、显式 AI 标识下载衍生物、隐式元数据、版本化协议/接受记录、投诉入口、任务/账本/操作审计、模型终端权限。正式输出审核供应商、真实企业资料、正式协议和投诉 URL 仍未配置，所以上线检查继续阻断。
- 检查开始时工作区已有未提交修改，包括上一阶段 P0 文件、`admin-vue/src/utils/aiImageDb.ts` 和 `artifacts/admin-*.png`。本次没有清理、覆盖或提交这些用户修改，没有删除旧模块，没有修改 Logo。

完整 P0 盘点见 `docs/wechat-miniprogram-ai-compliance-p0-delivery.md`。

## 2. 官方文档核对与技术选择

已按 2026-07-19 可访问的腾讯云开发官方文档核对：

- [第三方 AI 能力资质申请](https://docs.cloudbase.net/ai/release/algorithm-filing)：深度合成类目要求非个人主体；CloudBase 属于第三方技术提供场景；需准备技术提供方算法备案证明及包含算法名称、场景、备案编号的合作协议材料，CloudBase 环境需完成企业认证且有效期满足官方要求。材料获取和微信公众平台上传均是人工操作，本项目没有代为登录、接受协议或提交审核。
- [图片模型概览](https://docs.cloudbase.net/ai/image-model/overview)：采用当前文生图 `HY-Image-3.0-Plus-4090-Tob-v1.0` 和图生图 `HY-Image-v3.0-I2I-ToB-v1.0.1`；未采用已经下线的旧模型 ID。返回 URL 为临时地址，Go 后端会立即下载到现有对象存储。
- [Node SDK](https://docs.cloudbase.net/ai/image-model/node-sdk) 与 [图生图](https://docs.cloudbase.net/ai/image-model/image-to-image)：云函数使用当前 SDK 版本下限；图生图只接受一张 HTTPS 参考图，Go 和云函数两侧都做校验。
- [自定义水印](https://docs.cloudbase.net/ai/image-model/custom-watermark)：云函数强制设置 `footnote`，客户端不能关闭；平台原有下载标识服务仍会生成受控下载版本，形成双层显式标识。
- [云函数 HTTP API](https://docs.cloudbase.net/cloud-function/function-calls/http-api)、[API Key](https://docs.cloudbase.net/http-api/basic/apikey) 和 [环境变量](https://docs.cloudbase.net/cloud-function/function-configuration/env)：Go 后端通过官方网关调用云函数，服务端凭证只从环境变量注入，不进入小程序包、源码、响应或日志。

选择的链路为：`小程序 -> 知启云 Go API -> 原任务/钱包/审核/合规路由 -> CloudBase HTTP 云函数 -> CloudBase AI 图片模型 -> Go 立即持久化 -> 输出审核/标识 -> 作品/结算`。CloudBase 没有替换现有后端、数据库、任务中心、钱包或作品中心。

## 3. 实施内容与文件清单

- Provider：`backend-go/internal/provider/image/cloudbase_function.go`、`cloudbase_function_test.go`。复用现有 image Provider 形态，严格允许当前两个官方模型、官方 HTTPS 网关、服务端 Bearer 凭证、500 字提示词和单张图生图参考图。
- 路由与审计：`backend-go/internal/httpserver/api.go`、`generation_storage.go`、`store.go`、`postgres_store.go`、`internal/app/generation/service.go`。小程序合规模型优先使用显式绑定的 `channel_id`；未绑定时从支持该模型的可用通道中按优先级选择。保存 provider task ID、实际模型和供应商元数据。
- 合规门与上线检查：`backend-go/internal/httpserver/miniprogram_compliance.go`、`server.go`。模型仍必须同时满足 approved、miniprogram、image、开关开启、协议有效和完整备案字段；NewAPI 主体会被拒绝。上线检查新增 CloudBase Adapter 和 CloudBase 合规模型检查。
- 渠道管理：`backend-go/internal/httpserver/api_channel_probe.go`、`admin-vue/src/App.vue`。增加 `cloudbase-function` 协议配置；测试按钮只检查配置，不产生图片费用，也不会伪装为已验证资质。
- 云函数：`cloudbase/functions/zhiqiyun-ai-image/index.js`、`package.json`、`README.md`。固定当前模型白名单、水印、revise 和输入约束，不含任何真实凭证。
- 审核版终端收口：`apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`、`v531/V531HomePage.vue`、`v531/V531StudioPage.vue`、`scripts/patch-mp-native-login.cjs`。由公开终端配置接口过滤入口，默认仅 image/infographic；原生生成包兼容补丁保持相同门禁，模块保留，PC/Web 不受影响。
- 配置：`.env.example`、`.env.production.example`、`compose.yml`、`compose.prod.yml`。
- 数据库：复用 `database/migrations/060-miniprogram-ai-compliance-p0.sql` 的合规、审核、标识、协议和审计字段，本次 CloudBase Adapter 不新增表，不自动插入或启用模型。

## 4. 环境变量

Go 服务：

- `CLOUDBASE_ENABLED=false`
- `CLOUDBASE_ENV_ID`
- `CLOUDBASE_IMAGE_FUNCTION_NAME=zhiqiyun-ai-image`
- `CLOUDBASE_IMAGE_FUNCTION_URL`（可选；为空时由环境 ID 和函数名构造官方 URL）
- `CLOUDBASE_API_KEY`（仅 Secret 注入）
- `CLOUDBASE_IMAGE_TIMEOUT_MS=150000`
- `CLOUDBASE_AI_WATERMARK_TEXT=AI生成`
- `MINIPROGRAM_CREATION_MODES=image,infographic`

CloudBase 云函数：`ENV_ID`、`AI_WATERMARK_TEXT`。示例文件只有变量名和说明，没有真实密钥。

## 5. API 变更

- `GET /api/v1/public/terminal-capabilities`：返回当前终端允许的创作模式，小程序前端 fail-closed。
- `POST /api/v1/generation-tasks`：小程序执行创作模式、模型合规和技术通道可用性校验；伪造 model ID 仍会被拒绝，模型无需强制绑定技术通道。
- `GET /api/v1/admin/compliance/miniprogram-launch-check`：检查合规模型是否存在、是否绑定可用技术通道，以及是否存在已启用但不可路由的模型。
- 管理端渠道协议新增 `cloudbase-function`。其“测试”只做非计费配置校验，文案明确未调用模型、未验证资质材料。

## 6. 管理端配置顺序

1. 在 CloudBase 控制台由有权人员完成企业认证、资源开通、协议确认和资质材料获取；本项目未执行。
2. 人工部署 `cloudbase/functions/zhiqiyun-ai-image`，配置 900 秒函数超时和环境变量，再把 API Key 作为后端 Secret 注入。
3. 管理端“AI 能力中心 -> 渠道”选择 `cloudbase-function`，只填写官方函数 URL/环境变量名和当前官方模型。
4. “AI 能力中心 -> 模型管理”录入真实 `Provider=CloudBase`、技术主体公司全称、算法名称、备案号、模型版本、合同有效期、image 能力、miniprogram 终端、approved 和小程序开关。
5. 运行上线检查。任何企业资料、协议、审核、标识、投诉、CloudBase Adapter 或合规模型检查失败，都不得提交审核。

不得把 new-api 填为 `provider_company`，不得把渠道连通等同于算法资质通过。

## 7. 小程序测试步骤

1. 不登录打开首页，确认可浏览；首页/创作页只显示服务端允许的图片类能力。
2. 点击生成或上传参考图，确认统一登录门触发；登录后草稿和原动作恢复。
3. 未配置真实资质时，小程序模型列表为空且生成被拒绝；PC/Web 原模型和页面保持可用。
4. 伪造未 approved、协议过期、非 image、非 miniprogram 或 NewAPI 主体的 model ID，确认后端拒绝且未调用 CloudBase。
5. 资质和 Secret 由有权人员配置后，分别验证文生图和单图图生图；检查任务 provider task ID、备案快照、作品和 Token 账本。
6. 输入审核拒绝时确认无 Provider 请求；输出审核拒绝时确认结果不可见、不可下载并按原幂等路径退款。
7. 下载通过审核的图片，确认页面“本内容由人工智能生成”和文件“AI生成”标识；普通下载接口不能返回原图。
8. 将合同到期时间改为过去，确认模型立即从小程序列表消失且任务提交被拒绝。

## 8. 微信审核前人工材料与阻塞项

- 企业主体证明、CloudBase 企业认证和剩余有效期证明。
- CloudBase 提供的真实算法备案截图/材料；算法名称、应用场景、备案编号必须与后台模型记录及合作协议一致。
- 双方真实合作协议及有效期。不得把协议文件提交 Git。
- 微信公众平台深度合成类目的人工选择、材料上传和最终审核提交。
- 正式输入/输出内容审核服务、健康检查、人工复核和应急处置证明。
- 法务定稿的用户协议、隐私政策、AI 内容规范、平台公约、未成年人保护说明及真实投诉/侵权入口。
- 下载标识样例、隐式标识方案、数据留存与审计说明。

当前未接通/未执行：真实 CloudBase 环境和 API Key、云函数部署、真实模型调用、CloudBase 资质材料获取、正式输出审核服务、正式协议和投诉页面、微信审核提交。因此小程序模型数量保持 0、上线检查保持 blocked 是预期安全状态。

## 9. 风险与回滚

- `CLOUDBASE_ENABLED` 默认 false；数据库不存在自动开启逻辑；即使环境配置完整，也必须有真实合规模型记录才能调用。
- 紧急停用只需关闭 `CLOUDBASE_ENABLED`，或取消模型 `miniprogram_enabled`/移除 miniprogram 终端；PC/Web 不受影响。
- 删除 CloudBase 运行时配置不会破坏现有 Provider、任务、钱包或作品。数据库迁移只增列/增表，如需物理回滚必须由 DBA 备份后逐项操作，本次不提供破坏性脚本。
- CloudBase 返回 URL 有时效，Go 会同步持久化；若存储失败，任务进入原失败/退款路径，不把临时 URL 当正式作品。

## 10. 验证记录

- `go test ./internal/provider/image ./internal/httpserver ./internal/app/generation`：通过。
- `npm.cmd run typecheck`（`apps/user-uni`）：通过。
- `npm.cmd run build`（`admin-vue`）：通过。
- `npm.cmd run build:mp-weixin`（`apps/user-uni`）：通过；主包 1.43 MB，总包 2.06 MB。
- `node --check cloudbase/functions/zhiqiyun-ai-image/index.js`：通过。
- `docker compose -f compose.yml config --quiet`：通过。
- `npm.cmd run verify:api-boundaries`：通过，没有新增页面直连请求。
- `npm.cmd run verify:admin-navigation`：未通过；检查器报告现有 `Customer360Center`、`OrderFulfillmentCenter` 尚未在 `App.vue` 接入，与 CloudBase/小程序改动无关，未擅自扩展本次范围修复。
- `git diff --check`：通过，仅输出 Windows LF/CRLF 提示。
- 未调用真实 CloudBase 模型，未产生云资源费用，未登录平台，未接受协议，未提交审核，未提交或推送 Git。
