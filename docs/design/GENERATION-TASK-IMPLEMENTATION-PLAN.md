# AI 生图任务生命周期实施计划

> 基于 `docs/design/GENERATION-TASK-LIFECYCLE-DESIGN.md` 制定。
>
> 本文只规划，不修改业务代码、不创建 PR、不创建 commit。

## 1. 现状与总体判断

原先拆分为 5 个 PR 的方向基本合理，但结合当前工作区实际情况，建议调整为 **PR0 + PR1～PR5 共 6 个阶段**：

- **PR0** 先处理已有前端临时修复的隔离、验证和 Git 纳入，避免把当前工作区的大量无关 dirty changes 一起提交。
- **PR1～PR4** 先完成后端数据、状态机、worker 恢复和 MQ/DLQ 闭环。
- **PR5** 最后调整前端轮询和交互，使前端消费稳定、规范化的任务状态，而不是继续猜测后端状态。

不建议把 PR0 与 PR5 合并：PR0 的目标是安全纳入现有修复，PR5 的目标是基于最终后端契约重做体验，二者风险和验收标准不同。

### 1.1 当前已确认的基础能力

- Outbox publishing claim 有 5 分钟 lease 和失败重试。
- RabbitMQ consumer 支持手动 ack、retry、DLQ。
- Inbox 通过 `(consumer_name,event_id)` 做幂等。
- `provider_executions` 已有 provider request、状态转换和 Get-only recovery。
- 任务完成/失败有行锁、幂等判断以及 freeze/capture/release 账单保护。

### 1.2 当前必须保护的行为

后续所有 PR 都不得破坏：

1. Outbox 事务一致性与重复发布保护。
2. RabbitMQ retry/DLQ 拓扑及手动 ack 语义。
3. Worker 幂等、重复消息不会重复生成。
4. freeze/capture/release 扣费流程。
5. provider unknown/processing 时禁止盲目二次 Create。
6. 已成功 provider 结果的 Get-only 本地恢复路径。
7. 现有图片、视频、PPT 及 Connector 链路的任务隔离。

## 2. 推荐实施顺序

```text
PR0  隔离并纳入前端临时修复
  ↓
PR1  数据库生命周期字段与兼容 migration
  ↓
PR2  统一任务状态机与转换约束
  ↓
PR3  worker lease / heartbeat / reaper
  ↓
PR4  MQ / DLQ 状态收敛与补偿
  ↓
PR5  前端轮询、超时、重试、取消体验
```

每个阶段都必须先在独立 worktree 完成，并从干净基线分支创建；禁止从当前含大量无关未提交改动的工作区直接提交。

## 3. PR0：安全纳入已有前端临时修复

### 目标

将当前 `admin-vue/src/App.vue` 中“轮询不因客户端次数上限静默丢失、超过 15 分钟显示等待服务商”的临时修复，单独提取到干净分支，完成最小验证后纳入 Git 流程。

### 修改范围

仅允许包含：

- 删除前端 `aiGenerationPollMaxAttempts` 上限及对应静默移除逻辑。
- 保留 capped backoff 轮询。
- 增加运行超过 15 分钟的明确提示。
- 对应最小测试或类型检查。

不应包含当前工作区其他认证、支付、PPT、Connector、部署、视频和文档 dirty changes。架构审计文档可作为独立 docs-only commit，或与计划文档一并提交，但不得混入业务修复 diff。

### 涉及文件

- `admin-vue/src/App.vue`
- 可选：`admin-vue/tests/` 下新增针对纯函数/展示状态的最小测试
- 可选：`docs/design/GENERATION-TASK-LIFECYCLE-DESIGN.md`

### 建议 Git/worktree 操作

1. 记录当前分支和 dirty tree，禁止 `git add .`。
2. 以目标基线创建独立 worktree 和 `fix/image-task-polling-visibility` 分支。
3. 只提取 `App.vue` 的目标 hunk，使用 `git diff --no-index` 或人工精确移植，禁止携带其他文件。
4. 执行 `npm run typecheck`，再执行相关前端测试。
5. 提交单一目的 commit，创建 PR0。
6. PR 合并后再开始 PR1；不要在原 dirty worktree 上继续叠加。

### 风险

- 当前 `App.vue` 是超大文件，误带无关改动的风险高。
- 无限轮询只能解决客户端任务消失，不能解决后端孤儿任务。
- 15 分钟提示不能把任务直接标记为失败，否则可能破坏 provider unknown 的扣费和恢复语义。

### 回滚方案

回滚 PR0 的单一 commit，恢复客户端原有轮询上限；不回滚后端或数据库。若该提示造成误导，可只回滚提示 hunk，保留任务不静默丢失行为。

### 验收标准

