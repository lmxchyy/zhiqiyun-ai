# 先知 AI 生产环境迁移整理

本文用于把当前项目从本地/Docker 演示环境迁移到生产环境前做统一盘点。它不是替代现有架构文档，而是把生产上线需要准备的资源、模块边界、迁移步骤、验证项和回滚预案串在一起。

## 1. 当前项目定位

先知 AI 当前是“一站式内容生产 + 代理商运营 + 主控 SaaS 后台”平台，正在从早期 Node/静态页面实现迁移到以下目标架构：

| 层级 | 当前主入口 | 说明 |
| --- | --- | --- |
| 员工/客户产品端 | `frontend-vue` | uni-app + Vue 3，H5/小程序/App 同源工程，H5 构建后由 Go 服务托管。 |
| PC 管理后台 | `admin-vue` | Vue 3 + Vite + Element Plus，构建产物由 Go 服务托管在 `/admin/`。 |
| 后端 API | `backend-go` | Go 1.25 + Gin，统一提供 `/api/v1/*`、`/api/v1/admin/*` 和客户侧兼容接口。 |
| 数据库 | `database/schema.sql` + `database/migrations/*` | PostgreSQL 作为生产主数据源，Redis/RabbitMQ/MinIO 作为基础设施。 |
| Legacy 参考 | 已移除 | 旧 `frontend`、`backend`、`admin-web` 不再保留在当前工作树。 |

生产迁移应以 `frontend-vue + admin-vue + backend-go + PostgreSQL` 为准。

## 2. 当前已有板块

### 2.1 产品端

- 登录认证与会话：`/api/v1/auth/login`、`/api/v1/auth/me`、`/api/v1/auth/logout`、`/api/v1/auth/change-password`。
- AI 创作工作台：生成任务、模型选择、提示词、参考图、素材历史。
- 灵感画布：`frontend-vue/src/components/InspirationCanvas.vue` 和相关 API 封装。
- 用户仪表盘：积分账户、用量、在线图片、AI 状态保存。
- API 设置：员工端可配置/查看推荐 API、平台供应商和使用设置。
- 素材资产：资产列表、下载、缩略图补全、删除。

### 2.2 PC 管理后台

- 数据总览：经营指标、客户、订单、用量、渠道等概览。
- 客户中心：客户列表、新建、更新、状态管理。
- 代理商中心：一级/二级渠道、渠道树、渠道客户、提现与佣金。
- 产品中心：文生图、API、Agent、GEO、代运营产品目录。
- 套餐中心：套餐价格、权益、状态和模型权限。
- 订单中心：手工订单、标记收款、续费。
- 交付中心：交付项目和进度。
- 用量中心：用量列表、导出、生成任务管理。
- 分润中心：佣金、提现审核、结算相关操作。
- 系统中心：品牌、模型供应商、API 上游渠道、模型目录、API Key、客户分组。
- 计费中心：计费概览、客户计费、产品/套餐、订阅、事件、用量汇总、账单、支付、钱包、优惠券等 Lago 风格基础视图。

### 2.3 后端与接口

- 服务入口：`backend-go/cmd/api/main.go`。
- 路由入口：`backend-go/internal/httpserver/server.go`。
- 配置入口：`backend-go/internal/config/config.go`。
- PostgreSQL 主数据仓库：`backend-go/internal/httpserver/postgres_store.go`。
- 后台管理 API：`backend-go/internal/httpserver/admin_api.go`、`admin_types.go`。
- 渠道代理 API：`backend-go/internal/httpserver/channel_api.go`。
- 认证 API：`backend-go/internal/httpserver/auth_api.go`。
- 图片供应商：`backend-go/internal/provider/image/*`。
- 视频供应商：`backend-go/internal/provider/video/*`。
- 聊天供应商：`backend-go/internal/provider/chat/*`。

### 2.4 数据与基础设施

- PostgreSQL：核心业务数据、领域表、审计、RBAC、备份记录。
- Redis：缓存、后续队列/限流/会话辅助。
- RabbitMQ：异步任务基础设施。
- MinIO：附件、素材、合同、发票、交付文件等对象存储。
- Docker Compose：当前本地一键启动 `xianzhi-ai`、`postgres`、`redis`、`rabbitmq`、`minio`、`postgres-backup`、`migrate`。

