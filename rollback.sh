#!/usr/bin/env bash
set -Eeuo pipefail

TARGET_VERSION="${1:-}"
IMMUTABLE_RELEASE="${IMMUTABLE_RELEASE:-0}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-${TARGET_VERSION}}"

if [ -z "$TARGET_VERSION" ]; then
  echo "Usage: ./rollback.sh <git-tag-or-commit-or-manifest>"
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

if [ "$IMMUTABLE_RELEASE" = "1" ]; then
  [ -x ops/verify-release-manifest.sh ] || { echo "[rollback] ERROR: manifest validator is not executable." >&2; exit 1; }
  [ -f "$RELEASE_MANIFEST" ] || { echo "[rollback] ERROR: release manifest does not exist." >&2; exit 1; }
  XIANZHI_IMAGE_REFERENCE="$(bash ops/verify-release-manifest.sh "$RELEASE_MANIFEST")"
  export XIANZHI_IMAGE_REFERENCE
  echo "[rollback] Pulling exact immutable image: $XIANZHI_IMAGE_REFERENCE"
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull xianzhi-ai smartvideo-worker
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" rm -f migrate >/dev/null 2>&1 || true
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --no-build --remove-orphans
  for service in xianzhi-ai smartvideo-worker; do
    container_id="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q "$service")"
    [ -n "$container_id" ] || { echo "[rollback] ERROR: no running container for $service." >&2; exit 1; }
    repo_digests="$(docker inspect --format '{{join .RepoDigests "\\n"}}' "$container_id")"
    printf '%s\n' "$repo_digests" | grep -Fqx -- "$XIANZHI_IMAGE_REFERENCE" \
      || { echo "[rollback] ERROR: $service is not running the manifest digest." >&2; exit 1; }
  done
  echo "[rollback] Immutable rollback completed. Database rollback was not performed."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs --tail=100
  exit 0
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
