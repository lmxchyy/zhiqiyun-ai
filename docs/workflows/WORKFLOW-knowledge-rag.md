# WORKFLOW: 知识库摄取与 RAG 对话

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 知识库/ACL、文档解析切块与向量写入、智能体绑定、对话检索、流式生成、引用、计费、取消和重试

## Overview

文档摄取与 RAG 执行目前都在 HTTP 请求上下文内同步推进，不由 durable worker 消费。摄取先解析/切块，再建立 `INDEXING/RUNNING` 文档包，调用 embedding 与向量库，最后把文档/job 更新为 `READY`；失败则 best-effort 标记 `FAILED`。RAG 先写用户消息与 `QUEUED` run，随后经历 `RETRIEVING -> GENERATING -> COMPLETED`，流式返回答案并保存助手消息、引用和用量。客户端断流/请求取消时，defer 可能不写 `FAILED`，而跨进程取消也找不到内存 cancel 句柄，存在非终态悬挂。

## Evidence Map

| Surface | Current source |
|---|---|
| Routes | `server.go`: knowledge bases/ACL/ingest/docs/chunks/search/agents/bindings/conversations/runs/events/citations |
| API/orchestration | `knowledge_api.go`, `internal/app/knowledge/*` |
| Repositories/vector | knowledge repositories, embedding/vector runtime adapters |
| Client | user/admin knowledge API, agent center and mini-chat components |
| Billing | `RecordRAGUsage` and Billing V1 ledgers/events |
| Schema | migrations `033`-`036` |
| Config | model runtime profiles, provider timeout, OCR/parser/vector settings |

## Actors and prerequisites

Authenticated user; knowledge-base ACL (`UPLOAD`, read/search/manage as appropriate); Go API; parser/OCR; embedding provider; vector store; chat model; PostgreSQL; billing wallet/rules. Inline upload is limited to 20 MiB. Provider timeout defaults to 30s unless a runtime profile overrides it; the shared mini-program API client permits up to 600s.

## Trigger

- Ingestion: knowledge-base ingest/upload endpoint.
- Setup: create knowledge base, ACL, agent and bindings, then conversation.
- Answer: run or stream-run endpoint; follow-up cancel/retry/event/citation endpoints.

## Workflow Tree

### STEP 1: Configure knowledge base, ACL and runtime

**Actor/Action**: authorized user/admin creates the knowledge base, grants ACL, selects parser/embedding/vector/chat runtime profiles, creates an active knowledge agent and bindings.  
**Timeout**: 5s PostgreSQL store operations; runtime resolution has request-context limits.  
**Success**: enabled knowledge base and active agent/bindings -> STEP 2 and STEP 4.  
**Failures**: forbidden tenant/ACL, disabled KB/agent, invalid or secret-leaking runtime profile, missing provider/vector config, DB error.  
**Recovery**: correct ACL/profile and retry; secrets stay server-side and sanitized in responses; switch to authorized tenant context.  
**Observable state**: KB/ACL/agent/binding rows and admin/user views; request/audit logs; provider secret values must never be observable.  
**Test cases**: tenant isolation; each ACL action; disabled KB/agent; secret sanitization; missing runtime; binding only authorized KBs.

### STEP 2: Parse, normalize and chunk an upload

**Actor/Action**: ingest endpoint authorizes `UPLOAD`, resolves runtime, validates 20 MiB inline size, extracts text (and optional OCR), normalizes it and generates chunks/content hash before the DB bundle is created.  
**Timeout**: entire HTTP request; parser/OCR/provider timeout follows runtime profile/default 30s; client cap 600s.  
**Success**: deterministic content/chunks ready -> STEP 3.  
**Failures**: oversized/unsupported/corrupt file, empty extraction, OCR/parser timeout, request cancellation, unauthorized upload.  
**Recovery**: split/convert/re-upload; correct OCR/runtime; retry safely using the same content because later bundle idempotency derives from tenant/KB/content hash. No document row exists if failure precedes bundle creation.  
**Observable state**: HTTP progress/error and parser logs only; no persisted pre-parse job or resumable upload state.  
**Test cases**: supported formats; 20 MiB boundary; empty/corrupt content; OCR success/timeout; cancellation; same-content normalization.

### STEP 3: Persist document bundle, embed and index vectors

