#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'
export PATH="/usr/bin:/bin:$PATH"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_ROOT="${BACKUP_ROOT:-/opt/zhiqiyun-ai/backups}"
NOW="${NOW_EPOCH:-}"
JSON=0
APPLY=0

usage() {
  printf '%s\n' 'Usage: backup-retention.sh [--root PATH] [--now EPOCH|ISO] [--json] [--dry-run] [--apply]' >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root) [ "$#" -ge 2 ] || { usage; exit 2; }; BACKUP_ROOT="$2"; shift 2 ;;
    --now) [ "$#" -ge 2 ] || { usage; exit 2; }; NOW="$2"; shift 2 ;;
    --json) JSON=1; shift ;;
    --dry-run) shift ;;
    --apply) APPLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

if [ "$APPLY" -eq 1 ]; then
  printf '%s\n' 'APPLY_NOT_IMPLEMENTED' >&2
  exit 2
fi

PYTHON_BIN=""
if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN="python3"
elif command -v python.exe >/dev/null 2>&1 && python.exe -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN="python.exe"
fi
if [ -z "$PYTHON_BIN" ]; then
  printf '%s\n' 'ERROR: python3 is required for portable timestamp and JSON handling.' >&2
  exit 1
fi

export RETENTION_BACKUP_ROOT="$BACKUP_ROOT"
export RETENTION_NOW="$NOW"
export RETENTION_JSON="$JSON"
exec "$PYTHON_BIN" - "$ROOT" <<'PY'
import datetime as dt
import json
import os
import pathlib
import re
import stat
import sys
import tempfile

script_root = pathlib.Path(sys.argv[1])
root_arg = os.environ["RETENTION_BACKUP_ROOT"]
json_mode = os.environ["RETENTION_JSON"] == "1"
now_arg = os.environ.get("RETENTION_NOW", "")

def fail(message):
    print("RETENTION_SAFETY_CHECK_FAILED", file=sys.stderr)
    print(message, file=sys.stderr)
    raise SystemExit(1)

try:
    root = pathlib.Path(root_arg).resolve(strict=True)
except (FileNotFoundError, OSError):
    print(f"ERROR: backup root does not exist: {root_arg}", file=sys.stderr)
    raise SystemExit(1)
if not root.is_dir():
    print(f"ERROR: backup root is not a directory: {root}", file=sys.stderr)
    raise SystemExit(1)

def is_within(path, parent):
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False

if root in {pathlib.Path("/"), pathlib.Path("/opt"), pathlib.Path("/opt/zhiqiyun-ai")}:
    print(f"ERROR: refusing broad backup root: {root}", file=sys.stderr)
    raise SystemExit(1)
temp_root = pathlib.Path(tempfile.gettempdir()).resolve()
production_root = pathlib.Path("/opt/zhiqiyun-ai/backups").resolve()
if root == temp_root or (root != production_root and not is_within(root, temp_root)):
    print(f"ERROR: root must be an explicit backups directory or test temporary directory: {root}", file=sys.stderr)
    raise SystemExit(1)

def parse_now(value):
    if not value:
        return int(dt.datetime.now(dt.timezone.utc).timestamp())
    if re.fullmatch(r"[0-9]+", value):
        return int(value)
    text = value.replace("Z", "+00:00")
    parsed = dt.datetime.fromisoformat(text)
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=dt.timezone.utc)
    return int(parsed.timestamp())

now = parse_now(now_arg)
now_dt = dt.datetime.fromtimestamp(now, dt.timezone.utc)

def relative(path):
    try:
        return path.relative_to(root).as_posix()
    except ValueError:
        fail(f"entry escaped backup root: {path}")

def inside(path):
    return is_within(path, root)

files = []
warnings = []
pending = [root]
while pending:
    current_path = pending.pop()
    try:
        entries = sorted(os.scandir(current_path), key=lambda entry: entry.name)
    except (FileNotFoundError, OSError):
        warnings.append(f"UNSAFE_ENTRY: {relative(current_path)}")
        continue
    for entry in entries:
        path = pathlib.Path(entry.path)
        rel = relative(path)
        try:
            info = os.lstat(entry.path)
        except (FileNotFoundError, OSError):
            warnings.append(f"UNSAFE_ENTRY: {rel}")
            continue
        if stat.S_ISLNK(info.st_mode) or entry.is_symlink():
            warnings.append(f"SKIP_SYMLINK: {rel}")
            continue
        if stat.S_ISDIR(info.st_mode):
            pending.append(path)
            continue
        if not stat.S_ISREG(info.st_mode):
            warnings.append(f"UNSAFE_ENTRY: {rel}")
            continue
        try:
            resolved = path.resolve(strict=True)
            if not inside(resolved):
                fail(f"inventory entry escaped backup root: {path}")
            files.append({"path": str(path), "relative_path": rel, "size": info.st_size, "mtime": int(info.st_mtime)})
        except (FileNotFoundError, OSError):
            warnings.append(f"UNSAFE_ENTRY: {rel}")

