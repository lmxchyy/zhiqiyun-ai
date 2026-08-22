#!/usr/bin/env bash
# Behavioral harness for backup.sh (stub docker/pg_dump; no real DB).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_SRC="${BACKUP_SRC:-$ROOT/backup.sh}"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/backup-harden.XXXXXX")"
STUB_BIN="$WORKDIR/bin"
PASS=0
FAIL=0

cleanup() {
  rm -rf -- "$WORKDIR"
}
trap cleanup EXIT

pass() {
  PASS=$((PASS + 1))
  printf 'PASS %s\n' "$1"
}

fail() {
  FAIL=$((FAIL + 1))
  printf 'FAIL %s\n' "$1" >&2
  if [ -n "${2:-}" ]; then
    printf '  detail: %s\n' "$2" >&2
  fi
}

assert_eq() {
  local name="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    pass "$name"
  else
    fail "$name" "got='$got' want='$want'"
  fi
}

assert_true() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    pass "$name"
  else
    fail "$name" "command failed: $*"
  fi
}

assert_false() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$name" "command unexpectedly succeeded: $*"
  else
    pass "$name"
  fi
}

mkdir -p "$STUB_BIN" "$WORKDIR/backups/postgres"

# Minimal compose + env so backup.sh passes prechecks.
cat >"$WORKDIR/compose.prod.yml" <<'EOF'
services:
  postgres:
    image: postgres:16-alpine
EOF

cat >"$WORKDIR/.env.production" <<'EOF'
POSTGRES_DB=testdb
POSTGRES_USER=testuser
POSTGRES_PASSWORD=testpass
EOF

# Stub docker: only needs to satisfy `docker compose ... exec ... pg_dump`.
cat >"$STUB_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
args="$*"
if [[ "$args" != *"pg_dump"* ]]; then
  echo "stub docker: unexpected invocation: $args" >&2
  exit 90
fi
if [[ "$args" != *"--clean"* ]] || [[ "$args" != *"--if-exists"* ]]; then
  echo "stub docker: pg_dump must keep --clean --if-exists" >&2
  exit 91
fi
if [ "${STUB_PG_DUMP_FAIL:-0}" = "1" ]; then
  echo "stub pg_dump: simulated failure" >&2
  exit 1
fi
# Emit compressible SQL payload.
printf '%s\n' '-- stub dump' 'CREATE TABLE stub_t(id int);'
for i in $(seq 1 50); do
  printf 'INSERT INTO stub_t VALUES (%s);\n' "$i"
done
exit 0
EOF
chmod +x "$STUB_BIN/docker"

cp -- "$BACKUP_SRC" "$WORKDIR/backup.sh"
chmod +x "$WORKDIR/backup.sh" 2>/dev/null || true

# Fixture git identity for SHA in filename/meta.
git -C "$WORKDIR" init -q
git -C "$WORKDIR" config user.email "backup-test@example.com"
git -C "$WORKDIR" config user.name "Backup Test"
git -C "$WORKDIR" add compose.prod.yml .env.production backup.sh
git -C "$WORKDIR" commit -qm "fixture"
SHORT_SHA="$(git -C "$WORKDIR" rev-parse --short=10 HEAD)"
FULL_SHA="$(git -C "$WORKDIR" rev-parse HEAD)"
BRANCH="$(git -C "$WORKDIR" branch --show-current)"

export PATH="$STUB_BIN:$PATH"

echo "==== SUCCESS FIXTURE ===="
set +e
(
  cd "$WORKDIR"
  unset STUB_PG_DUMP_FAIL
  bash ./backup.sh
)
SUCCESS_EC=$?
set -e

assert_eq "success_exit_0" "$SUCCESS_EC" "0"

mapfile -t GZ_FILES < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name 'db_*.sql.gz' | sort)
mapfile -t PART_FILES < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.part' | sort)
mapfile -t BARE_SQL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name 'db_*.sql' ! -name '*.sql.gz' | sort)

assert_eq "success_one_gz" "${#GZ_FILES[@]}" "1"
assert_eq "success_no_part_left" "${#PART_FILES[@]}" "0"
assert_eq "success_no_bare_sql" "${#BARE_SQL[@]}" "0"

