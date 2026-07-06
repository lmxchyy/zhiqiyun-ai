# Workflows Registry

**Date**: 2026-07-01
**Scope**: current `backend-go + admin-vue + frontend-vue` checkout.
**Discovery commands used**: route scan, worker/job scan, status/state scan, migration/config scan.

## Discovery Summary

Confirmed current runtime:

- `Dockerfile` builds `frontend-vue`, `admin-vue`, then `backend-go/cmd/api`, and starts `/app/xianzhi-api`.
- `compose.yml` runs the Go app as `xianzhi-ai` on `PORT=3100`, with PostgreSQL, Redis, RabbitMQ and MinIO services available.
- Current generation entrypoints live in `backend-go/internal/httpserver/server.go`.
- The active image generation path uses Go goroutines, PostgreSQL projection tables and OpenAI-compatible providers.
- Legacy Node worker source has been removed from the current worktree; the current Docker image only starts the Go API.

Important reality gaps:

- Product docs describe `QUEUED`, `RETRYING`, `CANCELLED`, retry, cancel and refund flows. Current Go runtime exposes `PROCESSING -> SUCCEEDED/FAILED` for image generation and does not expose retry/cancel endpoints.
- Current Go runtime checks available points at enqueue and again at completion, but deducts points only on success. It does not freeze points on enqueue.
- Asset delete in current Go runtime physically deletes the `xz_assets` row and removes the asset id from the task's `resultIds`; the older product doc says logical deletion.

## Workflows

| Workflow | Spec file | Status | Trigger | Primary actor | Last reviewed |
|---|---|---|---|---|---|
| AI image generation task and artifact loop | [WORKFLOW-generation-task-artifact-loop.md](WORKFLOW-generation-task-artifact-loop.md) | Draft | `POST /api/v1/generation-tasks` | Go API + image provider | 2026-07-01 |
| Reference image upload | Missing | Missing | `POST /api/v1/reference-images` | Go API | 2026-07-01 |
| Asset list, download and delete | Covered by generation spec; split recommended | Review | `GET /api/v1/assets`, `GET /api/v1/assets/{id}/download`, `DELETE /api/v1/assets/{id}` | Go API | 2026-07-01 |
| User AI workspace state sync | Missing | Missing | `PATCH /api/v1/user/ai-state` | Go API | 2026-07-01 |
| Generation task cancellation and compensation | Missing | Missing | Product doc says `POST /api/v1/generation-tasks/{id}/cancel`; current Go route absent | User + Go API | 2026-07-01 |
| Failed generation retry | Missing | Missing | Product doc says `POST /api/v1/generation-tasks/{id}/retry`; current Go route absent | User + Go API | 2026-07-01 |
| Asset favorite and regenerate | Missing | Missing | Product doc says `POST /api/v1/assets/{id}/favorite` and `/regenerate`; current Go route absent | User + Go API | 2026-07-01 |
| PPT generation | Missing | Missing | `POST /api/v1/ppt/generate` | Go API + PPT service | 2026-07-01 |
| Legacy Node RabbitMQ generation worker | Missing | Removed | Removed from current worktree | Node server + RabbitMQ worker | 2026-07-04 |

Status values: `Approved`, `Review`, `Draft`, `Missing`, `Deprecated`.

## Components

| Component | File(s) | Workflows it participates in |
|---|---|---|
| Go route registration | `backend-go/internal/httpserver/server.go` | AI image generation task and artifact loop; Reference image upload; Asset list, download and delete; User AI workspace state sync; PPT generation |
| Generation API handlers | `backend-go/internal/httpserver/api.go` | AI image generation task and artifact loop; Reference image upload; Asset list, download and delete; User AI workspace state sync |
| Generation app service | `backend-go/internal/app/generation/service.go` | AI image generation task and artifact loop |
| OpenAI-compatible image provider | `backend-go/internal/provider/image/openai_compatible.go` | AI image generation task and artifact loop |
| Image provider router | `backend-go/internal/provider/image/router.go` | AI image generation task and artifact loop |
| PostgreSQL store | `backend-go/internal/httpserver/postgres_store.go` | AI image generation task and artifact loop; Asset list, download and delete; User AI workspace state sync |
| Runtime projection migration | `database/migrations/021-runtime-projections.sql` | AI image generation task and artifact loop; Asset list, download and delete |
| Legacy base schema | `database/schema.sql`, `database/migrations/006-generation-billing.sql`, `database/migrations/012-generation-asset-lifecycle.sql` | Deprecated or planned generation cancellation/retry/favorite workflows |
| Admin AI workspace | `admin-vue/src/App.vue` | AI image generation task and artifact loop; Reference image upload; User AI workspace state sync |
| User H5 workspace | `frontend-vue/src/pages/AiCreationPage.vue` | AI image generation task and artifact loop; Asset list/read; Points read |
| Legacy Node worker | Removed from current worktree | Legacy Node RabbitMQ generation worker; Generation task cancellation and compensation; Failed generation retry; Asset favorite and regenerate |
| Runtime config | `backend-go/internal/config/config.go`, `compose.yml`, `Dockerfile` | AI image generation task and artifact loop |

