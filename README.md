# 先知 AI 平台

先知 AI 是一站式内容生产与代理商运营平台。当前工程正在按 PC 管理后台、移动端和 Go 服务三层架构演进。

## 技术栈

- PC 管理后台：`admin-vue`，Vue 3、Vite、Element Plus、Pinia、Axios，构建产物由 Go 服务托管在 `/admin/`。
- 移动端：`frontend-vue`，uni-app、Vue 3、Pinia、TypeScript；请求层使用 `uni.request` 封装，以兼容 H5、小程序和 App。
- 后端：`backend-go`，Go 1.25、Gin、PostgreSQL、Redis；入口为 `backend-go/cmd/api`。
- 数据：Docker 环境使用 PostgreSQL、Redis、RabbitMQ、MinIO。当前业务状态优先写入 PostgreSQL `platform_state`，本地无数据库时 fallback 到 `data/store.json`，后续继续拆分到领域表。
- Legacy：旧 `frontend`、`backend`、`admin-web` 目录已移除；当前开发入口统一为 `frontend-vue`、`admin-vue`、`backend-go`。

## 本地开发

启动 Go API：

```powershell
npm.cmd start
```

默认监听 <http://localhost:3100>，健康检查为 <http://localhost:3100/api/v1/health>。

启动 PC 管理后台：

```powershell
npm.cmd run dev:admin
```

后台开发服务默认监听 <http://localhost:5174>，并把 `/api` 代理到 Go API。

启动 uni-app 移动端/H5：

```powershell
npm.cmd run dev:uni
```

移动端开发服务默认监听 <http://localhost:5173>，并把 `/api` 代理到 Go API。

## 构建与测试

```powershell
npm.cmd run build:admin
npm.cmd run build:uni
npm.cmd run build:mp-weixin
npm.cmd run build:app-plus
npm.cmd test
```

移动端产物路径：H5 为 rontend-vue/dist/build/h5，微信小程序为 rontend-vue/dist/build/mp-weixin，App Plus 为 rontend-vue/dist/build/app。App 真机和商店包仍需 HBuilderX/原生打包链。

本机没有 Go 工具链时，可使用 Docker Go 镜像执行后端测试。

## Docker 启动

```powershell
npm.cmd run docker:up
npm.cmd run docker:ps
```

Docker 镜像会构建 PC 管理后台、uni-app H5 静态资源和 Go API，由 Go 服务托管 `/admin/`、移动端 H5 和 `/api/v1/*`。

## 当前 Go API

- `GET /api/v1/health`
- `POST /api/v1/auth/login`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout`
- `GET /api/v1/channel/me`
- `GET /api/v1/models`
- `GET /api/v1/generation-tasks`
- `POST /api/v1/generation-tasks`
- `GET /api/v1/assets`
- `DELETE /api/v1/assets/{id}`
- `GET /api/v1/admin/*`

## 演示账号

- 管理员：`admin@xianzhi.ai` / `Admin123!`
- 普通用户：`demo@xianzhi.ai` / `Demo123!`
- 一级代理商：`agent1@xianzhi.ai` / `Agent123!`

## 架构说明

更多迁移边界和开发约束见 [docs/architecture-modernization.md](docs/architecture-modernization.md)。


## 规范化生产架构状态

当前后端在配置 PostgreSQL 时会使用 `postgresStore` 作为主数据仓库，核心业务不再通过整包 `platform_state` JSON 读写：

- 用户：`xz_users`
- 套餐：`xz_plans`
- 积分账户：`xz_point_accounts`
- 订单：`xz_orders`
- 渠道代理：`xz_channel_agents`
- 佣金：`xz_commissions`
- 提现：`xz_withdrawals`
- 生成任务：`xz_generation_tasks`
- 素材资产：`xz_assets`

`platform_state` 仍保留为历史兼容表，但运行服务已优先走 PostgreSQL 业务表。生成任务创建会在同一事务内完成积分锁定扣减、任务写入、素材写入和审计记录；订单、佣金、提现审核也通过事务更新。

### RBAC 与审计

- 审计日志表：`xz_audit_logs`
- 操作日志表：`xz_operation_logs`
- 角色权限表：`xz_role_permissions`
- 备份记录表：`xz_backup_runs`

生产环境可设置 `XIANZHI_ENFORCE_RBAC=true` 启用后台接口强制 RBAC。未开启时，系统仍记录变更请求审计，但不会阻断当前演示环境的后台操作。

### 固定接口契约

移动端与管理后台以 `/api/v1` 为稳定契约前缀：

- `GET /api/v1/points/account`
- `GET /api/v1/generation-tasks`
- `POST /api/v1/generation-tasks`
- `GET /api/v1/assets`
- `DELETE /api/v1/assets/:id`
- `GET /api/v1/admin/customers`
- `POST /api/v1/admin/customers`
- `GET /api/v1/admin/plans`
- `GET /api/v1/admin/orders`
- `POST /api/v1/admin/orders`
- `GET /api/v1/admin/commissions`

移动端继续使用 `uni.request` 封装，以兼容 H5、微信小程序和 App Plus；PC 管理后台使用 Axios。

### 数据迁移与备份恢复

迁移文件：

- `database/migrations/021-runtime-projections.sql`
- `database/migrations/022-production-governance.sql`

备份命令：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\backup-postgres.ps1
```

恢复命令：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\restore-postgres.ps1 -InputFile .\backups\xianzhi-YYYYMMDD-HHMMSS.sql
```
