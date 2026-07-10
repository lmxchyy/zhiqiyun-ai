# 主控 SaaS 可行性与对接方案

## 目标定位

主控 SaaS 是先知 AI 的平台运营后台，用来统一管理客户、渠道、产品、套餐、订单、交付、用量、分润、经营数据和系统配置。

推荐实现方式是“同平台主控后台”，而不是另起一套独立 CRM：

- 产品端继续服务普通客户，保留现有 AI 创作、Agent、GEO 等能力。
- 主控端新增 `/admin` 后台入口，服务平台管理员、运营、财务、交付和渠道团队。
- 后端继续扩展 Go 服务，在 `backend-go` 中新增 `/api/v1/admin/*` 管理接口。
- 数据层复用现有 PostgreSQL 业务表，并通过迁移补齐主控 SaaS 所需的客户档案、产品目录、交付项目、权限和系统配置表。
- Docker 继续沿用现有 `compose.yml`，由同一个应用容器托管产品端和主控端静态资源。

## 当前项目基线

当前仓库已经具备主控 SaaS 的基础设施和部分业务域：

- 用户端基线：`apps/user-uni`，Vue 3 / uni-app。
- 后端基线：`backend-go`，Go HTTP 服务，默认端口 `3100`。
- Docker 基线：应用容器、PostgreSQL、Redis、RabbitMQ、MinIO。
- 数据库基线：`database/schema.sql` 已包含用户、企业、套餐、订单、支付、代理、佣金、模型、生成任务、Agent、GEO、审计等核心表。
- 当前 Go API 边界仍偏演示：已暴露健康检查、模型、积分、生成任务、资产等接口，主控 SaaS 需要补齐真实管理 API。

## 信息架构

主控 SaaS 建议采用 10 个一级中心：

| 中心 | 业务职责 | 主要角色 |
| --- | --- | --- |
| 客户中心 | 管理所有客户、企业客户、客户状态、归属渠道、套餐和余额 | 管理员、运营、销售 |
| 代理商中心 | 管理一级渠道、二级渠道、邀请关系、渠道客户和渠道审核 | 管理员、渠道经理 |
| 产品中心 | 管理文生图、API、Agent、GEO、代运营登录等可售产品 | 管理员、产品运营 |
| 套餐中心 | 管理价格、权益、用量额度、可售状态和套餐版本 | 管理员、产品运营 |
| 订单中心 | 管理收款、续费、退款、发票、人工订单和支付状态 | 管理员、财务 |
| 交付中心 | 管理代运营、GEO、Agent 搭建等项目进度和里程碑 | 管理员、交付 |
| 用量中心 | 管理 API 调用、对话量、文生图消耗、GEO 任务消耗 | 管理员、运营、客户成功 |
| 分润中心 | 管理代理收益、分润规则、结算单、提现和打款 | 管理员、财务、渠道经理 |
| 数据中心 | 查看收入、成本、毛利、客户增长、用量趋势和渠道贡献 | 管理员、老板视角 |
| 系统中心 | 管理权限、域名、支付、品牌、模型供应商和审计日志 | 超级管理员 |

## 与现有产品体系对接

## 参考 New API 中转站后的补充定位

用户提供的 ChatGPT 分享页主题是 New API 中转站能力，核心参考点不是“另做一个 API 中转站”，而是把其中成熟的 API 运营模型吸收到先知 AI 主控 SaaS：

- 渠道管理：上游服务商、Base URL、API Key、模型列表、模型映射、优先级、健康测试。
- 模型管理：模型可用性、能力类型、按量计费、按次计费、输入输出倍率、缓存优惠、阶梯计费。
- 用户分组：不同客户、套餐、代理渠道使用不同分组倍率和模型权限。
- Token/Key 管理：API Key 可绑定额度、模型限制、IP 白名单、过期时间和调用频率。
- 配额充值：用内部配额/点数承接充值、套餐赠送、人工赠送和消耗扣减。
- 使用记录：按时间、模型、Key、客户、Token、任务类型过滤调用日志。
- OpenAI 兼容：API 产品应尽量支持 OpenAI SDK 风格的调用与用量查询，降低客户接入成本。

因此，主控 SaaS 里的“API 产品”应按“中转站级别”建设，但它只是产品中心与用量中心的一部分；文生图、Agent、GEO、代运营仍然是并列产品线。

### 客户中心

复用：