## 3. 已有项目文档

| 文档 | 用途 |
| --- | --- |
| `README.md` | 当前技术栈、启动命令、Docker、演示账号、生产架构状态。 |
| `docs/architecture-modernization.md` | Vue3/uni-app + Go 架构基线和开发边界。 |
| `docs/admin-saas-master-control-plan.md` | 主控 SaaS 的业务中心、数据模型、接口和开发阶段。 |
| `docs/lago-style-billing-saas-plan.md` | 计费中心、订阅、计量事件、账单和发票方案。 |
| `先知AI平台开发文档.md` | 原始产品/业务需求总文档。 |
| `实施进度.md` | 已实现模块和开发进度记录。 |
| `认证会话实施说明.md` | 登录、会话和认证实现说明。 |
| `生成任务与作品闭环实施说明.md` | 生成任务、素材、作品闭环说明。 |
| `会员权益与兑换码实施说明.md` | 会员、积分、权益、兑换码相关说明。 |
| `模型供应商与计费管理实施说明.md` | 模型供应商、计费规则和成本管理说明。 |
| `代理商业绩与排行榜实施说明.md` | 代理商业绩、排行榜和渠道数据说明。 |
| `代理商结算单实施说明.md` | 结算单和提现相关说明。 |
| `GEO内容发布与效果跟踪实施说明.md` | GEO 内容发布和效果跟踪说明。 |
| `PPT编辑与导出实施说明.md` | PPT 生成、编辑和导出说明。 |
| `智能体分享与反馈实施说明.md` | Agent 分享、调用和反馈说明。 |
| `Docker会员模型权限实施说明.md` | Docker 环境下会员和模型权限说明。 |

## 4. 生产迁移前必须准备

### 4.1 服务器与网络

- 生产服务器或云主机，建议至少准备应用节点、数据库持久化存储和对象存储持久化卷。
- 域名与 HTTPS 证书，例如：
  - 产品端：`https://app.example.com` 或主域 `/`。
  - 管理后台：`https://admin.example.com` 或 `/admin/`。
  - API：可复用同域 `/api/v1`，也可独立 `https://api.example.com`。
- 反向代理：Nginx、Caddy、Traefik 或云负载均衡。
- 防火墙策略：只公开 Web 入口，PostgreSQL/Redis/RabbitMQ/MinIO 管理端默认不公网暴露。
- 日志采集与监控：至少要有容器日志、应用健康检查、磁盘容量、数据库连接数和错误率监控。

### 4.2 运行时与构建环境

- Docker 与 Docker Compose。
- Node.js 22+，生产镜像当前使用 Node 24 Alpine 构建前端。
- Go 1.25，生产镜像当前使用 `golang:1.25-alpine` 构建 API。
- PostgreSQL 16。
- Redis 7。
- RabbitMQ 3 management。
- MinIO。

### 4.3 环境变量与密钥

生产环境必须替换掉 `compose.yml` 中的演示账号和默认密钥。

