# AI 生图任务生命周期设计审计

> 审计范围：`backend-go`、`admin-vue`、`apps/user-uni`、`database`。
>
> 审计性质：只读架构审计。本文档不代表已实施方案；本次仅新增设计文档，不修改业务代码、不创建 commit、不创建 PR。
>
> 重要背景：网页端此前存在“轮询达到最大次数后静默移除任务”的问题，当前工作区已有一个未提交的临时前端修复：取消客户端轮询上限，并在超过 15 分钟时提示仍在等待服务商响应。该改动不属于本次架构审计的实施内容。

## 1. 执行摘要

当前系统已经具备一定的异步安全基础：

- generation task 与 provider execution 分层；provider execution 有较严格的状态转换和 `provider_request_id`。
- PostgreSQL 完成/失败路径有行锁、幂等判断和账单释放保护。
- Outbox 有 claim lease、重试、DLQ；Inbox 有 `(consumer_name,event_id)` 幂等约束。
- provider 结果不明确时不会盲目二次提交，避免重复生成和重复扣费。

但整体还不是完整的企业级任务生命周期系统，主要缺口是：

1. `xz_generation_tasks` 是宽松字符串状态，没有数据库状态约束和统一状态机。
2. generation task 没有 worker heartbeat、lease、owner、timeout_at 等运行态字段。
3. stale repair 只在 API 初始化时执行一次，服务持续运行时无法周期性发现孤儿任务。
4. MQ 消息进入 DLQ 后，没有通用机制将对应 generation task 收敛为失败、可恢复或人工介入状态。
5. provider execution 已有恢复能力，但 generation task 与 provider execution 的恢复触发链路不完整。
6. 前端可以无限轮询运行中任务，却没有标准化“超时/待人工介入/取消中”状态和管理入口。
7. 任务表与 `provider_executions`、`generation_task_attempts` 的字段职责和命名存在漂移。

**结论：需要后续开发，建议拆分为 5 个 PR，先补任务可观测性与 reaper，再补状态机和前端体验。**

## 2. 当前架构分析

### 2.1 同步与异步入口

`POST /generation-tasks` 在 `backend-go/internal/httpserver/api.go:762-941` 完成鉴权、合规、报价、模型路由和任务创建。

- 视频路径创建 pending task 后启动 `runVideoGenerationTask`。
- 普通图片路径创建 pending task 后启动 `runGenerationTask`。
- 特定 canary 路径通过 Outbox 投递到 RabbitMQ。
- 任务创建与账单预留在持久化层完成，完成/失败时再结算或释放。

### 2.2 普通图片执行路径

`api.go:1137-1196`：

1. 创建带超时的 context。
2. 检查任务是否已进入终态。
3. 调用 `PrepareImageTask`，其中包含 provider execution hook。
4. 审计生成结果。
5. 持久化图片 artifact。
6. `CompleteGenerationTask` 完成任务和账单。

明确 provider 失败会调用 `failImageGenerationTask`；但 provider 超时或提交结果不明确时，会进入 `unknown` / `ErrUnknownResubmitBlocked`，本地任务保持活动态，以避免重复提交。

### 2.3 Provider execution

`backend-go/internal/providerexecution/model.go:10-72`、`store.go:62-175`：

- provider execution 有 `prepared/submitting/submitted/processing/succeeded/failed/unknown` 等状态。
- 状态转换受到代码约束，并使用行锁、`SKIP LOCKED` 防止并发 claim。
- provider 已有 request ID 时可以执行 Get-only recovery；无 request ID 的不明确提交禁止自动盲重试。

这是当前系统最可靠的生命周期子系统，但它没有自动保证 generation task 一定被重新唤醒。

### 2.4 消息链路

- Outbox：`backend-go/internal/messaging/outbox.go`，publishing claim 有 5 分钟 lease，失败可重试。
- 图片 canary consumer：手动 ack、并发 1、默认最大重试 3 次（`generation_worker.go:28-53`）。
- 视频 canary consumer：手动 ack、并发 1、最大重试 30 次（`video_generation_worker.go:20-45`）。
- 消息重试达到上限、格式错误或 permanent error 会进入 DLQ（`messaging/consumer.go:189-226`）。

