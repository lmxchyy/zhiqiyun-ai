# Issue199: default-block drain groundwork — activation BLOCKED

## Decision and boundary

This is **partial safe groundwork**, not functional quarantine activation or
permission to release. The deployed gate has **no exception input**: no manifest,
hash, approval/recovery boolean, environment switch or model result subtracts a
provider execution. All unresolved executions, including actual9, still block.
No business recovery, persistence, settlement, provider, canary, killswitch or
DLQ policy is changed. Issue198 remains separate.

The audited source versions are candidate/base
`0183f267a3faa63e9dec0c14d871ad136fcfe779` and old runtime
`b45e72613dff3863d9eacc13cc99e4d320cf284f` (local Git materialization, not a live
inspection). Failed task status and released money do not establish provider
terminality. An expired lease does not stop a goroutine or external operation.

## Deployed observations

`deploy.sh:check_safe_drain` invokes `ops/verify-safe-drain.py`; the existing
pre/post runtime, manifest, digest, config, lock, migration, rollback and dirty
worktree gates remain in place. The helper and its existing runtime dependency
are in the prestage script inventory. Proof verification now requires every
inventory member, so omission of the new helper cannot make its bytes unbound.
Old proofs lacking that binding must be regenerated, never bypassed.

The helper executes only container discovery, read-only PostgreSQL SELECT and
the existing configured-vhost RabbitMQ management **GET** probe. It uses psql
`-X`, `ON_ERROR_STOP`, session-enforced read-only mode and statement timeout.
It never consumes, acknowledges, requeues, declares or deletes a message. Errors
are fixed summaries; subprocess stderr, credentials and evidence are not printed.
It reads and compiles the proof-bound runtime helper source directly, bypassing
ignored `.pyc` caches. `dont_write_bytecode` alone cannot prevent cache reads.
Tests preseed a timestamp/size-valid malicious cache, demonstrate ordinary-loader
execution in an isolated positive control, then verify the deployed helper ignores
that cache and rejects missing CLI arguments. No bytecode is written to the
proof-bound checkout.

Independent blockers:

* Any unexpired task lease, regardless of status. Nonterminal tasks also block,
  even without a lease (conservative strengthening over the previous count).
  In particular, five protected PROCESSING tasks can still block: this change
  does not assert that only nine blockers remain or that release can proceed.
* NULL/unknown task status, disagreement between `status` and `task_status`,
  or any nonterminal/NULL/unknown execution status. `COMPLETED` maps to
  `SUCCEEDED` only in the legacy task status column.
* Missing/ambiguous task association, NULL execution identity/attempt, invalid
  attempt, duplicate task/execution identities or duplicate task/attempt pairs.
* Pending **and publishing** outbox rows, for either outbox, plus NULL/unknown
  outbox states. Only existing terminal `published`/`failed` are nonblocking.
  Migration106 defines video outbox states `pending`, `published`, `failed`;
  hypothetical/future `publishing` and any other unknown state block, rather
  than treating a legal historical `failed` row as active.
* Ready/unacked work in the existing `x.ai.*` active/retry queue scope, malformed
  metadata, missing required queues/consumers or duplicate queue names.
  The existing `.dlq` exclusion is unchanged; DLQ depth is neither approval nor
  evidence of provider completion. No Issue198 cleanup is authorized.
* Missing required tables/columns or any DB/query/metadata/container error.
  Required tables are referenced directly, not optional zero-count fallbacks.
  No inner join or generic `FAILED + expired lease` execution exclusion exists.

A zero SQL count is followed by fresh DB and queue observations and stable
container identities before returning. A changed count/identity blocks. Timeout
is finite and bounded (0..300 seconds); each observation command is bounded by
the runtime helper. These samples **narrow, but do not eliminate, TOCTOU**. They
are not a transactional queue/DB snapshot, provider fence or admission lock.
An operation can begin after the final observation. The unchanged stop/exit gates
do not turn a historical snapshot into restart-safe quarantine enforcement.

## Offline model and evidence format

`ops/quarantine-drain-model.py` is a pure, no-I/O **hypothetical model**, not a
release helper. Deployment never imports or invokes it. Every result contains
`deployment: BLOCKED`, including the synthetic positive case.

An entry binds:

