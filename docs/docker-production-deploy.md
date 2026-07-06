# 先知 AI Docker 生产部署流程

这份流程用于把当前仓库部署到一台 Linux 云服务器。当前推荐路径是单机 Docker Compose：`xianzhi-ai` 应用容器托管产品端 H5、`/admin/` 后台和 `/api/v1/*`，PostgreSQL、Redis、RabbitMQ、MinIO 作为内部服务运行。

## 1. 需要你提供的信息

发服务器信息时建议包含这些项：

- SSH 地址、端口、登录用户、密码或私钥。
- 服务器系统版本，以及是否已安装 Docker 和 Docker Compose。
- 域名和 DNS 解析情况。没有域名也可以先用 `IP:3100` 临时验收。
- 是否需要我配置 HTTPS。推荐有域名时使用 Caddy 或 Nginx。
- 模型供应商地址、API Key、图片模型、视频模型、PPT 文本模型。
- 支付回调密钥、微信支付/支付宝密钥。如果暂不上支付，可以先给占位强密钥。
- 是否迁移本地演示数据，还是生产环境从空库开始。

不要把生产密钥提交到仓库。我会在服务器上创建 `.env.production`。

## 2. 本地发布前检查

当前已验证：

- `docker compose -f compose.yml config --quiet` 通过。
- 本机没有 Go 工具链，`npm.cmd test` 不能直接跑。
- 使用 Docker Go 镜像执行 `go test ./...` 通过。
- 本地 Docker 服务健康，`/api/v1/health` 返回 `{"service":"xianzhi-ai-go-gin","status":"ok"}`。
- 本地 `/` 和 `/admin/` 返回 `200`。

正式上线前还会在目标服务器重新跑一次 Compose 配置校验、构建、启动和健康检查。

## 3. 服务器准备

目标服务器建议至少准备：

- Ubuntu 22.04/24.04 或 Debian 12。
- Docker Engine 和 Docker Compose v2。
- 2 核 CPU、4 GB 内存起步。PPT、图片、视频生成链路更建议 4 核 8 GB。
- 40 GB 以上磁盘，生产素材和备份较多时单独挂载数据盘。
- 防火墙只开放 `22`、`80`、`443`。临时 IP 验收时才开放 `3100`。

基础设施端口 PostgreSQL、Redis、RabbitMQ、MinIO 不对公网开放。

## 4. 文件与配置

生产部署使用这两个文件：

- `compose.prod.yml`
- `.env.production`

先在服务器上复制模板：

```bash
cp .env.production.example .env.production
```

然后替换所有 `change-me` 值。密码建议使用长随机字符串。如果密码中包含 `@`、`:`、`/`、`#` 等字符，`DATABASE_URL`、`REDIS_URL`、`RABBITMQ_URL` 中要做 URL 编码。

生产环境推荐保持：

```env
APP_BIND_HOST=127.0.0.1
APP_PORT=3100
XIANZHI_ENFORCE_RBAC=true
```

这样公网流量只经过 HTTPS 反向代理进入应用。

## 5. 构建与启动

在服务器项目目录执行：

```bash
docker compose --env-file .env.production -f compose.prod.yml config --quiet
docker compose --env-file .env.production -f compose.prod.yml build --progress plain
docker compose --env-file .env.production -f compose.prod.yml up -d
docker compose --env-file .env.production -f compose.prod.yml ps
```

第一次启动会自动执行 `database/migrations/*.sql`。`postgres-backup` 会写入 `postgres-backups` 数据卷，默认每 24 小时备份一次，不自动删除历史备份。

## 6. HTTPS 反向代理

有域名时推荐 Caddy，配置最短：

```caddyfile
your-domain.example.com {
  reverse_proxy 127.0.0.1:3100
}
```

如果使用 Nginx，反向代理目标同样是 `http://127.0.0.1:3100`，并确保 `/admin/`、`/app/*`、`/api/v1/*` 都转发到同一个应用端口。

## 7. 上线验收

基础验收：

```bash
curl -i http://127.0.0.1:3100/api/v1/health
curl -i http://127.0.0.1:3100/
curl -i http://127.0.0.1:3100/admin/
```

当前 Go 服务还没有注册 Prometheus 文本指标路由，`/metrics` 会按前端静态资源 fallback 返回页面。上线验收先以健康检查、容器状态、日志和业务接口为准；后续接入指标时再补 Prometheus scrape 路由。

容器验收：

```bash
docker compose --env-file .env.production -f compose.prod.yml ps
docker compose --env-file .env.production -f compose.prod.yml exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\dt"
docker compose --env-file .env.production -f compose.prod.yml exec redis redis-cli -a "$REDIS_PASSWORD" ping
docker compose --env-file .env.production -f compose.prod.yml exec rabbitmq rabbitmq-diagnostics -q ping
docker compose --env-file .env.production -f compose.prod.yml exec postgres-backup ls -lh /backups
```

业务验收：

- 产品端 `/` 能打开。
- 后台 `/admin/` 能打开且刷新不 404。
- 管理员和普通用户能登录。
- `/api/v1/models` 能返回模型列表。
- 图片、视频、PPT 至少各跑一次最小任务。
- 积分、任务、资产、用量记录保持一致。
- 后台客户、订单、用量、渠道、模型网关页面能访问。

## 8. 回滚与备份

上线前先确认已有备份：

```bash
docker compose --env-file .env.production -f compose.prod.yml exec postgres-backup ls -lh /backups
```

应用回滚保留上一版镜像 tag，不立即清理旧镜像。数据库回滚只从明确的 SQL 备份恢复。不要批量删除生产文件、目录或对象存储内容；如确实需要大范围清理，停止自动操作，改成人工逐项确认。

## 9. 我拿到服务器凭证后的执行顺序

1. 登录服务器，确认系统、磁盘、内存、防火墙、Docker 状态。
2. 上传或拉取当前项目代码。
3. 创建 `.env.production`，填入生产密钥和模型配置。
4. 校验 Compose 配置。
5. 构建镜像并启动服务。
6. 配置反向代理和 HTTPS。
7. 执行健康检查、容器检查、页面检查、登录检查和核心业务检查。
8. 给你回传访问地址、容器状态、健康接口结果、备份位置和后续运维命令。
