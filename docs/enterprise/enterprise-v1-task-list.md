# Enterprise V1 实施任务清单

> 状态：FROZEN
>
> 约束：每个任务限定为一个 Codex 会话可完成、可单独审查、可独立验证的交付单元。本文只拆解任务，本轮未执行任何任务。

## 1. 负责人角色

| 负责人代号 | 责任 |
| --- | --- |
| `DB-R` | 数据库负责人：迁移、约束、索引、回滚兼容和数据库对账 |
| `BE-R` | 后端负责人：Go 领域服务、API、支付、履约、计费、佣金隔离 |
| `MP-R` | 小程序负责人：Vue 3 + TypeScript + uni-app、统一 API Client、微信支付体验 |
| `ADM-R` | 管理后台负责人：Vue 3 + Pinia + Axios + Element Plus、平台 RBAC 与运营页面 |
| `QA-R` | 测试负责人：测试矩阵、自动化、故障注入、回归、对账和验收证据 |
| `REL-R` | 发布负责人：迁移 rehearsal、灰度、监控、回滚与生产门禁；生产动作需另行授权 |

同一任务可以有一名主负责人和协作负责人，但只能有一名合并责任人。

## 2. 并发标记

| 标记 | 含义 |
| --- | --- |
| `SERIAL` | 必须等待依赖完成；不得在另一 worktree 同时修改关键文件 |
| `WT-PARALLEL` | 可以在独立 worktree 并行，前提是只修改列出的独占文件且 API/类型契约已冻结 |
| `READONLY-PARALLEL` | 可与开发并行做只读审计、测试设计或验收准备 |
| `PROD-SERIAL` | 生产步骤，取得授权后仍必须按 runbook 串行 |

## 3. 建议执行顺序

| 阶段 | 任务 |
| --- | --- |
| 0. 基线 | `EV1-0001` -> `EV1-0002` -> `EV1-0003` -> `EV1-0003A` -> `EV1-0003B` |
| 1. 数据与商品 | `EV1-0101` -> `EV1-0102` -> `EV1-0110` -> `EV1-0120` -> `EV1-0130` |
| 2. 主体与订单 | `EV1-0201` -> `EV1-0210` -> `EV1-0220` -> `EV1-0230` |
| 3. 履约 | `EV1-0301` -> `EV1-0310` -> `EV1-0320` |
| 4. 佣金隔离 | `EV1-0401` 和 `EV1-0410`，随后 `EV1-0420` |
| 5. 查询 API | `EV1-0501` -> `EV1-0510` 与 `EV1-0520` |
| 6. 小程序/后台 | `EV1-0601` 后 `EV1-0610`、`EV1-0620`、`EV1-0630`、`EV1-0640`；并行执行 `EV1-0701`、`EV1-0710`、`EV1-0720` |
| 7. 系统验收 | `EV1-0801` -> `EV1-0810`；并行 `EV1-0820`、`EV1-0830`；随后 `EV1-0840` |
| 8. 灰度发布 | `EV1-0901` -> `EV1-0910` -> `EV1-0920`，全部需要独立生产授权 |

## 4. Epic 0：安全与冻结

### EV1-0001：实施基线与迁移号安全扫描

- 主负责人：`DB-R`
- 协作：`BE-R`、`QA-R`
- 依赖：无
- 顺序：1
- 并发：`SERIAL`
- 目标：在实际开发 worktree 中记录 Git 基线、既有脏文件和 migration/rollback 编号集合，确定下一个可用迁移编号。
- 交付：基线记录、下一个安全候选号和实施时动态重扫规则；明确当前未跟踪 `101` 不可占用，候选号不视为预留。
- 涉及文件：只读扫描 `database/migrations/`、`database/rollbacks/`；本任务不创建迁移。
- 验证：已跟踪与未跟踪 migration/rollback 均扫描；历史重复编号和缺失配对均明确列出；没有执行 reset/checkout/stash。
- 禁止：与任何创建迁移文件的任务并行。

### EV1-0002：Enterprise Quota 功能开关与错误契约

