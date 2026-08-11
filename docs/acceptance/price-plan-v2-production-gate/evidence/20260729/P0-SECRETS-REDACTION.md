# P0 处置记录 — 敏感凭据入仓（密钥不轮换）

**执行时间：** 2026-07-29T10:40:00+08:00（本地）  
**决策：** 负责人确认 **密钥不轮换**  
**范围：** 仅本地证据树处置；未连生产、未部署、未改支付/微信配置、未输出任何凭据值。

## 已完成

| 动作 | 结果 |
|---|---|
| 工作区 `container-inspect.json` 脱敏 | Env 134 条非空值全部改为 `***REDACTED***`；`DATABASE_URL`/`REDIS_URL`/微信支付相关键已验证为占位符 |
| `image-inspect.json` 同步脱敏 | Env 值同样占位 |
| 写入 redaction meta | `container-inspect.redaction-meta.json`、`image-inspect.redaction-meta.json` |
| 导出脚本防再入仓 | `probe-and-export-local-immutable.sh` 在 `docker inspect` 后强制 python 脱敏 Env |
| gitignore 加固 | `evidence/20260729/.gitignore` 忽略 `*.raw.json` / `*.unredacted.json` |
| 明文复扫（工作区） | 当前工作区 inspect JSON **无** 未脱敏 Env 值 |

## 明确不做

- **不轮换** 微信 / 支付 / DB / Redis / 对象存储等密钥（负责人决策）
- 本步 **不** 做 Git 历史改写（filter-repo/BFG）
- 本步 **不** 部署、不改开关

## 残余风险（接受）

1. **Git 历史**中仍可能存在脱敏前的明文（首次入仓提交起）。HEAD/工作区已脱敏后，新克隆若只取最新提交则不再带明文；但旧 commit / 旧 clone / 已外发副本仍可能有。
2. **密钥未轮换** ⇒ 历史暴露窗口内的凭据仍有效。本 P0 关闭的是「工作区继续扩散明文」；不是「泄露归零」。

若以后要进一步收敛：可选做历史清理（需单独审批）或再评估轮换。

## P0 结论

```text
P0_STATUS = CLOSED-WITH-ACCEPTED-RESIDUAL
KEY_ROTATION = DECLINED
BLOCKS_REDEPLOY_FOR_SECRETS_IN_WORKTREE = CLEARED
OVERALL_GATE = 仍 NO-GO（P1 三 SHA / P2 现场对账 / P3–P5 等未关）
```

## 下一步（上线最短路径）

1. **P2** 生产只读对账（迁移 / 镜像 / 三开关）— 不部署  
2. **P3+P4** 代理 ¥996 + 微信商品真人核对  
3. **P1** 发布身份三 SHA 写清  
4. **P6** 最终 GO 签字  

证据互链：`STATUS-NO-GO-RECONCILE.md` · `OWNER-TODO-NO-GO.md`
