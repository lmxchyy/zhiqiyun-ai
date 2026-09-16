# Execution Generation & Completion Fencing — Architecture Audit → Design Decision

> Phase: Architecture Audit → **Design Decision ONLY**. No source/test/config changes, no migration execution, no commit/push/PR/merge/deploy.
> Worktree: `E:/code/work/先知AI/.worktrees/execution-generation-fencing`, branch `feat/execution-generation-fencing`, base `42eff306665cc80741d1bc2d0f41c4c2e6e2e701`.
> Read-first contract: `AGENTS.md`, `docs/process/DELIVERY-LOOP.md`, `docs/design/GENERATION-TASK-LIFECYCLE-DESIGN.md`, `docs/design/USER-CONCURRENCY-FAIR-SCHEDULER-DESIGN.md` (all read).
> Reference-only: `#142` readiness evidence worktree `E:/code/work/先知AI/.worktrees/user-concurrency-readiness` was listed for orientation only (same base `42eff30`); **no cherry-pick/merge/copy**, no modification.
> Decision: **`GO_IMPLEMENT`** (scoped fencing change; see §13). Not an `ARCHITECTURE_HOLD`: no large refactor is required.

---

## 1. Current model (how execution works today)

Lifecycle layers (base `42eff30`):

1. **Task admission → `QUEUED`.** `POST /generation-tasks` creates a row in `xz_generation_tasks` with billing reservation; PR1/PR2 (scheduler) parks excess demand in `QUEUED`.
2. **Fair Scheduler → `DISPATCHING` + Outbox.** `GenerationScheduler.dispatchUserTx` (per-user advisory lock + `FOR UPDATE SKIP LOCKED` claim of oldest `QUEUED` rows) flips `task_status='DISPATCHING'` and inserts an outbox event in the same tx. `RecoverStaleDispatches` flips `DISPATCHING→QUEUED` after 60 s by `updated_at` age.
3. **Worker/Inbox claim → `RUNNING` → provider call.** Image canary worker (`generation_worker.go`), video worker (`video_generation_worker.go`), PPT worker, and direct `runGenerationTask`/`runVideoGenerationTask` goroutines all converge on `generationTaskTerminal` (running-check) + `guardedImage`/`guardedVideo` provider-execution hook.
4. **Provider execution history.** `provider_executions(task_id, attempt)` with `prepared/submitting/submitted/processing/succeeded/failed/unknown`, row-locked `ClaimPrepared`/`Transition`/`SaveSucceededResult`, and `unknown`-blocks-resubmit policy.
5. **Local completion.** `CompleteGenerationTask` (artifact insert + `CAPTURE` + billing events) or `FailGenerationTask` / `FailGenerationTaskDurable` / `FailGenerationTaskUnknownGrace` (`RELEASE` + terminal mark), plus watchdog/stale-repair and manual recovery (`RESOLVE_CAPTURE`/`RESOLVE_RELEASE`, redrive).

The authoritative task state is the `xz_generation_tasks` row (`status` + `task_status` + `raw` JSONB projection); the authoritative provider-attempt history is `provider_executions`. There is **no execution-generation (fencing) counter** binding a worker claim to the completion write.

---

## 2. Existing fields (source evidence)

### 2.1 `xz_generation_tasks` — no lease/ownership/generation columns

Base table (`database/migrations/021-runtime-projections.sql:97-112`):

```sql
CREATE TABLE IF NOT EXISTS xz_generation_tasks (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  type TEXT,
  model TEXT,
  status TEXT,
  progress INT NOT NULL DEFAULT 0,
  point_cost BIGINT NOT NULL DEFAULT 0,
  prompt TEXT,
  params JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  error JSONB NOT NULL DEFAULT 'null'::jsonb,
  created_at TEXT,
  updated_at TEXT,
  worker_finished_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);
```

Only additive columns since (`044-enterprise-p0-safety.sql:231-236`, `048-billing-center-v1.sql:144-157`):

