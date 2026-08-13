# Xianzhi AI Production Deployment

This project uses the following release flow:

Windows local development
-> GitHub/Gitee private repositories
-> domestic server git pull from Gitee or Codeup
-> run `./backup.sh`
-> run `./deploy.sh`
-> Docker Compose rebuilds with `compose.prod.yml`
-> check status and logs
-> run `./rollback.sh <tag-or-commit>` when needed

## 1. Local Git setup on Windows

Current local branch: `main`.

```powershell
cd "E:\code\work\先知AI"
git init
git status
git add .
git commit -m "init production version"
```

If the repository is already initialized, do not re-run `git init`; use `git status` first.

## 2. Configure GitHub and Gitee remotes

```powershell
git remote add origin https://github.com/你的账号/xianzhi-ai.git
git remote add gitee https://gitee.com/你的账号/xianzhi-ai.git
```

Push the current branch. This checkout currently uses `main`:

```powershell
git push origin main
git push gitee main
```

If your local branch is `master`, use:

```powershell
git push origin master
git push gitee master
```

Do not rename `main` to `master`, or `master` to `main`, unless you explicitly decide to do that.

## 3. Daily local development flow

```powershell
git status
git add .
git commit -m "更新说明"
git push origin 当前分支
git push gitee 当前分支
```

Recommended commit groups:

1. Deployment files: `.gitignore`, `.dockerignore`, `.env.example`, `compose.prod.yml`, `deploy.sh`, `backup.sh`, `rollback.sh`, `DEPLOY.md`.
2. Database migrations: `database/migrations`, `database/schema.sql`.
3. Backend code: `backend-go`.
4. Frontend business source: `admin-vue/src`, `apps/user-uni/src`, `packages/*/src`, package files.
5. Static assets, reviewed separately: `admin-vue/public/static`, `apps/user-uni/public`, `apps/user-uni/static`, `vendor/fonts`, `mock-video.mp4`, `dist`.

Do not blindly commit `dist`, large media files, copied demo pages, or vendor fonts without a business review.

## 4. First deployment on a domestic server

Prefer Gitee or Codeup on domestic servers. Avoid untrusted GitHub acceleration services for private repositories.

```bash
cd /opt
git clone https://gitee.com/你的账号/xianzhi-ai.git xianzhi-ai
cd /opt/xianzhi-ai
cp .env.example .env
nano .env
mkdir -p backups/postgres
docker compose -f compose.prod.yml --env-file .env up -d --build
docker compose -f compose.prod.yml --env-file .env ps
docker compose -f compose.prod.yml --env-file .env logs -f --tail=100
```

Fill `.env` with real production values before starting services. Never commit `.env`.

Required production infrastructure values are validated both by Compose and by the Go API startup path:

- `DATABASE_URL`
- `REDIS_URL`
- `RABBITMQ_URL`
- `S3_ENDPOINT`
- `S3_ACCESS_KEY`
- `S3_SECRET_KEY`
- `S3_BUCKET`
- `PAYMENT_CALLBACK_SECRET`

If the server does not preserve executable bits from Git, run:

```bash
chmod +x deploy.sh backup.sh rollback.sh
```

## 5. Later production releases

Always create a PostgreSQL backup before deployment:

```bash
./backup.sh
./deploy.sh
```

`deploy.sh` uses `compose.prod.yml`, reads `.env`, backs up the current `compose.prod.yml` into `backups/compose`, runs `git pull --ff-only`, rebuilds and starts services, prunes unused images, and prints service status and recent logs.

## 6. Rollback

Rollback to a Git tag:

```bash
./rollback.sh v1.0.1
```

Rollback to a commit:

```bash
./rollback.sh abc1234
```

Important: code rollback is not database rollback. If the failed release already ran database schema migrations, confirm that the old code is compatible with the current database. If needed, restore PostgreSQL from `backups/postgres`.

## 7. Backups

Manual backups are written to:

```bash
ls -lh backups/postgres
```

The `postgres-backup` service in `compose.prod.yml` also writes automatic backups to the host directory:

```yaml
./backups/postgres:/backups
```

