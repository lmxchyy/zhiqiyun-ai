# Task 2 — personal point lot core

## Scope

Implemented the server-side personal point lot core only. No auth handlers,
HTTP routes, frontend files, shared contracts, or production configuration were
changed.

## Behavior covered

- Missing accounts read as zero and a failed reserve never creates an account.
- Registration grants use the current plan grant amount; no 959 fallback is
  used.
- Gift lots snapshot the published policy and calculate calendar-month expiry
  with month-end day clamping in the policy time zone. Disabled policies produce
  permanent lots; paid/correction sources are permanent.
- FEFO lot allocation, reservation/capture/release conservation, lazy expiry,
  append-only movement records, and account/user ownership checks.
- Grant/reserve/capture/release idempotency and conflicting-key rejection.
- JSON sidecar adapter for local/test state and PostgreSQL adapter using one
  transaction with account → lots → reservation/allocation lock ordering.
- PostgreSQL allocations persist explicit `account_id` and `user_id`; wallet
  ledger transitions and lot movements are written in the same transaction.

## Validation

- RED evidence: before implementation, the new personal-point test package did
  not compile because the domain service, commands, store, and errors were not
  defined.
- Targeted GREEN: `go test ./internal/httpserver -run TestPersonalPoints -count=1`
  — PASS (6 tests).
- Affected package: `go test ./internal/httpserver -count=1` compiles, but the
  existing baseline has expected failures where old tests assume implicit 959
  points (asset cancellation/deletion and a legacy WeChat balance assertion),
  plus unrelated mini-program/commission failures and PostgreSQL tests blocked
  by the unavailable local test database at 127.0.0.1:55441.
- PostgreSQL integration tests for this adapter were not executed because no
  migration-backed PostgreSQL instance was available in the worktree
  environment.

## Concern for Task 3

`admin_api.go` still seeds the demo `user_000002` account with an explicit
959-point fixture. Task 2 intentionally leaves that file outside its ownership
boundary; Task 3 should replace the fixture with zero or route it through the
server-owned free-plan grant service before the full-suite acceptance run.

## Rollback

Revert this task's commit. The migration itself is unchanged; no production
database or traffic was touched.
