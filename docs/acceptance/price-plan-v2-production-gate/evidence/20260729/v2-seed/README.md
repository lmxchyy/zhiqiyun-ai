# V2 business-row seed + force-equality（2026-07-29）

| 项 | 值 |
|---|---|
| 授权 | 用户继续 production gate；三开关保持 false |
| 执行时间 | 2026-07-29 05:25–05:26 +08 |
| 路径 | 无冻结 seed SQL → **SQL 仅引导首个 plan_version**（admin API 鸡生蛋缺口）+ **admin API** 建 pricePlan/good/binding |
| 环境 | `WECHAT_VIRTUAL_PAY_ENV=production`；offerId=`1450579876`；mode=`short_series_goods` |
| V2 三开关 | 全程 **false**（见 `flags-before.txt` / `flags-after.txt`） |

## 为何需要 SQL bootstrap

`createBusinessPlanVersion` → `managedBusinessTypeForUpdate` 要求已存在恰好一个 MEMBER/AGENT version。  
零 version 时 admin `GET/POST .../versions` 对 legacy plan 返回 `BUSINESS_PLAN_NOT_FOUND`。  
因此首个 ACTIVE entitlement 用 `bootstrap-plan-versions.sql`（事务 + `WHERE NOT EXISTS` 幂等）。

## 已创建对象（PRODUCTION）

| 业务 | kind | pricePlan code | cents | productId | 状态 |
|---|---|---|---:|---|---|
| MEMBER | NORMAL | `pp_member_normal_prod_996` | 99600 | `MEMBER_YEAR_996` | ACTIVE + default |
| AGENT | NORMAL | `pp_agent_normal_prod_996` | 99600 | `AGENT_JOIN_996` | ACTIVE + default |
| MEMBER | TEST | `pp_member_test_prod_entry` | 100 | `MEMBER_TEST_1YUAN` | ACTIVE（非 default） |
| AGENT | TEST | `pp_agent_test_prod_entry` | 100 | `AGENT_TEST_1YUAN` | ACTIVE（非 default） |

稳定 ID 见 `created-inventory.json`。所有 pricePlan `giftPoints=0`。

## 强制等式结论

| 层 | 结果 |
|---|---|
| 静态对象等式 `sale = binding.snapshot = good.platform` + channel/env/offer/mode | **PASS**（`force_equality_blockers=0`，矩阵缺行=0） |
| 与微信双签价对齐（99600 / 100） | **PASS**（引用既有双签证据） |
| 含 `quote.transactionPrice` 的端到端强制等式 | **仍 BLOCKED** — 三开关 false，禁止开开关发 quote；**不得**宣称 quote 层 PASS |

证据：`force-equality-verify.out`、`force-equality-verify.sql`。

## 沙箱真机

**STOP / 未开测。** 当前运行时 `WECHAT_VIRTUAL_PAY_ENV=production`；V2 quote 需要履约/创建/TEST 开关按 Gate 顺序开启。本轮**未开启任何开关**，不发明沙箱 QA PASS。

## 未做

- 未改三开关
- 未建 SANDBOX environment 行（运行时非 sandbox）
- 未跑真机支付
- 未宣称总 GO

## 脚本

- `bootstrap-plan-versions.sql` — 首个 ACTIVE version
- `seed-v2-objects.sh` — Redis 短会话 + admin API（幂等）
- `force-equality-verify.sql` — 静态强制等式核对