- 主负责人：`BE-R`
- 协作：`QA-R`、`MP-R`、`ADM-R`
- 依赖：`EV1-0001`
- 顺序：2
- 并发：`SERIAL`
- 目标：冻结默认关闭的商品、履约、查询、主体 UI、佣金隔离、通用对话和独立 Agent Run 开关及稳定错误码。
- 交付：`docs/enterprise/enterprise-v1-feature-flags-and-error-contracts.md` 契约；本任务不实现配置字段。
- 涉及文件：仅上述 Enterprise 文档。
- 验证：默认关闭、fail-closed、前端与后端边界、发布与回滚顺序均明确。
- 禁止：本任务不修改代码，不开放任何企业商品或通用对话/Agent Run。

### EV1-0003：Enterprise V1 文档基线提交准备

- 主负责人：`PM-R`
- 协作：`DB-R`、`BE-R`、`QA-R`、`MP-R`、`ADM-R`
- 依赖：`EV1-0002`
- 顺序：3
- 并发：`SERIAL`
- 目标：将 Enterprise V1 冻结文档形成独立、可复现的 docs-only 基线，为创建干净 worktree 做准备。
- 交付：仅暂存八份 `docs/enterprise` 冻结文档；提交动作需后续显式授权。
- 涉及文件：本文、freeze、domain boundaries、ADR、technical debt、implementation plan、implementation baseline、feature flags/error contracts。
- 验证：暂存区不包含任何非 `docs/enterprise` 文件；迁移号动态分配、V1 范围和 No-Go 结论一致。
- 禁止：不修改业务代码、不创建迁移、不提交、不推送、不清理其他任务工作区。

### EV1-0003A：功能开关与错误契约实现

- 主负责人：`BE-R`
- 协作：`QA-R`、`MP-R`、`ADM-R`
- 依赖：`EV1-0003` docs-only 基线提交完成并创建干净 worktree
- 顺序：3A
- 并发：`SERIAL`
- 目标：按冻结契约实现服务端配置、production validation、稳定 BusinessCode、capability 投影和客户端错误码透传。
- 交付：配置字段、环境变量样例、health、错误映射和默认关闭验证。
- 涉及文件：`backend-go/internal/config/config.go`、`.env.example`、`.env.production.example`、`backend-go/internal/httpserver/pricing_health_postgres.go`、共享 API Client 及相关 tests。
- 验证：未配置时全部 false；非法依赖组合 fail-closed；通用对话和独立 Agent Run 在 V1 全环境不可开启。
- 禁止：不开放企业商品，不创建 migration/rollback。

### EV1-0003B：双支付主链禁用门禁

- 主负责人：`QA-R`
- 协作：`BE-R`
- 依赖：`EV1-0003A`
- 顺序：3B
- 并发：`WT-PARALLEL`
- 目标：自动证明统一 Payment Center 的 `enterprise_plan` 无 handler、无 Enterprise Quota 商品配置。
- 交付：后端/静态契约测试。
- 涉及文件：`backend-go/internal/app/payment/fulfillment.go` 对应测试、`tests/price-plan-production-gate-contract.test.mjs`。
- 验证：若注册 `enterprise_plan` 或出现第二企业购买入口，测试失败。

## 5. Epic 1：企业商品类型

### EV1-0101：Enterprise Quota V1 唯一数据库迁移

- 主负责人：`DB-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0001`、`EV1-0002`、`EV1-0003`、`EV1-0003A`、`EV1-0003B`、已冻结 ADR/领域契约
- 顺序：4
- 并发：`SERIAL`
- 目标：在一份动态编号迁移中完成 V1 全部结构准备。
- 交付：动态编号的前向 migration 和同编号 rollback；不在本文固定编号。
- 数据内容：
  - `xz_plan_versions.business_type` 支持 `ENTERPRISE_QUOTA`。
  - plan version/price plan 的基础与赠送 `compute_units` 整数字段。
  - quote/order/fulfillment 的 `billing_subject_type`；fulfillment 的 `tenant_id`。
  - 企业订单 buyer/tenant/subject 约束。
  - 三个企业权限码及角色映射。
  - ledger/usage/order/fulfillment 查询索引与履约幂等约束。
- 涉及文件：实施时动态生成的 `database/migrations/<allocated>-enterprise-quota-v1.sql` 和 `database/rollbacks/<allocated>-enterprise-quota-v1.down.sql`。
- 验证：迁移可重复防护、旧 MEMBER/AGENT 数据不改写、rollback 不删除业务数据、未占用 101。
- 禁止：其他任务不得同时修改这两个迁移文件。

