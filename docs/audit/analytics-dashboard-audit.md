# 分角色运营驾驶舱只读审计与实施规划

## Executive Summary

当前 Analytics 成熟度：6 / 10

是否已经具备驾驶舱基础：
具备单角色基础（平台管理员），不具备多角色分权基础。

最大优势：
底座扎实。系统内已有 8 个平台级 Analytics 接口、1 套 AdminAnalyticsDashboard 页面、1 个基于 PostgreSQL/RabbitMQ 的运行态指标采集器（`/metrics`），以及基于 `xz_admin_exception_cases` 的异常中心，核心业务主表在历史演进中已沉淀 `tenant_id`、`agent_id` 与 `operation_center_id` 字段。

最大缺口：
数据隔离层完全缺失。`/api/v1/admin/analytics/*` 仅依赖平台管理员后台入口做粗粒度访问控制，底层 SQL 全部为全表裸扫，无任何基于身份推导的数据隔离（Scope），一旦向运营中心、代理商、企业租户开放，将形成 P0 级全平台数据越权泄漏。

---

## Existing Dashboard

现有前端面板位于 `admin-vue/src/components/admin/AdminAnalyticsDashboard.vue`。

已覆盖指标：
- 今日概览：日新增用户、DAU/WAU/MAU、今日 AI 用户、今日图片/视频生成数、积分消耗、Token 使用量、收入/成本、失败任务数、成功率、平均延迟。
- 7 日趋势折线图：涵盖新增、活跃、生成、积分、Token、收支等 13 个序列（当前界面激活展示 5 项）。
- 模型排名表：模型代码、调用次数、成功率、平均延迟、总成本。
- 供应商状态表：供应商代码、调用次数、成功率、平均延迟、总成本。
- Token 分析：今日/7日/30日总量与 Top 用户排行。
- 积分分析：今日消耗/充值/净变化/可用余额/冻结余额及 7 日趋势。

现有实现缺陷：
- 组件独立使用手写 `<table>`，未复用 `AdminDataTable`。
- ECharts 实例在组件内部直接初始化管理，缺少跨角色通用的图表包装器。
- 缺失业务细分：无 PPT、Agent、知识库分类数据。
- 缺失运行态透视：无处理中任务（PROCESSING）、Provider unknown、DLQ、Outbox 等指标。
- 缺失分权支持：请求路径写死 `/admin/analytics/*`，无角色与范围参数感知。

---

## Existing Analytics APIs

挂载于 `backend-go/internal/httpserver/server.go:851`，由 `analyticsGroup` 统一提供服务：

1. `GET /api/v1/admin/analytics/overview` -> `AnalyticsOverview`
2. `GET /api/v1/admin/analytics/users` -> `AnalyticsUsers`
3. `GET /api/v1/admin/analytics/generation` -> `AnalyticsGeneration`
4. `GET /api/v1/admin/analytics/tokens` -> `AnalyticsTokens`
5. `GET /api/v1/admin/analytics/points` -> `AnalyticsPoints`
6. `GET /api/v1/admin/analytics/models` -> `AnalyticsModels`
7. `GET /api/v1/admin/analytics/providers` -> `AnalyticsProviders`
8. `GET /api/v1/admin/analytics/trends` -> `AnalyticsTrends`

实现特征：
- 入参对象 `AnalyticsQueryParams` 仅包含 `days`、`timezone`、`startDate`、`endDate`。
- 底层存储实现 `backend-go/internal/httpserver/analytics_postgres_v1.go` 中的查询无任何动态过滤条件，所有统计均在全平台无边界执行。

---

## Existing Metrics

系统在 `backend-go/internal/httpserver/async_canary_observability.go` 实现了基于 Prometheus 文本协议的采集器，挂载于 `/metrics` 端点：

1. `xianzhi_async_canary_outbox_pending`：Outbox 待投递事件深度。
2. `xianzhi_async_canary_outbox_failed`：Outbox 投递失败事件数。
3. `xianzhi_async_canary_outbox_oldest_pending_age_seconds`：最长待处理事件存活秒数。
4. `xianzhi_async_canary_outbox_publish_retries_total`：Outbox 累计发布重试次数。
5. `xianzhi_async_canary_rabbitmq_queue_depth`：生成业务主队列排队数。
6. `xianzhi_async_canary_rabbitmq_retry_queue_depth`：重试队列深度。
7. `xianzhi_async_canary_rabbitmq_dlq_depth`：死信队列（DLQ）深度。
8. `xianzhi_async_canary_rabbitmq_consumers`：当前活跃消费端数量。
9. `xianzhi_async_canary_video_rabbitmq_queue_depth`：视频任务主队列深度。
10. `xianzhi_async_canary_video_rabbitmq_retry_queue_depth`：视频重试队列深度。

