# WORKFLOW: 微信虚拟支付与权益发放

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 当前工作树中的微信小程序虚拟商品下单、回调/查单、统一权益发放与补偿。

## Overview

用户从服务端商品目录选择虚拟商品，服务端固化金额与权益快照并生成微信签名；支付成功后，微信回调、官方查单补偿和后台人工补发统一进入 `GrantOrderEntitlements` 事务路径。成功结果是订单 `PAID`、权益 `SUCCESS`、独立账本写入且商业计费投影同步。

## Evidence Map

| Surface | Current source |
|---|---|
| Routes | `server.go:197-204`, admin payment routes |
| Order/callback/query/compensation | `wechat_virtual_payment.go` |
| Entitlement transaction/idempotency | `wechat_virtual_entitlements.go` |
| Coupon/commercial projection | `commercial_billing_wechat.go` |
| Schema | migrations `047`, `049`, `050-wechat-virtual-custom-token-unit`, `050-commercial-billing-wechat` |
| Client | `features/payment/*`, `UserVirtualPaymentPage.vue` |
| Config | `config.go`, `.env*`, `compose*.yml` |

## Actors and prerequisites

| Actor | Role |
|---|---|
| Authenticated mini-program user | Selects product, invokes WeChat payment, polls result. |
| Shared API client | Adds auth/context/request id; 600s client timeout. |
| Virtual payment API | Resolves tenant/session, signs order, verifies callback, queries WeChat. |
| PostgreSQL | Owns orders, payment events, entitlement/account ledgers and commercial projections. |
| WeChat virtual payment | Executes payment, sends callback, answers `query_order`. |
| Compensation loop/operator | Reconciles stale orders or retries failed entitlement grants. |

Prerequisites: PostgreSQL-backed store; valid WeChat mini-program session; `WECHAT_VIRTUAL_PAY_ENABLED=true`; offer id, app key/sandbox key, notify token, app id/secret provided by secrets; active product mapping; server-side amount and entitlement configuration. No client-supplied amount or entitlement is trusted.

## Trigger

- User: `POST /api/v1/payment/wechat-virtual/orders` with `{productCode, quantity, couponCode}`.
- Provider: signed `POST /api/v1/payment/wechat-virtual/notify`.
- Recovery: `POST /api/v1/payment/orders/:orderNo/sync`, compensation loop, or admin grant endpoint.

## Workflow Tree

### STEP 1: Load product and coupon catalog

**Actor/Action**: Client calls product/coupon routes; server reads active mapped plans and eligible bonus coupons.  
**Timeout**: Client 600s; route/DB call has no narrower explicit deadline.  
**Success**: Server returns server-priced items, `enabled`, environment and non-discount coupon benefits -> STEP 2.  
**Failures and recovery**:

- `auth_required` (401): re-login, then reload.
- `payment_unavailable` (503): operator completes config/DB readiness; client disables pay action.
- `catalog_db_error` (500): no order created; operator checks migrations/product mapping.
- `coupon_unavailable` (400): clear coupon or choose an eligible coupon; price must not change.

**Observable state**: customer sees loading/catalog/disabled state; operator sees request status and admin product mapping; DB remains read-only; no dedicated metric exists.
**Test cases**: authenticated catalog; disabled config; DB failure; expired/ineligible coupon; assert that client input cannot change price or benefits.

### STEP 2: Create immutable local order and signing payload

**Actor/Action**: Server validates product/quantity/coupon, resolves tenant and WeChat session, computes HMAC signatures, then one DB transaction inserts `xz_orders`, `xz_payment_records`, optional coupon reservation, invoice and payment request.  
**Timeout**: Request context only; payment expires after 30 minutes.  
**Success**: HTTP 201 `{orderNo, amountCent, signData, paySig, signature, mode}`; order=`PENDING`, entitlement=`PENDING`, payment record=`SIGNED` -> STEP 3.  
**Failures and recovery**:

- `invalid_input/product_mapping/quantity` (400): correct selection; transaction rolls back.
- `wechat_session_expired` (401 + `WECHAT_SESSION_EXPIRED`): re-login; no order committed.
- `tenant_forbidden` (403): switch to an authorized context.
- `database_or_constraint` (500): all local rows and coupon reservation roll back; safe to retry, but endpoint has no client idempotency key and may create another order if the first commit succeeded but response was lost.
- `timeout_unknown_commit`: client must query owned orders/admin records before resubmitting.

**Observable state**: customer sees “正在创建订单”; operator sees pending order/payment request and reserved coupon; DB contains price snapshot and OpenID hash, never raw secret/session key; logs have HTTP/audit only.
**Test cases**: valid order; invalid quantity/session/context; transaction rollback at every insert; response lost after commit; repeated tap creates detectable duplicate intent.

### STEP 3: Invoke WeChat virtual payment