### EV1-0102：数据库迁移契约测试

- 主负责人：`QA-R`
- 协作：`DB-R`
- 依赖：`EV1-0101`
- 顺序：5
- 并发：`WT-PARALLEL`
- 目标：覆盖业务类型、整数额度、主体、权限、索引和历史兼容约束。
- 交付：migration test。
- 涉及文件：新建或扩展 `backend-go/internal/httpserver/price_plan_phase*_migration_test.go`、企业 migration test。
- 验证：允许合法企业商品；拒绝负数/混合会员字段/主体缺失；旧订单仍可读。

### EV1-0110：Price Plan 后端企业业务类型

- 主负责人：`BE-R`
- 协作：`DB-R`、`QA-R`
- 依赖：`EV1-0101`
- 顺序：6
- 并发：`SERIAL`
- 目标：扩展 Go 管理类型、CRUD、校验和 health，支持企业额度但不复用 Token/Points。
- 交付：管理 API 能创建/读取/校验企业 plan version 和 price plan。
- 涉及文件：`backend-go/internal/httpserver/price_plan_admin_*.go`、`backend-go/internal/httpserver/pricing_health_postgres.go`、相关 tests。
- 验证：round-trip、发布门禁、MEMBER/AGENT 回归。

### EV1-0120：企业权益快照生成与校验

- 主负责人：`BE-R`
- 协作：`QA-R`
- 依赖：`EV1-0110`
- 顺序：7
- 并发：`SERIAL`
- 目标：报价时生成 versioned 企业 `rights_snapshot`，订单/履约后续只读快照。
- 交付：快照 schema、解析器、整数/永久 lot/无佣金校验。
- 涉及文件：`backend-go/internal/httpserver/price_plan_quote_v2.go`、`backend-go/internal/httpserver/price_plan_order_v2.go`、`backend-go/internal/httpserver/price_plan_v2*_test.go`。
- 验证：基础+赠送精确；已创建快照不随 plan 修改；缺字段 fail-closed。

### EV1-0130：管理后台企业商品类型基础支持

- 主负责人：`ADM-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0110` 的 API contract
- 顺序：8
- 并发：`WT-PARALLEL`
- 目标：让 Price Plan Governance 识别、筛选和编辑企业额度商品。
- 交付：TypeScript union、标签、表单、基础/赠送额度和永久有效字段。
- 涉及文件：`admin-vue/src/types/pricePlanAdmin.ts`、`domain/pricePlanAdmin.ts`、`api/pricePlanAdmin.ts`、`stores/pricePlanAdmin.ts`、`components/billing/price-plan-admin/*`。
- 验证：类型检查、管理 API contract、MEMBER/AGENT UI 回归。
- 禁止：不在本任务实现企业订单/消费页面。

## 6. Epic 2：企业报价、订单主体与权限

### EV1-0201：企业购买主体解析器与权限

- 主负责人：`BE-R`
- 协作：`DB-R`、`QA-R`
- 依赖：`EV1-0101`
- 顺序：9
- 并发：`SERIAL`
- 目标：通过认证用户当前 context、active member、role/permission 解析 buyer 与 beneficiary tenant。
- 交付：统一 `EnterprisePurchaseSubject` 服务；Admin/Finance 允许，其他角色默认拒绝。
- 涉及文件：`backend-go/internal/httpserver/user_rbac.go`、`enterprise_postgres_store.go`、`enterprise_runtime.go`、相关 tests。
- 验证：伪造 tenant header/body 无效；多企业用户只命中当前 context；权限撤销即时生效。

### EV1-0210：企业额度商品列表与报价 API

- 主负责人：`BE-R`
- 协作：`MP-R`、`QA-R`
- 依赖：`EV1-0120`、`EV1-0201`
- 顺序：10
- 并发：`SERIAL`
- 目标：只向有购买权限的当前企业返回可买商品并创建 buyer+tenant+subject 绑定报价。
- 交付：`GET /enterprise/quota-products`、`POST /enterprise/quota-price-quotes` 或对现有 V2 服务的薄封装。
- 涉及文件：`backend-go/internal/httpserver/wechat_virtual_payment.go`、`price_plan_quote_v2.go`、`server.go`、tests。
- 验证：跨租户、过期、退役、无权限、复用他人 quote token 失败。

