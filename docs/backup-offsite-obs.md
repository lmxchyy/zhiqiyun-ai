# Huawei OBS off-site provider (Phase 3B)

本阶段为 `ops/backup-upload-object-storage.sh` 增加 Huawei OBS provider。上传器仍只负责 replicate + verify；`backup.sh` 创建备份，`backup-retention.sh` 计算 retention，三者不互相调用。

本阶段没有配置生产凭证、创建或修改真实 OBS bucket，也没有上传生产数据库。

## 官方兼容性结论

根据华为云 OBS 官方 Python SDK 文档：

- 当前 SDK 文档标注 Python 3.6 及以上，安装包名为 `esdk-obs-python`。
- `putFile` 支持普通文件上传；单对象超过 5 GB 时应使用 multipart/resumable upload。
- `uploadFile` 支持分段、断点续传和 checkpoint。
- 自定义元数据使用 `x-obs-meta-*`，上传后可通过 HEAD 获取。
- `getObjectMetadata` 用于 HEAD；需要 `GetObject` 权限。
- multipart 完成后的 ETag 不是整个对象的 MD5，因此 ETag 只作辅助记录。
- SDK 支持 ENV 凭证和 ECS Agency；`OBS_DEFAULT` 可按 ENV 后 ECS 的顺序查找，ECS 方式依赖实例已配置 Agency。

官方参考：

