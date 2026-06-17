# Docker 运行说明

## 启动完整环境

```powershell
docker compose up -d --build
docker compose ps
```

## 服务入口

| 服务 | 地址 |
|---|---|
| 先知 AI 工作台 | <http://localhost:3100> |
| 生成任务 Worker | Docker 内部服务，无公开端口 |
| RabbitMQ 管理台 | <http://localhost:15672> |
| MinIO 控制台 | <http://localhost:9001> |
| PostgreSQL | `localhost:54321` |
| Redis | `localhost:63791` |

`migrate` 容器会在应用启动前自动执行 `database/migrations/*.sql`，成功后正常退出。
`postgres-backup` 容器启动后立即创建一次 PostgreSQL SQL 备份，之后默认每 24 小时创建一次。备份保存在 `postgres-backups` 数据卷中，不自动删除历史备份。

开发环境默认账号均使用 `xianzhi`。MinIO 密码为 `xianzhi-minio`，其他基础设施密码为 `xianzhi`。生产部署前必须通过环境变量或 Secret 管理系统替换默认密码。

## 模型供应商

通用 HTTP 供应商继续使用 `MODEL_PROVIDER_URL`、`MODEL_PROVIDER_API_KEY` 和 `MODEL_PROVIDERS_JSON`。

OpenAI 兼容图像和视频供应商可直接启用：

```powershell
$env:MODEL_PROVIDER_KIND="openai"
$env:OPENAI_BASE_URL="https://your-openai-compatible-host"
$env:OPENAI_API_KEY="<your-openai-api-key>"
$env:MODEL_PROVIDER_IMAGE_MODEL="<image-model-code>"
docker compose up -d --build
```

Worker 会将文本生图转换为 OpenAI Images API 调用，将文本生视频转换为 Sora 视频任务创建、轮询和内容下载。

## 验证

```powershell
docker compose ps
docker compose exec postgres psql -U xianzhi -d xianzhi -c "\dt"
docker compose exec redis redis-cli ping
docker compose exec rabbitmq rabbitmq-diagnostics -q ping
docker compose exec xianzhi-ai node -e "fetch('http://127.0.0.1:3100/api/v1/health').then(r=>r.text()).then(console.log)"
docker compose exec xianzhi-ai node -e "fetch('http://127.0.0.1:3100/metrics').then(r=>r.text()).then(console.log)"
docker compose exec postgres-backup ls -lh /backups
```

## 生产监控

应用暴露 Prometheus 文本指标：

```powershell
curl.exe http://localhost:3100/metrics
```

生产环境建议设置 `METRICS_TOKEN`，抓取时使用：

```powershell
curl.exe -H "Authorization: Bearer <token>" http://localhost:3100/metrics
```

## PostgreSQL 备份与恢复

查看备份文件：

```powershell
docker compose exec postgres-backup ls -lh /backups
```

恢复指定备份前，应先停止业务写入，并将 `<backup-file.sql>` 替换为实际文件名：

```powershell
docker compose exec postgres-backup sh -c "psql -h postgres -U xianzhi -d xianzhi -v ON_ERROR_STOP=1 < /backups/<backup-file.sql>"
```

生产环境应将备份卷同步到独立存储，并定期进行恢复演练。本项目不会自动删除历史备份。

## 停止

```powershell
docker compose down
```

不要使用 `docker compose down -v`，除非明确需要删除全部持久化数据。