### EV1-0220：企业受益订单创建

- 主负责人：`BE-R`
- 协作：`QA-R`
- 依赖：`EV1-0210`
- 顺序：11
- 并发：`SERIAL`
- 目标：消费企业报价创建 `buyer_user_id = actor`、`tenant_id = beneficiary` 的 V2 微信订单。
- 交付：订单写入、二次权限核验、subject 快照、企业订单查询隔离。
- 涉及文件：`backend-go/internal/httpserver/price_plan_order_v2.go`、`wechat_virtual_payment.go`、tests。
- 验证：同一 quote 只创建一单；权限撤销/企业切换后下单失败；个人 V2 回归。

### EV1-0230：企业订单读取 API

- 主负责人：`BE-R`
- 协作：`MP-R`、`ADM-R`、`QA-R`
- 依赖：`EV1-0220`
- 顺序：12
- 并发：`WT-PARALLEL`
- 目标：企业 Admin/Finance 按当前 tenant 查询企业额度订单，购买人可查看本人订单状态。
- 交付：`GET /enterprise/orders` 和订单详情/状态投影扩展。
- 涉及文件：`enterprise_postgres_store.go`、`wechat_virtual_payment.go`、`server.go`、tests。
- 验证：buyer/tenant 双重授权、分页、跨租户 404/403 语义稳定。

## 7. Epic 3：企业额度履约

### EV1-0301：Billing 企业额度原子发放命令

- 主负责人：`BE-R`
- 协作：`DB-R`、`QA-R`
- 依赖：`EV1-0101`、`EV1-0120`、`EV1-0220`
- 顺序：13
- 并发：`SERIAL`
- 目标：实现锁 wallet、写 base/bonus lots、增加余额、写 CREDIT ledger 的幂等事务命令。
- 交付：企业 grant service；确定性 idempotency keys；余额前后快照。
- 涉及文件：`backend-go/internal/httpserver/enterprise_runtime.go` 及专用 tests。
- 验证：精确发放、bonus=0、不重复、并发、故障回滚、永久 lot。

### EV1-0310：`GrantOrderEntitlements` 企业分支

- 主负责人：`BE-R`
- 协作：`QA-R`
- 依赖：`EV1-0301`
- 顺序：14
- 并发：`SERIAL`
- 目标：在统一权益服务中识别企业订单并调用 Billing grant；同步写 fulfillment 和订单状态。
- 交付：`ENTERPRISE_QUOTA` entitlement handler、错误持久化和重试幂等。
- 涉及文件：`backend-go/internal/httpserver/wechat_virtual_entitlements.go`、`wechat_virtual_payment_postgres_test.go`。
- 验证：wallet/lot/ledger/fulfillment 同事务；快照异常不猜默认值。

### EV1-0320：支付回调、查单、人工重试与状态闭环

- 主负责人：`BE-R`
- 协作：`ADM-R`、`MP-R`、`QA-R`
- 依赖：`EV1-0310`
- 顺序：15
- 并发：`SERIAL`
- 目标：确认三条入口都调用同一权益服务，并让 status API 返回企业到账摘要。
- 交付：callback/sync/retry 适配；企业 fulfillment 状态投影。
- 涉及文件：`backend-go/internal/httpserver/wechat_virtual_payment.go`、`server.go`、payment tests。
- 验证：重复 notify/sync/retry 十次只发一次；已支付失败可恢复；`enterprise_plan` 仍禁用。

## 8. Epic 4：企业消费佣金隔离

### EV1-0401：共享 Generation 企业佣金硬隔离

- 主负责人：`BE-R`
- 协作：`QA-R`
- 依赖：`EV1-0002`；企业任务可信 `BillingAccountType` 为当前代码基线
- 顺序：16
- 并发：`SERIAL`
- 目标：企业生成任务不调用佣金服务，个人生成保持原逻辑。
- 交付：`generationBillingArtifactsForTx` 企业短路及回归测试。
- 涉及文件：`backend-go/internal/httpserver/postgres_store.go`、`generation_billing_test.go`、`commission_postgres_test.go`。
- 验证：企业生图/改图/视频零佣金；PERSONAL 正常。

