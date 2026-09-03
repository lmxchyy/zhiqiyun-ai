# Async generation Stage 0 operations

Stage 0 covers only `TEXT_TO_IMAGE` and `IMAGE_TO_IMAGE`. Video, PPT,
Connector commands, and all other generation types remain outside the async
canary.

## Required production migrations

The read-only preflight requires these entries in `schema_migrations`:

- `113-async-messaging-foundation.sql`
- `114-provider-execution-safety.sql`
- `115-generation-artifact-identity.sql`

PR83 does not run these migrations.

## Fail-closed selection

All of the following are required at process start:

- `ASYNC_MESSAGING_ENABLED=true`
- `GENERATION_ASYNC_CANARY_ENABLED=true`
- `PROVIDER_EXECUTION_SAFETY_ENABLED=true`
- non-empty `GENERATION_ASYNC_CANARY_USERS`
- non-empty `GENERATION_ASYNC_CANARY_PROVIDER_ALLOWLIST`
- non-empty `GENERATION_ASYNC_CANARY_MODEL_ALLOWLIST`

Start with one explicitly verified provider/model and very few internal user
IDs. A denied user, type, provider, or model continues through the existing
synchronous path; it does not fail the request. `configured` is the execution
provider identity for the runtime-configured default route.

### User eligibility and wildcard support

Setting `GENERATION_ASYNC_CANARY_USERS=*` marks all users as eligible at the
user eligibility gate.

Important constraints:
- `GENERATION_ASYNC_CANARY_USERS=*` only satisfies the user eligibility gate.
- It does **not** bypass `GENERATION_ASYNC_CANARY_ENABLED` (the kill switch).
- It does **not** bypass the provider allowlist (`GENERATION_ASYNC_CANARY_PROVIDER_ALLOWLIST`).
- It does **not** bypass the model allowlist (`GENERATION_ASYNC_CANARY_MODEL_ALLOWLIST`).
- It does **not** enable async for video, PPT, Connector commands, or any non-image workload.
- An empty string `GENERATION_ASYNC_CANARY_USERS=""` continues to fail closed (rejects all users).
- Mixed CSV lists containing `*` (e.g. `user_000002,*`) make all users eligible.
- Only the exact token `*` is recognized as the wildcard; tokens like `ALL`, `all`, or `true` are treated as literal user IDs and do not act as wildcards.

Example production configuration for full user rollout:
```env
ASYNC_MESSAGING_ENABLED=true
GENERATION_ASYNC_CANARY_ENABLED=true
GENERATION_ASYNC_CANARY_USERS=*
GENERATION_ASYNC_CANARY_PROVIDER_ALLOWLIST=configured
GENERATION_ASYNC_CANARY_MODEL_ALLOWLIST=gpt-image-2
```


## Read-only preflight

From `backend-go`:

```sh
go run ./cmd/async-canary-preflight
```

The command reads PostgreSQL connectivity and `schema_migrations`, parses and
authenticates `RABBITMQ_URL`, verifies vhost access, and uses AMQP passive
exchange/queue declarations to verify durable topology. It never publishes,
declares new topology, writes a business table, runs a migration, generates an
asset, settles points, or changes flags.

Do not enable Stage 0 unless every `PREFLIGHT_*` line is `PASS`.

## Kill switch and drain policy

### Stop new async selection, continue recovery

Set `GENERATION_ASYNC_CANARY_ENABLED=false` (or clear all canary users), restart
the API through the normal deployment procedure, and leave
`ASYNC_MESSAGING_ENABLED=true` plus `PROVIDER_EXECUTION_SAFETY_ENABLED=true`.
New requests use the synchronous path. The outbox publisher and consumer keep
running, so existing outbox events and RabbitMQ deliveries drain and durable
provider/artifact recovery continues.

### Stop publisher and consumer

Set `ASYNC_MESSAGING_ENABLED=false` and restart through the normal procedure.
The embedded publisher and consumer stop. Pending outbox rows remain in
PostgreSQL; durable queued messages remain in RabbitMQ; an interrupted unacked
delivery is requeued by RabbitMQ. Existing point reservations remain reserved
and are not blindly released. Re-enable the same PR81+ binary to resume.

Never roll back to a binary that does not understand `provider_executions`
while the outbox, queues, active executions, or unsettled reservations are
non-empty.

## Metrics and Stage 0 alerts

The existing `/metrics` endpoint exposes the `xianzhi_async_canary_*` families.
Load `ops/monitoring-minimal/async-canary-alerts.yml` into Prometheus. Defaults
assume a tiny internal cohort: DLQ, failed outbox, UNKNOWN, duplicate prevention,
and artifact recovery failures tolerate zero; pending/submitting thresholds are
300 seconds; unsettled points and application-stuck thresholds are 900 seconds.
Copy and adjust numeric values in an environment rule file only after measuring
a baseline.

Useful direct queries:

```promql
xianzhi_async_canary_outbox_pending
xianzhi_async_canary_outbox_failed
xianzhi_async_canary_rabbitmq_queue_depth
xianzhi_async_canary_rabbitmq_retry_queue_depth
xianzhi_async_canary_rabbitmq_dlq_depth
xianzhi_async_canary_provider_execution_count
xianzhi_async_canary_generation_stuck
xianzhi_async_canary_points_reserved_unsettled
rate(xianzhi_async_canary_provider_submission_attempts_total[5m])
increase(xianzhi_async_canary_artifact_recovery_failures_total[5m])
```

Decision metrics use only fixed reason labels and never include prompts, tokens,
credentials, or user IDs.
