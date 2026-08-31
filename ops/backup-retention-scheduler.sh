#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'
export PATH="/usr/bin:/bin:$PATH"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OFFSITE_COMMAND="${BACKUP_RETENTION_OFFSITE_COMMAND:-$ROOT/ops/backup-offsite-upload-pending.sh}"
APPLY_COMMAND="${BACKUP_RETENTION_APPLY_COMMAND:-$ROOT/ops/backup-retention.sh}"
LOCK_PATH="${BACKUP_RETENTION_SCHEDULER_LOCK_PATH:-/var/lock/xianzhi-backup-scheduler.lock}"
FLOCK_COMMAND="${BACKUP_RETENTION_FLOCK_COMMAND:-flock}"
ENABLED="${RETENTION_AUTO_APPLY_ENABLED:-false}"
MAX_COUNT="${RETENTION_AUTO_MAX_COUNT:-4}"
MAX_BYTES="${RETENTION_AUTO_MAX_BYTES:-1073741824}"

fail() {
  printf '%s\n' "RETENTION_SCHEDULER_FAILED=$1" >&2
  exit 1
}

case "$ENABLED" in
  true|false) ;;
  *) fail 'RETENTION_AUTO_APPLY_ENABLED_MUST_BE_TRUE_OR_FALSE' ;;
esac

case "$MAX_COUNT" in ''|*[!0-9]*) fail 'RETENTION_AUTO_MAX_COUNT_INVALID' ;; esac
case "$MAX_BYTES" in ''|*[!0-9]*) fail 'RETENTION_AUTO_MAX_BYTES_INVALID' ;; esac
[ "$MAX_COUNT" -gt 0 ] || fail 'RETENTION_AUTO_MAX_COUNT_MUST_BE_POSITIVE'
[ "$MAX_COUNT" -le 5 ] || fail 'RETENTION_AUTO_MAX_COUNT_EXCEEDS_FIVE'
[ "$MAX_BYTES" -gt 0 ] || fail 'RETENTION_AUTO_MAX_BYTES_MUST_BE_POSITIVE'
[ "$MAX_BYTES" -le 1073741824 ] || fail 'RETENTION_AUTO_MAX_BYTES_EXCEEDS_ONE_GIB'
[ -x "$OFFSITE_COMMAND" ] || fail 'OFFSITE_COMMAND_NOT_EXECUTABLE'
[ -x "$APPLY_COMMAND" ] || fail 'APPLY_COMMAND_NOT_EXECUTABLE'

command -v "$FLOCK_COMMAND" >/dev/null 2>&1 || fail 'FLOCK_UNAVAILABLE'
exec 9>"$LOCK_PATH"
"$FLOCK_COMMAND" -n 9 || fail 'RETENTION_SCHEDULER_LOCK_BUSY'

printf '%s\n' 'RETENTION_SCHEDULER_PHASE=OFFSITE'
if ! "$OFFSITE_COMMAND"; then
  printf '%s\n' 'RETENTION_APPLY=NOT_RUN' >&2
  exit 1
fi

if [ "$ENABLED" = false ]; then
  printf '%s\n' 'RETENTION_AUTO_APPLY_ENABLED=false' 'RETENTION_APPLY=NOT_RUN'
  exit 0
fi

printf '%s\n' 'RETENTION_AUTO_APPLY_ENABLED=true' 'RETENTION_SCHEDULER_PHASE=RETENTION'
if "$APPLY_COMMAND" --apply --max-count "$MAX_COUNT" --max-bytes "$MAX_BYTES" --json; then
  printf '%s\n' 'RETENTION_APPLY=PASS'
else
  printf '%s\n' 'RETENTION_APPLY=FAILED' >&2
  exit 1
fi
