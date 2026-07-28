# 会员/代理价格方案 V2 生产前门禁资料包

状态：`PREPARATION ONLY / NO-GO`

本目录只用于会员和代理商 V2 价格方案的生产前准备，不表示生产数据库已检查、迁移已执行、微信商品已核验、沙箱真机已通过或生产流量已开启。

## 角色交接入口（优先发这个）

**`HANDOFF-ROLE-EXECUTION-PACK.md`** — 按发布 / DBA / 微信 / QA 分角色的可勾选执行包与回填表。  
未收回填前禁止生产启用；第三阶段保持 OUT OF SCOPE。

## 本轮边界

- 不连接生产数据库（DBA 领到 §2 预检任务后除外，且仅只读）。
- 不执行生产迁移；隔离演练仅在批准后按手册执行。
- 不擅自创建 release commit/tag、不擅自推送/部署镜像（发布领到 §1 审批单后除外）。
- 不开启任何 V2 功能开关；§6 仅在 §1–§5 全 PASS 后另开变更单。
- 不进入第三阶段，不修改退款、补偿、人工补发或其他套餐履约。

## 资料索引

0. **`HANDOFF-ROLE-EXECUTION-PACK.md`**：角色交接、命令入口、回填表、材料缺口（发各方执行用）。
1. `release-freeze-runbook.md`：release commit、镜像 digest、097–100 SHA256 冻结步骤。
2. `dba-readonly-preflight.sql`：可直接交 DBA 使用的 `psql` 只读预检。
3. `dba-preflight-decision-table.md`：每项 SQL 输出的 GO/NO-GO 判定。
4. `isolated-migration-rehearsal.md`：生产备份恢复到隔离 PostgreSQL 16 后的演练命令。
5. `wechat-goods-manual-checklist.md`：会员/代理、正式/沙箱、正常价/测试价微信商品人工核对表。
6. `sandbox-v2-quote-real-device-acceptance.md`：沙箱真机 V2 quote 支付验收步骤与证据要求。
7. `go-no-go-gate.md`：上线前最终门禁与回滚前提。

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

准备期三个开关必须保持：

```text
PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=false
PRICE_PLAN_TEST_ENTRY_ENABLED=false
SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=false
```

任何本目录中的 PASS 都只是对应单项证据；只有 `go-no-go-gate.md` 的所有硬门禁完成并由责任人签字后，才可以提出下一张生产变更单。

