# PPT Generation V2 Phase 2 Implementation Report

- Date: 2026-08-15
- Worktree: `E:\code\work\ppt-v2`
- Branch: `codex/ppt-v2`
- Phase 1 baseline: `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- Verified Phase 2 HEAD: `019f6f39b0b7d22dbc5aa96dbdf2874fca4ecd53`
- Pull request: `#6` (`feat/ppt-v2: durable generation phase 2`)
- Final GitHub Actions run: `user-core` run `#68`, ID `31889960550`
- Phase 2 Status: READY

This report is the current Phase 2 implementation record. The Phase 1 report remains unchanged as a historical snapshot; its statement that durable generation did not yet exist describes Phase 1 only and is no longer the current PPT V2 system status.

## 1. Phase 2 Scope

Phase 2 wraps the existing Phase 1 V2 vertical slice in a PostgreSQL-backed durable generation boundary. It adds persisted GenerationJob, DeckJob, SlideJob, attempt and transition records; explicit state transitions; leases and fencing; retry/restart recovery; cancellation; real checkpoint progress; effectively-once private artifact publication; and atomic relation to the existing PPT task.

The implementation remains on the Phase 1 path:

`Existing PPT Task -> Legacy Adapter -> SlideIR -> LayoutResult -> PptxGenJS Renderer -> Private File -> Work Center Asset -> Existing Task Relation`

Phase 2 does not add Research, dynamic outlines, approval, multi-page generation, preview, conversational editing, a new PPT API family, a second document model, billing changes, or Connector-specific generation logic.

The implementation and CI fixes are recorded by these pushed commits:

- `5c6e7ebd0 feat(ppt-v2): add durable generation postgres state machine`
- `006bb1712 fix(ppt-v2): verify postgres rollback atomically`
- `105b5ed74 ci(ppt-v2): run postgres integration gate`
- `330a9c84a ci(ppt-v2): install renderer dependencies for backend tests`
- `019f6f39b ci(ppt-v2): provide officecli for pptx validation`

## 2. GenerationJob Architecture

`GenerationJob` is the durable orchestration aggregate. It stores owner scope, existing task identity, client request and idempotency identities, status and stage, attempt limits, scheduling time, lease owner/expiry, fencing token, progress, child job identity, the immutable task input snapshot, rendered deck identity and bytes, artifact identities, structured failure data, cancellation time, and lifecycle timestamps.

`GenerationJobStore` is the application port for create, read, claim, renew, checkpoint, fail, and cancel operations. `GenerationTaskRelationStore` adds the PostgreSQL-specific atomic task-relation operation without coupling the renderer to PostgreSQL.

Production durable execution calls `configuredPPTV2GenerationJobStore`, which deliberately requires `PostgresGenerationJobStore`. The memory store exists for deterministic domain and application tests; it is not a production durability fallback.

## 3. DeckJob / SlideJob

Every GenerationJob creates exactly one DeckJob and one ordered SlideJob per requested slide in the same creation transaction.

- DeckJob tracks the generated `deckId`, revision, lifecycle status, and timestamps.
- SlideJob tracks stable job-local identity, `slideIndex`, source slide identity, lifecycle status, and one completed work unit.
- Child statuses are `PENDING`, `RUNNING`, `SUCCEEDED`, `FAILED`, or `CANCELLED`.
- Loading the task marks SlideJobs running and records source slide IDs; the rendered checkpoint completes the SlideJobs and attaches deck/revision data to the DeckJob.

The domain model accepts a positive slide count, but the current Phase 1-based durable HTTP orchestration intentionally creates two SlideJobs because the renderer vertical slice is still fixed to two slides. Dynamic 6–12 page planning belongs to Phase 3.

## 4. PostgreSQL Persistence

Migration 109 persists the complete durable aggregate in five tables:

- `xz_ppt_v2_generation_jobs`
- `xz_ppt_v2_deck_jobs`
- `xz_ppt_v2_slide_jobs`
- `xz_ppt_v2_generation_attempts`
- `xz_ppt_v2_generation_transitions`