| 变量 | 说明 |
| --- | --- |
| `PORT` 或 `XIANZHI_GO_ADDR` | Go 服务监听端口，默认 `3100`。 |
| `DATABASE_URL` | PostgreSQL 连接串，生产必须使用强密码。 |
| `REDIS_URL` | Redis 连接串。 |
| `RABBITMQ_URL` | RabbitMQ 连接串。 |
| `S3_ENDPOINT` | MinIO 或云对象存储地址。 |
| `S3_ACCESS_KEY` | 对象存储访问 Key。 |
| `S3_SECRET_KEY` | 对象存储密钥。 |
| `S3_BUCKET` | 生成作品、附件和导出文件使用的对象存储桶名。 |
| `OPENAI_API_KEY` / `MODEL_PROVIDER_API_KEY` | 默认模型供应商密钥。 |
| `OPENAI_BASE_URL` / `MODEL_PROVIDER_URL` | OpenAI 或兼容模型服务地址。 |
| `MODEL_PROVIDER_KIND` | 模型供应商类型。 |
| `MODEL_PROVIDER_IMAGE_MODEL` | 默认图片模型，当前默认 `gpt-image-2`。 |
| `MODEL_PROVIDER_VIDEO_MODEL` | 默认视频模型，当前默认 `sora-2`。 |
| `MODEL_PROVIDER_TIMEOUT_MS` | 模型请求超时时间。 |
| `MODEL_PROVIDERS_JSON` | 多供应商配置，适合生产配置多渠道。 |
| `PAYMENT_CALLBACK_SECRET` | 支付回调签名密钥，生产必须替换。 |
| `WECHAT_PAY_CALLBACK_SECRET` | 微信支付回调密钥。 |
| `ALIPAY_CALLBACK_SECRET` | 支付宝回调密钥。 |
| `METRICS_TOKEN` | 指标或内部运维接口 Token。 |
| `XIANZHI_ENFORCE_RBAC` | 是否强制启用后台 RBAC，生产建议为 `true`。 |
| `XIANZHI_DEV_AUTH_FALLBACK` / `XIANZHI_ALLOW_INSECURE_AUTH_TOKEN` | 是否允许无 Redis 的开发 token 降级，生产必须为 `false`。 |
| `XIANZHI_ENABLE_MOCK_LOGIN` / `XIANZHI_ALLOW_WECHAT_MOCK_LOGIN` | 是否允许小程序 mock 登录，生产必须为 `false`。 |
| `XIANZHI_DATA_PATH` | JSON fallback 文件路径，生产仅用于兼容或降级。 |
| `XIANZHI_STATIC_DIR` | 产品端 H5 静态资源目录。 |
| `XIANZHI_ADMIN_STATIC_DIR` | 管理后台静态资源目录。 |

### 4.4 数据准备

- 确认 PostgreSQL 已初始化 `database/schema.sql`。
- 确认 `database/migrations/*.sql` 已按文件名顺序执行。
- 生产数据库应启用自动备份和恢复演练。
- 导入演示数据前需要确认是否允许演示账号进入生产；生产建议删除或禁用默认演示账号。
- 若从本地/测试环境迁移数据，先执行备份，再在 staging 环境恢复验证。

### 4.5 安全准备

- 禁止使用 README 中的演示账号密码作为生产账号。
- 所有默认密码必须改掉，包括 PostgreSQL、RabbitMQ、MinIO、后台管理员、支付回调密钥。
- 后台 `/admin/` 建议限制来源 IP 或至少开启强 RBAC。
- 对象存储桶权限默认私有，通过应用签名或受控下载。
- 数据库和对象存储备份必须加密或放在受控存储中。
- 支付回调、模型供应商回调、导出接口都要保留审计日志。

## 5. 推荐生产部署结构

第一阶段推荐继续使用单应用容器，降低迁移复杂度：

```text
反向代理 / HTTPS
  -> xianzhi-ai 应用容器
       /                  产品端 H5
       /admin/             PC 管理后台
       /api/v1/*           产品与后台 API
       /v1/dashboard/*     客户侧兼容计费 API

PostgreSQL
Redis
RabbitMQ
MinIO 或云对象存储
```

后续当流量变大后再拆分：

- `xianzhi-api`：只跑 Go API。
- `xianzhi-admin-web`：如后续拆分独立静态服务，可承载 `admin-vue` 构建产物。
- `xianzhi-web`：产品端 H5 静态资源。
- `worker`：生成任务、计费聚合、GEO 监控、通知等异步任务。

## 6. 迁移步骤

### 阶段 0：冻结与盘点

1. 确认当前 Git 分支、未提交改动和需要上线的版本范围。
2. 确认 `frontend-vue`、`admin-vue`、`backend-go` 是本次生产迁移的目标入口。
3. 确认当前生产入口只包含 `frontend-vue`、`admin-vue`、`backend-go`。
4. 列出生产必需的第三方账号：模型供应商、对象存储、短信/邮件、支付、域名、证书。
5. 明确上线窗口、回滚窗口、负责人和验收人。

### 阶段 1：构建验证

在迁移前先在本地或 staging 执行：

```powershell
npm.cmd --prefix admin-vue run build
npm.cmd --prefix frontend-vue run typecheck
npm.cmd --prefix frontend-vue run build
npm.cmd test
docker compose -f compose.yml build --progress plain
```

