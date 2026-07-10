# WORKFLOW: AI Image Generation Task and Artifact Loop

**Version**: 0.1
**Date**: 2026-07-01
**Author**: Workflow Architect
**Status**: Draft
**Implements**: Current Go runtime behavior for authenticated image generation, task polling, asset creation, point reservation, billing and audit.

## Overview

This workflow describes the current code-backed path for creating an AI image generation task and turning provider output into user-visible assets. It is based on the active `backend-go` service started by `Dockerfile` and `compose.yml`.

The user did not specify a concrete `[功能名]`, so this spec selects the most central discovered workflow: generation task -> provider execution -> asset/works output.

## Evidence Map

| Evidence | File |
|---|---|
| Current Docker image starts Go binary `/app/xianzhi-api` | `Dockerfile` |
| Runtime dependencies and model-provider env vars | `compose.yml`, `backend-go/internal/config/config.go` |
| API routes for generation tasks, assets, points and user online image data | `backend-go/internal/httpserver/server.go` |
| Generation task handler and goroutine execution | `backend-go/internal/httpserver/api.go` |
| Image/video/chat capability service | `backend-go/internal/app/generation/service.go` |
| OpenAI-compatible image provider, timeout and retry behavior | `backend-go/internal/provider/image/openai_compatible.go` |
| PostgreSQL task, asset, billing, commission and audit writes | `backend-go/internal/httpserver/postgres_store.go` |
| Runtime projection tables | `database/migrations/021-runtime-projections.sql` |
| Admin UI submit, optimistic task and polling | `admin-vue/src/App.vue` |
| User H5 task/asset/points reads | `apps/user-uni/src/pages/AiCreationPage.vue` |
| Older product expectations and legacy gaps | `生成任务与作品闭环实施说明.md`, `生成任务取消与补偿实施说明.md`, `先知AI平台开发文档.md` |

## Actors

| Actor | Role in this workflow |
|---|---|
| Authenticated user | Submits prompt/reference images and owns tasks/assets/points. |
| Admin AI workspace | Primary current UI for prompt submission, optimistic task display and polling. |
| User H5 workspace | Reads tasks, assets, models and point account. |
| Go HTTP API | Authenticates request, selects provider route, creates task, starts generation goroutine and serves polling endpoints. |
| Auth session store | Resolves the authenticated user for API calls. |
| PostgreSQL store | Persists tasks, assets, point accounts, billing events, commissions and audit logs. |
| Generation service | Normalizes generation capability and delegates to image provider. |
| Image provider router | Chooses an enabled OpenAI-compatible provider. |
| OpenAI-compatible image provider | Calls image generation/edit/responses endpoints and decodes returned images. |
| API channel / user model route | Determines model/provider/base URL/API key when configured in admin SaaS. |
| Billing and commission subsystem | Writes usage billing event and optional commission records after success. |
| Audit subsystem | Writes enqueue, complete, fail and asset delete audit records. |

## Prerequisites

- User is authenticated; otherwise generation and polling return unauthorized errors.
- Prompt is non-empty after trimming.
- Request type is `TEXT_TO_IMAGE` or `IMAGE_TO_IMAGE` for this workflow.
- For non-mock image generation, a usable API channel or runtime provider must exist with a configured API key.
- User has enough available points at enqueue time; current Go runtime reserves the task point cost during enqueue and releases the reservation on task failure.
- PostgreSQL projection schema exists or can be ensured by `postgresStore.ensureReady`.
- For image-to-image, `params.referenceImages` contains at least one accessible data URL or HTTP(S) image URL.
- Current process stays alive until the goroutine completes. If the process exits, the next API startup runs stale-task repair after the task exceeds the configured age threshold, but the current Go path still does not resume provider work from a durable queue.

## Trigger

- Entry point: `POST /api/v1/generation-tasks`
- Initiating action/event:
  - Admin online image panel calls `/generation-tasks` with `type: "TEXT_TO_IMAGE"`.
  - Admin AI image workspace calls `/generation-tasks` with `TEXT_TO_IMAGE` or `IMAGE_TO_IMAGE`, including prompt, model, provider, image count, size, quality and reference image snapshot.

## Workflow Tree

### STEP 1: Submit Image Generation Request

