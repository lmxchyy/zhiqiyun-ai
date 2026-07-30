# Enterprise V1 实施基线与迁移号安全扫描

> 任务：EV1-0001  
> 扫描日期：2026-07-30（Asia/Shanghai）  
> 扫描性质：只读审计；除本文件外未修改代码、文档、migration 或 rollback，未运行迁移、格式化、测试、提交或推送。  
> 数据库依据：仅以 `database/migrations` 为运行时结构依据，不使用 `database/schema.sql` 作为主依据。

## 1. 结论摘要

| 项目 | 结论 |
| --- | --- |
| 仓库 | `E:\code\work\先知AI` |
| 分支 | `codex/channel-ecosystem-v132-phase3` |
| 上游 | `origin/codex/channel-ecosystem-v132-phase3`，扫描时显示 up-to-date |
| 基线 HEAD | `054a8fcaf4754ca9b3fd5492265685998924835c` |
| 工作区 | **不适合直接进入 EV1-0101**：21 个已跟踪文件有修改，24 个精确未跟踪文件；其中 `api.go`、`connector_generation.go`、`postgres_store.go` 与 Enterprise V1 高冲突区重叠 |
| 当前占用的最高迁移号 | `101`，migration/rollback 均未跟踪，属于其他“灵感模板体验配置”并发任务 |
| 下一个安全候选号 | `102`；本任务未创建、未预留、未占用，EV1-0101 开始前必须重新扫描 |
| EV1-0101 | **No-Go（当前工作区）**；需先建立可追溯、无冲突的独立 worktree，并完成 EV1-0002 前置条件 |

## 2. 仓库和分支基线

### 2.1 仓库定位

```text
absolute repository path: E:\code\work\先知AI
branch:                   codex/channel-ecosystem-v132-phase3
HEAD:                     054a8fcaf4754ca9b3fd5492265685998924835c
upstream:                 origin/codex/channel-ecosystem-v132-phase3
```

### 2.2 `git status --short`（新增本文件前）

```text
 M admin-vue/src/api/inspirations.ts
 M admin-vue/src/components/inspiration/InspirationManagement.vue
 M apps/user-uni/scripts/patch-mp-native-login.cjs
 M apps/user-uni/src/components/MiniProgramRoleWorkbench.vue
 M apps/user-uni/src/components/inspiration/InspirationCard.vue
 M apps/user-uni/src/components/v531/V531HomePage.vue
 M apps/user-uni/src/features/inspiration/draft.ts
 M apps/user-uni/src/features/inspiration/types.ts
 M apps/user-uni/src/pages/inspiration/InspirationDetailPage.vue
 M apps/user-uni/src/pages/user/UserMinePage.vue
 M backend-go/internal/httpserver/api.go
 M backend-go/internal/httpserver/connector_generation.go
 M backend-go/internal/httpserver/generation_storage.go
 M backend-go/internal/httpserver/generation_storage_test.go
 M backend-go/internal/httpserver/inspiration_api.go
 M backend-go/internal/httpserver/inspiration_api_test.go
 M backend-go/internal/httpserver/inspiration_repository.go
 M backend-go/internal/httpserver/postgres_store.go
 M backend-go/internal/httpserver/store.go
 M backend-go/internal/provider/video/openai_compatible.go
 M backend-go/internal/provider/video/openai_compatible_test.go
?? apps/user-uni/scripts/mp-native-patch-patterns.cjs
?? apps/user-uni/src/features/assets/homeRecentWorks.ts
?? apps/user-uni/src/features/auth/agentCommerceEntry.ts
?? apps/user-uni/src/features/generation/
?? backend-go/internal/httpserver/online_generation_settings_postgres_test.go
?? backend-go/internal/httpserver/recover_task_000082_test.go
?? backend-go/internal/httpserver/video_thumbnail.go
?? backend-go/internal/httpserver/video_thumbnail_test.go
?? database/migrations/101-inspiration-template-experience-config.sql
?? database/rollbacks/101-inspiration-template-experience-config.down.sql
?? design-qa.md
?? docs/enterprise/
?? docs/video-prompt-optimizer-investigation.md
?? tests/inspiration-photo-restoration.test.mjs
?? tests/mp-native-patch-component-pattern.test.mjs
?? tests/mp-package-fallback-retention.test.mjs
?? tests/user-home-recent-work-thumbnails.test.mjs
?? tests/user-mini-agent-entry.test.mjs
?? tests/user-mini-video-parameters.test.mjs
```

### 2.3 `git status --branch`（新增本文件前）

