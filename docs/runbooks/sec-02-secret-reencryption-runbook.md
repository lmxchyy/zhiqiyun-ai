# SEC-02 生产密钥版本化与存量数据重加密操作 Runbook

> **适用范围**：本 Runbook 规定了在 `STORAGE_MASTER_KEY` 与 `CONNECTOR_SECRET_ENCRYPTION_KEY` 轮换过程中，使用 `cmd/secret-reencrypt` 工具对存量加密数据执行安全迁移的标准作业程序 (SOP)。
> **对应 Release Blocker**：`#129 SEC-02` 及 `#118 SEC-01`
> **操作角色**：安全负责人 / IAM 管理员 / DBA

---

## 1. 生产迁移全生命周期阶段 (Phase A ~ H)

```mermaid
graph TD
    A[Phase A: 部署支持 Keyring/Dual-Read 版本<br/>active 仍为旧 Key] --> B[Phase B: 注入新 Key k2<br/>设置 active=k2, 保留旧 Key 在 Keyring]
    B --> C[Phase C: 验证新写入切换为 k2<br/>产出 enc:v2:k2]
    C --> D[Phase D: 运维执行 Dry-Run 试算<br/>--dry-run=true 确认无解密失败]
    D --> E[Phase E: 维护窗口执行正式重加密<br/>--dry-run=false CAS 乐观锁写入]
    E --> F[Phase F: 执行清零审计 SQL<br/>验证 enc:v1 与旧 v2 引用数归零]
    F --> G[Phase G: 全链路回归与历史数据可读性验证]
    G --> H[Phase H: 彻底移除旧 Key<br/>完成安全凭据销毁与最终留痕]
```

- **Phase A（代码部署）**：部署合并了 PR-118A 与 PR-118B 的生产镜像，环境变量仅配置原有旧 Key（系统默认赋予 `k1` 并激活为 `legacyV1Key`）。验证存量业务读写 100% 正常。
- **Phase B（新 Key 注入）**：生成符合安全要求的新 32 字节 MasterKey（例如 ID 为 `k2`）。通过独立环境变量将 `k2` 设为 active key，同时将原有旧 Key 作为读取保留 key 注入，重启服务。
- **Phase C（新写验证）**：在后台保存一条存储配置测试记录或触发一次 OAuth 回调，查验数据库新密文包含 `enc:v2:k2:` 前缀且能正确解密。
- **Phase D（预演试算 Dry-Run）**：运行重加密工具（默认 `--dry-run=true`），确认全量扫描无解密失败（`failed=0`）。
- **Phase E（维护窗口正式重加密）**：在夜间低峰期或维护窗口执行正式重加密（`--dry-run=false`），通过 CAS 条件更新完成存量密文覆盖。
- **Phase F（清零审计 Verification）**：运行零引用核对 SQL，验证两张表中所有 `enc:v1:` 与非 active key 的密文数量清零。
- **Phase G（端到端验证）**：验证所有历史存储文件下载与企业 Connector 通信均正常。
- **Phase H（旧 Key 销毁）**：从生产环境变量或 Keyring 配置中彻底剔除旧 Key，服务平稳运行，完成废弃闭环。

---

## 2. 详细执行 SOP

### 2.1 准备与配置检查

在执行主机上确认环境变量就绪（严禁在日志中回显真实密钥）：

```bash
# 检查数据库连接与 Keyring 配置
export DATABASE_URL="postgresql://xianzhi_prod:***@postgres:5432/xianzhi"

# 方式一：使用离散环境变量配置（推荐生产使用）
export STORAGE_MASTER_KEY_ACTIVE="k2"
export STORAGE_MASTER_KEY_LEGACY="<old-storage-master-key>"
export STORAGE_MASTER_KEY="<new-storage-master-key-k2>"

export CONNECTOR_SECRET_ACTIVE_KEY_ID="k2"
export CONNECTOR_SECRET_LEGACY_KEY="<old-connector-secret-key>"
export CONNECTOR_SECRET_ENCRYPTION_KEY="<new-connector-secret-key-k2>"

# 方式二：使用 JSON Keyring 环境变量
# export STORAGE_MASTER_KEY_RING='{"activeKeyId":"k2","keys":{"k1":"<old>","k2":"<new>"},"legacyV1Key":"<old>"}'
# export CONNECTOR_SECRET_KEYRING='{"activeKeyId":"k2","keys":{"k1":"<old>","k2":"<new>"},"legacyV1Key":"<old>"}'
```

### 2.2 Dry-Run 预演试算

执行工具预演（默认 dry-run 为 true）：

```bash
# 针对全量表执行 Dry-Run
./secret-reencrypt -table=all -batch-size=50 -dry-run=true

# 输出预期格式示例：
# [reencrypt-start] table=xz_storage_configs active_key_id=k2 dry_run=true batch_size=50
# [reencrypt] table=xz_storage_configs id=cfg_001 status=MIGRATED mode=DRY_RUN
# [reencrypt-finish] table=xz_storage_configs scanned=2 migrated=2 skipped=0 conflicted=0 failed=0
# [reencrypt-start] table=enterprise_connectors active_key_id=k2 dry_run=true batch_size=50
# [reencrypt] table=enterprise_connectors id=conn_001 status=MIGRATED mode=DRY_RUN
# [reencrypt-finish] table=enterprise_connectors scanned=1 migrated=1 skipped=0 conflicted=0 failed=0
# [reencrypt-summary] total: scanned=3 migrated=3 skipped=0 conflicted=0 failed=0 dry_run=true
```

**门禁判定**：
- 若 `failed > 0`：**严禁推进到正式迁移！** 检查旧 Key 或 legacyV1Key 是否匹配。
- 只有 `failed == 0` 时，方可进入正式执行步骤。

