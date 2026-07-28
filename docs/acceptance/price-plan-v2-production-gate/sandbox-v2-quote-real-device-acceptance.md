# 沙箱真机 V2 Quote 支付验收

本手册描述未来的沙箱验收窗口。本轮不启用开关、不发起支付、不连接微信后台。

`productCode` 旧脚本不能作为 V2 验收证据；V2 必须走服务端 quoteId 链路。

> **2026-07-29 政策更新：** 生产真机付真实 ¥996 **不再要求**。支付链路以 ¥1 TEST ACCEPTED；NORMAL 用 dry quote @99600 + 绑定强制等式；sandbox 临时运行时窗已跑 quote 并恢复 production。详见 `evidence/20260729/POLICY-NO-REAL-996.md`、`normal-996/`、`sandbox-runtime/`。
>
> **2026-07-29 05:36+08 历史：** PRODUCTION V2 对象已建；用户授权后三开关已开（`evidence/20260729/v2-flags-enable/`）。
## 1. 真实链路

```text
POST /api/v1/payment/price-quotes
或 POST /api/v1/payment/test-price-quotes
→ 服务端生成 5 分钟 quote
→ POST /api/v1/payment/wechat-virtual/orders，仅提交 quoteId + 新鲜 wxLoginCode
→ 服务端复核配置、白名单和价值守恒并保存 V2 快照
→ 小程序调用 wx.requestVirtualPayment
→ 微信回调/官方查单
→ V2 快照履约和幂等分润
```

## 2. 前置条件

- 沙箱服务来自冻结 release commit 和镜像 digest。
- 沙箱数据库已执行 097–100，且迁移/约束证据通过。
- `WECHAT_VIRTUAL_PAY_ENV=sandbox`，使用 sandbox AppKey 和沙箱 offer。
- 未来验收窗口按“V2 履约 → V2 创建 → TEST 入口”顺序临时开启；本轮仍全部为 false。
- MEMBER、AGENT 均有 ACTIVE 权益版本。
- 两类套餐均有 NORMAL 和 `100` 分 TEST 方案、独立沙箱微信商品及 ACTIVE 绑定。
- TEST 隐藏、非默认、非 PUBLIC；`giftPoints=0`；目标租户不处于 V132/CANARY 实切范围。
- 准备非白名单账号 U0 和白名单账号 U1。
- 真机微信基础库支持 `wx.requestVirtualPayment`。
- 隐藏页仅通过体验版自定义编译路径进入：

  ```text
  pages/user/UserVirtualPaymentTestPage?planId=<planId>&pricePlanId=<pricePlanId>
  ```

## 3. MEMBER/AGENT NORMAL 正向用例

1. U0 请求普通报价：

   ```http
   POST /api/v1/payment/price-quotes
   Content-Type: application/json

   {"planId":"<memberPlanId>","pricePlanId":"<forged-test-id>"}
   ```

2. 返回必须是公开 NORMAL 默认方案，`testOnly=false`；普通入口不得接受伪造 TEST 方案。
3. U1 作为白名单用户重复普通报价，仍必须返回 NORMAL。
4. 获取新鲜微信登录 code 后创建订单：

   ```http
   POST /api/v1/payment/wechat-virtual/orders
   Content-Type: application/json

   {"quoteId":"<quoteId>","wxLoginCode":"<fresh-code>"}
   ```

5. 解析响应 `signData`，核对：

   ```json
   {
     "offerId": "<sandbox-offer>",
     "buyQuantity": 1,
     "env": 1,
     "currencyType": "CNY",
     "productId": "<member-normal-sandbox-product>",
     "goodsPrice": "<member-normal-price-cents>",
     "outTradeNo": "<orderNo>",
     "attach": "<orderNo>"
   }
   ```

6. 真机拉起并完成沙箱支付。
7. 查询状态并执行一次官方同步：

   ```http
   GET /api/v1/payment/orders/<orderNo>/status
   POST /api/v1/payment/orders/<orderNo>/sync
   ```

8. 确认 `snapshotVersion=2`，会员权益、Token 和分润各一次。
9. 对 AGENT NORMAL 完整重复，确认代理身份和分润各一次。

## 4. MEMBER/AGENT TEST ¥1 正向用例

1. U1 已进入对应 TEST 白名单且有效。
2. 从隐藏入口调用：

   ```http
   POST /api/v1/payment/test-price-quotes
   Content-Type: application/json

   {"planId":"<memberPlanId>","pricePlanId":"<memberTestPricePlanId>"}
   ```