**Actor**: Admin AI workspace
**Action**: User enters a prompt, optional reference images and generation parameters; UI calls `POST /api/v1/generation-tasks`.
**Timeout**: Browser request timeout is not explicit in `admin-vue`; user sees request failure if network/API fails.
**Input**:

```json
{
  "type": "TEXT_TO_IMAGE or IMAGE_TO_IMAGE",
  "prompt": "string",
  "model": "string",
  "params": {
    "count": "1..8",
    "imageRatio": "1:1 | 3:4 | 4:3 | 9:16 | 16:9",
    "imageQuality": "auto | high | medium | low | standard",
    "provider": "optional channel id",
    "referenceImages": "optional array"
  }
}
```

**Output on SUCCESS**: HTTP response with task object, usually `status: "PROCESSING"` and `progress: 5`; server-side goroutine proceeds to STEP 5 while UI proceeds to STEP 7.

**Output on FAILURE**:

- `FAILURE(validation_error)`: prompt is empty in UI -> show `请输入生图提示词`; no API call.
- `FAILURE(reference_not_ready)`: user had reference images but none are serializable to remote/data URL -> show `参考图还没有准备好，请稍后重试`; no API call.
- `FAILURE(network_or_api_error)`: `adminRequest` throws -> show backend/client error; no task is tracked unless an optimistic task already exists.
- `FAILURE(duplicate_submit)`: current Go API has no idempotency key handling for this endpoint -> duplicate user clicks can create multiple tasks if the UI does not prevent it.
- `FAILURE(timeout)`: client waits too long or request is interrupted -> user sees submit failure; backend may still have created a task if the request reached the server.

**Recovery**:

- User can resubmit after fixing prompt/reference images.
- Operator can inspect `xz_generation_tasks` and audit logs if the client timed out but backend may have accepted the request.

**Observable states during this step**:

- Customer sees: submit loading state and then optimistic/running task or error message.
- Operator sees: no backend state until API accepts request.
- Database: no change until STEP 4.
- Logs/metrics/traces: frontend console warning/error only unless API receives the request.

### STEP 2: Authenticate and Validate API Request

**Actor**: Go HTTP API
**Action**: Resolve authenticated user, decode JSON, trim prompt, default missing type to `TEXT_TO_IMAGE`, initialize `params`.
**Timeout**: Store calls use a 5 second PostgreSQL timeout through `postgresStore.withTimeout`; route-level request timeout is otherwise not explicit.
**Input**: HTTP request body from STEP 1 plus auth cookie/token.
**Output on SUCCESS**: normalized `generation.CreateRequest` with server-side `UserID` -> GO TO STEP 3.

**Output on FAILURE**:

- `FAILURE(auth_required)`: authenticated user cannot be resolved -> HTTP 401.
- `FAILURE(json_decode_error)`: invalid JSON body -> HTTP 400.
- `FAILURE(validation_error)`: prompt is empty -> HTTP 400 `prompt is required`.
- `FAILURE(store_timeout)`: auth/AdminData read times out -> HTTP 500 or 401 depending failure point.

**Recovery**:

- User logs in again for auth failure.
- Client resubmits valid JSON and prompt.
- Operator verifies PostgreSQL readiness when store reads fail.

**Observable states during this step**:

- Customer sees: submit failure message.
- Operator sees: HTTP status and audit middleware entry if available.
- Database: no generation task row.
- Logs/metrics/traces: Gin recovery/audit middleware and HTTP error response.

### STEP 3: Select Generation Service and Provider Channel

**Actor**: Go HTTP API
**Action**: Choose the service in this priority order:

1. User model route if active and compatible with requested model.
2. Explicit `params.provider` channel id, excluding `channel_runtime_env`.
3. Configured API channel supporting requested model.
4. Default generation service from runtime config.

**Timeout**: AdminData/provider channel reads use store timeout; provider HTTP is not called yet.
**Input**: normalized generation request and authenticated user.
**Output on SUCCESS**: `generation.Service` bound to image provider/API key/model -> GO TO STEP 4.

**Output on FAILURE**:

