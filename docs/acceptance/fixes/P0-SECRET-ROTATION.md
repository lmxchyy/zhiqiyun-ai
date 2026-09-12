# P0 Secret Rotation

## 扫描范围

- Git tracked files 与 bounded Git history pattern scan
- `.env` / `.env.*` 文件是否被跟踪
- `.env.example`
- `compose.yml` / `compose.prod.yml`
- CI / config / acceptance 文档

## 当前分类

```text
SECRET_STATUS=ROTATION_REQUIRED
```

### A. 真实生产 Secret

当前工作区存在未被 Git 跟踪的本地环境文件，其中包含看起来可用于 Provider/微信等外部服务的运行时凭据。由于不能在报告中输出 Secret 值，也不能把本地凭据当作生产轮换证据，暂不判断具体凭据的当前有效性。

仓库既有验收材料同时记录历史凭据可能存在且尚未完成轮换。

### B. 测试 / 本地 Secret

Compose 和本地测试环境包含 MinIO、数据库、Redis、RabbitMQ 等开发凭据。它们只能用于本地隔离环境，不得复用到生产。

### C. 示例 Placeholder

`.env.example` 中的 Secret 字段为示例/占位配置，未发现应提交的真实 Secret 文件。

## 已完成

- 当前 tracked source bounded scan 未发现新的私密文件；唯一 pattern hit 为 `.env.example`。
- `.env` 与 `.env.production` 未被 Git 跟踪。
- 未修改或删除用户本地环境文件。
- 未把任何 Secret 值写入仓库、报告或日志。

## BLOCKED_REASON

生产 Secret/IAM/Provider/微信/对象存储凭据轮换需要外部平台权限，当前本地代理无法安全代替执行。需要安全负责人完成：

1. 重新生成生产 Provider、微信支付、对象存储、数据库和 Connector 密钥。
2. 更新生产 Secret 注入系统。
3. 证明旧凭据已失效。
4. 完成 GitHub secret history / repository secret 检查。
5. 提供不包含 Secret 值的轮换记录和时间戳。

## 状态

`BLOCKED_EXTERNAL_ROTATION`