* execution/task IDs, attempt, generation, execution status and durable request
  ID (explicit null allowed when absent);
* both task status fields, ownership and lease;
* financial evidence with source and SHA256 (wallet ledgers and personal
  reservations must be distinguished), assets, storage, durable results,
  correlation and log evidence, each with source and SHA256;
* canonical snapshot SHA256, approval identity/time/reference, release SHA,
  not-before window and expiry.

Missing/extra fields, invalid types, duplicate/extra requested IDs, duplicate
identities, active ownership/lease, conflicting statuses, expiry, release mismatch
and snapshot mutations fail closed. A separately supplied **synthetic test
oracle** binds the complete entry digest and assumes isolated recovery. This
oracle is deliberately not accepted by deployment, not a trusted approval file,
not cryptographic approval authentication and not implemented runtime isolation.
Rehashing a changed entry cannot match the unchanged oracle. Self-asserted
`approved=true` / `recovery_verified=true` is not an oracle and is rejected.
A hypothetical eligible entry excludes only itself; other IDs remain blockers.
Actual evidence cannot gain trust by being fed through this demonstration.

Production approval identity verification, evidence provenance/authentication,
restart/cutover isolation, comprehensive historical logs and atomic release
observations are **unimplemented prerequisites**, not TODO bypasses. A hash only
binds bytes. Activation requires a separately approved design and evidence that
all recovery paths obey an enforceable fence; not another attestation field.

## Recovery audit constraints (both versions unless noted)

Paths below are `backend-go/internal/httpserver/` unless specified. Line numbers
refer to the audited versions, before this control-plane-only change.

| Entry point | Source evidence and conclusion |
| --- | --- |
| Startup/stale watchdog | `server.go:135–136`, `api.go:264–279,315–327,371–379`: fresh scans skip terminal tasks; running ambiguous executions with request IDs can reconcile. Do not claim FAILED tasks are automatically polled. |
| Scheduler | Old `generation_scheduler.go:373–405`, candidate `408–445,514–524`: active dispatch/recovery; candidate also supports running image recovery. Preferential `task_status` can disagree with legacy `status`. |
| Worker restart/redelivery | Old `generation_worker.go:91–99`, candidate `109–120`; video `79–89`; PPT `95–105,173–181`: terminal-task guards exist. Pending/publishing outbox work remains independently eligible (`internal/messaging/outbox.go:49–91`). Candidate adds normal image consumer. |
| Ordinary user retry | `asset_center_api.go:416–429,433–479,510–517`: terminal UNKNOWN/SUBMITTED cannot directly resubmit. Safe-before-submit failure classification may enable a child retry. |
| Operator recovery API — blocker | Routes `server.go:767–768`; `generation_recovery_api.go:251–273` independently accepts UNKNOWN outcome-specific capture/release evidence without requiring running task. `370–419` transitions the execution before task settlement. Authenticated operator action, not an unauthenticated vulnerability or automatic poller. |
| In-flight provider/result handling — blocker | Candidate `api.go:1783–1801`, old `1706–1725`: initial terminal/ownership check precedes provider work. Candidate hooks `341–384`, old `326–369`: provider GET and durable outcome persistence may already be running. `internal/providerexecution/store.go` locks execution, not a quarantine state. |
| Artifact and settlement ordering | Candidate `api.go:1815–1834`, old `1738–1753`: archival precedes task completion. Durable local recovery candidate `1443–1543`, old `1408–1475` can also archive. `generation_storage.go:172–190` creates storage files. Completion has terminal no-op (candidate `postgres_store.go:1575–1584`, old `1565–1573`); therefore execution mutation does NOT prove task capture/asset creation. Storage is an earlier separate effect. |
| Billing | `billing_v1_store_postgres.go:395–430` lists reconciliation diagnostics, not a failed-task repair executor; personal reservation reconciliation skips inactive tasks. Released money alone proves no provider outcome. |
| Callbacks/connectors/library recovery | No generation-provider webhook registration found in inspected server; not proof about external systems. Hooks `83–100` persist correlation; connector queue `61–66,154–165` restarts working jobs. `providerexecution.Service.Recover` has test callers in inspected source, not a verified independent production loop. In-flight callbacks/connector associations/external schedulers still need evidence. |