```text
On branch codex/channel-ecosystem-v132-phase3
Your branch is up to date with 'origin/codex/channel-ecosystem-v132-phase3'.

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
        modified:   admin-vue/src/api/inspirations.ts
        modified:   admin-vue/src/components/inspiration/InspirationManagement.vue
        modified:   apps/user-uni/scripts/patch-mp-native-login.cjs
        modified:   apps/user-uni/src/components/MiniProgramRoleWorkbench.vue
        modified:   apps/user-uni/src/components/inspiration/InspirationCard.vue
        modified:   apps/user-uni/src/components/v531/V531HomePage.vue
        modified:   apps/user-uni/src/features/inspiration/draft.ts
        modified:   apps/user-uni/src/features/inspiration/types.ts
        modified:   apps/user-uni/src/pages/inspiration/InspirationDetailPage.vue
        modified:   apps/user-uni/src/pages/user/UserMinePage.vue
        modified:   backend-go/internal/httpserver/api.go
        modified:   backend-go/internal/httpserver/connector_generation.go
        modified:   backend-go/internal/httpserver/generation_storage.go
        modified:   backend-go/internal/httpserver/generation_storage_test.go
        modified:   backend-go/internal/httpserver/inspiration_api.go
        modified:   backend-go/internal/httpserver/inspiration_api_test.go
        modified:   backend-go/internal/httpserver/inspiration_repository.go
        modified:   backend-go/internal/httpserver/postgres_store.go
        modified:   backend-go/internal/httpserver/store.go
        modified:   backend-go/internal/provider/video/openai_compatible.go
        modified:   backend-go/internal/provider/video/openai_compatible_test.go

Untracked files:
  (use "git add <file>..." to include in what will be committed)
        apps/user-uni/scripts/mp-native-patch-patterns.cjs
        apps/user-uni/src/features/assets/homeRecentWorks.ts
        apps/user-uni/src/features/auth/agentCommerceEntry.ts
        apps/user-uni/src/features/generation/
        backend-go/internal/httpserver/online_generation_settings_postgres_test.go
        backend-go/internal/httpserver/recover_task_000082_test.go
        backend-go/internal/httpserver/video_thumbnail.go
        backend-go/internal/httpserver/video_thumbnail_test.go
        database/migrations/101-inspiration-template-experience-config.sql
        database/rollbacks/101-inspiration-template-experience-config.down.sql
        design-qa.md
        docs/enterprise/
        docs/video-prompt-optimizer-investigation.md
        tests/inspiration-photo-restoration.test.mjs
        tests/mp-native-patch-component-pattern.test.mjs
        tests/mp-package-fallback-retention.test.mjs
        tests/user-home-recent-work-thumbnails.test.mjs
        tests/user-mini-agent-entry.test.mjs
        tests/user-mini-video-parameters.test.mjs

no changes added to commit (use "git add" and/or "git commit -a")
```

### 2.4 `git diff --name-only`

```text
admin-vue/src/api/inspirations.ts
admin-vue/src/components/inspiration/InspirationManagement.vue
apps/user-uni/scripts/patch-mp-native-login.cjs
apps/user-uni/src/components/MiniProgramRoleWorkbench.vue
apps/user-uni/src/components/inspiration/InspirationCard.vue
apps/user-uni/src/components/v531/V531HomePage.vue
apps/user-uni/src/features/inspiration/draft.ts
apps/user-uni/src/features/inspiration/types.ts
apps/user-uni/src/pages/inspiration/InspirationDetailPage.vue
apps/user-uni/src/pages/user/UserMinePage.vue
backend-go/internal/httpserver/api.go
backend-go/internal/httpserver/connector_generation.go
backend-go/internal/httpserver/generation_storage.go
backend-go/internal/httpserver/generation_storage_test.go
backend-go/internal/httpserver/inspiration_api.go
backend-go/internal/httpserver/inspiration_api_test.go
backend-go/internal/httpserver/inspiration_repository.go
backend-go/internal/httpserver/postgres_store.go
backend-go/internal/httpserver/store.go
backend-go/internal/provider/video/openai_compatible.go
backend-go/internal/provider/video/openai_compatible_test.go
```

### 2.5 `git diff --cached --name-only`

```text
(empty)
```

### 2.6 精确未跟踪文件列表（`git ls-files --others --exclude-standard`）

