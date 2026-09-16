# 用户级 Running 并发与 Fair Scheduler 架构设计方案

> **文档版本**: 1.0.0  
> **设计日期**: 2026-09-16  
> **关联 Issue**: [#142 feat: 用户级 Running 并发与 Fair Scheduler 调度](https://github.com/lmxchyy/zhiqiyun-ai/issues/142)  
> **依据审计**: [#141](https://github.com/lmxchyy/zhiqiyun-ai/issues/141) / `docs/audit/USER-CONCURRENCY-AUDIT.md` (Commit `fc8d08b68`)  
> **实现分支**: `feat/user-concurrency-fair-scheduler`  
> **隔离工作区**: `.worktrees/user-concurrency-fair-scheduler`

---

## 一、架构演进目标与核心语义

### 1. 核心问题与重构目标
根据 Issue #141 的只读审计证据，知启云 AI 系统当前存在用户级并发限制（`enforcePostgresGenerationConcurrencyTx`），但其统计口径为在途任务总数（`CREATED + QUEUED + RUNNING + PROCESSING + RETRYING`），并在达到套餐上限时在 API 提交层直接返回 HTTP 429。这导致用户无法进行批发生图/视频提交，且多任务无法通过异步队列排队消化。

**重构目标**：
1. **语义解耦**：将“套餐并发”从“提交准入限制”转变为“执行时 Running 限制”。
2. **异步缓冲**：允许用户提交超过并发数的任务进入 `QUEUED` 状态排队。
3. **前置调度**：在任务进入 RabbitMQ 之前，由基于 PostgreSQL 的 Fair Scheduler（公平调度器）根据用户当前 Running 占用数与套餐额度严格按需放行。
4. **杜绝饥饿**：采用 User-level Round-Robin / Fair-Share 调度，彻底杜绝单大用户百级排队导致的全局队头阻塞（Head-of-Line Blocking）和中小用户饥饿。
5. **分阶段演进**：第一阶段（Phase 1）以最小爆炸半径聚焦收敛 **Image + Video** 统一调度主链，验证通过后再收敛 PPT（Phase 2）与智能体/混剪（Phase 3）。

---

## 二、状态机与端到端拓扑架构

### 1. 任务全生命周期状态机

```text
               ┌────────────────────────┐
               │    Client HTTP POST    │
               └───────────┬────────────┘
                           │
                           ▼
          ┌──────────────────────────────────┐
          │     API Admission 准入层          │
          │ 1. 鉴权 & 订阅有效期解析          │
          │ 2. 算力点数冻结 (Reserve Points)  │
          │ 3. Queue Backlog 软上限检查 (防刷)│
          └────────────────┬─────────────────┘
                           │ (合格写入 DB)
                           ▼
          ┌──────────────────────────────────┐
          │      Status: [QUEUED]            │
          │ (落库 xz_generation_tasks)       │
          │ 【不直接投递 RabbitMQ Outbox】   │
          └────────────────┬─────────────────┘
                           │
                           ▼ (Fair Scheduler 轮询抓取)
          ┌──────────────────────────────────┐
          │      Status: [DISPATCHING]       │
          │ 1. 原子获取用户 Running 槽位     │
          │ 2. 写入 xz_outbox_messages       │
          │ 3. 标记 dispatched_at = NOW()   │
          └────────────────┬─────────────────┘
                           │
                           ▼ (Outbox Publisher)
          ┌──────────────────────────────────┐
          │        RabbitMQ Exchange         │
          │  (x.ai.generation.image.canary)  │
          │  (x.ai.generation.video.canary)  │
          └────────────────┬─────────────────┘
                           │
                           ▼ (Worker Claim via Inbox)
          ┌──────────────────────────────────┐
          │      Status: [RUNNING]           │
          │ (开始调用 Provider 上游模型)     │
          └────────────────┬─────────────────┘
                           │
             ┌─────────────┴─────────────┐
             ▼                           ▼
  ┌──────────────────────┐   ┌──────────────────────┐
  │ Status: [SUCCEEDED]  │   │   Status: [FAILED]   │
  │ · 算力点数 Capture   │   │ · 算力点数 Release   │
  │ · 释放 Running 槽位  │   │ · 释放 Running 槽位  │
  └──────────────────────┘   └──────────────────────┘
```

---

## 三、关键设计决策与详细方案

### 1. 并发统计口径纯粹化
- **Running 统计状态集**：
  仅统计真正正在占用算力管道和 Provider 执行资源的任务：
  ```sql
  WHERE user_id = $1
    AND upper(coalesce(nullif(task_status,''), status)) IN ('DISPATCHING', 'RUNNING', 'PROCESSING')
  ```
- **彻底剔除的状态**：
  `QUEUED`、`CREATED` 明确不再计入 Running 并发。
  `FAILED`、`SUCCEEDED`、`CANCELLED` 属于终态，立即排除。

### 2. 排队积压软上限 (Queue Backlog Limit)
为防止恶意刷单、API 脚本无限提交导致数据库存储爆炸，系统必须在 API 提交层设定 `max_queued_per_user` 软上限：

| 套餐级别 | Running 并发额度 | 最大允许排队数 (`max_queued_per_user`) | 总在途上限 (Running + Queued) |
|---|---|---|---|
| **Free (体验版)** | 1 | 10 | 11 |
| **Basic (包月/包年)** | 3 | 30 | 33 |
| **Pro (包月/包年/996)** | 8 | 50 | 58 |
| **Ultimate (包月/包年)** | 20 | 100 | 120 |
| **Enterprise (企业版)** | 20 (安全基线) | 200 (可配置) | 220 |
| **默认/未知兜底** | 1 | 10 | 11 |

- **超限错误**：
  当用户已在排队的任务数达到 `max_queued_per_user` 时，API 返回 HTTP 429，附带明确业务错误体：
  ```json
  {
    "code": "GENERATION_QUEUE_LIMIT_EXCEEDED",
    "message": "当前排队任务已达上限 (30/30)，请等待前序任务完成后再提交",
    "details": {
      "running": 3,
      "queued": 30,
      "max_queued": 30
    }
  }
  ```

### 3. Fair Scheduler (公平调度器) 设计规范

#### 核心硬约束：调度器必须在 RabbitMQ 之前
严格禁止将未经并发准入的任务放入 RabbitMQ。RabbitMQ 仅作为“已获取并发名额的任务派发执行通道”。

#### 调度算法 (User-Level Fair-Share Round-Robin)
调度器按周期循环（每 500ms ~ 1s 触发一次调度 Tick）：
1. **聚合活跃排队用户**：
   ```sql
   SELECT user_id, min(created_at) as oldest_task_at
   FROM xz_generation_tasks
   WHERE upper(coalesce(nullif(task_status,''), status)) = 'QUEUED'
     AND capability IN ('image', 'video')
   GROUP BY user_id
   ORDER BY oldest_task_at ASC
   LIMIT 100;
   ```
2. **用户槽位配额计算**：
   对每个候选用户：
   - 读取有效套餐并发额度 $C$（若订阅已过期，降级回落为 Free 并发 1）。
   - 统计该用户当前 Running 占用数 $R$（包含 `DISPATCHING`, `RUNNING`, `PROCESSING`）。
   - 可用调度名额 $Available = \max(0, C - R)$。
   - 若 $Available == 0$，跳过该用户，不予派发。
3. **原子锁定与派发 (SKIP LOCKED)**：
   若 $Available > 0$，开启事务原子获取该用户最早的 $Available$ 个排队任务：
   ```sql
   SELECT id, type, model, prompt, params
   FROM xz_generation_tasks
   WHERE user_id = $1
     AND upper(coalesce(nullif(task_status,''), status)) = 'QUEUED'
   ORDER BY created_at ASC
   LIMIT $2
   FOR UPDATE SKIP LOCKED;
   ```
   在同一事务中：
   - 将任务状态变更为 `task_status = 'DISPATCHING'`，更新 `updated_at = NOW()`，记录 `dispatched_at = NOW()`。
   - 组装 Outbox 事件（如 Image Canary 事件 `x.ai.generation.image.canary.requested`），写入 `xz_outbox_messages`。
   - 事务提交。
4. **Outbox 发布与 Worker 消费**：
   - Outbox 轮询发布器将消息发布至 RabbitMQ。
   - Worker 消费消息并 claim 成功后，将 `task_status` 置为 `RUNNING`，开始真正上游生成。

#### 杜绝队头阻塞与饥饿验证推导：
- **场景**：User A 提交 100 个任务（Basic，并发 3），User B 提交 1 个任务（Basic，并发 3）。
- **调度过程**：
  - 调度器 Tick 1：User A 派发 3 个任务，Running 占用变为 3。User B 派发 1 个任务，Running 占用变为 1。两者同时进入 RabbitMQ 并行执行。
  - 调度器 Tick 2：User A 的 Running 为 3，无剩余可用槽位，被跳过；User B 无排队任务。
  - User B 完全不需要等待 User A 的后 97 个任务排完，立刻被执行！

### 4. 算力点数冻结策略 (Points Reserve vs Capture)
- **采用 Option 1 (提交时全额 Freeze)**：
  - 依据现有 `personalPointLots` 体系，用户在创建任务（即使进入 `QUEUED`）时，系统立刻执行 `service.Reserve(...)` 冻结预估点数。
  - **设计理由**：
    1. 杜绝空头支票：避免用户仅有 100 点却排队提交 10,000 点的任务，导致后续调度到前台时扣费失败、产生坏账或无效计算。
    2. 配合 `max_queued_per_user`：由于排队数量已受软上限保护（如 Basic 最多 30 个），冻结金额被严格封顶在合理范围内。
    3. 取消与失败自动释放：任务若在 `QUEUED` 状态被用户手动取消（Cancel），或排队超时失败，系统事务调用 `service.Release(...)`，被冻结的点数瞬间原路退回可用余额。

### 5. 订阅过期与企业版漏洞修复
1. **订阅过期自动降级**：
   在计算用户套餐并发额度时，执行统一定义的 `resolveUserEffectiveConcurrency`：
   ```go
   func resolveUserEffectiveConcurrency(planConcurrency int, expiresAt *time.Time) int {
       if expiresAt != nil && expiresAt.Before(time.Now().UTC()) {
           return 1 // 过期自动降级为 Free 额度 (并发 1)
       }
       if planConcurrency <= 0 {
           return 1 // 未知或异常兜底
       }
       return planConcurrency
   }
   ```
2. **企业版 (Enterprise) 防穿透安全基线**：
   - 废除无条件 `if ContextType == contextEnterprise { return nil }` 放任无限制的逻辑。
   - 本期为企业版设置安全基线并发上限（如单企业默认最大 Running 并发 20，支持环境变量/配置覆盖）。
   - 明确将完整的“多级企业组织算力分配与席位并发管理”留存后续专项演进。

### 6. Crash Recovery 与超时槽位回收
1. **DISPATCHING 超时回收**：
   若任务进入 `DISPATCHING` 后超过 60 秒依然未被 Worker 接管为 `RUNNING`（可能因 Outbox 崩溃或 RabbitMQ 断连）：
   调度器的自愈循环将超时的 `DISPATCHING` 任务重置回 `QUEUED`，释放虚增的 Running 计数。
2. **RUNNING 任务崩溃回收**：
   维持并强化 `runGenerationStaleWatchdog` 巡检逻辑。当 Worker 宕机导致任务长时间僵死：
   Watchdog 判定超时（图片 3~5 分钟，视频 20 分钟），事务将任务变更为 `FAILED`，并释放冻结算力点数，Running 槽位随之自动清空。

---

## 四、分阶段交付演进路线 (Phased Rollout)

为保证业务系统的极致稳定性与最小爆炸半径，严格划分为三个阶段：

```text
┌────────────────────────────────────────────────────────┐
│ Phase 1: 核心收敛 (本 Issue 交付范围)                   │
│ · Image (生图) + Video (生视频)                        │
│ · 状态机演进: QUEUED -> DISPATCHING -> RUNNING        │
│ · 准入排队软上限 (Backlog Limit)                       │
│ · Fair Scheduler 独立公平调度器                        │
│ · 订阅过期自动回落 Free 并发                           │
│ · 单元测试、并发竞争与防饥饿集成测试                   │
└───────────────────────────┬────────────────────────────┘
                            │ (生产验证通过)
                            ▼
┌────────────────────────────────────────────────────────┐
│ Phase 2: PPT 彻底合流 (Follow-up)                      │
│ · 废除 Non-Canary 3 秒窗口漏洞                         │
│ · PPT 生成大纲作为 Parent Task 入统一池                │
│ · 单页配图作为 Internal Subtask，共享母任务 Context， │
│   不重复抢占用户套餐顶级 Running 槽位                  │
└───────────────────────────┬────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────┐
│ Phase 3: Agent & SmartVideo 纳管评估 (Follow-up)       │
│ · 评估 SmartVideo 混剪任务是否纳入统一调度池           │
│ · 评估长耗时 Agent RAG 是否需要持久化任务化            │
└────────────────────────────────────────────────────────┘
```

---

## 五、实施拆 PR 规划 (PR Breakdown)

按照工程交付规范，拆解为以下 5 个明确边界的 PR：

### PR1: Concurrency Domain Contract & Queue Backlog Limit
- **主要变更**：
  - 修改 `backend-go/internal/httpserver/generation_concurrency.go`：
    - 将 `enforcePostgresGenerationConcurrencyTx` 改造为“排队软上限校验 (`enforceGenerationQueueBacklogTx`)”。
    - 引入 `resolveUserEffectiveConcurrency`，增加 `subscription_expires_at` 校验。
    - 增加 `max_queued_per_user` 配置与错误定义。
  - 保持现有任务创建可正常进入 `QUEUED` 状态。
- **测试覆盖**：
  - 测试排队数量未达上限时成功落库。
  - 测试排队达到 30/50 时返回明确的 429 业务错误。
  - 测试订阅过期用户并发数回落为 1。

### PR2: Fair Scheduler & Outbox Dispatcher (Image + Video)
- **主要变更**：
  - 在 `backend-go/internal/httpserver/` 新增 `generation_scheduler.go`。
  - 实现基于 PostgreSQL `FOR UPDATE SKIP LOCKED` 的多用户公平调度循环。
  - 仅在用户 Running 未满时，将任务状态从 `QUEUED` 推进到 `DISPATCHING`，并写入 Outbox。
  - 在 `generation-worker` 或 API 后台启动调度引擎。
- **测试覆盖**：
  - User A 提交 10 个任务，套餐并发 3，验证严格只有 3 个进入 Running，7 个处于 Queued。
  - 模拟 1 个任务执行完成，验证 Scheduler 自动补位，Running 维持 3。
  - 多用户公平性测试：User A 提交 20 个，User B 提交 1 个，验证 User B 第一时间获得执行槽位，无队头饥饿。

### PR3: Crash Recovery & Dispatch Self-Healing
- **主要变更**：
  - 在调度器中增加僵死 `DISPATCHING` 任务自愈重试逻辑。
  - 强化 Worker crash 后的租约/超时联动释放。
- **测试覆盖**：
  - 模拟调度派发后断网，超时后任务自动回滚为 `QUEUED` 重新派发。
  - 模拟 Worker 宕机，Watchdog 超时将任务标为 `FAILED` 并释放 Running 槽位。

### PR4: Frontend Queued UX & Observability Metrics
- **主要变更**：
  - 增强 `apps/user-uni` 和 `admin-vue` 生成状态展示：展示“排队中（等待运行位）”。
  - 增加 Prometheus 指标：
    - `generation_running_tasks{plan}`
    - `generation_queued_tasks{plan}`
    - `scheduler_dispatch_total`
    - `scheduler_dispatch_latency_seconds`
- **测试覆盖**：
  - 前端渲染排队卡片验证。
  - 指标抓取与看板无高基数 `user_id` 标签验证。

### PR5: Canary Rollout & Production Verification
- **主要变更**：
  - 引入开关 `GENERATION_FAIR_SCHEDULER_ENABLED`（默认灰度 Canary 用户）。
  - 生产环境执行灰度放量与双用户并发冒烟验证。

---

## 六、生产验证矩阵 (Production Verification Criteria)

上线后必须满足以下准出验收标准：
1. **并发守门**：同一用户并发提交 10 个任务，观察数据库与 Worker 日志，确认 Running 状态的任务数严格等于其套餐并发数（如 Basic 恒等于 3）。
2. **自动补位**：运行中的任务成功或失败退出后，排队中的任务在 1 秒内被自动补位拉起为 Running。
3. **公平调度**：大账户高负载排队时，新注册/其他用户的生成任务在 1~2 秒内立即被调度执行。
4. **无异常 429**：用户在排队软上限内连续快速点击，前端无报错，任务全部平滑进入排队。
5. **点数精准**：排队时预冻结，完成时真实扣除，取消时实时全额解冻，无账目偏差。