### EV1-0410：PPT、RAG、Agent、Connector 佣金覆盖审计与隔离

- 主负责人：`BE-R`
- 协作：`QA-R`
- 依赖：`EV1-0401`
- 顺序：17
- 并发：`SERIAL`
- 目标：修复 PPT 直接佣金调用，验证 RAG/Knowledge Agent 当前无佣金，Connector 不绕过共享守卫。
- 交付：PPT guard 和场景化 tests。
- 涉及文件：`postgres_store.go`、`knowledge_billing.go`、`connector_generation.go`、`connector_ppt_generation.go` 及 tests。
- 验证：所有企业 AI 场景 Commission 表和钱包流水零增量。

### EV1-0420：企业额度购买佣金双重隔离

- 主负责人：`BE-R`
- 协作：`QA-R`
- 依赖：`EV1-0310`
- 顺序：18
- 并发：`WT-PARALLEL`
- 目标：企业报价/订单/履约无佣金快照，Commission 对企业商品二次拒绝。
- 交付：Payment 与 Commission 双守卫。
- 涉及文件：`price_plan_quote_v2.go`、`price_plan_order_v2.go`、`wechat_virtual_entitlements.go`、`commission_*.go`、tests。
- 验证：有推荐关系的购买人完成企业购买后，所有佣金表零增量。

## 9. Epic 5：企业消费查询 API

### EV1-0501：Compute Ledger 与 Model Usage 查询仓储

- 主负责人：`BE-R`
- 协作：`DB-R`、`QA-R`
- 依赖：`EV1-0101`、`EV1-0301`
- 顺序：19
- 并发：`SERIAL`
- 目标：实现 tenant-scoped 稳定游标分页、筛选和关联字段，不复制消费数据。
- 交付：ledger/usage repository methods、有效 lot summary、reconciliation warning。
- 涉及文件：`backend-go/internal/httpserver/enterprise_runtime.go`、`enterprise_types.go`、tests。
- 验证：同时间戳分页无重漏；时间/用户/组织/模块/模型/reference 筛选正确。

### EV1-0510：企业端消费与计费主体 API

- 主负责人：`BE-R`
- 协作：`MP-R`、`QA-R`
- 依赖：`EV1-0201`、`EV1-0501`
- 顺序：20
- 并发：`WT-PARALLEL`
- 目标：提供 compute account、ledger、model usage、billing subject，并按角色脱敏。
- 交付：企业端 API 和 OpenAPI/类型契约。
- 涉及文件：`enterprise_postgres_store.go`、`enterprise_runtime.go`、`server.go`、`enterprise_api_test.go`。
- 验证：跨租户、权限、余额隐藏、游标和 filter。

### EV1-0520：管理端真实企业消费 API

- 主负责人：`BE-R`
- 协作：`ADM-R`、`QA-R`
- 依赖：`EV1-0501`
- 顺序：21
- 并发：`WT-PARALLEL`
- 目标：把 admin compute/transactions/orders 改到 ledger/usage/orders/fulfillment。
- 交付：管理投影与 totals；人工表只作为标记明确的辅助信息。
- 涉及文件：`admin_enterprise_operations_postgres.go`、`admin_enterprise_postgres_store.go`、`admin_enterprise_api_test.go`。
- 验证：Admin API 与企业 API 相同 filter totals 一致；不重复 join。

## 10. Epic 6：小程序企业计费体验

### EV1-0601：小程序 Enterprise Billing 数据层

- 主负责人：`MP-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0230`、`EV1-0320`、`EV1-0510` API contract
- 顺序：22
- 并发：`SERIAL`
- 目标：统一企业商品、报价、订单、subject、ledger、usage 的 types/API/store。
- 交付：缓存与 context 切换清理策略；无页面直写 request。
- 涉及文件：`apps/user-uni/src/features/enterprise/api.ts`、`types.ts`、`stores/enterprise.ts`、`features/payment/*`。
- 验证：类型检查；切换企业后旧 quote/order/subject/usage 缓存清空。

### EV1-0610：企业额度购买与订单页面