若本机没有 Go 工具链，可使用 Docker Go 镜像执行 `go test ./...`，但上线前必须有一次完整后端测试记录。

### 阶段 2：数据库初始化与迁移

1. 创建生产 PostgreSQL 数据库和独立用户。
2. 执行基础 schema：

```powershell
psql "<生产 DATABASE_URL>" -v ON_ERROR_STOP=1 -f database/schema.sql
```

3. 按文件名顺序执行迁移：

```powershell
Get-ChildItem database\migrations\*.sql | Sort-Object Name
```

逐个执行每个 SQL 文件，确认无错误。不要跳过 `021-runtime-projections.sql` 和 `022-production-governance.sql`。

4. 在 staging 恢复一次备份，验证应用可读写。

### 阶段 3：配置生产环境

1. 准备 `.env.production` 或云平台 Secret，不把生产密钥提交到仓库。
2. 替换所有演示密码和默认密钥。
3. 配置 `DATABASE_URL`、`REDIS_URL`、`RABBITMQ_URL`、对象存储、模型供应商、支付回调密钥。
4. 生产建议设置：

```text
XIANZHI_ENFORCE_RBAC=true
XIANZHI_DEV_AUTH_FALLBACK=false
XIANZHI_ALLOW_INSECURE_AUTH_TOKEN=false
XIANZHI_ENABLE_MOCK_LOGIN=false
XIANZHI_ALLOW_WECHAT_MOCK_LOGIN=false
MODEL_PROVIDER_TIMEOUT_MS=30000
```

5. 确认反向代理转发：

```text
/api/v1/*            -> xianzhi-ai:3100
/v1/dashboard/*      -> xianzhi-ai:3100
/admin/*             -> xianzhi-ai:3100
/*                   -> xianzhi-ai:3100
```

### 阶段 4：部署应用

Docker Compose 方式：

```powershell
docker compose -f compose.yml up -d --build
docker compose -f compose.yml ps
```

生产环境建议不要直接复用当前 `compose.yml` 的明文默认密码。应复制为生产 Compose 或云平台配置文件后替换：

- 数据库密码。
- RabbitMQ 用户密码。
- MinIO Root 用户密码。
- 支付和模型供应商密钥。
- 端口暴露策略。

### 阶段 5：上线验证

基础健康检查：

```powershell
curl.exe -i http://localhost:3100/api/v1/health
curl.exe -i http://localhost:3100/healthz
```

页面验证：

- 产品端 `/` 能打开。
- 管理后台 `/admin/` 能打开。
- 登录页能正常加载 Logo、JS、CSS。
- `/admin/` 刷新不会 404。
- `/app`、`/agent` 路由按预期返回页面，`/user` 已下线并应返回 404。

接口验证：

- 管理员登录成功。
- `GET /api/v1/auth/me` 返回当前用户。
- `GET /api/v1/models` 返回模型列表。
- `GET /api/v1/points/account` 返回积分账户。
- `GET /api/v1/generation-tasks` 返回任务列表。
- `GET /api/v1/assets` 返回资产列表。
- `GET /api/v1/admin/overview` 返回后台总览。
- `GET /api/v1/admin/customers` 返回客户列表。
- `GET /api/v1/admin/api/provider-channels` 返回上游渠道。
- `GET /api/v1/admin/billing/overview` 返回计费概览。

业务验证：

- 普通用户能登录产品端。
- 管理员能登录后台。
- 创建生成任务后积分、任务、资产状态一致。
- 后台能查看生成任务、客户、订单、用量。
- API 渠道测试能返回成功或明确错误。
- 审计日志有写入。

### 阶段 6：备份与恢复验证

