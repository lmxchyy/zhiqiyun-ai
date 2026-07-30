# Enterprise V1 技术债清单

> 状态：FROZEN
>
> 口径：只记录当前审计已确认或由现有实现直接推导的风险。临时处理是 V1 允许的受控边界，正式处理不自动进入 V1。

## 1. 优先级定义

| 优先级 | 定义 |
| --- | --- |
| P0 | 不处理就可能跨租户、错扣费、重复发放、错误分佣或无法上线 |
| P1 | 不处理会导致账务、展示、运营或回滚不完整；通常阻塞相关 V1 页面/能力 |
| P2 | V1 可通过明确限制运行，但必须列入后续专项 |

## 2. 技术债总表

| ID | 当前问题 | 影响 | V1 临时处理 | 正式处理 | 优先级 | 是否阻塞 V1 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-001 | 两套支付中心未完全统一 | 企业商品若双写会产生两套商品、订单、履约和对账口径 | 只扩展 Price Plan V2；`enterprise_plan` 保持禁用 | 支付中心统一专项，迁移商品/订单/履约/退款与对账 | P1 | 否，前提是严格单主链 |
| TD-002 | AI 创作页固定显示个人点数 | 页面主体与后端企业扣费不一致，可能误导用户 | 新增只读 Billing Subject 投影；企业上下文展示企业付费 | 统一所有创作入口的计费主体组件 | P0 | 是 |
| TD-003 | 企业消费页只有汇总和空态 | 企业无法查看成员、任务、模型和余额变化 | 接 Compute Ledger + Model Usage 分页 API | 建立统一企业成本分析和导出 | P1 | 是 |
| TD-004 | 管理后台企业明细读取旧人工调整表 | 看不到真实 AI DEBIT/REVERSAL，运营结论错误 | compute/transactions 改读 ledger/usage；人工表仅兼容 | 下线旧表主查询，保留审计关联或归档 | P0 | 是 |
| TD-005 | 企业消费佣金隔离缺失 | 企业生成/PPT 可因员工推荐关系产生代理佣金 | 在共享佣金调用前按 `billing_account_type` 硬短路并做零增量测试 | 事件化 Commission 白名单，只接受明确非企业事件 | P0 | 是 |
| TD-006 | lot 到期与钱包余额可能不一致 | wallet 显示有余额但有效 lot 不足，消费失败或对账漂移 | V1 企业额度包只发永久 lot；查询同时返回有效 lot 合计 | 到期任务原子标记 EXPIRED、扣钱包并写 expiry ledger | P1 | 否，前提是禁止到期商品 |
| TD-007 | `schema.sql` 与运行时 `xz_*` 双模型 | 设计或迁移可能改错表，产生无效实现 | 明确 V1 只扩展 Go 运行时使用的 `xz_*` | 统一 schema 生成/迁移基线并移除旧模型歧义 | P2 | 否 |
| TD-008 | 企业所谓 reserve 实际立即扣减 | 名称与财务语义不一致，长任务期间余额先下降 | 明示为立即 DEBIT + 失败 REVERSAL；不使用 `frozen_points` | 独立 Reservation V2 状态机 | P2 | 否 |
| TD-009 | 已有未跟踪 `101` 迁移编号 | 固定新迁移编号会冲突或覆盖用户工作 | 实施前扫描所有 migration/rollback，动态选下一个可用编号 | 引入迁移号分配/CI 冲突检查 | P0 | 是，直到实施会话重扫 |
| TD-010 | Price Plan V2 仅支持 MEMBER/AGENT | 企业商品无法创建、校验、展示或下单 | 扩展为 `ENTERPRISE_QUOTA`，增加 compute unit 快照契约 | 建立可扩展业务类型注册表 | P0 | 是 |
| TD-011 | 报价/订单主体仍按个人购买假设 | 当前 V2 把 user、buyer 设为同一人，未显式表达受益企业主体 | 订单 tenant 为受益企业、buyer 为购买人、subject 显式化 | 通用 beneficiary/payer 模型 | P0 | 是 |
| TD-012 | `resolveTenant` 只验证成员，未绑定当前上下文和购买权限 | 客户端 tenant header 可能扩大购买范围 | 企业购买使用服务端 current context resolver，header 仅一致性校验 | 全站统一 tenant subject middleware | P0 | 是 |
| TD-013 | `entitlementBalances` 只返回个人点数/图片/会员 | 企业支付结果无法证明企业额度到账 | 企业订单返回 wallet/lot/ledger grant 摘要 | 统一权益余额投影 | P1 | 是 |
| TD-014 | 企业 RAG 的 JSON/memory fallback 仍是个人语义 | 非 PostgreSQL 运行时不能保证企业扣费与隔离 | Enterprise V1 生产门禁要求 PostgreSQL；企业请求不降级到 memory billing | 为 fallback 建立等价租户计费或移除生产 fallback | P0 | 是（生产配置门禁） |
| TD-015 | 独立通用对话/Agent Run 未见统一企业计费入口 | 容易把 RAG 覆盖错误外推到未来入口 | V1 只声明 RAG 对话和 Knowledge Agent；其他入口未接入前禁止发布 | 建立统一 Paid AI Execution Gateway | P1 | 否，前提是不发布该入口 |
| TD-016 | `xz_tenant_point_transactions` 与 Compute Ledger 双重记录人工调整 | 查询合并可能重复计数 | 财务余额只算 Compute Ledger；旧表只显示为 admin-operation 辅助信息 | 用 outbox/read model 生成兼容投影，禁止双主账 | P1 | 否 |
| TD-017 | 企业额度商品购买佣金缺少系统级排除类型 | 只靠空规则可能被未来默认规则命中 | 商品快照 `commissionEligible=false`，Payment/Commission 双重硬拒绝 | Commission 输入事件使用显式白名单 schema | P0 | 是 |
| TD-018 | 购买与消费查询索引不足以支撑成员/模块/时间筛选 | 企业大数据量下页面慢、影响 DB | 新迁移增加 tenant + time + user/org/module 组合索引 | 分区、归档和分析库 | P1 | 是（基础索引） |

