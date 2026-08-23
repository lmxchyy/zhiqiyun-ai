#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail() {
  printf '[production-contract] ERROR: %s\n' "$*" >&2
  exit 1
}

grep_fixed() {
  local needle="$1"
  local file="$2"
  grep -Fq -- "$needle" "$file" || fail "missing '$needle' in $file"
}

grep_fixed 'ca-certificates' Dockerfile
grep_fixed 'tzdata' Dockerfile
grep_fixed 'curl' Dockerfile
grep_fixed 'USER xianzhi' Dockerfile
grep_fixed 'HEALTHCHECK' Dockerfile
grep_fixed 'COPY --from=api-build /out/xianzhi-api /app/xianzhi-api' Dockerfile
grep_fixed 'COPY --from=api-build /out/smartvideo-worker /app/smartvideo-worker' Dockerfile

grep_fixed 'migrate:' compose.prod.yml
grep_fixed 'condition: service_completed_successfully' compose.prod.yml
grep_fixed './database/migrations:/migrations:ro' compose.prod.yml
grep_fixed './ops/run-migrations.sh:/usr/local/bin/run-migrations.sh:ro' compose.prod.yml

mapfile -t migrations < <(
  find database/migrations -maxdepth 1 -type f \
    -name '[0-9][0-9][0-9]-*.sql' ! -name '*.down.sql' \
    -printf '%f\n' | sort
)
[ "${#migrations[@]}" -gt 0 ] || fail 'no forward migrations found'

for migration in "${migrations[@]}"; do
  case "$migration" in
    *.down.sql) fail "down migration selected: $migration" ;;
  esac
done

latest="${migrations[${#migrations[@]}-1]}"
printf '%s\n' "[production-contract] forward migrations: ${#migrations[@]} (latest=$latest)"
printf '%s\n' "[production-contract] static runtime and startup gates PASS"

if [ "${RUN_PRODUCTION_CONTRACT_DOCKER:-0}" != "1" ]; then
  printf '%s\n' '[production-contract] Docker replay skipped (set RUN_PRODUCTION_CONTRACT_DOCKER=1 in CI)'
  printf '%s\n' 'production contract harness PASS'
  exit 0
fi

command -v docker >/dev/null 2>&1 || fail 'Docker is required for production contract replay'
docker info >/dev/null 2>&1 || fail 'Docker daemon is unavailable'

image="${PRODUCTION_CONTRACT_IMAGE:-xianzhi-production-contract:${GITHUB_SHA:-local}}"
if [ -n "${PRODUCTION_CONTRACT_IMAGE:-}" ]; then
  printf '%s\n' "[production-contract] reusing prebuilt image: $image"
else
  docker build --tag "$image" .
fi
docker run --rm --entrypoint sh "$image" -ceu '
  test "$(id -u)" != "0"
  command -v curl >/dev/null
  command -v date >/dev/null
  test -s /app/xianzhi-api
  test -s /app/smartvideo-worker
  TZ=Asia/Shanghai date +%Z >/dev/null
'

pg_container="production-contract-postgres-${RANDOM}-${RANDOM}"
cleanup() {
  docker rm -f "$pg_container" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d --name "$pg_container" \
  -e POSTGRES_DB=xianzhi_contract \
  -e POSTGRES_USER=contract \
  -e POSTGRES_PASSWORD=contract \
  pgvector/pgvector:pg16 >/dev/null

for attempt in $(seq 1 30); do
  if docker exec "$pg_container" pg_isready -U contract -d xianzhi_contract >/dev/null 2>&1; then
    break
  fi
  [ "$attempt" -lt 30 ] || fail 'PostgreSQL did not become ready'
  sleep 1
done

docker exec -i "$pg_container" psql -U contract -d xianzhi_contract -v ON_ERROR_STOP=1 < database/schema.sql >/dev/null
for migration in "${migrations[@]}"; do
  printf '%s\n' "[production-contract] applying $migration"
  docker exec -i "$pg_container" psql -U contract -d xianzhi_contract -v ON_ERROR_STOP=1 < "database/migrations/$migration" >/dev/null
done

printf '%s\n' '[production-contract] Docker image, timezone, PostgreSQL replay, and forward migration gates PASS'
printf '%s\n' 'production contract harness PASS'
