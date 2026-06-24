# 主控 SaaS 计费中心复刻方案

## 目标

把当前主控后台从“套餐、订单、用量分散管理”升级为类似 Lago 的计费闭环：

```text
客户 -> 订阅 -> 计量事件 -> 费用聚合 -> 账单 -> 发票 -> 支付与催收 -> 收入分析
```

第一版不建议完整复制 Lago 的所有边界，而是复刻它最适合先知 AI 的核心能力：订阅制、按量计费、混合套餐、权益控制、账单生成、发票管理、客户计费档案和实时用量视图。

## Lago 借鉴点

Lago 的核心不是“支付收款”，而是计费逻辑中台：

- 支持订阅制、按量计费和混合定价。
- 通过事件摄入和聚合，把任意可追踪用量转换成可收费指标。
- 计划、订阅、优惠、税费、预付额度、超额费用最终汇总到账单和发票。
- 支付网关保持可插拔，计费系统不被 Stripe 或单一支付通道绑定。
- API 优先，后台 UI 是业务运营入口，核心流程应能被接口自动化。

映射到先知 AI，应重点围绕 AI API、图片生成、Agent 对话、GEO 任务、代运营服务和渠道客户做计费，而不是只做会员套餐 CRUD。

## 现状差距

当前代码已经具备基础模块：

- `admin-vue/src/stores/admin.ts` 已有客户、套餐、订单、用量、分润、API 设置等导航。
- `backend-go/internal/httpserver/admin_api.go` 已有 `/api/v1/admin/customers`、`plans`、`orders`、`usage`、`commissions`、`system/settings` 等接口。
- `backend-go/internal/httpserver/admin_types.go` 已有 `adminPlan`、`adminOrder`、`adminAPIModel`、`adminAPIKey`、`adminCustomerGroup` 等结构。
- `docs/admin-saas-master-control-plan.md` 已经定义主控后台 10 个中心。

主要缺口：

- 没有统一客户计费档案，客户、用户、企业、代理来源、开票信息和付款方式没有合并视图。
- 没有订阅实体，套餐购买后缺少生命周期、周期、试用、升降级、取消、续费、欠费状态。
- 用量只是汇总展示，没有独立计量事件、幂等键、聚合窗口和可回放审计。
- 套餐权益没有拆成固定订阅费、免费额度、超额单价、阶梯价格、预付额度。
- 账单、发票和支付没有形成可追踪状态机。
- 前端缺少“客户计费详情抽屉”和“账单运行批次”这样的财务工作台视角。

## 建议新增一级模块

在现有左侧导航保留原业务中心，同时新增或调整为以下计费分组：

| 模块 | 作用 |
| --- | --- |
| 计费驾驶舱 | MRR、用量收入、待开票、逾期账单、毛利、用量趋势 |
| 客户计费 | 客户档案、订阅、余额、开票信息、付款方式、账单历史 |
| 套餐产品 | 产品包、价格计划、权益、免费额度、超额计价、阶梯价 |
| 订阅管理 | 试用、激活、暂停、取消、续费、升降级、按比例折算 |
| 计量事件 | API、图片、Agent、GEO、代运营服务的原始事件和聚合结果 |
| 账单中心 | 周期账单、手工账单、超额账单、贷项、调整项、账单运行批次 |
| 发票中心 | 发票申请、抬头、税号、开票、红冲、附件和发票号码 |
| 支付与催收 | 支付通道、收款记录、失败重试、逾期提醒、人工标记收款 |
| 收入分析 | MRR、ARR、留存、扩张收入、产品收入结构、渠道贡献 |

## 核心数据模型

### 客户计费档案

新增 `billing_customers`，不要直接把所有计费字段塞进 `users`：

- `id`
- `user_id`
- `enterprise_id`
- `display_name`
- `email`
- `agent_id`
- `customer_group_id`
- `billing_status`
- `currency`
- `timezone`
- `invoice_title`
- `tax_number`
- `invoice_email`
- `payment_terms_days`
- `metadata`
- `created_at`
- `updated_at`

