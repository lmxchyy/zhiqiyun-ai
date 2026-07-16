# Workflows Registry

> Current registry: 2026-07-15. The 2026-07-01/10 content below is retained as a historical discovery snapshot; where rows conflict, this current section is authoritative.

## Current Discovery Scope

- Runtime: `backend-go + admin-vue + apps/user-uni + PostgreSQL + Redis`; RabbitMQ is deployed but none of the seven reviewed workflows consumes it.
- Entry points scanned: `backend-go/internal/httpserver/server.go`, user/admin API clients, page actions, callback endpoints.
- Worker/async scan: in-process generation goroutines, PPT slide-image goroutines, WeChat compensation loop, synchronous knowledge ingestion/RAG. No durable worker was found for these seven workflows.
- State scan: Go structs, status comparisons, database constraints and transition writes.
- Data/config scan: migrations `006`, `012`, `021`, `033`-`036`, `040`-`044`, `047`-`050`; `config.go`, `.env*`, `compose*.yml`.
- Reality boundary: payment and billing files/migrations are currently untracked or modified; this registry describes the working tree, not only `HEAD`.

## Current Workflows

| Workflow | Spec file | Status | Trigger | Primary actor | Last reviewed |
|---|---|---|---|---|---|
| WeChat virtual payment and entitlement grant | [WORKFLOW-wechat-virtual-payment.md](WORKFLOW-wechat-virtual-payment.md) | Draft | `POST /api/v1/payment/wechat-virtual/orders`; WeChat notify/query | Mini program + Go API + WeChat | 2026-07-15 |
| Billing Center V1 rule and ledger governance | [WORKFLOW-billing-center-v1.md](WORKFLOW-billing-center-v1.md) | Draft | Admin billing pages and generation billing lifecycle | Admin + Go API + PostgreSQL | 2026-07-15 |
| Commercial billing synchronized with WeChat | [WORKFLOW-commercial-billing-wechat.md](WORKFLOW-commercial-billing-wechat.md) | Draft | Virtual-order create/pay/refund plus admin billing actions | Go API + Admin | 2026-07-15 |
| PPT generation and slide-image enrichment | [WORKFLOW-ppt-generation.md](WORKFLOW-ppt-generation.md) | Draft | `POST /api/v1/ppt/generate` | User + Go API + model providers | 2026-07-15 |
| AI image generation, billing and artifact lifecycle | [WORKFLOW-ai-image-generation.md](WORKFLOW-ai-image-generation.md) | Draft | `POST /api/v1/generation-tasks` | User + Go API + image provider | 2026-07-15 |
| Enterprise center lifecycle and context switch | [WORKFLOW-enterprise-center-lifecycle.md](WORKFLOW-enterprise-center-lifecycle.md) | Draft | Enterprise create/invite/join/switch APIs | User + enterprise store | 2026-07-15 |
| Knowledge RAG ingestion and conversational run | [WORKFLOW-knowledge-rag.md](WORKFLOW-knowledge-rag.md) | Draft | Document ingest and conversation run/stream APIs | User + knowledge services | 2026-07-15 |
| Legacy generation task/artifact snapshot | [WORKFLOW-generation-task-artifact-loop.md](WORKFLOW-generation-task-artifact-loop.md) | Deprecated | Historical 2026-07-10 image workflow | Go API | 2026-07-15 |
| WeChat refund initiation | Missing | Missing | No official refund-initiation API found; only refund notification/query sync | Operator + WeChat | 2026-07-15 |
| Billing reconciliation repair | Missing | Missing | Reconciliation is read-only; no repair/close endpoint | Operator | 2026-07-15 |
| Commercial subscription renewal scheduler | Missing | Missing | No expiry/renewal scheduler found | Scheduler | 2026-07-15 |
| PPT durable task worker | Missing | Missing | Current task state is time-materialized from local persistence | Worker | 2026-07-15 |
| Enterprise certification review (user-side lifecycle) | Missing | Missing | Submit exists; review is only exposed in admin enterprise actions | Admin | 2026-07-15 |
| Knowledge ingestion retry/resume worker | Missing | Missing | Ingestion is synchronous; job fields contain attempts but no worker consumes them | Worker | 2026-07-15 |

