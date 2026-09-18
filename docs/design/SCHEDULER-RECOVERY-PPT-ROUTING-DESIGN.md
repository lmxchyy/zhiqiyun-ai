# Issue #147: Scheduler Recovery & PPT Routing Convergence Design

**Status:** Approved for Implementation (Delivery Loop Phase: Design)  
**Parent Issue:** #142 (ARCHITECTURE_HOLD)  
**Follow-up Issue:** #147  
**Prerequisites:** Issue #145 (Merged, `a12f0598e`), Issue #146 (Merged, `5450f8b1c`)  

---

## 1. Background & Problem Statement

In the audit and review of Issue #142, two specific vulnerabilities were identified:

1. **Recovery `LIMIT 100` Starvation**:
   In previous recovery implementations, querying stale `DISPATCHING` tasks with `ORDER BY updated_at LIMIT 100` *before* checking whether the task was actually recoverable (i.e. virgin dispatch without published/claimed outbox event or provider execution) caused permanent starvation. If 100 older ambiguous tasks existed in `DISPATCHING`, every recovery tick selected those exact 100 rows, skipped them, and never advanced to row 101+, starving genuinely recoverable tasks.
2. **PPT Eventless QUEUED**:
   In `postgres_store.go:createPendingGenerationTaskWithPPT`, when user running concurrency was saturated (`!admission.CanDispatch`), `eventType` was set to `""`. For Image and Video, tasks are picked up by the Fair Scheduler. But for PPT, PPT was excluded from the scheduler while the API still committed a `QUEUED` task and reserved user points. The client received a `200 OK` pending response, but no Outbox event was created and no scheduler ever dispatched the task, resulting in a permanent zombie task and frozen points until the 15-minute watchdog failed it.

---

## 2. Invariants to Preserve

- **#145 Fencing Invariant:**  
  `old execution can no longer mutate authoritative task state`  
  Execution generations must continue to bump monotonically on dispatch and recovery. Stale messages and deposed workers cannot Complete, Fail, renew leases, or mutate task/artifact records.
- **#146 Settlement Safety Invariant:**  
  `one authoritative execution generation can produce at most one authoritative financial settlement`  
  Financial operations (Capture / Release) remain strictly bounded to the authoritative generation under transactional row locks. Zero duplicate capture, zero erroneous release, and zero frozen point leaks.

---

## 3. Architecture & Technical Decisions

### 3.1 Decision 1: Elimination of Recovery Starvation

We eliminate the `LIMIT 100` starvation using a **Dual-Layer Architecture**:

1. **SQL Eligibility Predicate Prefilter**:
   The candidate selection query in `RecoverStaleDispatches` moves the revocability criteria directly into the SQL `WHERE` clause:
   ```sql
   SELECT t.id, t.execution_generation, o.event_id
   FROM xz_generation_tasks t
   LEFT JOIN outbox_events o ON o.aggregate_id = t.id AND o.aggregate_type = 'generation_task'
   WHERE upper(coalesce(nullif(t.task_status,''), t.status)) = 'DISPATCHING'
     AND ((t.lease_until IS NULL AND t.updated_at < $1) OR (t.lease_until IS NOT NULL AND t.lease_until < now()))
     AND (
       -- Virgin outbox event: never claimed, never published, 0 attempts
       (o.id IS NOT NULL 
        AND o.status = 'pending' 
        AND o.attempt_count = 0 
        AND o.claimed_at IS NULL 
        AND o.claim_owner IS NULL 
        AND o.published_at IS NULL 
        AND NOT EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id = t.id)
        AND NOT EXISTS (SELECT 1 FROM consumer_inbox ci WHERE ci.event_id = o.event_id)
        AND NOT EXISTS (SELECT 1 FROM outbox_events other WHERE other.aggregate_id = t.id AND other.event_id <> o.event_id)
       )
       OR
       -- Eventless task: no outbox event at all, no provider execution
       (o.id IS NULL 
        AND NOT EXISTS (SELECT 1 FROM outbox_events oe WHERE oe.aggregate_id = t.id)
        AND NOT EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id = t.id)
       )
     )
     AND ($2 = '' OR t.id > $2)
   ORDER BY t.id ASC
   LIMIT $3
   FOR UPDATE OF t SKIP LOCKED
   ```
   **Effect:** Ambiguous rows (where Outbox was claimed or published, or provider execution started) are filtered out immediately by PostgreSQL's query engine. They do not occupy slots in the `LIMIT` window, completely preventing starvation of downstream recoverable tasks.

