# 微信虚拟商品人工核对表

本表必须由微信支付负责人和价格负责人双人核对。`MANUALLY_CONFIRMED_PUBLISHED` 只是本地人工声明，不代表系统已实时连接微信后台验证。

**系统只读预填（2026-07-28）：** 已从生产库 `xz_wechat_virtual_product_mappings` + `xz_plans` + 运行时 env 填入可知字段。  
微信后台「发布状态 / 实时价截图」仍为待人工。详见 `evidence/20260728/system-snapshot.md`。  
**V2 表未建** → 无 V2 pricePlan/good/binding；本矩阵按 **现行 V1 映射** 填写，**结论默认 NO-GO**。

## 1. 必需商品矩阵

生产 TEST 商品默认不创建、不启用；若未来确需生产受控 TEST，必须另开审批单。

运行时：`WECHAT_VIRTUAL_PAY_ENV=production`，`offerId=1450579876`，`mode=short_series_goods`。

| 业务 | 方案 | 环境 | 目标价格 | 可见/受众/默认 | 微信 productId | offerId | 微信发布状态 | 本地确认 | 结论 |
|---|---|---|---:|---|---|---|---|---|---|
| MEMBER | NORMAL | SANDBOX | 本地 `99600` 分；微信后台价 **待人工核** | V1 映射 enabled=true；V2 无 | `MEMBER_YEAR_996` | `1450579876`（运行时） | 待人工核 | 系统有映射；`payment_product_code=MEMBER_PRO_YEAR_996`≠productId | **NO-GO**（待截图+码一致） |
| AGENT | NORMAL | SANDBOX | 本地 `price_cents=100`；微信 product=`AGENT_JOIN_996` 价 **待人工核** | V1 enabled=true；V2 无 | `AGENT_JOIN_996` | `1450579876` | 待人工核 | 本地 100 分 vs 996 命名道具，疑价格差 | **NO-GO** |
| MEMBER | TEST | SANDBOX | 系统 **无** 会员 ¥1 独立 productId | 无 V2 TEST 方案 | 系统无 | — | — | 仅有 Token `TOKEN_TEST_1FEN`，不属会员 | **NO-GO** |
| AGENT | TEST | SANDBOX | 系统 **无** 代理 ¥1 独立 productId（代理正式价已被改成 100 分，更危险） | 无独立 TEST productId | 系统无（勿复用 `AGENT_JOIN_996`） | — | — | 禁止把正式道具当 TEST | **NO-GO** |
| MEMBER | NORMAL | PRODUCTION | 本地 `99600`；微信后台价 **待人工核** | V1 enabled=true | `MEMBER_YEAR_996` | `1450579876` | 待人工核 | 同 SANDBOX 码不一致问题 | **NO-GO** |
| AGENT | NORMAL | PRODUCTION | 本地 `100` 分；微信 `AGENT_JOIN_996` 价 **待人工核** | V1 enabled=true | `AGENT_JOIN_996` | `1450579876` | 待人工核 | 强制等式高风险失败 | **NO-GO** |
| MEMBER | TEST | PRODUCTION | `N/A` | disabled | 不创建 | 不创建 | N/A | N/A | 默认 NO-GO |
| AGENT | TEST | PRODUCTION | `N/A` | disabled | 不创建 | 不创建 | N/A | N/A | 默认 NO-GO |

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
| AGENT 本地价 | `100`（异常，需业务确认是否误改） |
| MEMBER grant_points | `40000`（V2 要求 giftPoints=0 → 阻断） |
| AGENT grant_points | `100`（同上） |
| mode | `short_series_goods` |
| offerId（运行时） | `1450579876` |
| V2 pricePlanId / wechatGoodId / bindingId | **系统无（表未创建）** |

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

**当前：** V2 对象不存在，强制等式 **无法建立** → NO-GO。

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

微信负责人：__________  价格负责人：__________  复核时间：__________  变更单：__________

**协调人预填结论：矩阵 6 行可测业务均为 NO-GO（系统证据）。** 人工截图只能确认或加重阻断，不能在 V2 表缺失时改成 PASS。