- 主负责人：`MP-R`
- 协作：`QA-R`
- 依赖：`EV1-0601`
- 顺序：23
- 并发：`WT-PARALLEL`
- 目标：在现有 Enterprise Center 算力页显示商品、购买、订单、履约状态。
- 交付：Admin/Finance 购买；Member 只读主体；支付状态处理。
- 涉及文件：`EnterpriseCenterScreen.vue`、`EnterpriseBillingPage.vue`、`enterprise-center.css`。
- 验证：权限矩阵、支付取消/履约中/失败/同步成功；金额和权益只读。

### EV1-0620：AI 创作页真实计费主体

- 主负责人：`MP-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0510`、`EV1-0601`
- 顺序：24
- 并发：`WT-PARALLEL`
- 目标：移除企业上下文固定个人 `points.account` 展示，改用服务端 subject。
- 交付：个人/企业主体 badge、权限化余额、企业不足提示。
- 涉及文件：`apps/user-uni/src/pages/AiCreationPage.vue`。
- 验证：PERSONAL/ENTERPRISE 切换；企业不足个人充足仍不 fallback；提交任务主体与页面一致。

### EV1-0630：企业消费明细页面

- 主负责人：`MP-R`
- 协作：`QA-R`
- 依赖：`EV1-0601`
- 顺序：25
- 并发：`WT-PARALLEL`
- 目标：替换空态，展示 Ledger 与 Model Usage 两个视图。
- 交付：时间/成员/模块筛选、游标加载、详情关联。
- 涉及文件：`EnterpriseCenterScreen.vue`、`EnterpriseUsagePage.vue`、`enterprise-center.css`。
- 验证：CREDIT/DEBIT/REVERSAL、任务/模型/成员、空态/错误/重试。

### EV1-0640：企业支付结果页

- 主负责人：`MP-R`
- 协作：`QA-R`
- 依赖：`EV1-0320`、`EV1-0601`
- 顺序：26
- 并发：`WT-PARALLEL`
- 目标：企业订单显示受益企业、基础/赠送额度和履约，不显示个人到账点数。
- 交付：subject-aware order result。
- 涉及文件：`apps/user-uni/src/pages/user/UserOrderResultPage.vue`、`features/payment/types.ts`。
- 验证：个人订单旧展示不变；企业成功/失败/处理中正确。

## 11. Epic 7：管理后台企业消费与履约

### EV1-0701：Price Plan Governance 企业商品完整 UI

- 主负责人：`ADM-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0130`、`EV1-0120`
- 顺序：27
- 并发：`WT-PARALLEL`
- 目标：完成企业额度商品版本、价格、微信商品绑定、发布门禁和无佣金展示。
- 交付：可治理但默认不发布的企业商品 UI。
- 涉及文件：`admin-vue/src/components/billing/price-plan-admin/*`、price plan store/domain/types。
- 验证：构建、权限、金额/权益/绑定/环境校验、会员/代理回归。

### EV1-0710：Enterprise Management 真实消费与订单

- 主负责人：`ADM-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0520`
- 顺序：28
- 并发：`SERIAL`
- 目标：企业详情显示真实 wallet/lot/ledger/usage/order，不再把人工调整表当消费。
- 交付：compute、transactions、orders 三个模块的分页/筛选/详情。
- 涉及文件：`admin-vue/src/components/enterprise/EnterpriseManagement.vue`、企业 API 模块。
- 验证：与 API totals/对账一致；跨企业筛选；只读权限。

### EV1-0720：管理后台履约状态与统一重试

- 主负责人：`ADM-R`
- 协作：`BE-R`、`QA-R`
- 依赖：`EV1-0320`、`EV1-0520`
- 顺序：29
- 并发：`WT-PARALLEL`
- 目标：查看 paid/fulfilling/failed/success，调用既有统一 retry。
- 交付：履约列表、错误详情、重试确认和审计结果。
- 涉及文件：Payment/Billing 管理 API 客户端、相关 Element Plus 页面；避免与 `EnterpriseManagement.vue` 并发。
- 验证：无权限 403；重复 retry 幂等；不能输入额度数量；成功可追到 ledger。

## 12. Epic 8：测试、对账、灰度与发布

### EV1-0801：企业购买履约后端端到端测试