Outbox/MQ 层具备事件恢复能力，但 DLQ 没有通用的 generation task 状态收敛和人工补偿闭环。

## 3. 当前任务状态与风险

### 3.1 当前状态定义

当前 generation task 实际存在两套字段：

- `status`：持久化为 `PROCESSING`、`SUCCEEDED`、`FAILED` 等。
- `task_status`：异步创建时通常为 `QUEUED`，作为业务语义状态。

数据库没有对状态值或状态转换建立 CHECK/transition 约束，Go 代码只在 `isRunningGenerationTaskStatus` 中将 `PENDING/PROCESSING/RUNNING/QUEUED` 视作活动态（`api.go:291-298`）。

期望状态与当前映射建议如下：

| 业务状态 | 当前支持情况 | 说明 |
|---|---|---|
| `created` | 部分存在 | 创建动作存在，但没有统一持久化状态 |
| `queued` | 部分存在 | 主要由 `task_status=QUEUED` 表达 |
| `running` | 部分存在 | `status` 可为 `RUNNING/PROCESSING`，无 worker lease |
| `succeeded` | 支持 | `SUCCEEDED/COMPLETED/DONE` 多种兼容值 |
| `failed` | 支持 | `FAILED/ERROR` 多种兼容值 |
| `cancel_requested` | 缺失 | 前端存在取消入口，但没有独立中间状态 |
| `cancelled` | 部分支持 | 代码可识别，但数据库状态约束和统一展示不足 |
| `expired` | 缺失 | stale repair 使用失败消息代替，没有独立过期态 |

### 3.2 P0/P1/P2 问题列表

#### P0：任务可能永久处于活动态

- `newAPI` 只在初始化时异步执行一次 `repairStaleGenerationTasks`（`api.go:218-287`）。
- 服务持续运行期间，worker goroutine 崩溃、消息丢失、provider recovery 未被再次触发时，任务可长期保持 `PROCESSING/QUEUED`。
- provider 已成功但本地审计、artifact 持久化或完成提交失败时，任务会刻意保持活动态；这符合防重复扣费原则，但当前缺少周期性 recovery trigger。

影响：用户看到“生成中”数十分钟甚至更久；预留积分可能长期占用；运营无法区分真实运行、等待 provider、worker 孤儿和本地落库失败。

#### P1：没有 generation task worker lease/heartbeat

`xz_generation_tasks` 初始化结构（`backend-go/internal/httpserver/postgres_projections.go:205-223`）没有：

- `worker_id/worker_instance`
- `lease_until`
- `last_heartbeat_at`
- `started_at/finished_at/timeout_at`
- 结构化 `error_code/error_class/error_message`

当前的 5 分钟 lease 只保护 `outbox_events` 的 publishing claim，不保护 generation task 执行本身。

#### P1：DLQ 与任务状态脱节

MQ 达到重试上限后进入 DLQ，但没有通用处理器把 generation task 标记为 `FAILED`、`EXPIRED` 或 `MANUAL_REVIEW`。因此“消息已不可继续消费”和“任务仍显示运行中”可能同时存在。

#### P1：状态机不是数据库级约束

`status` 与 `task_status` 并存，且接受任意字符串。不同端状态映射也不完全一致：

- admin-vue 运行态包含 `PENDING/RUNNING/QUEUED/PROCESSING`。
- uni-app 将未知状态归为失败，可能造成误报。
- `CANCELED/CANCELLED` 在网页通用状态标签中可能显示原始英文。

#### P2：前端任务操作不完整

- admin-vue 生图卡片没有取消入口。
- admin-vue 的删除主要是本地隐藏，不一定调用后端删除 API；刷新后可能重新出现。
- admin-vue 失败重试主要是复用参数重新 POST，而不是统一调用原任务 retry API。
- uni-app 任务列表页的轮询依赖资产页生命周期，单独进入任务列表时可能不主动轮询。
- uni-app 取消请求缺少完整 loading、成功提示和错误处理。

