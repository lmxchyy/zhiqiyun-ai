#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${BACKUP_RETENTION_SCHEDULER_SRC:-$ROOT/ops/backup-retention-scheduler.sh}"
SCENARIO="${1:-disabled}"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/retention-automation.XXXXXX")"
LOG="$WORKDIR/events.log"

write_stub() {
  local path="$1" event="$2" exit_code="$3"
  cat > "$path" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '%s %s\\n' '$event' "\$*" >> '$LOG'
if [ '$event' = apply ]; then
  printf '%s\\n' OBS_DELETE_REQUESTED OBS_OVERWRITE_REQUESTED >> '$LOG'
fi
exit $exit_code
EOF
  chmod +x "$path"
}

offsite_code=0
apply_code=0
case "$SCENARIO" in
  upstream-failure) offsite_code=7 ;;
  apply-failure) apply_code=9 ;;
esac
write_stub "$WORKDIR/offsite.sh" offsite "$offsite_code"
write_stub "$WORKDIR/apply.sh" apply "$apply_code"
write_stub "$WORKDIR/flock.sh" flock 0

export BACKUP_RETENTION_OFFSITE_COMMAND="$WORKDIR/offsite.sh"
export BACKUP_RETENTION_APPLY_COMMAND="$WORKDIR/apply.sh"
export BACKUP_RETENTION_EVENT_LOG="$LOG"
export BACKUP_RETENTION_SCHEDULER_LOCK_PATH="$WORKDIR/scheduler.lock"
export BACKUP_RETENTION_FLOCK_COMMAND="$WORKDIR/flock.sh"
export RETENTION_AUTO_APPLY_ENABLED="${RETENTION_AUTO_APPLY_ENABLED:-false}"
export RETENTION_AUTO_MAX_COUNT="${RETENTION_AUTO_MAX_COUNT:-4}"
export RETENTION_AUTO_MAX_BYTES="${RETENTION_AUTO_MAX_BYTES:-1073741824}"

set +e
bash "$SCRIPT" > "$WORKDIR/result.out" 2> "$WORKDIR/result.err"
status=$?
set -e

python3 - "$WORKDIR/result.out" "$WORKDIR/result.err" "$LOG" "$status" <<'PY'
import json
import sys

stdout_path, stderr_path, log_path, status = sys.argv[1:]
stdout = open(stdout_path).read()
stderr = open(stderr_path).read()
events = open(log_path).read().splitlines() if __import__('os').path.exists(log_path) else []
print(json.dumps({"status": int(status), "stdout": stdout, "stderr": stderr, "events": events}, sort_keys=True))
PY