## Current Components

| Component | File(s) | Workflows it participates in |
|---|---|---|
| Route registry and admin RBAC | `backend-go/internal/httpserver/server.go`, `governance.go` | All seven workflows |
| Virtual payment API, callback and compensation loop | `wechat_virtual_payment.go`, `wechat_virtual_entitlements.go`, `wechat_virtual_admin.go` | WeChat virtual payment; Commercial billing |
| Payment UI/API client | `apps/user-uni/src/features/payment/*`, `UserVirtualPaymentPage.vue` | WeChat virtual payment |
| Billing V1 stores and UI | `billing_v1_*.go`, `BillingCenterV1.vue`, `admin-vue/src/api/billing.ts` | Billing Center V1; AI image generation; PPT; RAG |
| Commercial billing projection/admin | `commercial_billing*.go`, `CommercialBillingCenter.vue` | Commercial billing; WeChat virtual payment |
| PPT service/API/UI | `internal/app/ppt/service.go`, `ppt_api.go`, `ppt_export.go`, PPT stores/components | PPT generation |
| Generation runtime and asset center | `api.go`, `asset_center_api.go`, `postgres_store.go`, image providers | AI image generation; PPT slide images |
| Enterprise APIs/stores/UI | `enterprise_*.go`, `admin_enterprise_*.go`, `apps/user-uni/src/features/enterprise/*` | Enterprise center |
| Knowledge ingestion/RAG/repositories/UI | `internal/app/knowledge/*`, `knowledge_api.go`, knowledge API clients/components | Knowledge RAG |
| Runtime configuration and orchestration | `config.go`, `.env*`, `compose*.yml` | All seven workflows |
| Database lifecycle | `database/migrations/006`, `012`, `021`, `033`-`036`, `040`-`044`, `047`-`050` | All seven workflows |

## Current User Journeys

| What the user/operator/system experiences | Underlying workflow(s) | Entry point |
|---|---|---|
| User buys a server-priced virtual product and waits for membership/credits/image quota | WeChat virtual payment -> Commercial billing projection | Mini-program virtual payment page |
| Operator publishes a billing rule and investigates task/wallet anomalies | Billing Center V1 | Admin billing modules |
| Operator manages coupons, subscriptions, invoices, credit notes and dunning | Commercial billing | Admin commercial billing modules |
| User generates an outline/deck, waits for optional per-slide AI images, then exports | PPT generation -> AI image generation | Admin or mini-program PPT workspace |
| User submits, polls, cancels or retries an image task and receives an asset | AI image generation | AI creation / asset center |
| User creates or joins an enterprise and switches tenant/organization/role context | Enterprise center | `我的` -> enterprise page stack |
| User uploads documents, binds an agent, streams an answer and opens citations | Knowledge RAG | Knowledge agent center / mini chat |
| System repairs stale virtual orders | WeChat virtual payment | 20s startup delay, then every 2 minutes |
| System repairs stale image tasks | AI image generation | API startup repair |

## Current State Map