- `users`
- `enterprises`
- `enterprise_members`
- `point_accounts`
- `orders`
- `channel_agents`

补充：

- `customers`：客户档案，统一关联个人用户、企业、代理来源、客户状态、负责人和标签。
- 客户详情页聚合订单、套餐、余额、用量、交付项目和沟通备注。

### 代理商中心

复用：

- `channel_agents`
- `commissions`
- `withdrawal_requests`
- `settlement_statements`
- `users`

补充：

- 渠道树接口，展示一级渠道和二级渠道。
- 渠道客户归属关系从 `customers.agent_id` 或 `users.referred_by` 统一读取。
- 渠道审核、冻结、启用、停用操作写入 `audit_logs`。

### 产品中心

复用：

- `model_definitions`
- `model_providers`
- `agents`
- `geo_brands`
- `geo_monitor_tasks`
- `generation_tasks`

补充：

- `products`：统一产品目录，类型包括 `TEXT_TO_IMAGE`、`API`、`AGENT`、`GEO`、`OPS_LOGIN`。
- `product_entitlements`：产品权益项，例如图片次数、API 请求量、Agent 对话量、GEO 任务数、代运营席位。
- `api_model_catalog`：API 可售模型目录，包含模型名称、能力、计费模式、上下游映射和状态。
- `api_provider_channels`：API 上游渠道配置，包含 Base URL、模型列表、优先级和健康状态。

产品中心不是直接管理模型本身，而是管理“对客户售卖的产品包”。模型供应商仍放在系统中心。

API 产品要支持三层结构：

```text
上游渠道 -> 平台模型目录 -> 客户套餐权益
```

例如 `gpt-image-2` 需要同时满足：

- 上游渠道支持。
- 平台模型目录启用。
- 套餐权益包含。
- 客户 API Key 未限制。
- 调用入口匹配图片生成接口，而不是普通聊天接口。

### 套餐中心

复用：

- `membership_plans`
- `model_pricing_rules`

补充：

- `plan_products`：套餐与产品权益绑定。
- 套餐版本字段，避免改价影响历史订单。
- 权益快照写入订单，保证历史订单可追溯。
- `customer_groups`：客户分组，用于控制模型倍率、权限和渠道策略。
- 套餐可配置 API 额度、可用模型、并发限制、请求频率、是否支持独立域名、是否支持独立后台、是否支持二级渠道、是否支持充值系统。

套餐建议分成三种典型产品包：

| 套餐类型 | 适合对象 | 核心权益 |
| --- | --- | --- |
| 个人版 | 个人开发者、小团队 | 统一 API、基础模型、基础额度 |
| 企业版 | 企业客户 | 多账号、额度管理、日志、GEO/Agent 能力 |
| 渠道版 | 代理商 | 独立站点、独立后台、二级渠道、分销能力 |
| 代运营版 | 品牌客户 | 项目交付、内容发布、GEO 监控、服务工单 |

### 订单中心

复用：

- `orders`
- `payments`
- `payment_events`
- `invoices`
- `coupons`
- `redemption_codes`

补充：

- 人工收款订单、续费订单、补差价订单。
- 合同、付款凭证、发票附件可放入 MinIO。
- 订单创建后根据套餐权益写入客户可用额度。

### 交付中心

现有体系缺口较大，需要新增：

- `delivery_projects`：交付项目，关联客户、产品、订单、负责人、状态。
- `delivery_milestones`：里程碑，记录需求确认、素材收集、配置完成、验收、复盘等阶段。
- `delivery_tasks`：交付任务，按项目拆解到人。
- `delivery_files`：交付文件，附件走 MinIO。

交付中心主要服务代运营、GEO 项目、企业 Agent 搭建、API 接入支持等非纯自助产品。

### 用量中心

复用：

- `generation_tasks`
- `model_call_logs`
- `agent_call_logs`
- `geo_monitor_tasks`
- `point_transactions`
- `enterprise_quota_transactions`

补充：

- `api_keys`：客户 API Key，关联客户、套餐和额度。
- `api_call_logs`：API 调用日志。
- `usage_daily_summaries`：每日聚合，减少数据中心查询压力。
- `quota_accounts`：统一配额账户，可与现有 `point_accounts` 合并或逐步替换。
- `quota_transactions`：配额流水，覆盖充值、赠送、预扣、结算、退款、人工调整。

用量中心需要把不同产品统一映射为可计费指标：

