# Follow-up: `pointUnitAmountCents` 10 → 1

Suggested branch (do **not** create on `fix/text-to-image-params`): `fix/billing-point-cny-conversion`

This branch only records the defect. **Do not change `pointUnitAmountCents` here.**

## Locked commercial rule

- 1 RMB = 100 points
- 1 point = 0.01 RMB = **1 RMB cent**

If `pointUnitAmountCents` means “how many RMB cents one point is worth”, the correct value is **1**, not 10, and not 100.

Current code (`backend-go/internal/httpserver/store.go`):

```
pointUnitAmountCents = 10
```

That encodes **1 point = 0.10 RMB**, which is 10× the official wallet grant rate (WeChat recharge: 1 RMB → 100 points).

GPT Image user SKUs on this branch are priced in **points** (10 / 55 / 220). They must **not** use `pointUnitAmountCents=10` as the selling-price basis.

## What is already correct (do not migrate)

### A. User wallet point balances

- Grant: 1 RMB → 100 points
- Deduct: integer points (`QuotedPoints` / `ReservedPoints` / `CapturedPoints`)
- These ledgers should **not** be rescaled when fixing the CNY conversion constant

### B. GPT Image Phase-1 SKU

- Charged as points × `n`
- Independent of the cents conversion bug until Billing Center shows CNY

## What must be audited before 10 → 1

These fields multiply **points × `pointUnitAmountCents`** (or divide by 100 for CNY). Changing 10 → 1 without a migration will shrink displayed RMB and margin math by 10×, while leaving wallet points unchanged.

| Area | Typical fields | Risk |
| --- | --- | --- |
| Task snapshot | `UserChargeAmount` = `PointCost * pointUnitAmountCents` | Historical RMB 10× too high |
| Billing events / wallet ledger CNY | `AmountCents`, `UnitAmountCents` | Same 10× |
| Margin | `EstimatedMargin`, `PlatformProfit` = charge − upstream cost | False profit / false `NEGATIVE_MARGIN` |
| Billing Center validate | `minimumRevenue = max(basePrice, minimumCharge) * pointUnitAmountCents / 100` | With 10, 10 points look like 1.00 CNY; with 1 they look like 0.10 CNY |
| Provider cost compare | `UnitCost` in CNY vs converted user price | GPT Image `pcost_openai_gpt_image_2` is 0.60 CNY/image (stale DALL·E-style) |
| Admin list APIs | `amountCents`, `minAmountCents` | UI RMB |
| Knowledge billing | `UnitAmountCents`, `AmountCents` | Same constant |

## Historical data

- **Do not** rewrite wallet point balances or GPT Image point captures.
- Decide whether historical `*Cents` / `UserChargeAmount` / `EstimatedMargin` rows stay as-is (document as “old conversion”) or get a one-time `/10` migration. Mixing both in the same dashboard will break trends.

## Validate after the conversion branch

1. Recharge 1 RMB still grants 100 points
2. GPT Image low n=1 still deducts 10 points
3. Billing Center CNY for 10 points = **0.10 RMB**, not 1.00 RMB
4. `NEGATIVE_MARGIN` uses the same 1 point = 1 cent rule as finance
5. No double-fix of Seedance / PPT / video SKUs that were tuned against the old 10× CNY display

## GPT Image note

Phase-1 DRAFT prices (10 / 55 / 220) already assume 1 point = 0.01 RMB for **margin planning**. Publishing those point SKUs before fixing `pointUnitAmountCents` will make Billing Center CNY and margin checks look 10× too profitable.