The operator mutation and already-in-flight paths defeat snapshot-only isolation.
There is no finding that actual9 currently has an in-flight operation. There is
also no proof that none can resume across restart/cutover. Hence activation is
BLOCKED; business-code refactoring to change this is out of scope.

## Actual9: all BLOCK

Execution/task pairs: `103/324, 102/323, 100/321, 31/251, 25/245, 22/242,
14/234, 12/232, 1/221`.

Eight unknown/possibly-submitted and one submitted. Execution14 has a durable
request ID but insufficient trusted channel/correlation evidence. Only six
personal reservation rows are RELEASED; tasks221/232/234 instead have wallet
RESERVE/RELEASE evidence, not matching personal reservation rows. No provider
terminal proof or approval is fabricated. Tests use an identity/status-only
redaction, not raw payloads, secrets or historical provider requests.

## Local validation and protected surfaces

Commands (never execute deploy main against a real environment):

```sh
python3 tests/issue199-quarantine-drain-test.py
ISSUE199_DOCKER_REPLAY=1 python3 tests/issue199-quarantine-drain-test.py
shellcheck deploy.sh ops/prestage-release.sh ops/verify-prestage-proof.sh
node --test tests/prestaged-release.test.mjs tests/prestaged-round4-gates.test.mjs
node --test tests/production-contract.test.mjs
RUN_PRODUCTION_CONTRACT_DOCKER=1 PRODUCTION_CONTRACT_IMAGE=xianzhi-production-contract:local bash tests/production-contract.harness.sh
```

The focused replay requires a local unix/npipe Docker endpoint, cached
`postgres:16-alpine`, and creates a uniquely named, network-disabled disposable
DB. Fixture writes are isolated; the observed SQL runs read-only. It tests actual
SQL, corruption, unsupported schema, NULLs, leases, both outboxes, actual9 and a
changed release observation. This is not production recovery/restart validation.
Production-contract image reuse is not a rebuild of this candidate. Detailed
pass/fail/not-run results belong in the implementation evidence, not an overall
release PASS claim.

- [x] P1: dirty/pushed-commit gates retained; helper bytes required in proof.
- [x] P2: release-focused regressions provided; results reported separately.
- [ ] W*/M*: no frontend/business changes; unrelated regressions not run.
- [ ] Production health, full recovery isolation and release authorization: not
  performed or established. No production connection, commit, push or PR.

## Optional non-enrolled history protocol (forward fix; not rollout authority)

The current source adds an optional history-protocol-1 challenge to the existing
packaged API/generation-worker capability evidence. Frozen v1 evidence without
this extension remains strict. The verified, unexpired official Prestage Proof
must bind helper/SQL/schema source, release SHA, immutable image and packaged
binaries; local synthetic images and these regression tests are **not** an
official Carrier, human approval or permission to release. Rollback capability
still uses the original challenge, without inheriting this extension.

Only two separately proven shapes can be classified read-only:

* Unique `TEXT_TO_IMAGE`/`IMAGE_TO_IMAGE`, `PROCESSING`/`DISPATCHING`, exactly one
  attempt-1 succeeded image execution with a well-formed durable image array,
  positive generations and execution strictly older. Mode is empty, and canary,
  fair-scheduled and dispatch-owner keys must be **absent**, not false/blank.
  There is no valid lease, active transport, asset or task result. Owner strings
  are not eligibility: full arbitrary/null/blank worker, lease and heartbeat
  bytes remain bound to the evidence.
* Unique positive attempt-1 failed image orphan, `definitive_not_submitted`,
  NULL generation, created/updated before migration119, zero task associations,
  no provider request, result, next check, asset or active transport. NULL is
  **not** a fence: actual task-rooted entrypaths reject task-not-found before
  creating an attempt, provider call, persistence or settlement.

A repeatable-read/read-only observation validates exact schemas and task/
execution identity constraints. It compares canonical digests of classified
rows and their linked financial, correlation, output-storage and transport
context, not whole business tables. Unrelated normal in-flight work can finish
while all global lease, nonterminal, duplicate/identity and transport counters
remain strict. Same-count historical evidence drift rejects a fresh drain.
Changes to shared linked financial context conservatively require a fresh drain.
Pending inbox claims for either actual image consumer independently block the
optional observation globally, even with NULL metadata and no outbox linkage.
Task-linked pending inbox evidence also blocks. No age, terminal-task or enrolled
execution exemption is implied: the terminal consumer branch can complete an
inbox before the provider quarantine check. Completed unbound markers without
attribution are outside the historical digest, not certified inert history;
packaged callback before/after comparisons still observe all fixture inbox rows.

