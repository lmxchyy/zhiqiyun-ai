# 给老板/负责人的上线待办清单

**总状态：** `GO`（2026-07-29T10:58:00+08:00）  
**真相源：** [`OPERATIONAL-GO-SIGNATURE.md`](./OPERATIONAL-GO-SIGNATURE.md)

| 优先级 | 待办 | 执行人 | 负责人需要确认 | 支付影响 | 当前状态 |
|---|---|---|---|---|---|
| P0 | 处理敏感凭据入仓事件 | 安全负责人、运维 | 工作区脱敏；**不轮换密钥**（已接受残余） | 本轮不轮换，无支付中断 | **CLOSED-WITH-ACCEPTED-RESIDUAL** |
| P1 | 统一发布版本身份 | 发布负责人 | 三 SHA 分写、不得混用 | 间接/高 | **DOCUMENTED** |
| P2 | 生产状态只读对账 | DBA、运维 | 镜像/开关/097–100 | 只读无影响 | **PASS** |
| P3 | 代理正式价最终确认 | 价格/微信 | ¥996 正式；¥1 仅 TEST | 直接/高 | **PASS**（「一致」） |
| P4 | 微信商品实时核对 | 微信支付负责人 | productId/价/发布状态 | 直接/高 | **PASS**（「一致」） |
| P5 | 确认测试豁免 | QA、产品 | 沿用 §5 替代（无沙箱真机/无真实 ¥996） | 直接 | **ACCEPTED — 沿用** |
| P6 | 最终 GO 签字 | 全角色 | P0–P5 已关 | 维持真实支付能力 | **SIGNED GO** |

## 三 SHA（禁止混用）

```text
运行镜像源码 = a39485ef159dabf348a71059a0e922af4894ab5a
PO 文档提交   = 719f898c5ca160348ca5d597f9644901d0a60242
证据 HEAD     = cd9b88abcdf79227d9e65333049dd6f97e0fdb8a
```

## 代理价

```text
AGENT NORMAL / AGENT_JOIN_996 / ¥996 / 99600 分
AGENT TEST   / AGENT_TEST_1YUAN / ¥1 / 100 分
```

## GO 后仍须遵守

- 禁止 `docker compose up -d --build` / 同 tag 覆盖  
- 禁止盲目重跑 097–100  
- 禁止外发历史未脱敏 inspect  
- 密钥未轮换残余已接受（见 `P0-SECRETS-REDACTION.md`）
