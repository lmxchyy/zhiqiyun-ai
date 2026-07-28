# deliver-notify remediation（2026-07-29）

**Checked:** 2026-07-29 06:32–06:37 +0800  
**Scope:** WeChat virtual-pay 「未发货」ack for already-fulfilled TEST orders  
**No refund. No plan price changes. V2 flags kept true.**

## Orders

| Role | Merchant order | WeChat txn | wx_order_id | Local | notify_provide_goods |
|---|---|---|---|---|---|
| MEMBER TEST | `ZQY202607282159389857812495` | `4500000265202607293548593890` | `VPO260729055939046718422` | PAID/SUCCESS | **API errcode=0 OK** |
| AGENT TEST | `ZQY20260728221656E339AB7A54` | `4500000301202607297737425043` | `VPO260729061656022982037` | PAID/SUCCESS | **API errcode=0 OK** |

## What was fixed

1. **Code:** after non-push fulfillment (`query_order_paid` / admin grant), call `/xpay/notify_provide_goods` with `pay_sig` (idempotent via `xz_payment_events`). Push `xpay_goods_deliver_notify` path still skips (official: push success already marks shipped).
2. **Admin:** `POST /api/v1/admin/payment/virtual/orders/:orderNo/notify-provide-goods` (no re-grant).
3. **Oneshot:** `tools/notify-provide-goods-oneshot.sh` (prod Python 3.6 compatible) — used to backfill the two orders **before** waiting for image rebuild.
4. **Deploy:** image `local/xianzhi-ai-platform:a39485ef1` @ commit `a39485ef159dabf348a71059a0e922af4894ab5a`; container healthy; V2 three flags **true**; `WECHAT_VIRTUAL_PAY_ENV=production`.

## Evidence (this directory)

| File | Meaning |
|---|---|
| `oneshot-1785277952.json` | WeChat API responses for both orders (`errcode=0`) |
| `probe-db.out` | Local orders still PAID/SUCCESS; payment events include `notify_provide_goods` SUCCESS |
| `redeploy.meta` | Redeploy to correct tip + V2 flags true in container |
| `verify-deliver-status.py` | Attempted `query_order` status=4 check (blocked: openid mismatch) |
| `query-order-after.json` | query_order failed with `268490001 openid错误` — **not** used as console PASS |
| `operator-console-confirm.txt` | Operator text: console shows 已发货 for both TEST orders (no screenshot) |

## Honest WeChat-side verdict

| Check | Result |
|---|---|
| `/xpay/notify_provide_goods` HTTP/API | **PASS** (both orders `errcode=0`, `errmsg=OK`) |
| Local `xz_payment_events` | **PASS** (`notify_provide_goods` SUCCESS ×2) |
| `query_order` → status=4 | **NOT VERIFIED** (openid from prior evidence string rejected by WeChat) |
| WeChat MP console 「未发货」→「已发货」 | **CLOSED** — operator text confirm 2026-07-29（无截图；见 `operator-console-confirm.txt`） |

**Operator confirmation (2026-07-29):** User confirmed WeChat console now shows **已发货** for both deliver-notify orders (MEMBER + AGENT ¥1 TEST). No screenshot was pasted — recorded as text only; do not invent image evidence.

## Deploy note

First deploy attempt used a **stale** `origin/` tip (`f8a7632`) and briefly ran an image without V2 flag env wiring. Immediately redeployed pinned `a39485ef1` with flags forced true and verified in `docker inspect`.

## Remaining gaps (unchanged gate)

- ~~WeChat console 「未发货」FLAG~~ → **CLOSED** (operator text)
- NORMAL ¥996 real-device matrix
- sandbox runtime / pay env still production
- RepoDigest still empty
- §5 whole pack still PARTIAL / total **NO-GO**
