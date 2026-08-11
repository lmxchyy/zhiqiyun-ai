# NORMAL ¥996 — WAIVED real pay / SUBSTITUTED config+quote checks

**Policy (2026-07-29):** User directed stop requiring real ¥996 production payment.  
Payment pipe already proven by ¥1 TEST. NORMAL @99600 validated without charging.

## Verdict

| Item | Status |
|---|---|
| Real-device NORMAL ¥996 pay | **WAIVED** — do not ask user to pay ¥996 |
| Payment technical chain | **ACCEPTED** via ¥1 TEST (see below) |
| NORMAL @99600 package/config | **PASS (substituted)** — dry quote + force-equality bindings |

## Payment path ACCEPTED (¥1 covers pipe)

| Order | Product | Result |
|---|---|---|
| `ZQY202607282159389857812495` | `MEMBER_TEST_1YUAN` @100 | PAID/FULFILLED/SUCCESS + deliver-notify CLOSED |
| `ZQY20260728221656E339AB7A54` | `AGENT_TEST_1YUAN` @100 | PAID/FULFILLED/SUCCESS + deliver-notify CLOSED |

Evidence: `../member-test-pay/`, `../agent-test-pay/`, `../deliver-notify/`.

Same code path as NORMAL for: quoteId → create order → `wx.requestVirtualPayment` → callback/query_order → V2 snapshot fulfill → token/entitlement → deliver-notify.

## What ¥1 cannot cover (honest residual) — covered without charging

| Business point | Why ¥1 ≠ NORMAL | How covered without ¥996 charge |
|---|---|---|
| Exact NORMAL `productId` | TEST uses `MEMBER_TEST_1YUAN` / `AGENT_TEST_1YUAN` | DB binding: NORMAL → `MEMBER_YEAR_996` / `AGENT_JOIN_996` @99600 ACTIVE (force-eq below) |
| Default public pricePlan | TEST is non-default hidden | PRODUCTION NORMAL defaults `pp_member_normal_prod_996` / `pp_agent_normal_prod_996` is_default+enabled |
| Quote amountCent=99600 | TEST quotes 100 | Dry quote **201 @99600** PRODUCTION (+ SANDBOX window) |
| Commission base on 99600 | TEST commission base was 100 | Entitlement snapshot on NORMAL quote shows token/days; force-eq sale=good=99600. Full commission settlement on a 99600 paid order **not** re-run (would need charge or sandbox paid NORMAL — not required under this policy) |

## Dry quote re-verify (2026-07-29 ~07:34 +08)

```text
POST /api/v1/payment/price-quotes MEMBER NORMAL → 201 amountCent=99600 env=PRODUCTION testOnly=false
POST /api/v1/payment/price-quotes AGENT NORMAL  → 201 amountCent=99600 env=PRODUCTION testOnly=false
```

Files: `host-out/quote-member-normal.json`, `host-out/quote-agent-normal.json`, `host-out/03-quotes.txt`.

## Force-equality NORMAL bindings

```text
PRODUCTION pp_member_normal_prod_996 → MEMBER_YEAR_996 @99600 ACTIVE binding
PRODUCTION pp_agent_normal_prod_996  → AGENT_JOIN_996  @99600 ACTIVE binding
SANDBOX    pp_member_normal_sbx_996  → MEMBER_YEAR_996 @99600 ACTIVE binding
SANDBOX    pp_agent_normal_sbx_996   → AGENT_JOIN_996  @99600 ACTIVE binding
```

## Explicit non-actions

- No production ¥996 charge requested or invented
- No refund
- Official 996 prices unchanged
- Mine-page AGENT CTA for already-agents (“进入代理中心”) noted earlier — irrelevant under WAIVE