Automatic backups are compressed and validated before an atomic rename. The
default retention period is 30 days. At most one expired automatic
`xianzhi-*.sql` or `xianzhi-*.sql.gz` file is removed after each successful
backup; manual `db_*.sql` release backups are never selected by this rotation.
Set `BACKUP_RETENTION_DAYS` to adjust the policy. When the backup filesystem
has less than `BACKUP_MIN_FREE_BYTES` available (10 GiB by default), the
automatic backup is skipped and an error is logged instead of consuming the
remaining disk.

If older backups already exist in the Docker volume `postgres-backups`, they will not be automatically migrated to `./backups/postgres`. Copy them manually first, verify the copied files, then clean the old volume only after confirmation.

## 8. Status and logs

```bash
docker compose -f compose.prod.yml --env-file .env ps
docker compose -f compose.prod.yml --env-file .env logs -f --tail=100
```

## 9. Forbidden operation

Do not run:

```bash
docker compose down -v
```

The `-v` flag can delete PostgreSQL, Redis, RabbitMQ, MinIO, and application data volumes.

## 10. Production Compose notes

Production should use `compose.prod.yml`, not `compose.yml`.

The application port defaults to:

```yaml
"${APP_BIND_HOST:-127.0.0.1}:${APP_PORT:-3100}:3100"
```

This is intended for Nginx, Nginx Proxy Manager, Caddy, or another reverse proxy to terminate HTTPS.

PostgreSQL, Redis, RabbitMQ, and MinIO should not be exposed directly to the public internet. Redis must use a password. PostgreSQL, RabbitMQ, and MinIO must use strong non-default passwords in `.env`.

The current production file uses fixed versions for PostgreSQL, Redis, and RabbitMQ. MinIO is pinned by default through `MINIO_IMAGE`; update that value only after testing a new MinIO release.

All production containers use Docker's `local` logging driver with bounded
rotation (`DOCKER_LOG_MAX_SIZE=20m`, `DOCKER_LOG_MAX_FILE=5` by default).
`disk-monitor` reports transitions at 70%, 80%, and 90% usage and becomes unhealthy
at 90% usage or when free space falls below 10 GiB. `deploy.sh` enforces
the same 10 GiB hard floor before it creates a Compose backup or starts a
Docker build. RabbitMQ persists a 2 GiB disk alarm watermark by default; verify
the effective value after deployment with:

```bash
docker compose -f compose.prod.yml --env-file .env.production exec -T rabbitmq \
  rabbitmq-diagnostics -q environment | grep disk_free_limit
docker compose -f compose.prod.yml --env-file .env.production ps disk-monitor
docker compose -f compose.prod.yml --env-file .env.production logs --tail=20 disk-monitor
```

The Docker image does not install OfficeCLI by default. If you explicitly build with `INSTALL_OFFICECLI=true`, set `OFFICECLI_INSTALL_SHA256` to the reviewed installer script checksum; the build fails before execution when the checksum is missing or mismatched.

The migration service currently runs every SQL file in `database/migrations` during Compose startup. Before releases with schema changes, review whether migrations are idempotent. A later improvement is to add a migration history table or a dedicated migration tool.

## 11. GitHub + Gitee deployment strategy

- GitHub is the primary source repository.
- Gitee or Codeup is the domestic server pull repository.
- Domestic servers should pull from Gitee or Codeup to avoid unstable GitHub access.
- Do not expose private repository tokens.
- Do not use untrusted GitHub acceleration sites for private repositories.

Future upgrade path:

GitHub Actions
-> build Docker image
-> push to Aliyun ACR
-> domestic server runs `docker compose pull`
-> domestic server runs `docker compose up -d`
# 飞书企业 Connector

启用飞书机器人前，生产环境还必须注入 `CONNECTOR_SECRET_ENCRYPTION_KEY`（至少 32 字节），并把 `CONNECTOR_CALLBACK_BASE_URL` 设置为飞书可访问的 HTTPS API 根地址。完整检查清单见 [docs/feishu-enterprise-connector.md](docs/feishu-enterprise-connector.md)。