- `git diff` 仅包含目标前端文件和约定的测试/文档。
- admin-vue typecheck 与相关测试通过。
- 任务进入 `SUCCEEDED/FAILED/CANCELLED` 后轮询停止。
- 任务持续运行超过 15 分钟时不会从页面状态集合中消失。
- 无新增 API 调用、重复生成、重复扣费或改写已有任务状态。

## 4. PR1：数据库字段与 migration

### 目标

建立统一的任务生命周期数据基础，并保持旧字段兼容；让后续状态机和 reaper 可以可靠判断排队、执行、超时、provider 结果和错误。

### 修改范围

新增幂等 migration，不修改历史 migration：

`xz_generation_tasks` 建议增加：

- `task_status`
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

`generation_task_attempts` 建议增加：

- `error_code/error_class/error_message`
- `submitted_at/processing_at/failed_at`
- `created_at/updated_at`
- `(task_id,attempt)` 唯一约束

同时补充 `(task_status,lease_until)`、`(task_status,updated_at)`、`(user_id,created_at)` 等索引，统一 provider execution 摘要字段与历史尝试字段的职责。

### 涉及文件

- 新 migration 文件，例如 `database/migrations/xxx-generation-task-lifecycle.sql`
- `backend-go/internal/httpserver/postgres_projections.go`
- `backend-go/internal/httpserver/postgres_store.go`
- `backend-go/internal/httpserver/store.go`（若 JSON fallback 需要兼容）
- `backend-go/internal/providerexecution/`（字段映射）
- `backend-go/internal/messaging/model.go`（与 113 migration 对齐）
- 对应 PostgreSQL migration/schema 测试

### 风险

- 生产数据库存在旧版本和部分 migration 执行的情况。
- `status` 与 `task_status` 双写期间可能不一致。
- provider request ID 的唯一性必须结合 provider/channel，不能简单全局唯一。
- 修改 projection select/scan 容易影响作品中心、管理员任务和账单查询。

### 回滚方案

- migration 采用新增 nullable 字段和索引，应用先兼容读旧字段。
- 应用回滚到旧版本时新字段可保留，不删除已写入数据。
- 禁止在回滚中删除生产列；如索引有问题，只回滚应用读取逻辑并单独处理索引。

### 验收标准

- migration 可重复执行、可在空库和已有库执行。
- 老任务仍可查询、列表排序、完成、失败、取消和重试。
- provider execution 仍是多次 provider 尝试的权威历史。
- 不改变 freeze/capture/release 的金额和状态语义。
- PostgreSQL projection、JSON fallback、任务详情和账单测试通过。

## 5. PR2：任务状态机与转换约束

### 目标

将 `status`/`task_status` 的宽松字符串行为收敛为单一业务状态机，阻止非法跳转和终态回退。

### 修改范围

实现统一 transition service/repository，推荐状态：

```text
created → queued → running
running → succeeded
running → failed
running → cancel_requested → cancelled
queued → expired
running → expired
running → manual_review
manual_review → succeeded / failed / cancelled
```

要求：

- 终态不可回到运行态。
- 所有转换在事务内 `FOR UPDATE` 完成。
- 保留旧 `status` 兼容字段，但由规范化 `task_status` 作为业务来源。
- `expired` 与 `failed` 分离；账单释放由统一账单策略决定。
- provider `unknown/processing` 进入 recovery 或 `manual_review`，不能直接失败或盲重试。

### 涉及文件

- 新增任务状态常量/transition service
- `backend-go/internal/httpserver/api.go`
- `backend-go/internal/httpserver/postgres_store.go`
- `backend-go/internal/httpserver/store.go`
- 任务 cancel/retry/complete/fail 相关文件
- Go 状态机单元测试、PostgreSQL 并发测试

### 风险

- 既有客户端和 Connector 可能发送旧状态或依赖 `PROCESSING`。
- 视频、PPT、图片的状态完成路径不同，统一状态机不能抹平业务差异。
- 过严的数据库 CHECK 可能阻断历史兼容数据。
- 状态转换与账单事务边界错误会造成积分未释放或重复 capture。

### 回滚方案

- 保留旧字段读取和兼容映射。
- 先通过应用层 transition guard，数据库 CHECK 在历史数据清理和灰度验证后再启用。
- 回滚应用时继续读取旧 `status`，不删除新字段。

### 验收标准

- 非法跳转和终态回退被拒绝并可审计。
- 重复 complete/fail/cancel/retry 是幂等的。
- provider unknown 不会触发第二次 Create。
- 并发完成、取消、失败只产生一个最终账单结果。
- 图片、视频、PPT、Connector 相关回归测试通过。

## 6. PR3：Worker lease、heartbeat、reaper

### 目标

解决 worker 崩溃、goroutine 丢失、queued 长期等待和 provider 成功后本地未完成的问题，建立持续运行的恢复机制。

### 修改范围

