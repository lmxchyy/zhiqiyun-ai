# 门禁状态更正记录（历史）— 曾降为 NO-GO，现已被运营 GO supersede

**原生效时间：** 2026-07-29T10:31:00+08:00（NO-GO 降级）  
**当前有效决定：** **`GO`** @ 2026-07-29T10:58:00+08:00 — 见 [`OPERATIONAL-GO-SIGNATURE.md`](./OPERATIONAL-GO-SIGNATURE.md)  
**本文件保留为降级过程证据，不再作为总状态真相源。**

## 当前总状态（指针）

```text
OVERALL = GO
TRUTH   = OPERATIONAL-GO-SIGNATURE.md
AT      = 2026-07-29T10:58:00+08:00
VERBATIM = 「P5 沿用，GO」
```

## 降级时关键判断纠正（历史）

| 错误口径 | 正确口径 |
|---|---|
| 「尚待上线」/ 按原步骤继续部署 | **证据包记录已上线**，但当时现场未复核 + 安全硬阻断 |
| 沿用 10:03 PO 签字当运营绿灯 | 曾被 10:31 NO-GO supersede；10:58 经 P0–P6 关闭后重新签运营 GO |

## P0–P6（关闭后摘要）

- P0 CLOSED-WITH-ACCEPTED-RESIDUAL（不轮换）  
- P1 三 SHA DOCUMENTED  
- P2 PASS（10:52 实读）  
- P3/P4 PASS（10:54「一致」）  
- P5 ACCEPTED 沿用（10:58）  
- P6 SIGNED GO（10:58）

## 三个 commit 必须分开写（禁止混用）

| 身份 | FULL SHA | 含义 | 禁止 |
|---|---|---|---|
| 运行镜像源码 | `a39485ef159dabf348a71059a0e922af4894ab5a` | 现网镜像构建依据 | 用文档 commit 当镜像 commit |
| PO GO 文档提交 | `719f898c5ca160348ca5d597f9644901d0a60242` | 仅文档：曾声明 GO | 当作现有镜像构建 commit |
| 证据仓库 HEAD（GO 前） | `cd9b88abcdf79227d9e65333049dd6f97e0fdb8a` | GO 前证据树 | 与上两者混称为「同一 release」 |

## 交叉链接

- **当前真相源：** [`OPERATIONAL-GO-SIGNATURE.md`](./OPERATIONAL-GO-SIGNATURE.md)
- 待办：[`OWNER-TODO-NO-GO.md`](./OWNER-TODO-NO-GO.md)
- P0：[`P0-SECRETS-REDACTION.md`](./P0-SECRETS-REDACTION.md)
- P2：[`P2-READONLY-RECONCILE.md`](./P2-READONLY-RECONCILE.md)
- P3/P4：[`P3-P4-HUMAN-CONFIRM.md`](./P3-P4-HUMAN-CONFIRM.md)
