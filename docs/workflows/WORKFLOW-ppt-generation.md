# WORKFLOW: PPT 生成与逐页图片补全

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 当前工作树中的大纲生成、PPT 任务持久化、逐页 AI 图片、编辑、导出与删除

## Overview

用户提交主题或已有大纲，服务端同步生成/校验大纲，把 PPT 任务写入本地 `ppt-tasks.json`，并立即记录 PPT 用量。任务没有独立 worker：读取任务时按创建后的经过时间将 `pending -> processing -> success` 物化。若启用 AI 图片，服务端另起进程内 goroutine，为每页最多尝试两次图片生成；单页失败只记日志，不会把 PPT 主任务置为失败。PPTX 同步导出，PDF 当前只返回空 URL。

## Evidence Map

| Surface | Current source |
|---|---|
| Routes | `server.go` 中 `/api/v1/ppt/*` 与兼容 `/api/ppt/*` |
| API orchestration | `ppt_api.go`, `ppt_export.go` |
| Task state/persistence | `internal/app/ppt/service.go`, `ppt-tasks.json` |
| Slide images | `runPPTTaskImageGeneration`, generation task/store/provider |
| Clients | admin PPT store/components; mini-program PPT API/pages |
| Billing/config | Billing V1 usage recorder, model/image provider config |
| Database migrations | 无 PPT 主任务迁移；逐页图片复用 generation/asset/billing 表 |

## Actors and prerequisites

| Actor | Role |
|---|---|
| Authenticated user | Supplies topic/outline, polls, edits and exports. |
| PPT API/service | Generates outline, persists and materializes task state. |
| Text model provider | Optionally generates structured outline. |
| Image generation runtime | Optionally creates one image per slide. |
| Billing/store | Records PPT usage and image reservations/captures. |

Prerequisites: authenticated model-call permission; writable PPT data directory; configured text provider or deterministic outline fallback; image provider only when AI images are enabled. The PPT task store is a local JSON file and is not safe as a shared multi-replica queue.

## Trigger

- `POST /api/v1/ppt/generate` with topic/config and optional outline.
- Separate outline path: `POST /api/v1/ppt/outline/generate`, then save/generate.
- Follow-up: task/history/edit/regenerate/export/delete endpoints.

## Workflow Tree

### STEP 1: Validate request and resolve outline

**Actor/Action**: API authenticates and authorizes a model call, decodes the PPT request, then accepts a supplied outline or calls the outline generator.  
**Timeout**: Request context; configured model-provider timeout (default 30s where no profile override is present).  
**Success**: A normalized outline and slide list are available -> STEP 2.  
**Failures**: malformed input (400), unauthorized/forbidden (401/403), provider timeout/error, non-JSON model response.  
**Recovery**: correct input/re-login; retry outline generation; when the provider is not configured the current implementation uses deterministic `buildPPTOutline`; malformed configured-provider output returns an error rather than silently changing source.  
**Observable state**: HTTP status and provider logs; no PPT task exists yet; UI shows outline loading/error or editable outline.  
**Test cases**: valid supplied outline; prompt-only fallback; provider JSON success; malformed JSON; provider timeout; unauthorized user.

### STEP 2: Create and persist the PPT task

**Actor/Action**: `pptService.Generate` creates slides and a `pending` task, then rewrites local `ppt-tasks.json`; API reads the task and calls `RecordPPTGenerationUsage`.  
**Timeout**: Request context; file write has no narrower deadline; billing DB operations inherit the PostgreSQL store's 5s timeout.  
**Success**: task is returned, persisted and billed -> STEP 3; optional image enrichment is started after creation.  
**Failures**: unwritable/corrupt task file, concurrent process overwrite, billing record failure, response loss after persistence.  
**Recovery**: fix filesystem ownership/corruption and retry; inspect history by task id before resubmission; there is no create idempotency key, rollback of the local task, or automatic billing reversal if later deck work is ineffective.  
**Observable state**: task file contains `pending`, timestamps, outline and slides; billing lifecycle/ledger may already contain usage; HTTP/logs are the only admission signal.  
**Test cases**: successful persistence; unwritable directory; malformed existing JSON; billing failure; duplicate submit/response loss; two-process write race.

### STEP 3: Materialize progress during polling

**Actor/Action**: `GetTask`/history computes status from elapsed wall time: initially `pending`, at about 700ms `processing` with progress 65, and at about 2500ms `success` with progress 100.  
**Timeout**: No server-side execution timeout because no core execution occurs; mini-program polling currently stops after five 750ms waits, while admin-side behavior is UI-driven.  
**Success**: caller observes `success` -> STEP 5 or STEP 6.  
**Failures**: process clock change, local-file loss, client stops polling before terminal state, replica reads another local file, task is reported successful without generated content validation.  
**Recovery**: refetch history/task on the same instance; operator restores the task file; no durable retry/resume/lease exists.  
**Observable state**: `pending/processing/success` and progress; the current materializer does not produce `failed`; no worker heartbeat, queue depth or attempt count exists.  
**Test cases**: exact boundary timing; clock skew; restart; missing task; cross-replica read; assert that no false worker is claimed by monitoring.

### STEP 4: Generate optional per-slide AI images

