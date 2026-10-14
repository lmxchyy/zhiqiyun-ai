# Fixed recovery forward switch (development delivery; production forbidden)

This is a separate controller for exactly two services: `xianzhi-ai` and
`smartvideo-worker`. It is not ordinary deployment or a generic recovery entry.
No production execution, SSH, signing, provider request or task creation is
currently authorized. Review and actual exact-head Ubuntu CI must precede any
later production authorization; CI success alone is not authorization.

## Fixed identities and replacement semantics

- OLD application: `b749a9ad1e543ac2f0b272aaac1db9b56065c85b`;
  digest `sha256:6ca5046ac2bd53b4ee7e5b96bab5bb04cd8d308d8a292d4fd2004bc75609390b`;
  local image `sha256:3955f86a32d12e68e478c70536607455546885fa07f730a6f5c9089cd0fbf2da`.
- NEW application: `071ccc00136459b612c2c274be6f65bfdc72b76d`;
  digest `sha256:bc6da7a08a53907f7f4c3d302d616012a718163c42e136cd34c70feca6226cbd`.
  Production uses the pinned Aliyun reference in the entry; isolated CI uses
  the same official GHCR digest, never a rebuilt application.
- Manifest bytes: `62d18f187f143bf713b6eb47cbf61ac1f4ba7a042ed67d0ba4d333bf181e22c8`.
  The published controller SHA is separate from both application SHAs.

The entry stops **both exact OLD anchor CIDs**, verifies clean exit, removes
only those stopped containers (no force/volume removal), and recreates only the
NEW pair. Native Compose uses `--no-start --no-deps --no-recreate --no-build
--pull never`, external existing networks/volumes and `restart=no`. Full NEW
CIDs are durably bound before serial starts. CIDs change; this is **not** an
in-place image change or start-existing-only recovery. There is no OLD restart,
rollback, migrate, enrollment or desired-image persistence.

## Authority and state

The existing private root-owned trust key, shared release lock and cold hold
remain in place. Earliest authenticated stopped-hold identity/proof/anchor
establish historical ancestry; the latest successful recovery candidate and
approval bind through that successful attempt's exact hashes, operator, CIDs,
windows and once-spent latest nonce. Earlier completely fenced attempts may
remain in the spent ledger. Expired history grants no forward authority.

Only after external production authorization, the reviewed published entry's
three actions are `prepare`, `approve --operator <operator> --confirmation
<exact printed confirmation>`, and `execute`. `prepare` acquires/checks the
fixed target and runs real packaged capability/history attestation before any
OLD stop. It publishes a fresh source/hold/full-snapshot-bound candidate, not
execution approval. `approve` signs the exact entry-generated confirmation
(maximum300 seconds within candidate900 seconds), but performs no cutover.
`execute` consumes its nonce once before mutations; never replay or renew an
attempt. No target/path/skip/environment override is offered.

Immediately before cutover, both current hold and complete snapshot must equal
the candidate. Final checks retain exact9 quarantine records, seven protected
history hashes, completed migrations108–121, PostgreSQL binding, ancillary
identity/epochs, barrier, broker/readiness and normal PID1 policies. Task334's
succeeded113 manifest/generation reuse precondition is read-only; its transient
heartbeat/events are not frozen. Acceptance of PROCESSING/RUNNING or
SUCCEEDED/SUCCEEDED is not proof of end-to-end normal-consumer completion,
persisted artifacts, billing completion or a global no-provider-call guarantee.

A sole child guard owns all runtime mutations and settlement. Native CLI groups
and PostgreSQL pipe waits share its monotonic deadline and check parent EOF.
Local CLI termination is **not** cancellation/settlement of an Engine request.
Unresolved operations keep their marker, `RECOVERY_REQUIRED_UNKNOWN` and lock;
fencing never resends or samples the pending exact target into settlement. Other
independently authorized exact pair targets alone may be fenced. Complete scoped
fencing can record `STOPPED_RECOVERY_REQUIRED`; ownership loss refuses cleanup.
Guard SIGKILL/death can leave retained uncertain evidence, never automatic lock
reclamation. Parent settlement waiting is bounded and never becomes a second
mutation owner. Do not manually restart OLD, run Compose up/down, delete the
hold/lock or infer settlement from a later stopped sample.

Success is `FORWARD_API_WORKER_VERIFIED_ANCILLARY_HELD`, with NEW identities
recorded in the authenticated retained hold. Ancillary services and protected
compose/env bytes remain unchanged; ordinary deployment stays blocked.

## Evidence boundary and later authorization grammar

The new Ubuntu workflow requires exact published HEAD/clean clone, actual
Python3.6.8, owned real PostgreSQL/RabbitMQ and actual NEW packaged attestation;
missing dependencies fail, never synthetic fallback. New fixture MinIO uses
`docker.io/minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e`
with acquisition and actual image-identity checks. Public registry probes
returned401: source availability is **unproved**, and actual Ubuntu CI is still
needed. Historical denied-mirror workflow/controller/shared helpers are unchanged.

Local Windows unit passes and Linux-only test skips are not native evidence.
Response-loss integration injections occur after acknowledged real results, not
in-flight Engine acceptance. Historical OLD certificates in the isolated setup
are explicitly synthetic preconditions; NEW certification is never mocked.
Unresolved prior prepare-attestation journals remain a known P2 review item;
this delivery does not certify preparation cleanup/publication safe. Frozen
review evidence is `.evidence/forward-switch/review.diff` and
`review-source.json`. Independent review remains mandatory.

The **single later external production authorization grammar** is the following
one-line TEMPLATE, with the controller placeholder filled by the parent only
after independent review and required exact-head native CI. It delegates one
fresh bounded prepare→mechanical exact-phrase approve→execute session:
prepare1/approve1/execute1/renewal0. Failure, expiry or UNKNOWN means STOP, never
silent reprepare or resend. No candidate or approval exists now; placeholders
and this document are not authorization:

`AUTHORIZE PRODUCTION FIXED FORWARD controller=<40hex> old_app=b749a9ad1e543ac2f0b272aaac1db9b56065c85b old_digest=sha256:6ca5046ac2bd53b4ee7e5b96bab5bb04cd8d308d8a292d4fd2004bc75609390b new_app=071ccc00136459b612c2c274be6f65bfdc72b76d new_digest=sha256:bc6da7a08a53907f7f4c3d302d616012a718163c42e136cd34c70feca6226cbd API_WORKER_ONLY STOP_REMOVE_RECREATE_PAIR RETAIN_HOLD ANCILLARY_UNCHANGED NO_MIGRATION NO_REENROLLMENT NO_OLD_IMAGE_START NO_PROVIDER_RETRY NO_NEW_TASK NO_MANUAL_BUSINESS_WRITES NO_ROLLBACK SINGLE_USE prepare1/approve1/execute1/renewal0`

This external authorization is distinct from the fresh exact human confirmation
printed by `prepare`, which still binds the actual fresh candidate/hold hashes,
controller/source, fixed identities and candidate nonce/time/schema through its
signed digest. Neither ceremony relaxes signatures, TTL, single-use or ownership.
Normal runtime may complete334 automatically; the controller/operator never
manually writes tasks/executions/ledger/DLQ/files or retries a provider/new task.
