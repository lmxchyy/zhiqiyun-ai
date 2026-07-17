# 品牌宣传物料公共存储策略

## 推荐落点

品牌海报和视频属于需要长期公开访问的品牌资产，应使用数据库中当前启用的“R2 公共存储”配置（Bucket：`zhiqiyun-public`），不要放入默认私有桶，也不要提交到 Git。

| 资产 | businessType | businessId | visibility | 原始文件名 |
| --- | --- | --- | --- | --- |
| 海报 | `marketing_poster` | `poster-{NNN}` | `PUBLIC` | `poster-{NNN}.png` |
| 视频 | `marketing_video` | `video-{NNN}` | `PUBLIC` | `video-{NNN}.{ext}` |

## 上传约束

1. 上传前从 `xz_storage_configs` 动态查找当前租户可用、`bucket=zhiqiyun-public`、`status=ENABLED` 的公共配置，并确认连接测试为 `SUCCESS`；当前运行态对应 `tenant_id=tenant_default`、`name=R2 公共存储`。使用查询到的当前 `id`，禁止把存储配置 ID、密钥、Endpoint 或域名写死在任务文件或业务代码中。
2. 调用文件中心上传初始化时，必须同时传入解析后的 `storageConfigId` 和 `visibility=PUBLIC`。仅传 `PUBLIC` 不会自动选择公共桶。
3. `isTemporary=false`，品牌物料不得进入临时过期清理。
4. 上传完成后必须调用 complete 接口，并保存返回的 `fileId`、`objectKey` 与最终公共 URL。
5. URL 对外展示优先使用公共配置的 CDN/Public Domain；不得公开签名 Endpoint、Access Key 或 Secret Key。

## 当前对象键真相

现有 `backend-go/internal/storage/service.go` 统一生成：

```text
tenants/{tenantId}/{businessType}/{yyyy}/{mm}/{dd}/{objectId}.{ext}
```

因此平台海报的实际对象键形如：

```text
tenants/tenant_default/marketing_poster/2026/07/17/{objectId}.png
```

视频同理位于 `marketing_video`。对象键中的随机 ID 用于避免覆盖；稳定业务定位依靠 `businessType + businessId`，展示文件名依靠 `originalName`。当前文件中心不接受调用方自定义对象键，因此不要绕过服务直接向 Bucket 写入“漂亮路径”。

## 发布记录

每个任务完成后，在对应 Markdown 的 front matter 中填写：

```yaml
status: completed
file_id: file_xxx
object_key: tenants/tenant_default/marketing_poster/...
public_url: https://...
generated_at: 2026-07-17T00:00:00+08:00
```

Docker Desktop 未运行或无法核对实时配置时，不得猜测 ID 或假装上传成功；保持任务未完成并报告阻塞原因。
