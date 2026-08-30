#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'
export PATH="/usr/bin:/bin:$PATH"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_ROOT=""
NOW=""
MAX_COUNT=""
MAX_BYTES=""
JSON=0

usage() {
  printf '%s\n' 'Usage: backup-retention-apply.sh --root PATH [--now EPOCH|ISO] --max-count N --max-bytes N [--json]' >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root) [ "$#" -ge 2 ] || { usage; exit 2; }; BACKUP_ROOT="$2"; shift 2 ;;
    --now) [ "$#" -ge 2 ] || { usage; exit 2; }; NOW="$2"; shift 2 ;;
    --max-count) [ "$#" -ge 2 ] || { usage; exit 2; }; MAX_COUNT="$2"; shift 2 ;;
    --max-bytes) [ "$#" -ge 2 ] || { usage; exit 2; }; MAX_BYTES="$2"; shift 2 ;;
    --json) JSON=1; shift ;;
    *) usage; exit 2 ;;
  esac
done

case "$MAX_COUNT" in ''|*[!0-9]*) printf '%s\n' 'INVALID_MAX_COUNT' >&2; exit 2 ;; esac
case "$MAX_BYTES" in ''|*[!0-9]*) printf '%s\n' 'INVALID_MAX_BYTES' >&2; exit 2 ;; esac
[ "$MAX_COUNT" -gt 0 ] || { printf '%s\n' 'MAX_COUNT_MUST_BE_POSITIVE' >&2; exit 2; }
[ "$MAX_COUNT" -le 5 ] || { printf '%s\n' 'MAX_COUNT_EXCEEDS_CONTROLLED_LIMIT' >&2; exit 2; }
[ "$MAX_BYTES" -gt 0 ] || { printf '%s\n' 'MAX_BYTES_MUST_BE_POSITIVE' >&2; exit 2; }
[ -n "$BACKUP_ROOT" ] || { usage; exit 2; }

PYTHON_BIN="$(command -v python3 || true)"
[ -n "$PYTHON_BIN" ] || { printf '%s\n' 'PYTHON3_REQUIRED' >&2; exit 1; }
LOCK_PATH="${BACKUP_RETENTION_LOCK_PATH:-${TMPDIR:-/tmp}/xianzhi-backup-retention.lock}"
command -v flock >/dev/null 2>&1 || { printf '%s\n' 'FLOCK_UNAVAILABLE' >&2; exit 1; }
REPORT_PATH="$(mktemp "${TMPDIR:-/tmp}/retention-report.XXXXXX")"
MANIFEST_PATH="$(mktemp "${TMPDIR:-/tmp}/retention-manifest.XXXXXX")"
AUDIT_PATH="$(mktemp "${TMPDIR:-/tmp}/retention-audit.XXXXXX")"
CURRENT_PATH=""
VERIFY_REMOTE_SCRIPT="${BACKUP_RETENTION_VERIFY_REMOTE_SCRIPT:-$ROOT/ops/backup-retention-verify-remote.sh}"
cleanup() {
  rm -f -- "$REPORT_PATH"
  rm -f -- "$MANIFEST_PATH"
  rm -f -- "$AUDIT_PATH"
  [ -z "$CURRENT_PATH" ] || rm -f -- "$CURRENT_PATH"
}
trap cleanup EXIT

exec 9>"$LOCK_PATH"
flock -n 9 || { printf '%s\n' 'RETENTION_LOCK_BUSY' >&2; exit 1; }

if ps -eo pid=,args= 2>/dev/null | awk -v self="$$" '$1 != self && $0 ~ /(^|[\047 \/])(backup-uploader|backup-retention\.sh|backfill)([ \047\/]|$)/ {found=1} END {exit found ? 0 : 1}'; then
  printf '%s\n' 'ACTIVE_RETENTION_DEPENDENCY' >&2
  exit 1
fi

DRY_ARGS=(--root "$BACKUP_ROOT" --json)
[ -n "$NOW" ] && DRY_ARGS+=(--now "$NOW")
bash "$ROOT/ops/backup-retention.sh" "${DRY_ARGS[@]}" >"$REPORT_PATH"

"$PYTHON_BIN" - "$REPORT_PATH" "$MANIFEST_PATH" "$MAX_COUNT" "$MAX_BYTES" <<'PY'
import json
import os
import sys

report_path, manifest_path, max_count, max_bytes = sys.argv[1:]
with open(report_path, "r") as handle:
    report = json.load(handle)
eligible = sorted(report.get("delete_eligible", []), key=lambda item: (item["mtime"], item["path"]))
selected = []
total = 0
for item in eligible:
    if len(selected) >= int(max_count):
        break
    if total + int(item["size"]) > int(max_bytes):
        continue
    required = (
        item.get("delete_eligible") is True,
        item.get("offsite_status") == "VERIFIED",
        item.get("offsite_verified") is True,
        item.get("remote_main_exists") is True,
        item.get("remote_size_match") is True,
        item.get("remote_sha256_match") is True,
        item.get("remote_meta_exists") is True,
        item.get("remote_meta_verified") is True,
        item.get("remote_sha_exists") is True,
        item.get("remote_sha_verified") is True,
        os.path.isabs(item.get("absolute_path", "")),
    )
    if not all(required):
        continue
    selected.append({"path": item["absolute_path"], "size": int(item["size"]), "sha256": item["sha256"], "remote_key": item["remote_key"], "retention_reason": item["retention_reason"]})
    total += int(item["size"])