**Actor/Action**: Mini program calls `wx.requestVirtualPayment` with server-signed payload.  
**Timeout**: WeChat SDK controls interactive timeout; repo defines no explicit client deadline for the native call.  
**Success**: SDK resolves; client begins status synchronization -> STEP 4.  
**Failures and recovery**:

- `unsupported_runtime/version`: show unsupported message; keep order `PENDING` until expiry.
- `user_cancel`: do not grant; allow manual status query; compensation later closes unpaid order.
- `sdk/network/error`: payment outcome is ambiguous; call sync instead of creating a new order.
- `duplicate_tap`: UI `paying` flag blocks normal repeats, but backend lacks order-create idempotency.

**Observable state**: customer sees native payment sheet/result; operator only sees the local pending order until callback/query; DB unchanged; native errors remain client-visible.
**Test cases**: SDK success; user cancel; unsupported client; ambiguous network error followed by sync; duplicate-tap guard.

### STEP 4: Confirm payment by signed callback or official query

**Actor/Action**: Callback verifies SHA1 token signature and 1 MiB body limit, parses XML/JSON, or query path obtains/caches access token then calls WeChat with a 10s HTTP client timeout. Both normalize a `virtualPayNotification`.  
**Timeout**: external HTTP 10s; callback inherits request context; access token cached until `expires_in - 300s`.  
**Success**: payment event is locked/idempotently upserted, order snapshot/amount/product/quantity/env/OpenID are validated -> STEP 5.  
**Failures and recovery**:

- `invalid_signature/body/event` (400/401): reject with no grant; failed notification is recorded when possible.
- `wechat_timeout/5xx/token_error`: sync returns error; compensation retries later.
- `payment_mismatch` (500 callback / 400 sync): no grant; persist failed event; operator investigates, never overrides amount client-side.
- `duplicate_event`: event/transaction unique key returns prior `SUCCESS`; no second grant.
- `closed_or_refunded_order`: reject paid transition.

**Observable state**: customer keeps polling; operator sees `xz_payment_events.processing_status`; DB event is `PROCESSING/FAILED/SUCCESS`; callback returns provider-compatible retry response on internal error.
**Test cases**: signed JSON/XML callback; bad signature/oversized body; query timeout; amount/product/OpenID mismatch; duplicate and concurrent callback/query.

### STEP 5: Atomically mark paid and grant entitlements

**Actor/Action**: A transaction locks the order, sets order/payment record paid, grants exactly the server snapshot (`TOKEN_ONLY/TOKEN_UPGRADE`, `IMAGE_QUOTA_PACK`, or `MEMBER_PACKAGE`), applies coupon bonus, synchronizes invoice/payment/subscription projections, marks fulfillment and event success.  
**Timeout**: Request/compensation context; background compensation gives the whole pass 90s.  
**Success**: order=`PAID`, fulfillment=`FULFILLED`, entitlement=`SUCCESS`; membership/credits/image quota and corresponding idempotent ledgers exist -> STEP 6.  
**Failures and recovery**:

- `unsupported_or_invalid_snapshot`: main transaction rolls back; a second transaction persists order `PAID` + entitlement `FAILED`.
- `ledger/account/audit/projection_error`: no partial entitlement commit; persist failure if possible; compensation/admin grant retries same service.
- `concurrent_callback/query/admin_grant`: row lock plus per-entitlement idempotency keys prevent double grant.
- `failure_persistence_error`: order may remain `PENDING/PROCESSING`; callback requests retry and operator must inspect events/order.

**Observable state**: customer sees “支付已确认，权益处理中/失败/成功”; operator sees failure list and grant action; DB changes are atomic across entitlement ledgers and commercial projections; no dedicated alert is emitted.
**Test cases**: every product type; coupon bonus; failure at each ledger/projection write; concurrent callback/query/admin grant; retry after persisted entitlement failure.

### STEP 6: Poll, compensate and close stale orders

**Actor/Action**: Client polls status and tries sync; process starts compensation after 20s, then every 2 minutes scans 50 candidates, claims each for 90s, grants paid failed/pending entitlements or queries stale pending orders, and closes expired unpaid orders.  
**Timeout**: client polls 10 times at 1.5s in current page; sync HTTP 10s; loop pass 90s.  
**Success**: client receives `completed=true` and balances, emits entitlement-updated event; stale unpaid order becomes `CLOSED`.  
**Failures and recovery**:

- `poll_budget_exhausted`: show manual-sync action; background loop continues.
- `process_restart`: loop restarts with 20s delay; locks expire after 90s.
- `multi_replica_race`: DB claim limits duplicate work, but there is no leader election; idempotency remains the last guard.
- `loop_error`: error is discarded; no alert/metric; operator uses admin failure/pending views and manual grant.

**Observable state**: customer sees final balances or actionable failure; operator sees pending/failed counts and compensation locks; DB lock is cleared best-effort; logs do not currently expose loop failures.
**Test cases**: poll exhaustion/manual sync; 20s startup and 2-minute pass; 50-item cap; 90s claim expiry; restart/multi-replica race; expired unpaid close.

