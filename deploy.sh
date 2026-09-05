#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
GIT_BRANCH="${GIT_BRANCH:-}"
IMMUTABLE_RELEASE="${IMMUTABLE_RELEASE:-0}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-}"
RELEASE_REGISTRY="${RELEASE_REGISTRY:-}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

log() {
  printf '[deploy] %s\n' "$*"
}

fail() {
  printf '[deploy] ERROR: %s\n' "$*" >&2
  exit 1
}

update_env_file_key() {
  local file="$1" key="$2" value="$3"
  local tmp env_backup_dir
  tmp="$(mktemp "${file}.tmp.XXXXXX")" || fail "Failed to create a temporary env file."
  env_backup_dir="backups/env"
  mkdir -p "$env_backup_dir"
  cp -p "$file" "$env_backup_dir/$(basename "$file").${TIMESTAMP}.bak"

  chmod --reference="$file" "$tmp" 2>/dev/null || chmod 600 "$tmp" || {
    rm -f "$tmp"
    fail "Failed to preserve permissions for $file."
  }

  awk -v k="$key" -v v="$value" '
    BEGIN { re = "^[#[:space:]]*" k "=" }
    $0 ~ re {
      if (!replaced) print k "=" v
      replaced = 1
      next
    }
    { print }
    END { if (!replaced) print k "=" v }
  ' "$file" > "$tmp"

  [ -s "$tmp" ] || { rm -f "$tmp"; fail "Failed to update $file: temporary file is empty."; }
  grep -Fqx "${key}=${value}" "$tmp" >/dev/null || {
    rm -f "$tmp"
    fail "Verification failed for $key in temporary env file."
  }
  chmod --reference="$file" "$tmp" 2>/dev/null || chmod 600 "$tmp" || {
    rm -f "$tmp"
    fail "Failed to preserve permissions for $file."
  }

  mv -f "$tmp" "$file"
}

validate_compose_desired_state() {
  local rendered="$1"
  python3 -c '
import json
import sys

expected, rendered = sys.argv[1], sys.stdin.read()
data = json.loads(rendered)
services = data.get("services", {})
for service in ("xianzhi-ai", "smartvideo-worker"):
    actual = services.get(service, {}).get("image")
    if actual != expected:
        raise SystemExit(f"Compose service {service} desired image mismatch: expected {expected}, got {actual}")
' "$XIANZHI_IMAGE_REFERENCE" <<< "$rendered" || fail "Compose desired state does not match the immutable release."
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

log "Checking deployment filesystem capacity..."
DISK_WARN_PERCENT="${DISK_WARN_PERCENT:-70}" \
DISK_CRITICAL_PERCENT="${DISK_CRITICAL_PERCENT:-80}" \
DISK_EMERGENCY_PERCENT="${DISK_EMERGENCY_PERCENT:-90}" \
DISK_MIN_FREE_BYTES="${DEPLOY_MIN_FREE_BYTES:-10737418240}" \
  sh ops/disk-guard.sh "$SCRIPT_DIR" || fail "Insufficient disk space for a safe deployment."

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

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  [ -n "$RELEASE_MANIFEST" ] || fail "RELEASE_MANIFEST is required for immutable release."
  [ -x ops/verify-release-manifest.sh ] || fail "ops/verify-release-manifest.sh must be executable."
  export XIANZHI_IMAGE_REFERENCE="$(bash ops/verify-release-manifest.sh "$RELEASE_MANIFEST" "$(git rev-parse HEAD)" "" "$RELEASE_REGISTRY")"
  case "$XIANZHI_IMAGE_REFERENCE" in
    *@sha256:*) ;;
    *) fail "Release manifest did not provide a digest-pinned image reference." ;;
  esac
  log "Immutable image: $XIANZHI_IMAGE_REFERENCE"
fi

log "Validating Docker Compose configuration..."
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  config >/dev/null

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  log "Validating Docker Compose desired state matches manifest..."
  compose_config="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" \
    || fail "Failed to render Docker Compose configuration."
  validate_compose_desired_state "$compose_config"
fi

# migrate 是一次性容器。删除旧容器，确保本次部署重新执行最新数据库迁移。
log "Preparing database migration..."
docker compose \
  -f "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  rm -f migrate >/dev/null 2>&1 || true

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  log "Pulling and starting immutable production services..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull xianzhi-ai smartvideo-worker
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --no-build --remove-orphans
else
  log "Building and starting production services..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --build --remove-orphans
fi

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  expected_image_id="$(docker image inspect "$XIANZHI_IMAGE_REFERENCE" --format '{{.Id}}')" \
    || fail "Cannot inspect the manifest image locally."
  [ -n "$expected_image_id" ] || fail "Manifest image has no local image ID."
  for service in xianzhi-ai smartvideo-worker; do
    container_id="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q "$service")"
    [ -n "$container_id" ] || fail "No running container found for $service."

    configured_image="$(docker inspect --format '{{.Config.Image}}' "$container_id")" \
      || fail "Cannot inspect the configured image for $service."
    [ "$configured_image" = "$XIANZHI_IMAGE_REFERENCE" ] \
      || fail "PARTIAL_RELEASE_DETECTED: $service Config.Image is not the manifest reference."

    running_image_id="$(docker inspect --format '{{.Image}}' "$container_id")" \
      || fail "Cannot inspect the running image for $service."
    [ "$running_image_id" = "$expected_image_id" ] \
      || fail "PARTIAL_RELEASE_DETECTED: $service is not running the manifest image digest."

    repo_digests="$(docker image inspect "$running_image_id" --format '{{range .RepoDigests}}{{println .}}{{end}}')" \
      || fail "Cannot inspect RepoDigests for $service."
    printf '%s\n' "$repo_digests" | grep -Fqx -- "$XIANZHI_IMAGE_REFERENCE" \
      || fail "PARTIAL_RELEASE_DETECTED: $service RepoDigests do not contain the manifest reference."
  done
  log "Running API and worker match the immutable release digest."

  update_env_file_key "$ENV_FILE" "XIANZHI_IMAGE_REFERENCE" "$XIANZHI_IMAGE_REFERENCE"
  log "Persisted XIANZHI_IMAGE_REFERENCE to $ENV_FILE."

  log "Rechecking fresh Compose resolution from the persisted env file..."
  fresh_compose_config="$(env -u XIANZHI_IMAGE_REFERENCE docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" \
    || fail "Failed to render fresh Docker Compose configuration."
  validate_compose_desired_state "$fresh_compose_config"
fi

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