- [Python SDK compatibility](https://support.huaweicloud.com/sdk-python-devg-obs/obs_22_0001.html)
- [SDK installation](https://support.huaweicloud.com/sdk-python-devg-obs/obs_22_0400.html)
- [OBS client initialization and ECS Agency](https://support.huaweicloud.com/sdk-python-devg-obs/obs_22_0601.html)
- [File upload and custom metadata](https://support.huaweicloud.com/intl/en-us/sdk-python-devg-obs/obs_22_0903.html)
- [Object metadata / HEAD](https://support.huaweicloud.com/sdk-python-devg-obs/obs_22_0920.html)
- [Resumable multipart upload](https://support.huaweicloud.com/sdk-python-devg-obs/obs_22_0905.html)

## Runtime strategy

生产宿主机仍是 Python 3.6.8。本阶段没有把 SDK 安装到宿主机，也没有修改生产 compose。推荐未来使用独立 backup-uploader container，固定 Python 与 `esdk-obs-python` 版本，并只挂载备份目录和必要的 secret。这样 SDK 升级不会污染 CentOS 宿主机。

当前脚本只在 `--provider obs --upload` 时 lazy-import `obs`；未安装 SDK 时返回 `CONFIG_REQUIRED`，不会 fallback 到 MinIO、S3 或其他业务 provider。fake provider 测试不需要 SDK。

## Shared bucket boundary

当前允许与 AI 图片、视频共用 bucket，但数据库备份 object key 强制从以下固定前缀开始：

```text
backups/postgres/
```

当前实现的 deploy key：

```text
backups/postgres/deploy/YYYY/MM/<basename>
```

daily/event 预留为同一前缀下的对应 category。调用者不能传入 prefix；业务的 `images/`、`videos/`、`assets/`、`user-files/` 不在 uploader 的可写范围内。

未来迁移到独立 bucket `zhiqiyun-ai-prod-backups` 时，只改变 bucket/endpoint 配置，不改变文件名、meta schema 或 retention 状态模型。

## Credentials

推荐优先使用 ECS Agency：

```text
BACKUP_OBS_SECURITY_PROVIDER=ECS
BACKUP_OBS_BUCKET=...
BACKUP_OBS_ENDPOINT=https://obs.<region>.myhuaweicloud.com
BACKUP_OBS_REGION=...
```

若暂时使用环境变量凭证，使用 OBS SDK 识别的变量：

```text
BACKUP_OBS_SECURITY_PROVIDER=ENV
OBS_ACCESS_KEY_ID=...
OBS_SECRET_ACCESS_KEY=...
OBS_SECURITY_TOKEN=...   # temporary credentials only
```

不把 AK、SK、SecurityToken 写入 Git、fixture、meta、sidecar、deploy log、stdout 或 stderr。缺 bucket、endpoint 或 region 返回 `CONFIG_REQUIRED`。本阶段没有真实凭证。

## IAM and prefix isolation

建议创建只服务于 backup uploader 的身份，资源限制到：

```text
obs:*:*:object:<bucket>/backups/postgres/*
```

所需能力按用途拆分：

- `obs:object:putObject`：backup、meta、sha256 和 multipart 初始化/上传/合并
- `obs:object:getObject`：HEAD/恢复读取；HEAD 所需权限按官方文档为 GetObject
- `obs:object:listMultipartUploadParts`：只在需要列出 multipart parts 时
- `obs:bucket:listBucket`：只有确实需要 prefix inventory 时才增加

不授予 DeleteObject、DeleteBucket、PutBucketPolicy、PutBucketAcl、生命周期修改或其他 bucket/业务 prefix 权限。Phase 3B 不配置真实 IAM policy。

## Upload and verification

顺序固定为：

1. 校验 regular file、realpath、meta、gzip、bytes、SHA256。
2. 用固定 `backups/postgres/` key HEAD。
3. 已存在且 size/SHA256 metadata 一致时返回 `ALREADY_OFFSITE_VERIFIED`。
4. 不存在时上传 backup，并将 `x-obs-meta-sha256` 写入对象 metadata。
5. HEAD 校验 remote size 与 sha256 metadata。
6. 上传并 HEAD 校验 `.meta.json`。
7. 上传并 HEAD 校验 `.sha256`。
8. 最后原子写 `<backup>.offsite.json`，状态为 `OFFSITE_VERIFIED`。

size 和自定义 SHA256 metadata 是完整性依据；ETag 只记录，不当作 SHA256。任何 auth、timeout、网络、partial、meta 或 sha sidecar 失败都不会生成 `OFFSITE_VERIFIED`。

## Lifecycle, versioning and WORM

共享 bucket 上未来若配置生命周期，只能使用精确 prefix filter：

```text
backups/postgres/
```

禁止对整个共享 bucket 配置 90/180 天删除，否则可能影响业务图片和视频。OBS 生命周期文档明确空 prefix 会作用于整个 bucket。

Versioning/WORM/immutable storage 建议等拆分独立 backup bucket 后再启用。它们可能改变业务素材的覆盖/删除行为，不在共享 bucket 上提前开启。本阶段不创建 lifecycle、不改 versioning、不改 WORM。

## Retention integration

本阶段不修改 `backup-retention.sh`，没有真实删除。未来安全门禁仍为：

```text
DELETE_CANDIDATE + offsite.verification == OFFSITE_VERIFIED
    -> APPLY_ELIGIBLE

otherwise
    -> LOCAL_ONLY_PROTECTED
```

## Production one-shot container

生产宿主机不需要安装 OBS SDK。`compose.prod.yml` 中的 `backup-uploader` 只启用
`backup-uploader` profile，默认不会启动，也不属于业务服务。它使用固定的 Python
3.11 镜像和锁定的 `esdk-obs-python` 版本，容器为 read-only，仅将
`backups/postgres` 作为必要的 sidecar 输出目录挂载为可写。

先在生产服务器以 root 创建权限为 600 的 secret env file，例如：

```text
/opt/zhiqiyun-ai/secrets/backup-obs.env
```

该文件只应包含专用 backup identity 的 OBS 变量，不应包含业务 storage credentials：

```text
BACKUP_OBJECT_PROVIDER=obs
BACKUP_OBS_BUCKET=zhiqiyun-private
BACKUP_OBS_ENDPOINT=https://obs.cn-north-9.myhuaweicloud.com
BACKUP_OBS_REGION=cn-north-9
BACKUP_OBS_SECURITY_PROVIDER=ECS
```

若使用 ENV 凭证，额外变量只放在该文件中：

```text
OBS_ACCESS_KEY_ID=...
OBS_SECRET_ACCESS_KEY=...
OBS_SECURITY_TOKEN=...
```

执行一次上传时，通过 `BACKUP_OBS_ENV_FILE` 指向该文件；不要把 secret 放在命令
行或 Git tracked env 中：

```bash
BACKUP_OBS_ENV_FILE=/opt/zhiqiyun-ai/secrets/backup-obs.env \
docker compose --env-file .env.production --profile backup-uploader \
  run --rm backup-uploader \
  --root /var/lib/zhiqiyun/backups/postgres \
  --file /var/lib/zhiqiyun/backups/postgres/db_YYYYMMDD_HHMMSS_<sha>.sql.gz \
  --provider obs --upload
```

该服务不是常驻服务，不扫描目录，也不启用删除能力；每次只传入一个明确的备份
文件。没有 secret env file 时，OBS provider 应返回 `CONFIG_REQUIRED`。