风险与异常数据源：
- `xz_admin_exception_cases`（`backend-go/internal/httpserver/admin_experience.go`）：支持状态流转（`OPEN` / `IN_PROGRESS` / `RESOLVED`）与 SLA 到期时间追踪，前端由 `AdminExceptionCenter.vue` 消费。

---

## Current Role & Scope Model

角色体系定义于 `backend-go/internal/httpserver/auth_api.go`：
- 当前平台管理员枚举：`SUPER_ADMIN`、`ENTERPRISE_OPERATOR`、`CERTIFICATION_REVIEWER`、`FINANCE`、`RISK_MANAGER`、`CUSTOMER_SERVICE`。
- 权限判定依据：`permissionsForIdentity(role, hasAgent, hasOperationCenter)`。
- 存在断层：代码中尚未定义系统级的 `OPERATION_CENTER`、`AGENT`、`TENANT` 纯粹数据范围角色实体，而是散落在企业中心、渠道代理等独立模块中。

各主表数据列现状：
- `xz_generation_tasks`：已具备 `tenant_id`、`agent_id`、`operation_center_id`、`module_code`（迁移 028/044）。
- `xz_billing_events`：已具备 `tenant_id`、`agent_id`、`operation_center_id`、`module_code`（迁移 028/088）。
- `xz_users`：仅有基础用户字段与 `referred_by`，**无** `tenant_id` / `agent_id` / `operation_center_id` 列。
- `xz_tenants` / `xz_tenant_members`：提供租户与成员绑定关系（迁移 033）。
- `xz_channel_agents`：具备代理层级与邀请链（迁移 021）。
- `customers`：具备 `enterprise_id` 与 `channel_agent_id` 归属（迁移 020）。

---

## Data Scope Design

### 统一设计原则
- 客户端传参绝对不可信任：禁止客户端向统计端点上传 `tenant_id`、`agent_id` 或 `operation_center_id`。
- 鉴权上下文注入：由网关或 RBAC 中间件在服务端根据 Session/Token 解析出 `AnalyticsScope`，注入 `context.Context`，在 Repository 层作为强过滤条件。

### 四级 Scope 规则

1. **PLATFORM**
   - 目标角色：`SUPER_ADMIN` 及具备全局权限的平台管理员。
   - 过滤条件：无。统计全平台全量数据。

2. **OPERATION_CENTER**
   - 目标角色：运营中心主账号。
   - 过滤条件：`operation_center_id = :resolved_oc_id` 或下属代理商列表关联的所有客户。

3. **AGENT**
   - 目标角色：渠道代理商。
   - 过滤条件：`agent_id = :resolved_agent_id`。

4. **TENANT**
   - 目标角色：企业客户管理员。
   - 过滤条件：`tenant_id = :resolved_tenant_id`。

---

## Metric Data Source Map

| 指标 | 数据来源 | 表/Metric | 是否已有 | 是否需要聚合 | Scope |
|---|---|---|---|---|---|
| 今日新增用户 | PostgreSQL | `xz_users` + 关联映射 | 部分（缺归属） | 否 | 全部 |
| 活跃用户 (DAU/WAU/MAU) | PostgreSQL | `xz_generation_tasks` / `agent_call_logs` / `xz_billing_events` | 是 | 是 (UNION 聚合) | 全部 |
| AI 总生成量 | PostgreSQL | `xz_generation_tasks` | 是 | 否 | 全部 |
| 细分生成量 (图/视频/PPT/Agent/知识库) | PostgreSQL | `xz_generation_tasks.type` + `module_code` | 部分 (缺细分映射) | 否 | 全部 |
| 任务成功率 / 失败量 | PostgreSQL | `xz_generation_tasks.status` | 是 | 否 | 全部 |
| 模型调用分析 | PostgreSQL | `model_call_logs` | 是 | 否 | PLATFORM (成本隐藏) |
| 供应商分析 | PostgreSQL | `model_call_logs.provider_code` | 是 | 否 | PLATFORM (不可对外) |
| 积分消耗 / 发放 / 充值 | PostgreSQL | `xz_billing_events` | 是 | 否 | 全部 |
| 积分可用 / 冻结余额 | PostgreSQL | `xz_user_wallets` | 是 | 否 | 全部 |
| 充值收入 | PostgreSQL | `xz_billing_events.amount_cents` | 是 | 否 | 全部 (依权限脱敏) |
| 当前处理中任务 (PROCESSING) | PostgreSQL | `xz_generation_tasks.status='PROCESSING'` | 否 | 否 | PLATFORM / TENANT |
| 运行态队列 / DLQ / Outbox | Prometheus / 内存 | `xianzhi_async_canary_*` | 是 | 否 | PLATFORM |
| 运行态异常任务 | PostgreSQL | `xz_admin_exception_cases` | 是 | 否 | PLATFORM |

