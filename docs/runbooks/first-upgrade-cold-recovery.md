# First-upgrade cold recovery — development/review contract

## Authority in this change

Authorized: development in the isolated `fix/first-upgrade-cold-recovery` worktree,
owned disposable tests, independent review, and parent-owned PR CI.
Not authorized: production SSH, installation, prestage/proof generation,
enrollment, deployment, rollback, stopping/starting services, new generation,
history/DB/DLQ/key operations, merge or release. CI results are not production
approval. Do not use production as a fallback when local Docker is unavailable.

## Opt-in policy (not an operator execution procedure)

The signed `first-upgrade-cold` policy is restricted to the actual official
unsupported baseline `b45e72613dff3863d9eacc13cc99e4d320cf284f`, its pinned source
tree, source hashes, official manifest/provenance, actual receipt, immutable
image ID/digest, and binary hashes. An arbitrary timeout, unknown SHA, alternate
image, or capable rollback image is not a cold baseline. Target packaged
behavioral capability evidence remains mandatory and fresh; this policy does
not weaken target validation. No cold flag means the ordinary capable rollback
contract is unchanged.

Cold release creates a persistent HMAC-authenticated
`.prestage/cold-recovery-required/state.json` before stopping anything. It sets
`restart=no` on every project non-infrastructure container and every globally
exact old/target business image match, then stops and inspects them.
Infrastructure is limited to postgres/redis/rabbitmq/minio; infrastructure
labels never exempt actual business-image bytes. This deliberately includes
proxy/frontend and maintenance processes in the verified project.

All new non-infrastructure containers, including migration, are created using
the signed normalized cold Compose model with `restart=no`, **not** patched
after creation. This covers the SIGKILL window: the persistent hold remains and
new containers cannot automatically restart. SIGKILL cannot run a trap; it does
not claim all currently running target processes were stopped. Treat such a
hold as unverified recovery-required, never as a stopped PASS.

On deploy EXIT/INT/TERM failure, or explicit cold rollback, the helper performs
only Docker inventory/update/stop/inspection and signed filesystem audit. It
never starts the legacy business image and performs no SQL, migrations,
provider/billing/MQ/task/execution/quarantine mutations. Incomplete stop,
inspection, state or audit is UNKNOWN, never PASS. Receipts record SHA, stage,
trigger reason, observed container state and stopped roles. Separate release
health/catalog/drain gates retain their existing contracts; cold recovery is
not a replacement for those gates. Cron observation behavior is unchanged;
monitor mutations and release reentry are blocked by the persistent hold.

Only the **same successful in-flight owner** may remove its own armed hold,
after forward migration/runtime/health verification. Completion independently
checks authenticated/fresh proof, effective configuration, current lock token,
actual PID1/binary/image/environment/mount policies and role coverage, all
actual non-infrastructure restart policies, and restoration of previously
running project services (except the migration job). Required proxy/frontend
remaining down means failed completion and a retained hold, not silent success.
Successful completion preserves a signed `cold-forward-<nonce>.json` audit.
Target `restart=no` remains an intentional conservative **opt-in success
policy**; it is not automatically restored to `always`.

A failed or interrupted hold never auto-clears. There is no resume/start/hold
clear CLI in this change. API restart, any recovery/hold remediation and any
production action require separate explicit human authorization and a reviewed
procedure, with current stopped/unknown state investigated first.

## Isolated verification only

Run from a development checkout with the pinned baseline source available:

```sh
bash -n deploy.sh rollback.sh ops/prestage-release.sh ops/verify-prestage-proof.sh
shellcheck deploy.sh rollback.sh ops/prestage-release.sh ops/verify-prestage-proof.sh
PYTHONDONTWRITEBYTECODE=1 python3 tests/first-upgrade-cold-test.py
node --test tests/prestaged-release.test.mjs tests/migration-release.test.mjs
# Disposable local/CI daemon only; no production context or host database.
docker pull pgvector/pgvector:pg16
PYTHONDONTWRITEBYTECODE=1 python3 tests/first-upgrade-cold-docker-test.py
```

The shell suite runs actual release/helper paths with owned Docker transport
fixtures (not runtime proof). The real Docker suite uses UUID-owned image IDs,
project labels, network-none PostgreSQL, no host ports, full repository schema
and migrations, seeded task/execution/quarantine/billing/storage rows, and
before/after snapshots of all public tables. Poisonous new business commands
remain unstarted, stop/update/inspection faults remain UNKNOWN, repeat fencing
is idempotent, and actual SIGKILL confirms creation-time `restart=no` does not
restart. Ownership is rechecked before every fixture mutation and cleanup.
Missing Docker/image/schema failure is a nonzero test result, never skipped
runtime PASS. Neither suite proves official target behavior on production.

The existing `user-core` production-contract CI job checks out full baseline
history and requires cold shell, ordinary rollback and actual owned Docker/PG
tests. Independent review and CI must pass before the parent considers a PR
complete; the parent stops without merge or production operations.
