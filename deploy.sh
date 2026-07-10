#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
GIT_BRANCH="${GIT_BRANCH:-}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

log() {
  printf '[deploy] %s\n' "$*"
}

fail() {
  printf '[deploy] ERROR: %s\n' "$*" >&2
  exit 1
}

command -v git >/dev/null 2>&1 || fail "git is not installed."
command -v docker >/dev/null 2>&1 || fail "Docker is not installed."
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is not available."

# 自动识别生产服务器 CPU 架构，并传给 compose.prod.yml。
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)
    export TARGET_PLATFORM="linux/amd64"
    ;;
  aarch64|arm64)
    export TARGET_PLATFORM="linux/arm64"
    ;;
  armv7l|armv7)
    export TARGET_PLATFORM="linux/arm/v7"
    ;;
  *)
    fail "Unsupported CPU architecture: $ARCH"
    ;;
esac

log "Project directory: $SCRIPT_DIR"
log "Compose file: $COMPOSE_FILE"
log "Env file: $ENV_FILE"
log "CPU architecture: $ARCH"
log "Docker platform: $TARGET_PLATFORM"

[ -d .git ] || fail "$SCRIPT_DIR is not a Git repository. Clone the GitHub repository first."
[ -f "$ENV_FILE" ] || fail "$ENV_FILE does not exist. Create it and fill in the production values first."
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE does not exist."

# 生产服务器不应保留未提交的源码修改，防止部署结果与 GitHub 不一致。
if ! git diff --quiet || ! git diff --cached --quiet; then
  fail "Tracked files contain uncommitted changes. Commit/revert them before deploying."
fi

if [ -z "$GIT_BRANCH" ]; then
  GIT_BRANCH="$(git symbolic-ref --quiet --short HEAD)" \
    || fail "Cannot determine the current Git branch. Set GIT_BRANCH explicitly."
fi

mkdir -p backups/compose
cp "$COMPOSE_FILE" "backups/compose/$(basename "$COMPOSE_FILE").${TIMESTAMP}.bak"
log "Backed up $COMPOSE_FILE."

log "Fetching GitHub source: $GIT_REMOTE/$GIT_BRANCH"
git fetch --prune "$GIT_REMOTE" "$GIT_BRANCH"
git pull --ff-only "$GIT_REMOTE" "$GIT_BRANCH"
log "Current commit: $(git rev-parse --short HEAD)"

# 拉取代码后重新检查，避免仓库更新时文件被删除或改名。
[ -f "$ENV_FILE" ] || fail "$ENV_FILE is missing after the Git update."
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE is missing after the Git update."

log "Validating Docker Compose configuration..."
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  config >/dev/null

# migrate 是一次性容器。删除旧容器，确保本次部署重新执行最新数据库迁移。
log "Preparing database migration..."
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  rm -f migrate >/dev/null 2>&1 || true

log "Building and starting production services..."
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  up -d --build --remove-orphans

log "Pruning dangling images..."
docker image prune -f

log "Service status:"
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  ps

log "Recent logs:"
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  logs --tail=100

log "Deployment completed successfully."