```sql
-- 044
ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS tenant_id TEXT,
  ADD COLUMN IF NOT EXISTS organization_id TEXT,
  ADD COLUMN IF NOT EXISTS billing_account_type TEXT NOT NULL DEFAULT 'PERSONAL',
  ADD COLUMN IF NOT EXISTS billing_account_id TEXT;
-- 048
ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS client_request_id TEXT,
  ADD COLUMN IF NOT EXISTS task_status TEXT NOT NULL DEFAULT 'CREATED',
  ADD COLUMN IF NOT EXISTS billing_status TEXT NOT NULL DEFAULT 'UNQUOTED',
  ADD COLUMN IF NOT EXISTS billing_rule_version_id TEXT,
  ADD COLUMN IF NOT EXISTS quoted_points NUMERIC(18,6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS reserved_points NUMERIC(18,6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS captured_points NUMERIC(18,6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS released_points NUMERIC(18,6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS refunded_points NUMERIC(18,6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS supplier_cost NUMERIC(18,6),
  ADD COLUMN IF NOT EXISTS estimated_margin NUMERIC(18,6),
  ADD COLUMN IF NOT EXISTS provider_channel TEXT;
```

Full-repo search for `worker_id|lease_until|heartbeat|execution_stage|execution_generation|fence` against `xz_generation_tasks` migrations returns **zero task-lease columns** (only unrelated `xz_labor_worker_profiles.provider_worker_id` in `052`, and `heartbeat_at` on smart-video tables in `075/076/106`). Go projection confirms: `backend-go/internal/httpserver/types.go:54` (`type generationTask struct`) has `WorkerFinishedAt` only — **no `WorkerID`, `LeaseUntil`, `LastHeartbeatAt`, `StartedAt`, `TimeoutAt`, `ExecutionGeneration`, `AttemptCount`, `ExecutionStage`**. `generationTaskForUpdate` (`postgres_store.go:6293`) selects `raw, client_request_id, task_status, billing_status, …` — no ownership columns because none exist.

### 2.2 What *does* exist (and what it actually protects)

| Field / mechanism | Location | Protects |
|---|---|---|
| `provider_executions.attempt INTEGER CHECK (attempt > 0)`, `UNIQUE(task_id, attempt)` | `114-provider-execution-safety.sql:3-28` | provider-attempt identity |
| `provider_executions.provider_operation_key DEFAULT ''`, fingerprint index, active-partial-unique `WHERE status NOT IN ('succeeded','failed')` | `114:32-43`, `providerexecution/store.go:33` (`generation:{task}:{attempt}`) | duplicate-submit suppression per attempt |
| `outbox_events` 5-min publishing-claim lease (`claimed_at/claim_owner/status/attempt_count`) | `113-async-messaging-foundation.sql`, `messaging/outbox.go`, `generation_recovery_api.go:redriveGenerationEvent` (resets `claimed_at=NULL,claim_owner=NULL`) | **outbox publishing only — not task execution** |
| Scheduler `Owner: "generation-fair-scheduler"` | `generation_scheduler.go:DefaultGenerationSchedulerOptions` | in-memory producer label only; **never persisted per task row** |
| `task_status DISPATCHING + updated_at` | `generation_scheduler.go:dispatchUserTx` / `RecoverStaleDispatches` | dispatch liveness by wall-clock age; **no owner** |
| `status/task_status` terminal strings | `api.go:354` (`PENDING/PROCESSING/RUNNING/QUEUED` = running), `postgres_store.go` terminal checks | idempotent no-op on re-entry; **not fencing** |

**Answer (1): current lease ownership fields = NONE on the task.** No `worker_id`, `lease_until`, `last_heartbeat_at`, `execution_generation`, or `execution_stage` column exists on `xz_generation_tasks`.

---

## 3. Existing guards (with exact SQL/conditions)

### 3.1 Worker claim preconditions

- Scheduler claim (`generation_scheduler.go:dispatchUserTx`): per-user `SELECT pg_advisory_xact_lock(hashtext('generation-concurrency:'||user))`, running-count predicate
  ```sql
  WHERE user_id=$1 AND upper(coalesce(nullif(task_status,''), status)) IN ('DISPATCHING','RUNNING','PROCESSING')
  ```
  then `SELECT … WHERE user_id=$1 AND …='QUEUED' ORDER BY created_at ASC LIMIT $2 FOR UPDATE SKIP LOCKED`, then `UPDATE xz_generation_tasks SET task_status='DISPATCHING', updated_at=$2 WHERE id=$1` + outbox insert, one tx.
