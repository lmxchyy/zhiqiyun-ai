# Off-site backup upload (Phase 3A)

本阶段把已经生成并通过本地完整性校验的 backup artifact 复制到对象存储。上传器不创建数据库备份、不执行 retention、不部署，也不删除文件。

## 使用边界

默认模式是 dry-run，且没有参数时也不会上传。真实上传必须显式使用 `--upload`。当前 COS provider 仅完成安全配置门禁；在没有经过验证的 COS CLI/adapter 和凭证时返回 `UPLOAD_NOT_CONFIGURED`，不会尝试复用同机 MinIO 或业务存储凭证。

```sh
./ops/backup-upload-object-storage.sh \
  --file backups/postgres/db_20260822_214315_b2084b737b.sql.gz

./ops/backup-upload-object-storage.sh \
  --file backups/postgres/db_20260822_214315_b2084b737b.sql.gz \
  --provider cos --upload
```

测试使用 fake provider，不访问真实 COS：

```sh
./ops/backup-upload-object-storage.sh --provider fake \
  --fake-root /tmp/fake-object-store --root /tmp/backups \
  --file /tmp/backups/postgres/db_example.sql.gz --upload --json
```

## 上传前校验

文件必须是 backup root 内的 regular file，不能是 symlink；对应 `.meta.json` 必须存在且可解析。gzip 文件会先做完整 gzip 校验，然后以压缩文件本身计算 `sha256` 和 `bytes`，并与 meta 一致后才允许上传。

## Object key

key 只使用安全的 basename 和文件 mtime 的 UTC 年月，路径固定为：

```text
zhiqiyun-ai/postgres/deploy/YYYY/MM/<basename>
```

daily 预留为 `zhiqiyun-ai/daily/YYYY/MM/<basename>`。basename 不会被当作任意路径解释，控制字符、绝对路径和 `..` 会被拒绝。

## 远端校验与幂等

上传 backup object 后执行远端 HEAD，必须同时满足远端 size 等于本地 size、远端自定义 sha256 metadata 等于本地 sha256。随后才上传原始 meta object 和 `.sha256` object；全部完成后才写本地 `<backup>.offsite.json`。

sidecar 不修改原始 backup meta，结构为：

```json
{
  "version": 1,
  "provider": "cos",
  "bucket": "private-backup-bucket",
  "object_key": "zhiqiyun-ai/postgres/deploy/2026/08/db_example.sql.gz",
  "uploaded_at": "2026-08-23T00:00:00Z",
  "local_bytes": 123,
  "local_sha256": "...",
  "remote_bytes": 123,
  "remote_etag": "provider-etag",
  "remote_sha256": "...",
  "verification": "OFFSITE_VERIFIED"
}
```

已有对象若 size 和 sha256 metadata 都一致，返回 `ALREADY_OFFSITE_VERIFIED`；若任一不一致，返回 `REMOTE_CONFLICT`，绝不覆盖。远端校验失败时不会写 `OFFSITE_VERIFIED`。

## 凭证

生产凭证只允许通过环境注入，例如 `BACKUP_OBJECT_PROVIDER`、`BACKUP_OBJECT_BUCKET`、`BACKUP_OBJECT_REGION`、`BACKUP_OBJECT_ENDPOINT`、`TENCENTCLOUD_SECRET_ID` 和 `TENCENTCLOUD_SECRET_KEY`。SecretId/SecretKey 不写入 Git、meta、sidecar、fixture 或日志。本阶段没有配置生产凭证。

建议使用独立 private COS bucket，按最小权限授予 Put/Head/Get；可在平台侧单独评估 SSE、versioning、访问日志、生命周期和 Object Lock。本阶段不创建 bucket、不修改 bucket policy 或 lifecycle。

同生产机上的 MinIO 不构成异地备份，因此不作为生产 COS 的 fallback。

## Retention 预留

未来只有同时满足 `DELETE_CANDIDATE` 和 `<backup>.offsite.json.verification == OFFSITE_VERIFIED` 才可进入 `APPLY_ELIGIBLE`。没有 sidecar 或验证状态不正确时保持 `LOCAL_ONLY_PROTECTED`。本阶段不修改 `backup-retention.sh`，也没有真实删除实现。