## User Journeys

| What the user/operator/system experiences | Underlying workflow(s) | Entry point |
|---|---|---|
| User submits a prompt and sees a running image task | AI image generation task and artifact loop | Admin: `admin-vue/src/App.vue` -> `POST /api/v1/generation-tasks`; H5 reads `GET /api/v1/generation-tasks` |
| User attaches reference images before generating | Reference image upload -> AI image generation task and artifact loop | `POST /api/v1/reference-images`, then `POST /api/v1/generation-tasks` with `params.referenceImages` |
| User waits for generation and sees the resulting asset | AI image generation task and artifact loop -> Asset list/download | UI polling `GET /api/v1/generation-tasks/{id}` |
| User opens works center / asset list | Asset list, download and delete | `GET /api/v1/assets` |
| User deletes a generated work | Asset list, download and delete | `DELETE /api/v1/assets/{id}` |
| User expects cancel/refund while generation is queued | Generation task cancellation and compensation | Missing in current Go runtime |
| User expects retry or regenerate | Failed generation retry; Asset favorite and regenerate | Missing in current Go runtime |
| Operator checks generation task portfolio from admin | AI image generation task and artifact loop | `GET /api/v1/admin/generation-tasks` |

## State Map

| State | Entered by | Exited by | Workflows that can trigger exit |
|---|---|---|---|
| `PROCESSING` | `CreatePendingGenerationTask` after authenticated image request passes validation and point availability check | `CompleteGenerationTask` or `FailGenerationTask` | AI image generation task and artifact loop |
| `SUCCEEDED` | `CompleteGenerationTask` after provider output is prepared, assets are inserted, points are deducted, billing and commissions are written | Terminal in current Go runtime | AI image generation task and artifact loop |
| `FAILED` | `FailGenerationTask` after provider timeout/error, completion transaction failure, or second point availability check failure | Terminal in current Go runtime | AI image generation task and artifact loop |
| `CANCELLED` | Present in Go terminal guard but no current Go route sets it | Terminal if implemented later | Generation task cancellation and compensation |
| `QUEUED` | Legacy Node workflow only | Legacy Node worker sets `PROCESSING`, retry/cancel may exit | Legacy Node RabbitMQ generation worker |
| `RETRYING` | Legacy Node workflow only | Legacy Node worker sets `PROCESSING`, final failure, success or cancel | Legacy Node RabbitMQ generation worker; Failed generation retry |
| Asset row exists in `xz_assets` | `CompleteGenerationTask` inserts asset rows | `DeleteAsset` physically deletes row | AI image generation task and artifact loop; Asset list, download and delete |
| Asset id appears in task `resultIds` | `CompleteGenerationTask` updates task result ids | `DeleteAsset` filters deleted id from `resultIds` | AI image generation task and artifact loop; Asset list, download and delete |
| Billing event `SUCCEEDED` | `generationBillingArtifactsForTx` + `insertBillingEvent` during successful completion | Terminal ledger event | AI image generation task and artifact loop |
| Audit log `generation.enqueue` | `CreatePendingGenerationTask` | Terminal audit record | AI image generation task and artifact loop |
| Audit log `generation.complete` | `CompleteGenerationTask` | Terminal audit record | AI image generation task and artifact loop |
| Audit log `generation.fail` | `FailGenerationTask` | Terminal audit record | AI image generation task and artifact loop |

## Review Backlog

| Finding | Severity | Workflow | Recommended next spec |
|---|---|---|---|
| Product docs promise cancel/retry/refund/favorite/regenerate routes that current Go runtime does not expose. | High | Generation task cancellation and compensation; Failed generation retry; Asset favorite and regenerate | `WORKFLOW-generation-task-cancel-compensation.md` |
| Go runtime does not have a persistent queue or resumable worker for in-flight image generation; goroutine work is lost if the process exits after `PROCESSING` is committed. | High | AI image generation task and artifact loop | Add recovery/timeout sweeper spec |
| Point deduction happens only on success, not freeze-on-enqueue. This is safer for failures but can reject a task at completion if points are consumed elsewhere while provider work is running. | Medium | AI image generation task and artifact loop | Add point reservation spec |
| Asset delete physically removes `xz_assets` rows, while older docs describe logical deletion. | Medium | Asset list, download and delete | Add asset lifecycle spec |
| Legacy Node worker contains richer retry/cancel/queue behavior but is not started by the current Docker image. | Medium | Legacy Node RabbitMQ generation worker | Decide deprecate vs port behavior to Go |