#### P2：任务表/尝试表/消息模型字段漂移

- `database/migrations/028-ai-capability-center.sql` 等迁移存在 `upstream_request_id/failure_reason`，但任务表没有统一的 `provider_request_id/error_code/error_class/error_message`。
- `generation_task_attempts` 有开始/结束时间，但缺结构化错误和阶段时间。
- messaging Go 的 `OutboxEvent` 字段命名与 113 迁移真实列名不完全一致，容易形成错误使用预期。

## 4. 推荐任务状态机

建议将“业务任务状态”和“provider execution 状态”明确分层：

```text
created
  -> queued
  -> running
       -> succeeded
       -> failed
       -> cancel_requested
            -> cancelled
       -> expired
       -> manual_review
```

建议转换规则：

| 当前状态 | 允许转换 | 触发条件 |
|---|---|---|
| `created` | `queued` | 任务、报价、账单预留和 outbox 事务成功 |
| `queued` | `running` | worker 成功 claim，写入 owner/lease/started_at |
| `queued` | `expired` | 超过 queue deadline 且无有效 claim |
| `running` | `succeeded` | artifact 与账单完成事务成功 |
| `running` | `failed` | provider 明确失败、参数失败或不可恢复本地失败 |
| `running` | `cancel_requested` | 用户或管理员请求取消 |
| `cancel_requested` | `cancelled` | provider 取消确认或确认未提交，账单释放完成 |
| `running` | `expired` | 超过最大执行时间，且没有 provider active/unknown 保护 |
| `running` | `manual_review` | provider 结果不明确、账单/结果一致性无法自动判断 |
| `manual_review` | `succeeded/failed/cancelled` | recovery worker 或人工补偿完成 |

原则：

1. 不允许终态回到活动态。
2. 所有转换通过单一 repository/service，并在数据库事务中加行锁。
3. `expired` 不等于 `failed`：expired 表示超过 SLA/最大时限，是否退款由统一账单策略决定。
4. provider execution 的 `unknown` 不应直接映射为 generation task failed；应进入 recovery 或 manual review。
5. `status` 保留兼容字段时，新增规范化 `task_status` 作为唯一业务状态来源，逐步收敛旧字段。

## 5. Timeout 策略建议

当前代码证据：

- 图片 provider timeout 默认 10 分钟，图片 task timeout 默认 12 分钟，并保证至少比 provider timeout 多 2 分钟（`config/config.go:208-215,386-395`）。
- 视频执行阈值为 20 分钟（`api.go:181-183,249-253`）。
- 普通任务 stale repair 阈值为 15 分钟，但只在 API 启动时执行一次。

不建议直接把建议值硬编码到本次修复；建议先按模型、provider、媒体类型配置化，并区分 SLA 与 hard timeout：

| 阶段 | 图片建议 | 视频建议 | 动作 |
|---|---:|---:|---|
| 正常 SLA | 结合历史 p50/p95，初始可按 5 分钟评估 | 结合模型时长，初始可按 10 分钟评估 | 前端展示“仍在处理”，不失败 |
| provider timeout | 当前约 10 分钟，需按 provider 校准 | 当前任务约 20 分钟，需按模型校准 | provider context 结束，进入 failed 或 unknown recovery |
| 最大执行时间 | 建议先评估 20 分钟上限 | 建议先评估 60 分钟上限 | 只有 provider 不活跃且可安全收敛时才 expired |
| queue timeout | 新增配置 | 新增配置 | 无 worker claim 时转 expired |
| worker lease | 建议 1-2 分钟、心跳续租 | 建议 1-2 分钟、心跳续租 | lease 过期由 reaper 重新入队或人工介入 |

这里的“最大执行时间”不能简单地强制释放：provider execution 为 `submitted/processing/unknown/succeeded` 时，必须先 Get-only recovery 或转 `manual_review`，不能为了清除 UI 卡片而盲目释放积分或重新提交。

## 6. Worker 恢复机制

### 6.1 建议新增 generation task reaper

