# Task 6B — Trigger-Preserving PostgreSQL Test Cleanup Report

## Verdict

**PASS for the approved Task 6B test-only scope.**

The real PostgreSQL HTTP fixture no longer disables database triggers or deletes append-only personal-point audit records. The dedicated `ppt_agent_phase1_*` test database and per-run unique suffix now retain the fixture users, point accounts, lots, reservations, allocations, and movements as intentional audit evidence. Cleanup continues to remove generation, PPT, billing, wallet, and audit rows through ordinary SQL with all production triggers enabled.

No production code, migration, schema, public API, frontend, Provider, Connector, deployment, shared database, or production system changed. No push, merge, deploy, production migration, or traffic action occurred.

## Scope and parent

- Worktree: `E:\code\work\先知AI-ppt-agent-phase1-integration-20260806`
- Required starting HEAD: `a57e544599fcedbcc3c539c6f66e28c7d2a4dfec`
- Modified implementation/test file: `backend-go/internal/httpserver/ppt_postgres_http_test.go`
- Evidence file: this report.
- Other agents' files and reports were not modified or staged.

## Static RED/GREEN

The initial source scan matched all unsafe cleanup elements:

- `cleanupPPTHTTPPersonalPointRows` call and helper;
- `set local session_replication_role = replica`;
- deletes from `xz_personal_point_lot_movements`, allocations, reservations, and lots;
- the following deletes from `xz_point_accounts` and `xz_users`.

That was the expected static RED because the fixture cleanup could bypass append-only and policy triggers.

After the minimal test-only change, the same file was scanned case-insensitively for:

```text
session_replication_role
cleanupPPTHTTPPersonalPointRows
DISABLE TRIGGER
TRUNCATE
delete from xz_point_accounts
delete from xz_users
delete from xz_personal_point_
```

Result: zero matches.

## Exact change

1. Removed `cleanupPPTHTTPPersonalPointRows` completely.
2. Removed the call that invoked it.
3. Removed the later point-account and user deletion loop.
4. Added a narrow test comment explaining that the dedicated database and unique suffix own the retained append-only audit trail, accounts, and users.
5. Preserved the existing legal cleanup of assets, generation tasks, PPT tasks, billing rows, wallet rows, and audit rows.

There is no trigger disabling, replica session, truncate, fallback, or direct fabrication of point-lot rows.

## Real PostgreSQL verification

Only local dedicated database `ppt_agent_phase1_service_codex_20260807` was used through `PPT_TEST_DATABASE_URL`.

| Command | Result |
| --- | --- |
| `go test ./internal/httpserver -run '^TestPostgresPPTAPIAssemblyDoesNotImportOrMirrorLegacyTaskFile$' -count=3 -v` | PASS, three consecutive real-DB executions, 8.608s |
| `go test ./internal/httpserver -run 'PPT' -count=1 -v` | PASS, 21.285s; unrelated personal-point PostgreSQL case SKIP because its separate DSN was unset |
| Task 0 exact protected Feishu/HTTP/video/storage aggregate | PASS: Feishu 2.630s; HTTP 7.812s; provider/video 1.483s; storage 1.398s |
| `gofmt -w internal/httpserver/ppt_postgres_http_test.go` | PASS |

Post-run database evidence after three direct assembly runs plus the focused-suite assembly run:

- retained fixture users: 12;
- retained point accounts: 12;
- retained personal-point lots: 4;
- retained reservations: 4;
- retained movements: 14;
- remaining generation tasks for `ppt_http_%`: 0;
- remaining PPT tasks for `ppt_http_%`: 0;
- remaining billing events for `ppt_http_%`: 0;
- remaining actor audit rows for `ppt_http_%`: 0.

This proves the append-only point audit trail remains while ordinary mutable test artifacts are cleaned.

## Risk and rollback

The dedicated test database intentionally accumulates uniquely suffixed audit fixtures. This avoids collisions and preserves production trigger behavior, at the cost of bounded local test-data growth. The database-name guard prevents this fixture from running against a non-`ppt_agent_phase1_*` database.

Rollback is a commit revert only. Reverting would restore the trigger bypass and is therefore not recommended. No production-data rollback exists or is required.
