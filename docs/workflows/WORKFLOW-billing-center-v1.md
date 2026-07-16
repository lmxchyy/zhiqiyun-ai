# WORKFLOW: 账单中心 V1 规则发布与任务计费闭环

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 当前工作树的计费规则版本、供应商成本、任务计费生命周期、钱包流水与只读对账。

## Overview

管理员创建并校验价格草稿，串行发布为当前有效规则；生成任务随后按规则报价、冻结、确认或释放余额，同时写入生命周期事件和钱包流水。账单中心读取这些不可变事实构建成本、毛利和异常对账视图。

## Evidence Map

| Surface | Source |
|---|---|
| Admin routes/API | `server.go` billing routes, `billing_v1_api.go` |
| Version/cost/reconciliation store | `billing_v1_store_postgres.go`, `billing_v1_store_json.go` |
| Runtime charging | `postgres_store.go`, `knowledge_billing.go`, `pricing_catalog.go` |
| Types/states | `billing_v1_types.go`, `types.go` |
| Schema | migration `048-billing-center-v1.sql` |
| UI | `admin-vue/src/api/billing.ts`, `BillingCenterV1.vue` |

## Actors and prerequisites

Actors: authorized admin, admin API/RBAC, PostgreSQL store, generation/PPT/RAG callers, personal or enterprise wallet, provider-cost catalog, operator. Prerequisites: migration 048 applied; admin permission resolved by `adminPermissionForRequest`; exactly one active billing account per admitted task; provider/model/module codes normalized; store uses 5s DB contexts.

## Trigger

- Governance: admin opens billing modules, creates draft via existing rule mutation, validates and publishes.
- Runtime: generation/PPT/RAG task requests quote/reserve/capture/release.
- Audit: admin opens events, reconciliation or wallet ledger.

## Workflow Tree

### STEP 1: Load billing control plane

**Action**: Admin UI calls overview/rules/provider-costs/events/reconciliation/wallet-ledger endpoints.  
**Timeout**: backend PostgreSQL operations 5s; admin Axios timeout follows shared client configuration.  
**Success**: current versions, costs, recent task anomalies and wallet entries are shown -> STEP 2 or STEP 5.  
**Failures/recovery**: `401/403` -> authenticate/obtain permission; `schema_unready/DB timeout` -> migration/DB recovery and reload; `partial_page_load` -> UI currently fails the requested module, no cached authoritative fallback.  
**Observable**: customer N/A; operator sees page loader/error; DB read-only; HTTP errors only, no dashboard-load metric.
**Test cases**: authorized/forbidden access; schema unready; DB timeout; empty and populated control-plane views.

### STEP 2: Create a new rule version draft

**Action**: Admin edits base price, minimum charge and parameter rules; server locks latest version under a serializable transaction, increments version, copies immutable identity fields and inserts `DRAFT` with invalid/unvalidated result.  
**Timeout**: 5s DB context.  
**Success**: new `brv_*_vN`, source promoted from `CODE_DEFAULT` to `DATABASE` when applicable -> STEP 3.  
**Failures/recovery**: invalid JSON/client fields -> 400 before write; unknown source rule -> 404/400; serialization/version conflict -> transaction rollback and reload/retry; commit timeout ambiguity -> reload versions before retry.  
**Observable**: operator sees new draft; DB old version untouched; no audit row is written by this V1 draft path.
**Test cases**: first/new version; concurrent draft creation; invalid source; commit-response loss; verify previous versions are unchanged.

### STEP 3: Validate draft against catalog and cost coverage

**Action**: Server loads rule, admin data, all rule versions and provider costs; validation result with explicit issues is persisted.  
**Timeout**: 5s, although nested `AdminData()` may use its own context.  
**Success**: `{valid:true, issues:[]}` persisted -> STEP 4.  
**Failures/recovery**: semantic issue (invalid unit/price/parameters/coverage) -> valid=false; correct by creating/updating draft/cost and revalidate; DB error -> no reliable validation timestamp, reload; concurrent draft mutation can make a just-written validation stale because validation and publish are separate operations.  
**Observable**: operator receives validation dialog; DB `validation_result`; no customer impact until publish.
**Test cases**: valid rule; invalid unit/price/coverage; missing provider cost; validation followed by concurrent draft mutation.

### STEP 4: Publish one rule version atomically