新增独立、可水平扩展的 reaper（不建议继续依赖 `newAPI` 启动 goroutine）：

1. 每 30-60 秒扫描 `queued/running/cancel_requested`。
2. 使用 `FOR UPDATE SKIP LOCKED` claim 到期任务。
3. 判断 worker lease、queue deadline、task timeout 和 provider execution 状态。
4. provider 有 request ID：执行 Get-only recovery。
5. provider 仍 processing/unknown：延长 recovery deadline 或转 `manual_review`，不重复 Create。
6. provider 明确 failed 且可安全释放：失败任务并释放预留积分。
7. 没有 provider execution 且 lease 过期：重新入队，带 attempt 上限。
8. 超过 retry/repair 上限：转 `manual_review`，发告警和运营事件。

### 6.2 Worker claim 与 heartbeat

worker claim 时写入：

- `worker_id`
- `lease_until`
- `started_at`
- `last_heartbeat_at`
- `attempt_count`

worker 处理期间按固定间隔更新 heartbeat；完成、失败、取消和过期时写 `finished_at` 并清理 lease。心跳不能单独作为成功依据，只用于判断 worker 是否仍拥有执行权。

### 6.3 MQ 与 DLQ 收敛

每个消息应携带 `task_id`、`event_id`、`attempt`、`trace_id`。DLQ 消费或管理补偿操作必须：

- 幂等读取 task 与 inbox；
- 将 task 收敛到 `failed/expired/manual_review` 之一；
- 记录 `error_code/error_message`；
- 按统一账单服务释放或保留预留；
- 允许只重放已有 task，不重新创建任务和重复扣费。

## 7. 数据库设计建议

建议新增幂等迁移，不修改历史迁移。

### 7.1 `xz_generation_tasks`

新增 nullable 字段：

- `task_status`：规范化业务状态
- `worker_id`
- `lease_until`
- `queue_timeout_at`
- `timeout_at`
- `started_at`
- `submitted_at`
- `last_heartbeat_at`
- `finished_at`
- `provider_request_id`
- `provider_execution_id`
- `error_code`
- `error_class`
- `error_message`
- `attempt_count`
- `last_checked_at`
- `manual_review_reason`

建议索引：

- `(task_status, lease_until)`
- `(task_status, updated_at)`
- `(provider_request_id)`，必要时唯一约束需结合 provider/channel
- `(user_id, created_at desc)`

`provider_executions` 继续作为多次 provider 尝试的权威历史；任务表中的 provider 字段只作当前执行摘要，不能覆盖历史尝试。

### 7.2 `generation_task_attempts`

新增：

- `error_code`
- `error_class`
- `error_message`
- `submitted_at`
- `processing_at`
- `failed_at`
- `created_at`
- `updated_at`

增加 `(task_id, attempt)` 唯一约束，保证重试记录可审计且幂等。

### 7.3 Inbox/Outbox

Outbox 当前 claim/retry 能力基本完整，但建议统一 Go model 与 113 迁移的真实字段命名，并补齐 claim/retry 语义。Inbox 可增加：

- `attempt_count`
- `last_attempt_at`
- `updated_at`
- `last_error_code`

这样 DLQ 和人工补偿可以被观测，而不仅依赖日志。

## 8. 前端交互策略

### 8.1 admin-vue

当前图片轮询为：2s、3s、5s、8s、13s、20s，随后固定 20s；任务进入非运行状态停止。当前临时修复已取消客户端最大次数，并在 15 分钟后显示服务商响应较慢。

企业级建议：

- 0-5 分钟：5 秒轮询。
- 5-15 分钟：20 秒轮询。
- 15 分钟以后：60 秒低频轮询，并显示“仍在恢复/等待服务商”。
- 任务进入 `expired/manual_review/cancel_requested` 时停止高频轮询并展示明确动作。
- 增加取消、查看错误详情、重新同步、人工介入状态。
- 删除应调用后端 API；失败重试统一调用 task retry API，保持原任务幂等链。
- 后续可用 SSE/WebSocket 推送终态，但必须保留 GET 轮询作为断线兜底。

