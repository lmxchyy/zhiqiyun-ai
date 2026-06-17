# 先知 AI 平台

先知 AI 是一站式内容生产与代理商运营平台。项目技术架构统一为 Vue 3 / uni-app 前端 + Go 后端服务。

## 技术栈

- 前端：uni-app、Vue 3、TypeScript；H5、小程序和 App 端按 uni-app 约束复用。
- 后端：Go 1.22+ HTTP 服务，入口为 `backend-go/cmd/api`。
- 数据：当前本地演示数据使用 `data/store.json`，Docker 环境保留 PostgreSQL、Redis、RabbitMQ、MinIO 等基础设施服务。
- Legacy：`frontend` 与 `backend` 目录保留为历史实现参考，不再作为默认运行入口。

## 本地开发

启动 Go API：

```powershell
npm.cmd start
```

默认监听 <http://localhost:3100>，健康检查为 <http://localhost:3100/api/v1/health>。

启动 uni-app Vue 3 前端：

```powershell
npm.cmd run dev:web
```

前端开发服务默认监听 <http://localhost:5173>，并把 `/api` 代理到 Go API。

## 构建与测试

```powershell
npm.cmd test
npm.cmd run build:web
npm.cmd run build:api
```

## Docker 启动

```powershell
npm.cmd run docker:up
npm.cmd run docker:ps
```

Docker 镜像会先构建 uni-app H5 静态资源，再编译 Go API，由 Go 服务托管前端并暴露 `3100` 端口。

## 当前 Go API

- `GET /api/v1/health`
- `GET /api/v1/models`
- `GET /api/v1/generation-tasks`
- `POST /api/v1/generation-tasks`
- `GET /api/v1/assets`
- `DELETE /api/v1/assets/{id}`

## 演示账号

- 管理员：`admin@xianzhi.ai` / `Admin123!`
- 普通用户：`demo@xianzhi.ai` / `Demo123!`
- 一级代理商：`agent1@xianzhi.ai` / `Agent123!`

## 架构说明

更多迁移边界和开发约束见 [docs/architecture-modernization.md](docs/architecture-modernization.md)。