---

## Missing Metrics

业务统计缺失：
- PPT、Agent、知识库独立调用量及占比切片。
- 当前处于 `PROCESSING` 状态且超时的任务数统计。
- 任务环比变动百分比与 7 日 Mini-chart 统计结构。

运行态技术缺失（仅面向平台管理员）：
- 长时间处于 `RESERVED` 状态未结算的积分流水。
- 业务任务长时间 `PROCESSING`（例如 > 15 分钟）卡死报警计数。
- Provider Execution 状态为 `unknown` 的持久化监控计数。

---

## Security / RBAC Risks

1. **P0 - 全局数据无防护越权（越权查看）**
   - 当前 `/admin/analytics/*` 内部 SQL 全部为全表聚合。若将这些接口下发给非平台管理员，运营中心、代理商将能直接查看全平台收入、其他运营中心业绩以及全量商业敏感数据。
2. **P1 - 客户端伪造参数越权风险**
   - 若在重构中允许前端在 Query/Body 传入 `tenant_id` 等参数作为查询范围，必须坚决在服务端阻断，禁止基于客户端输入覆盖权限推导出的范围。
3. **P1 - 跨租户成员反查失真**
   - `xz_users` 缺乏直接归属字段，若通过多表关联统计新增用户，若关联设计不严谨容易出现重复计数或漏计。

---

## SQL Cost Risks

1. **多源 UNION DAU 查询成本**
   - `AnalyticsOverview` 与 `AnalyticsTrends` 中每次计算活跃用户均执行三表 `UNION ALL`（`xz_generation_tasks`、`agent_call_logs`、`xz_billing_events`）后再求 `DISTINCT`，在大数据量下易引发 CPU 峰值。
2. **文本时间戳转换导致索引失效**
   - `NULLIF(created_at,'')::timestamptz` 会导致无法直接利用 `created_at` 基础 B-Tree 索引。
3. **缺少针对 Scope 的复合索引**
   - 在追加 `tenant_id` / `agent_id` 过滤后，现有单列索引会导致大范围索引合并或全表扫描。需针对业务主表补充范围与时间复合索引。

---

## Dashboard Layout

```
[顶部第一层] 8 张核心 KPI 卡片
  1. 今日新增用户  2. 今日活跃用户  3. 今日 AI 生成量  4. 今日生成成功率
  5. 今日积分消耗  6. 今日收入      7. 处理中任务      8. 异常风险任务
  (每张卡片展示：今日绝对值、昨日数值、环比百分比、7日趋势 Mini-Line)

[第二层] 核心趋势分析
  - 7日 / 30日 核心指标走势（新增、活跃、生成、积分、收入）

[第三层] AI 应用类型与产出
  - 细分形态分布（文生图、图生图、文生视频、图生视频、PPT、Agent、知识库）
  - 各类别的调用量、成功率与平均耗时

[第四层] 模型与供应商透视（非管理员自动隐藏成本）
  - 模型榜单：调用频次、成功率、耗时、成本
  - 供应商榜单：调用分布、稳定性、平均延迟

[第五层] 财务与资产分析
  - 充值与积分发放/消耗流动瀑布流
  - 资产存储增长与文件产出

[第六层] 运行态基础设施（仅平台管理员可见）
  - Outbox 积压、RabbitMQ 队列深度、DLQ 死信数、Worker 处理吞吐

[第七层] 风险与告警列表（仅平台管理员可见）
  - 异常任务、卡死任务、积分冻结过久未释放、SLA 超期案例
```

---

## Platform Admin View

- 全览经营 + 运行态数据。
- 完整包含收入、供应商支出底价、利润率及毛利分析。
- 包含全部 Prometheus / RabbitMQ / Outbox 等系统级健康状态。

---

## Operation Center View

- 经营边界：仅统计属于自身运营中心以及下属代理商、终端企业/用户的数据。
- 屏蔽范围：不可见全平台总收入、供应商底层调用成本、技术运维监控（RabbitMQ/DLQ 等）。
- 重点展示：下属代理商发展进度、终端客户消耗排行榜、自身分润测算。

---

## Agent View

- 经营边界：仅统计当前代理商直属拓客的终端客户与企业。
- 屏蔽范围：其他代理商数据、运营中心全局、平台底价成本、技术运维监控。
- 重点展示：客户活跃度、今日拉新、客户生成量、代理佣金收益。

---

## Tenant View

