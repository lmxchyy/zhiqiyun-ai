# Issue #200 runtime quarantine barrier

This is a default-empty runtime fence. It does not enroll the historical nine
executions, change Safe Drain (#199), touch DLQ (#198), or authorize release.

## Decision

`providerexecution.RejectTask` / `RejectExecution` run inside the caller's
write transaction after the execution or task row lock, and again before a
provider call where the call happens outside that transaction.

- Relation absent (`to_regclass` NULL) is a successful observation that no
  approval can exist. It does not block ordinary work and is not a bypass flag.
- Any failed read, nil querier, malformed row, duplicate, identity or
  generation mismatch, release or evidence mismatch, expiry, or valid matching
  row blocks the mutation.
- A valid row still blocks. This barrier does not grant a Safe Drain exception.
- Logs contain only decision, reason, operation, execution/task IDs, approval
  ID, evidence hash, and release SHA.

## Covered writes

Operator recovery actions, watchdog repair, scheduler recovery, image/video
provider guard before Get/Create/Generate, video persistence before download,
execution transition/claim/result/check updates, fenced task completion and
failure, and generation capture/release/reserve point mutations.

Provider calls and billing/asset writes are refused before their side effects
when the check succeeds. A check after the row lock closes the write TOCTOU
window. A network provider call cannot hold the database lock for its whole
duration; the pre-call check plus the locked recheck is the enforced fence.
An approval inserted after the pre-check but before the call can still observe
one provider GET. It cannot commit execution, task, asset, or billing writes.

## Not done

No production enrollment, admin activation API, or #199 exception input.
Restart isolation for a provider call already in flight before the barrier
code starts remains a process-boundary limit. Query failure blocks the
operation; it does not prove the provider itself has stopped outside this
process.
