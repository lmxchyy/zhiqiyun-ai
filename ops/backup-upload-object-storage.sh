#!/usr/bin/env bash
set -euo pipefail
IFS='
	'
ROOT="${BACKUP_ROOT:-/opt/zhiqiyun-ai}"
FILE=""
PROVIDER="${BACKUP_OBJECT_PROVIDER:-cos}"
FAKE_ROOT="${BACKUP_OBJECT_FAKE_ROOT:-}"
MODE=dry-run
JSON_OUTPUT=0
while (($#)); do
  case "$1" in
    --file) FILE="$2"; shift 2;;
    --root) ROOT="$2"; shift 2;;
    --provider) PROVIDER="$2"; shift 2;;
    --fake-root) FAKE_ROOT="$2"; shift 2;;
    --dry-run) MODE=dry-run; shift;;
    --upload) MODE=upload; shift;;
    --json) JSON_OUTPUT=1; shift;;
    -h|--help) echo "usage: $0 --file PATH [--dry-run|--upload] [--provider cos|fake] [--root PATH] [--fake-root PATH] [--json]"; exit 0;;
    *) echo "unknown option: $1" >&2; exit 2;;
  esac
done
[[ -n "$FILE" ]] || { echo "missing --file" >&2; exit 2; }
PYTHON_BIN="${PYTHON_BIN:-}"
if [[ -z "$PYTHON_BIN" ]]; then
  command -v python3 >/dev/null 2>&1 && PYTHON_BIN="$(command -v python3)" || PYTHON_BIN="$(command -v python)"
fi
export OFFSITE_ROOT="$ROOT" OFFSITE_FILE="$FILE" OFFSITE_PROVIDER="$PROVIDER"
export OFFSITE_FAKE_ROOT="$FAKE_ROOT" OFFSITE_MODE="$MODE" OFFSITE_JSON_OUTPUT="$JSON_OUTPUT"
exec "$PYTHON_BIN" - <<'PY'
from __future__ import print_function
import datetime, gzip, hashlib, json, os, re, shutil, stat, sys

def out(data, code=0):
    if os.environ.get("OFFSITE_JSON_OUTPUT") == "1":
        print(json.dumps(data, sort_keys=True, separators=(",", ":")))
    else:
        print("{}: {}".format(data.get("status", "RESULT"), data.get("message", "")))
        for key in ("object_key", "offsite_path", "local_bytes", "remote_bytes", "local_sha256", "remote_sha256"):
            if key in data:
                print("{}: {}".format(key, data[key]))
    sys.exit(code)

def fail(status, message):
    out({"status": status, "message": message}, 1)

def inside(path, root):
    try:
        return os.path.commonpath([path, root]) == root
    except (AttributeError, ValueError):
        return path == root or path.startswith(root + os.sep)

def regular(path):
    try:
        mode = os.lstat(path).st_mode
    except OSError:
        return False
    return stat.S_ISREG(mode) and not stat.S_ISLNK(mode)

def local_hash(path):
    digest = hashlib.sha256()
    try:
        if path.endswith(".gz"):
            with gzip.open(path, "rb") as check:
                while check.read(1024 * 1024):
                    pass
        with open(path, "rb") as handle:
            while True:
                chunk = handle.read(1024 * 1024)
                if not chunk:
                    break
                digest.update(chunk)
    except Exception as exc:
        fail("LOCAL_BACKUP_INVALID", "backup content validation failed: {}".format(exc))
    return os.path.getsize(path), digest.hexdigest()