新增 worker claim/renew/release 和独立 reaper：

1. worker claim 使用 `FOR UPDATE SKIP LOCKED`，写入 worker owner、lease、started_at。
2. 处理期间更新 `last_heartbeat_at` 并续租。
3. 完成/失败/取消/过期清理 lease 并写 finished_at。
4. 每 30～60 秒扫描到期 queued/running 任务。
5. 无 provider execution 且 lease 过期：按上限重新入队。
6. provider 有 request ID：执行 Get-only recovery。
7. provider processing/unknown：延长 recovery 或进入 manual review，不二次 Create。
8. 明确 provider failed 且可安全释放：任务失败并按账单规则 release。
9. reaper 本身需要 leader/claim 机制，支持多实例部署。

### 涉及文件

- 新增 generation task worker/reaper 包或运行时
- `backend-go/internal/httpserver/api.go`
- `backend-go/internal/httpserver/generation_worker.go`
- `backend-go/internal/httpserver/video_generation_worker.go`
- `backend-go/internal/providerexecution/`
- `backend-go/cmd/` 下 worker 启动入口
- config、metrics、health check
- worker crash/recovery/lease integration tests

### 风险

- reaper 与正常 worker 并发处理同一任务。
- lease 设置过短会造成重复 claim；设置过长会延迟恢复。
- provider unknown 的恢复不能被误判为安全失败。
- 当前 API 启动时 stale repair 与新 reaper 可能重复执行。

### 回滚方案

- 先以 feature flag 灰度启用 reaper，关闭后保留字段和旧 worker。
- reaper 只处理新字段完整且明确可恢复的任务。
- 遇到 provider active/unknown/succeeded 默认 fail-closed，宁可进入 manual review，不自动释放。

### 验收标准

- worker 崩溃后 lease 到期，任务可重新入队或进入 manual review。
- queued 任务不会永久等待。
- provider 已成功但本地 artifact/complete 失败时，可 Get-only 恢复。
- 同一 task 不会被两个 worker 同时有效执行。
- reaper 多实例、进程重启、DB 短暂不可用测试通过。
- 账单 freeze/capture/release 在恢复路径中保持幂等。

## 7. PR4：MQ/DLQ 补偿与任务收敛

### 目标

把消息 retry、dead letter、generation task 和运营补偿连接成完整闭环。

### 修改范围

- 统一消息 envelope 携带 `task_id/event_id/attempt/trace_id`。
- transient error 继续使用现有 retry/ack 语义。
- permanent error、重试耗尽进入 DLQ 后，幂等地将任务转为 `failed/expired/manual_review`。
- 增加 DLQ replay/manual recovery，仅重放已有 task，不新建任务。
- 补充 inbox attempt/error/last attempt 可观测字段。
- 增加 task、outbox、inbox、DLQ 的 metrics 和告警。
- 对齐 messaging Go model 与 113 migration 的真实列名。

### 涉及文件

- `backend-go/internal/messaging/consumer.go`
- `backend-go/internal/messaging/outbox.go`
- `backend-go/internal/messaging/inbox.go`
- `backend-go/internal/messaging/envelope.go`
- `backend-go/internal/messaging/topology.go`
- `backend-go/internal/httpserver/generation_worker.go`
- `backend-go/internal/httpserver/video_generation_worker.go`
- 新增 DLQ handler/replay service
- `database/migrations/113-async-messaging-foundation.sql` 的兼容后续 migration
- messaging/generation integration tests

### 风险

- 修改 ack/reject 顺序可能导致重复消费或消息丢失。
- DLQ 收敛过早可能在 provider 仍处理中时错误释放积分。
- replay 如果重新走 Create 而非已有 task recovery，会重复生成和扣费。
- 图片 canary、视频 canary、Connector 消息的 retry policy 不同。

### 回滚方案

- 不修改现有 retry exchange 和 DLQ 拓扑，先新增旁路 DLQ observer/handler。
- replay 默认只读/人工确认，关闭 feature flag 即停止自动补偿。
- 保留原有 consumer ack/reject 路径，出现异常时回退到旧 DLQ 行为。

### 验收标准

- transient error 的 retry 次数和 DLQ 行为不回归。
- permanent error 最终有 task 状态收敛和错误原因。
- DLQ replay 不创建新 generation task、不重复扣费。
- 重复 envelope、重复 replay、进程崩溃恢复均幂等。
- Outbox publish lease、Inbox 唯一约束和 RabbitMQ topology 测试通过。

## 8. PR5：前端体验与状态消费

### 目标

在后端状态机和 recovery 契约稳定后，统一 admin-vue 与 uni-app 的任务体验，减少无意义高频轮询，并明确超时、取消和人工介入。

### 修改范围

#### admin-vue

