#!/bin/sh
set -eu

MIGRATION_DIR="${MIGRATION_DIR:-/migrations}"
PGHOST="${PGHOST:-postgres}"

log() { printf '[migration] %s\n' "$*"; }
fail() { printf '[migration] ERROR: %s\n' "$*" >&2; exit 1; }

psql_run() {
  PGPASSWORD="${PGPASSWORD:?PGPASSWORD is required}" \
    psql -h "$PGHOST" -U "${POSTGRES_USER:?POSTGRES_USER is required}" \
      -d "${POSTGRES_DB:?POSTGRES_DB is required}" -v ON_ERROR_STOP=1 "$@"
}

validate_name() {
  name="$1"
  case "$name" in
    [0-9][0-9][0-9]-*.sql) ;;
    *) fail "Invalid migration filename: $name" ;;
  esac
  case "$name" in
    *.down.sql) fail "Down migration is not executable: $name" ;;
  esac
  [ "$name" = "$(basename "$name")" ] || fail "Migration path must be a filename: $name"
}

version_of() { printf '%s\n' "$1" | cut -d- -f1; }
is_applied() { printf '%s\n' "$applied" | grep -Fx "$1" >/dev/null 2>&1; }

record_applied() {
  name="$1"
  psql_run -q -c "INSERT INTO schema_migrations (filename) VALUES ('$name') ON CONFLICT (filename) DO NOTHING;"
  applied="$(psql_run -A -t -q -c 'SELECT filename FROM schema_migrations ORDER BY filename;')"
}

record_baseline() {
  name="$1"
  {
    printf '%s\n' "SELECT pg_advisory_lock(hashtext('xianzhi:schema_migrations'));"
    printf '%s\n' "INSERT INTO schema_migrations (filename) VALUES ('$name') ON CONFLICT (filename) DO NOTHING;"
    printf '%s\n' "SELECT pg_advisory_unlock(hashtext('xianzhi:schema_migrations'));"
  } | psql_run -q -f -
}

run_one() {
  name="$1"
  result="$({
    printf '%s\n' "SELECT pg_advisory_lock(hashtext('xianzhi:schema_migrations'));"
    printf '%s\n' "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = '$name') AS already_applied \\gset"
    printf '%s\n' '\if :already_applied'
    printf '%s\n' "SELECT 'SKIPPED' AS migration_result;"
    printf '%s\n' '\else'
    printf '%s\n' "\\i $MIGRATION_DIR/$name"
    printf '%s\n' "INSERT INTO schema_migrations (filename) VALUES ('$name') ON CONFLICT (filename) DO NOTHING;"
    printf '%s\n' "SELECT 'APPLIED' AS migration_result;"
    printf '%s\n' '\endif'
    printf '%s\n' "SELECT pg_advisory_unlock(hashtext('xianzhi:schema_migrations'));"
  } | psql_run -A -t -q -f -)" || return $?
  case "$result" in
    *APPLIED*) log "Applied: $name" ;;
    *) log "Already applied: $name" ;;
  esac
}

[ -d "$MIGRATION_DIR" ] || fail "Migration directory not found: $MIGRATION_DIR"
psql_run -q -c '
CREATE TABLE IF NOT EXISTS schema_migrations (
  filename text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);'

applied="$(psql_run -A -t -q -c 'SELECT filename FROM schema_migrations ORDER BY filename;')"
applied_count="$(psql_run -A -t -q -c 'SELECT count(*) FROM schema_migrations;')"

# MIGRATION_FILES is the existing release baseline marker. On the first run
# only, it records those explicitly named files without re-running them. Files
# with lower versions are skipped by the baseline range, but are not falsely
# inserted into schema_migrations.
baseline_max=0
for baseline in ${MIGRATION_FILES:-}; do
  validate_name "$baseline"
  [ -f "$MIGRATION_DIR/$baseline" ] || fail "Baseline migration file not found: $baseline"
  version="$(version_of "$baseline")"
  [ "$version" -gt "$baseline_max" ] && baseline_max="$version"
done

if [ -z "$applied" ] && [ "$baseline_max" -eq 0 ]; then
  fail 'schema_migrations is empty; MIGRATION_FILES baseline is required'
fi

if [ -z "$applied" ] && [ "$baseline_max" -gt 0 ]; then
  log "Bootstrapping explicitly configured migration baseline through version $baseline_max"
  for baseline in ${MIGRATION_FILES:-}; do
    record_baseline "$baseline"
    log "Baseline recorded: $baseline"
  done
  applied="$(psql_run -A -t -q -c 'SELECT filename FROM schema_migrations ORDER BY filename;')"
fi

find "$MIGRATION_DIR" -maxdepth 1 -type f -name '[0-9][0-9][0-9]-*.sql' ! -name '*.down.sql' -exec basename {} \; \
  | sort \
  | while IFS= read -r name; do
      validate_name "$name"
      version="$(version_of "$name")"
      if [ "$baseline_max" -gt 0 ] && [ "$version" -le "$baseline_max" ]; then
        log "Baseline range: $name"
        continue
      fi
      if is_applied "$name"; then
        log "Already applied: $name"
        continue
      fi
      log "Applying: $name"
      run_one "$name"
    done

log 'Migration release completed successfully.'
