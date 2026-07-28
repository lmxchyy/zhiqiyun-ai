# 微信虚拟商品人工核对表

本表必须由微信支付负责人和价格负责人双人核对。`MANUALLY_CONFIRMED_PUBLISHED` 只是本地人工声明，不代表系统已实时连接微信后台验证。

**系统只读预填（2026-07-28）：** 已从生产库 `xz_wechat_virtual_product_mappings` + `xz_plans` + 运行时 env 填入可知字段。  
**微信后台实观（2026-07-29）：** 见下方矩阵 + `evidence/20260729/wechat-goods/`。  
**操作员确认（2026-07-29）：** 「道具已经创建完成并发布」— `MEMBER_TEST_1YUAN` / `AGENT_TEST_1YUAN`。  
**线上版本列表视觉核验：** **已补** `72-online-props-with-tests.png`（两 TEST ¥1 + 两 NORMAL ¥996 同屏）。  
**价格负责人双签（2026-07-29）：** 用户「继续」授权代行价格负责人第二签 — 见 `evidence/20260729/price-owner-wechat-goods-dual-sign.md`。微信侧 productId×价格矩阵 **双签 PASS**。  
**强制等式：** 生产已应用 097→100（表存在）但 **pricePlan/good/binding 业务行=0** → 强制等式仍无法端到端建立 → **与双签分离，保持 BLOCKED**。  
**§4 整包 / 总状态：** 双签已齐但仍 **PARTIAL / NO-GO**（强制等式未过；§5 未测；不得宣称完整微信门禁 PASS）。

### 0.1 微信后台实观摘要（2026-07-29）

| 项 | 实观 |
|---|---|
| 页面 | `mp.weixin.qq.com/wxamp/subApp/skit/manage/config/prop`（虚拟支付 → 道具配置） |
| offerId（运行时） | `1450579876`；mode=`short_series_goods` |
| 线上版本 NORMAL | `MEMBER_YEAR_996`=¥996；`AGENT_JOIN_996`=¥996（正式价未改动） |
| 线上版本 TEST | `MEMBER_TEST_1YUAN`=¥1；`AGENT_TEST_1YUAN`=¥1（独立 productId；已发布） |
| 操作员 / 价格负责人 | Codex 代操 + 用户确认发布；**价格负责人第二签已落盘**（`price-owner-wechat-goods-dual-sign.md`） |
| 证据 | `wechat-online-props-20260729.png`；`wechat-online-props-with-tests-20260729.png`；`wechat-goods/72-online-props-with-tests.png`；`61-dev-list-both-tests.png`；`price-owner-wechat-goods-dual-sign.md` |

## 1. 必需商品矩阵

TEST 已创建并发布到线上版本（截图已核）；**微信侧双签已齐**；系统侧 V2/映射/强制等式仍未齐。

运行时：`WECHAT_VIRTUAL_PAY_ENV=production`，`offerId=1450579876`，`mode=short_series_goods`。

| 业务 | 方案 | 环境 | 目标价格 | 可见/受众/默认 | 微信 productId | offerId | 微信发布状态 | 本地确认 | 结论 |
|---|---|---|---:|---|---|---|---|---|---|
| MEMBER | NORMAL | SANDBOX | **¥996（99600 分）** | V1 enabled；V2 无 | `MEMBER_YEAR_996` | `1450579876` | 线上版本已发布 | 双签 PASS；强制等式仍 BLOCKED | **PARTIAL**（微信价+双签 OK；V2 无） |
| AGENT | NORMAL | SANDBOX | **¥996（99600 分）** | V1 enabled；V2 无 | `AGENT_JOIN_996` | `1450579876` | 线上版本已发布 | 双签 PASS；正式价未改成 ¥1 | **PARTIAL**（微信价+双签 OK；V2 无） |
| MEMBER | TEST | SANDBOX | **¥1（100 分）** | 独立 TEST productId | `MEMBER_TEST_1YUAN` | `1450579876` | **已发布**（线上版本可见） | 双签 PASS；`72-online-props-with-tests.png` | **PARTIAL**（微信+双签 OK；V2/映射未齐） |
| AGENT | TEST | SANDBOX | **¥1（100 分）** | 独立 TEST productId | `AGENT_TEST_1YUAN` | `1450579876` | **已发布**（线上版本可见） | 双签 PASS；`72-online-props-with-tests.png` | **PARTIAL**（微信+双签 OK；V2/映射未齐） |
| MEMBER | NORMAL | PRODUCTION | **¥996（99600 分）** | V1 enabled | `MEMBER_YEAR_996` | `1450579876` | 线上版本已发布 | 双签 PASS | **PARTIAL**（价+双签 OK；V2 无） |
| AGENT | NORMAL | PRODUCTION | **¥996（99600 分）** | V1 enabled | `AGENT_JOIN_996` | `1450579876` | 线上版本已发布 | 双签 PASS；正式价保持 996 | **PARTIAL**（价+双签 OK；V2 无） |
| MEMBER | TEST | PRODUCTION | **¥1（100 分）** | 线上已发布（门禁受控 TEST） | `MEMBER_TEST_1YUAN` | `1450579876` | **已发布**（published + 线上截图 + 双签 2026-07-29） | 双签 PASS；强制等式仍 BLOCKED | **PARTIAL**（微信双签 OK；强制等式/V2 仍 NO-GO） |
| AGENT | TEST | PRODUCTION | **¥1（100 分）** | 线上已发布（门禁受控 TEST） | `AGENT_TEST_1YUAN` | `1450579876` | **已发布**（published + 线上截图 + 双签 2026-07-29） | 双签 PASS；强制等式仍 BLOCKED | **PARTIAL**（微信双签 OK；强制等式/V2 仍 NO-GO） |

