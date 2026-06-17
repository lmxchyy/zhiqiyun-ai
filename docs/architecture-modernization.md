# 先知 AI 统一技术架构

## 当前架构基线

- 前端统一采用 uni-app + Vue 3 组件体系，H5、小程序和 App 端都按 uni-app 工程约束组织。
- 后端统一采用 Go HTTP 服务，默认入口为 `backend-go/cmd/api`。
- 默认本地前端目录为 `frontend-vue`，现阶段是 uni-app Vue 3 工程，H5 输出目录为 `frontend-vue/dist/build/h5`。
- 旧 `frontend` 和 `backend` 目录保留为 legacy 参考，不再作为默认启动、构建或 Docker 服务入口。

## 运行入口

| 场景 | 命令 | 说明 |
| --- | --- | --- |
| Go API | `npm.cmd start` 或 `npm.cmd run dev:api` | 启动 `backend-go/cmd/api`，默认监听 `3100` |
| uni-app Vue 3 前端 | `npm.cmd run dev:web` 或 `npm.cmd run dev:uni` | 启动 `frontend-vue`，开发代理指向 Go API |
| Go 测试 | `npm.cmd test` | 执行 `go test ./...` |
| 前端构建 | `npm.cmd run build:web` 或 `npm.cmd run build:uni` | 执行 uni-app H5 构建 |
| Docker | `npm.cmd run docker:up` | 构建 uni-app H5 静态资源，编译 Go API，并由 Go 服务托管前端 |

## 后端 API 边界

Go 服务当前承接以下 `/api/v1` 端点：

- `GET /api/v1/health`
- `GET /api/v1/models`
- `GET /api/v1/generation-tasks`
- `POST /api/v1/generation-tasks`
- `GET /api/v1/assets`
- `DELETE /api/v1/assets/{id}`

这些接口先以 `data/store.json` 作为本地演示数据源，保持现有 AI 创作页可运行。后续迁移业务模块时，应在 Go 服务内继续扩展 API、队列、模型网关和资产服务，而不是把新能力加回 Node 入口。

## 前端准则

- 业务接口统一使用 `/api/v1`，开发代理指向 Go 服务。
- 新页面优先使用 Vue 3 Composition API，并落在 `pages.json` 管理的 uni-app 页面中。
- 组件模板优先使用 `view`、`text`、`image`、`scroll-view`、`picker` 等 uni-app 跨端组件，不引入只适用于传统 DOM 页面的业务依赖。
- 旧 `frontend` 目录只用于对照历史交互，不作为新功能落点。

## AI 创作页交互准则

- 画布区域负责查看历史生成结果，不出现整页滚动条。
- 底部输入区固定，不随画布拖动或滚动。
- 生成记录按时间从上到下排列，最新结果在最下面。
- 每轮记录左侧展示图片，右侧展示轮次、类型、状态、时间、提示词、复用配置、删除。
- 模型连接成功时显示绿色 ONLINE。
