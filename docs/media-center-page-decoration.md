# 素材中心 + 页面装修 + 图片占位替换

## 架构与复用边界

当前项目以 PostgreSQL、`database/sql`、Gin、Vue3/Element Plus、uni-app/Pinia 为真实运行栈。本模块复用现有会话、管理员 RBAC、审计中间件、Redis、统一 API 客户端和 Docker 迁移容器；运营素材使用独立 `xz_media_*` 表，不与用户生成作品 `xz_assets` 混用。

页面读取优先级为：当前租户素材位覆盖 → 平台 `default` 素材位 → 小程序本地基础兜底。素材 ID 是配置主键，URL 仅是后端解析后的交付字段。

## 数据库迁移

`037-media-page-decoration.sql` 创建：

- `xz_media_assets`：素材元数据、存储信息、哈希、审核、版权和软删除。
- `xz_media_categories`：租户素材分类树。
- `xz_page_asset_slots`：四页素材位及有效期、启停、fallback。
- `xz_page_configs`：租户页面草稿和当前发布版本。
- `xz_page_config_versions`：不可变发布快照。
- `xz_media_asset_usage`：素材引用位置。

迁移包含 13 个平台默认分类、38 个 V5.3 RC 素材位和四页初始草稿，使用 `if not exists` / `on conflict`，可重复执行。Docker 启动时 `migrate` 服务自动按文件名顺序应用。

## 管理 API

统一前缀 `/api/v1`，后台接口由现有 RBAC 和审计中间件保护：

- `GET /admin/media/assets`
- `POST /admin/media/assets/upload`
- `POST /admin/media/assets/batch-upload`
- `GET|PUT|DELETE /admin/media/assets/:id`
- `POST /admin/media/assets/:id/enable|disable`
- `GET /admin/media/assets/:id/usages`
- `GET|POST /admin/media/categories`
- `PUT|DELETE /admin/media/categories/:id`
- `GET|PUT /admin/page-configs/:pageCode`
- `POST /admin/page-configs/:pageCode/publish`
- `GET /admin/page-configs/:pageCode/versions`
- `POST /admin/page-configs/:pageCode/rollback/:version`
- `GET /admin/page-slots/:pageCode`
- `PUT /admin/page-slots/:pageCode/:slotKey`

公开合并接口：`GET /api/v1/app/pages/{home|studio|assets|profile}` 和兼容入口 `GET /api/v1/app/page-config/:pageCode`。响应含 `modules`、`slots`、`slotList`、租户和版本；不返回内部 `storage_key`。Redis 缓存十分钟，素材位变更、发布或回滚会主动删除对应缓存。

## 后台操作

1. 在「运营中心 → 素材中心」选择分类并上传一张或多张图片。
2. 上传支持 jpg/jpeg/png/webp/avif/svg；服务端校验扩展名、实际 MIME、大小、最大 16384px、SHA-256，并拦截 SVG 脚本、事件和外链。
3. 在「页面装修」选择页面，点击手机预览或左侧素材位，从素材选择器绑定 `asset_id`。
4. 可拖动素材位排序，设置启停、替代文本和生效区间，先保存草稿再发布。
5. 发布产生新版本；版本抽屉可选择旧版本回滚并立即再次发布。
6. 正在被素材位引用的素材不能删除；先在页面装修中替换引用。

## 小程序联调与回退

`usePageConfig`/`pageConfig store` 先读 `uni` 本地缓存，同时请求最新版本；版本或素材位变化才覆盖缓存。`AppImage` 回退顺序：主 URL → 后端 fallback URL → `/static/fallbacks/` 基础图。`RemoteCover` 已接入首页 Hero/工具/灵感、创作 Banner/模板、作品默认封面、个人头像和背景。真实作品缩略图仍来自作品 API，不属于运营图片。

## 演示素材初始化

`backend-go/static/demo-assets/manifest.json` 定义 18 个测试素材。迁移完成后执行：

```powershell
cd backend-go
$env:DATABASE_URL='postgresql://xianzhi:xianzhi@localhost:54321/xianzhi?sslmode=disable'
$env:MEDIA_STORAGE_ROOT='../data/media-assets'
go run ./cmd/seed-media
```

命令生成测试 SVG、写入当前本地存储、按内容哈希去重、创建素材记录并绑定素材位。它只用于测试/演示；生产环境应由运营人员上传正式素材。

## 环境变量

- `MEDIA_STORAGE_PROVIDER`：支持 `local`、`s3`、`aliyun_oss`、`tencent_cos`、`qiniu`；后四种统一走 S3 兼容协议，使用下方 S3 连接参数。
- `MEDIA_STORAGE_ROOT`：本地对象根目录。
- `MEDIA_PUBLIC_BASE_URL`：同源公开文件前缀，默认 `/api/v1/media/files`。
- `MEDIA_CDN_BASE_URL`：生产 CDN 根域名，设置后优先对外返回。
- `MEDIA_MAX_UPLOAD_BYTES`：单文件上限，默认 12 MiB。
- `MEDIA_KEEP_ORIGINAL`：保留原图策略开关。

## 正式图片来源与版权

优先使用有记录的 AI 生成图（保存 prompt、model、时间和版权说明）、企业自有 Logo/产品/员工/案例素材、Adobe Stock、Shutterstock、Envato Elements、Icons8、Storyset、LottieFiles 等正版库，或运营人员明确上传的授权素材。禁止未授权网络图、竞品图片、来源不明图片和第三方水印图。表中已预留 `source_type/source_name/license_type/license_note/prompt/model_name/copyright_owner`。

## 部署注意

生产建议 CDN 使用 HTTPS、长缓存和内容哈希 URL。`local` Provider 适合单实例或共享持久卷；多实例生产把 `MEDIA_STORAGE_PROVIDER` 切到 OSS/COS/七牛/S3，并将 `S3_ENDPOINT/S3_ACCESS_KEY/S3_SECRET_KEY/S3_BUCKET` 指向供应商的 S3 兼容端点。未配置 `MEDIA_CDN_BASE_URL` 时需要将 Bucket 配置为公开读；私有 Bucket 应配置 CDN 鉴权或由部署层扩展签名 URL 刷新。供应商密钥只放环境变量或系统密钥管理，不进入页面配置和 API 响应。
