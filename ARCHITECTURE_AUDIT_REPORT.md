# 先知AI 管理后台分析仪表盘 - 架构审计报告

## 执行摘要

本报告对 `先知AI-admin-analytics` 仓库进行全面架构审计，评估现有能力对构建"平台运营监控中心"的支撑程度。目标：管理员在 30 秒内看懂平台健康度（日新增用户、活跃用户、AI 用户、图文生成、积分消耗、Token 使用、模型榜单、供应商成功率、模型成本、失败任务、7 日趋势）。

**总体结论**：现有后端数据模型完备，已有基础管理 API 与前端概览页，**核心数据源均已具备**，但缺乏专用的分析聚合 API、趋势时序查询、模型/供应商维度的成功率与成本分析。无重大架构阻塞，**可直接进入 V1 实施**。

---

## 1. 数据层审计

| 能力项 | 现状 | 分类 | 说明 |
|--------|------|------|------|
| 用户表 (`users` / `xz_users`) | 完整字段：id, email, name, role, status, plan_id, created_at, subscription_expires_at | **EXISTING** | 支持日新增、活跃、流失统计 |
| 积分账户 (`point_accounts` / `xz_point_accounts`) | available/frozen/version, 事务表 `point_transactions` 完整 | **EXISTING** | 可算日消耗、充值、冻结、释放 |
| 积分流水 (`point_transactions`) | type, available_delta, frozen_delta, reference_type/id, created_at | **EXISTING** | 明细级可追溯 |
| 计费事件 (`xz_billing_events`) | module_code, task_id, metric_code, quantity, unit_amount_cents, amount_cents, point_cost, balance_before/after, model, status, occurred_at | **EXISTING** | 核心计量粒度，含模型维度 |
| Token 流水 (`xz_token_records`) | change_type, amount, balance_before/after, order_id, created_at | **EXISTING** | 订阅/赠送/扣减全链路 |
| 生成任务 (`xz_generation_tasks`) | type, model, status, point_cost, supplier_cost, estimated_margin, provider_channel, created_at, worker_finished_at | **EXISTING** | 含上游成本、毛利、供应商通道 |
| 模型调用日志 (`model_call_logs`) | provider_code, model_code, status, latency_ms, cost_cents, provider_request_id, error, created_at | **EXISTING** | **关键**：供应商成功率、延迟、成本的原始来源 |
| 资产表 (`xz_assets`) | media_type(image/video), task_id, created_at | **EXISTING** | 区分图/视频生成量 |
| 订单 (`xz_orders`) | amount_cents, status, paid_at, price_snapshot | **EXISTING** | 收入侧核心表 |
| 佣金/分润 (`xz_commissions`, `xz_withdrawals`) | amount_cents, rate, status, rule_snapshot | **EXISTING** | 渠道成本侧 |
| 模型定义/供应商 (`model_definitions`, `model_providers`, `model_pricing_rules`) | 代码、能力、点数成本、计费来源 | **EXISTING** | 模型元数据字典 |
| 企业/多租户 (`enterprises`, `enterprise_members`, `xz_user_business_identities`) | 完整 | **EXISTING** | 租户维度聚合需要 |

**缺口**：
- 无预聚合快照表（日/周/月粒度），大时段查询需全表扫描
- `model_call_logs` 无 `task_type` 字段（需关联 `generation_tasks` 推导）
- 无 `dau/wau/mau` 预计算表

---

## 2. 后端 API 审计

