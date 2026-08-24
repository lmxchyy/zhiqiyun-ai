# Points Module Extraction Plan

> For the executing-plans skill: execute this plan in order, keep the worktree
> isolated, and stop only on a genuine external blocker or a safety regression.

## Goal

Move the Points capability toward a modular-monolith boundary while preserving
the existing API, PostgreSQL schema, transactions, idempotency, audit records,
policy provenance, and all current production behavior.

## Non-goals

- No database migration or schema redesign.
- No public API or response-shape changes.
- No Membership or Billing extraction in this worktree.
- No production deployment or business mutation.
- No new ORM, RPC, service, or database.

## Current contract to preserve

- `ADMIN_GIFT` creates one lot, wallet ledger row, lot movement, audit record,
  and preserves non-null `policy_version_id` plus the canonical policy snapshot.
- Explicit gift validity changes only `expires_at`.
- `ADMIN_CORRECTION` remains permanent and idempotent.
- Payment and generation callers continue to share the caller-owned SQL
  transaction through the existing `grantTx`, `reserveTx`, `captureTx`, and
  `releaseTx` operations.
- HTTP handlers retain existing paths, payloads, status mappings, and request
  diagnostics.

## Implementation steps

1. Add characterization tests around the existing service/repository boundary:
   admin gift provenance and validity, correction, idempotency, rollback,
   reservation/capture/release, policy reads, and ownership checks.
2. Add a `internal/points` package containing only domain/application types and
   errors that do not import Gin or HTTP packages. Keep the first slice small:
   grant, correction, policy, balance, and lot query contracts.
3. Add repository interfaces in the Points package and adapters in
   `internal/httpserver` that delegate to the existing PostgreSQL and JSON
   implementations without changing SQL or transaction ownership.
4. Move the service-level validation and pure policy/expiry helpers first;
   leave SQL implementations in place until the new package contract is
   covered by tests.
5. Change HTTP and non-HTTP callers to depend on the Points application
   contract, using adapters where necessary. Preserve transaction-aware paths
   for Billing and generation.
6. Run focused tests, all Go tests, production-contract tests, and protected
   Points regressions. Fix failures with the smallest change and add a test for
   each discovered regression.
7. Review the diff for dependency direction, secrets, schema changes, public
   API changes, and accidental Membership/Billing changes. Commit, push, and
   open one reviewable Points PR.

## Verification commands

```text
go test ./internal/httpserver/...
go test ./...
bash tests/production-contract.harness.sh
node --test tests/production-contract.test.mjs tests/migration-release.test.mjs
```

## Exit gate

`Points` owns its domain/application contract, HTTP remains a transport
adapter, all existing protected behavior is green, and the branch is ready for
review without production changes.
