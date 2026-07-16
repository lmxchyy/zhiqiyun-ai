后续默认技术标准：
• SaaS 后台：Vue 3 + Pinia + Axios + Element Plus
• 小程序：Vue 3 + TypeScript + uni-app
• 共享状态优先 composables + 本地缓存，复杂后再用 Pinia
• 请求统一走 API Client，页面禁止直接散写 uni.request
后续开发和代码评审默认按此执行，除非你明确调整。

企业 Connector 约束：
• 飞书、后续钉钉和企业微信必须通过统一 Connector 抽象接入，禁止把平台 SDK 类型写入 AI 生图核心业务。
• 外部消息只能通过 connector_key 解析企业，禁止接受消息体传入的 tenant_id 或 enterprise_id 作为租户依据。
• 外部消息 ID、Connector 任务 ID 和既有生成任务 client_request_id 必须形成幂等链，禁止重复生成和重复扣费。
• AppSecret、VerificationToken、EncryptKey 只能加密入库，日志与前端响应不得输出明文。

微信虚拟支付约束：
• 虚拟商品统一使用微信虚拟支付。
• 金额和权益只能由服务端商品配置决定，客户端不得上传或决定金额、会员天数、credits、Token 或图片额度。
• 前端不得直接修改会员、credits、Token、图片额度或钱包余额。
• 权益发放必须具备数据库事务、独立账本和幂等保护。
• 支付密钥只能通过环境变量或 Secret 注入，禁止写入源码、YAML、小程序包或 Git。
• 小程序请求统一使用项目 API Client，页面禁止直接调用 uni.request，且不得引入 Axios。
• 微信支付回调、官方查单补偿和后台人工补发必须共用统一的 GrantOrderEntitlements 服务。