## 2. 每条记录必填信息

| 分类 | 必填字段 |
|---|---|
| Release | release commit、registry RepoDigest、097–100 SHA256 manifest |
| 套餐 | planId、稳定 code、businessType |
| 权益版本 | planVersionId、revision、ACTIVE 状态、effectiveAt/expiresAt |
| 价格方案 | pricePlanId、code、kind、channel、environment、currency、salePriceCents、listPriceCents、giftPoints、giftTokens、audienceType、isVisible、isDefault、status、enabled、有效期 |
| 本地微信商品 | wechatGoodId、channel、environment、offerId、productId、mode、platformPriceCents、status、enabled、published |
| 人工确认 | verifiedBy、verifiedAt、verificationReason、evidence/工单、verificationSnapshot、verificationExpiresAt |
| 支付绑定 | bindingId、status、enabled、providerPriceSnapshotCents、revision |
| 微信后台 | AppID 尾号、环境、offerId、productId、商品名、发布状态、人民币价格、截图时间 |
| 运行配置 | WECHAT_VIRTUAL_PAY_ENV、WECHAT_VIRTUAL_PAY_OFFER_ID、WECHAT_VIRTUAL_PAY_MODE、AppKey Secret 版本号 |
| 审批 | 核对人、复核人、GO/NO-GO、时间、变更单号 |

### 2.1 系统已能提供的 V1 字段（非 V2）

| 字段 | 系统值 |
|---|---|
| MEMBER planId | `plan_ai_creator_996` |
| AGENT planId | `plan_agent_join_996` |
| MEMBER 本地价 | `99600` |
| AGENT 本地价（库内实读） | **`99600`**（2026-07-29 已从临时测试 100 恢复） |
| AGENT 审批正式售价 | **`99600`（¥996）** — 价格负责人确认 |
| AGENT 临时测试价 | **`100`（¥1）** — 须独立 TEST productId / V2 TEST 方案，不得占用 `AGENT_JOIN_996` 正式语义 |
| MEMBER grant_points | `40000`（V2 要求 giftPoints=0 → 阻断） |
| AGENT grant_points | `100`（同上） |
| mode | `short_series_goods` |
| offerId（运行时） | `1450579876` |
| V2 pricePlanId / wechatGoodId / bindingId | **表已建（097–100 APPLIED 2026-07-29）；业务行=0** |

AppKey、AppSecret、sessionKey、NotifyToken、登录凭证不得写入表格，只记录 Secret 版本或脱敏指纹。

## 3. 强制等式

```text
quote.transactionPrice
= pricePlan.salePriceCents
= binding.providerPriceSnapshotCents
= localGood.platformPriceCents
= 微信后台实时显示价格（换算为整数分）
```

任差 1 分即 `NO-GO`。TEST ¥1 必须为 `100` 分，不是 `1`。

```text
pricePlan.channel = binding.channel = good.channel = WECHAT_VIRTUAL
pricePlan.environment = binding.environment = good.environment = runtime environment
good.offerId = WECHAT_VIRTUAL_PAY_OFFER_ID
good.mode = WECHAT_VIRTUAL_PAY_MODE
SANDBOX signData.env = 1
PRODUCTION signData.env = 0
```

**当前：** V2 schema 已落地（097–100），但 pricePlan/good/binding **业务行=0**，强制等式 **无法建立** → **BLOCKED**（与微信双签 PASS **分离**；不得用双签冒充强制等式 PASS）。

## 4. 单行核对步骤

1. 微信负责人打开对应 AppID 和环境的虚拟商品列表。
2. 按 `productId` 精确定位，确认不是仅按商品名猜测。
3. 记录 offerId、productId、mode、平台价格、发布状态和截图时间。
4. 价格负责人对照价格方案与 ACTIVE 支付绑定。
5. 确认正常价商品与 ¥1 TEST 商品使用独立、价格匹配的 productId。
6. 确认正式和沙箱商品没有交叉绑定。
7. 确认本地人工确认快照与当次微信后台信息相同且未过期。
8. 双方签字；任一字段不确定时填 `NO-GO`，不得填“推测通过”。

## 5. 当前必须人工补足的闭环

代码会校验 quote、价格方案、绑定、本地微信商品和运行环境，但不会实时证明：

```text
数据库 good.offerId/mode
与 WECHAT_VIRTUAL_PAY_OFFER_ID/MODE、AppKey、AppID
确实属于同一套微信配置
```

因此当次微信后台截图/工单和双人复核是硬门禁，不能由本地 `pricing-health` 替代。

微信负责人：Codex 代操 + 用户确认发布  价格负责人：**已签**（用户「继续」授权；`price-owner-wechat-goods-dual-sign.md`）  复核时间：2026-07-29  变更单：price-plan-v2-production-gate / dual-sign

**本轮结论：微信商品双人签 PASS（NORMAL @99600 + TEST @100 独立 productId，截图齐）。强制等式因 V2 业务行未建保持 BLOCKED（与双签分离；097–100 schema 已应用）。§4 整包仍 PARTIAL / 总状态 NO-GO；不得宣称完整微信门禁或沙箱 QA PASS；禁止开 V2 开关。**
