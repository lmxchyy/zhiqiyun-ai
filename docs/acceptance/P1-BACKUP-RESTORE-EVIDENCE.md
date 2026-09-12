# P1 Backup Restore Evidence

- **采集时间**：2026-09-09
- **结论**：`BLOCKED / NOT VERIFIED`

## 已验证

备份相关本地 harness：

```text
67 PASS
2 SKIPPED
```

覆盖批次上传、校验和、幂等、冲突保护、retention 约束、Python 3.6 兼容和生产契约静态检查。

## 未验证

- 当前 release 的生产 PostgreSQL backup 文件
- 真实 offsite OBS/R2/COS 上传
- `OFFSITE_VERIFIED` 远端 metadata
- 独立 PostgreSQL clean restore
- restore 后关键表、账本、File Object、Outbox 数据校验
- RPO/RTO 实测

## BLOCKED_REASON

当前环境没有可安全使用的生产备份文件、远端 offsite 授权和独立恢复数据库。本地 fake provider 测试不能替代真实恢复演练。

## 状态

`BLOCKED_EXTERNAL_BACKUP_AND_RESTORE_ENVIRONMENT`