```text
apps/user-uni/scripts/mp-native-patch-patterns.cjs
apps/user-uni/src/features/assets/homeRecentWorks.ts
apps/user-uni/src/features/auth/agentCommerceEntry.ts
apps/user-uni/src/features/generation/videoParameters.ts
backend-go/internal/httpserver/online_generation_settings_postgres_test.go
backend-go/internal/httpserver/recover_task_000082_test.go
backend-go/internal/httpserver/video_thumbnail.go
backend-go/internal/httpserver/video_thumbnail_test.go
database/migrations/101-inspiration-template-experience-config.sql
database/rollbacks/101-inspiration-template-experience-config.down.sql
design-qa.md
docs/enterprise/enterprise-domain-boundaries.md
docs/enterprise/enterprise-v1-adr.md
docs/enterprise/enterprise-v1-freeze.md
docs/enterprise/enterprise-v1-implementation-plan.md
docs/enterprise/enterprise-v1-task-list.md
docs/enterprise/enterprise-v1-technical-debt.md
docs/video-prompt-optimizer-investigation.md
tests/inspiration-photo-restoration.test.mjs
tests/mp-native-patch-component-pattern.test.mjs
tests/mp-package-fallback-retention.test.mjs
tests/user-home-recent-work-thumbnails.test.mjs
tests/user-mini-agent-entry.test.mjs
tests/user-mini-video-parameters.test.mjs
```

新增本文件后，精确未跟踪列表会再增加：

```text
docs/enterprise/enterprise-v1-implementation-baseline.md
```

### 2.7 工作区适用性判断

当前工作区不适合直接开发 Enterprise V1：

1. 已有大量未提交修改，且归属至少覆盖灵感中心、视频生成/缩略图、小程序角色入口等其他任务。
2. `backend-go/internal/httpserver/api.go`、`connector_generation.go`、`postgres_store.go` 是 Enterprise V1 后续也会触碰的共享高冲突文件。
3. `101` migration/rollback 已由另一个并发任务以未跟踪文件形式占用。
4. Enterprise V1 冻结文档本身仍未跟踪；从当前 HEAD 新建 worktree 不会自动包含这些文件，无法仅凭 HEAD 复现设计基线。

建议后续在 EV1-0101 前创建独立 worktree，但本任务不创建。worktree 应从团队确认的、包含冻结文档的可追溯基线建立；若仍从当前 HEAD 建立，必须通过明确的受控交接把冻结文档带入，不能假定未跟踪文件会存在。

## 3. 迁移编号安全扫描

### 3.1 Migration 编号全集

扫描目录：`database/migrations`。共 103 个 migration 文件，100 个唯一编号；编号 `002` 至 `101` 连续占用，但 `050`、`056`、`078` 各有两个文件。

```text
002 003 004 005 006 007 008 009
010 011 012 013 014 015 016 017 018 019
020 021 022 023 024 025 026 027 028 029
030 031 032 033 034 035 036 037 038 039
040 041 042 043 044 045 046 047 048 049
050 050 051 052 053 054 055 056 056 057 058 059
060 061 062 063 064 065 066 067 068 069
070 071 072 073 074 075 076 077 078 078 079
080 081 082 083 084 085 086 087 088 089
090 091 092 093 094 095 096 097 098 099
100 101
```

重复编号明细：

| 编号 | Migration 文件 |
| --- | --- |
| 050 | `050-commercial-billing-wechat.sql`; `050-wechat-virtual-custom-token-unit.sql` |
| 056 | `056-connector-qr-authorization.sql`; `056-wechat-virtual-test-token-1fen.sql` |
| 078 | `078-promotion-invite-token.sql`; `078-publish-full-legal-agreements.sql` |

历史重复编号已存在，因此执行顺序不能只按整数去重；迁移工具必须按仓库既有完整文件名规则处理。Enterprise V1 不应再制造重复编号。

### 3.2 Rollback 编号全集

扫描目录：`database/rollbacks`。共 12 个文件，无重复编号，无“rollback 有但 migration 缺失”的编号。

| 编号 | Rollback 文件 | Git 状态 |
| --- | --- | --- |
| 066 | `066-user-business-identity-foundation.down.sql` | tracked |
| 067 | `067-admin-identity-change-workflow.down.sql` | tracked |
| 068 | `068-controlled-identity-downgrade.down.sql` | tracked |
| 070 | `070-identity-rbac-security-hardening.down.sql` | tracked |
| 071 | `071-identity-command-consistency.down.sql` | tracked |
| 074 | `074-ai-smart-video-foundation.down.sql` | tracked |
| 075 | `075-smart-video-media-analysis.down.sql` | tracked |
| 076 | `076-smart-video-render-worker.down.sql` | tracked |
| 077 | `077-agent-invite-apk-distribution.down.sql` | tracked |
| 078 | `078-promotion-invite-token.down.sql` | tracked |
| 095 | `095-payment-refund-adapter-query-verification.down.sql` | tracked |
| 101 | `101-inspiration-template-experience-config.down.sql` | **untracked** |

### 3.3 Migration/rollback 配对检查

- migration 有、rollback 缺失的编号：

