# Release213 bounded cold recovery (DEV/TEST deliverable; NOT READY)

This separate entry is **not authority to execute production recovery now**.
Independent review, exact-controller-head Ubuntu CI, fixed-image certification,
current authenticated observations and a new explicit human approval remain
required. Parent alone owns publication and any later operational authorization.

## Fixed scope

- App source: `b749a9ad1e543ac2f0b272aaac1db9b56065c85b`.
- App digest: `sha256:6ca5046ac2bd53b4ee7e5b96bab5bb04cd8d308d8a292d4fd2004bc75609390b`.
- Local app image: `sha256:3955f86a32d12e68e478c70536607455546885fa07f730a6f5c9089cd0fbf2da`.
- Controller source: a **different clean, reviewed commit**, bound separately.
- Only the two existing, pinned API/Worker CIDs in
  `ops/cold-recovery-anchor.json` may start. No recreation, pull, Compose up,
  dependency/old-image/ancillary startup, migration, re-enrollment, retries,
  business SQL writes, desired-env persistence or original `complete`.

The anchor records local historical exact9 full enrolled records (native NULL
and bigint generation included), seven historical business/financial hashes,
and service identities. It is not current runtime authorization. The seven
queries retain the original scope: exact9 + orphan execution10 + stale tasks
313/322/326/331/332/333; `row_to_json`, ID text ordering and MD5 remain exact.
Actual current data must match them. Original authenticated proof bytes and
proof nonce are read as **historical identity only**, never renewed authority.

## Entry and later human ceremony (description only)

`ops/cold-recovery-resume.py` has three separate phases with fixed repository
paths; there are no target/path/synthetic/skip CLI switches:

1. `prepare`: authenticate the stopped hold and historical identity; take fresh
   native READ ONLY observations; run real packaged `attest` with full history
   on owned isolated fixtures; reobserve; sign a recovery-only candidate Proof.
   This cannot start business or auto-approve execution.
2. `approve --operator <operator> --confirmation '<displayed exact phrase>'`:
   a root key holder explicitly reviews and mechanically signs the candidate.
   Root-owned, non-symlink paths and a private existing release-trust key are
   required. No new trust key/system or environment secret bypass is created.
3. `execute`: independently validate the signed human document and all current
   bindings, consume its nonce durably **before** starts, start only the bound
   pair, and run the existing process/readiness/worker/runtime validators.

Exact later phrase grammar (not present authorization):

```text
APPROVE COLD RECOVERY API_WORKER_ONLY candidate=<sha256> hold=<sha256> controller=<40hex> app=b749a9ad1e543ac2f0b272aaac1db9b56065c85b digest=sha256:6ca5046ac2bd53b4ee7e5b96bab5bb04cd8d308d8a292d4fd2004bc75609390b ANCILLARY_STAYS_STOPPED NO_MIGRATION NO_REENROLLMENT SINGLE_USE
```

The phrase alone is insufficient: the signed document binds the full candidate,
operator, nonce, snapshot, CIDs, controller/app identity and validity window.
HMAC authenticates the trusted key-holder ceremony, not physical human presence.
Candidate validity is at most900s; human execution validity at most300s and never
beyond candidate validity. Checks occur after long observations, after sentinel
fsync and before each start and commit. Expiry fails; there is no TTL extension.
Full-path timing on the fixed production-like configuration is still unmeasured.

## Durable outcomes and guard

The hold directory always remains and continues blocking ordinary
prestage/deploy/rollback/monitor mutation. Spent nonces and original historical
hold remain through failures and later attempts.

```text
STOPPED_RECOVERY_REQUIRED -> RECOVERY_STARTING
RECOVERY_STARTING -> RECOVERY_API_WORKER_VERIFIED_ANCILLARY_HELD
RECOVERY_STARTING -> STOPPED_RECOVERY_REQUIRED (only after verified fence)
RECOVERY_STARTING -> RECOVERY_REQUIRED_UNKNOWN (uncertainty)
```

Limited success is **only** API/Worker verified health, with disk-monitor,
postgres-backup and migrate intentionally held. It is not full original release
completion, env persistence or promotion of the ordinary release ledger.

A bounded, ephemeral Linux fork guard owns start sequencing and cleanup. It is
not an installed daemon, cron, service or general recovery framework. The guard
alone writes terminal success, serialized with its deadline/EOF fence. It checks
signed attempt, lock ownership, CID bindings, fresh human/capability authority
and deadline at commit. Parent cannot overwrite a stopped/unknown fence with
success. Lost owner/substituted attempt never authorizes foreign cleanup.

A Docker start error (including non-timeout disconnect) leaves an authenticated
indeterminate-operation sentinel, UNKNOWN and retained lock after attempted
fencing. **UNKNOWN does not promise business stopped.** Neither a killed CLI nor
an immediate stop sample proves daemon-side operation settlement. No stale-lock
reclaim or recovery bypass is offered. Whole-host/guard loss is likewise not
proved safe by traps. Operational admission remains blocked pending review of
actual Docker ordering/settlement evidence; no broader infrastructure is added.

## Validation and limits

- `python -B tests/cold-recovery-resume-test.py`: adversarial signed-domain,
  replay/expiry/source, native types, readonly authority and frozen-source tests.
  Mock passes are not packaged certification.
- Actual Linux Python3.6.8: `tests/cold-recovery-resume-docker-test.py` uses only
  UUID-owned network-none PostgreSQL and TERM-responsive mechanism containers.
  It exercises real `execute`, native reads, exact two starts, partial failures,
  guard deadlines, post-start controller SIGKILL/TERM/INT, foreign ownership,
  fsync expiry, and terminal commit/EOF/audit races. Its internal scoped adapter
  mocks fixed-image certification and production process health; those mechanism
  results do **not** prove the fixed production profile or its full health gates.
- The late-completion adversary intentionally starts a **new owned Docker
  client after fencing**. It models unresolved delayed completion and verifies
  durable UNKNOWN/sentinel/lock honesty; it does **not** prove an earlier daemon
  request was actually accepted before CLI disconnect. Actual dockerd ordering
  and production daemon version are not established by this test.
- `.github/workflows/cold-recovery-resume.yml`: nonpublishing Ubuntu job uses
  ephemeral Actions `GITHUB_TOKEN` with `packages:read` to obtain **only** the
  fixed official GHCR digest, validates its local ID, then runs real packaged
  full-history `attest` and owned tests on Python3.6.8, zero skips. Unauthorized
  or absent package is failure; no rebuilt controller app image substitution.
  Tooling images/runner are not the target. No production/root credentials.
- Preserve history300s / legacy180s / synthetic900s / host900s budgets. The
  ordinary103 descriptors and original deploy/prestage/rollback/control tests
  are unchanged. Existing regression suites must stay green independently.

Remaining P1: exact-head CI and fixed digest availability/certification;
independent review of actual daemon settlement and guard crash windows;
full fixed-profile snapshot/start/health timing and integration; fresh current
hold bindings and later human authorization. Until cleared: **not safe for the
current hold**, irrespective of a mechanism test PASS.