### 价格与权益

现有 `membership_plans` 继续保留，但新增价格版本：

- `billing_products`：文生图、AI API、Agent、GEO、代运营。
- `billing_plans`：套餐主表，支持版本、周期、状态。
- `billing_plan_charges`：收费项，区分固定订阅费、按量费用、一次性费用。
- `billing_entitlements`：功能权益，例如模型白名单、并发、API 请求率、图片额度、GEO 任务数。
- `billing_tiers`：阶梯价，例如 0-10000 token 免费，10001-100000 按 0.02 元/千 token。

### 订阅

新增 `billing_subscriptions`：

- `id`
- `billing_customer_id`
- `plan_id`
- `plan_version`
- `status`
- `billing_cycle`
- `current_period_start`
- `current_period_end`
- `trial_ends_at`
- `cancel_at_period_end`
- `prepaid_balance_cents`
- `entitlement_snapshot`
- `price_snapshot`
- `created_at`
- `updated_at`

状态建议：

```text
TRIALING -> ACTIVE -> PAST_DUE -> PAUSED -> CANCELED
```

### 计量事件

新增 `billing_events` 和 `billing_usage_summaries`：

- `billing_events.transaction_id` 必须唯一，避免重复计费。
- 事件字段包含 `customer_id`、`subscription_id`、`metric_code`、`quantity`、`unit_amount_cents`、`occurred_at`、`properties`。
- 聚合表按 `customer + subscription + metric + day/month` 生成汇总。

先知 AI 的指标建议：

| 指标 | 来源 |
| --- | --- |
| `api.input_tokens` | OpenAI 兼容 API 调用 |
| `api.output_tokens` | OpenAI 兼容 API 调用 |
| `image.generations` | 文生图任务 |
| `image.edits` | 图生图/局部编辑任务 |
| `agent.messages` | Agent 对话 |
| `geo.monitor_tasks` | GEO 监测 |
| `ops.service_items` | 代运营交付项 |

### 账单与发票

新增：

- `billing_invoices`：账单/发票统一主表，可先用 `invoice_type` 区分账单和正式发票。
- `billing_invoice_lines`：账单明细，包含固定订阅费、用量费、超额费、折扣、税费、手工调整。
- `billing_payments`：支付记录，保留支付通道、流水号、状态。
- `billing_credit_notes`：退款、红冲和贷项调整。
- `billing_dunning_runs`：催收记录。

账单状态：

```text
DRAFT -> FINALIZED -> PAYMENT_PENDING -> PAID
                         -> OVERDUE -> VOID
```

发票状态：

```text
REQUESTED -> ISSUED -> SENT -> RED_INVOICE_REQUESTED -> RED_INVOICED
```

## 后端接口第一版

在现有 `/api/v1/admin/*` 下补齐：

```text
GET    /api/v1/admin/billing/overview
GET    /api/v1/admin/billing/customers
GET    /api/v1/admin/billing/customers/{id}
PATCH  /api/v1/admin/billing/customers/{id}

GET    /api/v1/admin/billing/products
GET    /api/v1/admin/billing/plans
POST   /api/v1/admin/billing/plans
PATCH  /api/v1/admin/billing/plans/{id}

GET    /api/v1/admin/billing/subscriptions
POST   /api/v1/admin/billing/subscriptions
POST   /api/v1/admin/billing/subscriptions/{id}/change-plan
POST   /api/v1/admin/billing/subscriptions/{id}/cancel

GET    /api/v1/admin/billing/events
POST   /api/v1/admin/billing/events
GET    /api/v1/admin/billing/usage-summaries

GET    /api/v1/admin/billing/invoices
POST   /api/v1/admin/billing/invoices/run
POST   /api/v1/admin/billing/invoices/{id}/finalize
POST   /api/v1/admin/billing/invoices/{id}/mark-paid
POST   /api/v1/admin/billing/invoices/{id}/issue-tax-invoice

GET    /api/v1/admin/billing/payments
POST   /api/v1/admin/billing/payments
POST   /api/v1/admin/billing/dunning-runs
```