| Entity/state | Entered by | Exited by | Workflow(s) |
|---|---|---|---|
| Virtual order `PENDING` | Local order/prepay transaction | `PAID`, `CLOSED` | WeChat virtual payment |
| Entitlement `PENDING/PROCESSING` | Order creation/payment confirmation | `SUCCESS`, `FAILED` | WeChat virtual payment |
| Virtual order `REFUND_PENDING/REFUNDED` | Refund callback or query sync | terminal/repair | WeChat virtual payment; Commercial billing |
| Billing rule `DRAFT` | New version | `PUBLISHED`; prior published -> `ARCHIVED` | Billing Center V1 |
| Billing task `CREATED/QUEUED/RUNNING` | Task admission/execution | `SUCCEEDED/FAILED/CANCELLED` | Billing Center V1; AI image generation |
| Billing `UNQUOTED/QUOTED/RESERVED` | Quote/reservation | `CAPTURED/RELEASED/REFUNDED/BILLING_FAILED` | Billing Center V1 |
| Coupon redemption `RESERVED` | Order transaction | `APPLIED/CANCELLED` | Commercial billing |
| Invoice/payment request `FINALIZED/PENDING` | Virtual-order transaction | `PAID/SUCCEEDED`, `CREDITED/REFUNDED`, `CANCELLED` | Commercial billing |
| PPT task `pending/processing` | Local persistent service | `success/failed` (failure is not reached by current materializer) | PPT generation |
| Image task `PROCESSING` + billing `RESERVED` | Pending-task transaction | `SUCCEEDED/CAPTURED`, `FAILED/RELEASED`, `CANCELLED/RELEASED` | AI image generation |
| Enterprise member `ACTIVE` | Create/invite/join approval | `DISABLED/REMOVED` | Enterprise center |
| Enterprise invite/join request `PENDING` | Create request | `ACCEPTED/APPROVED/REJECTED/EXPIRED` | Enterprise center |
| Knowledge document/job `INDEXING/RUNNING` | Ingestion bundle transaction | `READY`, `FAILED` | Knowledge RAG |
| RAG run `QUEUED` | User message + run creation | `RETRIEVING -> GENERATING -> COMPLETED`, `FAILED`, `CANCELLED` | Knowledge RAG |

## Current Review Backlog

| Finding | Severity | Workflow | Required next action/spec |
|---|---|---|---|
| Payment compensation is an in-process infinite loop without shutdown hook, distributed leader election or surfaced errors. | High | WeChat virtual payment | Define durable scheduler/lease and alerting contract |
| Billing reconciliation detects anomalies but provides no repair/acknowledge workflow. | High | Billing Center V1 | Add reconciliation repair workflow |
| Commercial credit-note approval explicitly does not initiate a WeChat refund. | High | Commercial billing | Add official refund-initiation and entitlement reversal workflow |
| PPT core task succeeds after elapsed time and slide image failures do not fail the deck. | High | PPT generation | Replace materializer with durable task state and page-level failure policy |
| Image task cancellation writes `FAILED` settlement first, then changes only the top-level status to `CANCELLED`; task/billing snapshots can diverge. | High | AI image generation | Make cancel one atomic transition |
| Enterprise organization writes and direct audit writes are not consistently in one transaction. | Medium | Enterprise center | Define atomic mutation+audit contract |
| Knowledge ingestion and RAG execute inside request contexts; attempt fields are not backed by a durable worker. | High | Knowledge RAG | Add resumable ingestion/run worker workflow |

---

## Historical Discovery Snapshot (2026-07-01 to 2026-07-10)

**Date**: 2026-07-01
**Scope**: current `backend-go + admin-vue + apps/user-uni` checkout.
**Discovery commands used**: route scan, worker/job scan, status/state scan, migration/config scan.

## Discovery Summary

Confirmed current runtime:

- `Dockerfile` builds `apps/user-uni`, `admin-vue`, then `backend-go/cmd/api`, and starts `/app/xianzhi-api`.
- `compose.yml` runs the Go app as `xianzhi-ai` on `PORT=3100`, with PostgreSQL, Redis, RabbitMQ and MinIO services available.
- Current generation entrypoints live in `backend-go/internal/httpserver/server.go`.
- The active image generation path uses Go goroutines, PostgreSQL projection tables and OpenAI-compatible providers.
- Legacy Node worker source has been removed from the current worktree; the current Docker image only starts the Go API.

Important reality gaps:

- Product docs describe `QUEUED`, `RETRYING`, `CANCELLED`, retry, cancel and refund flows. Current Go runtime exposes `PROCESSING -> SUCCEEDED/FAILED` for image generation and does not expose retry/cancel endpoints.
- Current Go runtime reserves available points at enqueue, settles the reservation on success, and refunds the active reservation on failure. It does not yet expose a separate hold/refund ledger.
- Asset delete in current Go runtime now writes `xz_assets.deleted_at`, removes the asset id from the task's `resultIds`, and filters deleted assets from user/channel/admin active views.

