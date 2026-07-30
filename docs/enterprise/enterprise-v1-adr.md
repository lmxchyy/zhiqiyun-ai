# Enterprise V1 架构决策记录

> 状态：ACCEPTED / FROZEN
>
> 日期：2026-07-30
>
> 规则：变更以下任一决策必须新增 ADR，不得直接覆盖本记录。

## ADR-001：复用 `xz_orders`，不新建企业订单表

### 背景

现有 Price Plan V2 和微信虚拟支付已经使用 `xz_orders` 保存订单、价格快照、权益快照、购买人、租户、支付和履约状态。新建企业订单表会复制支付状态机、查单补偿和对账逻辑。

### 决策

Enterprise V1 继续使用 `xz_orders`。`buyer_user_id` 表示购买人，`tenant_id` 表示企业额度的受益企业，`billing_subject_type = ENTERPRISE` 明确订单主体；`user_id` 保留为购买人兼容字段。

### 原因

- 复用已验证的订单、支付、幂等和补偿链。
- 避免个人/企业订单查询和支付回调双轨。
- 订单快照能承载购买人与受益主体分离。

### 被否决方案

- 新建 `enterprise_orders`。
- 把企业 ID 塞入 `raw` JSON 而不建立明确主体语义。
- 将 `user_id` 改为企业 ID，破坏现有外键和订单归属逻辑。

### 影响

- 企业订单查询必须同时理解购买人和受益企业。
- 旧查询不得再假设 `tenant_id` 与购买人的默认租户等价。
- 需要新增主体字段、约束和索引，但不迁移历史个人订单语义。

### 后续条件

只有出现无法由统一订单状态机表达的独立合同/账期业务，并通过新 ADR 证明隔离必要时，才评估新订单聚合；后付费不在 V1。

## ADR-002：复用 `xz_tenant_wallets`，不新建企业钱包

### 背景

当前企业 AI 扣费已使用 `xz_tenant_wallets`、Credit Lot 和 Compute Ledger。另建企业额度钱包会让购买发放与 AI 消费落在不同余额模型。

### 决策

企业额度包发放增加既有 `xz_tenant_wallets.point_balance`，同时写 `xz_compute_credit_lots` 和 `xz_compute_ledger_entries`；不创建新钱包。

### 原因

- 与现有企业扣费事实源一致。
- 保持个人钱包完全独立。
- lot 和 ledger 已具备批次消耗、幂等和审计基础。

### 被否决方案

- 新建 `enterprise_quota_wallets`。
- 将企业额度写入 `xz_point_accounts`。
- 只改钱包余额，不写 lot 和 ledger。

### 影响

- Billing 是企业钱包唯一写入者。
- 钱包余额必须能与有效 lot、ledger 对账。
- V1 商品只允许永久 lot，规避到期余额漂移。

### 后续条件

若引入多币种额度、部门预算或后付费账户，需要新的账户模型 ADR；不得在 `xz_tenant_wallets` 中暗加混合语义。

## ADR-003：企业额度发放统一走 `GrantOrderEntitlements`

### 背景

项目规范要求微信支付回调、官方查单补偿和后台人工补发共用统一权益服务。企业额度需要同时更新钱包、lot、ledger、fulfillment，任何散写都会造成重复发放或账务不一致。

### 决策

支付成功后的企业额度发放只允许由 `GrantOrderEntitlements(orderNo)` 编排。该服务验证不可变订单快照，并在同一事务内调用 Billing grant 命令完成所有账务和履约写入。

### 原因

- 单一幂等入口。
- 回调、查单和人工重试行为一致。
- 支付成功但履约失败可安全重试。

### 被否决方案

- 在微信 notify handler 中直接改钱包。
- 后台提供“直接补额度”按钮绕过订单履约。
- 不同补偿入口各自实现 grant SQL。

### 影响

- `GrantOrderEntitlements` 必须识别 `ENTERPRISE_QUOTA`。
- fulfillment 需要记录企业主体、幂等结果和失败原因。
- 人工处理入口只接受 fulfillment/order 标识，不接受额度数值。

### 后续条件

新增任何付费权益类型都应注册到同一权益服务并满足相同幂等、事务和对账约束。

## ADR-004：V1 只扩展 Price Plan V2，不同时扩展两套支付主链

### 背景