The job row is the recovery authority. It stores `input_snapshot` as JSONB, rendered PPTX bytes as `bytea`, the render SHA-256, file and asset IDs, lease/fencing state, progress, and structured error JSON. Attempts preserve worker/fence/error history; transitions preserve the ordered checkpoint history. PostgreSQL transactions and `FOR UPDATE` row locks serialize mutations of the aggregate.

## 5. State Machine

Job status and generation stage are separate persisted dimensions.

Job statuses:

`QUEUED -> RUNNING -> SUCCEEDED`

with controlled branches to `RETRY_WAIT`, `FAILED`, or `CANCELLED`.

Generation stages are strictly linear:

`CREATED -> TASK_LOADED -> RENDERED -> FILE_STORED -> ASSET_CREATED -> TASK_RELATED -> COMPLETED`

Each checkpoint validates the only legal next stage and the required payload for that stage. Invalid jumps fail with `ErrGenerationJobTransition`; incomplete checkpoint payloads fail with `ErrGenerationJobInvalid`. `COMPLETED` is the only stage that changes the job to `SUCCEEDED` and completes the active attempt.

## 6. Lease / Fencing

A worker must claim a job before mutating it. Claiming records the worker, creates a new attempt, increments `attempt_count` and `fencing_token`, and sets `lease_expires_at`. An active unexpired lease blocks another worker; a valid worker can explicitly renew it.

Every checkpoint, failure, and atomic relation validates all of the following against the locked PostgreSQL row:

- tenant and owner scope;
- job status is `RUNNING`;
- lease owner matches the worker;
- fencing token matches the current token;
- lease has not expired.

After an expired lease is reclaimed, the new attempt receives a higher fencing token. The old worker can no longer checkpoint, publish the Work Center asset, or relate the task. Cancellation and terminal states also invalidate subsequent lease mutations.

## 7. Idempotency

Creation is idempotent on `(tenant_id, user_id, idempotency_key)`. A replay returns the existing job only when the existing task, organization, and slide count match; a conflicting reuse returns `ErrGenerationJobIdempotencyConflict`. A second job for the same existing task is prevented by a separate unique index.

The client request ID is persisted for traceability, while the explicit idempotency key is the GenerationJob replay authority. The file and Work Center layers use the GenerationJob ID as their stable business identity. Replaying an already successful job returns its persisted file, asset, task relation, and render bytes without invoking the renderer again.

## 8. Retry / Restart Recovery

Retryable failures persist a structured error, finish the current attempt as `RETRY_WAIT`, clear the lease, and set `run_after`. A later claim starts a new attempt at the existing persisted stage. Non-retryable failures, or exhaustion of `max_attempts`, end in `FAILED`.

An expired running lease is recorded as `LEASE_EXPIRED`; if attempts remain, another worker can reclaim the job with a higher fence. The orchestrator switches on the persisted stage and executes only the remaining work. The restart test persists a rendered checkpoint, constructs a new store/process boundary, resumes from `RENDERED`, and verifies that the renderer is not called again.

The default maximum is three attempts and the database constraint permits 1–20 attempts.

## 9. Cancel

Cancel is an owner-scoped durable transaction. It sets status `CANCELLED`, records `cancel_requested_at` and `finished_at`, clears the lease, cancels unfinished DeckJob/SlideJob records, and closes the active attempt as cancelled. Repeated cancel calls against a terminal job are idempotent.

A cancelled job cannot be reclaimed or checkpointed. Fenced artifact creation also checks cancellation while holding the GenerationJob row lock, preventing a stale worker from making an asset visible after cancellation.

## 10. Progress Model

Progress is derived from five completed work units, not elapsed time:

| Durable checkpoint | Completed units | Progress |
| --- | ---: | ---: |
| `CREATED` | 0/5 | 0% |
| `TASK_LOADED` | 1/5 | 20% |
| `RENDERED` | 2/5 | 40% |
| `FILE_STORED` | 3/5 | 60% |
| `ASSET_CREATED` | 4/5 | 80% |
| `TASK_RELATED` | 5/5 | 100% |
| `COMPLETED` | 5/5 | 100% and `SUCCEEDED` |

