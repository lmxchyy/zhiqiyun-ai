# Offline quarantine approval trust and candidate format

Production quarantine verification supports two authorization modes:
1. **Operator Release Authorization (Standard)**: Reuses the existing release trust secret (`RELEASE_TRUST_SECRET` / `/etc/zhiqiyun/release-trust.key` / `.prestage/release-trust.key`) and authenticated operator SSH workflow. When the human operator inspects and explicitly approves the candidate, `ops/quarantine-approval.py approve` generates an authenticated manifest binding the candidate SHA256, snapshot SHA256, release SHA, exact execution identities, operator SSH identity, timestamp, and unique approval ID with HMAC-SHA256 signature.
2. **Offline RSA Authority (Optional/Legacy)**: Uses `registry.json` as the Carrier/Prestage-bound trust registry with an offline RSA keypair.

`registry.json` is the Carrier/Prestage-bound trust registry placeholder for RSA authorities.

## Key custody and provisioning

- A named human production-release approver owns the decision and RSA private key. Do not create, request, copy, log, or use that private key in Pi, CI, the repository, release host, deploy tooling, or a general automation.
- The approver supplies the public key, approved key ID, and SHA-256 fingerprint through the reviewed change. Verify the fingerprint out-of-band with that approver before merging. The public key and registry change must be in the reviewed commit and Carrier/Proof; no manual `/etc` trust-root edits.
- Keep revocation/rotation in the same Carrier-bound registry. A replacement key is not trusted merely because it is present in a release-host directory.
- The current empty registry is a deliberate provisioning placeholder, not an approval authority and not a reason to bypass verification.

## Pre-migration 119 NULL execution identities (#207)

Exactly six reviewed `(execution_id, task_id, attempt)` tuples are Carrier-pinned by SHA-256 in `ops/quarantine-approval.py`. Each pin hashes the ASCII bytes of `json.dumps({'attempt': attempt, 'execution_id': execution_id, 'task_id': task_id}, sort_keys=True, separators=(',', ':'), ensure_ascii=True)`. A NULL generation outside this exact set is rejected even if its timestamps precede migration 119. Within the same repeatable-read transaction, the sampler requires an exact `schema_migrations` row for `119-execution-generation-fencing.sql`, the execution creation time and task creation time to precede its applied time, a single attempt-1 execution in `unknown`/`submitted` state for that task, and the raw execution generation to remain NULL. Its canonical snapshot binds the actual NULL, identity digest, both creation times, migration time, and `legacy_generation_unverifiable=true` with reason `pre-migration-119-generation-not-recorded`; it never assigns a fabricated generation. Post-migration NULL or a changed identity is rejected. The signed manifest binds the exact identity and live snapshot hash; the six pins grant only eligibility for *human review*, not enrollment or an approval signature. Normal fenced executions retain their existing generation checks.

## Candidate and human decision

`ops/create-quarantine-candidate.py` is the release-host entry point. It accepts a bounded JSON array of exact execution identities, release SHA, validity interval and output path; it binds the current Compose/PostgreSQL container, refuses a release/recovery lock, and calls `sample_unsigned_candidate_read_only(...)`. That API starts one `REPEATABLE READ READ ONLY` transaction, projects core task/execution, personal-financial, and artwork/provider-result/storage-metadata families, hashes each canonical snapshot, and rolls back. The CLI rechecks the DB binding and release lock before writing an exclusive mode-0600 candidate file. It has no registration or task/execution mutation path. Remote object readability and exact historical provider-attempt attribution remain explicitly unproven; the snapshot does not promote them to verified facts.

Example shape (use operator-reviewed paths/identities and an explicitly bounded time window):

```bash
python3 ops/create-quarantine-candidate.py \
  --compose compose.prod.yml --env-file .env.production \
  --entries /secure/review/execution-identities.json \
  --release-sha "$RELEASE_SHA" --key-id release-trust-key \
  --not-before "$NOT_BEFORE" --expires-at "$EXPIRES_AT" \
  --output /secure/review/quarantine-candidate.json
```

The candidate is capped at 32 MiB and is marked `UNSIGNED_REQUIRES_HUMAN_REVIEW`. It contains no signature, approval ID, review digest, or decision. A named human approver must inspect the candidate and independently establish the exception basis.

To approve the candidate under the operator release authorization model:

```bash
python3 ops/quarantine-approval.py approve \
  --candidate /secure/review/quarantine-candidate.json \
  --release-sha "$RELEASE_SHA" \
  --confirm APPROVE \
  --output /secure/review/quarantine-manifest.json
```

This binds the operator identity, unique approval ID, and candidate SHA256, and signs the manifest with the host's release trust key. Verification with `verify(...)` checks the HMAC-SHA256 signature and re-checks the live database state under lock before enrollment.

No AI agent, CI job, or candidate-generation code is an approver. A candidate or unsigned manifest is never production acceptance, and it does not authorize enrollment, Prestage, migration, quarantine-table writes, retries, capture/release, DLQ actions, or cutover.
