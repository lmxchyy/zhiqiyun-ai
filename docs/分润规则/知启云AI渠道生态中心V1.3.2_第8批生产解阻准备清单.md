# 知启云AI渠道生态中心 V1.3.2 第8批生产解阻准备清单

变更单：`CR-2026-OC-008`  
生产主机：`ai.zs-kjhn.cn`  
生产库：`zhiqiyun`  
只读角色：`xianzhi_readonly`  
本阶段结论：**变更窗口已执行 / 生产只读 Preflight PASS / 应用未更新**

## 0. 变更审批单（已填）

| 字段 | 内容 |
|---|---|
| 变更单号 | `CR-2026-OC-008` |
| 变更标题 | 运营中心 V1.3.2 生产解阻：079–096 迁移 + 规则发布 + 财务双人 |
| 申请人 | mosilyu |
| 审批人 | mosilyu（负责人自批；唯一运维拍板） |
| 审批结论 | **同意** |
| 审批时间 | 2026-07-27 05:15 Asia/Shanghai |
| 变更窗口 | 2026-07-27 05:30–07:30 Asia/Shanghai |
| 操作人 | mosilyu |
| 复核人 | mosilyu（窗口内逐项核对报告） |
| 回滚负责人 | mosilyu |
| 影响主机/库 | `ai.zs-kjhn.cn` / `zhiqiyun` |
| 批准范围 | 仅：迁移前备份、079–088、089–096、`seed-prod-publish-v132.sql`、`seed-prod-finance-dual.sql`、只读 Preflight |
| 明确不批准 | 不更新/重启应用；不开 Canary/白名单/调度；不真实支付或退款；不改 rollout 出 SHADOW |

签字记录：

```text
CR-2026-OC-008 APPROVED
approver=mosilyu
approved_at=2026-07-27T05:15:00+08:00
window=2026-07-27T05:30:00+08:00/2026-07-27T07:30:00+08:00
scope=prod-db-migrate-079-096+publish-seed+finance-dual+readonly-preflight
```

## 1. 本批边界

本批准备材料已完成；**生产写入仅允许在上方已批准窗口内**按第4节执行。窗口外仍不得生产迁移，不得更新/重启生产应用，不得创建真实白名单，不得开启 Canary / 比例放量 / 生产调度器，不得发起真实支付或退款。

## 2. 已完成证据索引

| 证据 | 路径 | 结果 |
|---|---|---|
| 本机结构演练 A–F（088+089–096） | `E:\xianzhi-rehearsal\reports\operation-center-089-096-rehearsal.json` | PASS |
| 生产 live dump + manifest | `E:\xianzhi-rehearsal\backups\prod-live-sanitized-20260726T210342Z.dump` / `CR-2026-OC-008-prod-live.manifest.yaml` | 已导出 |
| 生产数据量演练（restore → sanitize → 079–088 → 089–096） | `E:\xianzhi-rehearsal\reports\operation-center-089-096-prodcopy-rehearsal.json` | PASS |
| 生产数据量差异报告 | `docs/分润规则/reports/operation-center-089-096-prodcopy-difference-report.md` | 已生成 |
| 发布/财务种子脚本 | `docs/分润规则/scripts/seed-prod-*.sql` | 已落盘未执行 |
| Preflight env 模板 | `docs/分润规则/scripts/prod-change-window.env.example` | 已落盘 |
| 生产只读 Preflight | `E:\xianzhi-rehearsal\reports\step-g-real-prod-preflight.json` | FAIL / BLOCKED |

生产 dump 校验：

- 引用：`prod-live-sanitized-20260726T210342Z`
- SHA256：`be9016ee84bc9b1934227acf081eb8a5f93faf3c49e4d956360274c175aa7f4b`
- 脱敏规则版本：`sanitize-v1.0`（隔离库清空 `password_hash`、掩码 `mobile`；不回写生产）

## 3. 生产缺口对照（Preflight）

| Preflight 检查 | 当前 | 解除方式（变更窗口） |
|---|---|---|
| `migrations_089_096` | 全 false | 先 079–088，再 089–096 |
| `rollout_safe_defaults` | 表不存在 | 085–087 后保持 SHADOW / 无白名单 |
| `published_rule_set_exists` | 表不存在 / 0 | 执行 `seed-prod-publish-v132.sql` |
| `operation_center_plan_complete` | 表不存在 / 0 | 同上（`rbacRole=OPERATION`） |
| `full_only_refund_policy` | 表不存在 / 0 | 同上（`FULL_ONLY*`） |
| `financial_permission_separation` | submitter/approver=0 | 执行 `seed-prod-finance-dual.sql`，禁用 `user_000001` 充当双人 |
| OC 相关 metrics | 表不存在 | 089–096 后自动可读 |

当前生产财务现状：仅 `user_000001`（`admin@xianzhi.ai`）带 `FINANCE`，不可作为提交人/审批人分离账号。

## 4. 变更窗口操作顺序