The integer progress calculation is clamped to 0–100. The separate final status prevents 100% related work from being mistaken for terminal success before `COMPLETED` is committed.

## 11. Artifact Effectively-once

Rendered bytes and their SHA-256 are checkpointed before storage. The private file uses business type `ppt_v2_generation`, business ID equal to the GenerationJob ID, visibility `PRIVATE`, and tenant/owner scope. A partial retry first looks up the existing file; migration 109 also enforces one active file per `(tenant_id, user_id, business_id)` for this business type.

The Work Center adapter finds or creates the artifact by GenerationJob identity. PostgreSQL uses an advisory transaction lock plus a unique partial index on `metadata.pptV2GenerationJobId`. The fenced path commits Work Center asset creation/reuse and the `ASSET_CREATED` checkpoint in one transaction. These measures provide effectively-once visible file/asset results across retry, replay, and competing workers without vendoring or bypassing the existing storage boundary.

## 12. Task Relation

The existing PPT task stores only the V2 relation fields `v2DeckId`, `v2Revision`, and `pptxAssetId`. Replaying the same relation is accepted; a different relation cannot overwrite it.

For PostgreSQL, `RelateTaskArtifact` locks both authorities and commits the task JSON update, GenerationJob transition to `TASK_RELATED`, progress update, and transition-history row in one transaction. It verifies lease/fence, tenant, owner, organization, deck, revision, and asset identity before writing.

The rollback integration test forces the task update to fail and then verifies in a separate read-only transaction that no relation fields, `TASK_RELATED` stage/progress, or transition row leaked from the aborted transaction.

## 13. Tenant / Owner Isolation

GenerationJob reads and mutations are scoped by both `tenant_id` and `user_id`; an out-of-scope job is reported as not found. Task loading and relation additionally validate tenant and, when present, organization. Private file access and Work Center artifact lookup use the same tenant/owner scope.

The PostgreSQL isolation gate proves a cross-tenant read is denied. Application tests also prove cross-owner cancellation is denied and a wrong-tenant durable run fails before renderer, private storage, or Work Center side effects.

## 14. Migration Final Number

The final Phase 2 migration is:

`database/migrations/109-ppt-v2-durable-generation.sql`

It adds tenant/organization columns and an owner index to the existing PPT task table, creates the five durable tables, constrains all status/stage/progress/attempt values, adds recovery and owner indexes, and installs unique indexes for GenerationJob idempotency, one job per existing task, one Work Center asset per job, and one active private file per job.

The migration-number contract test loads migration 109 directly and verifies the required tables, fencing/lease/cancel/render/progress columns, and artifact uniqueness constraints.

## 15. PostgreSQL Integration Tests

The final real-database gate contains exactly these three tests:

- `TestPostgresGenerationJobLeaseFencingRestartCancelAndIsolation`: idempotent create, lease renewal and contention, restart reclaim, stale-fence rejection, checkpoint recovery, tenant isolation, cancellation, and terminal protection.
- `TestPostgresGenerationJobRetryAndAtomicTaskRelation`: retry transition, second attempt recovery, ordered checkpoints, atomic task relation, completion, 100% progress, and terminal rewrite rejection.
- `TestPostgresGenerationJobArtifactConstraintsAndTransactionRollback`: unique Work Center/file constraints and forced rollback of the task relation plus durable checkpoint.

In final CI all three emitted `=== RUN` followed by `--- PASS`; none emitted `SKIP`.

## 16. GitHub Actions Database Gate

PR #6 final run `31889960550` used the checked-in `user-core` workflow and a real `pgvector/pgvector:pg16` service. The `backend-go` job:

1. set up Node 22 and ran locked root `npm ci --ignore-scripts`, allowing the Go/HTTP renderer regression to resolve `pptxgenjs` 4.0.1 from the root workspace lockfile;
2. initialized PostgreSQL from `database/schema.sql` and every checked-in migration, including 109;
3. set `PPT_TEST_DATABASE_URL` to the live service;
4. enumerated and required all three PostgreSQL test names before executing `go test ./internal/app/ppt -run '^TestPostgresGenerationJob' -count=1 -v`;
5. ran the full backend command `go test ./...`.