**Actor/Action**: repository creates document=`INDEXING`, version=`PARSED`, ingest job=`RUNNING` stage `INDEXING`, progress 75, maxAttempts 3 and derived idempotency key; service embeds chunks, validates cardinality, upserts vectors, then best-effort updates document/job to `READY`.  
**Timeout**: request context; embedding/runtime default 30s unless overridden; PostgreSQL operations 5s; vector adapter has its configured/request deadline.  
**Success**: document/job=`READY`, searchable chunks/vectors -> STEP 4.  
**Failures**: bundle transaction/idempotency conflict, embedding error/timeout, embedding-count mismatch, vector partial/upsert failure, DB `READY` update failure after vectors succeed, client disconnect.  
**Recovery**: service best-effort marks document/job `FAILED`; user can re-ingest. `maxAttempts=3` is stored but no worker was found to consume/retry it. Operators must reconcile vectors that succeeded while DB remained `INDEXING/FAILED`.  
**Observable state**: document `INDEXING/READY/FAILED`; job `RUNNING/READY/FAILED`, stage/progress/error/attempt fields; chunks in DB and vectors in vector store; logs.  
**Test cases**: happy path; content idempotency; embedding mismatch; partial vector failure; vectors-success/ready-update-failure; request cancel; repeated ingest; maxAttempts remains non-operative.

### STEP 4: Create conversation, user message and queued run

**Actor/Action**: run endpoint validates conversation/question, loads active agent and history/bindings, writes the user message as `COMPLETED`, then creates run=`QUEUED` and registers an in-memory cancel handle.  
**Timeout**: 5s per DB operation; stream request/client can remain open up to the 600s client cap, with no narrower overall RAG SLA.  
**Success**: durable user message and queued run -> STEP 5.  
**Failures**: unauthorized/missing conversation, inactive agent, invalid question, no enabled binding, DB failure between user message and run creation, process crash before cancel registration.  
**Recovery**: correct context/agent/bindings; retry request. Because user-message and run creation are not described as one transaction/idempotent command, inspect conversation before retry to avoid duplicate user messages.  
**Observable state**: user message, run id/status, run-started event/SSE; cancel registry is process-local and not observable across replicas.  
**Test cases**: authorization; inactive/no bindings; DB partial failure; duplicate submit; response loss; restart after run creation; message/run linkage.

### STEP 5: Rewrite and retrieve evidence

**Actor/Action**: service emits start events, optionally rewrites the query using history, sets run=`RETRIEVING`, searches each enabled KB binding and persists retrieval hits. Rewrite failure is ignored and original question continues.  
**Timeout**: model profile/default 30s for rewrite; DB 5s; vector/search deadline follows request/runtime; no aggregate per-binding deadline is separately documented.  
**Success**: ranked authorized chunks/hits -> STEP 6.  
**Failures**: rewrite error, vector timeout, one binding failure, disabled/deleted KB, ACL drift, no hits, request cancellation.  
**Recovery**: fallback to original query for rewrite only; hard retrieval error fails the run; fix index/binding and retry. No durable continuation resumes from persisted hits.  
**Observable state**: run=`RETRIEVING`, events, persisted hits/scores/source ids, logs; user may see stream progress but not every binding failure.  
**Test cases**: rewrite success/fallback; multi-KB ranking; ACL isolation; no hits; one binding error; vector timeout; cancellation.

### STEP 6: Generate/stream answer, persist citations and bill

**Actor/Action**: run becomes `GENERATING`; chat model streams deltas, service creates the assistant message, persists citations, records RAG usage/billing, then marks run `COMPLETED` and emits terminal event.  
**Timeout**: chat provider profile/default 30s unless overridden; stream has no independent server-wide SLA; DB calls 5s; client cap 600s.  
**Success**: completed run, assistant answer, authorized citations and captured/recorded usage.  
**Failures**: provider timeout/stream break, malformed delta, message/citation write failure, billing failure after answer artifacts are already committed, terminal-status/event write failure.  
**Recovery**: defer normally marks non-cancel errors `FAILED`; user can retry. Billing-after-artifact failure currently can leave a visible answer/citations with a `FAILED` run; operator must reconcile usage and decide whether to hide/retain partial output.  
**Observable state**: SSE deltas/events; run `GENERATING/COMPLETED/FAILED`; assistant message/citations; billing lifecycle/ledger/usage; logs.  
**Test cases**: answer+citations happy path; provider stream failure; citation write failure; billing failure after answer; terminal event failure; exact-once usage; citation tenant/ACL checks.

### STEP 7: Cancel a run or handle disconnect