```text
002, 003, 004, 005, 006, 007, 008, 009,
010, 011, 012, 013, 014, 015, 016, 017, 018, 019,
020, 021, 022, 023, 024, 025, 026, 027, 028, 029,
030, 031, 032, 033, 034, 035, 036, 037, 038, 039,
040, 041, 042, 043, 044, 045, 046, 047, 048, 049,
050, 051, 052, 053, 054, 055, 056, 057, 058, 059,
060, 061, 062, 063, 064, 065, 069,
072, 073, 079,
080, 081, 082, 083, 084, 085, 086, 087, 088, 089,
090, 091, 092, 093, 094, 096, 097, 098, 099, 100
```

- rollback 有、migration 缺失：无。
- 重复 rollback 编号：无。
- 未跟踪 migration：`101-inspiration-template-experience-config.sql`。
- 未跟踪 rollback：`101-inspiration-template-experience-config.down.sql`。
- 已占用但未提交编号：`101`。

现有大量 migration 没有 rollback 是仓库历史现状，不代表 EV1-0101 可以省略 rollback。EV1-0101 的任务契约明确要求同编号成对文件。

### 3.4 101 专项核查

| 项目 | 结论 |
| --- | --- |
| Migration | `database/migrations/101-inspiration-template-experience-config.sql` |
| Rollback | `database/rollbacks/101-inspiration-template-experience-config.down.sql` |
| Git 状态 | 两者均为 `??`，未跟踪、未提交 |
| 是否成对 | 是；文件号和主题一致，rollback 撤销该 migration 的索引和新增列 |
| 内容用途 | 为 `inspiration_templates` 增加 `scenario_code`、`display_config_json`、`input_requirements_json`、`preset_config_json`；增加场景索引；upsert 图像增强类目 |
| Rollback 行为 | 保留已 upsert 类目数据，仅删除场景索引和四个新增列 |
| Enterprise V1 归属 | 否 |
| 并发任务归属判断 | **属于其他灵感模板/体验配置并发任务**。依据是文件名、DDL 对象、同工作区大量 inspiration 相关变更；这是内容归属推断，不是 Git 作者身份确认 |

结论：Enterprise V1 不得使用、改名、覆盖或删除 `101`。

### 3.5 下一个安全可用编号

- 数字 `001` 虽未出现，但属于历史低位空洞，禁止回填。
- 最高已占用编号为未跟踪的 `101`。
- 本次扫描得到的下一个安全候选号为 **`102`**。
- `102` 仅是扫描结果，不是预留或分配；EV1-0101 会话必须再次扫描 tracked + untracked migration/rollback 后，才可最终分配编号。

## 4. Enterprise V1 文件冲突扫描

### 4.1 高冲突文件矩阵

| 文件/区域 | 后续职责 | 并发规则 | 当前状态 |
| --- | --- | --- | --- |
| `database/migrations/<dynamic>-enterprise-quota-v1.sql` / rollback | EV1-0101 唯一结构变更 | 所有 DB 任务串行；动态编号唯一所有者 | 尚不存在；101 已被其他任务占用 |
| `backend-go/internal/httpserver/price_plan_quote_v2.go` | 企业商品解析、报价、快照 | EV1-0120/0210/0420 一次一个 | clean |
| `backend-go/internal/httpserver/price_plan_order_v2.go` | 企业订单、buyer/beneficiary/subject | EV1-0120/0220/0420 一次一个 | clean |
| `backend-go/internal/httpserver/wechat_virtual_payment.go` | 微信虚拟支付商品、下单、状态 | EV1-0210/0220/0320 一次一个 | clean |
| `backend-go/internal/httpserver/wechat_virtual_entitlements.go` | `GrantOrderEntitlements` 企业分支 | 履约任务唯一所有者 | clean |
| `backend-go/internal/httpserver/enterprise_runtime.go` | 企业扣费、lot/ledger/usage、消费查询 | 履约与查询契约串行集成 | clean |
| `backend-go/internal/httpserver/postgres_store.go` | Generation/PPT/Connector 结算与佣金隔离 | EV1-0401/0410 一次一个 | **modified，已冲突** |
| `backend-go/internal/httpserver/api.go` | Generation 共享编排/API 注册 | 共享集成点单人修改 | **modified，已冲突** |
| `backend-go/internal/httpserver/connector_generation.go` | Connector 生成与企业计费 | 与 EV1-0410 串行 | **modified，已冲突** |
| `backend-go/internal/httpserver/connector_ppt_generation.go` | Connector PPT 企业计费 | 与 Connector/佣金任务协调 | clean |
| `backend-go/internal/httpserver/knowledge_billing.go` | RAG/Knowledge 企业计费 | 与计费契约稳定后独立 | clean |
| `backend-go/internal/httpserver/server.go` | 路由和依赖注册 | 跨 Epic 集成点单人修改 | clean |
| `backend-go/internal/httpserver/enterprise_postgres_store.go` | 企业订单/消费查询 | 企业 API 单一所有者 | clean |
| `backend-go/internal/httpserver/admin_enterprise_operations_postgres.go` | 管理后台真实消费 | 管理查询单一所有者 | clean |
| `apps/user-uni/src/pages/AiCreationPage.vue` | 展示真实计费主体 | EV1-0620 唯一所有者 | clean |
| `apps/user-uni/src/components/enterprise/EnterpriseCenterScreen.vue` | 购买、余额、消费入口 | EV1-0610 与 EV1-0630 串行 | clean |
| `apps/user-uni/src/features/enterprise/api.ts`、`types.ts`、`stores/enterprise.ts` | 小程序 API 契约和共享状态 | 一个集成人维护，页面可并行 | clean |
| `admin-vue/src/components/enterprise/EnterpriseManagement.vue` | 企业消费、订单、履约 | 企业管理页面唯一所有者 | clean |
| `admin-vue/src/types/pricePlanAdmin.ts`、`api/pricePlanAdmin.ts`、`stores/pricePlanAdmin.ts`、`domain/pricePlanAdmin.ts` | 企业额度商品治理 | Epic 1/7 串行集成 | clean |
| `admin-vue/src/components/billing/price-plan-admin/*` | Billing 商品/价格/微信绑定 | 组件可拆分，公共类型/Store 串行 | clean |
| `admin-vue/src/components/billing/BillingCenterV1.vue`、`backend-go/internal/httpserver/payment_center_api.go` | Billing/履约管理 | 与企业管理 API 契约冻结后修改 | clean |

