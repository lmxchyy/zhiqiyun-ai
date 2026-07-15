# 对象存储与文件中心：第一阶段实施报告

实施日期：2026-07-14

## 1. 结论

第一阶段已在现有技术栈上完成：统一存储配置、统一文件元数据、租户配额、MinIO/S3 兼容适配器、前端直传签名、上传完成校验、访问签名、回收站、恢复、永久删除、管理端配置/文件/配额页面，以及 Docker 运行态验证。

本项目当前使用 PostgreSQL 和字符串 ID，因此没有机械照搬需求文档中的 MySQL/BIGINT 示例；表结构、事务和索引均按当前 PostgreSQL 主链路实现。现有图片、媒体、知识库和 PPT 文件链路没有被替换，避免影响已有功能。

## 2. 改造前现状

- 后端：Go，Gin 与现有 `net/http` handler 兼容封装。
- 数据库：PostgreSQL，通过 `database/migrations/*.sql` 迁移。
- 认证：Bearer access token，用户、租户和管理员权限沿用现有会话与 RBAC。
- 对象存储：项目已有 MinIO/S3 依赖和媒体存储实现，但只服务于局部媒体场景。
- 文件入口：图片素材、知识库文档、PPT/Office 产物、生成任务 URL 等由不同模块分别管理，缺少统一文件 ID、配额和生命周期。
- 部署：Docker Compose 已包含 PostgreSQL、Redis、RabbitMQ、MinIO 和主应用。

## 3. 第一阶段架构

```text
Browser / App
    │ 1. POST /files/upload/init
    ▼
File Center Service ── quota reservation ── PostgreSQL
    │
    │ 2. presigned PUT URL (public endpoint)
    ▼
Browser ───────────── PUT ────────────────► MinIO / S3
    │
    │ 3. POST /files/upload/complete
    ▼
File Center Service ── HeadObject + size/MIME check ──► Object Storage
    │
    └── ACTIVE metadata + quota settlement ───────────► PostgreSQL
```

存储 Endpoint 被拆为两个用途：

- `S3_ENDPOINT`：服务端容器内部连接地址，例如 `http://minio:9000`。
- `STORAGE_PUBLIC_ENDPOINT`：签名 URL 使用、浏览器可访问的地址，例如开发环境 `http://localhost:9000`、生产环境 `https://files.example.com`。

不能在生成签名后简单替换域名，因为 AWS SigV4 会将 Host 纳入签名；实现中使用独立签名客户端保证签名有效。

## 4. 数据模型

迁移文件：`database/migrations/046-storage-file-center.sql`。

| 表 | 用途 |
|---|---|
| `xz_storage_configs` | 平台/租户存储配置；密钥只保存 AES-GCM 密文 |
| `xz_file_objects` | 文件统一元数据、对象键、状态、业务归属、临时/过期信息 |
| `xz_file_relations` | 源文件与派生产物关系基础 |
| `xz_tenant_storage_quotas` | 租户额度、已用、预留、文件数和预警阈值 |
| `xz_storage_jobs` | 后续清理、迁移、重试任务的持久化基础 |

对象键格式：

```text
tenants/{tenantId}/{businessType}/{yyyy}/{mm}/{dd}/{fileId}.{ext}
```

上传初始化和配额预留在数据库事务中完成；上传完成后通过 `HeadObject` 校验实际大小和 MIME，再把预留额度转为已用额度。永久删除成功后再扣减已用额度。

## 5. Provider 能力

统一接口当前包括：

- `PutObject`
- `HeadObject`
- `DeleteObject`
- `CopyObject`
- `CreatePresignedUploadURL`
- `CreatePresignedDownloadURL`
- `TestConnection`

第一阶段对 MinIO 做了完整运行验证。通用 S3、Cloudflare R2、阿里云 OSS、腾讯云 COS、华为云 OBS 已提供 S3 兼容配置入口；正式接入各云厂商前仍需按实际账号验证 endpoint、region、path style 和兼容差异。官方 SDK 专用适配器不在第一阶段范围。