**Actor/Action**: cancel endpoint updates an owned active run and invokes its in-memory cancel handle when found; request context cancellation stops current provider work.  
**Timeout**: immediate context cancellation plus 5s DB write; no provider cancellation acknowledgement deadline.  
**Success**: run=`CANCELLED`, terminal event and no further persisted answer.  
**Failures**: cancel hits another replica and cannot stop provider; disconnect path is excluded from generic `FAILED` defer and may leave `QUEUED/RETRIEVING/GENERATING`; completion races with cancel; client misses terminal event.  
**Recovery**: refetch run; repeat cancel against authoritative status; startup/periodic stale-run repair is not present and must be added for suspended states.  
**Observable state**: run status/events plus process-local registry; logs may show context cancellation; no heartbeat/lease or stale-run alert exists.  
**Test cases**: cancel in each state; disconnect in each state; cross-replica cancel; cancel/completion race; repeated cancel; stale-state detection.

### STEP 8: Retry and clean up

**Actor/Action**: retry loads an owned prior run and creates a new run with `RetryOfRunID`, reusing the original query; document delete removes vectors first and then soft-deletes the document.  
**Timeout**: retry inherits normal RAG limits; vector deletion/request context and DB 5s.  
**Success**: new independently observable run, or document removed from active search.  
**Failures**: retry duplicates the original user message, repeated taps produce multiple runs, vector delete failure leaves document active, vector success followed by DB soft-delete failure leaves an active document with missing vectors.  
**Recovery**: add/reuse an idempotent retry intent; refetch before retry; on deletion partial failure re-index or complete DB soft delete via operator reconciliation.  
**Observable state**: retry lineage, extra user/run messages, vector contents, document deleted/active status, logs; no deletion saga state exists.  
**Test cases**: retry lineage and duplicate-message policy; retry double-tap; vector-delete failure; DB-delete failure after vectors; repeated delete; deleted KB excluded from retrieval.

## State Transitions

| Entity | Transition |
|---|---|
| Document | `INDEXING -> READY` or `FAILED`; deletion is a separate soft-delete operation |
| Ingest job | `RUNNING -> READY/FAILED`; attempt fields exist without a consuming retry worker |
| RAG run | `QUEUED -> RETRIEVING -> GENERATING -> COMPLETED`, or `FAILED/CANCELLED` |
| Message | user message `COMPLETED`; assistant message created late in generation |
| Billing | RAG usage recorded near terminal generation; exact state follows Billing V1 lifecycle |

## Handoffs and Cleanup

- Parser-to-index handoff is in one HTTP call; no durable task owns pre-parse work.
- DB bundle can exist before vector success; vector contents can exist before DB `READY` succeeds.
- Run cancel ownership is memory-local; no distributed lease routes cancellation to the executing replica.
- Failed run may retain user message, hits, partial answer/citations and billing projections; retention policy must be explicit.
- Vector-first document deletion is not an atomic saga and needs a reconciliation state/action.

## Reality Checker

- Ingest job `maxAttempts=3` suggests retries, but no worker consumes the attempts.
- Document readiness/failure updates are best effort after external vector effects, so DB and vector state can diverge.
- RAG is synchronous within the stream request; there is no durable run worker, lease or heartbeat.
- Client disconnect can leave a non-terminal run because cancellation is deliberately not converted to `FAILED` and may not persist `CANCELLED`.
- Billing occurs after answer artifacts; billing failure can produce a failed run with usable output.
- Retry can add another copy of the original user question and lacks a clear idempotency contract.
- Document delete performs vector deletion before DB soft delete without compensation.

## Test Cases

1. KB/ACL/agent/binding tenant and data-scope isolation.
2. Upload size/format/parser/OCR boundaries.
3. Content-idempotent ingest and concurrent duplicate ingest.
4. Embedding mismatch, vector partial failure and DB/vector reconciliation.
5. No-worker assertion for stored attempt/maxAttempts fields.
6. Run admission partial failure and duplicate submit.
7. Rewrite fallback and multi-KB retrieval authorization.
8. Stream success with exact messages, events, hits and citations.
9. Provider/citation/billing/terminal-event failure at every boundary.
10. Disconnect/cancel across all states and replicas.
11. Retry lineage, duplicate-message and double-tap behavior.
12. Vector-first delete partial failures and repair.

## Assumptions and Open Questions

- Should ingestion and RAG be moved to durable workers, or are bounded synchronous requests an intentional product contract?
- What is the terminal policy for client disconnect: `CANCELLED`, resumable, or `FAILED`?
- Must answer/citations be hidden or free when billing fails after generation?
- What saga/reconciliation owns DB-vector consistency for ingest and deletion?
- Should retry reuse the original user message instead of inserting another copy?
