# 飞书 Connector API

所有 `/api/v1/enterprise/*` 接口使用当前登录会话、当前企业上下文和服务端 RBAC。公开事件入口只接受由服务端配置生成的 `connectorKey`，不接受消息体传入企业 ID。

## 企业配置

- `GET /api/v1/enterprise/connectors/feishu`：读取脱敏配置。
- `POST|PUT /api/v1/enterprise/connectors/feishu`：创建或更新连接器。Secret 留空表示不修改；响应永不回显明文/密文。
- `POST /api/v1/enterprise/connectors/feishu/test`：测试飞书租户 Token 和机器人身份。
- `POST /api/v1/enterprise/connectors/feishu/enable|disable`：启停连接器。
- `GET /api/v1/enterprise/connectors/feishu/users`：成员绑定和权限列表。
- `PUT /api/v1/enterprise/connectors/feishu/users/:id`：更新绑定、成员能力和额度。

## 任务

- `GET /api/v1/enterprise/connectors/feishu/tasks?capability=video.generate&status=delivery_failed&limit=100`
- `POST /api/v1/enterprise/connectors/feishu/tasks/:taskId/retry-delivery`

重投接口需要 `enterprise.connector.manage`，同时校验任务属于当前企业和当前 Connector、作品属于任务绑定的内部用户。它只重新签发 PRIVATE 文件的短期地址并发送卡片，不调用任何生成服务或账单接口。

## 事件

- `POST /api/open/connectors/feishu/events/:connectorKey`

服务端验证 Verification Token/Encrypt Key、限制请求体与频率、清理敏感 payload 后，以 `(platform, external_message_id)` 幂等入队并立即响应。异步 Worker 负责实际能力执行。