- `FAILURE(provider_not_found)`: explicit provider id does not match an API channel -> HTTP 400.
- `FAILURE(provider_disabled)`: channel status is not active/configurable/enabled -> HTTP 400.
- `FAILURE(provider_missing_key)`: no saved API key and no configured key env var -> HTTP 400.
- `FAILURE(model_not_configured)`: requested model has no usable configured channel -> HTTP 400.
- `FAILURE(route_key_missing)`: user model route has no active API key -> HTTP 400.
- `FAILURE(store_timeout)`: AdminData read exceeds store timeout -> HTTP error.

**Recovery**:

- Operator enables channel, saves API key or configures `MODEL_PROVIDER_API_KEY`/`OPENAI_API_KEY`.
- User selects another provider/model.
- For route-specific failures, operator fixes user model route and API key binding.

**Observable states during this step**:

- Customer sees: submit failure with provider/model/key error.
- Operator sees: API channel config and model route status in admin SaaS.
- Database: no generation task row.
- Logs/metrics/traces: HTTP 400 with error message; no generation audit record yet.

### STEP 4: Enqueue Current Go Image Task

**Actor**: PostgreSQL store
**Action**: `CreatePendingGenerationTask` opens a transaction, locks point account, checks available points, reserves the task point cost, creates `xz_generation_tasks` row with `billingReserved` reservation metadata, writes audit log `generation.enqueue`, commits.
**Timeout**: 5 second store timeout.
**Input**: image generation request.
**Output on SUCCESS**:

```json
{
  "status": "PROCESSING",
  "progress": 5,
  "pointCost": "imageCount(params) * modelPointCost(model)",
  "resultIds": []
}
```

-> GO TO STEP 5.

**Output on FAILURE**:

- `FAILURE(insufficient_points)`: available points less than projected point cost -> HTTP 400; no task row committed.
- `FAILURE(database_error)`: schema/ID/insert/audit/commit error -> HTTP 400 in current handler for this store call.
- `FAILURE(concurrent_account_lock_delay)`: point account lock cannot complete before timeout -> task is not committed.

**Recovery**:

- User recharges points or chooses cheaper/smaller generation.
- Operator checks PostgreSQL health and audit table availability.
- Client can resubmit; current Go flow does not use idempotency to dedupe.

**Observable states during this step**:

- Customer sees: newly created running task with 5% progress.
- Operator sees: `xz_generation_tasks.status = PROCESSING`, `progress = 5`.
- Database: one task row in `xz_generation_tasks`; point account available balance reduced by the reserved point cost; task params record reservation amount and before/after balances; audit log `generation.enqueue`; no assets or billing event yet.
- Logs/metrics/traces: API returns task object; audit middleware may also log request.

### STEP 5: Run Provider Generation in Goroutine

**Actor**: Go API goroutine + generation service + image provider
**Action**: `runGenerationTask` creates a background context with 10 minute timeout and calls `service.PrepareImageTask`.
**Timeout**:

- Outer task execution timeout: 10 minutes.
- Provider HTTP client timeout: `MODEL_PROVIDER_TIMEOUT_MS` when set; compose default is `600000` ms.
- Image-to-image edit timeout: 150 seconds.
- Image-to-image fallback generation timeout: 90 seconds.
- JSON generation retry: up to 3 attempts for HTTP 502/503/504 or network timeout, with 700 ms then 1400 ms sleep before later attempts.

**Input**: pending task id, selected generation service, generation request.
**Output on SUCCESS**: request enriched with `GeneratedImages` -> GO TO STEP 6.

**Output on FAILURE**:

- `FAILURE(unsupported_type)`: non-image capability not supported by selected service -> `FailGenerationTask`.
- `FAILURE(provider_disabled_or_no_provider)`: provider missing base URL/API key or no provider supports model -> `FailGenerationTask`.
- `FAILURE(image_to_image_missing_reference)`: `IMAGE_TO_IMAGE` lacks valid `referenceImages` -> `FailGenerationTask`.
- `FAILURE(reference_fetch_error)`: HTTP/data URL reference cannot be loaded/decoded -> `FailGenerationTask`.
- `FAILURE(provider_timeout)`: provider or outer context times out -> `FailGenerationTask` with `生成超时，请稍后重试`.
- `FAILURE(transient_provider_error)`: 502/503/504 or network timeout persists after retries -> `FailGenerationTask`.
- `FAILURE(permanent_provider_error)`: provider returns non-retryable HTTP error, empty payload or invalid JSON -> `FailGenerationTask`.
- `FAILURE(process_exit)`: Go process exits after STEP 4 but before completion/failure write -> task can remain `PROCESSING` until the next API startup stale-task repair marks it `FAILED` and refunds any active point reservation.

