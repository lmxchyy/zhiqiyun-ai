# P3 + P4 真人确认 — 代理价与微信商品

**确认时间：** 2026-07-29T10:54:00+08:00  
**确认人：** 用户（本轮兼任运维 + DBA；对价格与微信后台现场结论口头确认）  
**确认原文：** 「一致」  
**前置：** P2 现场只读对账 **PASS**（`P2-READONLY-RECONCILE.md` / `p2-reconcile/`）

## P3 代理正式价

| 项 | 确认值 |
|---|---|
| AGENT NORMAL | `AGENT_JOIN_996` / **¥996 / 99600 分** |
| AGENT TEST | `AGENT_TEST_1YUAN` / **¥1 / 100 分**（独立商品，不得冒充正式价） |
| 结论 | **PASS — 一致** |

## P4 微信商品（结合 P2 库内 PUBLISHED 行 + 用户后台核对）

| productId | 库内 PRODUCTION 价 | 状态（P2） | 用户后台核对 |
|---|---:|---|---|
| `AGENT_JOIN_996` | 99600 | PUBLISHED | **一致** |
| `AGENT_TEST_1YUAN` | 100 | PUBLISHED | **一致** |
| `MEMBER_YEAR_996` | 99600 | PUBLISHED | **一致** |
| `MEMBER_TEST_1YUAN` | 100 | PUBLISHED | **一致** |

**P4 结论：PASS — 一致**

## 本步未做

- 未改价、未改开关、未部署、未操作支付密钥  
- 未发明微信截图；以用户口头「一致」+ P2 只读库证为据

## 门禁影响

```text
P3 = PASS
P4 = PASS
OVERALL = 仍 NO-GO，待 P5（测试豁免确认）+ P1（三 SHA 分写归档）+ P6 最终签字
```
