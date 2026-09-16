# 用户级生成任务并发限制现状与策略决策审计报告

> **文档版本**: 1.0.0  
> **审计日期**: 2026-09-16  
> **关联 Issue**: [#141 audit: 用户级生成任务并发限制现状与策略决策](https://github.com/lmxchyy/zhiqiyun-ai/issues/141)  
> **代码基线**: `main` (commit `de8a24351`)  
> **审计范围**: `backend-go`, `database`, `admin-vue`, `apps` (全量只读审计，未改动业务代码与配置)

---

## 一、Executive Summary (审计执行摘要)

针对核心问题：
> **当前一个用户最多能同时提交/运行多少个生成任务？系统是否存在真正的用户级并发限制？**

审计结论为：**PARTIAL (部分能力存在真正运行时限制，但架构语义为“提交层 In-Flight 拦截”，而非“执行层 Running 调度”)**。

具体事实如下：

1. **运行时限制真实存在**：
   在 `backend-go/internal/httpserver/generation_concurrency.go` 中实现了 `enforcePostgresGenerationConcurrencyTx`，使用 PostgreSQL 事务级咨询锁 `pg_advisory_xact_lock(hashtext('generation-concurrency:' || userID))` 对单用户串行加锁，读取 `xz_plans.concurrency`，并在超限时返回 HTTP `429 Too Many Requests` (`errGenerationConcurrencyLimit`)。
2. **并发语义不是“Running”，而是“In-Flight”**：
   系统统计状态包含 `('PENDING','QUEUED','RUNNING','PROCESSING','RETRYING','CREATED')`。因此，哪怕任务还在排队（Queued），也立刻消耗 1 个并发名额。用户**无法批量提交任务到排队区**，超过套餐并发额度即直接被 API 拒绝。
3. **能力池未完全拉通 (存在绕过与不对称)**：
   - **Image (生图)** 与 **Video (视频)**：完全纳入 `xz_generation_tasks` 统一并发池，严格共享并发上限。
   - **PPT (演示文稿)**：若走 Async Canary 路径，纳入统一池；若走 Non-Canary 路径，仅在提交瞬间检查 3 秒内的本地任务 (`created_at > now() - interval '3 seconds'`)，且与生图使用不同的 Advisory Lock Key (`ppt:user:` vs `generation-concurrency:`)，存在并发泄漏与绕过。此外，PPT 生成单页配图时会作为子任务再次请求生图接口，可能因母任务占用并发而导致配图失败降级。
   - **SmartVideo (智能混剪/视频项目)**：完全独立于 `xz_generation_tasks`，仅有每日 20 次 Plan 限制，**无任何用户并发限制**。
   - **Agent / Knowledge RAG (知识库/智能体)**：HTTP 同步/流式直接调用上游模型，**无任何用户并发限制**。
   - **Enterprise (企业版)**：代码中显式 `if authorization.ContextType == contextEnterprise { return nil }`，**完全跳过用户并发检查，且目前租户级并发限制为 0 (未实现租户并发上限)**。
4. **RabbitMQ / Worker 层无用户并发感知**：
   Worker 的 `Prefetch(1)` 和 `MaxConcurrency(1)` 仅为 Worker 进程的消费速率与全局工作线程上限，Worker 从 MQ 取出任务时**完全不检查用户并发**。

---

## 二、当前系统架构与限制层级图

```text
                                  Client Request
                    (Web / WeChat MiniProgram / Open API)
                                        │
                                        ▼
                   ┌─────────────────────────────────────────┐
                   │           HTTP API Router               │
                   │        (backend-go/httpserver)          │
                   └────────────────────┬────────────────────┘
                                        │
        ┌───────────────────────────────┴───────────────────────────────┐
        ▼                                                               ▼
【SmartVideo / Agent】                                        【Image / Video / PPT】
  /video-projects/*                                             /generation-tasks
  /knowledge-conversations/*                                    /ppt/generate
        │                                                               │
        │ (无用户并发限制)                                               ▼
        │                                             ┌───────────────────────────────────┐
        │                                             │   Postgres 事务级 Advisory Lock   │
        │                                             │ pg_advisory_xact_lock(...)         │
        │                                             │ Key: generation-concurrency:user_id│
        │                                             └─────────────────┬─────────────────┘
        │                                                               │
        │                                                               ▼
        │                                             ┌───────────────────────────────────┐
        │                                             │   [真正生效的并发限制层]            │
        │                                             │ enforcePostgresGenerationConcurrency│
        │                                             │ COUNT(xz_generation_tasks)        │
        │                                             │ WHERE status IN (PENDING,QUEUED,  │
        │                                             │   RUNNING,PROCESSING,RETRYING...) │
        │                                             │ >= plan.concurrency               │
        │                                             └─────────┬───────────────┬─────────┘
        │                                                       │               │
        │                                            超过限制   │               │ 未超限制
        │                                                       ▼               ▼
        │                                              HTTP 429 Rejection    Durable Task DB
        │                                              (直接报错拒绝提交)    (xz_generation_tasks)
        │                                                                       │
        │                                                                       ▼
        │                                                                 Transactional Outbox
        │                                                                 (xz_outbox_messages)
        │                                                                       │
        │                                                                       ▼
        │                                                                 RabbitMQ Exchange
        │                                                                       │
        │                                                                       ▼
        │                                                                 RabbitMQ Queues
        │                                                                       │
        │                                                                       ▼
        │                                                             ┌───────────────────┐
        │                                                             │ Worker Consumers  │
        │                                                             │ (Prefetch=1,      │
        │                                                             │  MaxConcurrency=1)│
        │                                                             │ [无用户并发检查]   │
        │                                                             └─────────┬─────────┘
        │                                                                       │
        ▼                                                                       ▼
  Direct Upstream /                                                       Provider Connector
  Local FFmpeg / Worker                                                   (Seedance/Volcengine
                                                                           Minimax/RunningHub)
```

---

## 三、套餐配置 vs 实际执行映射表 (Plan Mapping)

代码证据源：
- 数据库定义: `database/migrations/021-runtime-projections.sql`
- 数据库种子数据: `database/migrations/026-subscription-sku-cycles.sql`
- 静态配置代码: `backend-go/internal/httpserver/pricing_catalog.go`
- 后台编辑组件: `admin-vue/src/components/billing/PlanEditorDialog.vue`
- 小程序前端展示: `apps/user-uni/src/pages/AiCreationPage.vue`
- 运行时读取执行: `backend-go/internal/httpserver/generation_concurrency.go`

| 套餐名称 | Plan ID | 数据库/配置 Concurrency 字段 | 前端宣传文案 (AiCreationPage) | 实际运行时执行值 | 执行状态说明 |
|---|---|---|---|---|---|
| **体验版 (Free/Trial)** | `plan_free` | `1` | 1个 | **1** | 代码读取 `plan.concurrency`，限制最多 1 个 in-flight 任务 |
| **Basic 连续包月** | `plan_month` | `3` | 3个 | **3** | 代码读取 `plan.concurrency`，限制最多 3 个 in-flight 任务 |
| **Basic 单月购买** | `plan_basic_single` | `3` | 3个 | **3** | 与 `plan_month` 逻辑一致 |
| **Basic 连续包年** | `plan_basic_year` | `3` | 3个 | **3** | 与 `plan_month` 逻辑一致 |
| **Pro 连续包月** | `plan_pro` | `8` | 8个 | **8** | 代码读取 `plan.concurrency`，限制最多 8 个 in-flight 任务 |
| **Pro 单月购买** | `plan_pro_single` | `8` | 8个 | **8** | 与 `plan_pro` 逻辑一致 |
| **Pro 连续包年** | `plan_pro_year` | `8` | 8个 | **8** | 与 `plan_pro` 逻辑一致 |
| **Pro 996年度会员** | `plan_ai_creator_996` | `8` | 8个 | **8** | 与 `plan_pro` 逻辑一致 |
| **Ultimate 连续包月** | `plan_year` | `20` | 20个 | **20** | 代码读取 `plan.concurrency`，限制最多 20 个 in-flight 任务 |
| **Ultimate 单月购买** | `plan_ultimate_single` | `20` | 20个 | **20** | 与 `plan_year` 逻辑一致 |
| **Ultimate 连续包年** | `plan_ultimate_year` | `20` | 20个 | **20** | 与 `plan_year` 逻辑一致 |
| **企业版 (Enterprise)** | `plan_enterprise` | `0` | 定制 | **无限制 (Bypass)** | 代码判断 `concurrency == 0` 或 `ContextType == contextEnterprise` 直接返回 `nil` |
| **单次点数充值包** | `recharge_100` 等 | `0` | - | **继承原套餐** | 充值包不改动用户的 `plan_id`，用户继续保持其会员套餐并发 |
| **未知/历史兜底** | (未关联 Plan) | `-1` / null | - | **1 (Fallback)** | 代码兜底：`if concurrency < 0 { concurrency = 1 }` |

---

## 四、各生成能力全链路审计矩阵

| 能力 | 用户提交限制 (Admission) | 用户 Running 限制 | Worker 级别限制 | Provider 上游限制 | 套餐联动控制 |
|---|---|---|---|---|---|
| **Image (生图)** | **严格限制**<br>· 检查点: `POST /api/v1/generation-tasks`<br>· 代码: `postgres_store.go:1214, 1415`<br>· 机制: 事务锁 + COUNT >= plan.concurrency 报 429 | **无独立 Running 限制**<br>（直接限制 In-Flight 总数，Running 包含在内） | · Consumer: `RunGenerationImageCanaryWorker`<br>· 限制: `Prefetch(1)`, `MaxConcurrency(1)`<br>· 代码: `generation_worker.go:48` | · 框架: `providerexecution`<br>· 指纹幂等 + 防重发<br>· 超时: `ImageGenerationTimeout` (~3-5m) | **生效**<br>读取 `xz_plans.concurrency` |
| **Video (视频)** | **严格限制**<br>· 检查点: `POST /api/v1/generation-tasks`<br>· 代码: 与 Image 共用 `createPendingGenerationTask`<br>· 与 Image 共享同一并发池 | **无独立 Running 限制**<br>（与 Image 共享 In-Flight 计数） | · Consumer: `RunGenerationVideoCanaryWorker`<br>· 限制: `Prefetch(1)`, `MaxConcurrency(1)`<br>· 代码: `video_generation_worker.go:37` | · 规范指纹: `video_fingerprint.go`<br>· 超时: `videoGenerationTimeout = 20 * time.Minute` | **生效**<br>读取 `xz_plans.concurrency` |
| **PPT (大纲与幻灯片)** | **部分限制 (双轨分支)**<br>· Canary 模式: 走 `generation-tasks`，全量计入 In-flight<br>· Non-Canary 模式: 走 `GenerateWithConcurrency` (`postgres.go:144`)，仅检查 3 秒内本地任务 | **单 Deck 并发限制**<br>· 幻灯片图片池并发上限为 3 (`pptSlideImagePoolSize = 3`)<br>· 代码: `ppt_generation_stages.go:24` | · Consumer: `RunGenerationPPTCanaryWorker`<br>· 限制: `Prefetch(1)`, `MaxConcurrency(1)`<br>· 代码: `ppt_generation_worker.go:48` | · 超时: Deck 30m, 规划 35s, 配图 115s<br>· OCR 校验防包含文字 | **生效**<br>读取 `xz_plans.concurrency` |
| **PPT 配图子任务** | **受制于母任务并发**<br>· 代码: `ppt_api.go:1101`<br>· 机制: 每张配图创建 `createPendingGenerationTask`，若用户并发已满则配图被 429 拦截降级 | 同上 | 同上 | 同上 | **间接生效**<br>受父用户并发制约 |
| **Agent / Knowledge RAG** | **无限制 (0 拦截)**<br>· 检查点: `POST /knowledge-conversations/:id/runs`<br>· 代码: `server.go:501`, `rag_service.go` | **无限制** | 无 Worker (API 同步/流式直连) | · 仅 HTTP 超时控制 (`ModelTimeoutMS`, 默认 30s) | **无套餐并发联动**<br>(仅扣除算力点数) |
| **SmartVideo (智能混剪)** | **无并发限制**<br>· 仅每日 Plan 上限 20 (`model.go:11`)<br>· 代码: `plan_service.go:37` | **无限制** | · Redis Queue 驱动<br>· 限制: `SMARTVIDEO_*_CONCURRENCY`<br>· 代码: `smartvideoruntime/runtime.go` | · 本地 FFmpeg 进程超时 (10m) | **无套餐并发联动** |

---

## 五、“并发”语义的源码级界定

在产品定义与实际工程落地之间，存在以下四种典型理解：
- **A. 最多同时运行 N 个 (Running Only)**
- **B. 最多同时存在 N 个在途任务 (In-Flight: Created + Queued + Running)**
- **C. 单个 capability 独立 N 个 (Image N 个, Video N 个, PPT N 个)**
- **D. 整个账号统一 N 个 (Image + Video + PPT 共享 N 个)**

### 源码证据剖析

在 `backend-go/internal/httpserver/generation_concurrency.go` 第 39-44 行：
```sql
select count(*)
from xz_generation_tasks
where user_id=$1
  and upper(coalesce(nullif(task_status,''),status)) in (
      'PENDING','QUEUED','RUNNING','PROCESSING','RETRYING','CREATED'
  )
```

**明确结论**：
1. **当前系统执行的是 B + D 组合语义**：
   - **B 语义 (In-Flight 限制)**：只要任务状态处于 `CREATED`、`PENDING`、`QUEUED`、`RUNNING`、`PROCESSING`、`RETRYING` 任意一种，均全额计入用户的并发占用数。
   - **D 语义 (统一池)**：SQL 统计直接以 `user_id` 汇总，未区分 `type`（未区分生图、生视频）。一个 Basic 会员（并发 3）如果提交了 2 个生图任务排队，同时又提交了 1 个生视频任务排队，并发计数即达到 3。
2. **核心体验后果**：
   - 现存系统**不是“提交后在后台慢慢排队”**，而是**“提交时若达到并发数就直接报错 429 拒收”**。
   - 这意味着所谓的“并发 3”，用户无法一口气发 10 个生图让系统逐个跑，发到第 4 个直接弹窗报错 `package plan_month has 3 active task(s), limit 3`。

---

## 六、用户绕过场景与边界风险审计

### 场景 1：用户连续极速点击 / 并发批量请求 (Race Condition)
- **代码实现**：
  `enforcePostgresGenerationConcurrencyTx` 内第一条执行语句：
  ```sql
  select pg_advisory_xact_lock(hashtext('generation-concurrency:' || $1))
  ```
- **审计结论**：**无法绕过 (安全)**。
  所有创建任务的 HTTP 请求均在独立的 PostgreSQL 事务内执行。Advisory Lock 强行使同一 `userID` 的所有创建请求在数据库层严格串行化执行。请求 A 完成插入并提交后释放锁，请求 B 才能开始读取 `count(*)`，能够精准看到请求 A 插入的记录。不存在高频连击穿透并发限制的竞态条件。

### 场景 2：跨不同能力 API 相互绕过
- **代码实现**：
  - Image 和 Video 走统一的 `POST /generation-tasks`，共享 `xz_generation_tasks`。
  - Non-Canary PPT (`POST /api/ppt/generate`) 写入 `xz_ppt_tasks`，锁为 `ppt:user:`，且统计条件为 `created_at > now() - interval '3 seconds'` (`postgres.go:144`)。
  - SmartVideo 与 Agent 知识库完全不调用并发检查。
- **审计结论**：**存在绕过 (不一致)**。
  - 用户提交 1 个 PPT 任务（耗时 1~2 分钟），3 秒后其在 `xz_ppt_tasks` 的 3 秒窗口失效；此时用户可全额提交 3 个生图任务。实际上同时有 1 个 PPT 和 3 个生图在跑（突破套餐 3 并发）。
  - 用户可以无限制同时发起 SmartVideo 混剪任务和 Agent 问答，不受套餐并发约束。

### 场景 3：Web 端与微信小程序端行为是否一致
- **代码实现**：
  Web 前端 (`apps/user-uni` H5 / PC) 与微信小程序端使用相同的后端 REST 接口 (`/api/v1/generation-tasks`)，经过相同的 JWT 鉴权解析出 `user.ID`。
- **审计结论**：**完全一致**。
  小程序端的合规拦截 (`checkMiniProgramText`) 发生在并发检查之前，通过后进入完全相同的数据库事务与并发锁逻辑。

### 场景 4：同一企业租户多成员 (User Concurrency vs Tenant Concurrency)
- **代码实现**：
  `generation_concurrency.go` 第 18-21 行：
  ```go
  if authorization.ContextType == contextEnterprise {
      // Enterprise concurrency is governed by tenant compute and seat policies.
      return nil
  }
  ```
  `enterprise_runtime.go` 第 293 行 `reserveEnterpriseComputeTx`：
  仅检查并扣减企业租户算力余额 (`xz_tenant_wallets`)，没有任何租户级或用户级并发校验。
- **审计结论**：**存在穿透 (企业模式无并发限制)**。
  - 个人版套餐：按用户独立计算，即使同属于一个组织，也是每人各自使用各自的套餐并发。
  - 企业版模式：代码直接放行 (`return nil`)，既没有限制“单企业全局总并发”，也没有限制“企业子成员单人并发”。如果企业有多人同时操作，可以发起数十上百个并发任务，仅受企业点数余额限制。

### 场景 5：Worker Crash 导致任务挂起 (卡死占用额度)
- **代码实现**：
  Worker 崩溃时，任务在 `xz_generation_tasks` 中停留在 `RUNNING` 或 `PROCESSING`。
  系统依赖 `backend-go/internal/httpserver/api.go` 中的 `runGenerationStaleWatchdog` 巡检：
  ```go
  videoGenerationTimeout      = 20 * time.Minute
  otherGenerationStaleTimeout = 15 * time.Minute
  generationStaleWatchdogInterval = time.Minute
  ```
- **审计结论**：**存在长时间卡槽风险**。
  若 Worker 在执行过程中遭遇 OOM 或突发宕机，该任务将持续占用用户的并发槽位长达 **15 ~ 20 分钟**，直到 Watchdog 判定超时并触发 `FAILED` 状态修复。在此期间，用户将持续收到 429 报错，无法提交新任务。

### 场景 6：任务 Failed / Cancelled 之后释放
- **代码实现**：
  任务失败或取消时，状态变更为 `FAILED` 或 `CANCELLED`。
  并发统计 SQL 仅查询 `'PENDING','QUEUED','RUNNING','PROCESSING','RETRYING','CREATED'`。
- **审计结论**：**释放正确且实时**。
  一旦事务将任务标记为终态，SQL `count(*)` 立即不包含该任务，槽位立刻释放。

---

## 七、并发控制与积分系统的解耦性验证

审计重点：**套餐并发额度与点数余额是否独立实现？**

**源码证据**：
在 `backend-go/internal/httpserver/postgres_store.go` 中（以 `createPendingGenerationTaskWithPPT` 为例）：
1. **并发准入检查 (第 1415 行)**：
   ```go
   if err := enforcePostgresGenerationConcurrencyTx(ctx, tx, userID, authorization); err != nil {
       return generationTask{}, err
   }
   ```
2. **算力点数校验与冻结 (第 1424 行)**：
   ```go
   if account.Available < int64(pointCost) {
       return generationTask{}, newInsufficientPointsError(account.Available, int64(pointCost))
   }
   ```
3. **两者的逻辑与存储完全独立**：
   - **点数**：由 `xz_point_accounts`、`xz_personal_point_lots`、`xz_personal_point_reservations` 支撑，关注的是**资费计量与账户偿付能力**。
   - **并发**：由 `xz_generation_tasks` 的状态统计与 `xz_plans.concurrency` 支撑，关注的是**瞬时管道吞吐能力**。
   - 用户购买单次点数充值包（如 `recharge_100`），仅增加可用点数，`concurrency` 保持为 0（即不提升并发等级，继承用户原有会员等级）。

---

## 八、并发策略选项评估 (Option A ~ D)

针对知启云 AI 未来的并发架构，根据现有代码基线梳理出以下四种演进方案：

| 方案 | 核心定义 | 优点 | 缺点 | 适用场景 |
|---|---|---|---|---|
| **Option A: 限制 Running<br>(允许排队 Queued)**<br>*(推荐演进方向)* | · 允许用户提交最多 M 个任务进入 `QUEUED`<br>· 仅严格限制处于 `RUNNING` 的任务数 <= 套餐并发数 N<br>· 后台调度器按 N 放行 | · 用户体验极佳（可批量连续点 10 张图）<br>· 符合异步任务系统的行业惯例（Midjourney/可灵）<br>· 充分利用离线 Worker 产能 | · 需要新增调度器/分发机制<br>· 需防止单个用户狂刷大量 Queued 占满 DB | **异步任务生产系统的最优选择** |
| **Option B: 限制 In-Flight<br>(当前现状)** | · 限制 `QUEUED + RUNNING` 总和 <= 套餐并发数 N<br>· 超过立即报 429 拒绝提交 | · 实现极为简单（当前即此状态）<br>· 对系统内部积压保护最好<br>· 零调度复杂度 | · 用户体验差：Basic 用户点完 3 张必须等 1 张跑完才能点下一张<br>· 前端批量提交功能直接瘫痪 | 当前 baseline 现状 |
| **Option C: 能力独立隔离并发** | · 生图 N 个，视频 M 个，PPT K 个独立限额<br>· 例如 Basic: 图 3 / 视 1 / PPT 1 | · 隔离重任务（视频 5 分钟）对轻任务（生图 15 秒）的阻塞<br>· 便于精细化控制各上游 Provider 成本 | · 套餐权益过于繁琐，用户理解成本高<br>· 数据库与代码需重构为多维度计数 | 平台重资产视频量极大时 |
| **Option D: 全能力统一池<br>(当前演进中状态)** | · Image + Video + PPT 共享总并发 N<br>· Basic=3, Pro=8, Ultimate=20 | · 概念统一，用户易于理解<br>· 代码计数模型收敛于一张主表 | · 慢任务（视频）容易把并发占死，导致用户无法生图 | 中小型 SaaS 的平衡折中选择 |

---

## 九、架构深度风险分析：RabbitMQ Claim 与公平调度 (Fair Scheduling)

用户在 Issue 中特别提出了一个高危架构假设：
> **如果采用“Worker 从 RabbitMQ 消费任务后，才检查用户并发”，会发生什么？**

### 1. 致命缺陷分析 (Head-of-Line Blocking 与死循环)
若在 Worker 端进行并发检查：
```text
RabbitMQ Queue (FIFO)
[ UserA-Task4, UserA-Task5, ..., UserA-Task100, UserB-Task1 ]
       ▲
       │
Worker 消费 UserA-Task4 ──发现 UserA 已有 3 个正在运行 (并发超限)
```
此时 Worker 面临绝境：
1. **如果 `requeue = true`**：
   任务被立即退回 RabbitMQ 队头。Worker 下一次拉取的依然是 UserA-Task4！系统瞬间陷入 **100% CPU 空转死循环 (Busy Loop Thrashing)**。
2. **如果发往延迟队列 (Retry/Delay Exchange)**：
   虽然避开了立即死循环，但由于 RabbitMQ 队列前面积压了 UserA 的 100 个任务，**UserB 的任务被硬生生卡在队列最后**。
   Worker 必须把 UserA 的 97 个超限任务全拉一遍、NACK 一遍，UserB 才能被消费。这就是严重的 **队头阻塞 (Head-of-Line Blocking) 与饥饿 (Starvation)**。

### 2. 正确解法：两级准入调度体系 (Two-Tier Admission & Dispatch)
坚决**不能**让 Worker 消费 MQ 后再做并发拒绝。合理的架构应为：

```text
[第 1 级: API 缓冲准入]
用户提交任务 ──► 检查用户在途总数 (Running + Queued) 是否超过防刷软上限 (例如 50)
                 │
                 ├── 超出 50: 报 429 (防止恶意狂刷打爆 DB)
                 └── 未超出: 写入 xz_generation_tasks，状态为 'QUEUED'
                     注意：此时【不直接】投递到 RabbitMQ！

[第 2 级: 数据库级公平调度器 (Scheduler / Outbox Dispatcher)]
后台独立调度组件 (每秒轮询或状态触发):
1. 统计当前正在 RUNNING 的用户任务
2. 筛选出处于 QUEUED 且其所属用户 RUNNING < plan.concurrency 的任务 (Fair-Share Round Robin)
3. 将这批任务的状态置为 'DISPATCHED'，并写入 Outbox 发往 RabbitMQ！
                 │
                 ▼
[第 3 级: RabbitMQ & Worker]
RabbitMQ 队列里永远【只有具备执行资格】的任务！
Worker 只管无脑 claim 执行，永不退回，永不阻塞其他用户！
```

---

## 十、审计结论与下一步建议

### 1. 现状核心定性
- **知启云系统当前“确实存在”用户级并发限制**，且确实读取了 `Basic=3 / Pro=8 / Ultimate=20`。
- **但其本质是“API 提交拦截器 (In-Flight Limit 429)”**，而不是“异步排队调度器 (Running Limit with Queue)”。
- **能力间存在明显裂隙**：Image 和 Video 严格纳管并共享池；PPT 存在 Canary/Non-canary 历史双轨且时间窗口存在漏洞；SmartVideo 和 Agent 完全脱管；企业版完全放行。

### 2. 决策与行动建议

建议当前状态：**HOLD (暂不修改代码，等待产品与架构评审确认策略)**。

若后续启动改造，预估涉及：
1. **需修改模块**：
   - `backend-go/internal/httpserver/generation_concurrency.go` (重构准入与计数逻辑)
   - `backend-go/internal/httpserver/ppt_api.go` 与 `backend-go/internal/app/ppt/` (彻底下线 3 秒窗口漏洞，全面归一到统一池)
   - `backend-go/internal/httpserver/smart_video_api.go` (评估是否纳管)
   - 调度层 (需设计或引入轻量级 Outbox 排队放行调度器)
2. **数据库迁移**：
   - 无需破坏性重构，但可能需要在 `xz_generation_tasks` 增加调度辅助索引，如 `(user_id, status, created_at)`。
   - 若引入租户级并发，需在 `xz_enterprises` / `xz_tenants` 新增 `max_concurrency` 字段。
3. **接口兼容性**：
   - 若改为“允许 Queued 排队”，API 将不再返回 429，而是返回 200/202 (任务状态为 `QUEUED`)，对前端用户体验是纯收益，完全后向兼容。

---
*报告生成于 worktree: `.worktrees/audit-user-concurrency-limits`*  
*关联分支: `audit/user-concurrency-limits`*
