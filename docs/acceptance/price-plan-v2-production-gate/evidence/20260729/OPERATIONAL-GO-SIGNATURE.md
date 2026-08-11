# 运营门禁最终签字 — GO

**决定时间：** 2026-07-29T10:58:00+08:00  
**决定：** **`GO`**  
**确认原文：** 「P5 沿用，GO」  
**签字人：** 用户（本轮兼任运维 + DBA + 价格/微信现场确认 + 产品/QA 豁免确认）  
**本签字动作：** **仅文档**。未改 V2 开关、未改 `WECHAT_VIRTUAL_PAY_ENV`、未改价、未部署、未轮换密钥。

**本文件地位：** 自 10:58 起，本目录 **唯一总状态真相源 = `GO`**。  
此前 `STATUS-NO-GO-RECONCILE.md`（10:31 降级）与历史 `PO-GO-SIGNATURE.md`（10:03）均被本签字 **supersede**（历史证据保留，不作冲突绿灯来源）。

## P0–P6 关闭一览

| 项 | 状态 | 证据 |
|---|---|---|
| P0 凭据入仓 | **CLOSED-WITH-ACCEPTED-RESIDUAL**（不轮换密钥） | `P0-SECRETS-REDACTION.md` |
| P1 三 SHA | **DOCUMENTED**（禁止混用） | 见下 |
| P2 现场只读 | **PASS** | `P2-READONLY-RECONCILE.md` / `p2-reconcile/` |
| P3 代理价 | **PASS** | `P3-P4-HUMAN-CONFIRM.md`（「一致」） |
| P4 微信商品 | **PASS** | 同上 |
| P5 测试豁免 | **ACCEPTED — 沿用** | 本文件；沿用 §5 替代（无沙箱真机付 / 无真实 ¥996） |
| P6 最终签字 | **SIGNED GO** | 本文件 |

## 三 SHA（FULL，禁止混用）

```text
运行镜像源码 runtimeImageGitSha = a39485ef159dabf348a71059a0e922af4894ab5a
IMAGE_ID                        = sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32
imageRef                        = local/xianzhi-ai-platform:a39485ef1

历史 PO 文档提交 docsGoCommit   = 719f898c5ca160348ca5d597f9644901d0a60242
  （不是镜像构建 commit）

证据仓库 HEAD（本 GO 文档写入前） = cd9b88abcdf79227d9e65333049dd6f97e0fdb8a
```

## 现网运行时（P2 实读，本签字未改动）

```text
SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=true
PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=true
PRICE_PLAN_TEST_ENTRY_ENABLED=true
WECHAT_VIRTUAL_PAY_ENV=production
```

## 代理价（已确认）

```text
AGENT NORMAL / AGENT_JOIN_996 / ¥996 / 99600 分
AGENT TEST   / AGENT_TEST_1YUAN / ¥1 / 100 分
```

## P5 沿用内容（诚实声明）

1. 支付管线以生产 ¥1 TEST（MEMBER+AGENT）为 ACCEPTED  
2. NORMAL ¥996 真机付 = **WAIVED**（dry quote + 强制等式）  
3. 沙箱真机付未跑；不以发明 PASS 补齐  
4. §5 #5 幂等 = 服务端 re-sync×2，**非**微信 push 风暴  

## GO 条件 / 残余（必须继续遵守）

1. **P0 残余：** Git 历史可能仍有脱敏前明文；**密钥未轮换**（已接受）  
2. **禁止** `docker compose up -d --build` / 同 tag 覆盖重建  
3. **禁止** 盲目重跑 097–100；**禁止** 混用三 SHA  
4. **禁止** 对外分发历史未脱敏 `container-inspect` 副本  
5. Registry RepoDigest 仍为长期标准；本轮 §1=`PASS-WITH-LOCAL-IMMUTABLE`  
6. 第三阶段（退款/补偿/人工补发 V2）= **OUT OF SCOPE**  
7. 回滚人员须保持可联络；事故回退顺序见 `go-no-go-gate.md`

## 角色签字（本轮）

| 角色 | 签字 |
|---|---|
| 运维 / 发布 | 用户（P2 实跑 + 本 GO） |
| DBA | 用户（P2 只读对账 PASS） |
| 价格 / 微信现场 | 用户（P3/P4「一致」） |
| 产品 / QA（P5） | 用户（「P5 沿用」） |
| 安全 | P0 残余已接受（不轮换） |
| 业务负责人 | **SIGNED GO** — 「P5 沿用，GO」@ 2026-07-29T10:58:00+08:00 |

**最终决定：** `GO`  
**变更单号：** `OPS-GO-20260729-P0-P6-CLOSED`

## 交叉链接

- `OWNER-TODO-NO-GO.md` — 待办全关  
- `STATUS-NO-GO-RECONCILE.md` — 历史降级记录（已被本 GO supersede）  
- `PO-GO-SIGNATURE.md` — 历史 10:03 GO（曾被 10:31 NO-GO supersede；现由本运营 GO 接续）  
- `../../go-no-go-gate.md` · `../../HANDOFF-ROLE-EXECUTION-PACK.md` · `../../README.md`
