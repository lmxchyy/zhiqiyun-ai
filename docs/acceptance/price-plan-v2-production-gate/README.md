# 会员/代理价格方案 V2 生产前门禁资料包

状态：`GO`（2026-07-29T10:58:00+08:00；「P5 沿用，GO」）

**真相源：** [`evidence/20260729/OPERATIONAL-GO-SIGNATURE.md`](./evidence/20260729/OPERATIONAL-GO-SIGNATURE.md)  
**待办：** [`evidence/20260729/OWNER-TODO-NO-GO.md`](./evidence/20260729/OWNER-TODO-NO-GO.md)（P0–P6 已关）

路径摘要：10:03 PO GO → 10:31 NO-GO（安全+现场）→ 10:58 运营 GO（P0–P6 关闭）。本 GO 签字未改开关/未部署。

本目录同时保留准备期手册与 2026-07-29 证据树。历史 PO `GO` 签字见 `PO-GO-SIGNATURE.md`（已 SUPERSEDED）。本更正轮次仅本地只读核对。

## 角色交接入口（优先发这个）

**`HANDOFF-ROLE-EXECUTION-PACK.md`** — 按发布 / DBA / 微信 / QA 分角色的可勾选执行包与回填表。  
当前总状态 **NO-GO**；第三阶段保持 OUT OF SCOPE。

## 本轮边界（10:31 更正后）

- 本更正轮次不连接生产、不部署、不操作微信后台。
- P0 关闭前禁止再次部署、重新开关、对外分发未脱敏证据包。
- 禁止盲目重跑 097–100；现场只读对账由 DBA/运维执行（P2）。
- 三 SHA 不得混用（镜像 / PO 文档 / 证据 HEAD）。
- 不进入第三阶段。

## 资料索引

0. **`evidence/20260729/STATUS-NO-GO-RECONCILE.md`**：当前总状态真相源（优先）。  
0b. **`evidence/20260729/OWNER-TODO-NO-GO.md`**：老板/负责人 P0–P6 待办。  
0c. **`HANDOFF-ROLE-EXECUTION-PACK.md`**：角色交接、命令入口、回填表、材料缺口。  
1. `release-freeze-runbook.md`：release commit、镜像 digest、097–100 SHA256 冻结步骤。  
2. `dba-readonly-preflight.sql`：可直接交 DBA 使用的 `psql` 只读预检。  
3. `dba-preflight-decision-table.md`：每项 SQL 输出的 GO/NO-GO 判定。  
4. `isolated-migration-rehearsal.md`：生产备份恢复到隔离 PostgreSQL 16 后的演练命令。  
5. `wechat-goods-manual-checklist.md`：会员/代理、正式/沙箱、正常价/测试价微信商品人工核对表。  
6. `sandbox-v2-quote-real-device-acceptance.md`：沙箱真机 V2 quote 支付验收步骤与证据要求。  
7. `go-no-go-gate.md`：上线前最终门禁与回滚前提（当前 `NO-GO`）。

## 固定执行顺序

```text
冻结候选 release
→ 生产只读预检
→ 生产备份恢复到隔离库
→ 097–100 隔离演练及恢复演练
→ 真实角色权限核对
→ 微信商品人工核对
→ 沙箱真机 V2 quote 验收
→ GO/NO-GO 审批
→ 另行批准后才允许生产迁移或开关变更
```

**注意：** 上述顺序是准备期手册；在 P0 安全处置与 P2 现场只读对账完成前，**不得**把该顺序当作「再上一次」的执行清单。

准备期三个开关必须保持（Gate A 默认关闭标准；与现网证据记录值区分）：

```text
PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=false
PRICE_PLAN_TEST_ENTRY_ENABLED=false
SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=false
```

任何本目录中的 PASS 都只是对应单项证据；只有 `go-no-go-gate.md` 硬门禁完成、P0–P5 关闭并由责任人签字后，才可以提出下一张生产变更单。当前总状态为 **`NO-GO`**。
