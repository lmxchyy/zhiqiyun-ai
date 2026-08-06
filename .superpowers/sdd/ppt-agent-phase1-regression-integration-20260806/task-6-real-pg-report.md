# Task 6A — Real PostgreSQL Gate Remediation Report

## Verdict

**PASS for the approved Task 6A code and focused verification scope. Repository release remains NO-GO.**

The processing `confirm-outline` replay is now strictly read-only, the real HTTP legacy adapter uses only the exact permitted `legacy-outline` scope, and the real HTTP fixture seeds FEFO personal points through the existing PostgreSQL point-lot service. The required real PostgreSQL PPT and HTTP gates passed against the dedicated local database.

No public API, migration, frontend, Connector, Provider, billing-store production path, deployment, production/shared database, or external service was changed. No push, merge, deploy, production migration, or traffic action occurred.

## Scope and integrity

- Worktree: `E:\code\work\先知AI-ppt-agent-phase1-integration-20260806`
- Required parent before commit: `22a923f4a1c898753125350fb8d128b4ada33cf4`
- Allowed implementation files:
  - `backend-go/internal/app/ppt/postgres.go`
  - `backend-go/internal/app/ppt/postgres_test.go`
  - `backend-go/internal/app/ppt/postgres_integration_test.go`
  - `backend-go/internal/httpserver/ppt_postgres_http_test.go` (fixture/cleanup only)
- Evidence file: this report.
- `git diff --check`: PASS before staging.
- No unrelated or concurrent worktree changes were observed.

## Behavior and call-chain changes

1. An existing same-key/same-hash processing generation claim now returns through `errPPTPostgresReadOnly`. It preserves `Replay=true` and the original operation token without updating the idempotency timestamp, task timestamp, raw JSON, `xmin`, or `ctid`.
2. Stale-cancel transfer remains based on the original processing-claim age; duplicate requests cannot extend the recovery window.
3. `requireOperationStage` accepts only the exact `legacy-outline` scope at `DRAFT` and `OUTLINE_READY`. Later stages and near-miss scope strings remain rejected.
4. The real HTTP test fixture grants the owner a permanent recharge lot via `NewPostgresPersonalPointStore(db).grant`. Cleanup first removes the fixture's generation/PPT/billing rows so the asynchronous capture transaction has crossed its database boundary, then removes movements, allocations, reservations, lots, and finally the aggregate point account.
5. The visual persistence test now validates the canonical stored image representation, preserves visual task/model metadata, and expects no history entry for the first successful visual asset.

Public API changes: none. Database schema or migration changes: none. Production billing fallback: none.

## Exact RED/GREEN evidence

### Processing generation replay

Command:

```powershell
$env:PPT_TEST_DATABASE_URL = '<dedicated local PG16 DSN>'
go test ./internal/app/ppt -run '^TestPostgresPPTNoopReplayBranchesAreReadOnly/processing_generation_replay$' -count=1 -v
```

- RED before the production fix: failed because both the idempotency record and returned task `UpdatedAt` were refreshed.
- GREEN after returning the existing read-only sentinel: PASS, `ok .../internal/app/ppt 2.458s`.

### Exact legacy-outline stage scope

Command:

```powershell
go test ./internal/app/ppt -run '^(TestGenerationClaimProcessingReplayIsReadOnlyAndPreservesOriginalStaleAge|TestRequireOperationStageAllowsOnlyExactLegacyOutlineScopeAtOutlineStages)$' -count=1 -v
```

- RED: the processing replay contract passed, while exact `legacy-outline` was rejected at `DRAFT` and `OUTLINE_READY`.
- GREEN after the exact switch case: both tests passed, `ok .../internal/app/ppt 2.507s`.

### Real HTTP assembly progression

Command:

```powershell
$env:PPT_TEST_DATABASE_URL = '<dedicated local PG16 DSN>'
go test ./internal/httpserver -run '^TestPostgresPPTAPIAssemblyDoesNotImportOrMirrorLegacyTaskFile$' -count=1 -v
```

