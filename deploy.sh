#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

printf '[deploy] Project directory: %s\n' "$SCRIPT_DIR"
printf '[deploy] Compose file: %s\n' "$COMPOSE_FILE"
printf '[deploy] Env file: %s\n' "$ENV_FILE"

if [ ! -f "$ENV_FILE" ]; then
  echo "[deploy] ERROR: $ENV_FILE does not exist. Copy .env.example to .env and fill production values first." >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "[deploy] ERROR: $COMPOSE_FILE does not exist." >&2
  exit 1
fi

mkdir -p backups/compose
cp "$COMPOSE_FILE" "backups/compose/${COMPOSE_FILE}.${TIMESTAMP}.bak"
echo "[deploy] Backed up current $COMPOSE_FILE to backups/compose/${COMPOSE_FILE}.${TIMESTAMP}.bak"

echo "[deploy] Pulling latest code..."
git pull --ff-only

echo "[deploy] Building and starting services with $COMPOSE_FILE..."
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --build

echo "[deploy] Pruning unused images..."
docker image prune -f

echo "[deploy] Service status:"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps

echo "[deploy] Recent logs:"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs --tail=100