Empty dispatch mode defaults to NORMAL. A pure succeeded-older-positive SELECT
under the consumer's task lock rolls back its inbox claim before returning
Fenced; equal/NULL/future/failed paths retain existing behavior. The challenge
uses matching real request fingerprints, valid envelopes, independent GET/POST
and storage observation, full fixture DB equality and a separate funded normal
valid-envelope consumer control per packaged role. This control uses a real
admitted model/channel against the owned sink (not a sentinel/mock model), and
requires provider/storage effects, durable output, capture and inbox completion.
It attests reachable entrypaths only: direct persist/transition primitives are
not a general historical immutability barrier. Valid-lease checks and sampled
DB/broker TOCTOU limitations remain; expired leases do not terminate old code.
No extra enrollment, history repair, provider recovery or DLQ action is added.

Mandatory historical DB/packaged modes require zero skips. Windows does not
provide the required CGO race toolchain: Ubuntu CI remains a required review gate.

### Bounded evidence-contract corrections (source-ready, validation pending)

The historical-only storage graph roots ALL classified identities, including
orphans, separately from actual task rows. Bidirectional file relations close
connected files (including cycles/diamonds with UNION visited-node termination)
and their referenced configuration, jobs, multipart uploads and parts. A shared
configuration is hashed when referenced, but does not pull every unrelated file
in that configuration into the graph. Connected/shared relevant mutations require
a fresh drain; there is no depth truncation, invented task or generic SQL fallback.
The shared exact9 approval projector is unchanged.

`generationTaskForUpdate` decodes behavior fields from `raw`: relational running
status alone is not sufficient. Classification requires exact typed parity for
id/userId/type/status/model/prompt and object params, plus optional non-overridden
mapped strings and no case-insensitive competing aliases. Optional NULL/empty
relational strings permit only absent/empty raw strings, reflecting Go zero
values. Overridden fencing/task_status/financial fields are not compared with
obsolete raw copies. The callback's terminal behavior is unchanged: a Go
counterexample demonstrates its inbox write for raw-terminal/column-running
rows, which native/fixed classification must reject. `{}` raw cannot certify
history. All six current production pending claims remain independent blockers.

Optional capability snapshots require exact source-defined table row fieldsets
and PostgreSQL-compatible types (integer is never bool/float/NULL; nullable
integer remains explicit NULL). Complete fixture lease/heartbeat/raw/result/error,
provider identity/results/timestamps and financial/storage/inbox rows are bound,
not abbreviated equal-hash projections. Rehashed deletion/truncation/type negatives
cover succeeded/orphan snapshots and funded consumer controls. The real seed and
snapshot routines are exercised on rolled-back owned PostgreSQL fixtures.

The Ubuntu `safe-drain-packaged-transport` job builds ALL4 actual final-source
Linux binaries, packages a NONOFFICIAL local fixture image, obtains an immutable
RepoDigest from a UUID-owned loopback-only registry and records an explicitly
NONOFFICIAL identity-only/nonbehavior prerequisite. It then runs unchanged strict
`attest(synthetic=False, include_history=True)` with its 300s budget inside the
actual Python3.6.8 full transport harness. Disposable HMAC/manifest/Proof fixtures
never become official Carrier/Proof or human approval. No flag is flipped, no
verifier branch bypasses synthetic rejection, and no control is omitted. Required
counts are both roles, blocked/allowed pairs, six unavailable faults, two history
observations, two funded controls, and all15 transport tests with zero skips.
Legacy180/synthetic900/host900 limits remain fixed. Owned resources and snapshots
are retained as CI artifacts on failure; incomplete phases/results/cleanup fail.

Parent must configure `safe-drain-required` as a required draft-PR check. This
always-run aggregate fails unless packaged/transport, existing backend Linux race
and existing cold production-contract jobs all actually succeed (not skip/cancel).
Those actual Ubuntu results remain PENDING; local source checks are not package
certification, official proof, merge/release approval or pending-inbox exemption.