## Workflows

| Workflow | Spec file | Status | Trigger | Primary actor | Last reviewed |
|---|---|---|---|---|---|
| AI image generation task and artifact loop | [WORKFLOW-generation-task-artifact-loop.md](WORKFLOW-generation-task-artifact-loop.md) | Draft | `POST /api/v1/generation-tasks` | Go API + image provider | 2026-07-10 |
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
| User H5 workspace | `apps/user-uni/src/pages/AiCreationPage.vue` | AI image generation task and artifact loop; Asset list/read; Points read |
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
| `PROCESSING` | `CreatePendingGenerationTask` after authenticated image request passes validation, point availability check and point reservation | `CompleteGenerationTask` or `FailGenerationTask` | AI image generation task and artifact loop |
| `SUCCEEDED` | `CompleteGenerationTask` after provider output is prepared, assets are inserted, the point reservation is settled, billing and commissions are written | Terminal in current Go runtime | AI image generation task and artifact loop |
| `FAILED` | `FailGenerationTask` after provider timeout/error or completion transaction failure; active point reservation is refunded | Terminal in current Go runtime | AI image generation task and artifact loop |
| `CANCELLED` | Present in Go terminal guard but no current Go route sets it | Terminal if implemented later | Generation task cancellation and compensation |
| `QUEUED` | Legacy Node workflow only | Legacy Node worker sets `PROCESSING`, retry/cancel may exit | Legacy Node RabbitMQ generation worker |
| `RETRYING` | Legacy Node workflow only | Legacy Node worker sets `PROCESSING`, final failure, success or cancel | Legacy Node RabbitMQ generation worker; Failed generation retry |
| Active asset row exists in `xz_assets` | `CompleteGenerationTask` inserts asset rows with `deleted_at` empty | `DeleteAsset` writes `deleted_at` and active queries filter the row | AI image generation task and artifact loop; Asset list, download and delete |
| Asset id appears in task `resultIds` | `CompleteGenerationTask` updates task result ids | `DeleteAsset` filters deleted id from `resultIds` | AI image generation task and artifact loop; Asset list, download and delete |
| Billing event `SUCCEEDED` | `generationBillingArtifactsForTx` + `insertBillingEvent` during successful completion | Terminal ledger event | AI image generation task and artifact loop |
| Audit log `generation.enqueue` | `CreatePendingGenerationTask` | Terminal audit record | AI image generation task and artifact loop |
| Audit log `generation.complete` | `CompleteGenerationTask` | Terminal audit record | AI image generation task and artifact loop |
| Audit log `generation.fail` | `FailGenerationTask` | Terminal audit record | AI image generation task and artifact loop |

## Review Backlog

| Finding | Severity | Workflow | Recommended next spec |
|---|---|---|---|
| Product docs promise cancel/retry/refund/favorite/regenerate routes that current Go runtime does not expose. | High | Generation task cancellation and compensation; Failed generation retry; Asset favorite and regenerate | `WORKFLOW-generation-task-cancel-compensation.md` |
| Go runtime does not have a persistent queue or resumable worker for in-flight image generation; startup repair fails stale `PROCESSING` tasks after max age, but provider work is not resumed. | Medium | AI image generation task and artifact loop | Add durable queue/resume spec if retry continuity is required |
| Point reservation is implemented in task params and point-account balance, but there is no separate hold/refund ledger event for finance reporting. | Low | AI image generation task and artifact loop | Add explicit point hold/refund ledger spec if required |
| Asset delete now preserves `xz_assets` rows with `deleted_at`; a dedicated asset lifecycle spec is still useful for restore/permanent-purge policy. | Low | Asset list, download and delete | Add asset lifecycle spec |
| Legacy Node worker contains richer retry/cancel/queue behavior but is not started by the current Docker image. | Medium | Legacy Node RabbitMQ generation worker | Decide deprecate vs port behavior to Go |
