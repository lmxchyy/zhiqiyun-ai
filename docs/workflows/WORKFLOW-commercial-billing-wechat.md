# WORKFLOW: 商业计费与微信虚拟支付同步

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 微信虚拟订单驱动的客户、优惠券、订阅、账单、付款请求、贷项与催收投影。

## Overview

本工作流把真实微信虚拟支付订单投影为商业计费对象。优惠券只增加权益、绝不折扣微信商品金额；支付/关闭/退款事件同步账单和付款请求，管理员可管理订阅、税票状态、贷项审核与人工催收记录，但贷项审核不等于微信退款。

## Evidence Map

| Surface | Source |
|---|---|
| Admin routes/UI | `server.go` commercial billing routes, `commercial_billing.go`, `CommercialBillingCenter.vue` |
| WeChat synchronization | `commercial_billing_wechat.go`, `wechat_virtual_entitlements.go` |
| Schema/backfill | `050-commercial-billing-wechat.sql` |
| Client | `admin-vue/src/api/commercialBilling.ts` |

## Actors, prerequisites and trigger

Actors: billing admin, virtual-payment user, virtual payment service, PostgreSQL, WeChat, finance/operator. Prerequisites: migrations 047 and 050 applied; admin RBAC; virtual order has immutable amount/product snapshot; coupon benefits limited to credits/image quota/membership days. Trigger: coupon/admin actions, virtual-order transaction, payment confirmation, order close, refund notification, or dunning action.

## Workflow Tree

### STEP 1: Configure an entitlement-bonus coupon

**Action**: Admin creates/updates coupon code, benefit, applicable products, redemption limits, active window and status.  
**Timeout**: No endpoint-specific deadline; request context and DB control it.  
**Success**: coupon `DRAFT/ACTIVE/INACTIVE/EXPIRED` persisted -> STEP 2.  
**Failures/recovery**: invalid/zero benefit, status, limit or date constraint -> 400 and no write; duplicate code -> constraint error; unauthorized -> 403; ambiguous timeout -> list coupon before retry.  
**Observable**: operator sees row/status/counts; customer sees only eligible active coupons; DB records creator/time; mutation has no dedicated business audit row.
**Test cases**: active/draft/expired coupon; invalid/zero benefit; duplicate code; usage limits; authorization; ambiguous write response.

### STEP 2: Reserve coupon and create commercial order projections

**Action**: During the same virtual-order DB transaction, service validates eligibility and per-user/global limits, reserves redemption, inserts one invoice and one payment request keyed by the order. Coupon does not alter `amount_cents`.  
**Timeout**: Parent order request context; payment request expires with the 30-minute order.  
**Success**: invoice=`FINALIZED/PENDING`, payment request=`PENDING/NOT_STARTED`, coupon=`RESERVED` -> STEP 3.  
**Failures/recovery**: coupon expired/not applicable/limit exceeded -> order creation rejected or coupon cleared by caller policy; any projection insert error -> whole order transaction rolls back; concurrent redemption -> DB count/unique constraints, but limit check is not visibly protected by coupon-level row lock; duplicate order -> unique order projection prevents duplicate.  
**Observable**: customer sees order/coupon selection; operator sees invoice/payment request/reservation; DB total equals original price.
**Test cases**: eligible/ineligible coupon; concurrent final redemption; rollback at each projection insert; duplicate order projection; assert price is not discounted.

### STEP 3: Synchronize paid order, invoice, payment and subscription

**Action**: Inside entitlement-grant transaction, service marks invoice/payment request paid/resolved and upserts subscription from membership entitlement records; coupon bonus is granted and redemption becomes `APPLIED`.  
**Timeout**: Same callback/query/compensation context; compensation pass maximum 90s.  
**Success**: invoice=`PAID`, payment request=`SUCCEEDED/RESOLVED`, optional subscription=`ACTIVE/EXPIRED`, coupon=`APPLIED` -> STEP 4.  
**Failures/recovery**: projection/grant error rolls back all entitlements and paid projections, then payment workflow persists entitlement `FAILED`; duplicate confirmation is idempotent; partial pre-existing projection is updated by unique order key.  
**Observable**: customer sees granted rights; operator sees commercial records aligned with paid order; DB has no discounted amount; errors appear in payment failure list, not a commercial-specific alert.
**Test cases**: paid synchronization; duplicate confirmation; projection failure rolls back entitlements; subscription/invoice/payment consistency.

