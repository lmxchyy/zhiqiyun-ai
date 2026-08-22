#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$REPO_ROOT/ops/backup-upload-object-storage.sh"
SCENARIO="${1:-valid-dry-run}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/backup-offsite-upload.XXXXXX")"
BACKUP_ROOT="$ROOT/backups"
POSTGRES_ROOT="$BACKUP_ROOT/postgres"
FAKE_ROOT="$ROOT/fake-object-store"
mkdir -p "$POSTGRES_ROOT" "$FAKE_ROOT"

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

make_named_valid() {
  set_file "db_20260822_155244_abcdef [x] --.sql.gz"
  printf 'fixture sql payload with a dangerous filename\n' | gzip -c >"$FILE"
  write_meta
  touch -t 202608221552.44 "$FILE" "$META"
}

make_bad_gzip() {
  set_file "db_20260822_155244_abcdef1234.sql.gz"
  printf 'not a gzip stream\n' >"$FILE"
  write_meta
  touch -t 202608221552.44 "$FILE" "$META"
}

run_fake() {
  "$SCRIPT" --root "$BACKUP_ROOT" --provider fake --fake-root "$FAKE_ROOT" --file "$FILE" --json "$@"
}

run_obs_fake() {
  BACKUP_OBS_FAKE=1 BACKUP_OBS_BUCKET=fake-bucket BACKUP_OBS_ENDPOINT=https://obs.example.invalid BACKUP_OBS_REGION=cn-north-4 \
    BACKUP_OBJECT_FAKE_ROOT="$FAKE_ROOT" \
    "$SCRIPT" --root "$BACKUP_ROOT" --provider obs --fake-root "$FAKE_ROOT" --file "$FILE" --json "$@"
}

expect_no_marker() {
  local runner status markers
  runner="$1"
  shift
  set +e
  "$runner" "$@"
  status=$?
  set -e
  markers="$(find "$BACKUP_ROOT" -type f -name '*.offsite.json' -print)"
  if [[ -n "$markers" ]]; then
    printf 'UNEXPECTED_OFFSITE_MARKER: %s\n' "$markers" >&2
    exit 1
  fi
  return "$status"
}

