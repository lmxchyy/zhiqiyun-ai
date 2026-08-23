# Production Contract CI

Phase 1 establishes a production-like gate before an application change can be
merged. The gate does not deploy, modify production data, or replace the
formal release process.

## What it proves

- The production image contains the runtime dependencies used by production:
  CA certificates, timezone data, `curl`, and the API and worker binaries.
- The runtime image is non-root and exposes the API healthcheck.
- The production Compose graph requires the migration service to complete
  successfully before the API or SmartVideo worker starts.
- A fresh PostgreSQL 16-compatible database can apply `database/schema.sql`
  followed by every numbered forward migration in sorted order.
- `.down.sql` files are excluded from the forward migration inventory.
- Existing Go tests remain responsible for transaction rollback, idempotency,
  trigger/constraint behavior, membership, points, and billing golden paths.

## What it does not prove

It does not use production credentials, connect to production, create real
business data, or prove external provider availability. Those checks remain
deployment and acceptance gates.

## Run locally

Static contract checks:

```bash
bash -n tests/production-contract.harness.sh
bash tests/production-contract.harness.sh
```

When Docker is available, run the full image and PostgreSQL replay contract:

```bash
RUN_PRODUCTION_CONTRACT_DOCKER=1 bash tests/production-contract.harness.sh
```

The same full command runs in the `production-contract` GitHub Actions job.

## Release relationship

The production release path remains:

```text
GitHub main → Gitee main → ./backup.sh → ./deploy.sh → health/runtime checks
```

`deploy.sh` remains the only supported production deployment entry point.
Migration failure must keep the API and worker stopped through
`service_completed_successfully`. A code rollback does not roll back database
schema changes; verify compatibility or restore from a verified backup before
rolling code back.

## Completion gate

Phase 1 is complete only when the production-contract job, backend tests,
protected-surface regression tests, build checks, and documentation checks are
green:

`PRODUCTION_CONTRACT_CI_READY`