### STEP 4: Maintain subscription and tax-invoice state

**Action**: Admin toggles subscription `ACTIVE/CANCELLED` and optional end time; invoice tax state moves to `REQUESTED/ISSUED/REJECTED` with title/number/email/issued number.  
**Timeout**: Request context only.  
**Success**: selected commercial record updated; wallet/entitlement balances are intentionally unchanged.  
**Failures/recovery**: invalid status/body -> 400; missing record -> 404; DB constraint/error -> no update; concurrent edits use last-write-wins because no version field/`If-Match` contract.  
**Observable**: operator sees new status; customer-facing entitlement may diverge from manually reactivated subscription because this endpoint does not update the user membership record.
**Test cases**: valid/invalid subscription transitions; tax request/issue/reject; concurrent edits; verify manual status cannot grant user entitlement.

### STEP 5: Create and review a credit note

**Action**: Admin creates a positive-amount credit note for an invoice with reason; reviewer changes `PENDING_REVIEW` to `FINALIZED` or `REJECTED`.  
**Timeout**: Request context only.  
**Success**: reviewed credit note recorded; UI explicitly states that finalized credit still requires the WeChat refund path -> STEP 6.  
**Failures/recovery**: missing invoice/invalid amount/reason -> 400/404; amount exceeding invoice is not clearly constrained by migration contract; repeat review/concurrent review requires current-status guard inspection; DB failure leaves original invoice untouched.  
**Observable**: operator sees review/refund status separately; customer receives no automatic refund; DB does not create a WeChat refund request.
**Test cases**: create/approve/reject note; invalid/excess amount; concurrent/repeated review; assert no provider refund or entitlement reversal occurs.

### STEP 6: Record manual dunning action

**Action**: For pending payment request, admin records reminder/contact/stop action and channel, increments attempts and updates dunning status.  
**Timeout**: Request context only; no scheduled due-date worker.  
**Success**: append-only dunning event and updated payment request.  
**Failures/recovery**: unsupported action/channel -> 400; non-pending/missing request -> reject; DB failure -> no event/status change if transaction is used by handler; duplicate manual click has no idempotency key and can increment twice.  
**Observable**: operator sees attempt count/event; customer is not actually contacted by this code; no SMS/email sender is invoked.
**Test cases**: every supported action/channel; duplicate manual click; missing/non-pending request; transaction failure; assert no external message is claimed sent.

### STEP 7: Synchronize close or refund

**Action**: Expired/unpaid close sets payment request `CANCELLED/STOPPED` and coupon reservation `CANCELLED`. Refund notification sets order/payment request/refund record; success credits invoice and finalizes a linked credit note, failure retains prior paid state with failed refund status.  
**Timeout**: Callback/query/compensation context.  
**Success**: commercial projections match provider outcome.  
**Failures/recovery**: unknown/refund mismatch -> no mutation; duplicate refund -> idempotency key and unique refund link; projection failure returns provider retry; entitlement reversal remains manual.  
**Observable**: operator sees invoice payment status, credit note and refund record; customer rights may remain granted after monetary refund until a separate reversal process runs.
**Test cases**: unpaid close; signed refund event; duplicate refund; projection failure/provider retry; verify rights remain until an explicit reversal workflow exists.

## State Transitions

```text
coupon: DRAFT -> ACTIVE -> INACTIVE/EXPIRED
redemption: RESERVED -> APPLIED | CANCELLED
subscription: ACTIVE -> EXPIRED | CANCELLED -> ACTIVE (manual)
invoice: FINALIZED/PENDING -> PAID -> CREDITED
payment request: PENDING/NOT_STARTED -> SUCCEEDED/RESOLVED
                 PENDING -> CANCELLED/STOPPED
                 SUCCEEDED -> REFUND_PENDING/REFUNDED
credit note: PENDING_REVIEW -> FINALIZED | REJECTED
refund status: PENDING -> SUCCEEDED | FAILED
```