**Actor/Action**: an in-process goroutine processes at most three slides concurrently. Each slide creates a billable image task, calls the provider, persists the artifact and writes its URL back to the PPT task.  
**Timeout**: 115s per attempt, two attempts, with 1s then 2s retry delay; effective slide latency can exceed 230s plus queueing/persistence. Admin initial/background waiting windows are UI heuristics, not server SLAs.  
**Success**: slide receives an image URL; all possible slide updates finish independently -> STEP 5.  
**Failures**: provider timeout/rate limit, billing reservation failure, artifact storage failure, task file update race, process restart, partial deck images.  
**Recovery**: automatic second attempt; billing failure path releases the image reservation; user can regenerate an individual slide; orphaned goroutines are not resumed after restart.  
**Observable state**: generation task/billing/artifact rows expose per-image status; PPT task exposes each slide URL; logs expose failures. The PPT parent remains `success` even when every slide image fails.  
**Test cases**: all slides succeed; one/all slides fail; concurrency cap 3; 115s cancellation; second-attempt success; storage failure cleanup; restart mid-run; parent/child status consistency.

### STEP 5: Edit or regenerate a slide

**Actor/Action**: user saves the outline/slide content or requests single-slide regeneration; service rewrites the local task record and may invoke image generation again.  
**Timeout**: file operation has no explicit deadline; manual image endpoint uses 115s context.  
**Success**: requested slide/content is updated -> STEP 6.  
**Failures**: stale client overwrites newer content, invalid slide index, task missing, image provider/storage failure.  
**Recovery**: refetch before editing; correct slide id; retry single-slide generation. There is no revision/ETag conflict protection.  
**Observable state**: updated task file and slide image task; UI edit/regeneration state; server logs.  
**Test cases**: content save; invalid slide; concurrent edits; regeneration success/failure; retry after refresh.

### STEP 6: Export or delete

**Actor/Action**: PPTX export renders the current task synchronously and fetches remote images; PDF endpoint returns an empty URL; delete removes the task record.  
**Timeout**: PPTX image collection has an overall 25s context and a 15s HTTP client timeout per remote image; delete/file rewrite has no narrower deadline.  
**Success**: PPTX bytes download, or task is removed. PDF has no implemented success artifact.  
**Failures**: slow/broken image URL, malformed task, renderer error, output interrupted, delete/write failure, advertised PDF with empty URL.  
**Recovery**: retry PPTX after replacing/removing bad images; regenerate missing images; retry delete after filesystem repair; PDF requires implementation rather than user retry.  
**Observable state**: HTTP download/error and export logs; no export job/status table; after delete the task disappears, while generation assets and billing records are not automatically removed.  
**Test cases**: export without images; mixed local/remote images; 15s image timeout; 25s aggregate timeout; PDF contract test; delete and repeated delete; verify asset/billing retention.

## State Transitions

| Entity | Current transitions |
|---|---|
| PPT task | `pending -> processing -> success`; `failed` exists in type but current materializer never enters it |
| Slide image task | `PROCESSING/QUEUED/RESERVED -> SUCCEEDED/CAPTURED` or `FAILED/RELEASED` |
| Export | synchronous request only; no persisted state |

## Handoffs and Cleanup

- Text-outline handoff ends when normalized slides are created; no provider job id is retained.
- PPT-to-image handoff uses task/slide identifiers, but the parent has no child completion aggregate.
- Failed image persistence performs best-effort artifact cleanup through the generation workflow.
- Deleting a PPT task does not reverse charges or remove generated image assets.
- Process shutdown has no drain/resume contract for slide-image goroutines.

## Reality Checker

- Core PPT “generation” status is elapsed-time materialization, not execution by a durable worker.
- PPT usage is recorded at admission, before a meaningful completion boundary is proven.
- Slide images are child workflows whose failure does not affect parent success.
- `ppt-tasks.json` can diverge across replicas and is vulnerable to last-writer-wins updates.
- PDF export is an API placeholder returning an empty URL.
- Admin client mock fallbacks can make an unavailable backend look like usable sample data.
- No cancel, retry, worker heartbeat, attempt counter or PPT database migration was found.

## Test Cases

1. End-to-end prompt -> outline -> task -> PPTX without image provider.
2. Configured outline-provider success, timeout and malformed JSON.
3. Task-file write failure and restart/reload behavior.
4. Polling boundaries at 0ms, 700ms and 2500ms.
5. Multi-replica/local-file divergence test.
6. Per-slide image concurrency, retry, timeout and partial failure.
7. Billing recorded exactly once per intended PPT/image action.
8. Provider success followed by artifact persistence failure and cleanup.
9. Concurrent slide edits and stale overwrite.
10. PPTX image fetch per-request and aggregate timeout.
11. PDF endpoint must fail explicitly until a real artifact is produced.
12. Delete retains or explicitly cleans child assets and ledger according to policy.

## Assumptions and Open Questions

- Should a deck be `success`, `partial_success`, or `failed` when required slide images fail?
- What is the billable milestone: admission, outline completion, deck completion, or successful export?
- Is single-instance local persistence intentional for production? If not, define DB schema, durable queue, lease, retry and cancellation semantics.
- Should delete be content-only, or also schedule asset cleanup while preserving immutable billing/audit records?