当前仓库同时存在 Price Plan V2 + 微信虚拟支付，以及统一 Payment Center。统一 Payment Center 仅注册 `grant_token`，`enterprise_plan` 是无处理器的预留契约。

### 决策

Enterprise V1 唯一购买主链为：

```text
Price Plan V2 -> 微信虚拟支付 -> GrantOrderEntitlements
```

统一 Payment Center 的 `enterprise_plan` 保持不启用，不创建企业商品、不注册 handler、不开放入口。

### 原因

- 微信小程序虚拟商品已有可用签名、回调和查单链路。
- 避免两个商品目录、两个履约 handler 和两套对账口径同时变化。
- 降低冻结阶段的发布面和回归风险。

### 被否决方案

- 同时为两套支付中心增加企业额度。
- 将 Price Plan V2 改造成统一 Payment Center 的临时代理层。
- 直接使用 legacy productCode 下单绕过 V2 报价。

### 影响

- Price Plan V2 的业务类型、快照、后台治理和产品列表必须支持企业额度。
- 统一 Payment Center 仍可能在管理端显示其他订单，但不是企业额度创建入口。

### 后续条件

只有完成支付中心统一专项，包括商品、订单、履约、退款、对账迁移方案后，才能通过新 ADR 切换主链。

## ADR-005：企业额度不足不回退个人钱包

### 背景

同一用户可以同时拥有个人和企业上下文。额度不足时回退个人钱包会造成员工个人资产被企业任务消耗、页面主体与真实扣费不一致，并模糊财务责任。

### 决策

当前服务端上下文为 `ENTERPRISE` 时，只能扣受益企业钱包。余额不足立即返回明确业务错误并禁止模型调用，个人钱包余额必须保持不变。

### 原因

- 财务主体确定且可审计。
- 防止隐性使用个人资产。
- 简化企业消费对账和权限控制。

### 被否决方案

- 企业不足后自动扣个人。
- 前端弹窗允许本次切换个人付费。
- 按企业/个人余额比例拆分扣费。

### 影响

- UI 必须显示真实计费主体和不足原因。
- 需要覆盖“企业不足但个人充足”的集成测试。
- 用户如需个人支付，必须先显式切换为个人工作空间，再重新发起任务。

### 后续条件

未来若支持员工自愿个人代付，必须设计独立、显式、可撤销的授权和订单，不得作为自动 fallback。

## ADR-006：企业 AI 消费不产生代理佣金

### 背景

现有 Generation 和 PPT 账单链会根据操作用户的推荐关系调用佣金函数。企业任务的费用由企业额度池承担，操作员工的个人推荐关系不应改变企业成本或产生消费分佣。

### 决策

当 `billing_account_type = ENTERPRISE` 时，Generation、PPT、RAG、Connector、Agent 等所有 AI 消费不得调用 Commission，也不得创建佣金兼容记录、正式记录或佣金钱包流水。

### 原因

- 企业付费主体与员工个人关系隔离。
- 防止一次企业额度购买被后续消费重复分佣。
- 保证渠道体系与企业消费账本可独立对账。

### 被否决方案

- 依据员工推荐人继续按每次消费分佣。
- 仅把佣金比例配置为 0，但仍创建记录。
- 仅在页面隐藏企业消费佣金。

### 影响

- 共享 Generation 和 PPT 的佣金调用前必须有后端硬守卫。
- 企业消费测试需断言所有 Commission 表零增量。
- 个人 AI 消费和既有代理佣金逻辑保持不变。

### 后续条件

若未来企业项目销售需要佣金，只能在企业额度包销售时通过独立版本和快照结算一次；仍禁止按 AI 消费分佣。

## ADR-007：V1 使用立即扣减 + 失败冲正，不实现真实 Reservation

### 背景

当前企业“reserve”实现会立即消耗 lot 并减少钱包余额，失败时创建 reversal lot 和 ledger；`frozen_points` 未参与真实冻结。重做冻结状态机将扩大 V1 范围。

### 决策

V1 保留当前立即 `DEBIT`、失败追加 `REVERSAL` 的模型。代码、接口、页面和文档不得把它描述为真正冻结；`RESERVE/CAPTURE` 仅作为任务计费生命周期兼容事件。

### 原因

