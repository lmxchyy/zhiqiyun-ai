#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'
export PATH="/usr/bin:/bin:$PATH"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${BACKUP_RETENTION_SRC:-$ROOT/ops/backup-retention.sh}"
NOW="2026-08-22T15:52:44+08:00"
SCENARIO="${1:-full}"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/backup-retention.XXXXXX")"
BACKUP_ROOT="$WORKDIR/backups"

touch_at() {
  local path="$1" timestamp="$2"
  mkdir -p "${path%/*}"
  printf 'fixture %s\n' "${path##*/}" > "$path"
  touch -d "$timestamp" "$path"
}

write_offsite_evidence() {
  local file="$1" bytes sha
  bytes="$(wc -c <"$file" | tr -d '[:space:]')"
  sha="$(sha256sum "$file" | awk '{print $1}')"
  printf '{"verification":"OFFSITE_VERIFIED","uploaded_at":"2026-08-16T06:00:01Z","local_bytes":%s,"remote_bytes":%s,"local_sha256":"%s","remote_sha256":"%s","object_key":"backups/postgres/deploy/2026/08/%s"}\n' "$bytes" "$bytes" "$sha" "$sha" "${file##*/}" > "${file}.offsite.json"
  touch -d '2026-08-16 14:00:01 +0800' "${file}.offsite.json"
}

mkdir -p "$BACKUP_ROOT/postgres"

if [ "$SCENARIO" = "insufficient" ]; then
  touch_at "$BACKUP_ROOT/postgres/db_20260822_214315_a.sql.gz" '2026-08-22 14:00:00 +0800'
  touch_at "$BACKUP_ROOT/postgres/db_20260821_214315_b.sql.gz" '2026-08-21 14:00:00 +0800'
  touch_at "$BACKUP_ROOT/postgres/db_20260820_214315_c.sql" '2026-08-20 14:00:00 +0800'
  export BACKUP_ROOT
  if [ "${BACKUP_RETENTION_JSON:-1}" = "1" ]; then
    exec bash "$SCRIPT" --root "$BACKUP_ROOT" --now "$NOW" --json
  fi
  exec bash "$SCRIPT" --root "$BACKUP_ROOT" --now "$NOW"
fi

mkdir -p "$BACKUP_ROOT"/{daily,weekly,monthly,events,compose,logs,releases,release,storage-config,build,binaries,source}

# Deploy: seven snapshots, including old and new naming conventions.
touch_at "$BACKUP_ROOT/postgres/db_20260822_214315_b2084b737b.sql.gz" '2026-08-22 14:00:00 +0800'
touch_at "$BACKUP_ROOT/postgres/db_20260821_214315_a2084b737b.sql.gz" '2026-08-21 14:00:00 +0800'
touch_at "$BACKUP_ROOT/postgres/db_20260820_214315_c2084b737b.sql.gz" '2026-08-20 14:00:00 +0800'
touch_at "$BACKUP_ROOT/postgres/db_20260819_214315_d2084b737b.sql.gz" '2026-08-19 14:00:00 +0800'
touch_at "$BACKUP_ROOT/postgres/db_20260818_214315_e2084b737b.sql.gz" '2026-08-18 14:00:00 +0800'
touch_at "$BACKUP_ROOT/postgres/db_20260817_214315_f2084b737b.sql.gz" '2026-08-17 14:00:00 +0800'
touch_at "$BACKUP_ROOT/postgres/db_20260816_214315_g2084b737b.sql" '2026-08-16 14:00:00 +0800'
write_offsite_evidence "$BACKUP_ROOT/postgres/db_20260816_214315_g2084b737b.sql"

# Daily: recent and old files, with overlap candidates for weekly/monthly.
for day in 22 21 20 19 18 17 16 15 14 13 12 11 10 09; do
  touch_at "$BACKUP_ROOT/daily/xianzhi-202608${day}T010000.sql.gz" "2026-08-${day} 01:00:00 +0800"