### ABORT/REFUND boundary

Unpaid orders are closed and reserved coupons cancelled. Refund callbacks create refund records and commercial credit projections, but entitlement reversal is explicitly manual and no official refund-initiation endpoint exists; this workflow must not claim that a refund automatically removes already granted membership, credits or quota.

## State Transitions

```text
order: PENDING -> PAID -> REFUND_PENDING/REFUNDED
       PENDING -> CLOSED
entitlement: PENDING -> PROCESSING -> SUCCESS
             PENDING/PROCESSING -> FAILED -> (compensation/admin grant) -> SUCCESS
coupon redemption: RESERVED -> APPLIED | CANCELLED
payment event: RECEIVED/PROCESSING -> SUCCESS | FAILED | IGNORED
```

## Handoff Contracts

| Boundary | Payload/success | Failure/timeout | Recovery |
|---|---|---|---|
| Client -> create order | product code, quantity, coupon; 201 signed payload | 400/401/403/503/500; client 600s | re-login/fix input/query before duplicate submit |
| Client -> WeChat SDK | signed `signData/paySig/signature/mode` | native cancel/error; no repo timeout | sync existing order |
| WeChat -> notify | signed XML/JSON <=1 MiB; `{ErrCode:0}` | 401/400 or 500 `{ErrCode:-1}` | WeChat retry + compensation |
| API -> WeChat query | access token + signed order request; HTTP 2xx/code 0 | 10s, HTTP/error code | compensation/manual sync |
| Confirmation -> entitlement service | locked order + immutable snapshot | transaction error, retryable unless mismatch | persist FAILED; retry same service |

## Cleanup Inventory

| Resource | Cleanup/terminal action | Failure risk |
|---|---|---|
| Pending order/payment request | close after expiry | stuck pending if loop silent-fails |
| Reserved coupon | apply on success; cancel on close | reserved count leak |
| Compensation lock | clear after attempt; 90s expiry | delayed retry |
| Access token cache | TTL expiration | sync unavailable if Redis error is treated as fatal |
| Entitlement/account ledgers | append-only/idempotent; never delete | manual reversal required after refund |

## Reality Checker Findings

| # | Finding | Severity | Resolution |
|---|---|---|---|
| RC-1 | Order creation has no client idempotency key. | High | Add user+client-request uniqueness contract. |
| RC-2 | Compensation is an in-process endless loop; errors are discarded. | High | Add lease, shutdown and alerting workflow. |
| RC-3 | Refund recording does not reverse entitlements. | High | Separate refund-initiation/reversal spec. |
| RC-4 | Redis access-token read error blocks fallback to in-memory token/API. | Medium | Define degraded cache behavior. |
| RC-5 | Payment/config implementation is uncommitted working-tree code. | Medium | Review/migrate/deploy before `Review` status. |

## Test Cases

| ID | Trigger | Expected |
|---|---|---|
| TC-01 | Valid mapped product/session | Pending order + signed response + invoice/payment request in one commit. |
| TC-02 | Client sends forged amount/benefits | Ignored; server snapshot controls amount/rights. |
| TC-03 | Expired session | 401, no committed order. |
| TC-04 | Duplicate successful callback/query | One grant and one set of ledgers. |
| TC-05 | Amount/product/quantity/env/OpenID mismatch | No grant; failed event observable. |
| TC-06 | Grant fails after paid confirmation | `PAID + entitlement FAILED`; no partial ledger commit. |
| TC-07 | Compensation retries failed grant | Same order reaches `SUCCESS` without double credit. |
| TC-08 | Pending order exceeds 30m | `CLOSED`; coupon reservation cancelled. |
| TC-09 | Callback body/signature invalid | 400/401, no state transition. |
| TC-10 | WeChat query times out | Existing order preserved; later retry succeeds. |
| TC-11 | Concurrent compensation workers | Claim/idempotency prevents double grant. |
| TC-12 | Refund success/failure callback | Refund/commercial projection updated; no automatic entitlement reversal claimed. |

## Assumptions and open questions

| Assumption | Evidence | Risk |
|---|---|---|
| PostgreSQL is the required payment store. | API is unavailable without `postgresStore`. | Non-Postgres deployments cannot pay. |
| Mini-program client polling is ten 1.5s iterations. | Current page implementation. | May drift; no server SLA. |

Open questions: Who initiates official refunds? What is the entitlement reversal policy? What alerts on compensation failures? What idempotency key should the mini program persist across ambiguous submits?

## Spec vs Reality Audit Log

| Date | Finding | Action |
|---|---|---|
| 2026-07-15 | Read current routes, payment/entitlement code, migrations, config, UI and tests. | Created Draft; surfaced missing refund and durable compensation workflows. |
