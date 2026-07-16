# 企业 Connector 统一扫码授权

企业管理员可访问 `/app/enterprise/connectors` 生成 5 分钟有效的一次性二维码。统一二维码先打开平台选择页；平台卡片也可以直接生成专属二维码。

## 当前能力

- 飞书：复用企业已加密保存的自建应用凭据，通过 OAuth 校验扫码用户并绑定当前知启云账号。
- 微信：配置微信开放平台“网站应用”后，通过 `snsapi_login` 完成个人微信身份绑定。
- 企业微信、钉钉：后台如实展示第三方应用套件前置条件。取得并配置 ISV/第三方企业应用资质后，再接授权回调，当前不会返回模拟成功。

统一二维码并不意味着四个平台共用一套 OAuth。二维码只负责承载知启云的一次性票据，用户选择平台后，服务端再跳转到相应平台的官方授权页。

## 安全模型

- 会话创建必须具备 `enterprise.connector.manage` 权限。
- 企业 ID、组织 ID 和操作用户只从已认证的服务端上下文写入会话，不接受请求体或扫码回调传入租户标识。
- 二维码使用 256 位随机票据，数据库只保存 SHA-256 摘要；有效期 5 分钟，只允许一次成功或失败消费。
- OAuth access token 仅用于本次用户信息查询，不保存到数据库，不写日志，不返回前端。
- 飞书 App Secret 继续使用 Connector Secret Cipher 加密入库。

## 环境变量

```dotenv
CONNECTOR_CALLBACK_BASE_URL=https://api.example.com
FEISHU_API_BASE_URL=https://open.feishu.cn/open-apis
FEISHU_ACCOUNTS_BASE_URL=https://accounts.feishu.cn

WECHAT_OPEN_APP_ID=
WECHAT_OPEN_APP_SECRET=
WECHAT_OPEN_API_BASE_URL=https://api.weixin.qq.com
WECHAT_OPEN_AUTHORIZE_BASE_URL=https://open.weixin.qq.com
```

`CONNECTOR_CALLBACK_BASE_URL` 必须是手机和平台 OAuth 服务均可访问的 HTTPS 根地址。飞书应用需要登记：

```text
https://api.example.com/api/open/connectors/oauth/feishu/callback
```

微信开放平台网站应用需要登记：

```text
https://api.example.com/api/open/connectors/oauth/wechat/callback
```

## API

- `GET /api/v1/enterprise/connector-authorizations/platforms`
- `POST /api/v1/enterprise/connector-authorizations`
- `GET /api/v1/enterprise/connector-authorizations/:id`
- `POST /api/v1/enterprise/connector-authorizations/:id/cancel`
- `GET /api/open/connectors/authorize/:ticket`
- `GET /api/open/connectors/authorize/:ticket/start?platform=feishu`
- `GET /api/open/connectors/oauth/feishu/callback`
- `GET /api/open/connectors/oauth/wechat/callback`