2. **Keyset Cursor Traversal**:
   The scheduler maintains `lastRecoveredID string`. Each tick queries `WHERE t.id > $2 LIMIT 100`. If fewer than 100 rows return or the cursor reaches the end of the table, the cursor resets to `""`. This guarantees forward progress across large datasets (>100, >1000 tasks) without indefinite stalls.

3. **Atomic Outbox Revocation & Generation Bump**:
   Within the transaction:
   - If an outbox event exists, atomically `DELETE FROM outbox_events WHERE event_id = $event_id AND status = 'pending' AND attempt_count = 0 AND claimed_at IS NULL ...`.
   - If deletion succeeds (or task was eventless), atomically `UPDATE xz_generation_tasks SET task_status = 'QUEUED', execution_generation = execution_generation + 1, worker_id = NULL, lease_until = NULL, last_heartbeat_at = NULL, updated_at = $now WHERE id = $id AND execution_generation = $expected_gen`.
   - Bumping `execution_generation` ensures that even in an edge-case network delay, the previous dispatch attempt is permanently fenced.

4. **Multi-Scheduler Concurrency**:
   `FOR UPDATE OF t SKIP LOCKED` ensures multiple scheduler instances running in parallel concurrently process disjoint sets of tasks with zero duplicate recoveries.

---

### 3.2 Decision 2: PPT Routing & Fail-Fast Admission

1. **Fail-Fast Before Reservation/Freeze**:
   In `postgres_store.go:createPendingGenerationTaskWithPPT`:
   When `pptReq != nil || isPPTGenerationType(req.Type)`:
   If `!admission.CanDispatch`:
   ```go
   if pptReq != nil || isPPTGenerationType(req.Type) {
       if !admission.CanDispatch {
           return generationTask{}, errGenerationConcurrencyLimit
       }
   } else if !admission.CanDispatch {
       eventType = ""
   }
   ```
   **Effect:**
   - Evaluated *before* `reserveTx` or enterprise reservation.
   - Evaluated *before* inserting into `xz_generation_tasks` or `xz_ppt_tasks`.
   - Returns `errGenerationConcurrencyLimit`, which `ppt_api.go` translates to HTTP 429 Too Many Requests.
   - Zero points frozen, zero orphan tasks, zero eventless `QUEUED` records.

2. **Fair Scheduler Explicit Scope**:
   In `generation_scheduler.go`, the fair scheduler explicitly excludes PPT generation types:
   ```sql
   AND upper(coalesce(type, '')) NOT IN ('PPT_GENERATION', 'PPT')
   ```
   Ensuring Phase 1 Fair Scheduling remains strictly bounded to Image and Video.

---

## 4. Schema & Reaper Impact

- **Schema Change Required:** **NO**.  
  Existing schema and indexes from migration 113 (`outbox_events`, `consumer_inbox`) and migration 119 (`execution_generation`, `lease_until`) fully support all queries and locking semantics.
- **Reaper Redesign Required:** **NO**.  
  The long-term provider-level watchdog (`api.go:repairStaleGenerationTasksWithContext`) remains the authority for ambiguous/running executions, while `RecoverStaleDispatches` strictly handles virgin dispatch recovery.

---

## 5. Verification & Test Plan

1. **Recovery Starvation Test**:
   - Create 100 (and 200) ambiguous tasks in `DISPATCHING` (outbox status = `published` or `claimed` or `attempt_count > 0`).
   - Create 1 virgin dispatch task (outbox status = `pending`, `attempt_count = 0`, no claims).
   - Run `RecoverStaleDispatches`.
   - Assert: The virgin task is recovered on the very first tick; ambiguous tasks remain intact; recovered task generation is bumped.
2. **Recovery Keyset Traversal Test**:
   - Create 150 virgin dispatch tasks.
   - Run `RecoverStaleDispatches` with batch size 50.
   - Assert: Cursor advances across batches and successfully recovers all 150 tasks without infinite loops.
3. **PPT Fail-Fast Test**:
   - Fill user running concurrency to plan limit.
   - Call `CreatePendingGenerationTaskWithPPTCanaryOutbox`.
   - Assert: Fails fast with `errGenerationConcurrencyLimit`.
   - Assert: Zero points reserved (`xz_personal_point_reservations` count = 0), zero generation tasks (`xz_generation_tasks` count = 0), zero PPT tasks (`xz_ppt_tasks` count = 0).
4. **PPT Success When Capacity Available**:
   - User with available concurrency submits PPT canary task.
   - Assert: Succeeds, points reserved, task created with `QUEUED`, outbox event inserted with `generation.ppt.requested:<id>`.
5. **Regression Verification**:
   - Run all #145 fencing tests (`TestFencing_*`).
   - Run all #146 paid settlement crash tests (`TestPaidSettlement_*`).
   - Run existing PPT canary and durable artifact tests.