- Direct/canary entry: `generationTaskTerminal` (`api.go:~1360-1380`) refuses start when `!isRunningGenerationTaskStatus(status)`; canary worker re-checks under `generationTaskForUpdate` (`generation_worker.go:78,167`).
- Provider-claim barrier: `providerexecution/store.go:claimPrepared` with `lockTask=true` (`ClaimPreparedForGenerationTask`, `CreatePreparedForGenerationTask`) holds `SELECT status FROM xz_generation_tasks WHERE id=$1 FOR UPDATE` and allows only
  ```go
  case "PENDING", "PROCESSING", "RUNNING", "QUEUED":
  ```
  else `generation task %s is terminal (%s)`. Execution claim itself: `SELECT id FROM provider_executions WHERE task_id=$1 AND status='prepared' … FOR UPDATE SKIP LOCKED LIMIT 1` → `UPDATE … SET status='submitting'`.

### 3.2 Complete / Fail preconditions

- `CompleteGenerationTask` (`postgres_store.go:1536`): `generationTaskForUpdate` (`SELECT … FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, `postgres_store.go:6293`) then
  ```go
  if task.Status == "SUCCEEDED" || task.Status == "FAILED" || task.Status == "CANCELLED" {
      return task, tx.Commit()   // silent idempotent no-op
  }
  ```
  followed by billing-marker validation, asset inserts, `CAPTURE`, `insertGenerationTask` upsert, commit.
- `FailGenerationTask` (`postgres_store.go:1817`) → `mutatePostgresGenerationFailureTx`: same terminal early-return; `RELEASE` only if reserved-and-active.
- `FailGenerationTaskDurable` (`postgres_store.go:1983`) → `failGenerationTaskDurable`: terminal early-return **plus** `providerExecutionBlocksLocalFailureTx` —
  ```sql
  select status from provider_executions where task_id=$1 order by attempt desc limit 1 for update
  ```
  blocks (`return … not eligible for durable failure`) when latest ∈ `{prepared, submitting, submitted, processing, unknown, succeeded}`.
- `FailGenerationTaskUnknownGrace` (`postgres_store.go:1990`) → `unknownExecutionEligibleForGraceTx` —
  ```sql
  select status, provider_request_id, result_metadata, coalesce(unknown_at, updated_at)
  from provider_executions where task_id=$1 order by attempt desc limit 1 for update
  ```
  eligible iff `status='unknown' AND provider_request_id IS NULL/'' AND result_metadata empty AND now-unknown_at >= grace`.
- Provider layer: `Transition`/`SaveSucceededResult` (`providerexecution/store.go:87-150`) lock the execution row `FOR UPDATE` and enforce `ValidateTransition` (`model.go:47-72`); safe retry creates `Attempt = latest.Attempt + 1` **only** for `Failed` with `ErrorClass ∈ {DefinitiveNotSubmitted, RetryableBeforeSubmit}` (`provider_execution_hooks.go:169-175` image, `:363-369` video; `ppt_generation_stages.go:129-131`); everything else returns `ErrUnknownResubmitBlocked` / `ErrProviderStillProcessing`.

**Answer (2): claim→Complete/Fail preconditions are (a) task row `FOR UPDATE` lock, (b) non-terminal `status` check, (c) billing-marker consistency, (d) for durable-fail paths, latest-execution eligibility. No worker-identity or generation predicate appears in any of them.**

---

## 4. Missing guards (answers 3–8)

### (3) Stale worker CAN still write after lease expiry — YES

`CompleteGenerationTask` / `Fail*` take `(taskID, req/message)` only — no `worker_id`/`lease`/`generation` argument, and `insertGenerationTask` (`postgres_store.go:6265`) is an unconditional upsert:

```sql
insert into xz_generation_tasks (…) values (…)
on conflict (id) do update set status=excluded.status, … , task_status=excluded.task_status, … , raw=excluded.raw
-- NO WHERE clause, NO generation/owner predicate
```

Consequences, each evidenced:

- **Final state:** `runGenerationTask` (`api.go:~1196-1285`) calls `a.store.CompleteGenerationTask(taskID, prepared)` (`api.go:1279`) after a 10–12 min ctx (`config/config.go` image timeout); on `DeadlineExceeded` the deferred `convergeGenerationTaskDeadline` fires `FailGenerationTaskDurable` (`api.go:~1391-1399`), but the timed-out goroutine's own `Complete` is still eligible to commit afterwards — whoever holds the row lock first wins, the loser gets a silent nil-error no-op. Same pattern in `runVideoGenerationTask` (`api.go:1542` Complete / `:1535` Durable-fail on persist error / `:1521` Fail on provider error), `connector_generation.go:78,161`, `connector_capabilities.go:285-291`, `ppt_api.go:1135`, `ppt_generation_worker.go:519`, `generation_recovery_api.go:346,372` (`RESOLVE_CAPTURE`→Complete, `RESOLVE_RELEASE`→Durable-fail).
- **Artifact:** asset rows are inserted inside the same `Complete` tx (`existingGenerationAssetID` + `insertAsset`), so a stale winner's artifacts become the durable result.
- **Provider result:** `SaveSucceededResult` locks **only** the execution row and never reads the task row — a timed-out attempt can durably record `succeeded + result_metadata` after the task was reaped/failed/requeued.
- **`execution_stage`:** no such column exists (nearest are `status`/`task_status`); both are overwritten by the unconditional upsert.
- **Billing trigger:** `CAPTURE` (Complete) / `RELEASE` (Fail paths) execute inside the same winning tx with idempotency keys (`generation:capture:`, `generation:release:`/`:durable-release:`), so the stale winner's billing direction is the one that settles.
- **Scheduler-level staleness:** `RecoverStaleDispatches` (`generation_scheduler.go:~305-330`) is a plain гражданство-free
  ```sql
  UPDATE xz_generation_tasks SET task_status='QUEUED', updated_at=$1
  WHERE upper(…)='DISPATCHING' AND updated_at < $2
  ```
  — the previous `DISPATCHING` claimant keeps running with no owner check and can still advance the task.

### (4) Retry/requeue execution-generation — DOES NOT EXIST

`provider_executions.attempt` + `UNIQUE(task_id, attempt)` + `provider_operation_key = generation:{task}:{attempt}` exist at the **execution** layer only. At the **task** layer there is no `execution_generation` counter, no requeue-epoch column, and no fencing token passed to `Complete`/`Fail`. Task-level retry (`asset_center_api.go:~400-490`) either spawns a **new child task id** (image/new-param path) or re-drives the **same task id with unchanged identity** (video child-allowed path `go a.runVideoGenerationTask(original.ID, …)`); redrive (`generation_recovery_api.go`) resets only the outbox row. A same-id requeue therefore reuses the exact lock-and-overwrite channel a stale worker still holds.

### (5) `attempt` field sufficient? — NO; new generation/fencing token REQUIRED

`attempt` is per-`provider_executions` row; **no completion path reads or compares it**: `CompleteGenerationTask` never takes an execution id/attempt; `SaveSucceededResult` takes an execution id but never checks the task row; the guarded hooks' `Attempt+1` logic gates *provider submission*, not *task settlement*. Old attempt-1 late-`succeeded` and new attempt-2 `succeeded` are two rows that both funnel into one unconditional task upsert. A task-level `execution_generation` (bumped on every (re)claim, requeue, redrive, scheduler dispatch) and threaded through claim→Complete/Fail as a conditional predicate is required.

### (6) Conditional-UPDATE coverage of completion paths — NONE (0 of N)

Every completion path uses `SELECT … FOR UPDATE` (serialization) + unconditional `insertGenerationTask … ON CONFLICT(id) DO UPDATE` (overwrite), with terminal-status early-return as the only guard. Paths audited: `CompleteGenerationTask`, `FailGenerationTask`, `failGenerationTaskDurable` (both Durable + UnknownGrace), `updateGenerationRecoveryState` (`MANUAL_REVIEW` mark), `resolveGenerationCapture/Release`, scheduler dispatch `UPDATE … WHERE id=$1`, `RecoverStaleDispatches` age-based `UPDATE`, provider `Transition`/`SaveSucceededResult` (execution-row only). **No `UPDATE … WHERE id=$1 AND execution_generation=$n` (or equivalent) exists anywhere.**

### (7) Provider late-success bypass of task fencing — YES

Because there is no task fencing, there is nothing to bypass *yet* — and the provider layer independently allows the stale write: `SaveSucceededResult` has no task-terminal/owner check, and `recoverSucceededGenerationTask` (`api.go:~1296-1360`) + watchdog (`api.go:285-352`: `execution.Status==Succeeded && len(ResultMetadata)>0` → recover-and-Complete) will settle a task from **any** succeeded execution row, including a stale attempt's. Corollaries: a stale `succeeded` row actively **blocks** legitimate durable failure (`providerExecutionBlocksLocalFailureTx` returns true for `Succeeded`), and the `Succeeded` fast-paths in `guardedImage`/`guardedVideo` return cached results without consulting task ownership.

### (8) Concurrent old/new writer arbitration — NONE

Beyond `FOR UPDATE` serialization + terminal-first-wins, there is no arbitration: no fencing token, no `WHERE generation` predicate, no owner check, no conflicting-write error (the provider-layer `ErrTransitionConflict` in `Service.Execute` has **no task-layer equivalent** — task losers receive `nil` error with the winner's row, indistinguishable from owned success, and all callers treat it as success).

---

## 5. Race matrix (required scenario + baseline)

### 5.1 Fencing race: A claim → A expiry → requeue → B claim → A late success → B success

| Step | Actor | Action (current code) | Guard evaluated | Required outcome under fencing |
|---|---|---|---|---|
| 1 | Worker A | claims task (scheduler `DISPATCHING` / `ClaimPreparedForGenerationTask`), generation `g` | row lock + non-terminal | A owns `g`; lease recorded |
| 2 | A lease expires | watchdog/scheduler observes `lease_until < now` (or `updated_at` age today) | wall-clock only (today) | eligibility also requires generation match |
| 3 | Reaper | requeues: `UPDATE … SET execution_generation=g+1, worker_id=NULL, lease_until=NULL WHERE id AND execution_generation=g` | **NEW — conditional bump; 0 rows = lost race, abort** | only one reaper wins; losers abort |
| 4 | Worker B | claims with `generation=g+1`, lease recorded | row lock + non-terminal + generation match | B sole owner of `g+1` |
| 5 | A late success | A calls `Complete(task, gen=g)` / `SaveSucceededResult(oldExec)` | **NEW — `WHERE execution_generation=g` matches 0 rows → fenced no-op (typed `ErrFencedStaleExecution`, no state/artifact/billing mutation); provider-result write must carry the task generation** | old execution mutates **nothing** authoritative: no Complete/Fail, no artifact rows, no `CAPTURE`/`RELEASE`, no `status/task_status` change |
| 6 | B success | B calls `Complete(task, gen=g+1)` | predicate matches → commits artifacts + `CAPTURE` exactly once | B sole owner; task terminal reflects B's result |

Net invariant: **at most one generation's completion tx can commit; stale generations fail closed with an explicit fenced error, never a silent overwrite.**

### 5.2 Baseline provider race (must be reconciled, NOT weakened)

`TestTEST_E_ClaimPreparedVsStaleFailureRace` (`backend-go/internal/providerexecution/batch_a_mandatory_test.go:~120-151`): `ClaimPrepared(task)` (prepared→submitting) vs `Transition(id, Failed/DefinitiveNotSubmitted)` (stale repair) run concurrently; **exactly 1 writer must commit** (`nils != 1 → Fatal`). Companion barrier `TestTEST_P0_CancelVsClaimBarrierNeverSubmitsAfterRelease` proves a terminal task can never acquire a new execution. Fencing **layers above** this: execution-row `FOR UPDATE` + `ValidateTransition` arbitration stays exactly as-is; task-generation predicates gate *task settlement*, never *execution transition*. Weakening the execution transition table (e.g. allowing prepared→failed to always win, or claim to skip the lock) is explicitly out of scope and would break the P0 barrier.

---

## 6. Schema change: **YES**

| Table | Change | Nullability / default | Why |
|---|---|---|---|
| `xz_generation_tasks` | `ADD execution_generation BIGINT NOT NULL DEFAULT 1` | backfill `1`; new claims bump | **mandatory fencing token** |
| `xz_generation_tasks` | `ADD worker_id TEXT`, `ADD lease_until TIMESTAMPTZ`, `ADD last_heartbeat_at TIMESTAMPTZ` | nullable; `NULL` = unowned | owner + expiry needed for reaper eligibility (§7) |
| `xz_generation_tasks` | index `(task_status, execution_generation)`, index `(lease_until) WHERE lease_until IS NOT NULL` | — | reaper/scheduler scans |
| `provider_executions` | none (attempt/fingerprint/operation-key already sufficient) | — | no change |
| `generation_task_attempts` | none (no such table in base; history lives in `provider_executions`) | — | no change |

Idempotent migration (`ADD COLUMN IF NOT EXISTS`, replayable like `114`).

---

## 7. Reaper / watchdog eligibility redesign: **YES**

Current eligibility is wall-clock-only and owner-blind:

- `repairStaleGenerationTasksWithContext` (`api.go:285-352`): `ListGenerationTasks` + `now - updated_at > maxAge` (15 min image / 20 min video) + `providerExecutionForRetry` exceptions (skip `prepared/submitting/submitted/processing/succeeded`, `unknown`-with-request-id, `failed`-non-retryable) → else `FailGenerationTaskDurable` / `FailGenerationTaskUnknownGrace` / `recoverSucceededGenerationTask`.
- `RecoverStaleDispatches` (`generation_scheduler.go:~305-330`): age-only `DISPATCHING→QUEUED`.

Redesigned eligibility (all three required):

1. **Lease-aware:** a task is reaper-eligible only when `lease_until IS NULL OR lease_until < now()` (plus existing `updated_at` age as a backstop during rollout).
2. **Generation-atomic requeue:** requeue/recover-dispatch performs the conditional bump (`SET execution_generation = execution_generation+1 … WHERE id=$1 AND execution_generation=$2`); the generation the reaper observed is the only one it may act on.
3. **Fenced settlement:** watchdog `FailDurable`/`UnknownGrace` and `recoverSucceededGenerationTask→Complete` must carry the observed generation; `Complete`/`Fail*` become `UPDATE … WHERE id AND execution_generation` (via the upsert path carrying the predicate — e.g. `insertGenerationTask` gains a generation argument and asserts `xmax`/row-count, or completion switches to a guarded `UPDATE` on the hot path with upsert retained for legacy backfill).

---

## 8. Proposed fencing model

- **Token:** `xz_generation_tasks.execution_generation` (monotonic per task, starts 1). Every ownership transfer — scheduler dispatch, worker claim, requeue/redrive, manual `RESOLVE_*`, video same-id child retry — bumps it atomically under the task row lock and returns the new value to the owner.
- **Threading:** the owner threads `(taskID, execution_generation)` through provider-hook context (`providerExecutionTaskParam`-style, internal only, never client-supplied) into `CompleteGenerationTask` / `Fail*` / `SaveSucceededResult`-adjacent task settlement.
- **Enforcement:** settlement tx re-locks the task row, compares stored vs presented generation; mismatch → `ROLLBACK` + typed `ErrFencedStaleExecution`, **no artifact insert, no billing entry, no status change**. Match → exactly-once settlement as today (idempotency keys retained).
- **Provider-result binding:** stale `SaveSucceededResult` rows remain queryable history, but `recoverSucceededGenerationTask` only completes when the execution's bound generation equals the task's current generation; otherwise the row is ignored (left for audit, never settled from).
- **Preserved invariants:** execution transition table, `unknown`-blocks-resubmit, fingerprint-mismatch refusal, safe-`Failed`-only `Attempt+1`, TEST_E exactly-one-writer, P0 cancel-vs-claim barrier, `client_request_id` idempotency, billing reserve/capture/release keying — all unchanged.

---

## 9. Backward compatibility

- New columns nullable except `execution_generation DEFAULT 1`; old code paths that ignore them keep working (they simply don't fence) until the completion call-sites are migrated one by one.
- Reads treat `NULL` generation as `1`; pre-migration rows fence correctly from the first bump onward.
- No API surface change: generation is internal (like `providerExecutionTaskParam`), never accepted from clients (Connector constraint: no tenant/execution identity from message bodies).
- Scheduler/watchdog run mixed-version safe: age-based backstop retained until all writers present generations, then lease+generation becomes authoritative.

---

## 10. Migration / backfill plan (no execution in this phase)

1. Land idempotent DDL migration (new file, `ADD COLUMN IF NOT EXISTS` + indexes + `UPDATE xz_generation_tasks SET execution_generation=1 WHERE execution_generation IS NULL` guarded for replay).
2. Backfill verify (read-only probes): `SELECT count(*) WHERE execution_generation IS NULL` → 0; spot-check terminal vs running rows keep status/task_status/billing_status.
3. Code PRs: (a) claim/requeue bump + threading; (b) fenced settlement predicates; (c) reaper eligibility; each behind existing config flags where behavior changes (`PROVIDER_EXECUTION_SAFETY_ENABLED`, scheduler flags).
4. Production verification per DELIVERY-LOOP: dry-run → read-only probe → human gate → smallest write → post-write verify → second idempotency run. No production actions in this audit phase.

## 11. Rollback plan

- Code rollback: revert fencing PRs; unfenced (current) semantics resume — safe because fencing only *narrows* who may settle.
- Schema rollback: `ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS …` for the added columns/indexes; no historical data loss (status/billing/artifact rows untouched; generation values discarded).
- Forward-fix preference: if a fenced writer misbehaves, bump generation via the reaper path rather than dropping the columns.

## 12. Test plan

- Unit: conditional-settlement predicate (stale gen → `ErrFencedStaleExecution`, 0 rows mutated; current gen → commits); generation bump atomicity (`WHERE gen` lost-update → 0 rows).
- Concurrency (extend crash-matrix style, cf. `batch_a_mandatory_test.go`, `crash_matrix_test.go`): A(claim g)→requeue(g+1)→B(claim)→A-late-Complete(fenced)→B-Complete(commits); assert artifacts/billing reflect B only; assert TEST_E exactly-one-writer still green unmodified.
- Integration: watchdog/reaper eligibility (lease-expired reaps; leased skips; unknown-grace still requires no-request-id + grace); scheduler stale-dispatch recovery bumps generation.
- Regression: full Go/Node suites for touched surfaces per `AGENTS.md` protected-surfaces; billing double-settle (double-Complete/double-Fail) tests retained.
- Load/chaos: kill -9 worker mid-`submitting`, expire lease, verify single settlement and no duplicate `CAPTURE`.

## 13. Decision: **GO_IMPLEMENT**

Root-cause summary (minimal): task settlement is guarded by row-lock serialization + terminal-first-wins only; with no ownership or generation token, any live-or-resurrected holder of a task id can commit final state, artifacts, and billing — the exact stale-writer hazard Issue #145 names. The fix is scoped (one counter + predicates + reaper eligibility), preserves the proven provider-execution arbitration (TEST_E/P0 barrier untouched), and needs no large refactor — hence `GO_IMPLEMENT`, not `ARCHITECTURE_HOLD`.

---

### Evidence index (base `42eff30`, key excerpts inline above)

`021-runtime-projections.sql:97-112`; `044:231-236`; `048:144-157`; `114-provider-execution-safety.sql` (full); `types.go:54`; `postgres_store.go:1536,1817,1983,1990,6265,6293`; `api.go:285-352,354,~1196-1285,~1296-1399,1521-1542`; `generation_scheduler.go:DispatchOnce/dispatchUserTx/RecoverStaleDispatches`; `generation_worker.go:78,167`; `provider_execution_hooks.go:169-175,363-369,505+`; `providerexecution/store.go:33,42-55,87-177`, `model.go:47-72`, `service.go:Execute/Recover`, `batch_a_mandatory_test.go:TEST_E/TEST_P0`; `asset_center_api.go:~400-490`; `generation_recovery_api.go:redrive/resolve/updateRecoveryState`.
