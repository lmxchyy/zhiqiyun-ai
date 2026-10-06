# Offline quarantine approval trust and candidate format

`registry.json` is the Carrier/Prestage-bound trust registry. It currently has no authorities, so production verification is intentionally **fail-closed**. The only accepted authority identity is `prod-quarantine-approval-v1`; an approval key, key ID, and SHA-256 fingerprint must be added through a reviewed source change and immutable Carrier. The registry's `public_key_file` is a basename resolved beside the registry, never an environment/CLI path override. The verifier checks the key file's owner/mode and SHA-256 against the Carrier-bound registry entry.

## Key custody and provisioning

- A named human production-release approver owns the decision and RSA private key. Do not create, request, copy, log, or use that private key in Pi, CI, the repository, release host, deploy tooling, or a general automation.
- The approver supplies the public key, approved key ID, and SHA-256 fingerprint through the reviewed change. Verify the fingerprint out-of-band with that approver before merging. The public key and registry change must be in the reviewed commit and Carrier/Proof; no manual `/etc` trust-root edits.
- Keep revocation/rotation in the same Carrier-bound registry. A replacement key is not trusted merely because it is present in a release-host directory.
- The current empty registry is a deliberate provisioning placeholder, not an approval authority and not a reason to bypass verification.

## Candidate and human decision

`ops/create-quarantine-candidate.py` is the release-host entry point. It accepts a bounded JSON array of exact execution identities, release SHA, validity interval and output path; it binds the current Compose/PostgreSQL container, refuses a release/recovery lock, and calls `sample_unsigned_candidate_read_only(...)`. That API starts one `REPEATABLE READ READ ONLY` transaction, projects core task/execution, personal-financial, and artwork/provider-result/storage-metadata families, hashes each canonical snapshot, and rolls back. The CLI rechecks the DB binding and release lock before writing an exclusive mode-0600 candidate file. It has no registration or task/execution mutation path. Remote object readability and exact historical provider-attempt attribution remain explicitly unproven; the snapshot does not promote them to verified facts.

Example shape (use operator-reviewed paths/identities and an explicitly bounded time window):

```bash
python3 ops/create-quarantine-candidate.py \\
  --compose compose.prod.yml --env-file .env.production \\
  --entries /secure/review/execution-identities.json \\
  --release-sha "$RELEASE_SHA" --key-id UNPROVISIONED \\
  --not-before "$NOT_BEFORE" --expires-at "$EXPIRES_AT" \\
  --output /secure/review/quarantine-candidate.json
```

The command does not create a production approval or alter the database. The `UNPROVISIONED` key ID is deliberate while the trust registry is empty; no final manifest can verify until the reviewed public key/fingerprint is Carrier-bound.

The candidate is capped at 32 MiB and is marked `UNSIGNED_REQUIRES_HUMAN_REVIEW`. It contains no signature, approval ID, review digest, or decision. A named human approver must inspect the candidate and independently establish the exception basis. Only after that person has made the approval decision may a separately controlled operator provide unique approval IDs and SHA-256 hashes of the human review evidence to `unsigned_manifest_bytes(...)`. That function produces canonical unsigned v2 manifest bytes; it does not approve or sign. `verify(...)` rejects the output until the authorized human's offline RSA signature is appended and verifies against the Carrier-pinned public-key fingerprint. Manifest signature is over canonical JSON without the `signature` field (RSA PKCS#1 v1.5 with SHA-256, as in Issue #203).

No AI agent, CI job, or candidate-generation code is an approver. A candidate or unsigned manifest is never production acceptance, and it does not authorize enrollment, Prestage, migration, quarantine-table writes, retries, capture/release, DLQ actions, or cutover.