## 6. 用户 API

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/v1/files/upload/init` | 校验文件与配额，创建 PENDING_UPLOAD 记录，返回 PUT 签名 |
| POST | `/api/v1/files/upload/complete` | HeadObject 校验并完成入库 |
| GET | `/api/v1/files/:fileId` | 查询当前用户可访问的文件 |
| GET | `/api/v1/files/:fileId/access-url` | 获取短期访问 URL |
| GET | `/api/v1/files/:fileId/download-url` | 获取短期下载 URL |
| DELETE | `/api/v1/files/:fileId` | 移入回收站 |
| POST | `/api/v1/files/:fileId/restore` | 从回收站恢复 |
| DELETE | `/api/v1/files/:fileId/permanent` | 永久删除对象和逻辑文件 |

`packages/business-sdk/src/files.ts` 提供浏览器侧完整直传 helper：初始化、PUT、完成确认、访问、删除和恢复。

## 7. 管理 API 与页面

管理端新增“对象存储与文件中心”，包含：

- 概览：文件总量、容量、待完成、回收站、异常文件、Provider 占用和租户配额。
- 存储配置：新增、编辑、删除、连接测试、默认配置和启停。
- 文件管理：筛选、下载、删除、恢复、永久删除。
- 配额：租户额度、告警阈值和严重阈值。

管理员 API 位于 `/api/v1/admin/storage/*`。RBAC 新增 `storage:view`、`storage:config:*`、`storage:file:*`、`storage:quota:*` 和 `storage:job:retry`。平台超级管理员拥有完整权限，平台管理员第一阶段默认拥有查看、下载、连接测试和配额查看权限。

接口不会返回 Access Key、Secret Key 或 Session Token；只返回 `hasAccessKey` / `hasSecretKey`。配置密钥使用 `STORAGE_MASTER_KEY` 派生的 AES-GCM 密钥加密。

## 8. 配置

核心环境变量：

```dotenv
S3_ENDPOINT=http://minio:9000
STORAGE_PUBLIC_ENDPOINT=http://localhost:9000
S3_REGION=us-east-1
S3_ACCESS_KEY=...
S3_SECRET_KEY=...
S3_BUCKET=xianzhi-assets
STORAGE_MASTER_KEY=change-me-at-least-32-bytes
STORAGE_DEFAULT_QUOTA_BYTES=10737418240
STORAGE_MAX_UPLOAD_BYTES=2147483648
STORAGE_UPLOAD_URL_TTL_SECONDS=600
STORAGE_ACCESS_URL_TTL_SECONDS=900
STORAGE_RECYCLE_DAYS=30
STORAGE_AUTO_CREATE_BUCKET=true
STORAGE_FORCE_PATH_STYLE=true
```

生产模式会强制校验 `STORAGE_PUBLIC_ENDPOINT` 和 `STORAGE_MASTER_KEY`。生产密钥必须由 Secret Manager、Kubernetes Secret 或同等级设施注入，不应提交到仓库。

## 9. 验证结果

已完成：

- 存储包和相关 HTTP 定向测试通过。
- `packages` TypeScript 全量类型检查通过。
- 管理端 `vue-tsc` 与 Vite 生产构建通过。
- `docker compose config --quiet` 通过。
- PostgreSQL 迁移在运行实例中成功应用并可幂等重放。
- Docker 主应用重新构建成功，`xianzhi-ai`、PostgreSQL、Redis、RabbitMQ、MinIO 均健康。
- 运行态 `/admin/` 返回 200，实际加载的生产 bundle 已确认包含 `storageCenter` 模块和 `/admin/storage/statistics/overview` API。
- 真实 E2E：连接测试成功；签名地址为 `localhost:9000`；34 字节文件直传成功；完成后状态为 `ACTIVE`；下载内容逐字节一致；删除后 `DELETE_PENDING`；恢复后 `ACTIVE`；永久删除返回 204；配额回到 0。
- MinIO 浏览器预检验证通过：带应用 Origin、PUT 方法和 Content-Type 请求头的 OPTIONS 返回 204，并返回对应的 CORS allow headers。
- 验收创建的临时账号、会话、对象和文件记录均已清理，数据库确认无残留测试文件，租户 used/reserved/file_count 均为 0。

全量 `go test ./...` 中，本次新增 `internal/storage` 全部通过；仓库已有的 `TestAssetCenterLifecycle` 仍失败，错误为 `billing_account_id is not allowed by schema for module image_generation`。该失败位于既有资产中心/图片计费参数校验链路，与文件中心改造无关，本次未扩大范围修改。

## 10. 回滚与兼容策略

- 代码回滚：移除文件中心路由和管理端入口后，既有媒体/知识库/PPT 路径仍按原逻辑运行。
- 配置回滚：停用数据库存储配置即可回退到环境变量 `env_default`。
- 数据回滚：优先保留新表，避免丢失对象索引；如必须删除表，应先备份并确认无业务引用，再由 DBA 按外键依赖顺序执行，不提供自动批量删除脚本。
- 对象回滚：数据库回滚不会自动删除 Bucket 中的对象，防止误删真实文件。

## 11. 未完成范围与下一步

第一阶段没有冒充完成以下后续内容：

1. 把 AI 图片/视频、PPT、知识库和作品中心的所有历史文件写入点切换到统一 `file_id`。
2. 分片上传、断点续传、秒传、远程 URL 拉取和客户端进度 UI。
3. 定时清理 worker、失败重试 worker、跨 Provider 迁移和完整审计操作台。
4. 图片缩略图、视频转码、文档预览、病毒扫描和内容安全流水线。
5. 各云厂商真实账号的兼容认证和专用 SDK 适配。

建议下一阶段优先迁移“AI 生成结果”和“PPT/Office 产物”两条高价值链路，并在双写观察期内保留旧 URL 字段，验证稳定后再逐步切读。
