#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/backup-retention-dry-run.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT
BACKUP_ROOT="$WORKDIR/backups"
mkdir -p "$BACKUP_ROOT/postgres"
printf '%s\n' 'fixture' > "$BACKUP_ROOT/postgres/xianzhi-20260919.sql.gz"
touch -d '2026-09-19 00:00:00Z' "$BACKUP_ROOT/postgres/xianzhi-20260919.sql.gz"

BACKUP_RETENTION_SOURCE="$ROOT/ops/backup-retention.sh" \
BACKUP_ROOT="$BACKUP_ROOT" \
NOW_EPOCH='2026-09-19T12:00:00Z' \
  "$ROOT/ops/backup-retention-dry-run-report.sh"
