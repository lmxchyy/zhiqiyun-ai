# WORKFLOW: AI 图片生成、计费与资产生命周期

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 当前 generation task 的创建、额度预留、进程内执行、资产落库、扣费、失败释放、取消和重试

## Overview

用户通过统一 generation-task API 提交图片请求。服务端先在数据库事务中解析价格规则、校验个人或企业余额、预留额度并创建 `PROCESSING/QUEUED/RESERVED` 任务，然后启动进程内 goroutine 调用图片提供商。成功时资产、任务完成、额度捕获、账单/佣金/审计在事务中提交；失败或启动修复时释放预留。取消当前先执行失败结算，再单独把顶层状态改为 `CANCELLED`，存在状态快照漂移风险；重试会创建新任务且缺少独立的重试幂等保护。

## Evidence Map

| Surface | Current source |
|---|---|
| Routes | `server.go`: generation list/detail/create/retry/cancel |
| Orchestration | `api.go`: `createGenerationTask`, `runGenerationTask` |
| Transactions/state | `postgres_store.go`: create/complete/fail/cancel generation task |
| Providers/storage | image provider implementations, artifact storage helpers |
| Repair | API startup stale-task repair |
| Schema | migrations `006`, `012`, `021`, `048` and later billing/audit additions |
| Clients | AI creation, generation status and asset-center pages/stores |

## Actors and prerequisites

Authenticated user; shared API client; Go API process; PostgreSQL; personal wallet or enterprise wallet; billing rules; selected image provider; configured object/local storage. Enterprise calls additionally require an active tenant service lifecycle and authorized context.

## Trigger

- Create: `POST /api/v1/generation-tasks`, with optional `Idempotency-Key`/client request id.
- Recovery/user action: retry and cancel routes; startup stale-task repair.

## Workflow Tree

### STEP 1: Validate, route and price the request

**Actor/Action**: API authenticates, normalizes type to text-to-image, validates prompt/options, resolves tenant/model/provider route and pricing rule.  
**Timeout**: HTTP request context; PostgreSQL calls use 5s store timeout.  
**Success**: prepared request and authoritative price -> STEP 2.  
**Failures**: invalid prompt/options (400), auth/context/tenant disabled (401/403), no route/provider/rule, configuration error.  
**Recovery**: correct request, switch context, enable tenant/provider/rule; no money is reserved before the admission transaction.  
**Observable state**: HTTP error and route/provider logs; no task/ledger row on validation failure.  
**Test cases**: personal/enterprise authorization; disabled tenant; absent provider; invalid parameters; server—not client—controls price.

### STEP 2: Atomically create task and reserve balance

**Actor/Action**: a Read Committed transaction takes an advisory lock for `(user, clientRequestID)`, checks existing idempotent task, verifies balance, creates task and billing lifecycle events, then reserves personal or enterprise funds.  
**Timeout**: 5s DB timeout; no queue admission timeout beyond request context.  
**Success**: task top-level=`PROCESSING`, task-status=`QUEUED`, billing=`RESERVED`, progress=5 -> STEP 3.  
**Failures**: insufficient balance, DB timeout/deadlock/constraint, invalid pricing, idempotency-key collision with incompatible payload, response loss after commit.  
**Recovery**: reuse the same client request id to obtain the committed task; top up balance; retry transaction on transient DB error. Without an idempotency key, a lost response can create duplicate billable tasks.  
**Observable state**: generation task, reserve ledger/lifecycle event, wallet reservation and audit record in one transaction; UI can poll returned id.  
**Test cases**: duplicate same key; concurrent same key; response loss; insufficient balance; personal vs enterprise reservation; DB rollback leaves no partial ledger.

### STEP 3: Start the in-process provider run

**Actor/Action**: after admission, API registers a cancel function and starts `runGenerationTask` in a goroutine; the provider prepares/submits the image job, with fallback routing where configured.  
**Timeout**: 3-minute task context; provider may perform up to three attempts for retryable 429/network/5xx failures.  
**Success**: provider returns image output -> STEP 4.  
**Failures**: process crash before/while goroutine starts, timeout/cancel, provider rate limit/5xx, invalid provider output, all fallback routes fail.  
**Recovery**: in-process retries/fallback; failure path -> STEP 5; startup stale repair marks abandoned old tasks failed and releases reservation, but does not resume provider work.  
**Observable state**: task remains `PROCESSING/QUEUED` with reserved billing; cancel registry and provider progress are memory-only; logs expose provider attempts; no durable queue heartbeat exists.  
**Test cases**: crash after commit before goroutine; provider retry/fallback; 3-minute timeout; cancellation propagation; restart repair; multi-replica cancellation ownership.

### STEP 4: Persist assets, capture charge and complete

**Actor/Action**: outputs are stored, then a transaction locks the task, creates asset records, marks task `SUCCEEDED`, task-status `SUCCEEDED`, billing `CAPTURED`, captures wallet funds, and writes billing/usage/commission/audit records.  
**Timeout**: storage follows request task context; completion DB transaction uses 5s timeout.  
**Success**: terminal task and retrievable assets; reserved funds are captured.  
**Failures**: storage write/URL failure, invalid image, transaction timeout, terminal-state race, capture/commission/audit error, response/process loss between storage and DB commit.  
**Recovery**: completion is terminal-guarded/idempotent where task state is already terminal; on failed completion the runner best-effort removes stored files and invokes failure settlement. Operators must inspect possible orphan blobs after ambiguous crashes.  
**Observable state**: task/asset rows, captured ledger, lifecycle events, usage/commission/audit; client sees progress/status/assets; storage cleanup errors appear only in logs.  
**Test cases**: successful capture exactly once; duplicate completion; storage succeeds/DB fails; DB succeeds/response lost; artifact checksum/URL; orphan cleanup.

