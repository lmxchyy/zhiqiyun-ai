# 飞书企业自建应用 Connector

## 能力边界

当前支持飞书文本消息触发生图、上一张图片改图、文生视频、图生视频、一句话生成 PPT、最近任务查询、文本/图片/文件/卡片回发、企业成员映射、分能力权限与额度、企业积分结算、作品中心保存、消息/任务幂等和投递失败后后台重发。钉钉、企业微信、通讯录全量同步和应用商店版飞书应用仍不在本阶段范围内。视频/PPT 的完整链路见 [飞书视频/PPT 接入交付说明](feishu-video-ppt-connector.md)。

## 架构与复用关系

飞书适配器实现统一 `PlatformConnector`：`VerifyEvent`、`ParseEvent`、`SendText`、`SendImage`、`SendFile` 和 `SendCard`。适配器只输出平台无关的 `IncomingMessage`、`MessageTarget` 与 `OutgoingMessage`。

完整链路：

1. `POST /api/open/connectors/feishu/events/:connectorKey` 根据随机 `connectorKey` 读取连接器并确定 `enterprise_id`。
2. 校验 Verification Token；配置 Encrypt Key 时校验签名并解密 AES-256-CBC 事件。
3. 通过 `platform + external_message_id` 幂等写入 `connector_messages`。
4. 投递 Redis 可靠列表队列后立即返回 200；开发环境没有 Redis 时使用有界本地队列。
5. Worker 加载或创建 `connector_user_bindings`。首次出现的飞书用户会创建隔离在当前企业和默认组织下的内部影子用户/企业成员，并授予 `ENTERPRISE_MEMBER` 角色。
6. 检查连接器状态、企业/订阅/组织/成员状态、`enterprise.ai.use`、成员开关、日额度和群聊规则。
7. `RuleIntentRouter` 识别 `image.generate`、`image.edit`、`video.generate`、`video.image_to_video`、`ppt.generate`、`task.query`、`help` 和 `unknown`。
8. `CapabilityRouter` 选择统一 `CapabilityHandler`，执行权限、参数、单次/每日/每月额度和费用预估校验。
9. 以 `feishu:<external_message_id>` 作为既有业务任务的 `client_request_id`，复用现有生图、视频和 PPT 服务。
10. 原链路负责模型路由、Reserve、Provider/PPT 引擎、私有文件中心、`xz_assets`、Capture/Release、模型用量和审计。
11. Worker 直接上传小文件；大文件或上传失败时使用现有短期签名地址发送完成卡片。生成成功但投递失败只标记 `delivery_failed`。

## 数据库迁移

迁移文件为 `database/migrations/053-enterprise-connectors-feishu.sql`，新增：

- `enterprise_connectors`：每个企业/平台一个连接器，密钥使用 AES-GCM 密文保存。
- `connector_user_bindings`：外部用户到内部企业成员的映射与渠道权限。
- `connector_messages`：入站/出站消息与 `external_message_id` 幂等记录。
- `connector_ai_tasks`：Connector 任务状态、原指令、意图、生成任务 ID 和费用。

迁移同时增加 `enterprise.connector.read`、`enterprise.connector.manage`，默认授予 `ENTERPRISE_ADMIN` 和 `AI_ADMIN`。Compose 的 migration 服务会按文件名顺序自动执行，也可以单独运行：

```powershell
docker compose run --rm migrate
```

## 管理 API

全部管理接口使用现有登录会话、当前企业上下文和服务端 RBAC：

- `GET /api/v1/enterprise/connectors`
- `GET|POST|PUT /api/v1/enterprise/connectors/feishu`
- `POST /api/v1/enterprise/connectors/feishu/test`
- `POST /api/v1/enterprise/connectors/feishu/enable`
- `POST /api/v1/enterprise/connectors/feishu/disable`
- `GET /api/v1/enterprise/connectors/feishu/users`
- `PUT /api/v1/enterprise/connectors/feishu/users/:id`
- `GET /api/v1/enterprise/connectors/feishu/logs`
- `GET /api/v1/enterprise/connectors/feishu/tasks`
- `POST /api/v1/enterprise/connectors/feishu/tasks/:taskId/retry-delivery`

密钥字段从不回显。响应只返回 `secretsConfigured.appSecret`、`verificationToken`、`encryptKey`；更新时密钥留空表示保持原值。

## 飞书开发者后台配置