done
touch_at "$BACKUP_ROOT/daily/xianzhi-20260822T010000.sql.gz" '2026-08-22 15:00:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-20260803T010000.sql.gz" '2026-08-03 01:00:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-20260727T010000.sql.gz" '2026-07-27 01:00:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-20260720T010000.sql.gz" '2026-07-20 01:00:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-20260713T010000.sql.gz" '2026-07-13 01:00:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-20260701T010000.sql" '2026-07-01 01:00:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-legacy.sql" '2026-07-01 00:30:00 +0800'
touch_at "$BACKUP_ROOT/daily/xianzhi-20260601T010000.sql.gz" '2026-06-01 01:00:00 +0800'

# Weekly and monthly source candidates.
touch_at "$BACKUP_ROOT/weekly/weekly-candidate-20260727.sql.gz" '2026-07-27 01:00:00 +0800'
touch_at "$BACKUP_ROOT/weekly/weekly-candidate-20260720.sql.gz" '2026-07-20 01:00:00 +0800'
touch_at "$BACKUP_ROOT/weekly/weekly-candidate-20260713.sql.gz" '2026-07-13 01:00:00 +0800'
touch_at "$BACKUP_ROOT/weekly/weekly-candidate-20260706.sql.gz" '2026-07-06 01:00:00 +0800'
touch_at "$BACKUP_ROOT/monthly/monthly-candidate-202608.sql.gz" '2026-08-01 01:00:00 +0800'
touch_at "$BACKUP_ROOT/monthly/monthly-candidate-202607.sql.gz" '2026-07-01 01:00:00 +0800'
touch_at "$BACKUP_ROOT/monthly/monthly-candidate-202606.sql.gz" '2026-06-01 01:00:00 +0800'

# Event files remain manual review regardless of age.
touch_at "$BACKUP_ROOT/events/pre-before-release.sql.gz" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/events/pre-before-release.dump" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/events/app-before-migration.csv" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/events/event archive.dump" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/events/manifest.json" '2020-01-01 00:00:00 +0800'

for i in $(seq -w 1 25); do touch_at "$BACKUP_ROOT/compose/compose.prod.yml.202608${i}.bak" "2026-08-${i} 02:00:00 +0800"; done
touch_at "$BACKUP_ROOT/logs/deploy-20260801.log" '2026-08-01 03:00:00 +0800'
touch_at "$BACKUP_ROOT/logs/deploy-20260701.log" '2026-07-01 03:00:00 +0800'
touch_at "$BACKUP_ROOT/releases/release-2020.dump" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/release/release-2020.dump" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/storage-config/config" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/build/build-output" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/binaries/tool" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/source/source.txt" '2020-01-01 00:00:00 +0800'
touch_at "$BACKUP_ROOT/.env.production.old bak" '2020-01-01 00:00:00 +0800'
printf 'outside\n' > "$WORKDIR/outside-target"
touch_at "$BACKUP_ROOT/odd name [old] --backup.sql" '2020-01-01 00:00:00 +0800'
if python3 -c 'import os, sys; os.symlink(sys.argv[1], sys.argv[2])' "$WORKDIR/outside-target" "$BACKUP_ROOT/symlink-outside.sql.gz" 2>/dev/null && test -L "$BACKUP_ROOT/symlink-outside.sql.gz"; then
  :
else
  printf '%s\n' 'SYMLINK_FIXTURE_UNAVAILABLE: test host denied symlink creation' >&2
fi

export BACKUP_ROOT
if [[ "$SCENARIO" == apply-* ]]; then
  export BACKUP_RETENTION_TEST_VERIFY=1
  exec bash "$SCRIPT" --root "$BACKUP_ROOT" --now "$NOW" --apply --max-count "${RETENTION_MAX_COUNT:-5}" --max-bytes "${RETENTION_MAX_BYTES:-1073741824}" --json
fi
if [ "${BACKUP_RETENTION_JSON:-1}" = "1" ]; then
  exec bash "$SCRIPT" --root "$BACKUP_ROOT" --now "$NOW" --json
fi
exec bash "$SCRIPT" --root "$BACKUP_ROOT" --now "$NOW"