| 端点 | 能力 | 分类 | 备注 |
|------|------|------|------|
| `GET /admin/overview` | 收入/成本/利润/客户/订单/渠道/用量 7 个卡片 | **EXISTING** | 仅聚合总量，**无趋势、无模型/供应商拆解** |
| `GET /admin/usage` | apiCalls/agentChats/geoTasks/assets 按 product 分组 | **PARTIAL** | 无时间范围、无模型维度、无成功率 |
| `GET /admin/generation-tasks` | 完整任务列表（含 supplier_cost, provider_channel） | **EXISTING** | 可二次聚合，但无分页/筛选参数 |
| `GET /admin/token-records` | Token 流水列表 | **EXISTING** | 无聚合视图 |
| `GET /admin/billing/overview` | MRR/用量收入/待开票/钱包余额 | **PARTIAL** | 计费视角，非运营视角 |
| `GET /admin/billing/events` | 计费事件流水（含 model, metric_code, point_cost） | **EXISTING** | 可作为模型成本/用量来源 |
| `GET /admin/ai/logs` | **不存在** | **MISSING** | 需新建：模型调用日志聚合 |
| `GET /admin/analytics/*` | **全缺失** | **MISSING** | V1 需新建的核心端点族 |

**现有聚合逻辑**（`admin_api.go:30-82`）：
- revenue = 所有 paid order amount_cents 求和
- modelCost = generationTasks.count * 12 + agentCalls cost 之和（硬编码估算，**不准**）
- commissionCost = commissions.amount_cents 求和
- 无**真实** supplier_cost 汇总，无模型/供应商维度拆解

---

## 3. 前端现状审计

| 组件/页面 | 能力 | 分类 | 备注 |
|-----------|------|------|------|
| `AdminOverviewDomain.vue` | 4 统计卡片、流量来源环图、周活跃柱图、月收支趋势线图 | **PARTIAL** | 仅**静态演示数据**，未接真实 API；无模型榜单、供应商状态、Token/积分摘要 |
| `admin.ts` store | 模块注册、数据缓存、懒加载 | **EXISTING** | 工程化完备 |
| `adminWorkspaces.ts` API 客户端 | customer360, orderTimeline, globalSearch, exception, experience | **EXISTING** | 无 analytics 相关端点 |
| ECharts/图表库 | **未引入** | **MISSING** | 当前用 SVG 手绘，V1 需引入 ECharts |

---

## 4. 指标定义与数据源映射（核心交付物）

| 指标 | 口径定义 | 主表 | 聚合 SQL 片段 |
|------|----------|------|---------------|
| **日新增用户** | `DATE(created_at) = today` 且 `role != 'SUPER_ADMIN'` | `users` / `xz_users` | `count(*) filter (where date(created_at) = current_date)` |
| **DAU** | 当日有任一 `generation_tasks`/`agent_call_logs`/`billing_events` 的 distinct user_id | `generation_tasks`, `agent_call_logs`, `xz_billing_events` | `count(distinct user_id) from (select user_id, date(created_at) as d from generation_tasks union all ...) where d = current_date` |
| **WAU/MAU** | 同 DAU，窗口 7/30 天 | 同上 | 同上 |
| **AI 用户数** | 当日 `generation_tasks.type in ('TEXT_TO_IMAGE',...) or agent_calls > 0` 的 distinct user_id | `generation_tasks`, `agent_call_logs` | 同上加类型过滤 |
| **图片生成量** | `generation_tasks.type in ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') and status='SUCCEEDED'` | `generation_tasks` | `count(*) filter (where type in (...) and status='SUCCEEDED' and date(created_at)=...)` |
| **视频生成量** | `type in ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO')` | `generation_tasks` | 同上 |
| **积分消耗** | `point_transactions.available_delta < 0` 绝对值求和 / `billing_events.point_cost` 求和 | `point_transactions`, `xz_billing_events` | `sum(abs(available_delta)) filter (where available_delta<0 and date(created_at)=...)` |
| **Token 使用量** | `agent_call_logs.token_usage` 求和 | `agent_call_logs` | `sum(token_usage) filter (where date(created_at)=...)` |
| **模型调用榜单** | 按 `model_code` 分组：调用量、成功率、平均延迟、总成本 | `model_call_logs` JOIN `generation_tasks` | `select model_code, count(*), avg(latency_ms), sum(cost_cents), count(*) filter(where status='SUCCESS')/count(*) from model_call_logs where date(created_at)=... group by model_code` |
| **供应商成功率** | 按 `provider_code`：成功数/总数、平均延迟、总成本 | `model_call_logs` | 同上按 provider_code 分组 |
| **模型成本** | `model_call_logs.cost_cents` 求和 / `generation_tasks.supplier_cost` 求和 | 两表均可 | 优先用 `model_call_logs.cost_cents`（实时上游成本） |
| **失败任务** | `generation_tasks.status in ('FAILED','ERROR')` 按 `failure_reason` / `model` / `provider_channel` 分桶 | `generation_tasks` | `select failure_reason, model, provider_channel, count(*) from generation_tasks where status in (...) and date(created_at)=... group by 1,2,3` |
| **7 日趋势** | 上述各核心指标按天分桶，最近 7 天 | 所有主表 | 通用：`select date(created_at) as d, <metric> from <table> where created_at >= now() - interval '7 days' group by d order by d` |

