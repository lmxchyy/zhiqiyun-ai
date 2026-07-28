# 生产系统只读快照（2026-07-28 23:23 +08）

> 来源：`zhiqiyun-ai-prod-postgres-1` + `zhiqiyun-ai-prod-xianzhi-ai-1` printenv  
> **只读查询；未改数据；总状态仍 NO-GO**

## 运行配置（脱敏）

| 键 | 值 |
|---|---|
| WECHAT_VIRTUAL_PAY_ENABLED | `true` |
| WECHAT_VIRTUAL_PAY_ENV | `production` |
| WECHAT_VIRTUAL_PAY_MODE | `short_series_goods` |
| WECHAT_VIRTUAL_PAY_OFFER_ID | `1450579876` |
| PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED | **未设置**（compose 默认 `false`） |
| PRICE_PLAN_TEST_ENTRY_ENABLED | **未设置**（默认 `false`） |
| SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED | **未设置**（默认 `false`） |
| 容器 Created | `2026-07-28T00:05:09Z` |
| 镜像 Image ID | `sha256:71f110f7123b34387881f84b8cc66edec964e921cb146c2053c93dc1eb26af66` |
| RepoDigests | **空**（本地构建镜像，无 registry digest） |

## V2 迁移状态

| 对象 | 生产存在？ |
|---|---|
| `xz_price_plans` | **否** |
| `xz_wechat_virtual_goods` | **否** |
| `xz_price_plan_payment_bindings` | **否** |
| `xz_price_plan_user_whitelist` | **否** |

→ **097–100 未在生产落地。** 无 V2 `pricePlanId` / `wechatGoodId` / binding 可填。

## 现行（V1）会员/代理 × 微信映射

| 业务 | plan_id | xz_plans.price_cents | payment_product_code | grant_points | wechat_product_id | env | mode | mapping.enabled |
|---|---|---:|---|---:|---|---|---|---|
| MEMBER | `plan_ai_creator_996` | **99600** | `MEMBER_PRO_YEAR_996` | 40000 | `MEMBER_YEAR_996` | 0 PRODUCTION | short_series_goods | true |
| MEMBER | `plan_ai_creator_996` | 99600 | `MEMBER_PRO_YEAR_996` | 40000 | `MEMBER_YEAR_996` | 1 SANDBOX | short_series_goods | true |
| AGENT | `plan_agent_join_996` | **100** | `AGENT_STANDARD_996` | 100 | `AGENT_JOIN_996` | 0 PRODUCTION | short_series_goods | true |
| AGENT | `plan_agent_join_996` | 100 | `AGENT_STANDARD_996` | 100 | `AGENT_JOIN_996` | 1 SANDBOX | short_series_goods | true |

## 系统已暴露的阻断信号（支持 NO-GO）

1. **V2 表缺失**：不能做 V2 quote / binding 验收。  
2. **业务码 ≠ 微信 productId**：会员 `MEMBER_PRO_YEAR_996` vs 微信 `MEMBER_YEAR_996`；代理 `AGENT_STANDARD_996` vs 微信 `AGENT_JOIN_996`。  
3. **代理价格（已恢复）**：正式售价 **99600 分（¥996）**；¥1 仅为临时测试。生产已于 2026-07-29 将 `plan_agent_join_996.price_cents` 从 100 恢复为 99600。  
4. **grant_points > 0**：会员 40000、代理 100；V2 门禁要求候选方案 `giftPoints=0`。  
5. **微信 ¥1 TEST productId（2026-07-29 已发布）**：`MEMBER_TEST_1YUAN` / `AGENT_TEST_1YUAN` @ ¥1；正式 `AGENT_JOIN_996`/`MEMBER_YEAR_996` 保持 ¥996。系统侧 V2 映射/绑定仍缺失。  
6. **无 RepoDigest**：不能按 digest 不可变发布。

## 仍须人工（系统内无）

- 微信后台实时标价 / 发布状态截图  
- offerId `1450579876` 与 AppKey/AppID 同套证明  
- DBA 正式预检脚本全量 A–R 与签字  
- 隔离迁移演练与恢复演练  
- 沙箱真机 V2 quote 用例