case "$SCENARIO" in
  valid-dry-run)
    make_valid
    run_fake --dry-run
    ;;
  valid-upload)
    make_valid
    run_fake --upload
    ;;
  idempotent)
    make_valid
    run_fake --upload >"$ROOT/first.json"
    run_fake --upload
    ;;
  remote-conflict)
    make_valid
    object="$FAKE_ROOT/backups/postgres/deploy/2026/08/$(basename "$FILE")"
    mkdir -p "$(dirname "$object")"
    printf 'conflicting remote object\n' >"$object"
    printf '{"size": 26, "sha256": "conflicting-checksum"}\n' >"$object.object-meta.json"
    expect_no_marker run_fake --upload
    ;;
  remote-size-mismatch)
    make_valid
    BACKUP_OBJECT_FAKE_HEAD_SIZE_DELTA=1 expect_no_marker run_fake --upload
    ;;
  remote-checksum-mismatch)
    make_valid
    BACKUP_OBJECT_FAKE_HEAD_SHA256=remote-conflict expect_no_marker run_fake --upload
    ;;
  missing-meta)
    set_file "db_20260822_155244_abcdef1234.sql.gz"
    printf 'fixture sql payload\n' | gzip -c >"$FILE"
    run_fake --dry-run
    ;;
  sha-mismatch)
    make_valid
    printf '{"bytes": 1, "sha256": "not-the-real-sha256"}\n' >"$META"
    run_fake --dry-run
    ;;
  bytes-mismatch)
    make_valid
    bytes="$(wc -c <"$FILE" | tr -d '[:space:]')"
    sha="$(sha256sum "$FILE" | awk '{print $1}')"
    printf '{"bytes": %s, "sha256": "%s"}\n' "$((bytes + 1))" "$sha" >"$META"
    run_fake --dry-run
    ;;
  bad-gzip)
    make_bad_gzip
    run_fake --dry-run
    ;;
  symlink)
    make_valid
    target="$FILE"
    set_file "db_20260822_155244_symlink.sql.gz"
    if ! ln -s "$target" "$FILE" 2>/dev/null; then
      printf '%s\n' 'SYMLINK_FIXTURE_UNAVAILABLE' >&2
      exit 0
    fi
    run_fake --dry-run
    ;;
  outside-root)
    outside="$ROOT/outside-db_20260822_155244.sql.gz"
    FILE="$outside"
    META="$outside.meta.json"
    printf 'outside fixture\n' | gzip -c >"$FILE"
    write_meta
    run_fake --dry-run
    ;;
  credentials-missing)
    make_valid
    env -u BACKUP_OBJECT_ENDPOINT -u BACKUP_OBJECT_BUCKET -u BACKUP_OBJECT_REGION \
      -u TENCENTCLOUD_SECRET_ID -u TENCENTCLOUD_SECRET_KEY \
      "$SCRIPT" --root "$BACKUP_ROOT" --provider cos --file "$FILE" --json --upload
    ;;
  obs-valid-upload)
    make_valid
    run_obs_fake --upload
    object_meta="$FAKE_ROOT/backups/postgres/deploy/2026/08/$(basename "$FILE").object-meta.json"
    grep -q 'sha256' "$object_meta"
    ;;
  obs-prefix-injection)
    make_valid
    BACKUP_OBJECT_PREFIX=images/ run_obs_fake --upload
    ;;
  obs-existing-identical)
    make_valid
    run_obs_fake --upload >/dev/null
    run_obs_fake --upload
    ;;
  obs-remote-conflict)
    make_valid
    object="$FAKE_ROOT/backups/postgres/deploy/2026/08/$(basename "$FILE")"
    mkdir -p "$(dirname "$object")"
    printf 'conflicting remote object\n' >"$object"
    printf '{"size": 26, "sha256": "conflicting-checksum"}\n' >"$object.object-meta.json"
    expect_no_marker run_obs_fake --upload
    ;;
  obs-auth-failure|obs-timeout|obs-network-failure|obs-partial-upload|obs-meta-failure|obs-sha-failure)
    make_valid
    BACKUP_OBJECT_FAKE_FAILURE="${SCENARIO#obs-}" expect_no_marker run_obs_fake --upload
    ;;
  obs-secret-redaction)
    make_valid
    FAKE_ACCESS_KEY=FAKE_ACCESS_KEY FAKE_SECRET_KEY=FAKE_SECRET_KEY \
      BACKUP_OBJECT_FAKE_FAILURE=auth-failure expect_no_marker run_obs_fake --upload
    ;;
  obs-config-missing)
    make_valid
    env -u BACKUP_OBS_BUCKET -u BACKUP_OBS_ENDPOINT -u BACKUP_OBS_REGION \
      -u OBS_ACCESS_KEY_ID -u OBS_SECRET_ACCESS_KEY -u OBS_SECURITY_TOKEN \
      "$SCRIPT" --root "$BACKUP_ROOT" --provider obs --file "$FILE" --json --upload
    ;;
  obs-bucket-missing)
    make_valid
    env -u BACKUP_OBS_BUCKET BACKUP_OBS_ENDPOINT=https://obs.example.invalid BACKUP_OBS_REGION=cn-north-4 \
      "$SCRIPT" --root "$BACKUP_ROOT" --provider obs --file "$FILE" --json --upload
    ;;
  obs-endpoint-missing)
    make_valid
    env -u BACKUP_OBS_ENDPOINT BACKUP_OBS_BUCKET=fake-bucket BACKUP_OBS_REGION=cn-north-4 \
      "$SCRIPT" --root "$BACKUP_ROOT" --provider obs --file "$FILE" --json --upload
    ;;
  obs-region-missing)
    make_valid
    env -u BACKUP_OBS_REGION BACKUP_OBS_BUCKET=fake-bucket BACKUP_OBS_ENDPOINT=https://obs.example.invalid \
      "$SCRIPT" --root "$BACKUP_ROOT" --provider obs --file "$FILE" --json --upload
    ;;
filename-safe)
    if [[ "${OSTYPE:-}" == msys* ]]; then
      printf '%s\n' 'SPECIAL_FILENAME_FIXTURE_UNAVAILABLE' >&2
      exit 0
    fi
    make_named_valid
    run_fake --dry-run
    ;;
  *)
    printf 'unknown scenario: %s\n' "$SCENARIO" >&2
    exit 2
    ;;
esac