The job and every named step completed successfully. Final check result: `user-core / backend-go: GREEN`.

## 17. Golden 1 Regression

Golden 1 remains the frozen Professional Business Deck regression for the existing two-slide V2 kernel. It still proves 100% semantic and geometry fixture parity, seven stable element IDs, strict SlideIR/LayoutResult authority separation, and byte-for-byte repeatable rendering.

Fresh focused local verification ran `npm.cmd run test:ppt-v2` with 13/13 passing. The final CI full Node regression also executed the same Golden 1 tests:

- `Golden 1 Professional Business Deck has 100% semantic and geometry parity`: PASS.
- `Golden 1 renderer is repeatable for the frozen fixtures`: PASS.

## 18. PPTX / OfficeCLI Integrity Gate

The renderer remains the Phase 1 PptxGenJS adapter and receives its locked dependency through the root workspace lockfile. The integrity test generates a real PPTX, runs `officecli validate`, requires exit code 0 and the successful validation message, rejects repair/validation warnings, and closes the document in cleanup.

The Windows `user-core` CI job installs OfficeCLI `1.0.144` from its versioned release, verifies SHA-256 `E780CC6A5385F84B4D54D71B0C179904ED534125EC33FE39B1A8711FA80E387E`, verifies the executable's reported version, adds it to `PATH`, and disables update checks during regression execution.

Final CI result:

- `OfficeCLI accepts the generated package without a repair warning`: PASS, not skipped.
- Node regression: 298 tests, 298 passed, 0 failed, 0 skipped.
- Final check result: `user-core / user-core: GREEN`.

## 19. Known Limitations

- The content kernel still generates exactly the Phase 1 two-slide Cover + Standard Content deck; GenerationJob durability does not make page planning dynamic.
- Supported visual content remains native text, bullets, shapes, and speaker notes; there is no Image, Chart, Table, Diagram, or richer layout work in Phase 2.
- There is no ResearchPack, citation provenance, independent Storyline, dynamic OutlinePlan, outline approval gate, preview workspace, conversational edit command, or revision/undo flow.
- The durable orchestrator is an internal application path; Phase 2 does not introduce a new public PPT API or UI.
- Cancellation prevents subsequent durable side effects but does not pre-empt a renderer call already executing inside a worker process; the next fenced mutation is rejected.
- Render bytes are retained in PostgreSQL to make this small vertical slice restart-safe. Large multi-page payload sizing and retention policy were not expanded in Phase 2.
- No billing/provider behavior, Connector boundary, enterprise governance, or protected user surface was changed.

## 20. Phase 2 Exit Gate

| Exit criterion | Final evidence | Result |
| --- | --- | --- |
| Durable GenerationJob and legal state machine | PostgreSQL aggregate, constraints, checkpoints, attempts, transitions | PASS |
| Lease, fencing, retry, restart, cancel | Real PostgreSQL gate and focused application tests | PASS |
| Idempotency and effectively-once artifact | Unique constraints, replay tests, fenced asset transaction | PASS |
| Atomic existing-task relation | Real PostgreSQL success and forced-rollback tests | PASS |
| Tenant/owner isolation | PostgreSQL and application isolation tests | PASS |
| Migration | Final migration 109 applied and contract-tested | PASS |
| PostgreSQL gates | Three named tests executed against the live CI database, no skip | PASS |
| Backend regression | `user-core / backend-go` | GREEN |
| Golden 1 | Focused 13/13 and final CI Golden tests | PASS |
| PPTX integrity | OfficeCLI 1.0.144 validation executed, not skipped | PASS |
| User/core protected regressions | 298/298 Node tests; `user-core / user-core` | GREEN |

All Phase 2 exit conditions are satisfied. This readiness decision authorizes the Phase 2 milestone record only; it does not merge `main` and does not begin Phase 3.

## Decision

**PHASE 2 STATUS: READY**