def load_local():
    root = os.path.realpath(os.environ["OFFSITE_ROOT"])
    if not os.path.isdir(root) or root in (os.path.realpath("/"), os.path.realpath("/opt"), os.path.realpath("/opt/zhiqiyun-ai")):
        fail("LOCAL_BACKUP_INVALID", "backup root is missing or too broad")
    supplied = os.path.abspath(os.environ["OFFSITE_FILE"])
    if not os.path.lexists(supplied) or not regular(supplied):
        fail("LOCAL_BACKUP_INVALID", "backup is missing, not regular, or is a symlink")
    path = os.path.realpath(supplied)
    if not inside(path, root) or path == root:
        fail("LOCAL_BACKUP_INVALID", "backup is outside backup root")
    meta = path + ".meta.json"
    if not regular(meta):
        fail("LOCAL_BACKUP_INVALID", "backup metadata is missing or unsafe")
    try:
        with open(meta, "r") as handle:
            metadata = json.load(handle)
        declared_bytes = int(metadata["bytes"])
        declared_sha = str(metadata["sha256"])
    except (IOError, ValueError, KeyError, TypeError):
        fail("LOCAL_BACKUP_INVALID", "backup metadata is invalid")
    size, digest = local_hash(path)
    actual = os.path.getsize(path)
    if size != actual or declared_bytes != actual:
        fail("LOCAL_BACKUP_INVALID", "backup byte count does not match metadata")
    if declared_sha != digest:
        fail("LOCAL_BACKUP_INVALID", "backup sha256 does not match metadata")
    name = os.path.basename(path)
    relative = os.path.relpath(path, root).replace(os.sep, "/")
    if not (relative.startswith("postgres/") and name.startswith("db_") and (name.endswith(".sql") or name.endswith(".sql.gz"))):
        if not re.match(r"^xianzhi-.*\.sql(?:\.gz)?$", name):
            fail("LOCAL_BACKUP_INVALID", "unsupported backup category")
        category = "daily"
    else:
        category = "deploy"
    if not name or name in (".", "..") or "/" in name or "\\" in name or ".." in name or any(ord(ch) < 32 for ch in name):
        fail("LOCAL_BACKUP_INVALID", "unsafe backup filename")
    stamp = datetime.datetime.utcfromtimestamp(os.path.getmtime(path))
    prefix = "postgres/" if category == "deploy" else ""
    key = "zhiqiyun-ai/{}{}/{:04d}/{:02d}/{}".format(prefix, category, stamp.year, stamp.month, name)
    return {"root": root, "path": path, "meta": meta, "category": category, "object_key": key, "bytes": actual, "sha256": digest}

class FakeProvider(object):
    def __init__(self, root):
        if not root:
            fail("UPLOAD_NOT_CONFIGURED", "fake provider requires --fake-root")
        self.root = os.path.realpath(root)
        if not os.path.isdir(self.root):
            os.makedirs(self.root)
    def path(self, key):
        target = os.path.realpath(os.path.join(self.root, *key.split("/")))
        if not inside(target, self.root):
            fail("LOCAL_BACKUP_INVALID", "object key escaped provider root")
        return target
    def info_path(self, key):
        return self.path(key) + ".object-meta.json"
    def head(self, key):
        target = self.path(key)
        if not os.path.isfile(target):
            return None
        try:
            with open(self.info_path(key), "r") as handle:
                info = json.load(handle)
        except (IOError, ValueError):
            fail("REMOTE_CONFLICT", "remote metadata is unreadable")
        if os.environ.get("BACKUP_OBJECT_FAKE_HEAD_SIZE_DELTA"):
            info["size"] = int(info["size"]) + int(os.environ["BACKUP_OBJECT_FAKE_HEAD_SIZE_DELTA"])
        if os.environ.get("BACKUP_OBJECT_FAKE_HEAD_SHA256"):
            info["sha256"] = os.environ["BACKUP_OBJECT_FAKE_HEAD_SHA256"]
        return info
    def put(self, source, key, sha256_value, size):
        target = self.path(key)
        parent = os.path.dirname(target)
        if not os.path.isdir(parent):
            os.makedirs(parent)
        shutil.copyfile(source, target)
        with open(self.info_path(key), "w") as handle:
            json.dump({"size": size, "sha256": sha256_value, "etag": hashlib.md5(open(source, "rb").read()).hexdigest()}, handle)
    def put_text(self, content, key):
        target = self.path(key)
        parent = os.path.dirname(target)
        if not os.path.isdir(parent):
            os.makedirs(parent)
        with open(target, "w") as handle:
            handle.write(content)
        with open(self.info_path(key), "w") as handle:
            json.dump({"size": len(content.encode("utf-8")), "sha256": hashlib.sha256(content.encode("utf-8")).hexdigest(), "etag": hashlib.md5(content.encode("utf-8")).hexdigest()}, handle)