3. 必须返回：

   ```text
   testOnly=true
   amountCent=100
   environment=SANDBOX
   pricePlanId=<requested TEST plan>
   ```

4. 使用 quoteId 创建订单，核对 `signData.goodsPrice=100`、`env=1`、TEST 专属 productId。
5. 完成真机支付并确认 V2 快照履约。
6. 对 AGENT TEST 完整重复，明确证明没有再走 `99600` 比较。

## 5. 拒绝、漂移与并发用例

| 场景 | 预期 |
|---|---|
| U0 请求 TEST quote | 403 `PRICE_PLAN_NOT_ELIGIBLE` |
| 白名单未生效、过期或停用 | 403 `PRICE_PLAN_NOT_ELIGIBLE` |
| quote 后停用白名单 | 下单 403；无订单、无签名、不替换正式价 |
| 停用旧白名单后新建另一条 | 旧 quote 仍拒绝，因为固定旧 entryId |
| 仅修改 reason 且资格仍有效 | 当前实现允许旧 quote 继续下单 |
| 其他用户消费 quote | 403 `PRICE_QUOTE_FORBIDDEN` |
| 不存在的 quote | 403 `PRICE_QUOTE_FORBIDDEN`，避免探测 |
| quote 超过 5 分钟 | 410 `PRICE_QUOTE_EXPIRED` |
| 重复或并发消费 | 仅一个 201，其余 409 `PRICE_QUOTE_ALREADY_CONSUMED` |
| 商品/绑定/权益/offerId/productId/mode 漂移 | 409 `PRICE_QUOTE_CONFIGURATION_CHANGED` |
| 任一价格差 1 分 | 409 `PRICE_PLAN_WECHAT_PRICE_MISMATCH` |
| 正式/沙箱交叉 | 409 `PRICE_PLAN_PAYMENT_ENV_MISMATCH` |
| 人工确认过期 | 409 `WECHAT_GOOD_VERIFICATION_EXPIRED` |
| V132/CANARY affected tenant | 422 `PRICE_PLAN_SETTLEMENT_CONFIGURATION_MISMATCH` |
| `giftPoints > 0` | 422 `PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE` |
| session 过期 | 401 `WECHAT_SESSION_EXPIRED` |
| 微信 code 刷新失败 | 401 `WECHAT_SESSION_REFRESH_FAILED` |
| V2 请求缺少 quoteId | 400 `PRICE_QUOTE_REQUIRED` |
| 用户取消支付 | 不发权益、不分润 |
| 重复/并发微信回调 | 权益、Token、分润、payment event 各一次 |
| 回调丢失后官方查单 | 恢复 PAID+SUCCESS，仍只履约一次 |
| 默认价切换后的旧 quote | 未漂移则按旧价；漂移则拒绝；绝不自动改价 |
| 关闭创建后处理已建 V2 订单 | 回调/查单/履约继续成功，V2 履约开关保持 true |
| V1 历史订单 | 继续由 V1 兼容分支处理 |

## 6. 已知风险必须显式验收

- 本地人工确认不是微信实时验证，缺当次微信后台证据即 `NO-GO`。
- 当前代码不会自动证明 good.offerId/mode 与运行时 offer/AppKey 属于同一套配置。
- 关闭创建开关后，带既有 quoteId 的请求不会创建 V2 订单，但当前可能落入 V1 并返回通用 400，而非稳定的 `PRICE_PLAN_FEATURE_DISABLED`；上线前需修复或书面豁免。
- 隐藏 TEST 页面成功后只提示，不自动轮询最终履约；必须另查 status/sync 和数据库证据。
- TEST 页面可被直接路由访问，真正授权边界是后端开关、登录、白名单三重校验。

## 7. 每笔成功订单留证

- release commit、镜像 digest、体验版版本号。
- 真机型号、微信版本、基础库版本、账号角色。
- quote 响应；quoteId 只保存哈希或脱敏值。
- `amountCent/mode` 及解析后的 `goodsPrice/productId/offerId/env/currencyType/outTradeNo`。
- 订单 V2 快照、支付记录、回调/查单事件。
- 权益、会员/代理身份、Token、分润和幂等键计数。
- requestId、审计记录、测试时间、执行人和复核人。
- `paySig/signature/wxLoginCode/sessionKey/AppKey` 不得进入截图、日志或报告。

测试负责人：__________  微信负责人：__________  后端复核：__________  日期：__________