**Action**: Server revalidates, opens serializable transaction, locks target, requires `DRAFT`, archives current `PUBLISHED` siblings, marks target `PUBLISHED` with effective/published timestamps.  
**Timeout**: validation 5s plus publish transaction 5s.  
**Success**: one published rule for the key; previous version `ARCHIVED` -> STEP 5.  
**Failures/recovery**: invalid validation -> 400; non-draft/conflict -> 400; serialization/deadlock/commit failure -> rollback and reload; `timeout_after_commit` -> read target and sibling statuses before retry.  
**Observable**: operator sees version status; DB transition is atomic; no publish audit event/notification is currently written.
**Test cases**: publish valid draft; reject invalid/non-draft; concurrent publishers; timeout after commit; exactly one published version.

### STEP 5: Maintain provider cost snapshot

**Action**: Admin updates provider/channel/model/unit/range/cost/currency/effective window/status under a locked DB transaction.  
**Timeout**: 5s.  
**Success**: future tasks select the latest matching `ACTIVE` cost; historical tasks retain supplier-cost snapshots -> STEP 6.  
**Failures/recovery**: negative/invalid cost or date/status -> 400; missing row -> 404/400; overlapping active cost windows are not prevented by an explicit exclusion constraint, so operator must inspect selection; DB error -> rollback.  
**Observable**: operator sees cost row; DB task history unchanged; no automatic margin recomputation for old tasks.
**Test cases**: create/update/delete cost; invalid dates/amount; overlapping active windows; historical task snapshot remains stable.

### STEP 6: Quote and reserve task charge

**Action**: Task admission resolves published rule and billing scope, deduplicates `userID + clientRequestID`, computes quote, locks personal wallet or enterprise compute account, then atomically writes task `QUEUED`, billing `RESERVED`, `QUOTE/RESERVE` events and wallet ledger.  
**Timeout**: 5s transaction; provider execution timeout belongs to the calling workflow.  
**Success**: balance moves available -> frozen/reserved; task can execute -> STEP 7.  
**Failures/recovery**: authorization/tenant disabled -> reject without task; insufficient balance -> reject without negative balance; duplicate request -> return existing task; concurrent wallet debit -> row lock/check; any ledger/event/task failure -> rollback all admission state.  
**Observable**: user sees accepted task or insufficient balance; operator sees task+events+ledger; DB has rule version/provider snapshot.
**Test cases**: personal/enterprise reserve; insufficient balance; duplicate/concurrent client request; rollback at every ledger/event write.

### STEP 7: Capture, release or refund

**Action**: Success transaction captures reserved points and writes usage/billing/commission/audit; failure/cancel releases reservation; explicit refund uses `REFUND`. Idempotency key is `taskID:event/entryType`.  
**Timeout**: 5s per settlement transaction.  
**Success**: task terminal plus billing `CAPTURED`, `RELEASED` or `REFUNDED` -> STEP 8.  
**Failures/recovery**: settlement DB error leaves task potentially active/reserved; caller attempts failure settlement but some call sites discard that error; duplicate settlement becomes no-op via ledger/event uniqueness; terminal race is guarded by task lock; operator must reconcile stuck reservations.  
**Observable**: customer sees task terminal and balance; operator sees lifecycle sequence; DB ledgers are append-only; no automatic alert for `BILLING_FAILED`.
**Test cases**: capture/release/refund; duplicate settlement; terminal race; settlement rollback; detect stuck reservation and `BILLING_FAILED`.

### STEP 8: Build reconciliation view

**Action**: Server reads up to 2000 tasks, lifecycle events and wallet entries, compares task/billing states and totals, and returns anomaly codes.  
**Timeout**: each list uses 5s; `ListBillingReconciliation` invokes additional list calls, so total request can exceed a single 5s wall-clock budget.  
**Success**: operator sees normal/abnormal counts and margin snapshots.  
**Failures/recovery**: query timeout/large result -> 500 and retry; missing supplier cost -> anomaly/null margin; inconsistent task/event/ledger -> anomaly only; there is no acknowledge, repair, replay or export workflow.  
**Observable**: operator sees anomaly filters; DB unchanged; no scheduled reconciliation or alerting.
**Test cases**: matched records; missing/duplicate event or ledger; missing cost; 2000-row boundary; timeout; confirm view performs no repair.

## State Transitions