- 文生图：生成任务数、图片张数、模型成本。
- API：请求数、成功数、失败数、Token 或点数消耗。
- Agent：对话次数、Token、成本、延迟。
- GEO：监测任务数、报告数、内容发布数。
- 代运营：项目服务项、席位数、交付节点。

API 用量计费需要兼容两种模式：

- 按量计费：按输入 Token、输出 Token、缓存命中、模型倍率、客户分组倍率计算。
- 按次计费：图片、视频、工具调用等按固定单价或固定配额消耗计算。

用量中心还需要提供客户可读的兼容接口：

```text
GET /v1/dashboard/billing/subscription
GET /v1/dashboard/billing/usage
```

这些接口用于兼容第三方 SDK 或客户自建面板查询订阅与用量。

### 分润中心

复用：

- `channel_agents`
- `commissions`
- `withdrawal_requests`
- `settlement_statements`

补充：

- `commission_rules`：分润规则，支持按产品、套餐、渠道等级、订单类型设置。
- 分润规则快照写入 `commissions.rule_snapshot`。
- 结算中心按周期生成结算单，提现审核通过后更新状态。

### 数据中心

复用：

- `orders`
- `payments`
- `commissions`
- `model_call_logs`
- `agent_call_logs`
- `generation_task_attempts`
- `usage_daily_summaries`

首批指标：

- 总收入、已收款、待续费、退款。
- 模型成本、Agent 成本、GEO 成本。
- 毛利、毛利率。
- 新增客户、付费客户、续费客户。
- 渠道贡献收入和分润成本。
- 产品收入结构和用量结构。

### 系统中心

复用：

- `users`
- `audit_logs`
- `model_providers`
- `payment_events`

补充：

- `roles`
- `permissions`
- `role_permissions`
- `user_roles`
- `system_settings`
- `brand_settings`
- `domain_bindings`
- `payment_channels`
- `api_provider_channels`
- `api_model_catalog`
- `customer_groups`

系统中心用于管理后台权限、品牌 Logo、站点名称、域名、支付参数、模型供应商配置和审计日志。

API 中转能力建议放在系统中心的“模型与渠道”子模块：

- 上游渠道：OpenAI、兼容 OpenAI 的第三方 API、其他模型供应商。
- 模型映射：平台模型名到上游模型名。
- 渠道测试：验证 Key、Base URL、模型可用性、响应延迟。
- 渠道策略：优先级、权重、故障切换、客户分组可用性。
- 计费规则：模型倍率、补全倍率、缓存倍率、按次价格、阶梯规则。

## 推荐数据库增量

第一批迁移建议新增以下表：

```sql
customers
products
product_entitlements
plan_products
delivery_projects
delivery_milestones
delivery_tasks
api_keys
api_call_logs
usage_daily_summaries
commission_rules
customer_groups
api_provider_channels
api_model_catalog
roles
permissions
role_permissions
user_roles
system_settings
brand_settings
domain_bindings
payment_channels
```

为了先快速形成可运行主控 SaaS，第一阶段可以只实现核心字段：

- 所有表都有 `id`、`status`、`created_at`、`updated_at`。
- 金额继续使用分。
- 历史快照使用 `jsonb`，例如套餐权益快照、分润规则快照、订单价格快照。
- 关键业务动作写入 `audit_logs`。

## 后端 API 边界

新增接口前缀：

```text
/api/v1/admin/*
```

第一阶段接口：

```text
GET    /api/v1/admin/overview
GET    /api/v1/admin/customers
GET    /api/v1/admin/customers/{id}
POST   /api/v1/admin/customers
PATCH  /api/v1/admin/customers/{id}

GET    /api/v1/admin/channel-agents
GET    /api/v1/admin/channel-agents/tree
POST   /api/v1/admin/channel-agents
PATCH  /api/v1/admin/channel-agents/{id}

GET    /api/v1/admin/products
POST   /api/v1/admin/products
PATCH  /api/v1/admin/products/{id}

GET    /api/v1/admin/plans
POST   /api/v1/admin/plans
PATCH  /api/v1/admin/plans/{id}

GET    /api/v1/admin/orders
GET    /api/v1/admin/orders/{id}
POST   /api/v1/admin/orders
POST   /api/v1/admin/orders/{id}/mark-paid
POST   /api/v1/admin/orders/{id}/renew

GET    /api/v1/admin/delivery-projects
POST   /api/v1/admin/delivery-projects
PATCH  /api/v1/admin/delivery-projects/{id}

GET    /api/v1/admin/usage
GET    /api/v1/admin/commissions
GET    /api/v1/admin/analytics
GET    /api/v1/admin/system/settings
PATCH  /api/v1/admin/system/settings

GET    /api/v1/admin/api/provider-channels
POST   /api/v1/admin/api/provider-channels
PATCH  /api/v1/admin/api/provider-channels/{id}
POST   /api/v1/admin/api/provider-channels/{id}/test

GET    /api/v1/admin/api/models
POST   /api/v1/admin/api/models
PATCH  /api/v1/admin/api/models/{id}

GET    /api/v1/admin/api/keys
POST   /api/v1/admin/api/keys
PATCH  /api/v1/admin/api/keys/{id}

GET    /api/v1/admin/customer-groups
POST   /api/v1/admin/customer-groups
PATCH  /api/v1/admin/customer-groups/{id}
```

