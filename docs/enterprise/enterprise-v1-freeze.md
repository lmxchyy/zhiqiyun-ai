# Enterprise V1 范围冻结

> 状态：FROZEN
>
> 冻结日期：2026-07-30
>
> 依据：当前代码只读审计。本文是 Enterprise V1 产品、架构、验收和上线范围的唯一基线；范围变化必须新增 ADR 并重新评审。

## 1. V1 产品目标

Enterprise V1 的目标是形成最小但完整的企业 AI SaaS 商业闭环：企业管理员或财务人员在企业工作空间购买企业额度包，微信支付成功后额度进入既有企业钱包与 Compute Credit Lot，企业成员在企业上下文使用 AI 时只扣企业额度，企业与平台管理员能够查询订单、履约、消费和模型用量。

V1 必须同时满足：

1. 不改变个人钱包、个人会员和代理钱包的既有语义。
2. 不新建企业钱包或企业订单系统。
3. 购买、履约、扣费、查询均可幂等、可审计、可对账。
4. 购买人与受益企业分离，客户端不能选择或伪造扣费主体。
5. 企业额度包购买和企业 AI 消费均不产生代理佣金。

## 2. V1 范围

V1 仅包含以下能力：

- 新增 `ENTERPRISE_QUOTA` 商品业务类型。
- 企业管理员、财务人员购买企业额度包。
- 复用 Price Plan V2、微信虚拟支付和既有 `xz_orders`。
- 支付成功、官方查单补偿、后台人工重试统一调用 `GrantOrderEntitlements(orderNo)`。
- 企业额度发放原子写入：
  - `xz_tenant_wallets`
  - `xz_compute_credit_lots`
  - `xz_compute_ledger_entries`
  - `xz_fulfillment_records`
- `buyer_user_id` 表示购买人，`tenant_id` 表示受益企业，`billing_subject_type` 明确为 `ENTERPRISE`。
- 企业额度不足直接拒绝，不回退个人钱包。
- 企业 AI 消费禁止创建任何代理佣金记录、佣金钱包流水或待结算项。
- AI 创作页显示服务端返回的真实计费主体与可展示余额。
- 企业消费明细以 Compute Ledger 为财务主账，以 Model Usage 为用量明细。
- 管理后台展示真实企业订单、履约状态、额度流水和模型用量。
- V1 企业额度包只允许永久 lot；任何非永久 lot 均不属于 V1，必须通过后续版本 ADR 和到期结算能力另行立项。

## 3. 非 V1 范围

以下能力明确不进入 V1：

- 部门预算、成员预算、成员限额。
- 真实 Reservation 冻结模型；`frozen_points` 不在本期启用。
- 后付费、信用额度、欠费追缴。
- 发票、合同、线下收款和销售审批。
- SSO、SCIM、私有化部署。
- AI 应用市场。
- 新建企业钱包、新建企业订单系统。
- 重构个人钱包、个人会员、代理钱包或代理等级体系。
- 同时扩展统一 Payment Center 的 `enterprise_plan`。
- 企业额度包购买佣金、企业项目佣金和消费分佣。
- 将旧 `schema.sql` 企业模型迁移为运行时主模型。

## 4. 用户角色

| 角色 | V1 权限 | 明确禁止 |
| --- | --- | --- |
| `ENTERPRISE_ADMIN` | 查看企业余额、商品、订单、履约、消费；创建报价和订单；使用企业 AI | 选择其他企业为受益主体；修改服务端金额或权益快照 |
| `FINANCE` | 查看企业余额、商品、订单、履约、消费；创建报价和订单 | 管理成员和 AI 能力；代表其他企业购买 |
| `AI_ADMIN` | 使用企业 AI；查看是否由企业付费；按授权查看消费汇总 | 购买额度包，除非后续显式授予购买权限 |
| `ENTERPRISE_MEMBER` | 使用企业 AI；看到真实计费主体 | 购买额度包；查看企业余额和全员消费，除非另行授权 |
| 平台财务/运营管理员 | 查看企业订单、支付、履约、额度和对账；执行既有人工履约重试入口 | 直接改钱包余额；绕过 `GrantOrderEntitlements` 补发 |
| 代理/运营中心 | 保留个人、代理既有能力 | 从企业额度包购买或企业 AI 消费获得佣金 |

