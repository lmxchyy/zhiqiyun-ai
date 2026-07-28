# V2 三开关启用窗口证据（2026-07-29）

| 项 | 值 |
|---|---|
| 授权 | 用户明确授权「明确授权开」 |
| 主机 | `root@119.29.191.227` `/opt/zhiqiyun-ai` |
| 容器 | `zhiqiyun-ai-prod-xianzhi-ai-1` |
| Compose | `compose.prod.yml` + `.env.production` |
| 支付环境 | **保持** `WECHAT_VIRTUAL_PAY_ENV=production`（**未**切 sandbox） |
| 正式价基线 | MEMBER/AGENT default + goods **全程 99600**（未改价） |

## 开关翻转时间（Asia/Shanghai）

| 顺序 | 变量 | FLIP_AT | 翻转后容器实读 | health |
|---:|---|---|---|---|
| 1 | `SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=true` | **2026-07-29 05:34:54 +0800** | true / false / false / production | healthy |
| 2 | `PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=true` | **2026-07-29 05:35:11 +0800** | true / true / false / production | healthy |
| 3 | `PRICE_PLAN_TEST_ENTRY_ENABLED=true` | **2026-07-29 05:35:27 +0800** | true / true / true / production | healthy |

每次：改 `.env.production` → `docker compose -f compose.prod.yml --env-file .env.production up -d --no-deps --force-recreate xianzhi-ai` → 等 healthy → `printenv` 校验。

最终容器：`true true true production`；`docker inspect` **healthy**。

## Quote / health 探测（只读/干跑，未下单）

| 探测 | 结果 |
|---|---|
| `GET /api/v1/health` | 200 |
| `GET /healthz` | 200 |
| `POST /api/v1/payment/price-quotes` 无鉴权 | 401 unauthorized |
| MEMBER NORMAL quote（用户 session 注入，**不下单**） | **201**；`amountCent=99600`；`environment=PRODUCTION`；`testOnly=false`；quoteId 已哈希脱敏 |
| AGENT NORMAL quote（同上） | **201**；`amountCent=99600`；`environment=PRODUCTION`；`testOnly=false` |
| `GET /api/v1/admin/pricing-health` | 200；`summary.blockedIssueCount=2`；`status=BLOCKED` |
| pricing-health 阻断原因 | 两条 `TEST_WHITELIST_MISSING`（MEMBER/AGENT TEST 无有效白名单成员） |
| NORMAL pricePlan health | MEMBER/AGENT NORMAL 均为 **HEALTHY** @99600 |
| runtime flags（health） | creation/test/fulfillment 均为 true；`v132Blocked=false`；affectedTenantCount=0 |

**未做：** 微信真机支付、sandbox 切环境、TEST quote（缺白名单）、订单创建/回调/履约。

## 沙箱真机剩余动作（人工）

按 `sandbox-v2-quote-real-device-acceptance.md`：完整沙箱验收要求 `WECHAT_VIRTUAL_PAY_ENV=sandbox` + sandbox AppKey/offer + SANDBOX env 商品行。

**本轮未切支付环境**（避免生产误收沙箱/正式交叉风险）。若要做沙箱真机：

1. 另开变更窗：临时将运行时切 `sandbox`（或独立沙箱栈），确认 sandbox 商品/绑定存在。
2. 为 TEST 方案写入有效白名单（清掉 `TEST_WHITELIST_MISSING`）。
3. 体验版真机走 quoteId → `wx.requestVirtualPayment`；测完按事故回退顺序收回 TEST/创建（履约可保持 true 若有在途单）。
4. **禁止**把本目录的 dry quote 201 记为「沙箱真机 PASS」。

## 文件清单

- `enable-v2-flags.sh` — 生产启用脚本（逐步 0/1/2/3/probe）
- `01-fulfillment.txt` / `02-creation.txt` / `03-test-entry.txt`
- `flip{1,2,3}-*-at.txt`
- `04-probe.txt` / `pricing-health.json`
- `quote-member-normal.json` / `quote-agent-normal.json`（quoteId 脱敏）
- `0*-prices-*.txt` — 价格基线校验

宿主机密钥备份仅存服务器 `backups/compose/.env.production.pre-v2-flags-*.bak`；**证据目录不含完整 .env**。