后续接口再补权限、支付通道、域名、品牌和审计日志管理。

## 前端实现方案

建议新增 `admin-web`，使用 Vue 3 + TypeScript + Vite，做桌面 SaaS 控制台。

不建议用现有移动优先的 AI 创作页面承载主控后台。主控 SaaS 需要表格、筛选、详情抽屉、统计卡片和权限型导航，桌面管理端会更稳定。

第一版页面：

- `/admin`：数据中心首页。
- `/admin/customers`：客户中心。
- `/admin/channel-agents`：代理商中心。
- `/admin/products`：产品中心。
- `/admin/plans`：套餐中心。
- `/admin/orders`：订单中心。
- `/admin/delivery`：交付中心。
- `/admin/usage`：用量中心。
- `/admin/commissions`：分润中心。
- `/admin/system`：系统中心。

界面原则：

- 主控后台要信息密度高、稳定、便于扫描。
- 以列表、筛选、详情页、抽屉和操作菜单为主。
- 不做营销式首页。
- 所有关键列表保留状态、负责人、更新时间、金额、归属渠道等字段。

## Docker 实现方式

推荐第一版继续使用单应用容器：

```text
xianzhi-ai
  /app/public/app    产品端前端构建产物
  /app/public/admin  主控端前端构建产物
  Go API             托管 /、/admin、/api/v1/*

postgres
redis
rabbitmq
minio
```

访问路径：

```text
http://localhost:3100/       产品端
http://localhost:3100/admin  主控 SaaS
```

后续如果主控端体量变大，可以拆成独立 `admin-web` 容器，但第一版不建议增加部署复杂度。

## 开发阶段

### 阶段 1：主控 SaaS 骨架

目标：能登录后台、看到经营概览和导航。

- 新增 `admin-web`。
- Go 服务托管 `/admin`。
- 新增后台权限基础模型。
- 新增 `/api/v1/admin/overview`。
- 数据中心首页展示收入、订单、客户、渠道、用量、成本、利润的基础指标。

### 阶段 2：交易与产品闭环

目标：完成“卖什么、卖给谁、多少钱、谁带来的”。

- 客户中心。
- 代理商中心。
- 产品中心。
- 套餐中心。
- 订单中心。
- 订单与套餐权益写入客户额度。

### 阶段 3：交付与用量闭环

目标：完成“客户买了以后怎么交付、用了多少”。

- 交付中心。
- 用量中心。
- API Key 管理。
- Agent、GEO、文生图、API 的统一用量聚合。

### 阶段 4：财务与系统治理

目标：完成“收入、成本、利润、分润、权限、品牌、支付”。

- 分润中心。
- 数据中心完整看板。
- 系统中心。
- 品牌、域名、支付配置。
- 审计日志。

## 第一版建议范围

为了快速形成可确认、可运行的主控 SaaS，第一版建议做：

- 客户中心：列表、详情、新建、编辑、状态、归属渠道。
- 代理商中心：一级/二级渠道树、列表、审核、启停。
- 产品中心：文生图、API、Agent、GEO、代运营登录产品目录。
- 套餐中心：价格、权益、可售状态。
- 订单中心：订单列表、手工创建、标记收款、续费。
- 用量中心：按客户和产品查看用量。
- 数据中心：首页指标。
- 系统中心：角色权限基础版、品牌基础配置。
- 交付中心和分润中心：先做导航、列表和基础详情，第二阶段增强。

## 风险与处理