V1 新增租户权限码：

- `enterprise.quota.purchase`
- `enterprise.order.read`
- `enterprise.compute.usage.read`

默认授权：`ENTERPRISE_ADMIN` 拥有三项权限；`FINANCE` 拥有三项权限；其他企业角色默认不拥有购买和全量查询权限。`enterprise.ai.use` 沿用现有授权。

## 5. 关键业务流程

### 5.1 唯一购买主链

```mermaid
flowchart LR
    A["企业管理员或财务进入企业工作空间"] --> B["服务端解析当前企业上下文和购买权限"]
    B --> C["Price Plan V2 返回 ENTERPRISE_QUOTA 商品"]
    C --> D["创建绑定 buyer_user_id 与 tenant_id 的不可变报价"]
    D --> E["创建 xz_orders 企业受益订单"]
    E --> F["微信虚拟支付"]
    F --> G["支付回调或官方查单确认成功"]
    G --> H["GrantOrderEntitlements"]
    H --> I["同一事务锁订单与企业钱包"]
    I --> J["写 Credit Lot 和 Compute Ledger CREDIT"]
    J --> K["更新企业钱包和履约 SUCCESS"]
```

冻结结论：Enterprise V1 唯一购买主链是：

```text
Price Plan V2 -> 微信虚拟支付 -> GrantOrderEntitlements
```

统一 Payment Center 中预留的 `FulfillmentEnterprisePlan = "enterprise_plan"` 本阶段保持无处理器、不可配置、不可发布。不得同时实现第二条企业支付主链。

### 5.2 企业额度发放

1. `GrantOrderEntitlements` 使用 `order_no` 锁定订单并验证支付成功、`snapshot_version = 2`、`billing_subject_type = ENTERPRISE`、`product_type = ENTERPRISE_QUOTA`。
2. 权益数量只能读取订单的不可变 `rights_snapshot`，不得回读当前商品配置，也不得读取客户端字段。
3. `rights_snapshot` 至少包含：

```json
{
  "entitlementType": "ENTERPRISE_COMPUTE",
  "unit": "COMPUTE_UNIT",
  "computeUnits": 100000,
  "bonusComputeUnits": 0,
  "lotValidity": "PERMANENT",
  "commissionEligible": false
}
```

4. 在同一数据库事务内：锁 `xz_tenant_wallets`；按基础额度和赠送额度分别写 lot；增加钱包 `point_balance`；写 Compute Ledger `CREDIT`；写或更新 `xz_fulfillment_records`；将订单权益状态置为成功。
5. lot 和 ledger 的幂等键由订单号与权益分量确定，例如 `enterprise-quota:{orderNo}:base`、`enterprise-quota:{orderNo}:bonus`、`enterprise-quota:{orderNo}:ledger`。
6. 重复回调、重复查单和重复人工重试只能得到同一发放结果，不得重复增加余额。
7. 支付成功但发放失败时，订单保持已支付/履约失败，可通过同一服务重试；禁止另写补发 SQL。

### 5.3 AI 扣费主体判定

1. 服务端从 `xz_user_role_context` 和有效 `xz_tenant_members` 解析当前上下文。
2. `PERSONAL`：沿用个人钱包链路。
3. `ENTERPRISE`：校验企业、成员、组织、服务状态和 `enterprise.ai.use`，仅扣 `xz_tenant_wallets` 对应的有效 lot。
4. 企业额度不足返回业务错误并阻止模型调用；不得尝试个人钱包。
5. V1 沿用“立即扣减 + 失败冲正”：调用前扣企业额度，失败时创建 reversal lot 和 `REVERSAL` ledger；不宣称真实冻结。
6. 客户端传入的 `tenantId`、`billing_subject_type`、`billing_account_id` 只能作为显示或一致性校验信息，不能成为授权或扣费依据。

