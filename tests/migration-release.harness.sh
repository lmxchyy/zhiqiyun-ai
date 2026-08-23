#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${MIGRATION_RUNNER_SRC:-$ROOT/ops/run-migrations.sh}"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/migration-release.XXXXXX")"
MIGRATIONS="$WORKDIR/migrations"
BIN="$WORKDIR/bin"
STATE="$WORKDIR/applied.txt"
RUNS="$WORKDIR/runs.txt"
mkdir -p "$MIGRATIONS" "$BIN"
trap 'rm -rf -- "$WORKDIR"' EXIT

printf '%s\n' '-- old migration' >"$MIGRATIONS/107-old.sql"
printf '%s\n' '-- baseline' >"$MIGRATIONS/108-baseline.sql"
printf '%s\n' '-- next migration' >"$MIGRATIONS/109-admin-manual-membership-subscription.sql"
printf '%s\n' '-- down migration' >"$MIGRATIONS/109-admin-manual-membership-subscription.down.sql"

cat >"$BIN/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
state="${FAKE_PSQL_STATE:?}"
runs="${FAKE_PSQL_RUNS:?}"
touch "$runs"
query=""
file=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -c)
      query="$2"
      shift 2
      ;;
    -f)
      file="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -n "$file" ]; then
  if [ "$file" = "-" ]; then
    script="$(cat)"
    name="$(printf '%s\n' "$script" | grep -o '[0-9][0-9][0-9]-[^[:space:]]*\.sql' | head -1 || true)"
    if [ -n "$name" ]; then
      printf '%s\n' "$name" >>"$runs"
      if [ "${FAKE_PSQL_FAIL_FILE:-}" = "$name" ]; then exit 7; fi
      if printf '%s\n' "$script" | grep -Fq "SELECT 'APPLIED'"; then
        grep -Fxq "$name" "$state" 2>/dev/null || printf '%s\n' "$name" >>"$state"
        printf 'APPLIED\n'
      fi
    else
      name="$(printf '%s\n' "$script" | sed -n "s/.*VALUES ('\([^']*\)').*/\1/p")"
      [ -z "$name" ] || grep -Fxq "$name" "$state" 2>/dev/null || printf '%s\n' "$name" >>"$state"
    fi
    exit 0
  fi
  name="$(basename "$file")"
  printf '%s\n' "$name" >>"$runs"
  if [ "${FAKE_PSQL_FAIL_FILE:-}" = "$name" ]; then exit 7; fi
  exit 0
fi
if [[ "$query" == *"SELECT filename FROM schema_migrations"* ]]; then
  [ -f "$state" ] && cat "$state"
  exit 0
fi
if [[ "$query" == *"SELECT count(*) FROM schema_migrations"* ]]; then
  if [ -s "$state" ]; then wc -l <"$state"; else printf '0\n'; fi
  exit 0
fi
if [[ "$query" == INSERT* || "$query" == insert* ]]; then
  name="$(printf '%s\n' "$query" | sed -n "s/.*VALUES ('\([^']*\)').*/\1/p")"
  grep -Fxq "$name" "$state" 2>/dev/null || printf '%s\n' "$name" >>"$state"
  exit 0
fi
exit 0
EOF
chmod +x "$BIN/psql"

export PATH="$BIN:$PATH"
export FAKE_PSQL_STATE="$STATE" FAKE_PSQL_RUNS="$RUNS"
export POSTGRES_USER=fixture POSTGRES_DB=fixture PGPASSWORD=fixture
export MIGRATION_DIR="$MIGRATIONS" MIGRATION_FILES="108-baseline.sql"

bash "$SCRIPT"
grep -Fxq '109-admin-manual-membership-subscription.sql' "$RUNS"
if grep -Fxq '107-old.sql' "$STATE"; then
  echo 'old migration was falsely marked as applied' >&2
  exit 1
fi
if grep -Fq '.down.sql' "$RUNS"; then
  echo 'down migration was executed' >&2
  exit 1
fi
: >"$RUNS"
bash "$SCRIPT"
if [ -s "$RUNS" ]; then
  echo 'already applied migration was re-executed' >&2
  exit 1
fi

printf '%s\n' '-- later migration' >"$MIGRATIONS/111-failure.sql"
if FAKE_PSQL_FAIL_FILE=111-failure.sql bash "$SCRIPT"; then
  echo 'failed migration unexpectedly succeeded' >&2
  exit 1
fi

rm -f "$STATE"
if MIGRATION_FILES= bash "$SCRIPT"; then
  echo 'empty migration history without baseline unexpectedly succeeded' >&2
  exit 1
fi

echo 'migration harness PASS'