| 风险 | 说明 | 处理方式 |
| --- | --- | --- |
| Go API 与数据库脱节 | 当前 Go API 仍以演示 JSON 数据为主 | 第一阶段同时补 PostgreSQL 访问层，避免只做静态页面 |
| 产品权益口径不统一 | 文生图、API、Agent、GEO 的计量单位不同 | 用产品权益表统一抽象，订单写入权益快照 |
| 分润历史不可追溯 | 分润规则后续会变 | 佣金记录必须保存规则快照 |
| 后台权限过粗 | 主控 SaaS 涉及财务、渠道、系统配置 | 第一版实现 RBAC 基础模型，后续细化到按钮级权限 |
| Docker 构建变复杂 | 新增后台前端会影响构建链路 | 第一版仍使用单应用容器，减少部署复杂度 |

## 结论

主控 SaaS 与现有先知 AI 产品体系高度匹配，应该作为当前平台的运营管理层扩展实现。

推荐确认后按“后台骨架 + 经营闭环 + 交付用量 + 财务系统”的顺序开发。第一版重点是让平台能从主控后台完成客户、渠道、产品、套餐、订单、用量和数据总览的管理，再逐步增强交付和分润。

## 确认后开发入口

确认方案后，第一轮开发建议按以下顺序落地，保证每一步都能运行和验证。

### 1. 数据库迁移

新增迁移文件：

```text
database/migrations/020-admin-master-control.sql
```

首批表：

- `customers`
- `products`
- `product_entitlements`
- `plan_products`
- `delivery_projects`
- `delivery_milestones`
- `api_keys`
- `quota_accounts`
- `quota_transactions`
- `usage_daily_summaries`
- `commission_rules`
- `customer_groups`
- `api_provider_channels`
- `api_model_catalog`
- `roles`
- `permissions`
- `role_permissions`
- `user_roles`
- `system_settings`
- `brand_settings`

第一版可以暂不新增 `delivery_tasks`、`api_call_logs`、`domain_bindings`、`payment_channels` 的完整实现，只保留系统设置中的占位配置，降低第一轮开发复杂度。`api_call_logs` 后续可以从 `model_call_logs`、`agent_call_logs` 和网关日志中抽象出来。

### 2. Go 管理 API

新增或扩展文件：

```text
backend-go/internal/httpserver/admin_types.go
backend-go/internal/httpserver/admin_api.go
backend-go/internal/httpserver/admin_store.go
backend-go/internal/httpserver/server.go
```

第一批接口：

- `GET /api/v1/admin/overview`
- `GET /api/v1/admin/customers`
- `GET /api/v1/admin/channel-agents`
- `GET /api/v1/admin/channel-agents/tree`
- `GET /api/v1/admin/products`
- `GET /api/v1/admin/plans`
- `GET /api/v1/admin/orders`
- `GET /api/v1/admin/delivery-projects`
- `GET /api/v1/admin/usage`
- `GET /api/v1/admin/commissions`
- `GET /api/v1/admin/system/settings`
- `GET /api/v1/admin/api/provider-channels`
- `GET /api/v1/admin/api/models`
- `GET /api/v1/admin/api/keys`
- `GET /api/v1/admin/customer-groups`

第一轮先实现读取和基础聚合；创建、编辑、审核、标记收款等写操作放在第二轮，避免在数据模型没有跑通前引入太多状态变化。

API 中转相关接口可在第一轮先做读取：

- 模型目录列表。
- 上游渠道列表。
- API Key 列表。
- 调用日志与配额流水概览。

渠道测试、模型映射编辑、Key 创建、分组倍率配置建议放到第二轮。

### 3. 主控前端

新增目录：

```text
admin-web
```

首批页面：

- `Dashboard`：数据中心首页。
- `Customers`：客户中心。
- `ChannelAgents`：代理商中心。
- `Products`：产品中心。
- `Plans`：套餐中心。
- `Orders`：订单中心。
- `Delivery`：交付中心。
- `Usage`：用量中心。
- `Commissions`：分润中心。
- `SystemSettings`：系统中心。

第一版后台以桌面端为主，采用左侧导航、顶部状态栏、表格、筛选、指标卡和详情抽屉。

### 4. Docker 构建

调整：

```text
Dockerfile
compose.yml
package.json
```

目标：

- 构建产品端 `apps/user-uni`。
- 构建主控端 `admin-web`。
- Go 服务托管 `/` 和 `/admin`。
- Docker 仍通过 `http://localhost:3100` 对外提供服务。