审批通过后，严格按以下顺序执行；任一步失败即停止并保持调度关闭。

1. **再打一份生产迁移前备份**（独立编号，保留至少一份可恢复副本）。
2. **应用 079–088**（补齐 V1.3.2 商业/灰度结构；生产当前缺这些表）。
3. **应用 089–096**（运营中心退款/奖励结构）。
4. **执行发布种子** [`scripts/seed-prod-publish-v132.sql`](scripts/seed-prod-publish-v132.sql)。
5. **执行财务双人种子** [`scripts/seed-prod-finance-dual.sql`](scripts/seed-prod-finance-dual.sql)。
6. **注入只读 Preflight 环境**（参考 [`scripts/prod-change-window.env.example`](scripts/prod-change-window.env.example)）。
7. **复跑** `go run ./cmd/operation-center-preflight`，要求全部 PASS。
8. 输出最终结论：`READY_FOR_CHANGE_APPROVAL` 仅在“迁移+发布已完成且 Preflight PASS”后给出；本准备批在窗口前保持 BLOCKED。

建议命令骨架（已审批，仅在 2026-07-27 05:30–07:30 窗口内执行）：

```bash
# on ai.zs-kjhn.cn, after approved backup
for f in 079 080 081 082 083 084 085 086 087 088; do
  psql ... -v ON_ERROR_STOP=1 -f database/migrations/${f}-*.sql
done
for f in 089 090 091 092 093 094 095 096; do
  psql ... -v ON_ERROR_STOP=1 -f database/migrations/${f}-*.sql
done
psql ... -v ON_ERROR_STOP=1 -f docs/分润规则/scripts/seed-prod-publish-v132.sql
psql ... -v ON_ERROR_STOP=1 -f docs/分润规则/scripts/seed-prod-finance-dual.sql
```

```powershell
# local readonly preflight via SSH tunnel
# ssh -N -L 127.0.0.1:15433:172.18.0.2:5432 root@ai.zs-kjhn.cn
$env:XIANZHI_ENV='production'
$env:DATABASE_URL='postgresql://xianzhi_readonly:<SECRET>@127.0.0.1:15433/zhiqiyun?sslmode=disable'
$env:XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_PROVIDER_MAPPINGS='WECHAT_VIRTUAL=MANUAL'
$env:XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_SUBMITTER_ID='98b945ba-878d-4c40-ae28-20ea3da1a6a5'
$env:XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_APPROVER_ID='e7af2217-3e0b-4ce8-b29c-7f74a1a4091f'
go run ./cmd/operation-center-preflight
```

## 5. 明确禁止项

- 不把更新/重启 `zhiqiyun-ai-prod-xianzhi-ai-1` 当作解阻手段。
- 不将 rollout 从 `SHADOW` 改为 Canary/Full，不开 `real_switch_enabled` / `percentage_rollout_enabled`。
- 不写入 `tenant/user/order/plan` 白名单。
- 不开启退款重试、UNKNOWN 核验、奖励释放调度器，不开启人工退款自动审批。
- 不发起真实支付或退款。
- 已执行 089–096 后，不做财务/奖励/钱包数据物理回滚；故障按第7批手册第7节前向修复。

## 6. 回滚与紧急停用

沿用 [第7批生产变更审批手册](知启云AI渠道生态中心V1.3.2_第7批生产变更审批手册.md) 第7节：

1. 保持或恢复 `mode=SHADOW`、关闭真实切换与比例放量，清空白名单。
2. 关闭三类 Operation Center 调度器。
3. 停止新增审核/退款管理写入，保留只读与审计导出。
4. 应用回滚不影响已固化 V1.3.2 历史订单。
5. 结构性故障：从迁移前备份恢复到隔离环境验证修复，再以新的前向迁移处理生产。

## 7. 审批材料勾选

- [x] 生产 live 脱敏/隔离演练用 dump + SHA256
- [x] 079–088 + 089–096 隔离数据量演练 PASS 与差异报告
- [x] 发布/财务种子脚本与财务双人 ID
- [x] 生产只读 Preflight FAIL 明细与解除对照表
- [x] 变更审批签字 / 窗口时间（见第0节；同意；2026-07-27 05:30–07:30）
- [x] 生产窗口执行记录与迁移后 Preflight PASS JSON  
  - 远端日志：`/root/cr-2026-oc-008-20260726T211654Z/`  
  - 迁移前备份 SHA256：见 `E:\xianzhi-rehearsal\reports\pre-migrate.sha256`  
  - Preflight：`docs/分润规则/reports/operation-center-production-preflight-post-window.json`（passed=true）

## 8. 当前门禁结论

- **本地/隔离演练前置**：已满足
- **变更审批**：已通过（CR-2026-OC-008）
- **生产窗口执行**：已完成（079–096 + 发布种子 + 财务双人；应用未更新）
- **生产只读 Preflight**：PASS
- **仍未批准事项**：不开 Canary/白名单/调度；不真实支付退款；不因本批重启应用