def item(file, category, reason):
    stamp = dt.datetime.fromtimestamp(file["mtime"], dt.timezone.utc).isoformat()
    return {"path": file["relative_path"], "size": file["size"], "mtime": stamp, "category": category, "keep_reason": reason, "reason": reason}

deploy_re = re.compile(r"^db_.*\.sql(?:\.gz)?$")
daily_re = re.compile(r"^xianzhi-.*\.sql(?:\.gz)?$")
compose_re = re.compile(r"^compose\.prod\.yml\..+\.bak$")
deploy_logs = re.compile(r"(?:^|/)deploy[^/]*\.log$")

keep = {}
delete = {}
manual = {}
analyze = {}
out_scope = {}
all_files = {f["relative_path"]: f for f in files}

def add_keep(file, category, reason):
    path = file["relative_path"]
    if path in delete:
        del delete[path]
    if path in keep:
        keep[path]["keep_reason"] = f'{keep[path]["keep_reason"]}; {reason}'
    else:
        keep[path] = item(file, category, reason)

def add_delete(file, category, reason):
    path = file["relative_path"]
    if path not in keep and path not in manual and path not in analyze and path not in out_scope:
        delete[path] = item(file, category, reason)

for file in files:
    rel = file["relative_path"]
    parts = rel.split("/")
    name = parts[-1]
    if parts[0] in {"releases", "release"}:
        analyze[rel] = item(file, "release", "ANALYZE_ONLY release backup")
    elif name.startswith("pre-") or "-before-" in name or "before-" in name or name.lower().startswith("manifest") or name.lower().endswith((".dump", ".csv")) or "event" in name.lower() or parts[0].lower() in {"events", "event"}:
        manual[rel] = item(file, "event", "MANUAL_REVIEW event backup")
    elif parts[0] in {"storage-config", "build", "binaries", "source"} or name.startswith(".env.production."):
        out_scope[rel] = item(file, "out_of_scope", "OUT_OF_SCOPE_KEEP")

def is_protected(file):
    path = file["relative_path"]
    return path in manual or path in analyze or path in out_scope

deploy = sorted((f for f in files if not is_protected(f) and f["relative_path"].split("/")[0] == "postgres" and deploy_re.match(pathlib.Path(f["relative_path"]).name)), key=lambda f: (-f["mtime"], f["relative_path"]))
for index, file in enumerate(deploy):
    if index < 5:
        add_keep(file, "deploy", "recent deploy backup (top 5)")
    else:
        add_delete(file, "deploy", "deploy backup older than top 5")

daily_files = [f for f in files if not is_protected(f) and daily_re.match(pathlib.Path(f["relative_path"]).name)]
pool = daily_files + deploy
for file in daily_files:
    if file["mtime"] >= now - 14 * 86400:
        add_keep(file, "daily", "daily backup within 14 days")

def week_key(epoch):
    return dt.datetime.fromtimestamp(epoch, dt.timezone.utc).isocalendar()[:2]

def month_key(epoch):
    value = dt.datetime.fromtimestamp(epoch, dt.timezone.utc)
    return value.year, value.month

current_week = now_dt.isocalendar()
weeks = []
for offset in range(4):
    date = now_dt.date() - dt.timedelta(days=now_dt.weekday() + offset * 7)
    weeks.append((date.isocalendar().year, date.isocalendar().week))
for week in weeks:
    candidates = sorted((f for f in pool if week_key(f["mtime"]) == week), key=lambda f: (-f["mtime"], f["relative_path"]))
    if candidates:
        preferred = next((f for f in candidates if f["relative_path"] in keep), candidates[0])
        add_keep(preferred, "weekly", f"weekly coverage {week[0]}-W{week[1]:02d}")

months = []
for offset in range(3):
    month = now_dt.month - offset
    year = now_dt.year
    while month <= 0:
        month += 12
        year -= 1
    months.append((year, month))
for month in months:
    candidates = sorted((f for f in pool if month_key(f["mtime"]) == month), key=lambda f: (-f["mtime"], f["relative_path"]))
    if candidates:
        preferred = next((f for f in candidates if f["relative_path"] in keep), candidates[0])
        add_keep(preferred, "monthly", f"monthly coverage {month[0]}-{month[1]:02d}")

compose_files = sorted((f for f in files if not is_protected(f) and compose_re.match(pathlib.Path(f["relative_path"]).name)), key=lambda f: (-f["mtime"], f["relative_path"]))
for index, file in enumerate(compose_files):
    (add_keep if index < 20 else add_delete)(file, "compose", "recent compose backup (top 20)" if index < 20 else "compose backup older than top 20")

for file in files:
    rel = file["relative_path"]
    if not is_protected(file) and deploy_logs.search(rel):
        if file["mtime"] >= now - 30 * 86400:
            add_keep(file, "deploy_log", "deploy log within 30 days")
        else:
            add_delete(file, "deploy_log", "deploy log older than 30 days")

for file in daily_files:
    if file["relative_path"] not in keep:
        add_delete(file, "daily", "daily backup outside 14 days and not required for weekly/monthly coverage")