### 2.3 正式执行迁移 (Live Re-encryption)

进入维护窗口执行正式更新：

```bash
# 必须显式指定 -dry-run=false
./secret-reencrypt -table=all -batch-size=50 -dry-run=false

# 检查进程退出码：
echo $? # 必须为 0
```

---

## 3. Rollback / Abort 触发条件与应急方案

### 3.1 立即中止 (Abort) 条件
1. 执行过程中出现任何一条记录解密失败（`failed > 0`）；
2. 数据库连接异常中断或持续超时；
3. `conflicted` 计数异常激增（说明存在高并发写入干扰）。

### 3.2 回滚 (Rollback) 与数据安全保证
- **CAS 机制天然防坏写**：工具采用 `WHERE id=$id AND col=$old_encrypted` 条件保护，解密失败或并发冲突时绝不覆写字段，**原始密文始终完整保存在数据库中**；
- **回滚操作**：
  1. 无需恢复数据库快照（未受损记录无需回滚）；
  2. 若新 Key 出现意外，保持原有旧 Key 在环境变量中，将 active key 切换回旧 Key 即可无损继续读取与写入。

---

## 4. 零引用核验 SQL (Zero-Reference Verification SQL)

迁移完成后，由 DBA 或安全人员连接数据库，执行以下只读核查 SQL。**所有检查项预期计数必须全部为 0**：

### 4.1 对象存储配置表 (`xz_storage_configs`)

```sql
-- 1. 检查是否存在未迁移的 v1 密文 (预期: 0)
SELECT count(*) AS v1_remaining_count
FROM xz_storage_configs
WHERE (access_key_encrypted LIKE 'enc:v1:%' OR secret_key_encrypted LIKE 'enc:v1:%' OR session_token_encrypted LIKE 'enc:v1:%');

-- 2. 检查是否存在非当前 active key (假设 active=k2) 的密文 (预期: 0)
SELECT count(*) AS non_active_key_count
FROM xz_storage_configs
WHERE (
  (access_key_encrypted <> '' AND access_key_encrypted NOT LIKE 'enc:v2:k2:%') OR
  (secret_key_encrypted <> '' AND secret_key_encrypted NOT LIKE 'enc:v2:k2:%') OR
  (session_token_encrypted <> '' AND session_token_encrypted NOT LIKE 'enc:v2:k2:%')
);

-- 3. 检查是否存在未知或非法 envelope (预期: 0)
SELECT count(*) AS malformed_count
FROM xz_storage_configs
WHERE (
  (access_key_encrypted <> '' AND access_key_encrypted NOT LIKE 'enc:v2:%') OR
  (secret_key_encrypted <> '' AND secret_key_encrypted NOT LIKE 'enc:v2:%') OR
  (session_token_encrypted <> '' AND session_token_encrypted NOT LIKE 'enc:v2:%')
);
```

### 4.2 企业连接器表 (`enterprise_connectors`)

```sql
-- 1. 检查是否存在未迁移的 v1 密文 (预期: 0)
SELECT count(*) AS v1_remaining_count
FROM enterprise_connectors
WHERE (app_secret_encrypted LIKE 'enc:v1:%' OR verification_token_encrypted LIKE 'enc:v1:%' OR encrypt_key_encrypted LIKE 'enc:v1:%');

-- 2. 检查是否存在非当前 active key (假设 active=k2) 的密文 (预期: 0)
SELECT count(*) AS non_active_key_count
FROM enterprise_connectors
WHERE (
  (app_secret_encrypted <> '' AND app_secret_encrypted NOT LIKE 'enc:v2:k2:%') OR
  (verification_token_encrypted <> '' AND verification_token_encrypted NOT LIKE 'enc:v2:k2:%') OR
  (encrypt_key_encrypted <> '' AND encrypt_key_encrypted NOT LIKE 'enc:v2:k2:%')
);

-- 3. 检查是否存在未知或非法 envelope (预期: 0)
SELECT count(*) AS malformed_count
FROM enterprise_connectors
WHERE (
  (app_secret_encrypted <> '' AND app_secret_encrypted NOT LIKE 'enc:v2:%') OR
  (verification_token_encrypted <> '' AND verification_token_encrypted NOT LIKE 'enc:v2:%') OR
  (encrypt_key_encrypted <> '' AND encrypt_key_encrypted NOT LIKE 'enc:v2:%')
);
```

---

## 5. Release Control 验收矩阵

| 门禁项 | 验证方式 | 合格判定标准 | 证据归档 |
|---|---|---|---|
| **Dry-Run 试算** | CLI `-dry-run=true` 执行输出 | `failed=0`, `scanned == migrated + skipped` | 终端脱敏日志 |
| **Live 重加密** | CLI `-dry-run=false` 执行输出 | `failed=0`, exit code = 0 | 终端脱敏日志 |
| **v1 清零审计** | 执行核验 SQL 1 | `v1_remaining_count == 0` | SQL 查询结果截图/日志 |
| **非 Active Key 清零** | 执行核验 SQL 2 | `non_active_key_count == 0` | SQL 查询结果截图/日志 |
| **未知 Envelope 清零** | 执行核验 SQL 3 | `malformed_count == 0` | SQL 查询结果截图/日志 |
| **历史数据可读性** | 业务只读请求测试 | 存储签名 URL 访问 HTTP 200，飞书应用回调解密正常 | 接口响应日志 |
| **旧 Key 销毁留痕** | 移除旧环境变量并重启 | 无旧 Key 情况下应用平稳启动且无解密告警 | 变更工单与发布留痕 |