### 5.4 消费与佣金

- Compute Ledger 是企业额度增减的财务事实源。
- Model Usage 是模型、模块、成员、组织、Token 原始用量的事实源。
- `xz_tenant_point_transactions` 仅保留为人工充值/调整兼容记录，不再作为企业 AI 消费页主数据。
- 企业 AI 消费在进入 Commission 前必须由服务端硬判定 `billing_account_type = ENTERPRISE` 并短路。
- 企业额度商品在报价、订单、履约三个阶段均固定 `commissionEligible = false`，不得生成佣金快照和结算记录。

## 6. 当前 AI 场景覆盖结论

| 场景 | 当前企业扣费 | V1 仍需完成 | V1 发布结论 |
| --- | --- | --- | --- |
| 生图、改图 | 已进入共享 Generation 企业鉴权、lot 扣减、失败冲正和 Model Usage | 阻断共享生成佣金；显示真实主体 | 完成后可发布 |
| 视频生成 | 已进入共享 Generation 企业链路 | 阻断共享生成佣金；显示真实主体 | 完成后可发布 |
| PPT | 已有专用企业鉴权与扣减 | 阻断 PPT 直接佣金调用；显示真实主体 | 完成后可发布 |
| RAG 对话 | PostgreSQL 路径已进入企业鉴权、lot 扣减和 Model Usage；当前未调用佣金 | 禁止使用个人化 JSON fallback 承诺企业能力；补查询与展示 | PostgreSQL 路径可发布 |
| Knowledge Agent | Agent 执行实际走 RAG，因而继承企业扣费 | 用知识 Agent 集成用例确认主体、用量和无佣金 | 完成验收后可发布 |
| Connector 生图/视频/PPT | 已复用共享生成或 Connector PPT 账单链路，按 connector_key 解析企业 | 阻断继承的生成/PPT 佣金；补端到端验收 | 完成后可发布 |
| 独立通用对话 | 未发现与 RAG 等价的独立公开付费入口 | 不属于 V1；后续版本须先接统一授权、企业扣费、用量和失败冲正并新增 ADR | V1 全环境禁止作为企业付费能力发布 |
| 独立通用 Agent Run | 未发现独立于 Knowledge Agent/RAG 的统一运行入口 | 不属于 V1；后续版本须接统一企业计费且不能复用前端主体参数 | V1 全环境禁止作为企业付费能力发布 |

因此，“对话、Agent 已覆盖”的冻结口径仅指 RAG 对话和 Knowledge Agent；不能外推为所有未来对话或 Agent 入口都已覆盖。

## 7. 数据边界

### 7.1 复用并扩展

| 数据 | V1 语义 |
| --- | --- |
| `xz_plan_versions` | 支持 `business_type = ENTERPRISE_QUOTA`；配置企业 compute units，但履约只读订单快照 |
| `xz_price_plans` | 继续承载渠道、环境、价格和赠送企业额度配置 |
| `xz_order_price_quotes` | 固定购买人、受益企业、计费主体、价格、商品、权益和不参与佣金快照 |
| `xz_orders` | 继续作为唯一订单；`tenant_id` 是受益企业，`buyer_user_id` 是购买人 |
| `xz_fulfillment_records` | 记录企业受益主体、履约类型、结果、重试和错误 |
| `xz_tenant_wallets` | 现有企业钱包；只由 Billing 领域服务写入 |
| `xz_compute_credit_lots` | 企业额度批次，V1 商品只发永久 lot |
| `xz_compute_ledger_entries` | 企业额度 CREDIT/DEBIT/REVERSAL 主账 |
| `xz_model_usage_records` | 企业 AI 模型和成员用量明细 |

### 7.2 明确不使用

