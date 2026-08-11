# CR-2026-OC-008 变更审批记录

## 审批结论

**同意**

## 表单

| 字段 | 内容 |
|---|---|
| 变更单号 | CR-2026-OC-008 |
| 变更标题 | 运营中心 V1.3.2 生产解阻：079–096 迁移 + 规则发布 + 财务双人 |
| 申请人 | mosilyu |
| 审批人 | mosilyu（负责人自批） |
| 审批时间 | 2026-07-27 05:15 Asia/Shanghai |
| 变更窗口 | 2026-07-27 05:30–07:30 Asia/Shanghai |
| 操作人 | mosilyu |
| 复核人 | mosilyu |
| 回滚负责人 | mosilyu |
| 影响范围 | `ai.zs-kjhn.cn` / 数据库 `zhiqiyun` |

## 批准范围

1. 生产迁移前备份  
2. 顺序应用迁移 079–088  
3. 顺序应用迁移 089–096  
4. 执行 `docs/分润规则/scripts/seed-prod-publish-v132.sql`  
5. 执行 `docs/分润规则/scripts/seed-prod-finance-dual.sql`  
6. 使用只读账号复跑 `operation-center-preflight`

## 明确不批准

- 更新或重启 `zhiqiyun-ai-prod-xianzhi-ai-1`
- 开启 Canary、比例放量、任意白名单
- 开启 Operation Center 调度器或人工退款自动审批
- 发起真实支付或退款
- 将 rollout 移出 `SHADOW`

## 签字

```text
CR-2026-OC-008 APPROVED
approver=mosilyu
approved_at=2026-07-27T05:15:00+08:00
window=2026-07-27T05:30:00+08:00/2026-07-27T07:30:00+08:00
scope=prod-db-migrate-079-096+publish-seed+finance-dual+readonly-preflight
decision=AGREE
```

## 关联材料

- `docs/分润规则/知启云AI渠道生态中心V1.3.2_第8批生产解阻准备清单.md`
- `docs/分润规则/reports/operation-center-089-096-prodcopy-difference-report.md`
- `E:\xianzhi-rehearsal\backups\CR-2026-OC-008-prod-live.manifest.yaml`