```text
rule: CODE_DEFAULT/PUBLISHED -> new DRAFT -> validated -> PUBLISHED
      prior PUBLISHED -> ARCHIVED
task: CREATED -> QUEUED -> RUNNING -> SUCCEEDED | FAILED | CANCELLED
billing: UNQUOTED -> QUOTED -> RESERVED -> CAPTURED
                                   \-> RELEASED | REFUNDED | BILLING_FAILED
wallet: available -> RESERVE(frozen) -> CAPTURE
                               \-> RELEASE
```

## Handoff Contracts

| Boundary | Success | Failure/timeout | Recovery |
|---|---|---|---|
| Admin -> rule draft/validate/publish | version/validation/item JSON | 400/403/5s DB | reload before repeat |
| Admin -> provider cost | updated cost snapshot | 400/404/5s | correct input/reload |
| Task caller -> billing admission | task + quote/reservation snapshots | auth/insufficient/conflict/5s | no execution; retry with same client id |
| Task caller -> settlement | terminal task + ledger/events | 5s/transaction error | reconciliation + idempotent replay workflow (missing) |
| Operator -> reconciliation | anomalies/summary | 500/large-read timeout | retry; no repair endpoint |

## Cleanup Inventory

| Resource | Terminal/cleanup | Failure risk |
|---|---|---|
| Draft rule | publish or retain; never delete | stale drafts accumulate |
| Superseded published rule | archive with `effective_to` | two active rules if publish contract bypassed |
| Wallet reservation | capture/release/refund | frozen balance leak |
| Billing lifecycle/wallet ledger | append-only idempotent | missing evidence if settlement transaction fails |
| Task snapshot | terminal transition | task/billing drift |

## Reality Checker Findings

| # | Finding | Severity | Resolution |
|---|---|---|---|
| RC-1 | Reconciliation is read-only and unscheduled. | High | Add repair/acknowledge/scheduled alert workflow. |
| RC-2 | Validation and publish are separate; publish validates before acquiring its final row lock. | Medium | Revalidate locked content or add version checksum. |
| RC-3 | Draft/publish/provider-cost mutations lack explicit audit records in this V1 path. | Medium | Add governance audit contract. |
| RC-4 | Provider-cost overlap is not prevented by a clear DB exclusion rule. | Medium | Define deterministic precedence and validation. |
| RC-5 | Billing V1 is uncommitted working-tree code. | Medium | Review migrations and acceptance tests before deployment. |

## Test Cases

| ID | Trigger | Expected |
|---|---|---|
| TC-01 | Create draft from published/code-default rule | New version `DRAFT`, source/version correct. |
| TC-02 | Concurrent draft creation | Unique monotonically increasing versions or one serialization retry; no duplicate version. |
| TC-03 | Invalid rule validation | persisted issues; publish rejected. |
| TC-04 | Publish valid draft | target published and prior sibling archived atomically. |
| TC-05 | Publish non-draft twice | second request rejected/no state drift. |
| TC-06 | Update active provider cost | new tasks snapshot new cost; old task unchanged. |
| TC-07 | Duplicate task client request id | same task/one reservation returned. |
| TC-08 | Insufficient personal/enterprise balance | no task/event/ledger side effects. |
| TC-09 | Successful task | QUOTE+RESERVE+CAPTURE and balanced wallet entries. |
| TC-10 | Failed/cancelled task | RELEASE once; balance restored. |
| TC-11 | Concurrent settlement | one terminal outcome, no double ledger. |
| TC-12 | Missing capture/ledger/provider cost | reconciliation reports expected anomaly. |
| TC-13 | Reconciliation query timeout | 500; no mutation. |

## Assumptions and open questions

| Assumption | Evidence | Risk |
|---|---|---|
| PostgreSQL is authoritative for production billing. | V1 store and migration. | JSON fallback has weaker transaction semantics. |
| Point values may be represented as float in V1 views but wallet storage is integer. | Current structs/schema. | Fractional rule rounding needs an explicit contract. |

Open questions: Who may publish rules? What is the rollout/rollback approval gate? How are fractional prices rounded? What repairs a stuck reservation? Which anomaly thresholds page an operator?

## Spec vs Reality Audit Log

| Date | Finding | Action |
|---|---|---|
| 2026-07-15 | Scanned rule/cost/task/ledger code, migration 048, routes, UI and acceptance tests. | Created Draft and separated governance from commercial billing. |
