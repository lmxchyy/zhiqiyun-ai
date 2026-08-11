后续默认技术标准：
• SaaS 后台：Vue 3 + Pinia + Axios + Element Plus
• 小程序：Vue 3 + TypeScript + uni-app
• 共享状态优先 composables + 本地缓存，复杂后再用 Pinia
• 请求统一走 API Client，页面禁止直接散写 uni.request
后续开发和代码评审默认按此执行，除非你明确调整。

防回归：
• 主提示：`.ai/CodexPrompt.md`（只做当前事、别整页重写、做完确认旧功能还在并勾选保护面）
• 不能丢的功能清单：`docs/regression/protected-surfaces.md`（含网页侧边栏点数、首页/生图首屏、小程序首页模板、登录落用户首页、游客浏览入口、视频模型/参数/预估积分/Grok Preview 套餐与通道/中文错误、视频下载强制可分享 mp4、自由P图首页主能力文案与全页、小程序视频灵感临时下架、作品列表等）
• 改页面遵守：`.ai/前端工人CodexPrompt.md`
• 发版遵守：`.ai/发版经理CodexPrompt.md`
• 触及 protected-surfaces 中 W1–W3 / M1 / M2 / M3 / M6 / M7 时，必须保持对应 Go / Node 回归绿；禁止把 `/points/account` 的 `total` 退回 `available+frozen`，禁止把首页/生图首屏默认 limit 改回 120/300；禁止 Seedance 默认预估退回约 90（应对齐 5s/720p=600）；禁止从套餐/通道去掉 `grok-imagine-video-1.5-preview` 或 `seedance-fast-2.0`；禁止视频下载原样下发 `.m4v`；禁止自由P图入口回退「信息图」/「AI办公」、移出首页主能力区、或再叠外层壳；禁止改掉「开始生成」`#ff6b00`；禁止登录默认跳代理商页；禁止「暂不登录」入口退化成灰色弱链；禁止在未恢复类目资质前把小程序视频灵感详情播放器重新开回去。
• 生产机禁止长期未提交热修；dirty tree 不得绕过 `deploy.sh`。

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
