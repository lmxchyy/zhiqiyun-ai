#!/usr/bin/env bash
set -e

TARGET_VERSION="${1:-}"

if [ -z "$TARGET_VERSION" ]; then
  echo "Usage: ./rollback.sh <git-tag-or-commit>"
  echo "Example: ./rollback.sh v1.0.1"
  echo "Example: ./rollback.sh abc1234"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env}"

printf '[rollback] Project directory: %s\n' "$SCRIPT_DIR"
printf '[rollback] Target version: %s\n' "$TARGET_VERSION"
echo "[rollback] WARNING: Code rollback is not database rollback."
echo "[rollback] If this release ran schema migrations, confirm database compatibility or restore from backup."

if [ ! -f "$ENV_FILE" ]; then
  echo "[rollback] ERROR: $ENV_FILE does not exist." >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "[rollback] ERROR: $COMPOSE_FILE does not exist." >&2
  exit 1
fi

echo "[rollback] Fetching all remotes and tags..."
git fetch --all --tags

echo "[rollback] Checking out $TARGET_VERSION..."
git checkout "$TARGET_VERSION"

echo "[rollback] Rebuilding and starting services..."
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --build

echo "[rollback] Service status:"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps

echo "[rollback] Recent logs:"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs --tail=100