## Handoff Contracts

| Boundary | Success | Failure/timeout | Recovery |
|---|---|---|---|
| Admin -> coupon/subscription/invoice/credit/dunning | JSON row/status | 400/403/404/500, no specific timeout | reload before retry |
| Virtual order -> commercial projection | invoice/payment request/reservation in same tx | transaction rollback | retry order safely only after checking commit |
| Entitlement grant -> paid projections | paid invoice/request/subscription/coupon | whole grant tx rollback | payment compensation |
| Refund event -> credit projections | refund record + credited/failed state | callback retry | operator reconciliation |

## Cleanup Inventory

| Resource | Cleanup/terminal action | Failure risk |
|---|---|---|
| Coupon reservation | apply or cancel on close | permanent reservation leak |
| Pending payment request | succeed/cancel/refund | stale dunning candidate |
| Subscription | expire/cancel; no scheduler found | ACTIVE past expiry until query interpretation/manual action |
| Credit note | finalize/reject | approved credit without refund |
| Dunning event | append-only | duplicate attempts |

## Reality Checker Findings

| # | Finding | Severity | Resolution |
|---|---|---|---|
| RC-1 | Credit-note approval does not initiate WeChat refund. | High | Add provider refund workflow and permissions. |
| RC-2 | Refund does not reverse entitlements. | High | Add entitlement reversal ledger/spec. |
| RC-3 | No subscription-expiry or automated dunning scheduler was found. | Medium | Add scheduled lifecycle workflows. |
| RC-4 | Manual subscription state can diverge from user membership entitlement. | High | Define authoritative source and synchronization. |
| RC-5 | Coupon limit check/concurrent reservation locking needs an explicit invariant. | Medium | Lock coupon/user scope or use atomic counter constraint. |
| RC-6 | Commercial billing code/migration is uncommitted. | Medium | Review and deploy before promotion beyond Draft. |

## Test Cases

| ID | Trigger | Expected |
|---|---|---|
| TC-01 | Create valid active coupon | persisted and listed. |
| TC-02 | Coupon changes requested payment amount | rejected by design; invoice/order retain product price. |
| TC-03 | Concurrent last coupon redemption | at most allowed count reserved. |
| TC-04 | Order transaction projection insert fails | no order/invoice/request/reservation committed. |
| TC-05 | Paid membership order | invoice paid, request succeeded, subscription and coupon applied atomically. |
| TC-06 | Entitlement projection failure | paid grant transaction rolled back; entitlement failure visible. |
| TC-07 | Close expired order | request/coupon cancelled. |
| TC-08 | Manual subscription cancel/reactivate | only commercial status/end time changes; wallet unchanged. |
| TC-09 | Concurrent invoice edits | document current last-write behavior; add future version test. |
| TC-10 | Create/review credit note | finalization does not claim provider refund. |
| TC-11 | Duplicate dunning click | currently may create two events; regression target for idempotency. |
| TC-12 | Duplicate refund callback | one refund/credit link and stable final state. |
| TC-13 | Refund success | invoice/request credited/refunded; entitlement reversal remains outstanding. |

## Assumptions and open questions

Assumption: commercial tables are projections of `xz_orders`, while entitlement/account ledgers remain the balance authority. Risk: admin subscription edits currently suggest a second authority. Open questions: required invoice/credit approval roles; maximum credit amount; provider refund initiator; entitlement clawback policy; scheduler ownership for expiry/dunning.

## Spec vs Reality Audit Log

| Date | Finding | Action |
|---|---|---|
| 2026-07-15 | Read admin routes/handlers/UI, payment synchronization and migration 050. | Created Draft; kept provider refund and entitlement reversal explicitly out of scope/missing. |