- 已有企业扣费与冲正路径可复用。
- 避免引入冻结 lot、超时释放、并发捕获和后台修复的新状态机。
- 对一次性 AI 调用，立即扣减与失败冲正可满足 V1 可对账性。

### 被否决方案

- V1 内实现 `available/frozen/captured/released` 完整状态机。
- 先只冻结钱包、不冻结 lot。
- 失败时直接更新原 DEBIT ledger。

### 影响

- 失败窗口内余额会先减少再恢复。
- 冲正必须幂等且追加记录，不可删除原账。
- 页面只展示“扣减/冲正”，不展示“冻结中额度”。

### 后续条件

长任务、批量任务或高并发预占出现明确业务需求时，启动独立 Reservation V2 ADR 和迁移。

## ADR-008：企业消费主数据来自 Compute Ledger 和 Model Usage

### 背景

管理后台当前企业交易查询读取 `xz_tenant_point_transactions`，该表只记录人工充值和调整；真实 AI 消费写入 Compute Ledger，并在 Model Usage 记录模型用量。

### 决策

企业额度财务明细以 `xz_compute_ledger_entries` 为主，模型和成员用量以 `xz_model_usage_records` 为主。`xz_tenant_point_transactions` 只作为人工操作兼容投影，并可通过 reference 与 ledger 关联。

### 原因

- 覆盖真实 CREDIT/DEBIT/REVERSAL。
- 同时满足财务余额变化和运营模型用量查询。
- 避免从人工调整表伪造消费事实。

### 被否决方案

- 继续扩展 `xz_tenant_point_transactions` 承载全部 AI 消费。
- 只查 `xz_billing_events`。
- 从 Generation 任务状态临时计算企业消费。

### 影响

- 企业端和管理端需要两个分页查询与关联投影。
- 对账以 wallet = ledger running balance = valid lot sum 为核心。
- 需要 tenant/user/org/time 组合索引。

### 后续条件

若建立统一数据仓库，可从两张事实表构建分析投影，但不能取代在线账本事实源。

## ADR-009：权益快照是企业额度发放唯一配置源

### 背景

商品配置和价格方案会变化。若支付成功后读取当前 plan 配置发放，历史订单会因配置变更得到不同权益。

### 决策

创建报价时将 Price Plan V2 的企业额度权益合并为不可变 `rights_snapshot`；创建订单时原样固化。`GrantOrderEntitlements` 只读取订单 `rights_snapshot`，不回读 `xz_plan_versions`、`xz_price_plans` 或客户端参数。

### 原因

- 保证报价、支付金额、权益发放可重放。
- 支持商品版本升级而不改历史订单。
- 便于人工重试和审计。

### 被否决方案

- 履约时读取当前商品配置。
- 只记录 plan ID，不记录权益数量。
- 让客户端回传购买时看到的额度。

### 影响

- 快照 schema 必须版本化、严格校验、整数化。
- 快照缺字段或不兼容时履约失败并可重试，不能猜测默认值。
- 管理后台发布商品前必须验证快照完整性。

### 后续条件

新增权益字段必须提升快照版本并保持旧版本解析器；禁止原地改变已支付订单快照。

## ADR-010：客户端不得选择扣费主体

### 背景

小程序会发送 `X-Tenant-Id`，部分页面也持有 tenantId。若服务端直接信任客户端，用户可以跨租户购买或把个人任务伪装为企业任务。

### 决策

购买和 AI 调用的 `billing_subject_type`、`tenant_id`、`billing_account_id` 必须由服务端根据认证用户的当前上下文、有效成员关系、组织和权限派生。客户端只能显示服务端返回结果；任何主体字段若出现在请求中，只能用于一致性校验，不能改变结算。

### 原因

- 关闭跨租户和越权扣费面。
- 保证 UI、任务快照、订单和账本主体一致。
- 兼容一人多上下文模型。

### 被否决方案

- 由前端单选“个人/企业支付”。
- 只校验 `X-Tenant-Id` 是否格式正确。
- 从商品页面 URL 的 enterpriseId 决定受益企业。

### 影响

- 需要统一服务端 subject resolver。
- 报价 token 必须绑定 user + tenant + subject，订单再次核验。
- 页面切换工作空间后必须清理旧主体缓存并重新获取 subject。

### 后续条件

若未来支持代购或跨企业采购，必须引入服务端授权委托和审批模型，不能放宽本决策。