def verify(provider, key, local):
    remote = provider.head(key)
    if remote is None:
        fail("REMOTE_SIZE_MISMATCH", "remote object is missing after upload")
    if int(remote.get("size", -1)) != local["bytes"]:
        fail("REMOTE_SIZE_MISMATCH", "remote size does not match local backup")
    if remote.get("sha256") != local["sha256"]:
        fail("REMOTE_CHECKSUM_MISMATCH", "remote sha256 metadata does not match local backup")
    return remote

local = load_local()
provider_name = os.environ["OFFSITE_PROVIDER"]
if os.environ["OFFSITE_MODE"] == "dry-run":
    out({"status": "DRY_RUN", "provider": provider_name, "object_key": local["object_key"], "uploaded": False, "local_bytes": local["bytes"], "local_sha256": local["sha256"]})
if provider_name == "cos":
    fail("UPLOAD_NOT_CONFIGURED", "COS provider is configuration-gated in Phase 3A")
if provider_name != "fake":
    fail("UPLOAD_NOT_CONFIGURED", "provider is not configured")
provider = FakeProvider(os.environ.get("OFFSITE_FAKE_ROOT", ""))
key = local["object_key"]
remote = provider.head(key)
if remote is not None:
    if int(remote.get("size", -1)) == local["bytes"] and remote.get("sha256") == local["sha256"]:
        out({"status": "ALREADY_OFFSITE_VERIFIED", "verification": "OFFSITE_VERIFIED", "object_key": key, "local_bytes": local["bytes"], "remote_bytes": int(remote["size"]), "local_sha256": local["sha256"], "remote_sha256": remote["sha256"]})
    fail("REMOTE_CONFLICT", "remote object exists with different size or checksum")
provider.put(local["path"], key, local["sha256"], local["bytes"])
remote = verify(provider, key, local)
with open(local["meta"], "r") as handle:
    meta_content = handle.read()
provider.put_text(meta_content, key + ".meta.json")
meta_remote = provider.head(key + ".meta.json")
if meta_remote is None or int(meta_remote.get("size", -1)) != len(meta_content.encode("utf-8")):
    fail("REMOTE_SIZE_MISMATCH", "remote metadata object could not be verified")
if meta_remote.get("sha256") != hashlib.sha256(meta_content.encode("utf-8")).hexdigest():
    fail("REMOTE_CHECKSUM_MISMATCH", "remote metadata checksum could not be verified")
checksum_content = local["sha256"] + "  " + os.path.basename(local["path"]) + "\n"
provider.put_text(checksum_content, key + ".sha256")
checksum_remote = provider.head(key + ".sha256")
if checksum_remote is None or int(checksum_remote.get("size", -1)) != len(checksum_content.encode("utf-8")):
    fail("REMOTE_SIZE_MISMATCH", "remote sha256 object could not be verified")
payload = {"version": 1, "provider": provider_name, "bucket": os.environ.get("BACKUP_OBJECT_BUCKET", "fake"), "object_key": key, "uploaded_at": datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"), "local_bytes": local["bytes"], "local_sha256": local["sha256"], "remote_bytes": int(remote["size"]), "remote_etag": remote.get("etag", ""), "remote_sha256": remote["sha256"], "verification": "OFFSITE_VERIFIED"}
offsite_path = local["path"] + ".offsite.json"
with open(offsite_path, "w") as handle:
    json.dump(payload, handle, sort_keys=True, indent=2)
    handle.write("\n")
payload.update({"status": "OFFSITE_VERIFIED", "offsite_path": offsite_path, "uploaded": True})
out(payload)
PY
