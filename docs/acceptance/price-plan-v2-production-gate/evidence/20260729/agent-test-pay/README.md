# AGENT TEST ¥1 真机支付复核（PRODUCTION）

**Checked:** 2026-07-29 06:18–06:23 +0800  
**User:** `user_000002` / `demo@xianzhi.ai`  
**Merchant order:** `ZQY20260728221656E339AB7A54`  
**WeChat txn:** `4500000301202607297737425043`  
**WeChat order:** `VPO260729061656022982037`  
**Product:** `AGENT_TEST_1YUAN` @100 分 / `price_plan_20260728212634000000000_2ec1c485`  
**V2 flags:** left unchanged (`true/true/true`, `WECHAT_VIRTUAL_PAY_ENV=production`)

## Verdict（诚实）

| 维度 | 结论 | 说明 |
|---|---|---|
| 用户侧支付 | **PASS** | 用户确认已支付；系统 `paid_at=2026-07-28T22:17:06Z`（+0800 06:17:06）；微信交易号已写入 |
| 系统内 V2 履约 / GrantOrderEntitlements | **PASS** | `status=PAID`，`fulfillment_status=FULFILLED`，`entitlement_status=SUCCESS`，`snapshot_version=2` |
| 代理权益到账 | **PASS** | `agent_status=ACTIVE`；`xz_user_business_identities` AGENT ACTIVE；`xz_channel_agents`/`xz_agent_profiles` level=2 ACTIVE；Token +20000（钱包余额 60793） |
| 微信推送回调 `xpay_goods_deliver_notify` | **MISSING** | 仅有 `query_order_paid`；`callback_payload={}` / `notify_payload={}`（与 MEMBER 同路径） |
| 微信控制台「未发货」 | **FLAG（预期同 MEMBER）** | 查单补偿履约后未调用 `/xpay/notify_provide_goods`；不等于用户未到账 |
| MEMBER TEST ¥1 | **PASS（此前）** | `ZQY202607282159389857812495` |
| §5 整包 | **PARTIAL** | MEMBER+AGENT TEST 内部履约均 PASS；仍缺 NORMAL 996、sandbox、幂等专项、RepoDigest、发货 ack |

**禁止**把本目录记为「沙箱真机全矩阵 PASS」或「§5 / 总门禁 GO」。

## Timeline（UTC → +0800）

| 事件 | UTC | +0800 |
|---|---|---|
| quote 创建 `quote_cf9092a39f600f9694868f5d` | 22:16:52Z | 06:16:52 |
| 下单 | 22:16:56Z | 06:16:56 |
| 支付成功 `paid_at` | 22:17:06Z | 06:17:06 |
| 查单补偿 + 履约 `fulfilled_at` | 22:19:48Z | 06:19:48 |

## Key evidence fields

```text
order:              ZQY20260728221656E339AB7A54
status:             PAID
fulfillment:        FULFILLED
entitlement:        SUCCESS
snapshot_version:   2
payment_environment:PRODUCTION
wechat_product_id:  AGENT_TEST_1YUAN
amount_cents:       100
price_plan_id:      price_plan_20260728212634000000000_2ec1c485
price_quote_id:     quote_cf9092a39f600f9694868f5d (CONSUMED, entry_type=TEST)
wechat_tx:          4500000301202607297737425043
wx_order_id:        VPO260729061656022982037
payment_event:      query_order_paid (ONLY) — official query compensation
agent_status:       ACTIVE
agent_identity:     AGENT ACTIVE (source_order_id=本单, commission_enabled=true)
channel_agent:      channel_000002 level=2 ACTIVE join_order_id=本单
token_grant:        AGENT_JOIN_GRANT +20000 ; wallet token_balance=60793 ; total_token_granted=60000
commission:         commission_platform 100分 RECORDED
openid (prior):     oCTyT7ZlaHEPWG2bPDm0d0Ok6Q5Y (MEMBER 控制台；本单未另截图)
```

## 与 MEMBER 对照

| | MEMBER TEST | AGENT TEST |
|---|---|---|
| order | `ZQY202607282159389857812495` | `ZQY20260728221656E339AB7A54` |
| productId | `MEMBER_TEST_1YUAN` | `AGENT_TEST_1YUAN` |
| amount | 100 | 100 |
| 确认路径 | `query_order_paid` | `query_order_paid` |
| deliver-notify | 无 | 无 |
| 权益 | member PRO + Token 40000 | agent ACTIVE + Token 20000 |

## 微信「未发货」含义（对本系统）

与 MEMBER 证据相同：本地已履约；微信侧缺发货确认。建议后续补 `/xpay/notify_provide_goods`；本轮**未退款**、**未改 V2 开关**。

## Probe outputs

- `probe-reverify.out` / `probe-reverify.sh`
- `summary.json`

## Remaining（非本窗 PASS）

- MEMBER/AGENT NORMAL ¥996 真机
- sandbox 运行时（当前仍 production）
- 幂等/并发回调专项、白名单失效、价格差 1 分等
- RepoDigest
- `notify_provide_goods` 代码/运维补齐
