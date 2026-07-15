# user-uni 与 Go 后端接口对照审计

审计日期：2026-07-12

审计范围：

- 小程序/用户端：`apps/user-uni/src`
- 共享请求层：`packages/business-sdk`、`packages/shared-auth`、`packages/api-client`
- Go 后端路由：`backend-go/internal/httpserver/server.go`
- 本地运行服务：`http://127.0.0.1:3100`

## 1. 结论

从 URL 和 HTTP 方法层面看，当前前端直接引用的接口没有发现后端路由缺失：

- 初次审计时 Go 服务注册路由 238 条；本轮新增 12 条用户/代理商/运营中心专用路由，当前源码共 250 条。
- 初次审计时 user-uni 与共享 SDK 共识别出 51 个 `/api/v1/*` 路径引用。
- 初次静态路由匹配结果为未匹配路径 0 条；本轮新增的前端路径均与同步新增的 12 条后端路由一一对应。
- 对普通用户、代理商、运营中心执行 32 个只读接口探测：除缺少必填 `module_code` 的裸 `/module-schema` 返回预期 400 外，其余均返回 200；补充使用 `image_generation`、`video_generation` 参数后均返回 200。

真正的问题集中在以下三类：

1. 页面有功能，但后端没有对应的专用查询或写入接口。
2. 后端已有接口，但前端仍依赖聚合接口、旧接口或本地配置。
3. 详情页通过“拉取全部列表后在前端查找”实现，数据量增加后会明显变慢，也难以做可靠权限校验和分页。

### 1.1 本轮已落地（2026-07-12）

- 普通用户：`GET /member/orders`、`GET /member/orders/:id`、`GET /assets/:id`、`GET /user/usage/:id`。
- 代理商：`GET /channel/orders/:id`、`GET /channel/commissions/:id`、`GET /channel/withdrawals/:id`、`GET /channel/children/:id`、`GET /channel/invite-records`。
- 运营中心：`GET /operation-center/agents/:id`、`GET /operation-center/orders/:id`、`GET /operation-center/commissions/:id`。
- 对应小程序详情页已改为按 ID 请求，不再先拉取全量列表；邀请记录已改用独立接口并区分注册、成交和升级。
- 新增接口均按当前登录用户、代理商可见客户范围或运营中心归属范围过滤，跨账号/跨中心请求返回 404 或 403。

验证结果：新增授权测试通过，`go test ./...`、`vue-tsc --noEmit` 和 `uni build -p mp-weixin` 均通过。

## 2. 前端功能存在，但缺少专用后端接口

| 优先级 | 前端功能/页面 | 当前实现 | 建议补充接口 |
| --- | --- | --- | --- |
| P0 | 用户 API 设置 | 暂缓：`GET /api/v1/user/api-settings` 按现有安全边界只返回能力、模型和配额，不暴露原始密钥 | 先确定加密存储、密钥脱敏、连通性测试和权限审计方案，再设计写接口 |
| 已完成 | 用户订单详情 | 已使用专用列表和详情接口 | `GET /api/v1/member/orders`、`GET /api/v1/member/orders/:id` |
| 已完成 | 用户作品详情 | 已按当前用户和作品 ID 查询 | `GET /api/v1/assets/:id` |
| 已完成 | 用户消耗详情 | 已按当前用户和计费事件 ID 查询 | `GET /api/v1/user/usage/:id` |
| P1 | 用户退款记录 | 当前只有 `POST /member/refund-requests`，无法独立查询申请列表和审核进度 | `GET /api/v1/member/refund-requests`、`GET /api/v1/member/refund-requests/:id` |
| 已完成 | 代理商订单详情 | 已按代理商可见客户范围和订单 ID 查询 | `GET /api/v1/channel/orders/:id` |
| 已完成 | 代理商分润详情 | 已按当前代理商和分润 ID 查询 | `GET /api/v1/channel/commissions/:id` |
| 已完成 | 代理商提现详情 | 已按当前代理商和提现 ID 查询 | `GET /api/v1/channel/withdrawals/:id` |
| 已完成 | 代理商团队成员详情 | 已限制为当前代理商直属下级 | `GET /api/v1/channel/children/:id` |
| 已完成 | 代理商邀请记录 | 已区分注册、成交和升级，并返回汇总 | `GET /api/v1/channel/invite-records` |
| P1 | 推广二维码 | `/channel/me` 只返回邀请链接，缺少小程序码生成和场景参数接口 | `POST /api/v1/channel/promotion-codes` 或 `GET /api/v1/channel/promotion-code` |
| 已完成 | 运营中心代理商详情 | 已按当前运营中心归属查询 | `GET /api/v1/operation-center/agents/:id` |
| 已完成 | 运营中心订单详情 | 已按当前运营中心归属查询 | `GET /api/v1/operation-center/orders/:id` |
| 已完成 | 运营中心分润详情 | 已按当前运营中心收款归属查询 | `GET /api/v1/operation-center/commissions/:id` |
| P2 | 电子凭证下载 | `/member/invoices` 只有列表，页面没有真正的凭证文件下载能力 | `GET /api/v1/member/invoices/:id`、`GET /api/v1/member/invoices/:id/download` |
| P2 | 提现收款账户 | 前端固定显示“微信零钱”，提交参数只有 `amountCents` | 增加收款方式查询/绑定接口，并在提现申请中传入 `payoutAccountId` |