## 3. 详细说明与涉及文件

### 3.0 全量涉及文件索引

| ID | 涉及文件 |
| --- | --- |
| TD-001 | `backend-go/internal/app/payment/fulfillment.go`、`service.go`；`backend-go/internal/httpserver/wechat_virtual_payment.go`、`price_plan_quote_v2.go`、`price_plan_order_v2.go`、`wechat_virtual_entitlements.go`；迁移 061/097 |
| TD-002 | `apps/user-uni/src/pages/AiCreationPage.vue`、`features/enterprise/api.ts`、`types.ts`、`stores/enterprise.ts`；后端 Billing Subject API |
| TD-003 | `apps/user-uni/src/components/enterprise/EnterpriseCenterScreen.vue`、`pages/enterprise/EnterpriseUsagePage.vue`、企业 store/API；`enterprise_runtime.go`、`server.go` |
| TD-004 | `backend-go/internal/httpserver/admin_enterprise_operations_postgres.go`、`admin_enterprise_postgres_store.go`；`admin-vue/src/components/enterprise/EnterpriseManagement.vue`；迁移 041/044 |
| TD-005 | `backend-go/internal/httpserver/postgres_store.go`、`enterprise_runtime.go`、`generation_billing_test.go`、`commission_postgres_test.go`、`connector_api_test.go` |
| TD-006 | `database/migrations/044-enterprise-p0-safety.sql`、`backend-go/internal/httpserver/enterprise_runtime.go`、`admin_enterprise_operations_postgres.go` |
| TD-007 | `database/schema.sql`、迁移 040/044、`backend-go/internal/httpserver/enterprise_postgres_store.go` |
| TD-008 | `backend-go/internal/httpserver/enterprise_runtime.go`、`postgres_store.go`；`database/migrations/040-enterprise-center-v1.sql` |
| TD-009 | `database/migrations/`、`database/rollbacks/`，尤其当前未跟踪的两个 `101-inspiration-template-experience-config` 文件 |
| TD-010 | `database/migrations/097-member-agent-price-plan-v2.sql`；`price_plan_quote_v2.go`、`price_plan_order_v2.go`、Price Plan admin Go 文件；`admin-vue/src/types/pricePlanAdmin.ts` 与治理页面 |
| TD-011 | `database/migrations/021-runtime-projections.sql`、047/061/097；`price_plan_order_v2.go`、`wechat_virtual_payment.go`、企业订单查询 API |
| TD-012 | `backend-go/internal/httpserver/wechat_virtual_payment.go`、`enterprise_postgres_store.go`、`user_rbac.go`、`price_plan_quote_v2.go`、`price_plan_order_v2.go` |
| TD-013 | `backend-go/internal/httpserver/wechat_virtual_payment.go`、`wechat_virtual_entitlements.go`；小程序 `UserOrderResultPage.vue` 与 payment types |
| TD-014 | `backend-go/internal/httpserver/knowledge_billing.go`、`knowledge_api.go`、`store.go`、`postgres_store.go`；生产 PostgreSQL 配置 |
| TD-015 | `backend-go/internal/httpserver/server.go`、`knowledge_api.go`、`connector_capabilities.go`、Agent/Generation 新入口（若未来增加） |
| TD-016 | `backend-go/internal/httpserver/admin_enterprise_operations_postgres.go`、`enterprise_runtime.go`；迁移 041/044 |
| TD-017 | `price_plan_quote_v2.go`、`price_plan_order_v2.go`、`wechat_virtual_entitlements.go`、`commission_*.go`、相关 PostgreSQL tests |
| TD-018 | 动态编号 Enterprise Quota V1 migration/rollback；`enterprise_runtime.go`、`admin_enterprise_operations_postgres.go`、企业/管理端分页 API |