本地脚本示例：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\backup-postgres.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\restore-postgres.ps1 -InputFile .\backups\xianzhi-YYYYMMDD-HHMMSS.sql
```

生产环境需要调整脚本中的容器名、数据库名、用户和备份目录。上线前至少完成一次：

- 手动备份。
- 从备份恢复到 staging。
- 恢复后应用可启动、可登录、可读取核心列表。

## 7. 回滚方案

### 7.1 应用回滚

- 保留上一个可运行镜像 tag。
- 新版本部署后不要立刻清理旧镜像。
- 如果健康检查或核心接口失败，回滚到上一镜像并保持数据库只读排查。

### 7.2 数据库回滚

- 每次迁移前必须先备份。
- SQL 迁移尽量只增表、增列、增索引，避免生产上线当天做破坏性字段删除。
- 如果必须修改表结构，先在 staging 跑完整恢复和回滚演练。
- 不能通过脚本批量删除生产文件或目录；如需大范围清理，停止自动操作，改为人工确认和执行。

### 7.3 对象存储回滚

- 生产素材、发票、合同、交付文件不要和测试桶混用。
- 删除文件必须逐个确认；不做脚本批量删除。
- 对象存储开启版本控制或定期快照更稳。

## 8. 上线风险与处理

| 风险 | 表现 | 处理 |
| --- | --- | --- |
| 演示配置进入生产 | 默认账号、默认数据库密码、默认支付密钥仍可用 | 上线前强制密钥检查，禁用演示账号。 |
| 静态路由错误 | `/admin/` 刷新 404，`/app` 路由加载错误 | 验证 Go 静态托管和反向代理 fallback。 |
| 数据库迁移遗漏 | 管理后台接口 500，字段或表不存在 | 按迁移文件顺序执行，staging 先跑一遍。 |
| 生成任务与积分不一致 | 任务创建成功但积分未扣或资产未写入 | 验证 PostgreSQL 事务路径和 `generation_tasks/assets/point_transactions`。 |
| 模型供应商不可用 | 生成任务失败或超时 | 配置多供应商、超时、错误提示和后台渠道健康检查。 |
| RBAC 未启用 | 后台接口权限过宽 | 生产设置 `XIANZHI_ENFORCE_RBAC=true`，检查角色权限表。 |
| 备份不可恢复 | 有备份文件但恢复失败 | 上线前做恢复演练，不只检查文件存在。 |
| 对象存储暴露 | 素材、合同、发票可被公网枚举 | 桶私有化，应用受控下载，避免公开目录。 |

## 9. 上线验收清单

- [ ] Git 版本已确认，当前上线 commit/tag 已记录。
- [ ] 未提交改动已确认是否纳入上线。
- [ ] `admin-vue` 构建通过。
- [ ] `frontend-vue` 类型检查通过。
- [ ] `frontend-vue` H5 构建通过。
- [ ] `backend-go` 测试通过。
- [ ] Docker 镜像构建通过。
- [ ] PostgreSQL schema 与 migrations 已执行。
- [ ] 生产密钥已替换，不使用演示密码。
- [ ] 默认演示账号已禁用、改密或限制。
- [ ] HTTPS 和反向代理已配置。
- [ ] `/api/v1/health` 和 `/healthz` 正常。
- [ ] 产品端 `/` 正常访问。
- [ ] 管理后台 `/admin/` 正常访问并可刷新。
- [ ] 普通用户登录正常。
- [ ] 管理员登录正常。
- [ ] 生成任务、积分、资产链路正常。
- [ ] 后台客户、代理、产品、套餐、订单、用量、分润、系统中心可访问。
- [ ] 计费中心概览、订阅、账单等页面可访问。
- [ ] 审计日志写入正常。
- [ ] PostgreSQL 备份成功。
- [ ] 备份已在 staging 恢复验证。
- [ ] 日志、监控、告警已接入。
- [ ] 回滚镜像和数据库备份位置已记录。

## 10. 建议的生产迁移顺序

推荐按“小步上线，先稳住核心链路”的顺序：

1. 先上线产品端、后台壳、登录、健康检查、基础读取接口。
2. 再验证生成任务、积分、资产、模型供应商。
3. 再开放主控后台的客户、代理、套餐、订单、用量、分润。
4. 再启用计费中心、订阅、账单、支付和发票相关操作。
5. 最后启用强 RBAC、审计、备份自动化和运维告警。

如果当前目标是“先可用上线”，第一版生产环境建议只开放：

- 产品端登录与 AI 创作。
- 素材资产查看与下载。
- 管理后台总览。
- 客户、代理、产品、套餐、订单、用量的读取和必要写操作。
- 模型供应商配置和健康测试。
- 基础备份、审计和 RBAC。

计费中心、发票、支付自动化、复杂分润和交付流程可以进入第二阶段，避免第一天上线面过大。