**Recovery**:

- User can resubmit manually after stale-task repair marks the old task failed; current Go route does not expose retry by task id.
- Operator fixes provider config/key/quota/model support.
- Operator can restart the API to trigger startup stale-task repair, or manually inspect stuck `PROCESSING` rows if failure settlement also fails.

**Observable states during this step**:

- Customer sees: running task during polling.
- Operator sees: task stuck in `PROCESSING` until provider returns or failure is written.
- Database: no asset or billing event changes during provider call; the point reservation remains held until success or failure settlement.
- Logs/metrics/traces: provider error only appears in task error after failure; in-code goroutine ignores returned failure-update errors.

### STEP 6: Complete Task, Create Assets and Settle Points

**Actor**: PostgreSQL store
**Action**: `CompleteGenerationTask` locks the task and point account, verifies the task is not terminal, creates asset rows, updates task to `SUCCEEDED`, settles the existing point reservation into a billing event, writes optional commissions and audit log `generation.complete`. Legacy unreserved tasks still use the success-time deduction fallback.
**Timeout**: 5 second store timeout.
**Input**: task id and prepared request containing provider-generated images.
**Output on SUCCESS**:

```json
{
  "status": "SUCCEEDED",
  "progress": 100,
  "resultIds": ["asset_..."],
  "workerFinishedAt": "timestamp"
}
```

-> GO TO STEP 7.

**Output on FAILURE**:

- `FAILURE(task_not_found)`: task row cannot be locked -> goroutine calls `FailGenerationTask`, which may also fail if row is missing.
- `FAILURE(terminal_race)`: task already `SUCCEEDED`, `FAILED` or `CANCELLED` -> no-op commit; current Go flow has no cancel route, but guard exists.
- `FAILURE(asset_insert_error)`: generated asset cannot be inserted -> transaction rolls back; goroutine marks task `FAILED`.
- `FAILURE(point_update_error)`: point account update fails for a legacy unreserved task -> transaction rolls back; goroutine marks task `FAILED`.
- `FAILURE(billing_or_commission_error)`: billing event or commission write fails -> transaction rolls back; goroutine marks task `FAILED`.
- `FAILURE(audit_error)`: complete audit write fails -> transaction rolls back; goroutine marks task `FAILED`.
- `FAILURE(fail_write_error)`: if the fallback `FailGenerationTask` also fails, task may remain `PROCESSING`; current code discards that error.

**Recovery**:

- User can resubmit after the failed task refunds its active reservation or after adding points.
- Operator checks asset/billing/commission/audit table health.
- Operator repairs stuck `PROCESSING` rows if completion and failure writes both failed.

**Observable states during this step**:

- Customer sees: completed task and asset after next poll or module refresh.
- Operator sees: task `SUCCEEDED`, asset row(s), point balance already reduced by the enqueue reservation, billing event `SUCCEEDED`, possible commission rows.
- Database: `xz_generation_tasks`, `xz_assets`, `xz_point_accounts`, `xz_billing_events`, `xz_commissions`, `xz_audit_logs`.
- Logs/metrics/traces: audit log `generation.complete`.

### STEP 7: Poll Task and Refresh Workspace

**Actor**: Admin AI workspace
**Action**: Track returned task id, poll `GET /api/v1/generation-tasks/{id}`, merge server task into local store, stop tracking terminal tasks, refresh workspace when a task completes.
**Timeout**:

- Poll delays: 2s, 3s, 5s, 8s, 13s, then 20s.
- Max poll attempts: 90 per tracked task, roughly 29 minutes worst case before the UI stops tracking.

**Input**: task id from STEP 4.
**Output on SUCCESS**: task becomes non-running (`SUCCEEDED` or `FAILED`), UI refreshes data -> GO TO STEP 8 for asset use if successful.

**Output on FAILURE**:

- `FAILURE(poll_unauthorized)`: session expires -> poll request fails; task may still complete server-side.
- `FAILURE(poll_not_found)`: task id absent or belongs to another user -> task is skipped.
- `FAILURE(poll_network_error)`: warning logged and polling continues until max attempts.
- `FAILURE(max_attempts_exceeded)`: UI stops tracking; task may remain `PROCESSING` or may later complete without automatic UI update.

