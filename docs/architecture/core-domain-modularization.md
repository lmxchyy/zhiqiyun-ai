# Core Domain Modularization

This is a modular-monolith refactor. Points, Membership, and Billing remain in
the same Go backend, PostgreSQL database, and production image.

## Current slice: Points domain boundary

`backend-go/internal/points` owns transport-independent invariants for point
grant sources and expiry-policy shape. HTTP and PostgreSQL code remain adapters
during this first extraction slice so the public API, SQL schema, transaction
ownership, and production behavior do not change.

The boundary currently guarantees:

- grants have a positive amount and an idempotency key;
- sources are server-owned and must be known;
- expiry policies have a version, timezone, and calendar-month unit;
- enabled policies have a positive duration;
- domain validation does not import Gin or HTTP status codes.

## Ownership and transaction rules

- Points owns point-lot, wallet, ledger, movement, policy, and expiry rules.
- Repository implementations retain PostgreSQL/JSON persistence details until
  each operation has a covered application contract.
- A caller-owned `pgx`/SQL transaction remains the transaction boundary for
  cross-domain operations. Extraction must not introduce nested commits.
- Membership manual grants do not grant points.
- Billing calls the Points application boundary for paid entitlements and does
  not write Points tables directly.

## Protected behavior

`ADMIN_GIFT` must preserve its policy version and policy snapshot; explicit
validity changes only `expires_at`. `ADMIN_CORRECTION`, idempotency, audit,
rollback, paid 996 fulfillment, and HTTP compatibility remain protected by
the existing regression suite.

Further extraction is intentionally sequential: complete Points, then
Membership, then Billing. No schema migration is required for this refactor.