说明：当前 `postgres_store.go` 的未提交修改位于 Online Image Settings 和 Asset Image Info 相关函数，`api.go`/`connector_generation.go` 的修改位于视频生成、缩略图与 Connector 视频持久化附近，内容看似不是 Enterprise V1，但文件级合并冲突风险真实存在，不能并发覆盖。

### 4.2 可以独立 worktree 并行的范围

在 API/DDL 契约冻结、且从同一可追溯基线创建 worktree 后，可按以下范围并行：

1. 管理后台 Price Plan UI：`admin-vue/src/components/billing/price-plan-admin/*` 与对应 types/domain/api/store；不与企业管理页共享编辑者。
2. 小程序页面：`AiCreationPage.vue` 可独立于企业消费页面；`EnterpriseCenterScreen.vue` 的购买和消费任务彼此不能并行。
3. 管理后台企业消费页：`EnterpriseManagement.vue` 可在企业查询 API contract 冻结后与小程序消费页并行。
4. 后端查询 API：`enterprise_types.go`、`enterprise_postgres_store.go`、admin enterprise store 可在订单/ledger 主体字段冻结后分 worktree推进。
5. 专项测试文件可在生产代码契约冻结后独立准备，但共享 fixture/migration test 文件仍需指定唯一所有者。

### 4.3 必须串行修改的文件

- 动态编号 Enterprise Quota migration/rollback。
- `price_plan_quote_v2.go`。
- `price_plan_order_v2.go`。
- `wechat_virtual_payment.go`。
- `wechat_virtual_entitlements.go`。
- `enterprise_runtime.go`。
- `postgres_store.go`。
- `server.go`。
- `apps/user-uni/src/components/enterprise/EnterpriseCenterScreen.vue`。
- Enterprise 小程序共享 API/types/store。
- admin-vue Price Plan 共享 types/domain/api/store。
- `admin-vue/src/components/enterprise/EnterpriseManagement.vue`。

### 4.4 禁止多个任务同时修改的文件

除上节文件外，以下共享入口也应按“唯一集成人”处理：

- `backend-go/internal/httpserver/api.go`
- `backend-go/internal/httpserver/connector_generation.go`
- `backend-go/internal/httpserver/connector_ppt_generation.go`
- `backend-go/internal/httpserver/payment_center_api.go`
- `admin-vue/src/components/billing/BillingCenterV1.vue`
- `admin-vue/src/App.vue`（若确需注册路由/模块）

### 4.5 建议 worktree 方案

| Worktree | 建议范围 | 前置条件 |
| --- | --- | --- |
| `ev1-db-core` | EV1-0101、0102；随后串行商品后端 | 冻结文档可追溯；重新扫描迁移号；该 worktree 无其他 migration |
| `ev1-commerce-backend` | Price Plan V2、微信虚拟支付、订单主体、履约 | EV1-0101 合入；共享四个 commerce 文件指定唯一所有者 |
| `ev1-consumption-backend` | 企业运行时、佣金隔离、查询 API | 订单/快照主体契约稳定；不得与当前 `postgres_store.go` 改动并发 |
| `ev1-miniapp` | 企业购买/消费/AI 主体页面 | 后端 contract 冻结；共享 enterprise store/API 唯一集成 |
| `ev1-admin` | 企业商品、消费、履约管理 | 管理 API contract 冻结；Price Plan 共享类型唯一集成 |