**Recovery**:

- User refreshes workspace or logs in again.
- Operator checks whether task is stuck in DB or merely UI polling stopped.

**Observable states during this step**:

- Customer sees: running, success or failed task card; on success asset thumbnail/result URL appears through `attachAssetImagesToTasks`.
- Operator sees: GET traffic and task state changes.
- Database: read-only polling.
- Logs/metrics/traces: frontend `console.warn` for skipped polls.

### STEP 8: Serve Asset List, Download and Delete

**Actor**: Go HTTP API + PostgreSQL store
**Action**: User fetches assets, downloads image/video URL/data URL, or deletes an asset.
**Timeout**:

- Store reads/deletes use 5 second timeout.
- Remote asset download uses request context and reads up to 512 MiB.

**Input**:

- `GET /api/v1/assets`
- `GET /api/v1/assets/{id}/download`
- `DELETE /api/v1/assets/{id}`

**Output on SUCCESS**:

- Asset list returns only current user's assets.
- Download streams data URL content or remote URL content with attachment headers.
- Delete removes asset row and removes the asset id from the source task's `resultIds`.

**Output on FAILURE**:

- `FAILURE(auth_required)`: unauthorized -> HTTP 401.
- `FAILURE(asset_not_found)`: asset absent or not owned by user -> HTTP 404.
- `FAILURE(remote_download_error)`: remote image URL fetch returns non-2xx or network error -> HTTP 502.
- `FAILURE(delete_tx_error)`: task/result id update, asset delete or audit write fails -> HTTP 500.

**Recovery**:

- User refreshes asset list or retries download.
- Operator checks whether asset URL is a remote provider URL that expired.
- Operator can reconcile task `resultIds` if delete partially fails, though current delete is transactional.

**Observable states during this step**:

- Customer sees: works list, downloaded file or deleted item disappearing from list.
- Operator sees: `xz_assets.deleted_at` set on delete; task raw `resultIds` updated.
- Database: `xz_assets`, `xz_generation_tasks.raw.resultIds`, audit `assets.delete`.
- Logs/metrics/traces: HTTP download/delete response and audit log for delete.

## State Transitions

```text
Current Go image path:

[no_task]
  -> POST /api/v1/generation-tasks accepted
  -> [PROCESSING progress=5]
  -> provider succeeds + completion transaction succeeds
  -> [SUCCEEDED progress=100 resultIds=[asset...]]

[PROCESSING]
  -> provider fails / provider timeout / completion transaction fails
  -> [FAILED progress=100 error.message]

[PROCESSING]
  -> Go process exits before completion/failure write
  -> [PROCESSING stale]
  -> next API startup repair after max age
  -> [FAILED]

[SUCCEEDED resultIds=[asset_1]]
  -> DELETE /api/v1/assets/asset_1
  -> [SUCCEEDED resultIds=[]] + asset deleted_at set
```

```text
Documented but not current Go path:

[QUEUED] -> [PROCESSING] -> [RETRYING] -> [PROCESSING]
[QUEUED or RETRYING] -> [CANCELLED]
[FAILED] -> retry endpoint -> [QUEUED]
```

## Handoff Contracts

### Admin UI -> Go API

**Endpoint/Event/Command**: `POST /api/v1/generation-tasks`

**Payload**:

```json
{
  "type": "TEXT_TO_IMAGE",
  "prompt": "string",
  "model": "gpt-image-2",
  "params": {
    "count": 1,
    "imageRatio": "1:1",
    "imageQuality": "high",
    "provider": "channel id",
    "sourceModule": "ai-image"
  }
}
```

**Success response**:

