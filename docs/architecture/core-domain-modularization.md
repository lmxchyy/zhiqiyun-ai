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

## Current slice: Membership domain boundary

`backend-go/internal/membership` now owns the transport-independent manual
grant request normalization and expiry rule. A manual grant is valid for at
least the requested duration and never shortens an existing later expiry.

The PostgreSQL application path remains the owner of the single transaction
that updates the user projection, entitlement history, billing subscription
projection, audit log, and operator log. Manual membership still never calls
the Points application and never grants the paid-plan point entitlement.

## Current slice: Billing entitlement boundary

`backend-go/internal/app/payment` remains the application owner for payment
orders, verified callbacks, fulfillment idempotency, provider interaction, and
the caller-owned PostgreSQL transaction. `backend-go/internal/billing` now
owns the transport-independent paid point-entitlement contract used by paid
product fulfillment.

`billing.ParsePaidPointEntitlement` validates the immutable product snapshot
before payment invokes the Points application hook. Payment still owns the
single transaction, payment state transitions, fulfillment records, audit, and
idempotency key; this extraction does not introduce nested commits or direct
Billing writes to Points tables.

```text
HTTP/provider -> payment application -> billing entitlement contract
                              |
                              v
                    caller-owned transaction -> Points application hook
```

The paid 996 path continues to grant its server-side snapshot amount exactly
once. No schema, migration, public API, or production deployment behavior is
changed by this slice.

Paid fulfillment orchestration now enters through
`billing.ApplyPaidPointFulfillment`. It performs the token-record idempotency
check and paid entitlement projection inside the transaction supplied by the
payment application, then delegates the actual point balance mutation through
the Points hook. Billing does not open or commit a second transaction and does
not directly write Points tables.

## Current slice: Points repository boundary

The PostgreSQL point store now owns ADMIN_GIFT lot-expiry persistence through
`UpdateLotExpiryTx`. The admin transport passes the existing transaction into
the store; it no longer embeds point-lot SQL. This preserves the original
atomic sequence and makes the transaction owner explicit:

```text
admin transport -> point application path -> PostgresPersonalPointStore
                                      \-> caller-owned transaction -> Commit
```

The operation does not open or commit a nested transaction. Canonical policy
version and snapshot data remain supplied by the existing grant operation and
are preserved when the explicit validity period updates only `expires_at`.

## Current slice: Membership write repository boundary

Manual membership persistence is now owned by
`backend-go/internal/membership/repository`. The repository owns plan and user
row locking, user projection updates, entitlement history inserts, and
operator-log persistence. The HTTP layer retains transport authorization and
membership-specific rule decisions, and passes the caller-owned transaction to
the repository. Subscription projection writes continue through
`UpsertSubscriptionTx` in the same transaction.

```text
HTTP authorization/rules -> Membership repository operations
                                      |
                                      v
                              caller-owned transaction -> Commit
```

The handler no longer owns manual-membership SQL for the user projection,
entitlement history, or operation log. The transaction still commits exactly
once after the audit and all membership projections succeed; a failure rolls
back the complete manual grant.

## Current slice: Points application transaction boundary

Permanent point inflow sequencing is now owned by
`backend-go/internal/points/application`. `GrantPermanentTx` loads the account
through an injected repository operation, invokes the repository grant inside
the caller-owned transaction, reloads the projection, and verifies the
expected balance delta before returning. The HTTP package only adapts its
PostgreSQL store to that application contract; it does not sequence the
projection check itself.

```text
caller transaction -> Points application -> Points repository adapter
                                      \\-> reload projection -> caller commit
```