- Before exact scope support: failed with HTTP 409 `PPT_INVALID_STAGE`.
- After exact scope support: advanced and exposed an HTTP 402 `PPT_BILLING_RESERVATION_FAILED` fixture mismatch.
- After seeding points through the real personal-point lot service: PASS, `ok .../internal/httpserver 6.760s`.
- A final repeated run exposed a cleanup-only deadlock with the asynchronous point-capture transaction (`SQLSTATE 40P01`). Reordering fixture cleanup across the generation transaction boundary resolved it; the assembly test then passed three consecutive executions in 8.104s and passed again within the focused PPT suite.

## Dedicated PostgreSQL evidence

Only local database `ppt_agent_phase1_service_codex_20260807` in the existing PG16 container was used. It was confirmed absent, created for this task, initialized from the local schema, and migrated through migration 106. No other database, role, container, volume, shared environment, or production system was altered. The database was intentionally retained.

Normal service tests used `PPT_TEST_DATABASE_URL`. The readiness test used the same dedicated database through `PPT_READONLY_DATABASE_URL` with connection option `default_transaction_read_only=on`; the test's `SHOW default_transaction_read_only` assertion passed. `PPT_MISSING_SCHEMA_DATABASE_URL` remained unset, so the missing-schema test skipped as permitted by the brief.

## Verification results

| Command | Result |
| --- | --- |
| `go test ./internal/app/ppt -run '^TestPostgres' -count=1 -v` with normal and read-only dedicated DSNs | PASS, 3.267s; missing-schema case SKIP because its separate DSN was unset |
| `go test ./internal/app/ppt/... -count=1` | PASS: `ppt` 3.083s; `skills` 1.000s |
| Real HTTP assembly test above | PASS three consecutive times, 8.104s aggregate; passed again in focused PPT suite |
| `go test ./internal/httpserver -run 'PPT' -count=1 -v` with the dedicated service DSN | PASS, 19.730s; unrelated personal-point PostgreSQL test SKIP because its separate DSN was unset |
| Task 0 exact protected Feishu/HTTP/video/storage aggregate | PASS: Feishu 2.790s; HTTP 8.156s; provider/video 1.555s; storage 1.540s |
| `npm.cmd --prefix admin-vue run test` | PASS, 2 files / 26 tests |
| `npm.cmd --prefix admin-vue run typecheck` | PASS |
| `npm.cmd run verify:api-boundaries` | PASS |
| `npm.cmd run build:admin` | PASS, 2019 modules; existing Rollup PURE and chunk-size warnings only |
| `go vet ./internal/app/ppt/...` | PASS |
| `go vet ./internal/httpserver` | FAIL at unchanged `internal/httpserver/api_channel_probe.go:315`: self-assignment of `raw`; outside Task 6A and not modified |
| `gofmt` on all four modified Go files | PASS |
| `git diff --check` | PASS |

## Protected surfaces, residual risks, and release gate

- Task 0 protected backend aggregate: confirmed passing.
- Admin tests, typecheck, build, and API-client boundary: confirmed passing.
- User-uni / mini-program source: unchanged by Task 6A; device/browser acceptance was not run.
- Real Provider, real private object storage, desktop PowerPoint-open acceptance, and production runtime: not run.
- The previously declared Connector stored-image PPTX export blocker remains outside this task.
- Full `internal/httpserver` vet is not green because of the unchanged self-assignment diagnostic above.
- Missing-schema real-PG behavior was not run because no separate dedicated missing-schema DSN was configured.

Therefore Task 6A is code-validated within its approved scope, but this report does not authorize or recommend merge, deployment, production migration, or traffic change.

## Rollback

Revert the single Task 6A commit to restore the prior behavior. There is no migration or production-data rollback for this task. The dedicated local verification database may be retained for later local evidence runs.