本任务不创建任何 worktree。

## 5. 数据库运行时基线

### 5.1 主表创建迁移、约束与 EV1-0101 影响

| 表 | 创建迁移 | 当前关键约束、索引或枚举 | EV1-0101 建议 |
| --- | --- | --- | --- |
| `xz_plans` | `021-runtime-projections.sql` | `id` PK；029 增加 `plan_type`、token/level 字段；047 增加 `payment_product_code`、`product_type` 及唯一部分索引；098 增加唯一 plan code 与 code 治理 trigger | 通常无需结构变更；使用既有 `plan_type`/`product_type` 表示 `ENTERPRISE_QUOTA`，除非最终契约另有独立字段 |
| `xz_plan_versions` | `097-member-agent-price-plan-v2.sql` | `business_type` CHECK 仅 `MEMBER/AGENT`；version > 0；status `DRAFT/ACTIVE/RETIRED`；plan/version 唯一；会员/代理级别条件约束；098 一 plan 仅一个 ACTIVE、发布后权益不可变 trigger | 扩展 `business_type` 为 `ENTERPRISE_QUOTA`；新增基础企业 compute units 整数字段；企业版本须正数、member/agent 字段为空、佣金禁用/永久 lot 契约可校验；同步 098 immutable trigger 的 core fields |
| `xz_price_plans` | `097-member-agent-price-plan-v2.sql` | `price_type` 为 `NORMAL/ACTIVITY/GRAY/TEST`；environment 为 `PRODUCTION/SANDBOX`；金额为正；bonus points/tokens 非负；099 增加 currency/audience CHECK、默认商品状态约束与 currency 维度默认唯一索引；098/099 有经济字段不可变治理 | 新增赠送企业 compute units 整数字段；同步经济字段不可变 trigger；补企业商品解析索引。无需新增 price_type |
| `xz_orders` | `021-runtime-projections.sql` | user/status 索引；030 增加 `buyer_user_id` 与索引；041/042 增加 `tenant_id` 与索引；047 增加微信支付/权益字段、order_no 唯一及补偿索引；061 增加统一支付字段及 `(user_id,idempotency_key)` 唯一部分索引；097 增加 V2 快照字段、FK（NOT VALID）、快照 CHECK（NOT VALID）、price plan/quote 索引；099 增加 V2 快照不可变 trigger | 新增 `billing_subject_type`；为 ENTERPRISE 强制 buyer/user、beneficiary tenant、subject 一致性；增加企业订单查询索引。保留 `user_id`/`buyer_user_id` 兼容个人和代理链路 |
| `xz_order_price_quotes` | `097-member-agent-price-plan-v2.sql` | tenant/user 必填；entry type `PUBLIC/TEST/LEGACY_PRODUCT_CODE`；snapshot_version 固定 2；status `AVAILABLE/CONSUMED/EXPIRED/CANCELLED`；quote hash 唯一、consumed order 唯一；099 增加 currency/bonus；100 增加 whitelist pin 完整性约束 | 新增 `billing_subject_type`；将 buyer + beneficiary tenant + subject 固化到报价；新增企业报价消费索引/约束，不信任客户端 tenantId |
| `xz_fulfillment_records` | `061-unified-payment-center-phase1.sql` | `fulfillment_status` 为 `PENDING/PROCESSING/SUCCESS/FAILED`；retry 非负；`UNIQUE(order_no,fulfillment_type)` | 新增 `tenant_id`、`billing_subject_type`；企业履约查询索引；保留既有唯一键或增强其企业语义，不新建履约表 |
| `xz_tenant_wallets` | `040-enterprise-center-v1.sql` | tenant PK/FK；044 增加 version、累计充值/赠送；余额非负 CHECK | **不新建企业钱包**。EV1-0101 通常不改结构；履约事务锁行并更新 point balance/version |
| `xz_compute_credit_lots` | `044-enterprise-p0-safety.sql` | source 为 `RECHARGE/BONUS/PACKAGE/ACTIVITY/LEGACY/REVERSAL/MANUAL`；original > 0；remaining 范围；status 为 `ACTIVE/EXHAUSTED/EXPIRED/REVERSED`；tenant + idempotency 唯一；consume/reference 索引 | 既有 `PACKAGE`、`BONUS` 足以发放基础/赠送 lot；通常不改表，仅确认 order 幂等键策略，必要时补 order reference 索引 |
| `xz_compute_ledger_entries` | `044-enterprise-p0-safety.sql` | entry 为 `CREDIT/DEBIT/FREEZE/UNFREEZE/REVERSAL`；source 包含 `PACKAGE/BONUS/MODEL_USAGE`；余额方程；tenant + idempotency 唯一；tenant-time/reference 索引；hash 与 append-only trigger | 既有枚举足够；按企业消费页补 actor/module/time 或 reference 查询索引，不改变 append-only 规则 |
| `xz_model_usage_records` | `044-enterprise-p0-safety.sql` | token/compute/amount 非负；status 为 `RECORDED/SETTLED/FAILED/REVERSED`；tenant + idempotency 唯一；tenant-time/task 索引；tenant+org 复合 FK | 补 tenant/user/time 与 tenant/capability/model/time 查询索引；不作为余额账本，仅作为模型用量明细 |