### TD-001：两套支付中心未完全统一

- 当前问题：
  - Price Plan V2 + 微信虚拟支付已有企业 V1 可复用能力。
  - `backend-go/internal/app/payment` 的统一 Payment Center 有独立商品、价格、订单履约注册机制。
  - `FulfillmentEnterprisePlan = "enterprise_plan"` 仅预留，无 handler。
- 影响：同阶段同时扩展会形成双入口、双履约和双对账。
- 临时处理：Enterprise V1 仅修改 Price Plan V2 链路；统一 Payment Center 不配置企业商品、不注册 handler。
- 正式处理：设计支付中心合并迁移，包括历史订单兼容、回调路由、退款和履约重放。
- 优先级：P1。
- 涉及文件：
  - `backend-go/internal/app/payment/fulfillment.go`
  - `backend-go/internal/app/payment/service.go`
  - `backend-go/internal/httpserver/wechat_virtual_payment.go`
  - `backend-go/internal/httpserver/price_plan_quote_v2.go`
  - `backend-go/internal/httpserver/price_plan_order_v2.go`
  - `backend-go/internal/httpserver/wechat_virtual_entitlements.go`
  - `database/migrations/061-unified-payment-center-phase1.sql`
  - `database/migrations/097-member-agent-price-plan-v2.sql`
- 是否阻塞 V1：否；若出现第二主链代码或配置，则转为阻塞。

### TD-002：AI 创作页显示个人积分

- 当前问题：`AiCreationPage.vue` 刷新时固定调用 `businessSdk.points.account()`，`quota` 来自个人 `PointAccount`。
- 影响：企业上下文实际扣企业额度，但页面显示个人余额。
- 临时处理：页面读取服务端 Billing Subject；企业上下文显示企业主体，按权限显示余额或“由企业承担”。
- 正式处理：抽取全端统一 `BillingSubjectBadge` 和 `useBillingSubject`。
- 优先级：P0。
- 涉及文件：
  - `apps/user-uni/src/pages/AiCreationPage.vue`
  - `apps/user-uni/src/api/client.ts`
  - `apps/user-uni/src/stores/user.ts`
  - `apps/user-uni/src/features/enterprise/api.ts`
  - `apps/user-uni/src/features/enterprise/types.ts`
- 是否阻塞 V1：是。

### TD-003：企业消费页为空

- 当前问题：`EnterpriseCenterScreen.vue` 的 usage 视图明确显示“暂无企业消费明细”，store 只加载 billing summary。
- 影响：企业管理员/财务无法审计 AI 消费。
- 临时处理：接入 ledger 与 usage 两个分页接口，不在前端合成余额。
- 正式处理：成本分析、导出、告警、成本中心。
- 优先级：P1。
- 涉及文件：
  - `apps/user-uni/src/components/enterprise/EnterpriseCenterScreen.vue`
  - `apps/user-uni/src/pages/enterprise/EnterpriseUsagePage.vue`
  - `apps/user-uni/src/stores/enterprise.ts`
  - `apps/user-uni/src/features/enterprise/api.ts`
  - `apps/user-uni/src/features/enterprise/types.ts`
  - `backend-go/internal/httpserver/enterprise_runtime.go`
  - `backend-go/internal/httpserver/server.go`
- 是否阻塞 V1：是。

### TD-004：管理后台读取旧人工调整表

- 当前问题：`postgresEnterpriseTransactions` 查询 `xz_tenant_point_transactions`，AI 消费实际进入 Compute Ledger/Model Usage。
- 影响：后台显示不完整甚至误导性“消费明细”。
- 临时处理：`compute` 和 `transactions` 使用 Compute Ledger + Model Usage；人工表仅作为标有 `ADMIN_OPERATION` 的辅助记录。
- 正式处理：统一管理端账务 read model。
- 优先级：P0。
- 涉及文件：
  - `backend-go/internal/httpserver/admin_enterprise_operations_postgres.go`
  - `backend-go/internal/httpserver/admin_enterprise_postgres_store.go`
  - `admin-vue/src/components/enterprise/EnterpriseManagement.vue`
  - `database/migrations/041-admin-enterprise-center-phase1.sql`
  - `database/migrations/044-enterprise-p0-safety.sql`
- 是否阻塞 V1：是。

### TD-005：企业消费佣金隔离缺失

