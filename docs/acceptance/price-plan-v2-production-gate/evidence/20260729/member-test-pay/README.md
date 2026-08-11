# MEMBER TEST ¥1 真机支付复核（PRODUCTION）

**Checked:** 2026-07-29 06:02–06:05 +0800  
**User:** `user_000002` / `demo@xianzhi.ai`  
**Merchant order:** `ZQY202607282159389857812495`  
**WeChat txn:** `4500000265202607293548593890`  
**Product:** `MEMBER_TEST_1YUAN` @100 分 / `price_plan_20260728212634000000000_049a91b1`  
**V2 flags:** left unchanged (`true/true/true`, `WECHAT_VIRTUAL_PAY_ENV=production`)

## Verdict（诚实）

| 维度 | 结论 | 说明 |
|---|---|---|
| 用户侧支付 | **PASS** | 微信控制台「支付成功」；实付 ¥1.00；支付时间 2026-07-29 05:59:58 |
| 系统内 V2 履约 / GrantOrderEntitlements | **PASS** | `status=PAID`，`fulfillment_status=FULFILLED`，`entitlement_status=SUCCESS`，`snapshot_version=2` |
| 会员权益到账 | **PASS** | `member_level=PRO`；membership 记录至 2027-07-28；Token +40000（余额 40793） |
| 微信推送回调 `xpay_goods_deliver_notify` | **MISSING** | `xz_payment_events` 仅有 `query_order_paid`；`callback_payload={}` / `notify_payload={}` |
| 微信控制台「未发货」 | **FLAG（非用户权益失败）** | 官方：正常应在 deliver-notify 返回成功；异常时调 `/xpay/notify_provide_goods`。本仓**无**该 API 调用；查单补偿履约后未回写微信发货态 |
| AGENT TEST ¥1 | **未测** | 近 2h 无 `AGENT_TEST_1YUAN` 订单 |
| §5 整包 | **PARTIAL** | 仅 MEMBER TEST 内部履约证明；缺 push 发货确认 + AGENT + 其余矩阵 |

**禁止**把本目录记为「沙箱真机全矩阵 PASS」或「§5 GO」。

## Timeline（UTC → +0800）

| 事件 | UTC | +0800 |
|---|---|---|
| quote 创建 `quote_cc2367c46d222553b85a53c5` | 21:58:56Z | 05:58:56 |
| 下单 | 21:59:38Z | 05:59:38 |
| 支付成功 `paid_at` | 21:59:58Z | 05:59:58 |
| 查单补偿 + 履约 `fulfilled_at` | 22:01:48Z | 06:01:48 |

## Key evidence fields

```text
order:              ZQY202607282159389857812495
status:             PAID
fulfillment:        FULFILLED
entitlement:        SUCCESS
snapshot_version:   2
payment_environment:PRODUCTION
wechat_product_id:  MEMBER_TEST_1YUAN
amount_cents:       100
price_plan_id:      price_plan_20260728212634000000000_049a91b1
price_quote_id:     quote_cc2367c46d222553b85a53c5 (CONSUMED, entry_type=TEST)
wechat_tx:          4500000265202607293548593890
wx_order_id:        VPO260729055939046718422
payment_event:      query_order_paid (ONLY) — official query compensation
membership:         PRO 2026-07-28 → 2027-07-28 ; source_order_no=本单
token_grant:        MEMBER_PACKAGE_GRANT +40000
openid (console):   oCTyT7ZlaHEPWG2bPDm0d0Ok6Q5Y
```

## 微信「未发货」含义（对本系统）

1. **不等于**用户没拿到权益 —— 本地已 `FULFILLED` / `SUCCESS`。
2. **等于**微信侧尚未收到「已发货」确认：本单未见 `xpay_goods_deliver_notify` 成功处理记录；查单履约路径未调用 `notify_provide_goods`。
3. **风险（官方社区）**：长期未发货可能影响结算感知、用户侧退款便利；应补推发货确认或修补偿路径。
4. **本轮动作**：记录为 **FLAG / 技术债**；**未退款**；**未改 V2 开关**；未在生产盲调发货 API（缺现成运维脚本且需 pay_sig）。

### 建议后续（非本窗已执行）

- 代码：查单补偿 `confirmPaidAndGrant` 成功后，若无 deliver-notify 事件，调用 `/xpay/notify_provide_goods`（`order_id` 或 `wx_order_id` + `env=0`）。
- 运维：对本单一次性调用 `notify_provide_goods`，再在微信控制台确认从「未发货」消失。
- 验收：再测 AGENT ¥1；观察是否出现 push notify 或仍走查单。

## Screenshots

| File | Source |
|---|---|
| `user-dialog-pay-returned.png` | 小程序「支付请求已返回」 |
| `user-dialog-alt.png` | 同会话备用截图 |
| `wechat-console-virtual-order-paid.png` | 微信「虚拟支付订单」现网；支付成功；筛选含「未发货」 |

## Probe outputs

- `probe-orders.out` / `.sh`
- `probe-fulfillment.out` / `.sh`
- `probe-ledgers.out` / `.sh`
- `probe-reverify.out` / `.sh`（与控制台单号交叉复核）

## AGENT remaining

- §5 用例 #4 AGENT TEST ¥1：**未测**
- NORMAL 996 真机、幂等回调、白名单失效等：**未测**