- 经营边界：严格限制在当前企业 `tenant_id` 范围内。
- 屏蔽范围：平台所有商业模型、代理层级、系统运行指标。
- 重点展示：企业员工数、活跃员工、各应用类型消耗分布、企业积分消耗对账、配额使用率。

---

## Minimal V1

第一版必须保障的最小可用范围：
1. 服务端数据范围隔离引擎（`AnalyticsScope` 解析与下推）。
2. 用户指标（新增、DAU）。
3. AI 任务生成指标（按类型分布：生图、视频、PPT、Agent、知识库）。
4. 任务质量指标（总调用量、成功量、失败量、成功率）。
5. 积分与财务指标（消耗积分、充值金额、可用余额）。
6. 模型分析榜单（调用量、成功率）。
7. 异常风险识别（超期处理中任务）。
8. 平台管理员端核心驾驶舱落地，严格保持非管理员访问隔离。

---

## Database Changes Required

**DATABASE_CHANGES_REQUIRED = NO**

不需要新增业务字段迁移。主要业务表已在 `028-ai-capability-center.sql`、`044-enterprise-p0-safety.sql`、`088-runtime-projection-baseline-completion.sql` 等历史迁移中补全了 `tenant_id`、`agent_id` 与 `operation_center_id` 字段。

可选性能加固项（非阻断）：
可根据线上负载按需补充复合索引（如 `(tenant_id, created_at DESC)`），不影响业务逻辑接入。

---

## API Changes Required

1. **类型定义升级**：`AnalyticsQueryParams` 引入服务端内联解析的 `AnalyticsScope`，包含解析后的租户 ID 列表、代理商 ID 列表、运营中心 ID 列表及全局平台标记。
2. **上下文范围解析中间件**：基于当前登录用户的认证信息解析所属 Scope，拒绝外部客户端注入伪造。
3. **Repository 接口重构**：修改 `AnalyticsOverview`、`AnalyticsGeneration` 等 8 个数据访问函数，统一要求传递 `AnalyticsScope`。
4. **SQL 查询改造**：全表 SQL 增加 `WHERE (tenant_id = ANY(:tenants) OR :is_platform)` 等条件分支控制。
5. **新增运行态只读端点**：`GET /api/v1/admin/analytics/runtime`，专门为平台管理员输出队列深度、Outbox 积压等运维指标。

---

## Frontend Changes Required

1. **抽取统一卡片与图表容器**：规范化 KPI 卡片组件（数值、环比、Mini 走势）。
2. **表格复用**：废弃现有手写原生 HTML 表格，接入现有 `AdminDataTable`。
3. **视图角色适配**：根据当前登录角色自动屏蔽技术运维看板与敏感成本列。
4. **安全路由保护**：在 PR1 服务端 Scope 机制合入前，坚决不对运营中心、代理商、租户开放驾驶舱页面。

---

## PR Plan

### PR1: Analytics Scope 核心鉴权与后端指标接口改造 (FIRST_PR)
- 目标：建立安全边界，阻断越权漏洞。
- 内容：
  - 定义 `AnalyticsScope` 模型与从用户身份推导权限的解析链路。
  - 改造 8 个 Analytics 接口，接入 Scope 过滤，全表 SQL 加上动态范围条件。
  - 新增专用单元测试，验证各角色 Scope 解析与防越权隔离。
  - 保持现有前端入口权限不变，不对非管理员放行。

### PR2: 平台管理员运营驾驶舱升级 (Platform Admin Dashboard V1)
- 目标：交付平台管理员所需的“30 秒全局透视”面板。
- 内容：
  - 顶部 8 核心 KPI 卡片重构（含环比与趋势 Mini-chart）。
  - AI 应用类型按文生图/图生图/视频/PPT/Agent/知识库细化分类。
  - 接入 `AdminDataTable` 与图表组件重构。
  - 引入运维运行态与异常中心告警区块（仅管理员渲染）。

### PR3: 运营中心 / 代理商 / 企业租户分权驾驶舱落地
- 目标：向分角色下发驾驶舱。
- 内容：
  - 前端路由与菜单针对对应角色放开驾驶舱入口。
  - 前端根据当前登录角色自适应切换展示布局与列定义（屏蔽成本与运维信息）。
  - 接入运营中心/代理商专属分润指标，接入企业租户员工消耗排名。

---

## First Implementation Slice

**FIRST_PR = PR1 (Analytics Scope 核心鉴权与后端指标接口改造)**

**READY_TO_IMPLEMENT = YES**

唯一阻断项：
无阻断。底层数据字段已具备，不需要执行数据库结构迁移（Migration），可直接基于现有架构启动 PR1 实施。