- 不使用 `database/schema.sql` 中的 `enterprises`、`enterprise_members`、`enterprise_quota_transactions` 作为运行时主模型。
- 不使用 `xz_tenant_point_transactions` 作为 AI 消费主账。
- 不把企业额度写入 `xz_point_accounts`。
- 不把 compute unit 写入个人 token、会员权益或代理钱包字段。
- 不创建 `enterprise_orders`、`enterprise_wallets` 或另一套 fulfillment 表。

## 8. API 边界

### 8.1 复用接口

- `GET /api/v1/payment/products`：在服务端确认企业上下文和权限后返回企业额度商品。
- `POST /api/v1/payment/price-quotes`：请求仅包含商品/价格方案标识；服务端派生 `billingSubjectType`、`tenantId` 和购买人。
- `POST /api/v1/payment/wechat-virtual/orders`：消费不可变报价创建企业受益订单。
- `GET /api/v1/payment/orders/:orderNo/status`：返回订单、支付、履约和企业额度到账摘要。
- `POST /api/v1/payment/orders/:orderNo/sync`：官方查单并复用 `GrantOrderEntitlements`。

### 8.2 V1 新增或扩展接口

- `GET /api/v1/enterprise/quota-products`
  - 权限：`enterprise.quota.purchase`
  - 返回企业可买的 Price Plan V2 投影；不返回可由前端修改的权益字段。
- `POST /api/v1/enterprise/quota-price-quotes`
  - 权限：`enterprise.quota.purchase`
  - 请求：`pricePlanId` 或稳定商品入口标识。
  - 服务端调用 Price Plan V2 报价服务；主体来自当前企业上下文。
- `GET /api/v1/enterprise/orders`
  - 权限：`enterprise.order.read`
  - 仅查询当前企业 `tenant_id` 下的企业额度订单。
- `GET /api/v1/enterprise/compute-account`
  - 扩展返回 `billingSubject`、余额、有效 lot 汇总和最近 ledger。
- `GET /api/v1/enterprise/compute-ledger`
  - 权限：`enterprise.compute.usage.read`
  - 支持时间、entryType、moduleCode、actorUserId、referenceId、游标分页。
- `GET /api/v1/enterprise/model-usage`
  - 权限：`enterprise.compute.usage.read`
  - 支持时间、成员、组织、模块、模型、任务 ID、游标分页。
- `GET /api/v1/billing-subject`
  - 返回服务端解析的当前计费主体、可展示余额、是否允许查看明细；不得接受主体选择参数。

管理后台扩展：

- `GET /api/v1/admin/enterprises/:enterpriseId/compute`
- `GET /api/v1/admin/enterprises/:enterpriseId/transactions`
- `GET /api/v1/admin/enterprises/:enterpriseId/orders`
- `GET /api/v1/admin/payment/fulfillments`

这些接口改为读取 Compute Ledger、Model Usage、`xz_orders` 和 `xz_fulfillment_records`。管理端 `enterpriseId` 由平台 RBAC 保护；企业端永远不能通过路径或请求体指定任意企业。

## 9. 页面边界

### 9.1 小程序/用户端

- 企业中心“企业算力”页：余额、可购额度包、购买按钮、最近订单、履约状态。
- 企业中心“消费明细”页：额度流水与模型用量两个视图，支持时间、成员、模块筛选。
- AI 创作页：显示“个人账户支付”或“企业：名称支付”；余额按权限展示，普通成员至少看到主体和额度是否可用。
- 支付结果页：企业订单显示受益企业、购买人、到账 compute units 和履约状态，不显示“个人到账点数”。
- 不新增第二套企业导航；继续使用现有 Enterprise Center 页面栈。

### 9.2 管理后台

- Price Plan Governance：支持 `ENTERPRISE_QUOTA` 类型、compute units 权益和无佣金状态。
- Enterprise Management：企业详情的算力与交易页读取真实 ledger/usage；订单页显示购买人、受益企业、支付与履约。
- Billing/Payment：履约列表可按企业、订单、状态筛选并使用既有人工重试服务。
- 不在管理页面提供直接改钱包余额的 SQL 型操作；保留既有有审计的人工调整命令。