EV1-0101 还需按冻结任务增加三个企业购买权限码及角色映射；对应 RBAC 表不在本次十张主表清单中，但属于同一迁移的明确交付。

### 5.2 EV1-0101 建议字段范围

实施前需冻结最终命名；本基线建议与现有实施文档保持一致：

- `xz_plan_versions.compute_units_amount BIGINT NOT NULL DEFAULT 0`
- `xz_price_plans.bonus_compute_units BIGINT NOT NULL DEFAULT 0`
- `xz_order_price_quotes.billing_subject_type`
- `xz_orders.billing_subject_type`
- `xz_fulfillment_records.tenant_id`
- `xz_fulfillment_records.billing_subject_type`

建议约束：

1. `billing_subject_type` 只允许 `PERSONAL/ENTERPRISE`，历史行可先保持 NULL 兼容；新 V2 企业数据必须为 `ENTERPRISE`。
2. `ENTERPRISE_QUOTA` plan version 的基础额度必须大于 0，member/agent level 必须为空，佣金快照必须为空或明确 `commissionEligible=false`。
3. 企业 quote/order/fulfillment 必须有 tenant；购买人由认证用户派生，受益主体由服务端当前企业上下文解析。
4. rollback 只撤销尚未被业务数据依赖的新增约束/索引/列；不得删除已产生的企业业务数据。

### 5.3 Migration 与 Go 运行时差异

| 差异 | 证据与影响 | EV1 处理 |
| --- | --- | --- |
| Enterprise schema 存在运行时自建 DDL | `ensureEnterpriseCenterSchema` 在 `enterprise_postgres_store.go` 重复创建 040 的企业表；其 `xz_tenant_wallets` 定义不含 044 的 `version/total_*`，也不创建 lot/ledger/usage。若 044 未执行，后续 Enterprise Runtime 会因字段/表缺失失败 | 迁移仍是唯一正式依据；EV1 不扩大运行时自建表，发布门禁必须验证 040/044/EV1 migration 均已执行 |
| Price Plan V2 当前仅支持 MEMBER/AGENT | DB `business_type` CHECK 与 Go 查询/校验都只接受 MEMBER/AGENT；Go 类型没有基础/赠送 compute units | EV1-0101 加字段/约束后，EV1-0110/0120 必须同步 Go 管理类型、resolver、快照和 immutable 校验 |
| 已有 buyer 字段未进入履约锁单模型 | `xz_orders.buyer_user_id` 自 030 已存在，V2 下单会写；但 `lockedVirtualOrder`/`virtualOrderForUpdate` 只读取 `tenant_id`、`user_id`，没有 buyer 和 billing subject | 企业履约前必须读取并二次校验 buyer + tenant + subject；不能仅凭 `tenant_id` 发放 |
| V2 quote/response 无企业计费主体字段 | `resolvedPriceQuoteV2` 有 tenant/user，但无 `billing_subject_type` 和 enterprise compute fields；响应仅 GiftPoints/GiftTokens | EV1 报价快照新增企业主体、基础/赠送额度；前端只展示服务端结果，不能提交主体 |
| 两套支付链的 fulfillment 写入不一致 | 统一 Payment Center 的 `backend-go/internal/app/payment/service.go` 会 upsert `xz_fulfillment_records`；当前微信虚拟支付 `GrantOrderEntitlements` 路径仅更新 `xz_orders.fulfillment_status`，未写该表 | Enterprise V1 的 Price Plan V2 主链必须在 `GrantOrderEntitlements` 企业分支中写 `xz_fulfillment_records`；不启用统一 Payment Center 的 `enterprise_plan` handler |
| Model Usage 命名存在语义差异 | DDL 字段名是 `capability`，当前 Go 写入 `task.ModuleCode`；报表若按能力与模块分别统计会混淆 | V1 明确该字段按 module/capability 统一枚举使用，查询 API 暂按现存列读取；是否拆列留作技术债 |
| lot 到期与钱包余额可能不一致 | 消费只选择未到期 lot，但 `xz_tenant_wallets.point_balance` 不会因 lot 自然到期自动扣减；可能出现钱包显示有余额而可消费 lot 不足 | V1 上线门禁需有对账/到期处理策略；在未解决前，履约 lot 建议按冻结方案使用永久有效，并对余额-lot 差异 fail-closed |
| “reserve” 是立即扣减 | `reserveEnterpriseComputeTx` 会立即扣 wallet、lot 并写 DEBIT ledger，失败后再 REVERSAL | 与 ADR-007 一致，不是 V1 schema 缺陷；UI/API 不得把它描述成真实冻结余额 |
| `xz_orders` 时间字段类型混用 | 021 的 `paid_at/created_at` 为 TEXT，后续部分支付字段为 TIMESTAMPTZ；Go 大量写 RFC3339 字符串 | EV1-0101 不重构历史订单表；新查询避免依赖非标准文本时间，时间类型统一列入后续债务 |

