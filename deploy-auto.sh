#!/usr/bin/env bash
set -Eeuo pipefail

# 自动识别生产服务器 CPU 架构
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
    echo "不支持的 CPU 架构：$ARCH" >&2
    exit 1
    ;;
esac

echo "检测到服务器架构：$ARCH"
echo "Docker 运行平台：$TARGET_PLATFORM"

# 可通过第一个参数指定 compose 文件，默认使用多架构生产配置
COMPOSE_FILE="${1:-compose.prod.multiarch.yml}"

if [[ ! -f "$COMPOSE_FILE" ]]; then
  echo "找不到 Compose 文件：$COMPOSE_FILE" >&2
  exit 1
fi

docker compose -f "$COMPOSE_FILE" config >/dev/null
docker compose -f "$COMPOSE_FILE" up -d --build --remove-orphans

echo "部署完成。"
docker compose -f "$COMPOSE_FILE" ps