- 主负责人：`QA-R`
- 协作：`BE-R`、`DB-R`
- 依赖：`EV1-0320`、`EV1-0420`
- 顺序：30
- 并发：`WT-PARALLEL`
- 目标：覆盖商品 -> 报价 -> 订单 -> 支付 -> entitlement -> wallet/lot/ledger/fulfillment。
- 交付：PostgreSQL 集成、并发、故障注入和跨租户测试。
- 涉及文件：`price_plan_v2_postgres_test.go`、`wechat_virtual_payment_postgres_test.go`、新增 enterprise quota test。
- 验证：重复十次一次发放、事务原子、无佣金、个人回归。

### EV1-0810：企业 AI 全场景计费矩阵测试

- 主负责人：`QA-R`
- 协作：`BE-R`
- 依赖：`EV1-0410`、`EV1-0510`
- 顺序：31
- 并发：`WT-PARALLEL`
- 目标：覆盖生图、改图、视频、PPT、RAG、Knowledge Agent、Connector 的 subject/debit/reversal/usage/no-commission。
- 交付：场景矩阵测试和“独立通用对话/Agent 未开放”断言。
- 涉及文件：`enterprise_p0_integration_test.go`、`generation_billing_test.go`、`knowledge_api_test.go`、`connector_api_test.go`、PPT tests。
- 验证：企业不足不调用 provider，个人余额不变；失败冲正一次；佣金零增量。

### EV1-0820：小程序契约、构建与角色验收

- 主负责人：`QA-R`
- 协作：`MP-R`
- 依赖：`EV1-0610`、`EV1-0620`、`EV1-0630`、`EV1-0640`
- 顺序：32
- 并发：`WT-PARALLEL`
- 目标：验证 API Client、context cache、角色页面和微信支付状态。
- 交付：静态 contract、typecheck、H5/mp-weixin build 和运行态验收记录。
- 涉及文件：根目录 `tests/enterprise-*.test.mjs`、小程序测试/产物。
- 验证：无散写 `uni.request`、无 Axios、主体真实、普通成员无越权。

### EV1-0830：管理后台契约、构建与权限验收

- 主负责人：`QA-R`
- 协作：`ADM-R`
- 依赖：`EV1-0701`、`EV1-0710`、`EV1-0720`
- 顺序：33
- 并发：`WT-PARALLEL`
- 目标：验证 Price Plan 企业类型、真实消费、履约重试和平台 RBAC。
- 交付：contract tests、typecheck/build、页面验收记录。
- 涉及文件：现有 price plan tests、新增 enterprise admin tests。
- 验证：只读/管理/财务权限分离；真实 facts；会员/代理治理回归。

### EV1-0840：对账 SQL 与发布 Runbook

- 主负责人：`REL-R`
- 协作：`DB-R`、`BE-R`、`QA-R`
- 依赖：`EV1-0801`、`EV1-0810`、`EV1-0820`、`EV1-0830`
- 顺序：34
- 并发：`SERIAL`
- 目标：形成迁移 rehearsal、对账、灰度、监控、停止和回滚步骤。
- 交付：实施阶段新增 release runbook 和只读 reconciliation SQL。
- 验证：临时 PostgreSQL rehearsal；wallet=ledger=valid lots；paid orders/fulfillment；企业佣金为零。
- 禁止：本任务不执行生产迁移或发布。

## 13. 生产任务（需独立授权）

### EV1-0901：生产前预检与备份

- 主负责人：`REL-R`
- 协作：`DB-R`、`BE-R`、`QA-R`
- 依赖：`EV1-0840`、生产授权
- 顺序：35
- 并发：`PROD-SERIAL`
- 目标：确认镜像、迁移号、微信商品、开关、备份、回滚和告警。
- 验证：所有上线门禁逐项有证据；阻塞即停止。

### EV1-0910：白名单企业灰度

- 主负责人：`REL-R`
- 协作：全角色
- 依赖：`EV1-0901`
- 顺序：36
- 并发：`PROD-SERIAL`
- 目标：对极少数测试企业开启商品/购买/AI/查询，完成真实支付和 AI 场景对账。
- 验证：无重复发放、跨租户、个人 fallback、企业佣金；订单到 ledger 全链可追踪。

### EV1-0920：全量发布或停止

- 主负责人：`REL-R`
- 协作：全角色
- 依赖：`EV1-0910` 灰度验收通过、全量授权
- 顺序：37
- 并发：`PROD-SERIAL`
- 目标：按门禁扩大范围；任一财务/隔离异常立即关闭新增购买和企业 AI 灰度。
- 验证：发布后持续对账；保留已付失败订单的履约补偿能力。