当前 Go 与现有 040/044 wallet/lot/ledger 核心列名总体一致；上述差异是进入 Enterprise V1 时必须显式处理的边界，不建议在 EV1-0101 中顺带重构个人/代理支付模型。

## 6. EV1-0101 前置条件

进入 EV1-0101 前必须全部满足：

1. 团队确认一个可追溯基线 commit；冻结文档不能只停留在未跟踪文件。
2. 在独立、无业务脏文件的 worktree 开发；不得复用当前含 `postgres_store.go/api.go/connector_generation.go` 修改的工作区。
3. EV1-0002 功能开关与错误契约已经冻结，EV1-0003 docs-only 基线提交完成，并从该提交创建干净 worktree。
4. 在干净 worktree 完成 EV1-0003A 功能开关实现与 EV1-0003B 双支付主链禁用门禁；所有开关默认关闭、fail-closed，且统一 Payment Center 的 `enterprise_plan` 无 handler。
5. EV1-0101 会话开始时重新扫描 migration/rollback；只有扫描仍显示 102 可用时才使用 102，否则使用新的最大编号 + 1。
6. 冻结字段命名、CHECK 兼容策略、rollback 数据保留策略，以及三个企业购买权限码。
7. 指定 migration/rollback 唯一负责人；其他任务在该迁移合入前不得创建同号文件。
8. 明确只扩展 `xz_*` 运行时模型，不以 `database/schema.sql` 或非 `xz_*` 双模型为实现依据。

## 7. 当前阻塞项

| 阻塞项 | 严重度 | 解除条件 |
| --- | --- | --- |
| 当前工作区有 21 个 tracked 修改和 24 个既有 untracked 文件 | Blocker | 使用干净独立 worktree，并保留当前用户改动不受影响 |
| 三个 Enterprise 高冲突后端文件已有未提交修改 | Blocker | 不在当前 worktree 开发；由原任务完成/交接后再集成 |
| 101 已被其他并发任务未提交占用 | Blocker for 101 only | EV1 不使用 101；实施时重新扫描并动态分配 |
| Enterprise 冻结文档未跟踪，HEAD 不含实施契约 | Traceability blocker | 由负责人以可追溯方式固化冻结文档，或明确受控交接基线 |
| EV1-0002 尚无本次扫描可验证的完成证据 | Sequence blocker | 完成默认关闭开关和稳定错误契约 |
| 102 尚未预留且可能被后续并发任务占用 | Expected gate | EV1-0101 开始时重新扫描；不得依赖本文静态编号 |

## 8. Go / No-Go 结论

### 当前结论：No-Go

不允许在当前工作区直接进入 EV1-0101。原因不是数据库设计不可行，而是实施基线不可安全复现：工作区脏、共享高冲突文件已被其他任务修改、101 已被并发任务占用、冻结文档未进入 HEAD，且 EV1-0002 前置条件没有本次可验证的完成证据。

### 转为 Go 的最小条件

1. 选择包含 Enterprise V1 冻结文档的确认 commit。
2. 从该 commit 创建独立 worktree。
3. 完成 EV1-0002，并确认企业能力默认关闭。
4. 在 EV1-0101 会话重新扫描编号；若无新占用则选择 102。
5. 只创建一对动态编号 migration/rollback，并由唯一负责人串行完成。

## 9. 本任务边界确认

- 未创建 migration/rollback。
- 未占用 102。
- 未运行数据库迁移。
- 未运行测试或格式化。
- 未执行 `git reset`、`git checkout`、`git stash`。
- 未提交、未推送。
- 唯一允许的写入是新增本基线文档。