客户侧兼容接口保留并增强：

```text
GET /v1/dashboard/billing/subscription
GET /v1/dashboard/billing/usage
GET /v1/dashboard/billing/invoices
```

## 前端第一版页面

当前 `admin-vue/src/App.vue` 体量较大，建议第一版少拆组件，先做可见闭环；第二版再拆 `views/billing/*`。

第一版视觉结构：

- 左侧导航新增“计费中心”分组。
- 顶部 KPI：MRR、本月用量收入、待开票、逾期账单。
- 中间主区：订阅生命周期漏斗、用量趋势、账单运行表。
- 右侧详情抽屉：客户计费档案、当前订阅、权益、余额、发票信息、付款方式。
- 表格支持客户、套餐、订阅状态、用量金额、账单状态、开票状态筛选。

优先实现的 5 个视图：

1. `billingDashboard`：计费驾驶舱。
2. `billingCustomers`：客户计费列表 + 详情抽屉。
3. `billingPlans`：套餐产品和价格版本。
4. `billingSubscriptions`：订阅管理。
5. `billingInvoices`：账单和发票中心。

## 计费流水

### AI API 按量计费

```text
API 请求完成
-> 写入 billing_events(transaction_id)
-> 按 metric_code 聚合 input_tokens/output_tokens
-> 匹配订阅和价格版本
-> 扣预付额度或生成未结费用
-> 周期末进入账单明细
```

### 图片生成按次计费

```text
生成任务成功
-> 根据模型和尺寸生成 image.generations 事件
-> 套餐内免费额度先抵扣
-> 超额部分按模型单价计费
-> 失败任务不计费或生成补偿流水
```

### 企业订阅续费

```text
订阅到期前提醒
-> 生成续费账单
-> 支付成功后延长 current_period_end
-> 写入权益快照
-> 更新客户计费状态
```

## 分阶段开发

### 阶段 1：可视化计费骨架

- 新增计费中心导航和 5 个页面。
- 后端提供 mock/JSON store 级别的计费聚合接口。
- 客户计费详情能展示订阅、余额、开票资料、账单历史。
- 不改真实扣费逻辑，先证明工作台形态。

### 阶段 2：计量事件入库

- API 调用、图片生成、Agent 对话、GEO 任务成功后写 `billing_events`。
- `transaction_id` 幂等。
- 每日/月度聚合生成 `billing_usage_summaries`。
- 用量中心从聚合数据读取，而不是临时计算。

### 阶段 3：订阅和账单状态机

- 新增订阅生命周期。
- 实现账单运行批次。
- 支持固定订阅费 + 用量费 + 超额费 + 手工调整。
- 支持标记收款、逾期、作废。

### 阶段 4：发票和支付编排

- 接入发票申请、开票、红冲。
- 支付通道抽象，微信/支付宝/线下转账先做人工确认。
- 催收记录和提醒模板。

## 验收标准

第一版完成后应能验证：

- `/admin` 左侧出现计费中心相关导航。
- `GET /api/v1/admin/billing/overview` 返回 MRR、用量收入、待开票、逾期账单。
- 客户计费详情展示订阅、权益、预付余额、账单、发票资料。
- 套餐产品能表达固定费、免费额度、超额单价和阶梯价。
- 订阅列表能区分试用、活跃、欠费、暂停、取消。
- 账单中心能展示账单运行批次、明细、支付状态和发票状态。
- 现有客户、套餐、订单、用量、分润页面不回退。

## 推荐确认

建议按这版推进：

- 第一轮先做 `admin-vue + backend-go JSON store` 的可见计费中心。
- 第二轮把计量事件接进真实生成/API/Agent/GEO 流程。
- 第三轮再做 PostgreSQL 迁移和正式账单状态机。

这样能最快看到 Lago 式计费后台的样子，同时避免一开始就把真实交易和发票逻辑改得过重。
