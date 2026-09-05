#!/usr/bin/env bash
set -Eeuo pipefail

TARGET_VERSION="${1:-}"
IMMUTABLE_RELEASE="${IMMUTABLE_RELEASE:-0}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-${TARGET_VERSION}}"
RELEASE_REGISTRY="${RELEASE_REGISTRY:-}"
TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"

if [ -z "$TARGET_VERSION" ]; then
  echo "Usage: ./rollback.sh <git-tag-or-commit-or-manifest>"
  echo "Example: ./rollback.sh v1.0.1"
  echo "Example: ./rollback.sh abc1234"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-compose.prod.yml}"
if [ -z "${ENV_FILE:-}" ]; then
  if [ -f .env.production ]; then
    ENV_FILE=".env.production"
  else
    ENV_FILE=".env"
  fi
fi

update_env_file_key() {
  local file="$1" key="$2" value="$3"
  local tmp env_backup_dir
  tmp="$(mktemp "${file}.tmp.XXXXXX")" || { echo "[rollback] ERROR: Failed to create a temporary env file." >&2; exit 1; }
  env_backup_dir="backups/env"
  mkdir -p "$env_backup_dir"
  cp -p "$file" "$env_backup_dir/$(basename "$file").${TIMESTAMP}.bak"

  chmod --reference="$file" "$tmp" 2>/dev/null || chmod 600 "$tmp" || {
    rm -f "$tmp"
    echo "[rollback] ERROR: Failed to preserve permissions for $file." >&2
    exit 1
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

  [ -s "$tmp" ] || { rm -f "$tmp"; echo "[rollback] ERROR: Failed to update $file: temporary file is empty." >&2; exit 1; }
  grep -Fqx "${key}=${value}" "$tmp" >/dev/null || {
    rm -f "$tmp"
    echo "[rollback] ERROR: Verification failed for $key in temporary env file." >&2
    exit 1
  }
  chmod --reference="$file" "$tmp" 2>/dev/null || chmod 600 "$tmp" || {
    rm -f "$tmp"
    echo "[rollback] ERROR: Failed to preserve permissions for $file." >&2
    exit 1
  }

  mv -f "$tmp" "$file"
}

validate_compose_desired_state() {
  local rendered="$1"
  python3 -c '
import json
import sys

expected = sys.argv[1]
data = json.load(sys.stdin)
for service in ("xianzhi-ai", "smartvideo-worker"):
    actual = data.get("services", {}).get(service, {}).get("image")
    if actual != expected:
        raise SystemExit(f"Compose service {service} desired image mismatch: expected {expected}, got {actual}")
' "$XIANZHI_IMAGE_REFERENCE" <<< "$rendered" || {
    echo "[rollback] ERROR: Compose desired state does not match the immutable release." >&2
    exit 1
  }
}

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
  XIANZHI_IMAGE_REFERENCE="$(bash ops/verify-release-manifest.sh "$RELEASE_MANIFEST" "" "" "$RELEASE_REGISTRY")"
  export XIANZHI_IMAGE_REFERENCE
  case "$XIANZHI_IMAGE_REFERENCE" in
    *@sha256:*) ;;
    *) echo "[rollback] ERROR: Release manifest did not provide a digest-pinned image reference." >&2; exit 1 ;;
  esac

  echo "[rollback] Validating Docker Compose configuration..."
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config >/dev/null

  compose_config="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" || { echo "[rollback] ERROR: Failed to render Docker Compose configuration." >&2; exit 1; }
  validate_compose_desired_state "$compose_config"

  echo "[rollback] Pulling exact immutable image: $XIANZHI_IMAGE_REFERENCE"
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull xianzhi-ai smartvideo-worker
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" rm -f migrate >/dev/null 2>&1 || true
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d --no-build --remove-orphans
  expected_image_id="$(docker image inspect "$XIANZHI_IMAGE_REFERENCE" --format '{{.Id}}')" \
    || { echo "[rollback] ERROR: Cannot inspect the manifest image locally." >&2; exit 1; }
  [ -n "$expected_image_id" ] || { echo "[rollback] ERROR: Manifest image has no local image ID." >&2; exit 1; }
  for service in xianzhi-ai smartvideo-worker; do
    container_id="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps -q "$service")"
    [ -n "$container_id" ] || { echo "[rollback] ERROR: no running container for $service." >&2; exit 1; }

    configured_image="$(docker inspect --format '{{.Config.Image}}' "$container_id")" \
      || { echo "[rollback] ERROR: Cannot inspect the configured image for $service." >&2; exit 1; }
    [ "$configured_image" = "$XIANZHI_IMAGE_REFERENCE" ] \
      || { echo "[rollback] ERROR: PARTIAL_RELEASE_DETECTED: $service Config.Image is not the manifest reference." >&2; exit 1; }

    running_image_id="$(docker inspect --format '{{.Image}}' "$container_id")" \
      || { echo "[rollback] ERROR: Cannot inspect the running image for $service." >&2; exit 1; }
    [ "$running_image_id" = "$expected_image_id" ] \
      || { echo "[rollback] ERROR: PARTIAL_RELEASE_DETECTED: $service is not running the manifest image digest." >&2; exit 1; }

    repo_digests="$(docker image inspect "$running_image_id" --format '{{range .RepoDigests}}{{println .}}{{end}}')" \
      || { echo "[rollback] ERROR: Cannot inspect RepoDigests for $service." >&2; exit 1; }
    printf '%s\n' "$repo_digests" | grep -Fqx -- "$XIANZHI_IMAGE_REFERENCE" \
      || { echo "[rollback] ERROR: PARTIAL_RELEASE_DETECTED: $service RepoDigests do not contain the manifest reference." >&2; exit 1; }
  done
  update_env_file_key "$ENV_FILE" "XIANZHI_IMAGE_REFERENCE" "$XIANZHI_IMAGE_REFERENCE"
  echo "[rollback] Persisted XIANZHI_IMAGE_REFERENCE to $ENV_FILE."
  fresh_compose_config="$(env -u XIANZHI_IMAGE_REFERENCE docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" config --format json 2>/dev/null)" \
    || { echo "[rollback] ERROR: Failed to render fresh Docker Compose configuration." >&2; exit 1; }
  validate_compose_desired_state "$fresh_compose_config"
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