- 0-5 分钟约 5 秒轮询。
- 5-15 分钟约 20 秒轮询。
- 15 分钟以后 60 秒低频轮询。
- 显示 queued/running/cancel_requested/expired/manual_review 的中文状态。
- 增加取消、查看错误、重新同步和人工介入提示。
- 失败重试调用统一 retry API。
- 删除调用后端 API，不仅本地隐藏。
- SSE/WebSocket 可作为优化，但保留 GET 轮询兜底。

#### uni-app

- 任务列表页独立管理轮询生命周期。
- API Client 增加任务查询专用 timeout。
- 统一状态归一化，未知状态显示同步中，不直接当失败。
- 完善取消 loading、成功/失败提示和错误恢复。
- 统一失败原因和过期文案。

### 涉及文件

- `admin-vue/src/App.vue`
- `admin-vue/src/stores/admin.ts`
- `admin-vue/tests/`
- `apps/user-uni/src/stores/assets.ts`
- `apps/user-uni/src/features/assets/api.ts`
- `apps/user-uni/src/features/assets/types.ts`
- `apps/user-uni/src/components/assets/GenerationTaskItem.vue`
- `apps/user-uni/src/components/assets/GenerationTaskListPage.vue`
- `apps/user-uni/src/components/assets/AssetCenterPage.vue`
- `apps/user-uni/src/composables/useUserAssetsPage.ts`
- 小程序任务回归测试

### 风险

- 前端状态映射改变会影响已有任务展示。
- 降频可能让用户感觉完成延迟，需要终态 push 或立即刷新补偿。
- 取消操作必须与 provider cancel、账单 release 的最终结果一致。
- 大型 `App.vue` 继续存在误改其他页面的风险。

### 回滚方案

- 通过前端 feature flag 分批启用新轮询和状态映射。
- 保留旧 API fallback 和兼容状态映射。
- SSE/WebSocket 失败时自动退回 GET polling。
- 回滚只涉及前端，不回滚已完成的后端状态字段和数据。

### 验收标准

- 前端不再依赖“运行中无限高频轮询”维持任务可见性。
- 所有规范化状态都有稳定中文展示和对应操作。
- 取消、重试、删除均调用正确后端 API，并有 loading/错误反馈。
- 网络断开、页面隐藏、重新进入、浏览器刷新后任务仍能恢复显示。
- admin-vue typecheck/test/build 与 uni-app 相关测试通过。

## 9. 跨 PR 验证矩阵

| 场景 | PR0 | PR1 | PR2 | PR3 | PR4 | PR5 |
|---|---:|---:|---:|---:|---:|---:|
| 正常图片成功 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| provider 明确失败 |  | ✓ | ✓ | ✓ | ✓ | ✓ |
| provider unknown/processing |  | ✓ | ✓ | ✓ | ✓ | ✓ |
| worker 崩溃 |  |  |  | ✓ | ✓ | ✓ |
| queued 孤儿 |  |  |  | ✓ | ✓ | ✓ |
| MQ retry/DLQ |  |  |  |  | ✓ | ✓ |
| 重复消息/重复回调 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| freeze/capture/release | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 页面刷新/隐藏后恢复 | ✓ |  |  | ✓ | ✓ | ✓ |

## 10. 发布与分支纪律

1. 每个 PR 使用独立 worktree 和独立分支，分支从已合并的上一个阶段或明确基线创建。
2. 禁止在当前含大量 dirty changes 的 `codex/image-capability-quote` 工作区直接 commit。
3. 每个 PR 只提交一个清晰主题；禁止 `git add .`。
4. PR 合并前必须通过对应 Go/Node/前端测试和 protected surfaces 回归。
5. 生产部署必须走既有 `deploy.sh`，不得绕过部署流程。
6. 任何涉及 freeze/capture/release、provider unknown、Outbox/RabbitMQ 的改动，必须附带失败和重复执行测试。
7. 若某 PR 发现状态字段或账单边界需要重新设计，应暂停后续 PR，不在 PR5 用前端逻辑掩盖后端一致性问题。

## 11. 最终建议

- **PR0 必须先做**：它是当前可见问题的安全 Git 收口，但不等于生命周期问题已解决。
- **PR1 与 PR2 不宜合并**：先补数据承载，再落状态转换，便于迁移和回滚。
- **PR3 是核心修复**：没有持续 reaper，PR1/PR2 只能让问题更可观测，不能消除孤儿任务。
- **PR4 紧随 PR3**：确保 worker/MQ 异常最终能进入任务状态和运营补偿闭环。
- **PR5 最后实施**：前端应消费后端规范化状态，不应继续自行推断 provider 生命周期。

因此推荐后续按 **6 个 PR 阶段**落地：

```text
PR0 → PR1 → PR2 → PR3 → PR4 → PR5
```

每一步都以“可回滚、可观测、幂等、不重复生成、不重复扣费”为完成标准。