## 3. 后端已有，但 user-uni 没有充分使用

| 后端接口 | 当前前端状态 | 建议 |
| --- | --- | --- |
| `GET /api/v1/channel/customers` | 代理商工作台主要从 `/channel/me` 读取全部客户 | 客户列表页改用专用接口，保留 `/channel/me` 只做概览 |
| `GET /api/v1/channel/usage` | 代理商工作台从 `/channel/me.usageEvents` 获取消费记录 | 消费明细页改为专用接口并支持分页 |
| `GET /api/v1/member/token-records` | 多数页面从 `/member/wallet` 合并记录 | 点数流水页直接使用该接口 |
| `GET /api/v1/agent/profile` | 代理商身份和概览主要依赖 `/channel/me` | 角色身份初始化使用 `/agent/profile`，业务列表按需加载 |
| `GET /api/v1/user/online-image` | user-uni 未直接读取 | 生图配置应从后端读取，不继续依赖页面本地配置 |
| `PATCH /api/v1/user/ai-state` | user-uni 未调用 | 保存用户默认模型、模式和最近参数时接入 |
| `GET /api/v1/generation-tasks/:id` | 普通生成任务主要轮询整个任务列表 | 任务详情和轮询改为按 ID 查询 |
| PPT outline、PDF、单页重生成、图片搜索接口 | 小程序主要只接了生成、任务、历史、PPTX 导出 | PPT 二级编辑页逐步接入现有接口 |
| 知识库、文档、标签、分类接口 | 小程序只接智能体和会话 | 若移动端需要知识库管理，再增加对应页面；否则保持 PC/后台专属 |
| OfficeCLI 文档接口 | 小程序未接入 | 仅在移动端确有文档生成需求时开放入口 |

## 4. 运行时接口验证

以下接口已使用三种角色的实际登录令牌执行只读验证：

- 普通用户：登录态、模型、套餐、会员资料、钱包、发票、点数账户、首页、API 设置、用量、任务、作品、知识智能体、会话、PPT 历史和模型、四个页面配置。
- 代理商：中心概览、客户、订单、客户消耗、分润、提现、代理商资料。
- 运营中心：中心资料、代理商、订单、分润。

主要响应结构符合前端当前适配方式：

- `/member/wallet`：`account`、`orders`、`tokenRecords`、`transactions`
- `/channel/me`：`agent`、`promotion`、`summary`、`customers`、`orders`、`commissions`、`usageEvents`、`withdrawals`、`children`
- `/operation-center/*`：资料返回 `operationCenter/summary`，列表返回 `items`
- `/user/usage`：`summary/items`
- `/knowledge-agents`、`/knowledge-conversations`：`items/nextCursor`
- `/app/pages/{home,studio,assets,profile}`：均返回 200，素材位数量分别为 24、7、5、3

### 4.1 本轮部署后复核

已重建 3100 端口的 `xianzhi-ai` 应用容器，健康检查返回 `ok`。使用现有三种角色数据验证：

- 普通用户订单详情、作品详情、消耗详情均返回对应真实 ID。
- 代理商订单、分润、提现详情及邀请记录均返回真实数据。
- 当前运行库中代理商直属下级、运营中心下属代理商/订单/分润为空；空数据详情路由返回预期 404，接口授权与有数据场景由自动化测试覆盖。

## 5. 额外风险

### 5.1 聚合接口过重

本次本地探测中 `/channel/me` 约 1002ms，而拆分后的订单、分润、提现接口通常为十几到几十毫秒。代理商多个页面继续依赖 `/channel/me` 会重复加载客户、订单、分润、用量和团队全量数据。

### 5.2 生产环境地址回退

`apps/user-uni/src/api/client.ts` 在微信小程序环境中默认回退到 `http://127.0.0.1:3100`。生产构建必须确保 `VITE_API_BASE_URL` 注入正式 HTTPS 域名，否则真机无法访问本机地址。

### 5.3 字符编码复核

使用 UTF-8 重新读取业务源码后，未发现此前终端显示中的所谓源码乱码；该现象来自 PowerShell 默认编码显示。当前 TypeScript 检查和微信小程序编译均已通过，不需要批量替换源码字符。

## 6. 推荐实施顺序

1. P1：代理商列表页面改用 `/channel/customers`、`/channel/usage` 等拆分接口，继续减轻 `/channel/me` 聚合接口负担。
2. P1：接入微信 `getUnlimited` 等正式能力，补推广小程序码接口、scene 规则、缓存和失效策略。
3. P2：补退款查询、发票下载、提现收款账户能力。
4. 安全专项：确定用户自有 API 密钥是否允许配置；若允许，先完成服务端加密、脱敏展示、测试代理和审计日志，再开放写接口。
5. 部署后使用三种角色重新执行真实 HTTP 验证，并在微信开发者工具中逐页点击详情入口。