---

## 5. 缺口分类汇总

| 类别 | EXISTING | PARTIAL | MISSING |
|------|----------|---------|---------|
| **数据表** | 11 | 0 | 2 (预聚合表、DAU预计算) |
| **后端 API** | 6 | 3 | 8 (analytics 端点族) |
| **前端组件** | 1 (基础框架) | 1 (概览页骨架) | 5 (趋势图、模型榜单、供应商卡、Token/积分卡、7日趋势) |
| **图表库** | 0 | 0 | 1 (ECharts) |
| **权限/RBAC** | 现有 admin 角色体系 | - | 需在路由守卫增加 `analytics:view` |

---

## 6. V1 实施范围确认（按需求文档）

**必须包含**：
1. ✅ 后端：`/api/admin/analytics/overview`、`/users`、`/generation`、`/tokens`、`/points`、`/models`、`/providers`、`/trends?days=7`
2. ✅ 索引：`model_call_logs(created_at, provider_code, model_code, status)`、`generation_tasks(created_at, type, status, model)`、`billing_events(occurred_at, module_code, model)`
3. ✅ 前端：`AdminAnalyticsDashboard.vue` 单页，含 5 区块（概览卡片、7日趋势、模型榜单、供应商状态、Token/积分摘要）
4. ✅ 时区：`Asia/Shanghai`，前后端统一
5. ✅ 权限：仅 `SUPER_ADMIN` / `AI_OPERATOR` / `FINANCE`
6. ✅ 测试：后端单测覆盖聚合 SQL，前端 Vitest + MSW

**不在 V1**：
- 导出 CSV/PDF
- 实时 WebSocket 推送
- 多租户下钻
- 预聚合物化视图（后续性能优化）

---

## 7. 风险与对策

| 风险 | 等级 | 对策 |
|------|------|------|
| `model_call_logs` 无 `task_type` 需 JOIN `generation_tasks` 查询变慢 | 中 | V1 先 JOIN，后续加 `task_type` 冗余字段或物化视图 |
| 大时段聚合全表扫描 | 中 | 先上线，观察 P99；超 500ms 再加日快照表 + 索引 |
| 前端 ECharts 首包体积 | 低 | 按需引入 `echarts/core` + `LineChart` `BarChart` `PieChart` 等模块 |
| 权限遗漏导致数据泄露 | 高 | 中间件统一校验 `requireAdminRole(['SUPER_ADMIN','AI_OPERATOR','FINANCE'])` |

---

## 8. 结论

**无重大架构阻塞**。所有核心原始数据已落库，现有 AdminData() 加载机制可复用，仅需：
1. 新建 `analytics_api.go` 聚合查询层（避免污染现有 `admin_api.go`）
2. 新增 2-3 个复合索引
3. 前端新建 `AdminAnalyticsDashboard.vue` + 路由 + ECharts 引入
4. 补全单测与集成测

**建议立即开始 V1 实施**。