with open(manifest_path, "w") as handle:
    json.dump({"count": len(selected), "bytes": total, "items": selected}, handle, sort_keys=True, separators=(",", ":"))
PY
chmod a-w "$MANIFEST_PATH"

"$PYTHON_BIN" - "$MANIFEST_PATH" <<'PY' >"$AUDIT_PATH"
import json
import sys
manifest = json.load(open(sys.argv[1], "r"))
for item in manifest["items"]:
    print("{}\t{}\t{}\t{}".format(item["path"], item["size"], item["sha256"], item["remote_key"]))
PY

DELETED_PATHS=()
while IFS=$'\t' read -r path size sha256 remote_key; do
  [ -n "$path" ] || continue
  CURRENT_PATH="$(mktemp "${TMPDIR:-/tmp}/retention-current.XXXXXX")"
  bash "$ROOT/ops/backup-retention.sh" "${DRY_ARGS[@]}" >"$CURRENT_PATH"
  "$PYTHON_BIN" - "$CURRENT_PATH" "$path" "$size" "$sha256" <<'PY'
import json
import sys
report = json.load(open(sys.argv[1], "r"))
path, size, sha256 = sys.argv[2:]
matches = [item for item in report.get("delete_eligible", []) if item.get("absolute_path") == path]
if len(matches) != 1:
    raise SystemExit("PRE_DELETE_RETENTION_NOT_ELIGIBLE")
item = matches[0]
if int(item.get("size", -1)) != int(size) or item.get("sha256") != sha256:
    raise SystemExit("PRE_DELETE_MANIFEST_MISMATCH")
PY
  rm -f -- "$CURRENT_PATH"
  CURRENT_PATH=""
  "$VERIFY_REMOTE_SCRIPT" "$remote_key" "$size" "$sha256" >/dev/null
  [ -f "$path" ] && [ ! -L "$path" ] || { printf 'PRE_DELETE_LOCAL_INVALID=%s\n' "$path" >&2; exit 1; }
  current_size="$(stat -c '%s' -- "$path")"
  [ "$current_size" = "$size" ] || { printf 'PRE_DELETE_SIZE_MISMATCH=%s\n' "$path" >&2; exit 1; }
  current_sha="$(sha256sum -- "$path" | awk '{print $1}')"
  [ "$current_sha" = "$sha256" ] || { printf 'PRE_DELETE_SHA256_MISMATCH=%s\n' "$path" >&2; exit 1; }
  rm -- "$path"
  [ ! -e "$path" ] || { printf 'LOCAL_EXISTS_AFTER=%s\n' "$path" >&2; exit 1; }
  "$VERIFY_REMOTE_SCRIPT" "$remote_key" "$size" "$sha256" >/dev/null
  DELETED_PATHS+=("$path")
done <"$AUDIT_PATH"

FINAL_REPORT="$(mktemp "${TMPDIR:-/tmp}/retention-final-report.XXXXXX")"
trap 'rm -f -- "$REPORT_PATH"; rm -f -- "$MANIFEST_PATH"; rm -f -- "$AUDIT_PATH"; rm -f -- "$FINAL_REPORT"' EXIT
bash "$ROOT/ops/backup-retention.sh" "${DRY_ARGS[@]}" >"$FINAL_REPORT"

"$PYTHON_BIN" - "$REPORT_PATH" "$FINAL_REPORT" "$MANIFEST_PATH" "${DELETED_PATHS[@]}" <<'PY'
import json
import os
import sys

before_path, after_path, manifest_path = sys.argv[1:4]
deleted = sys.argv[4:]
with open(before_path, "r") as handle:
    report = json.load(handle)
with open(after_path, "r") as handle:
    after_report = json.load(handle)
manifest = json.load(open(manifest_path, "r"))
report["apply"] = {
    "status": "PASS" if deleted else "NO_ELIGIBLE_WITHIN_LIMIT",
    "controlled_batch_count": len(deleted),
    "controlled_batch_size": sum(item["size"] for item in manifest["items"]),
    "manifest": manifest["items"],
    "deleted_paths": deleted,
    "delete_attempted_count": len(deleted),
    "delete_succeeded_count": len(deleted),
    "delete_failed_count": 0,
    "delete_freed_bytes": sum(item["size"] for item in manifest["items"]),
    "sidecars_deleted": 0,
    "obs_objects_deleted": 0,
    "obs_verified_objects_overwritten": 0,
    "post_delete_remote_verification": "PASS",
    "post_delete_summary": after_report.get("summary", {}),
}
print(json.dumps(report, ensure_ascii=False, sort_keys=True))
PY