if [ "${#GZ_FILES[@]}" -eq 1 ]; then
  FINAL="${GZ_FILES[0]}"
  BASE="$(basename "$FINAL")"
  META="${FINAL}.meta.json"

  case "$BASE" in
    db_*_"${SHORT_SHA}".sql.gz) pass "filename_has_short_sha" ;;
    *) fail "filename_has_short_sha" "base=$BASE short=$SHORT_SHA" ;;
  esac

  # Must contain a readable timestamp chunk (digits + underscore).
  if [[ "$BASE" =~ ^db_[0-9]{8}_[0-9]{6}_${SHORT_SHA}\.sql\.gz$ ]]; then
    pass "filename_timestamp_format"
  else
    fail "filename_timestamp_format" "base=$BASE expected db_YYYYMMDD_HHMMSS_${SHORT_SHA}.sql.gz"
  fi

  assert_true "gzip_t_ok" gzip -t "$FINAL"
  assert_true "test_s_ok" test -s "$FINAL"
  assert_true "meta_exists" test -f "$META"

  BYTES="$(wc -c <"$FINAL" | tr -d ' ')"
  if command -v sha256sum >/dev/null 2>&1; then
    GOT_SHA="$(sha256sum "$FINAL" | awk '{print $1}')"
  else
    GOT_SHA="$(shasum -a 256 "$FINAL" | awk '{print $1}')"
  fi

  # Parse meta without jq: python if present, else grep/sed.
  if command -v python3 >/dev/null 2>&1; then
    META_SHA="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["sha256"])' "$META")"
    META_BYTES="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["bytes"])' "$META")"
    META_GIT="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["git_sha"])' "$META")"
    META_SHORT="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["short_sha"])' "$META")"
    META_BRANCH="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["branch"])' "$META")"
    META_HOST="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["hostname"])' "$META")"
    META_FILE="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["backup_file"])' "$META")"
    META_START="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["started_at"])' "$META")"
    META_END="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["completed_at"])' "$META")"
    python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$META" && pass "meta_json_valid" || fail "meta_json_valid"
  else
    META_SHA="$(grep -o '"sha256"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_BYTES="$(grep -o '"bytes"[[:space:]]*:[[:space:]]*[0-9]*' "$META" | head -1 | sed 's/.*:[[:space:]]*//')"
    META_GIT="$(grep -o '"git_sha"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_SHORT="$(grep -o '"short_sha"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_BRANCH="$(grep -o '"branch"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_HOST="$(grep -o '"hostname"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_FILE="$(grep -o '"backup_file"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_START="$(grep -o '"started_at"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    META_END="$(grep -o '"completed_at"[[:space:]]*:[[:space:]]*"[^"]*"' "$META" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    pass "meta_json_valid_best_effort"
  fi

  assert_eq "meta_sha256_matches" "$META_SHA" "$GOT_SHA"
  assert_eq "meta_bytes_matches" "$META_BYTES" "$BYTES"
  assert_eq "meta_git_sha" "$META_GIT" "$FULL_SHA"
  assert_eq "meta_short_sha" "$META_SHORT" "$SHORT_SHA"
  assert_eq "meta_branch" "$META_BRANCH" "$BRANCH"
  assert_true "meta_hostname_nonempty" test -n "$META_HOST"
  assert_true "meta_started_at_nonempty" test -n "$META_START"
  assert_true "meta_completed_at_nonempty" test -n "$META_END"
  assert_eq "meta_backup_file_basename" "$(basename "$META_FILE")" "$BASE"
else
  fail "filename_has_short_sha" "no gz produced"
  fail "filename_timestamp_format" "no gz produced"
  fail "gzip_t_ok" "no gz produced"
  fail "test_s_ok" "no gz produced"
  fail "meta_exists" "no gz produced"
fi

# Historical file must not be deleted by a successful run.
HISTORIC="$WORKDIR/backups/postgres/db_historic_keep.sql"
echo historic >"$HISTORIC"
set +e
(
  cd "$WORKDIR"
  bash ./backup.sh
)
KEEP_EC=$?
set -e
assert_eq "second_success_exit_0" "$KEEP_EC" "0"
assert_true "historic_backup_preserved" test -f "$HISTORIC"

echo "==== JSON ESCAPE FIXTURE ===="
find "$WORKDIR/backups/postgres" -maxdepth 1 -type f \( -name 'db_*.sql.gz' -o -name 'db_*.sql.gz.meta.json' -o -name '*.part' \) -delete
SPECIAL_HOST='vm"quote\path'$'\t''host'
set +e
(
  cd "$WORKDIR"
  export BACKUP_HOSTNAME="$SPECIAL_HOST"
  unset STUB_PG_DUMP_FAIL BACKUP_TEST_INJECT_META_FAIL BACKUP_TEST_INJECT_AFTER_GZ_MV
  bash ./backup.sh
)
ESC_EC=$?
set -e
assert_eq "json_escape_exit_0" "$ESC_EC" "0"
mapfile -t ESC_GZ < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name 'db_*.sql.gz' | sort)
assert_eq "json_escape_one_gz" "${#ESC_GZ[@]}" "1"
if [ "${#ESC_GZ[@]}" -eq 1 ] && command -v python3 >/dev/null 2>&1; then
  ESC_META="${ESC_GZ[0]}.meta.json"
  ESC_HOST="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["hostname"])' "$ESC_META")"
  assert_eq "json_escape_hostname_roundtrip" "$ESC_HOST" "$SPECIAL_HOST"
  python3 -c 'import json,sys; json.load(open(sys.argv[1], encoding="utf-8"))' "$ESC_META" \
    && pass "json_escape_meta_parses" \
    || fail "json_escape_meta_parses"
else
  fail "json_escape_hostname_roundtrip" "python3 or gz missing"
fi

echo "==== FAILURE FIXTURE (pg_dump) ===="
# Clean generated gz/meta for failure isolation; keep historic.
find "$WORKDIR/backups/postgres" -maxdepth 1 -type f \( -name 'db_*.sql.gz' -o -name 'db_*.sql.gz.meta.json' -o -name '*.part' \) -delete

set +e
(
  cd "$WORKDIR"
  export STUB_PG_DUMP_FAIL=1
  unset BACKUP_TEST_INJECT_META_FAIL BACKUP_TEST_INJECT_AFTER_GZ_MV
  bash ./backup.sh
)
FAIL_EC=$?
set -e

assert_true "failure_nonzero_exit" test "$FAIL_EC" -ne 0

mapfile -t GZ_AFTER_FAIL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name 'db_*.sql.gz' | sort)
mapfile -t PART_AFTER_FAIL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.part' | sort)
mapfile -t META_AFTER_FAIL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.meta.json' | sort)

assert_eq "failure_no_final_gz" "${#GZ_AFTER_FAIL[@]}" "0"
assert_eq "failure_no_part" "${#PART_AFTER_FAIL[@]}" "0"
assert_eq "failure_no_meta" "${#META_AFTER_FAIL[@]}" "0"
assert_true "failure_historic_still_there" test -f "$HISTORIC"

echo "==== FAILURE FIXTURE (meta before finals) ===="
find "$WORKDIR/backups/postgres" -maxdepth 1 -type f \( -name 'db_*.sql.gz' -o -name 'db_*.sql.gz.meta.json' -o -name '*.part' \) -delete
set +e
(
  cd "$WORKDIR"
  unset STUB_PG_DUMP_FAIL BACKUP_TEST_INJECT_AFTER_GZ_MV
  export BACKUP_TEST_INJECT_META_FAIL=1
  bash ./backup.sh
)
META_FAIL_EC=$?
set -e
assert_true "meta_fail_nonzero" test "$META_FAIL_EC" -ne 0
mapfile -t GZ_META_FAIL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name 'db_*.sql.gz' | sort)
mapfile -t PART_META_FAIL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.part' | sort)
mapfile -t META_META_FAIL < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.meta.json' | sort)
assert_eq "meta_fail_no_gz" "${#GZ_META_FAIL[@]}" "0"
assert_eq "meta_fail_no_part" "${#PART_META_FAIL[@]}" "0"
assert_eq "meta_fail_no_meta" "${#META_META_FAIL[@]}" "0"
assert_true "meta_fail_historic_kept" test -f "$HISTORIC"

