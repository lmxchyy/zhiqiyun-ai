#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'
export PATH="/usr/bin:/bin:$PATH"

PROJECT_DIR="${PROJECT_DIR:-/opt/zhiqiyun-ai}"
BACKUP_ROOT="${BACKUP_ROOT:-$PROJECT_DIR/backups}"
RETENTION_SOURCE="${BACKUP_RETENTION_SOURCE:-$PROJECT_DIR/ops/backup-retention.sh}"
NOW="${NOW_EPOCH:-}"

[ -x "$RETENTION_SOURCE" ] || { printf '%s\n' 'RETENTION_DRY_RUN_FAILED=SOURCE_NOT_EXECUTABLE' >&2; exit 1; }
[ -d "$BACKUP_ROOT" ] || { printf '%s\n' 'RETENTION_DRY_RUN_FAILED=BACKUP_ROOT_NOT_FOUND' >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { printf '%s\n' 'RETENTION_DRY_RUN_FAILED=PYTHON3_REQUIRED' >&2; exit 1; }

REPORT_PATH="$(mktemp "${TMPDIR:-/tmp}/xianzhi-retention-report.XXXXXX")"
cleanup() { rm -f -- "$REPORT_PATH"; }
trap cleanup EXIT

args=(--root "$BACKUP_ROOT" --json)
[ -n "$NOW" ] && args+=(--now "$NOW")
if ! bash "$RETENTION_SOURCE" "${args[@]}" >"$REPORT_PATH"; then
  printf '%s\n' 'RETENTION_DRY_RUN_FAILED=INVENTORY_FAILED' >&2
  exit 1
fi

python3 - "$REPORT_PATH" "$BACKUP_ROOT" <<'PY'
import json
import pathlib
import re
import sys

report_path, backup_root = sys.argv[1:]
try:
    report = json.load(open(report_path, "r", encoding="utf-8"))
except (OSError, ValueError) as exc:
    print(f"RETENTION_DRY_RUN_FAILED=REPORT_INVALID:{exc}", file=sys.stderr)
    raise SystemExit(1)

summary = report.get("summary") or {}
keep = report.get("keep") or []
protected = (report.get("manual_review") or []) + (report.get("analyze_only") or []) + (report.get("out_of_scope") or [])
all_entries = keep + (report.get("delete_candidates") or []) + protected
backup_re = re.compile(r"(?:^|/)(?:db_|xianzhi-).*\.sql(?:\.gz)?$")
backup_entries = [entry for entry in all_entries if backup_re.search(str(entry.get("path", "")))]
latest = max(backup_entries, key=lambda entry: (str(entry.get("mtime", "")), str(entry.get("path", ""))), default=None)
kept_paths = {str(entry.get("path", "")) for entry in keep}
latest_preserved = latest is not None and latest.get("path") in kept_paths

values = {
    "RETENTION_DRY_RUN": "PASS",
    "RETENTION_ROOT": backup_root,
    "RETENTION_TOTAL_COUNT": summary.get("total_files", 0),
    "RETENTION_TOTAL_BYTES": summary.get("total_bytes", 0),
    "RETENTION_PROTECTED_COUNT": len(protected),
    "RETENTION_PROTECTED_BYTES": sum(int(entry.get("size", 0)) for entry in protected),
    "RETENTION_CANDIDATE_COUNT": summary.get("delete_candidates_count", 0),
    "RETENTION_CANDIDATE_BYTES": summary.get("delete_candidates_bytes", 0),
    "RETENTION_DELETE_ELIGIBLE_COUNT": summary.get("delete_eligible_count", 0),
    "RETENTION_DELETE_ELIGIBLE_BYTES": summary.get("delete_eligible_bytes", 0),
    "RETENTION_LATEST_BACKUP_PRESERVED": "yes" if latest_preserved else "no",
}
for key, value in values.items():
    print(f"{key}={value}")

if backup_entries and not latest_preserved:
    print("RETENTION_DRY_RUN_FAILED=LATEST_BACKUP_NOT_PRESERVED", file=sys.stderr)
    raise SystemExit(1)
PY