1. 打开飞书开放平台，创建「企业自建应用」。
2. 在应用能力中添加「机器人」。
3. 复制 App ID、App Secret；在事件与回调安全配置中获取 Verification Token，可选配置 Encrypt Key。
4. 申请并由企业管理员批准机器人接收消息、以应用身份发送消息、上传图片和上传文件所需权限。
5. 在知启云「我的 → 企业中心 → 企业设置 → 企业连接 · 飞书」保存 App ID、App Secret、Verification Token、Encrypt Key。
6. 复制页面生成的 HTTPS 事件回调地址到飞书事件订阅。
7. 订阅 `im.message.receive_v1`（接收消息）事件。URL 校验成功后，在知启云点击「测试连接」和「启用」。测试成功会读取并保存当前机器人的 `open_id`，用于精确判断群聊是否真正 @ 了本机器人。
8. 创建并发布飞书应用版本，把应用可用范围设置为需要使用机器人的成员。
9. 在单聊发送「生成 iPhone 17 的电商图」「生成 10 秒产品视频」「用刚才的图片生成 5 秒视频」或「生成一份 10 页招商 PPT」；群聊按企业设置先 @机器人。

常见错误：URL 验证失败时检查公网 HTTPS、Verification Token 和 Encrypt Key；测试连接失败时检查 App ID/Secret；收不到消息时检查应用版本、可用范围和事件订阅；提示算力不足时在企业算力账户充值。

## 配置与安全

```dotenv
FEISHU_HTTP_TIMEOUT_SECONDS=10
FEISHU_TOKEN_CACHE_PREFIX=xianzhi:connector:feishu:token:
FEISHU_API_BASE_URL=https://open.feishu.cn/open-apis
CONNECTOR_CALLBACK_BASE_URL=https://api.example.com
CONNECTOR_SECRET_ENCRYPTION_KEY=<至少32字节的Secret>
CONNECTOR_QUEUE_PREFIX=xianzhi:connector:jobs:
```

生产密钥必须由 Secret 管理器或环境变量注入。事件接口限制 1 MiB 请求体，并按 connector/IP 限流；原始 payload 入库前过滤 secret、token、encrypt 和 authorization 字段；日志只记录脱敏 App ID、连接器/任务 ID 和错误类别。

## 本地启动与测试

```powershell
docker info
docker compose up -d postgres redis minio migrate
docker compose up -d --build xianzhi-ai
npm.cmd --prefix apps/user-uni run typecheck
npm.cmd --prefix apps/user-uni run build:mp-weixin
docker run --rm -v "${PWD}/backend-go:/app" -w /app golang:1.25-alpine sh -lc "/usr/local/go/bin/go test ./internal/connector/... ./internal/httpserver"
```

如需同时执行 PostgreSQL 回调幂等、余额回滚和 Redis 队列恢复集成测试，可在 Compose 依赖启动后运行：

```powershell
docker run --rm `
  -e XIANZHI_P0_TEST_DATABASE_URL=postgresql://xianzhi:xianzhi@host.docker.internal:54321/xianzhi `
  -e XIANZHI_CONNECTOR_TEST_REDIS_URL=redis://host.docker.internal:63791/0 `
  -v "${PWD}/backend-go:/app" -w /app golang:1.25-alpine `
  sh -lc "/usr/local/go/bin/go test ./internal/connector/... ./internal/httpserver -count=1"
```

本地飞书回调必须通过 HTTPS 隧道暴露端口 3100，并把隧道根地址写入 `CONNECTOR_CALLBACK_BASE_URL`。不要把真实飞书密钥写进 `.env.example`、代码、Compose YAML 或小程序包。

## 生产部署检查

1. `TARGET_PLATFORM` 已设置，且 `docker compose --env-file .env.production -f compose.prod.yml config --quiet` 通过。
2. PostgreSQL 已执行 `053`、`062`，Redis 可用，队列前缀在同一环境唯一。
3. `CONNECTOR_SECRET_ENCRYPTION_KEY` 已安全注入且有备份；更换密钥前必须做密文轮换。
4. `CONNECTOR_CALLBACK_BASE_URL` 是公网 HTTPS，反向代理允许 1 MiB 事件体并保留飞书签名头。
5. 模型 Provider 和对象存储可从 API 容器访问；飞书开放接口可从生产网络访问。
6. 使用测试企业完成 URL 验证、重复 message_id、余额不足、成员禁用、群聊 @、生图/改图/视频/PPT、失败释放和 delivery_failed 后重投验收。

## 已知限制与下一阶段

- 当前成员自动映射使用影子内部用户；下一阶段可在获得通讯录权限后按手机号/邮箱受控合并，禁止猜测合并。
- 当前队列是 Redis 可靠列表，任务列表与人工重投已可用，但尚未提供通用死信队列管理页。
- 卡片 action 数据已为 `task.view`、`task.retry_delivery`、`video.generate_similar`、`ppt.regenerate`、`asset.download` 等预留；当前仅后台受 RBAC 保护的重新投递接口可执行，未开放不安全的飞书卡片回调入口。
- 下一阶段建议增加飞书通讯录增量同步、任务取消、进度主动更新和通用死信队列，再复用同一 Connector 接入钉钉和企业微信。