echo "==== FAILURE FIXTURE (after gz mv, before meta commit) ===="
find "$WORKDIR/backups/postgres" -maxdepth 1 -type f \( -name 'db_*.sql.gz' -o -name 'db_*.sql.gz.meta.json' -o -name '*.part' \) -delete
set +e
(
  cd "$WORKDIR"
  unset STUB_PG_DUMP_FAIL BACKUP_TEST_INJECT_META_FAIL
  export BACKUP_TEST_INJECT_AFTER_GZ_MV=1
  bash ./backup.sh
)
AFTER_GZ_EC=$?
set -e
assert_true "after_gz_fail_nonzero" test "$AFTER_GZ_EC" -ne 0
mapfile -t GZ_AFTER_GZ < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name 'db_*.sql.gz' | sort)
mapfile -t PART_AFTER_GZ < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.part' | sort)
mapfile -t META_AFTER_GZ < <(find "$WORKDIR/backups/postgres" -maxdepth 1 -type f -name '*.meta.json' | sort)
assert_eq "after_gz_fail_no_orphan_gz" "${#GZ_AFTER_GZ[@]}" "0"
assert_eq "after_gz_fail_no_part" "${#PART_AFTER_GZ[@]}" "0"
assert_eq "after_gz_fail_no_meta" "${#META_AFTER_GZ[@]}" "0"
assert_true "after_gz_fail_historic_kept" test -f "$HISTORIC"

echo "==== SUMMARY pass=$PASS fail=$FAIL ===="
if [ "$FAIL" -ne 0 ]; then
  exit 1
fi
exit 0
