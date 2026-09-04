#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$REPO_ROOT/ops/backup-offsite-upload-pending.sh"
SCENARIO="${1:-first-invalid-second-valid}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/backup-offsite-batch.XXXXXX")"
POSTGRES_ROOT="$ROOT/backups/postgres"
mkdir -p "$POSTGRES_ROOT"

set_file() {
  FILE="$POSTGRES_ROOT/$1"
  META="$FILE.meta.json"
}

write_meta() {
  local bytes sha
  bytes="$(wc -c <"$FILE" | tr -d '[:space:]')"
  sha="$(sha256sum "$FILE" | awk '{print $1}')"
  printf '{\n  "git_sha": "test-git-sha",\n  "short_sha": "test-git",\n  "branch": "main",\n  "hostname": "fixture",\n  "started_at": "2026-08-22T15:52:44+08:00",\n  "completed_at": "2026-08-22T15:52:45+08:00",\n  "backup_file": "%s",\n  "bytes": %s,\n  "sha256": "%s"\n}\n' "$FILE" "$bytes" "$sha" >"$META"
}

make_valid() {
  set_file "db_20260822_155244_abcdef1234.sql.gz"
  printf 'fixture sql payload\n' | gzip -c >"$FILE"
  write_meta
  touch -t 202608221552.44 "$FILE" "$META"
}

make_invalid_no_meta() {
  set_file "db_20260821_195734.sql"
  printf 'fixture sql payload without meta\n' >"$FILE"
  touch -t 202608211957.34 "$FILE"
}

make_second_valid() {
  set_file "db_20260823_160000_ffffff0000.sql.gz"
  printf 'second valid sql payload\n' | gzip -c >"$FILE"
  write_meta
  touch -t 202608231600.00 "$FILE" "$META"
}

run_pending() {
  BACKUP_ROOT="$POSTGRES_ROOT" \
  COMPOSE_FILE="$REPO_ROOT/compose.prod.yml" \
  ENV_FILE="$REPO_ROOT/.env.production.example" \
  BACKUP_OBS_ENV_FILE="$REPO_ROOT/ops/backup-uploader/backup-obs.env.example" \
  BACKUP_UPLOADER_IMAGE='xianzhi-ai-platform:test' \
  "$SCRIPT"
}

expect_marker() {
  local marker="$1"
  find "$POSTGRES_ROOT" -maxdepth 1 -name "*.offsite.json" -print | grep -q "$marker"
}

case "$SCENARIO" in
  first-invalid-second-valid)
    make_invalid_no_meta
    make_second_valid
    run_pending >"$ROOT/summary.json" 2>"$ROOT/summary.stderr" || true
    expect_marker "db_20260823_160000_ffffff0000.sql.gz.offsite.json"
    grep -q "LOCAL_BACKUP_INVALID" "$ROOT/summary.stderr"
    grep -q "db_20260821_195734.sql" "$ROOT/summary.stderr"
    ;;
  all-invalid)
    make_invalid_no_meta
    run_pending >"$ROOT/summary.json" 2>"$ROOT/summary.stderr" || true
    ! expect_marker "db_20260821_195734.sql.gz.offsite.json"
    grep -q "LOCAL_BACKUP_INVALID" "$ROOT/summary.stderr"
    ;;
  *)
    printf 'unknown scenario: %s\n' "$SCENARIO" >&2
    exit 2
    ;;
esac
