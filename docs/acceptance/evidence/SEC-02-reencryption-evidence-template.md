# SEC-02 生产密钥重加密运行时证据归档模板 (Evidence Template)

- **关联工单/Issue**：`#129 SEC-02` (技术前置) / `#118 SEC-01` (生产凭据轮换)
- **执行时间**：`YYYY-MM-DD HH:MM:SS CST`
- **执行环境**：`Production`
- **操作人 (Operator)**：`DBA / 安全负责人`
- **复核人 (Reviewer)**：`架构师 / IAM 管理员`
- **工单编号**：`SEC-OPS-YYYYMMDD-001`

---

## 一、配置与前置校验

- [ ] 新 MasterKey 生成并已通过环境变量安全注入：
  - `STORAGE_MASTER_KEY_ACTIVE`: `k2` (示例)
  - `CONNECTOR_SECRET_ACTIVE_KEY_ID`: `k2` (示例)
- [ ] 旧 MasterKey 已作为 `legacyV1Key` 保留注入
- [ ] 核心服务健康检查状态：`GET /health -> HTTP 200`
- [ ] 备份确认：生产数据库已完成执行前临时快照备份

---

## 二、Dry-Run 预演日志脱敏记录

```text
# 粘贴 ./secret-reencrypt -table=all -dry-run=true 执行日志（严禁包含任何密钥材质）
[reencrypt-start] table=xz_storage_configs active_key_id=k2 dry_run=true batch_size=50
...
[reencrypt-finish] table=xz_storage_configs scanned=X migrated=X skipped=X conflicted=0 failed=0
[reencrypt-start] table=enterprise_connectors active_key_id=k2 dry_run=true batch_size=50
...
[reencrypt-finish] table=enterprise_connectors scanned=X migrated=X skipped=X conflicted=0 failed=0
[reencrypt-summary] total: scanned=X migrated=X skipped=X conflicted=0 failed=0 dry_run=true
```

- **Dry-Run 判定**：`PASS (failed=0)`

---

## 三、Live 重加密执行日志脱敏记录

```text
# 粘贴 ./secret-reencrypt -table=all -dry-run=false 执行日志
[reencrypt-start] table=xz_storage_configs active_key_id=k2 dry_run=false batch_size=50
...
[reencrypt-finish] table=xz_storage_configs scanned=X migrated=X skipped=X conflicted=0 failed=0
[reencrypt-start] table=enterprise_connectors active_key_id=k2 dry_run=false batch_size=50
...
[reencrypt-finish] table=enterprise_connectors scanned=X migrated=X skipped=X conflicted=0 failed=0
[reencrypt-summary] total: scanned=X migrated=X skipped=X conflicted=0 failed=0 dry_run=false
```

- **执行退出码**：`0`
- **迁移耗时**：`X 秒`

---

## 四、零引用审计 SQL 执行结果 (Zero-Reference Verification)

| 检查项 | SQL 查询目标 | 预期值 | 实际执行结果 | 状态 |
|---|---|---|---|---|
| 1. `xz_storage_configs` v1 剩余引用 | `count(*) WHERE ... LIKE 'enc:v1:%'` | 0 | 0 | PASS |
| 2. `xz_storage_configs` 非 active key 引用 | `count(*) WHERE ... NOT LIKE 'enc:v2:k2:%'` | 0 | 0 | PASS |
| 3. `xz_storage_configs` 异常格式 | `count(*) WHERE ... NOT LIKE 'enc:v2:%'` | 0 | 0 | PASS |
| 4. `enterprise_connectors` v1 剩余引用 | `count(*) WHERE ... LIKE 'enc:v1:%'` | 0 | 0 | PASS |
| 5. `enterprise_connectors` 非 active key 引用 | `count(*) WHERE ... NOT LIKE 'enc:v2:k2:%'` | 0 | 0 | PASS |
| 6. `enterprise_connectors` 异常格式 | `count(*) WHERE ... NOT LIKE 'enc:v2:%'` | 0 | 0 | PASS |

---

## 五、业务端到端功能验证

- [ ] 存量对象存储文件（图片、视频、导出文档）经由私有签名 URL 访问：`HTTP 200` 正常；
- [ ] 新建存储文件上传与入库：使用 `enc:v2:k2:` 格式且读写正常；
- [ ] 存量企业飞书连接器事件接收与鉴权：解密正常，无鉴权失败日志；
- [ ] 核心 API 监控指标：5xx 错误率为 0，无加解密异常抛错。

---

## 六、旧 Key 废除与终态确认

- [ ] 生产服务已移除旧 Key 环境变量 (`STORAGE_MASTER_KEY_LEGACY` / `CONNECTOR_SECRET_LEGACY_KEY`)
- [ ] 生产容器重启成功，全链路平稳运行
- [ ] 安全负责人签章：`已完成轮换与旧 Key 物理销毁 (签名: _________ 日期: _________)`