```json
{
  "id": "task_000001",
  "userId": "user_...",
  "type": "TEXT_TO_IMAGE",
  "prompt": "string",
  "model": "gpt-image-2",
  "status": "PROCESSING",
  "progress": 5,
  "pointCost": 10,
  "resultIds": [],
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Failure response**:

```json
{
  "code": "400 or 401 or 502",
  "message": "prompt is required / provider error / auth error",
  "data": null
}
```

**Timeout**: No explicit UI timeout; server-side store calls 5s; provider work happens after the response in a goroutine.
**On failure**: UI shows error; no retry automation in current Go path.

### Go API -> PostgreSQL Store: Create Pending Task

**Endpoint/Event/Command**: `CreatePendingGenerationTask(req)`

**Payload**:

```json
{
  "userId": "server injected",
  "type": "TEXT_TO_IMAGE",
  "prompt": "string",
  "model": "string",
  "params": "map"
}
```

**Success response**:

```json
{
  "id": "task_...",
  "status": "PROCESSING",
  "progress": 5,
  "pointCost": "computed",
  "resultIds": []
}
```

**Failure response**:

```json
{
  "ok": false,
  "error": "insufficient remaining points or database error",
  "retryable": false
}
```

**Timeout**: 5s.
**On failure**: request returns HTTP 400 in current handler.

### Go Goroutine -> Image Provider

**Endpoint/Event/Command**:

- `POST {baseURL}/v1/images/generations`
- `POST {baseURL}/v1/images/edits`
- `POST {baseURL}/v1/responses` for selected edit mode

**Payload**:

```json
{
  "model": "gpt-image-2",
  "prompt": "string",
  "n": 1,
  "size": "1024x1024",
  "response_format": "b64_json"
}
```

**Success response**:

```json
{
  "data": [
    {
      "url": "https://...",
      "b64_json": "optional base64"
    }
  ]
}
```

**Failure response**:

```json
{
  "ok": false,
  "error": "provider HTTP status, invalid response, timeout or empty payload",
  "retryable": "true for 502/503/504 or network timeout within doJSONWithRetry"
}
```

**Timeout**:

- Provider HTTP client timeout from `MODEL_PROVIDER_TIMEOUT_MS`.
- 150s edit timeout and 90s fallback for image-to-image.
- 10 minute outer generation task timeout.

**On failure**: goroutine writes task `FAILED` through `FailGenerationTask`.

### Go API -> PostgreSQL Store: Complete Task

**Endpoint/Event/Command**: `CompleteGenerationTask(taskID, preparedRequest)`

**Payload**:

```json
{
  "taskId": "task_...",
  "generatedImages": [
    {
      "url": "data:image/png;base64,... or https://...",
      "thumbnailUrl": "string",
      "contentType": "image/png",
      "width": 1024,
      "height": 1024,
      "source": "model-provider"
    }
  ]
}
```

**Success response**:

```json
{
  "status": "SUCCEEDED",
  "progress": 100,
  "resultIds": ["asset_..."],
  "workerFinishedAt": "timestamp"
}
```

**Failure response**:

```json
{
  "ok": false,
  "error": "database, points, billing, commission or audit write failure",
  "retryable": "operator judgment"
}
```

**Timeout**: 5s.
**On failure**: goroutine calls `FailGenerationTask`; if that write fails, current code ignores the error.

### UI -> Go API: Poll Task

**Endpoint/Event/Command**: `GET /api/v1/generation-tasks/{id}`

**Payload**: path id only.

**Success response**:

```json
{
  "id": "task_...",
  "status": "SUCCEEDED",
  "resultIds": ["asset_..."],
  "imageUrl": "first asset URL",
  "thumbnailUrl": "first asset thumbnail"
}
```

**Failure response**:

```json
{
  "code": "401 or 404 or 500",
  "message": "generation task not found or auth/store error"
}
```

**Timeout**: no explicit client timeout; UI polling stops after 90 attempts.
**On failure**: UI logs warning and keeps polling until max attempts.

## Cleanup Inventory

| Resource | Created at step | Destroyed/closed by | Method | Failure if cleanup fails |
|---|---|---|---|---|
| `xz_generation_tasks` pending row | STEP 4 | Completion/failure path or startup stale-task repair | Terminal update to `SUCCEEDED` or `FAILED`; stale `PROCESSING` is failed on API startup after max age | Task can remain visible/running until next startup repair or manual repair if failure settlement fails |
| Provider HTTP request | STEP 5 | Context cancellation | 10 minute outer context, provider client timeout, edit/fallback timeouts | Provider call can hang until timeout; task remains `PROCESSING` until failure write or startup stale repair |
| Reference image local file | Reference image upload workflow, out of this spec | Not covered in current generation path | Missing cleanup spec | Uploaded reference files can accumulate |
| Provider-generated image URL/data URL | STEP 5/6 | Asset delete only hides the active asset row | `DELETE /api/v1/assets/{id}` writes `deleted_at`; remote provider URL is not deleted | External content may expire or remain remote |
| `xz_assets` row | STEP 6 | STEP 8 asset delete | Logical delete with `deleted_at`; task `resultIds` filtered; active asset queries exclude deleted rows | Asset stays in works list or task still points at missing asset |
| Point reservation | STEP 4 | `FailGenerationTask` on provider or completion failure | Reserve available points at enqueue, keep the reservation on success, refund the active reservation on failure with `billingRefunded` metadata | If failure settlement cannot be written, points can remain reserved and task may stay `PROCESSING` |
| Billing event | STEP 6 | No current cleanup | Inserted only inside success transaction | Usage ledger missing if transaction fails; task becomes `FAILED` |
| Commission records | STEP 6 | No current cleanup | Inserted only inside success transaction | Commission ledger missing if transaction fails; task becomes `FAILED` |
| Audit logs | STEP 4/6/8 | Append-only | Insert audit row | If complete audit fails, completion transaction fails and task becomes `FAILED` |

## Reality Checker Findings

| # | Finding | Severity | Spec section affected | Resolution |
|---|---|---|---|---|
| RC-1 | Current Go runtime does not expose `POST /api/v1/generation-tasks/{id}/cancel`, `retry`, `assets/{id}/favorite`, or `assets/{id}/regenerate`, although product docs describe them. | High | Registry; State Transitions | Keep those workflows `Missing`; add separate specs before implementation. |
| RC-2 | Current Go runtime now reserves points on enqueue and settles/refunds later. The reservation is stored in task params and point-account balance, not a separate hold ledger. | Low | STEP 4, STEP 6 | Add explicit hold/refund ledger events if accounting needs a full reservation subledger. |
| RC-3 | In-flight tasks are held by an in-process goroutine, not a durable worker queue. Startup stale-task repair fails old running tasks, but it does not resume provider work or run as a continuous worker. | Medium | STEP 5 | Add durable queue worker spec if task resume/retry is required. |
| RC-4 | `FailGenerationTask` refunds active point reservations idempotently, but does not write a separate refund billing-compensation event. | Low | STEP 5, Cleanup Inventory | Add failed-generation ledger events if finance reporting requires explicit refund rows. |
| RC-5 | Asset delete now uses `deleted_at` and active filters in Go runtime projection; restore/permanent purge policy remains unspecified. | Low | STEP 8 | Add an asset lifecycle spec if restore or retention windows are required. |
| RC-6 | Completion failure falls back to `FailGenerationTask`, but that fallback error is ignored. | Medium | STEP 6 | Add logging and repair workflow for failure-write failures. |
| RC-7 | Duplicate submit/idempotency is implemented in legacy Node code, not in current Go `POST /generation-tasks`. | Medium | STEP 1, STEP 4 | Add idempotency key contract if duplicate prevention is required. |

## Test Cases

| Test | Trigger | Expected behavior |
|---|---|---|
| TC-01: Happy path text-to-image | Authenticated user posts non-empty prompt, valid model/provider and sufficient points | API returns `PROCESSING`; points are reserved immediately; later polling returns `SUCCEEDED`; asset row exists; billing event and audit records written without a second point deduction |
| TC-02: Empty prompt | Post whitespace prompt | HTTP 400 `prompt is required`; no task row |
| TC-03: Unauthorized submit | Post without valid auth | HTTP 401; no task row |
| TC-04: Provider missing key | Post real model with API channel lacking env/saved key | HTTP 400 provider key error; no task row |
| TC-05: Insufficient points at enqueue | User point account available below computed point cost | HTTP 400 insufficient points; no task row |
| TC-06: Image-to-image without references | Post `IMAGE_TO_IMAGE` without `params.referenceImages` | Task may be created as `PROCESSING`, then becomes `FAILED` with reference image error |
| TC-07: Provider transient failure then success | Provider returns 502/503/504 once, then valid image | Provider retry succeeds; task becomes `SUCCEEDED` |
| TC-08: Provider permanent failure | Provider returns non-retryable 4xx or invalid payload | Task becomes `FAILED`; no asset/billing event; active point reservation is refunded |
| TC-09: Provider timeout | Provider exceeds configured/outer timeout | Task becomes `FAILED` with timeout message; active point reservation is refunded |
| TC-10: Second submit while points are reserved | User has enough points for one task, submits a second equal-cost task before the first settles | Second enqueue fails with insufficient points until the first task succeeds or refunds |
| TC-11: UI polling success | Submit through admin AI workspace and poll task id | Polling stops on terminal task and refreshes workspace |
| TC-12: UI polling max attempts | Task remains `PROCESSING` beyond 90 polls | UI stops tracking; task later completes, fails, or is failed by startup stale-task repair after max age |
| TC-13: Asset download data URL | Completed asset has data URL | Download returns attachment with detected content type |
| TC-14: Asset download remote URL failure | Asset URL returns non-2xx | HTTP 502 download error |
| TC-15: Delete own asset | User deletes asset that belongs to them | Asset row remains with `deleted_at`; active lists/download no longer expose it; task `resultIds` no longer contains asset id; audit `assets.delete` written |
| TC-16: Delete another user's asset | User deletes asset owned by someone else | HTTP 404; asset unchanged |
| TC-17: Process exit after enqueue | Kill app after `PROCESSING` commit and before provider completion, then restart after stale threshold | Startup repair marks task `FAILED` and refunds active point reservation; provider work is not resumed |
| TC-18: Duplicate submit | Send two identical POSTs quickly | Current Go runtime can create two tasks; use this as regression guard if idempotency is added |

## Assumptions

| # | Assumption | Where verified | Risk if wrong |
|---|---|---|---|
| A1 | `[功能名]` refers to the core generation task/works loop because no concrete feature name was supplied. | Inferred from repo central routes and existing docs | User may have intended another workflow such as PPT generation or channel-agent creation |
| A2 | `backend-go` is the current production-like runtime for this checkout. | `Dockerfile` starts `/app/xianzhi-api`; `compose.yml` service uses the same image | If Node backend is started separately in another deployment, retry/cancel behavior differs |
| A3 | `MODEL_PROVIDER_TIMEOUT_MS=600000` from compose is the local default when using Docker. | `compose.yml` | Non-Docker runtime may use 30000 ms default |
| A4 | The admin AI workspace is the primary current submitter for image generation. | `admin-vue/src/App.vue` submit and poll functions | Another UI may submit different params |
| A5 | Current Go failure path refunds active point reservations and marks `billingRefunded` in task params. | `CreatePendingGenerationTask`, `CompleteGenerationTask`, `FailGenerationTask` | Business may still require explicit failed-generation ledger events |

## Open Questions

- Should the Go runtime implement durable queue semantics using RabbitMQ/Redis, or should legacy Node worker behavior be retired completely?
- Should generation point reservations be promoted from task-param metadata to an explicit hold/refund ledger for finance reporting?
- Should `PROCESSING` tasks have a continuous timeout sweeper and operator repair UI in addition to startup stale-task repair?
- Should asset deletion become logical deletion to preserve auditability and match older docs?
- Should duplicate prevention be implemented with an idempotency key in the Go `POST /api/v1/generation-tasks` path?
- Should missing retry/cancel/favorite/regenerate endpoints be ported from legacy Node to Go?
- Should provider error taxonomy be normalized into stable user-facing error codes instead of raw provider messages?

## Spec vs Reality Audit Log

| Date | Finding | Action taken |
|---|---|---|
| 2026-07-01 | Initial discovery found active Go goroutine workflow plus legacy Node RabbitMQ workflow. | Wrote current Go workflow as Draft and marked legacy/retry/cancel gaps in registry. |
| 2026-07-01 | Product docs claim freeze/refund/cancel/retry paths not present in active Go routes. | Added RC-1, RC-2, RC-4 and Missing workflow rows. |
| 2026-07-01 | Current Go task can remain `PROCESSING` after process exit or failure-write failure. | Added RC-3, RC-6 and tests TC-12/TC-17. |
| 2026-07-10 | Current Go task creation now reserves points and failure refunds active reservations. | Updated STEP 4/5/6, cleanup inventory, risk findings and test cases to match runtime behavior. |
| 2026-07-10 | Current Go API starts `repairStaleGenerationTasks(15 * time.Minute)` on startup. | Updated process-exit, cleanup and RC-3 wording to distinguish startup repair from durable queue resume. |
