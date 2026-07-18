# 跨端游客访问与按需登录改造验收记录

更新时间：2026-07-19

## 当前入口与访问策略

- PC 用户端唯一主入口为 `/`。游客直接进入原 PC 工作台，不自动跳转登录页。
- `/login` 仅作为独立登录兜底页；普通游客浏览和创作不会被主动送往该页面。
- 管理后台 `/admin/*`、代理端和运营端继续执行原有身份与权限校验。
- 微信小程序、uni-app 和移动 H5 保持各自页面结构，共享认证核心与后端用户数据，不复用 PC 页面。

公开或游客可见的 PC 模块包括首页、AI 生图、AI 视频、PPT、智能体、无限画布和官方作品案例。钱包、订单、会员、私有作品、企业、知识库等入口在动作发生时统一触发登录。

## 认证与草稿恢复

- 网页认证状态集中在 `admin-vue/src/stores/auth.ts`。
- 通用认证、Pending Action 和 API auth mode 位于 `packages/shared-auth`、`packages/api-client`。
- Pending Action 同时保存在内存和本地缓存，默认 30 分钟失效，消费后立即清除。
- Prompt、模型、比例、分辨率、数量和高级参数进入草稿快照。
- 本地参考图通过 IndexedDB 临时保存，不依赖会在刷新后失效的 blob URL。
- 登录完成后先恢复页面与参数，再由用户二次确认生成；未确认时不调用生成或扣费接口。
- 图片和视频生成使用 `clientRequestId`，非幂等 POST 不会在 401 后自动重放。

## API 鉴权分层

公开展示接口：

- `GET /api/v1/public/home`
- `GET /api/v1/public/cases`
- `GET /api/v1/public/templates`
- `GET /api/v1/public/agents`
- `GET /api/v1/public/models`
- `GET /api/v1/public/pricing`

游客埋点接口：

- `POST /api/v1/public/experience-events`

埋点接口只接受固定事件白名单，并只保留 `action`、`authMethod`、`module`、`platform`、`reason`、`route`。Prompt、手机号、Token、API Key 和任意扩展字段均由服务端丢弃。

用户资料、私有作品、钱包、订单、充值和生成接口仍要求有效身份。公开首页不会因为这些私有接口返回 401 而跳转或白屏。

## 账号统一与冲突处理

- 手机号、微信网页扫码和微信小程序身份均以服务端 `user_id` 为数据归属。
- 微信身份匹配优先使用 UnionID/已绑定 OpenID；手机号绑定到已有用户时不会再次创建用户。
- 检测到手机号与微信分别属于两个用户时不自动粗暴合并，而是创建账号合并工单。
- 管理后台提供合并工单查询、预览、状态流转和执行接口；执行路径保留审计记录。

本次游客访问改造没有重写用户主表，也没有放开任何私有接口。账号冲突能力使用现有兼容投影和合并工单结构。

## 自动化验证

认证核心：

```text
node --test tests/auth-guest-flow.test.mjs
8 passed
```

覆盖 Pending Action 过期与单次消费、安全跳转、单例登录门、API auth mode、多个 401 共用一次刷新、非幂等 POST 不自动重试以及退出撤销 Refresh Token。

PC 网页端：

```text
npm run test:pc-web:smoke
5 passed
```

覆盖：

1. `/` 游客直接进入原 PC 工作台且不请求私有数据。
2. 游客填写生图 Prompt 后取消登录，内容保留且没有生成请求。
3. 游客作品中心只加载官方案例，点击“我的作品”时才登录。
4. 登录后恢复 Prompt，在二次确认前不提交；确认后只提交一次并携带 `clientRequestId`。
5. 已登录用户退出后回到 `/` 的游客工作台，不跳到登录页或旧路径。

其他已通过验证：

- Go 全量测试 `go test ./...`
- TypeScript packages 类型检查
- PC Vue 构建
- uni-app 类型检查
- 微信小程序构建
- Docker 3100 服务健康检查
- 公开接口 200、游客埋点 204、非法埋点 400、未登录私有作品接口 401

## 仍需人工环境验收

- 微信开发者工具中的授权登录、TabBar 切换和真机手机号授权。
- 微信真机从参考图选择到登录恢复的完整链路。
- App/uni-app 原生运行环境中的页面栈和系统登录授权。
- 正式微信开放平台 AppID 下的网页扫码、二维码过期和首次绑定手机号。
- 正式短信供应商、生产限流配置及异常网络下的验证码体验。

以上项目需要真实平台凭据或设备，不能由本地自动化结果替代。