## 10. 验收边界

V1 验收必须覆盖：

1. 企业管理员和财务可购买；其他成员创建报价或订单返回 403。
2. 报价、订单的 `tenant_id` 来自当前企业上下文；伪造 `X-Tenant-Id` 或请求体 tenantId 不能跨租户。
3. 购买人与受益企业正确分离；订单查询同时受购买人/企业权限约束。
4. 微信金额、商品、环境、报价快照不匹配时拒绝履约。
5. 一次支付只发放一次；重复回调、查单、人工重试不重复 lot、ledger、钱包余额。
6. 基础和赠送 lot、钱包增量、CREDIT ledger、fulfillment 记录在同一事务内一致。
7. 企业额度不足时模型调用未发生，个人余额不变。
8. 企业任务失败后产生一次冲正，余额和有效 lot 可对账。
9. 企业生图、视频、PPT、RAG、Knowledge Agent、Connector 测试均无佣金记录。
10. 企业额度包购买无佣金快照、记录和钱包流水。
11. AI 创作页展示主体与后端实际扣费一致。
12. 企业端和管理端消费合计可从 Compute Ledger 与 Model Usage 关联到任务、成员和模型。
13. 个人会员购买、个人充值、代理加入、个人 AI 扣费回归不变。

不属于 V1 验收：部门/成员预算、真实冻结、后付费、发票合同、SSO、私有化。

## 11. 上线门禁

以下条件全部满足才允许从冻结进入发布：

- 数据库迁移在实施时重新扫描并选用下一个可用编号；不得占用当前未跟踪的 `101`。
- 迁移向前、回滚和历史数据兼容演练通过。
- Price Plan V2 企业商品生产门禁、微信商品绑定和金额核对通过。
- 支付回调、官方查单、人工重试均证明调用同一 `GrantOrderEntitlements`。
- 企业购买与企业消费佣金硬隔离测试通过，数据库对账为零佣金。
- 所有已发布企业 AI 入口完成“主体、扣费、失败冲正、用量、无佣金”矩阵验收。
- RAG/Knowledge Agent 生产环境使用 PostgreSQL 持久化路径。
- 企业端和管理端不再把 `xz_tenant_point_transactions` 当作 AI 消费主账。
- 个人会员、充值、代理加入、个人生成回归通过。
- 灰度开关默认关闭；仅对白名单企业开启；具备监控、对账和停止入口。
- 未经单独生产授权，不执行迁移、部署、微信商品发布或流量切换。

任一门禁失败，状态为 `BLOCKED`，不得以“前端隐藏”替代后端安全约束。

## 12. 回滚边界

### 12.1 可回滚

- 关闭 Enterprise Quota 商品可见性和报价/下单开关。
- 关闭企业购买入口、企业消费明细入口和管理后台新视图。
- 停止灰度企业新增购买，同时保留订单和账本只读查询。
- 回滚应用代码到不识别 `ENTERPRISE_QUOTA` 的版本，前提是旧版本能忽略新增可空字段。

### 12.2 不可直接回滚

- 已支付订单、已成功履约记录、已写入 lot 和 ledger 不删除、不改写。
- 已消费或已冲正的 Compute Ledger 不做反向更新；纠错只能追加审计明确的调整/冲正记录。
- 不通过回滚把企业消费改扣个人钱包。
- 不执行 `git reset/checkout/stash` 或直接删迁移解决生产问题。

### 12.3 回滚后的处理

- 已支付但履约失败订单继续由统一补偿服务处理，或进入人工审核队列。
- 已成功发放的企业额度继续有效，企业 AI 是否继续开放由独立功能开关决定。
- 回滚前后分别保存订单、支付、fulfillment、wallet、lot、ledger、usage、commission 对账快照。