### STEP 5: Fail and release reservation

**Actor/Action**: `FailGenerationTask` locks the task, skips terminal work, releases/refunds active reservation, sets task and task-status `FAILED`, billing `RELEASED`, and records audit/lifecycle.  
**Timeout**: 5s DB timeout; invoked after provider's 3-minute boundary or earlier hard failure.  
**Success**: terminal failed task and spendable reservation restored.  
**Failures**: DB timeout/rollback leaves task reserved; process exits before fail call; cleanup of already-stored blobs fails.  
**Recovery**: startup stale repair retries the fail/release transition; operator reconciliation detects lingering reservation; there is no general durable repair command surfaced to users.  
**Observable state**: `FAILED/RELEASED`, wallet ledger and failure reason/raw payload; stale repair logs; no dedicated alert metric.  
**Test cases**: failure releases once; repeated failure idempotency; crash then startup repair; fail transaction rollback; cleanup failure.

### STEP 6: Cancel an active task

**Actor/Action**: API authorizes ownership, store currently calls the failure settlement first, then separately changes top-level/raw status to `CANCELLED`; API also invokes the in-memory provider cancel function if this process owns it.  
**Timeout**: DB 5s plus immediate context cancellation; no provider cancellation acknowledgement deadline.  
**Success**: reservation is released and user sees cancelled.  
**Failures**: first transaction succeeds and second fails (`FAILED` remains), second write changes only top-level state while task-status remains `FAILED`, provider continues on another process, completion races with cancel.  
**Recovery**: refetch authoritative terminal state; repeated cancel is guarded by terminal state; operator reconciliation/atomic-transition fix is required for drift.  
**Observable state**: potentially `status=CANCELLED`, `task_status=FAILED`, `billing=RELEASED`; provider registry exists only in one process; audit/logs show separate operations.  
**Test cases**: cancel before provider start; cancel during provider call; cancel/completion race; failure between two DB writes; multi-replica cancel; repeated cancel.

### STEP 7: Retry a terminal task

**Actor/Action**: API rejects active source tasks, clones allowed generation inputs, records `retryOf`, and submits a new task through normal admission/execution.  
**Timeout**: same 5s admission and 3-minute execution limits as a new task.  
**Success**: a distinct task with a new reservation and source link -> STEP 2.  
**Failures**: source missing/unauthorized/active, unsupported raw payload, insufficient balance, repeated retry taps create multiple tasks because retry has no dedicated idempotency key.  
**Recovery**: wait for terminal source, correct context/top up; client should supply a stable retry request id after contract support is made explicit. Never mutate or re-charge the source task.  
**Observable state**: source remains immutable; new task includes retry linkage, separate ledger and assets.  
**Test cases**: retry failed/cancelled task; reject active task; repeated tap; insufficient balance; source/new ledger separation; successful retry after startup repair.

## State Transitions

| Entity | Intended/current transition |
|---|---|
| Generation task | `PROCESSING -> SUCCEEDED` or `FAILED`; cancel then rewrites top-level to `CANCELLED` |
| Task status | `QUEUED -> SUCCEEDED/FAILED`; current cancel may leave `FAILED` |
| Billing | `RESERVED -> CAPTURED` or `RELEASED`; legacy/refund paths may use `REFUNDED` |
| Asset | absent -> persisted and attached on successful completion; best-effort cleanup on failed commit |

## Handoffs and Cleanup

- DB admission commits before the process-local worker handoff; a crash in that gap creates a stale reserved task.
- Startup repair converts stale work to failed/released; it does not resume it.
- Provider output must be persisted before the DB asset/capture transaction; ambiguous crash cleanup needs reconciliation.
- Cancel function ownership is process-local and is removed when the goroutine exits.
- Asset deletion is separate from generation settlement and must preserve immutable billing/audit records.

## Reality Checker

- The API now exposes list/detail/retry/cancel; the older generation workflow spec is no longer authoritative.
- Execution is a goroutine, not a durable worker/queue; restart means fail-and-refund, not resume.
- Cancel is not one atomic state transition and can produce `CANCELLED/FAILED/RELEASED` snapshot drift.
- Retry lacks a clearly enforced idempotency contract and can duplicate charges on repeated taps.
- Provider/store cleanup is best effort; orphan object reconciliation is not surfaced.
- RabbitMQ is deployed but not used by this workflow.

## Test Cases

1. Personal and enterprise happy paths with exact reservation/capture amounts.
2. Stable idempotency key under sequential and concurrent submits.
3. Lost admission response and safe lookup/replay.
4. Insufficient funds leaves no task or reservation.
5. Provider retry/fallback and three-minute timeout.
6. Crash after reservation; startup repair releases exactly once.
7. Storage success plus DB failure cleans or reports orphan output.
8. Duplicate completion/failure callbacks are terminal-idempotent.
9. Atomicity check across task, wallet, billing, usage, commission and audit.
10. Cancel at every race boundary, including multi-replica ownership.
11. Retry double-tap and retry-of lineage.
12. Asset list/detail authorization and tenant isolation.

## Assumptions and Open Questions

- Should cancellation have a first-class atomic `task_status=CANCELLED` transition?
- What durable dispatch/lease and provider-job resume semantics are required for production?
- Which component owns periodic orphan-blob and lingering-reservation reconciliation?
- Should retry require a mandatory client request id derived from `(sourceTask, retryIntent)`?