### 5. 验收标准

第一轮完成后，需要满足：

- `npm.cmd run docker:up` 能启动完整环境。
- `http://localhost:3100/` 产品端仍可访问。
- `http://localhost:3100/admin` 主控端可访问。
- `GET /api/v1/admin/overview` 返回收入、成本、利润、客户、订单、用量概览。
- 主控端 10 个中心的导航全部存在。
- 客户、代理商、产品、套餐、订单、交付、用量、分润、系统页面能从真实 API 读取数据。
- 现有生成任务接口不回退、不破坏。
- `npm.cmd test` 通过，或明确记录本地工具链阻塞原因。

## 待确认取舍

确认开发前建议只确认 3 件事：

1. 主控端是否继续由独立 `admin-vue` 承载，而不是并入用户端 `apps/user-uni`。
2. 第一版是否按“先读数据和看板，第二轮再做写操作”推进。
3. 交付中心和分润中心第一版是否先做基础列表和详情，第二轮再增强流程。

推荐选择：

- 使用独立 `admin-web`。
- 第一版先完成真实数据读取、导航、看板和列表。
- 写操作从第二轮开始逐个补齐，优先补客户、代理、产品、套餐、订单。

## 需求对照审计

| 原始要求 | 方案落点 | 当前结论 |
| --- | --- | --- |
| 客户中心：管所有客户 | `customers`、`users`、`enterprises`、客户列表和详情 | 已覆盖 |
| 代理商中心：管所有渠道、一级渠道和二级渠道 | `channel_agents`、渠道树、渠道客户归属、渠道审核 | 已覆盖 |
| 产品中心：管文生图/API/Agent/GEO/代运营登录 | `products`、`product_entitlements`、API 模型目录、GEO/Agent 对接 | 已覆盖 |
| 套餐中心：管价格和权益 | `membership_plans`、`plan_products`、权益快照、客户分组倍率 | 已覆盖 |
| 订单中心：管收款和续费 | `orders`、`payments`、人工订单、标记收款、续费接口 | 已覆盖 |
| 交付中心：管项目进度 | `delivery_projects`、`delivery_milestones`、交付状态和负责人 | 已覆盖 |
| 用量中心：管 API、对话量、GEO 任务 | `api_keys`、`usage_daily_summaries`、`agent_call_logs`、`geo_monitor_tasks` | 已覆盖 |
| 分润中心：管代理收益 | `commissions`、`commission_rules`、`settlement_statements`、提现 | 已覆盖 |
| 数据中心：看收入、成本、利润 | `/api/v1/admin/overview`、`analytics`、订单/成本/佣金聚合 | 已覆盖 |
| 系统中心：管权限、域名、支付、品牌 | RBAC、`system_settings`、`brand_settings`、支付和域名配置 | 已覆盖 |
| Docker 方式实现 | 复用 `compose.yml`，单应用容器托管产品端和 `/admin` | 已覆盖 |
| 和现有产品体系对接 | 复用现有用户、套餐、订单、代理、模型、Agent、GEO、生成任务表 | 已覆盖 |
| 参考 New API 中转站方案 | 增加渠道管理、模型映射、分组倍率、配额、API Key、OpenAI 兼容查询 | 已覆盖 |

## 确认后第一轮开发验收清单

第一轮开发完成时，至少应能验证：

- `http://localhost:3100/` 产品端仍正常访问。
- `http://localhost:3100/admin` 主控 SaaS 正常访问。
- 主控端左侧导航包含 10 个中心。
- 数据中心能展示收入、成本、利润、客户、订单、用量、渠道概览。
- 客户中心能读取客户列表。
- 代理商中心能读取一级/二级渠道结构。
- 产品中心能读取文生图、API、Agent、GEO、代运营登录产品目录。
- API 产品能读取模型目录、上游渠道和 API Key 列表。
- 套餐中心能读取价格和权益。
- 订单中心能读取收款和续费相关订单。
- 交付中心能读取项目进度。
- 用量中心能读取 API、Agent 对话、GEO 任务用量。
- 分润中心能读取代理收益和结算概览。
- 系统中心能读取权限、品牌、支付、域名基础配置。
- Docker 构建链路包含产品端、主控端和 Go API。
- 现有 `GET /api/v1/generation-tasks`、`POST /api/v1/generation-tasks`、`GET /api/v1/assets` 不被破坏。