### 8.2 uni-app

当前资产页轮询在 4s、8s、16s 后封顶 30s；任务列表页依赖资产页生命周期。建议：

- 任务列表页独立启动/停止轮询。
- 统一使用 API Client，任务查询设置专用 timeout。
- 展示 queued/running/cancel_requested/expired/manual_review 的明确文案。
- 取消操作增加 loading、成功提示和失败回滚。
- 未知状态不要一律映射为失败，应显示“状态同步中”并触发重新查询。
- 失败原因为空时提供稳定的错误码和用户可理解的兜底文案。

## 9. 推荐实施 PR 拆分

### PR1：任务可观测性与字段迁移

- 增加 task lifecycle 字段、attempt 字段、索引和兼容读取。
- 对齐 `provider_executions`、`generation_task_attempts`、Outbox Go model。
- 增加状态/字段迁移测试。

### PR2：统一任务状态机与事务转换

- 引入 task status enum/常量和单一 transition service。
- 加数据库 CHECK 或 transition guard。
- 统一 cancel、retry、expire、manual review、账单释放语义。

### PR3：Worker lease、heartbeat 与 reaper

- worker claim/续租/释放。
- 周期性 reaper，覆盖 queued orphan、running lease expired、provider recovery。
- 服务重启、worker 崩溃、provider unknown/succeeded 的集成测试。

### PR4：MQ/DLQ 收敛与补偿

- DLQ 任务状态收敛。
- 消息重放和人工补偿接口。
- 只重放已有 task，禁止重复创建/重复扣费。
- Outbox/Inbox 可观测字段和告警。

### PR5：前端任务体验

- admin-vue 与 uni-app 统一状态映射。
- 分阶段轮询、超时/过期/manual review 展示。
- 取消、重试、删除与后端 API 对齐。
- 可选 SSE/WebSocket，保留 GET fallback。

## 10. 验收标准

1. worker 崩溃后，任务在 lease 到期后可自动恢复或进入 manual review，不永久 running。
2. worker 未领取的 queued 任务超过 queue timeout 后可收敛，不永久 queued。
3. provider 明确失败时，generation task、artifact 和账单状态最终一致。
4. provider unknown/processing 时，不发生第二次盲目 Create，不重复扣费。
5. provider 已成功但本地完成失败时，reaper 能 Get-only 恢复并完成 artifact/账单。
6. MQ 进入 DLQ 后，任务有明确终态或 manual review 状态，且可审计、可补偿。
7. 所有终态转换幂等；重复消息、重复回调、重复重试不会重复生成或扣费。
8. admin-vue 和 uni-app 对状态、错误、超时和取消展示一致。
9. 生产数据库迁移可重复执行，旧任务可兼容读取。

## 11. 最终结论

### 当前发现的问题列表

- **P0**：generation task 缺少运行态 reaper，存在永久 queued/running 和 provider 成功但本地任务不收敛的风险。
- **P1**：缺少 worker heartbeat/lease、统一状态转换约束、DLQ 到任务状态的收敛链路。
- **P1**：任务表缺少生命周期时间、结构化错误和 provider 当前执行摘要字段。
- **P2**：前端超时、取消、删除、重试和独立任务页轮询体验不完整。
- **P2**：状态字段、迁移定义和 messaging Go model 存在命名/职责漂移。

### 推荐架构方案

采用“generation task 业务状态机 + provider execution 尝试历史 + worker lease/heartbeat + 周期性 reaper + Outbox/Inbox 幂等补偿”的五层结构。provider execution 的 unknown 必须进入 Get-only recovery 或人工介入，不得通过前端超时简单标失败或重新扣费。

### 是否需要后续开发

需要。当前临时前端修复只能避免任务从 UI 轮询集合中静默消失，不能解决后端孤儿任务、provider recovery 触发和账单最终一致性问题。

### 建议拆分 PR 数量

建议 5 个 PR：字段迁移、状态机、worker lease/reaper、MQ/DLQ 补偿、前端体验。建议按该顺序实施，并要求每个 PR 独立具备回滚路径和回归测试。