for file in files:
    rel = file["relative_path"]
    if rel not in keep and rel not in delete and rel not in manual and rel not in analyze and rel not in out_scope:
        out_scope[rel] = item(file, "out_of_scope", "OUT_OF_SCOPE_KEEP")

for week in weeks:
    candidates = [f for f in pool if week_key(f["mtime"]) == week]
    if candidates and not any(f["relative_path"] in keep for f in candidates):
        fail(f"weekly coverage missing for {week[0]}-W{week[1]:02d}")
for month in months:
    candidates = [f for f in pool if month_key(f["mtime"]) == month]
    if candidates and not any(f["relative_path"] in keep for f in candidates):
        fail(f"monthly coverage missing for {month[0]}-{month[1]:02d}")
if any(file["mtime"] >= now - 14 * 86400 and file["relative_path"] not in keep for file in daily_files):
    fail("daily backup inside 14-day window was not kept")

keep_list = sorted(keep.values(), key=lambda x: x["path"])
delete_list = sorted(delete.values(), key=lambda x: x["path"])
manual_list = sorted(manual.values(), key=lambda x: x["path"])
analyze_list = sorted(analyze.values(), key=lambda x: x["path"])
out_scope_list = sorted(out_scope.values(), key=lambda x: x["path"])

for path in set(keep) & set(delete):
    fail(f"path in KEEP and DELETE_CANDIDATE: {path}")
for entry in delete_list:
    candidate = root / entry["path"]
    try:
        if not inside(candidate.resolve(strict=True)) or not stat.S_ISREG(os.lstat(candidate).st_mode):
            fail(f"delete candidate is not a regular file inside backup root: {entry['path']}")
    except (FileNotFoundError, OSError):
        fail(f"delete candidate is not a stable file inside backup root: {entry['path']}")
if any(entry["path"].split("/")[0] in {"releases", "release"} for entry in delete_list):
    fail("release entered delete candidates")
if any(entry["category"] == "event" for entry in delete_list):
    fail("event entered delete candidates")
if len(deploy) >= 5 and sum(1 for entry in keep_list if entry["category"] == "deploy") < 5:
    fail("deploy retention below five")
if len(deploy) < 5 and any(entry["path"].split("/")[0] == "postgres" for entry in delete_list):
    fail("insufficient deploy set entered delete candidates")

def totals(entries):
    return {"count": len(entries), "bytes": sum(entry["size"] for entry in entries)}

def human_size(value):
    units = ("B", "KiB", "MiB", "GiB", "TiB")
    amount = float(value)
    for unit in units:
        if amount < 1024 or unit == units[-1]:
            return f"{amount:.1f} {unit}" if unit != "B" else f"{int(amount)} B"
        amount /= 1024

summary = {
    "total_files": len(files),
    "total_bytes": sum(f["size"] for f in files),
    "keep_count": len(keep_list), "keep_bytes": sum(x["size"] for x in keep_list),
    "delete_candidates_count": len(delete_list), "delete_candidates_bytes": sum(x["size"] for x in delete_list),
    "manual_review_count": len(manual_list), "manual_review_bytes": sum(x["size"] for x in manual_list),
    "analyze_only_count": len(analyze_list), "analyze_only_bytes": sum(x["size"] for x in analyze_list),
    "out_of_scope_count": len(out_scope_list), "out_of_scope_bytes": sum(x["size"] for x in out_scope_list),
}
report = {
    "summary": summary,
    "keep": keep_list,
    "delete_candidates": delete_list,
    "manual_review": manual_list,
    "analyze_only": analyze_list,
    "out_of_scope": out_scope_list,
    "expected_reclaimed_bytes": summary["delete_candidates_bytes"],
    "warnings": sorted(set(warnings)),
}

if json_mode:
    print(json.dumps(report, ensure_ascii=False, sort_keys=True))
else:
    print("# Backup Retention Dry Run\n")
    print("## Summary\n")
    for key, value in summary.items():
        print(f"{key}: {value}")
    print("\n## KEEP\n")
    for entry in keep_list:
        print(f"{entry['path']}\t{entry['size']}\t{entry['mtime']}\t{entry['category']}\t{entry['keep_reason']}")
    print("\n## DELETE CANDIDATES\n")
    for entry in delete_list:
        print(f"{entry['path']}\t{entry['size']}\t{entry['mtime']}\t{entry['category']}\t{entry['reason']}")
    print("\n## MANUAL REVIEW\n")
    for entry in manual_list: print(entry["path"])
    print("\n## ANALYZE ONLY\n")
    for entry in analyze_list: print(entry["path"])
    print("\n## OUT OF SCOPE\n")
    for entry in out_scope_list:
        print(f"{entry['path']}\t{entry['size']}\t{entry['mtime']}\t{entry['category']}\t{entry['keep_reason']}")
    print(f"\n## Expected Reclaimed Space\n\nbytes: {summary['delete_candidates_bytes']}\nhuman: {human_size(summary['delete_candidates_bytes'])}")
PY
