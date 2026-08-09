后续默认技术标准：
• SaaS 后台：Vue 3 + Pinia + Axios + Element Plus
• 小程序：Vue 3 + TypeScript + uni-app
• 共享状态优先 composables + 本地缓存，复杂后再用 Pinia
• 请求统一走 API Client，页面禁止直接散写 uni.request
后续开发和代码评审默认按此执行，除非你明确调整。

企业 Connector 约束：
• 飞书、后续钉钉和企业微信必须通过统一 Connector 抽象接入，禁止把平台 SDK 类型写入生图、视频或 PPT 核心业务。
• 外部消息只能通过 connector_key 解析企业，禁止接受消息体传入的 tenant_id 或 enterprise_id 作为租户依据。
• 外部消息 ID、Connector 任务 ID 和既有生成任务 client_request_id 必须形成幂等链，禁止重复生成和重复扣费。
• AppSecret、VerificationToken、EncryptKey 只能加密入库，日志与前端响应不得输出明文。
• 生图、改图、视频、PPT 必须统一经过 AICommand、CapabilityHandler、Connector 任务、既有业务服务、作品中心和账单链路；禁止在平台适配器里另写模型调用。
• 生成成功但飞书投递失败必须标记 delivery_failed；重投只能重发已有私有作品或短期签名链接，不得再次调用模型或扣费。
• 会话上下文至少按 connector_id + chat_id + external_user_id 隔离，上一张图片、上一任务和作品链接不得跨成员或跨企业读取。

微信虚拟支付约束：
• 虚拟商品统一使用微信虚拟支付。
• 金额和权益只能由服务端商品配置决定，客户端不得上传或决定金额、会员天数、credits、Token 或图片额度。
• 前端不得直接修改会员、credits、Token、图片额度或钱包余额。
• 权益发放必须具备数据库事务、独立账本和幂等保护。
• 支付密钥只能通过环境变量或 Secret 注入，禁止写入源码、YAML、小程序包或 Git。
• 小程序请求统一使用项目 API Client，页面禁止直接调用 uni.request，且不得引入 Axios。
• 微信支付回调、官方查单补偿和后台人工补发必须共用统一的 GrantOrderEntitlements 服务。

网页端防回归约束（平台首页 / AI 生图 / 侧边栏点数）：
• `/points/account` 的 `total` / `totalGranted` 必须是终身口径（可用 + 冻结 + 已消耗），禁止再写成 `summary.Total = available + frozen`，否则冻结为 0 时侧边栏会显示「可用 = 总额」。
• 侧边栏可用/总额计算必须走 `admin-vue/src/utils/sidebarPlanPoints.ts`；禁止在 `App.vue` 内重新散写一套点数换算。
• `/user/dashboard` 默认 `taskLimit`/`assetLimit` ≤ 30；`/user/online-image` 默认 ≤ 40；禁止把首屏默认值改回 120/300 或更大而不改产品需求与回归测试。
• 用户首页与 AI 生图/无线画布/作品/视频工作台必须保持即时壳（`usesInstantWorkspace`），首屏请求参数必须走 `admin-vue/src/utils/userWorkspaceLoad.ts` 的 `moduleListQuery`。
• 改动上述接口或侧边栏点数时，必须同步跑通并保持绿：
  - `backend-go`：`go test ./internal/httpserver/ -run 'TestPointAccountTotalIncludesConsumedPoints|TestUserDashboardDefaultPayloadStaysSmallAndExposesTotalPoints|TestUserOnlineImageDefaultListLimitsStayCapped'`
  - `admin-vue`：`npm test -- tests/webWorkspacePointsRegression.spec.ts`
• 生产机禁止长期保留未提交热修；发布只允许 Git 已推送提交，dirty tree 时不得强行绕过 `deploy.sh`。