## 14. 必须串行的任务链

1. 迁移链：`EV1-0001 -> EV1-0002 -> EV1-0003 -> EV1-0003A -> EV1-0003B -> EV1-0101 -> EV1-0102`。
2. 商品/报价/订单：`EV1-0110 -> EV1-0120 -> EV1-0201 -> EV1-0210 -> EV1-0220`。
3. 履约：`EV1-0301 -> EV1-0310 -> EV1-0320`。
4. Generation 佣金：`EV1-0401 -> EV1-0410`。
5. 查询契约：`EV1-0501 -> EV1-0510/EV1-0520`。
6. 小程序共享数据层：`EV1-0601` 完成后才能并行页面任务。
7. 管理后台 `EnterpriseManagement.vue`：`EV1-0710` 独占。
8. 发布：`EV1-0840 -> EV1-0901 -> EV1-0910 -> EV1-0920`。

## 15. 可独立 worktree 并行的任务

- `EV1-0003B`：双主链测试；前提是 `EV1-0003A` 已实现默认关闭门禁。
- `EV1-0102`：迁移测试，前提是迁移文件只读。
- `EV1-0130`：管理后台企业类型基础 UI。
- `EV1-0230`：订单读取投影，前提是不与下单服务共享文件。
- `EV1-0420`：购买佣金隔离，需避开正在修改的 V2 文件。
- `EV1-0510` 与 `EV1-0520`：企业端/管理端查询 API，共享 repository contract 先冻结。
- `EV1-0610`、`EV1-0620`、`EV1-0630`、`EV1-0640`：页面任务；`EnterpriseCenterScreen.vue` 涉及的 0610/0630 不能彼此并发。
- `EV1-0701` 与 `EV1-0710`：Price Plan 和企业管理 UI 可并行。
- `EV1-0801`、`EV1-0810`、`EV1-0820`、`EV1-0830`：不同测试面可并行。

## 16. 禁止并发修改矩阵

| 文件/区域 | 独占任务 | 冲突任务 |
| --- | --- | --- |
| 动态编号 Enterprise Quota migration/rollback | `EV1-0101` | 所有 DB 任务 |
| `price_plan_quote_v2.go` | `EV1-0120` 或 `EV1-0210` 或 `EV1-0420`，一次一个 | 企业快照、报价、购买佣金任务 |
| `price_plan_order_v2.go` | `EV1-0120` 或 `EV1-0220` 或 `EV1-0420` | 快照、下单、购买佣金任务 |
| `wechat_virtual_payment.go` | `EV1-0210`、`EV1-0220`、`EV1-0320`，一次一个 | 商品/报价、下单、支付状态任务 |
| `wechat_virtual_entitlements.go` | `EV1-0310` | 履约和购买佣金任务 |
| `enterprise_runtime.go` | `EV1-0301` 或 `EV1-0501`，一次一个 | grant/debit 与查询任务 |
| `postgres_store.go` | `EV1-0401` 或 `EV1-0410`，一次一个 | Generation/PPT/Connector 佣金任务 |
| `server.go` | 指定唯一集成人 | 新路由任务 |
| `features/enterprise/api.ts`、`types.ts`、`stores/enterprise.ts` | `EV1-0601` | 所有小程序页面任务 |
| `EnterpriseCenterScreen.vue` | `EV1-0610` 或 `EV1-0630`，一次一个 | 购买页和消费页 |
| `AiCreationPage.vue` | `EV1-0620` | 其他创作页改造 |
| `types/domain/store pricePlanAdmin` | `EV1-0130` 或 `EV1-0701`，一次一个 | 管理端商品任务 |
| `EnterpriseManagement.vue` | `EV1-0710` | 所有企业管理页面改造 |

## 17. 第一个建议进入开发的任务

第一个开发任务不是写迁移，而是 `EV1-0001：实施基线与迁移号安全扫描`。完成后，第一个实际代码/数据库任务是 `EV1-0101：Enterprise Quota V1 唯一数据库迁移`。

原因：当前工作区已经存在未跟踪的 `101` migration/rollback；先锁定基线和动态编号，才能避免占号、覆盖用户改动和让后续后端/前端围绕漂移的数据库契约开发。