- 当前问题：`generationBillingArtifactsForTx` 无企业主体守卫，PPT 也直接调用 `commissionArtifactsForUserTx`。
- 影响：企业员工有推荐关系时可能产生代理佣金。
- 临时处理：在共享结算入口按服务端任务快照 `BillingAccountType == ENTERPRISE` 返回空佣金；PPT 同样硬短路。
- 正式处理：Commission 只订阅带 `commission_eligible=true` 的非企业结算事件。
- 优先级：P0。
- 涉及文件：
  - `backend-go/internal/httpserver/postgres_store.go`
  - `backend-go/internal/httpserver/enterprise_runtime.go`
  - `backend-go/internal/httpserver/generation_billing_test.go`
  - `backend-go/internal/httpserver/commission_postgres_test.go`
  - `backend-go/internal/httpserver/connector_api_test.go`
- 是否阻塞 V1：是。

### TD-006：lot 到期与钱包余额不一致

- 当前问题：消费只选择未到期 lot，但未见到期时同步扣减 wallet 的结算任务。
- 影响：钱包余额可能大于有效 lot 合计，展示有余额但消费失败。
- 临时处理：V1 `lotValidity = PERMANENT`；发布校验拒绝非永久企业额度商品。
- 正式处理：到期扫描用行锁原子更新 lot、wallet 并追加 `EXPIRY` ledger，支持重试和对账。
- 优先级：P1。
- 涉及文件：
  - `database/migrations/044-enterprise-p0-safety.sql`
  - `backend-go/internal/httpserver/enterprise_runtime.go`
  - `backend-go/internal/httpserver/admin_enterprise_operations_postgres.go`
- 是否阻塞 V1：否，前提是永久 lot 门禁生效。

### TD-007：`schema.sql` 与 `xz_*` 双模型

- 当前问题：`database/schema.sql` 仍有 `enterprises`、`enterprise_members`、`enterprise_quota_transactions`，运行时 Go 使用 `xz_*`。
- 影响：后续开发可能改错模型或重复建表。
- 临时处理：文档、迁移、API 全部以运行时 `xz_*` 为准；旧表不进入 V1 数据流。
- 正式处理：从 migrations 生成规范 schema，标记或移除旧模型。
- 优先级：P2。
- 涉及文件：
  - `database/schema.sql`
  - `database/migrations/040-enterprise-center-v1.sql`
  - `database/migrations/044-enterprise-p0-safety.sql`
  - `backend-go/internal/httpserver/enterprise_postgres_store.go`
- 是否阻塞 V1：否。

### TD-008：reserve 实际是立即扣减

- 当前问题：`reserveEnterpriseComputeTx` 直接减少 wallet/lot 并写 DEBIT，失败再冲正。
- 影响：命名和前端“冻结”含义不准确，长任务中余额暂时降低。
- 临时处理：V1 对外使用“已扣减/已冲正”，不展示冻结企业额度。
- 正式处理：Reservation V2，含 frozen lot、capture/release、超时恢复和并发语义。
- 优先级：P2。
- 涉及文件：
  - `backend-go/internal/httpserver/enterprise_runtime.go`
  - `backend-go/internal/httpserver/postgres_store.go`
  - `database/migrations/040-enterprise-center-v1.sql`
- 是否阻塞 V1：否。

### TD-009：未跟踪 `101` 迁移编号冲突

- 当前问题：工作区已有未跟踪：
  - `database/migrations/101-inspiration-template-experience-config.sql`
  - `database/rollbacks/101-inspiration-template-experience-config.down.sql`
- 影响：本方案若预定 `101` 或固定后续编号，可能与用户工作冲突。
- 临时处理：本轮不创建迁移、不占号；实施迁移任务开始时同时扫描 migrations 和 rollbacks 的已跟踪/未跟踪文件，再分配下一个空闲编号。
- 正式处理：CI 检查迁移号唯一性，并提供仓库内分配规则。
- 优先级：P0。
- 涉及文件：
  - `database/migrations/`
  - `database/rollbacks/`
- 是否阻塞 V1：是，直到实施会话完成重新扫描和冲突确认。

## 4. V1 技术债门禁

以下技术债必须在发布前关闭：

- TD-002、TD-003、TD-004、TD-005、TD-009、TD-010、TD-011、TD-012、TD-013、TD-014、TD-017、TD-018。

以下技术债可在冻结约束下延期：

- TD-001：仅允许 Price Plan V2 单主链。
- TD-006：仅允许永久 lot。
- TD-007：仅允许 `xz_*` 运行时模型。
- TD-008：明确立即扣减 + 失败冲正。
- TD-015：不发布独立通用对话/Agent Run。
- TD-016：Compute Ledger 保持唯一财务主账。

任一延期约束被突破，对应技术债立即升级为 V1 阻塞项。
