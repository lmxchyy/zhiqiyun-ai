#!/usr/bin/env bash
set -euo pipefail
IFS='
	'
ROOT="${BACKUP_ROOT:-/opt/zhiqiyun-ai}"
FILE=""
PROVIDER="${BACKUP_OBJECT_PROVIDER:-cos}"
STORAGE_CONFIG_ID="${BACKUP_STORAGE_CONFIG_ID:-}"
FAKE_ROOT="${BACKUP_OBJECT_FAKE_ROOT:-}"
MODE=dry-run
JSON_OUTPUT=0
DOWNLOAD_TO=""
VERIFY_REMOTE_KEY=""
VERIFY_EXPECTED_SIZE=""
VERIFY_EXPECTED_SHA256=""
while (($#)); do
  case "$1" in
    --file) FILE="$2"; shift 2;;
    --root) ROOT="$2"; shift 2;;
    --provider) PROVIDER="$2"; shift 2;;
    --storage-config-id) STORAGE_CONFIG_ID="$2"; shift 2;;
    --download-to) DOWNLOAD_TO="$2"; shift 2;;
    --fake-root) FAKE_ROOT="$2"; shift 2;;
    --dry-run) MODE=dry-run; shift;;
    --upload) MODE=upload; shift;;
    --verify-only) MODE=verify-only; shift;;
    --remote-key) VERIFY_REMOTE_KEY="$2"; shift 2;;
    --expected-size) VERIFY_EXPECTED_SIZE="$2"; shift 2;;
    --expected-sha256) VERIFY_EXPECTED_SHA256="$2"; shift 2;;
    --json) JSON_OUTPUT=1; shift;;
    -h|--help) echo "usage: $0 --file PATH [--dry-run|--upload] [--provider obs|cos|fake] [--storage-config-id ID] [--download-to PATH] [--root PATH] [--fake-root PATH] [--json]"; exit 0;;
    *) echo "unknown option: $1" >&2; exit 2;;
  esac
done
if [[ "$MODE" == "verify-only" ]]; then
  [[ -n "$VERIFY_REMOTE_KEY" && -n "$VERIFY_EXPECTED_SIZE" && -n "$VERIFY_EXPECTED_SHA256" ]] || { echo "missing remote verification arguments" >&2; exit 2; }
elif [[ -z "$FILE" ]]; then
  echo "missing --file" >&2
  exit 2
fi
if [[ -n "$STORAGE_CONFIG_ID" ]]; then
  [[ "$PROVIDER" == "obs" ]] || { echo "BACKUP_STORAGE_CONFIG_NOT_FOUND: database backup config requires --provider obs" >&2; exit 1; }
  DB_UPLOADER_BIN="${BACKUP_DB_UPLOADER_BIN:-/usr/local/bin/backup-uploader-db}"
  DB_ARGS=(--root "$ROOT" --storage-config-id "$STORAGE_CONFIG_ID")
  if [[ "$MODE" == "verify-only" ]]; then
    DB_ARGS+=(--verify-only --remote-key "$VERIFY_REMOTE_KEY" --expected-size "$VERIFY_EXPECTED_SIZE" --expected-sha256 "$VERIFY_EXPECTED_SHA256")
  else
    DB_ARGS+=(--file "$FILE")
  fi
  [[ "$MODE" == "upload" ]] && DB_ARGS+=(--upload)
  [[ "$JSON_OUTPUT" == "1" ]] && DB_ARGS+=(--json)
  [[ -n "$DOWNLOAD_TO" ]] && DB_ARGS+=(--download-to "$DOWNLOAD_TO")
  exec "$DB_UPLOADER_BIN" "${DB_ARGS[@]}"
fi
PYTHON_BIN="${PYTHON_BIN:-}"
if [[ -z "$PYTHON_BIN" ]]; then
  command -v python3 >/dev/null 2>&1 && PYTHON_BIN="$(command -v python3)" || PYTHON_BIN="$(command -v python)"
fi
export OFFSITE_ROOT="$ROOT" OFFSITE_FILE="$FILE" OFFSITE_PROVIDER="$PROVIDER"
export OFFSITE_FAKE_ROOT="$FAKE_ROOT" OFFSITE_MODE="$MODE" OFFSITE_JSON_OUTPUT="$JSON_OUTPUT"
exec "$PYTHON_BIN" - <<'PY'
from __future__ import print_function
import datetime, gzip, hashlib, io, json, os, re, shutil, stat, sys, tempfile, time

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
    prefix = "backups/postgres/"
    key = "{}{}/{:04d}/{:02d}/{}".format(prefix, category, stamp.year, stamp.month, name)
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
        failure = os.environ.get("BACKUP_OBJECT_FAKE_FAILURE", "")
        if failure in ("auth-failure", "timeout", "network-failure"):
            fail("OFFSITE_UPLOAD_FAILED", "OBS request failed")
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
        failure = os.environ.get("BACKUP_OBJECT_FAKE_FAILURE", "")
        if failure in ("partial-upload", "auth-failure", "timeout", "network-failure"):
            fail("OFFSITE_UPLOAD_FAILED", "OBS upload failed")
        target = self.path(key)
        parent = os.path.dirname(target)
        if not os.path.isdir(parent):
            os.makedirs(parent)
        shutil.copyfile(source, target)
        with open(self.info_path(key), "w") as handle:
            json.dump({"size": size, "sha256": sha256_value, "etag": hashlib.md5(open(source, "rb").read()).hexdigest()}, handle)
    def put_text(self, content, key):
        failure = os.environ.get("BACKUP_OBJECT_FAKE_FAILURE", "")
        if failure == "meta-failure" and key.endswith(".meta.json"):
            fail("OFFSITE_UPLOAD_FAILED", "OBS metadata upload failed")
        if failure == "sha-failure" and key.endswith(".sha256"):
            fail("OFFSITE_UPLOAD_FAILED", "OBS checksum sidecar upload failed")
        digest = hashlib.sha256(content.encode("utf-8")).hexdigest()
        existing = self.head(key)
        if existing is not None:
            if int(existing.get("size", -1)) == len(content.encode("utf-8")) and existing.get("sha256") == digest:
                return False
            current = self.get_text(key)
            if current != content:
                fail("REMOTE_CONFLICT", "remote sidecar exists with different content")
        target = self.path(key)
        parent = os.path.dirname(target)
        if not os.path.isdir(parent):
            os.makedirs(parent)
        with open(target, "w") as handle:
            handle.write(content)
        with open(self.info_path(key), "w") as handle:
            json.dump({"size": len(content.encode("utf-8")), "sha256": digest, "etag": hashlib.md5(content.encode("utf-8")).hexdigest()}, handle)
        return True
    def get_text(self, key):
        target = self.path(key)
        if not os.path.isfile(target):
            return None
        with open(target, "r") as handle:
            return handle.read()


def response_value(response, name, default=None):
    if response is None:
        return default
    if hasattr(response, "get"):
        return response.get(name, default)
    return getattr(response, name, default)


def mapping_items(value):
    if value is None:
        return []
    if hasattr(value, "items"):
        return value.items()
    if isinstance(value, (list, tuple)):
        return value
    return []


class ObsProvider(object):
    def __init__(self):
        bucket = os.environ.get("BACKUP_OBS_BUCKET", "")
        endpoint = os.environ.get("BACKUP_OBS_ENDPOINT", "")
        region = os.environ.get("BACKUP_OBS_REGION", "")
        if not bucket or not endpoint or not region:
            fail("CONFIG_REQUIRED", "OBS bucket, endpoint, and region are required")
        try:
            from obs import ObsClient
        except ImportError:
            fail("CONFIG_REQUIRED", "Huawei OBS Python SDK is not installed")
        policy = os.environ.get("BACKUP_OBS_SECURITY_PROVIDER", "OBS_DEFAULT")
        if policy == "ENV":
            if not os.environ.get("OBS_ACCESS_KEY_ID") or not os.environ.get("OBS_SECRET_ACCESS_KEY"):
                fail("CONFIG_REQUIRED", "OBS ENV credentials are required")
        try:
            self.client = ObsClient(server=endpoint, security_provider_policy=policy)
        except Exception:
            fail("CONFIG_REQUIRED", "OBS client configuration failed")
        self.bucket = bucket

    def close(self):
        close = getattr(self.client, "close", None)
        if close:
            close()

    def _response(self, response, operation):
        status = int(response_value(response, "status", 0) or 0)
        if status < 200 or status >= 300:
            fail("OFFSITE_UPLOAD_FAILED", "OBS {} failed with status {}".format(operation, status))
        return response

    def _log(self, event, **fields):
        data = {"event": event}
        data.update(fields)
        sys.stderr.write("BACKUP_UPLOADER " + json.dumps(data, sort_keys=True, separators=(",", ":")) + "\n")
        sys.stderr.flush()

    def _install_multipart_logging(self):
        original_initiate = self.client.initiateMultipartUpload
        original_part = self.client._uploadPartWithNotifier
        original_complete = self.client.completeMultipartUpload

        def initiate(*args, **kwargs):
            started = time.time()
            self._log("MULTIPART_INIT_STARTED")
            try:
                response = original_initiate(*args, **kwargs)
            except Exception as exc:
                self._log("MULTIPART_INIT_FAILED", duration=round(time.time() - started, 3), error_class=exc.__class__.__name__)
                raise
            body = response_value(response, "body")
            upload_id = response_value(body, "uploadId", "")
            upload_id_hash = hashlib.sha256(str(upload_id).encode("utf-8")).hexdigest()[:12] if upload_id else "missing"
            self._log("MULTIPART_INIT_COMPLETED", duration=round(time.time() - started, 3), status=response_value(response, "status", 0), upload_id_hash=upload_id_hash)
            return response

        def upload_part(*args, **kwargs):
            part_number = args[2] if len(args) > 2 else kwargs.get("partNumber", "unknown")
            part_size = kwargs.get("partSize", args[6] if len(args) > 6 else "unknown")
            started = time.time()
            self._log("PART_STARTED", number=part_number, size=part_size)
            try:
                response = original_part(*args, **kwargs)
            except Exception as exc:
                self._log("PART_FAILED", number=part_number, size=part_size, duration=round(time.time() - started, 3), error_class=exc.__class__.__name__)
                raise
            body = response_value(response, "body")
            etag = response_value(body, "etag", "")
            self._log("PART_COMPLETED", number=part_number, size=part_size, duration=round(time.time() - started, 3), status=response_value(response, "status", 0), etag_present="yes" if etag else "no")
            return response

        def complete(*args, **kwargs):
            started = time.time()
            self._log("MULTIPART_COMPLETE_STARTED")
            try:
                response = original_complete(*args, **kwargs)
            except Exception as exc:
                self._log("MULTIPART_COMPLETE_FAILED", duration=round(time.time() - started, 3), error_class=exc.__class__.__name__)
                raise
            self._log("MULTIPART_COMPLETE_COMPLETED", duration=round(time.time() - started, 3), status=response_value(response, "status", 0))
            return response

        self.client.initiateMultipartUpload = initiate
        self.client._uploadPartWithNotifier = upload_part
        self.client.completeMultipartUpload = complete

    def head(self, key):
        try:
            response = self.client.getObjectMetadata(self.bucket, key)
        except Exception:
            fail("OFFSITE_UPLOAD_FAILED", "OBS HEAD request failed")
        status = int(response_value(response, "status", 0) or 0)
        if status == 404:
            return None
        self._response(response, "HEAD")
        body = response_value(response, "body")
        metadata = {}
        for source in (response_value(response, "metadata", {}), response_value(body, "metadata", {})):
            for name, value in mapping_items(source):
                metadata[str(name).strip().lower()] = value
        for name, value in mapping_items(response_value(response, "header", [])):
            normalized_name = str(name).strip().lower()
            if normalized_name in ("x-obs-meta-sha256", "sha256"):
                metadata["x-obs-meta-sha256"] = value
        return {"size": int(response_value(body, "contentLength", 0) or 0), "etag": str(response_value(body, "etag", "") or ""), "sha256": str(metadata.get("x-obs-meta-sha256", "") or "").strip()}

    def put(self, source, key, sha256_value, size):
        metadata = {"x-obs-meta-sha256": sha256_value}
        try:
            # Keep large uploads resumable and bounded. A single PUT of a
            # database dump can stall behind an otherwise healthy OBS path.
            if size >= 64 * 1024 * 1024:
                checkpoint = os.path.join(tempfile.gettempdir(), "backup-obs-" + hashlib.sha256(key.encode("utf-8")).hexdigest() + ".checkpoint")
                self._install_multipart_logging()
                response = self.client.uploadFile(self.bucket, key, source, 16 * 1024 * 1024, 1, True, checkpoint, True, metadata=metadata)
            else:
                response = self.client.putFile(self.bucket, key, source, metadata=metadata)
        except Exception:
            fail("OFFSITE_UPLOAD_FAILED", "OBS object upload failed")
        self._response(response, "PUT")

    def put_text(self, content, key):
        digest = hashlib.sha256(content.encode("utf-8")).hexdigest()
        existing = self.head(key)
        if existing is not None:
            expected_size = len(content.encode("utf-8"))
            if int(existing.get("size", -1)) == expected_size and existing.get("sha256") == digest:
                return False
            current = self.get_text(key)
            if current != content:
                fail("REMOTE_CONFLICT", "OBS sidecar exists with different content")
        try:
            response = self.client.putContent(self.bucket, key, io.BytesIO(content.encode("utf-8")), metadata={"x-obs-meta-sha256": digest})
        except Exception:
            fail("OFFSITE_UPLOAD_FAILED", "OBS sidecar upload failed")
        self._response(response, "PUT")
        return True

    def get_text(self, key):
        try:
            response = self.client.getObject(self.bucket, key, loadStreamInMemory=True)
        except Exception:
            fail("OFFSITE_UPLOAD_FAILED", "OBS sidecar download failed")
        self._response(response, "GET")
        body = response_value(response, "body")
        reader = getattr(body, "read", None)
        if callable(reader):
            body = reader()
        elif reader is not None or isinstance(body, dict):
            content = response_value(body, "content")
            if content is None:
                content = response_value(body, "buffer")
            body = content
        if isinstance(body, bytes):
            return body.decode("utf-8")
        if isinstance(body, str):
            return body
        if body is None:
            fail("OFFSITE_UPLOAD_FAILED", "OBS sidecar response body is unavailable")
        fail("OFFSITE_UPLOAD_FAILED", "OBS sidecar response body format is unsupported")
        return str(body or "")

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
if provider_name == "fake":
    provider = FakeProvider(os.environ.get("OFFSITE_FAKE_ROOT", ""))
elif provider_name == "obs":
    if os.environ.get("BACKUP_OBS_FAKE") == "1":
        provider = FakeProvider(os.environ.get("OFFSITE_FAKE_ROOT", ""))
    else:
        provider = ObsProvider()
else:
    fail("UPLOAD_NOT_CONFIGURED", "provider is not configured")
key = local["object_key"]
remote = provider.head(key)
if remote is not None:
    if int(remote.get("size", -1)) == local["bytes"] and remote.get("sha256") == local["sha256"]:
        main_uploaded = False
    else:
        fail("REMOTE_CONFLICT", "remote object exists with different size or checksum")
else:
    provider.put(local["path"], key, local["sha256"], local["bytes"])
    main_uploaded = True
remote = verify(provider, key, local)
with open(local["meta"], "r") as handle:
    meta_content = handle.read()
meta_uploaded = provider.put_text(meta_content, key + ".meta.json")
meta_remote = provider.head(key + ".meta.json")
if meta_remote is None or int(meta_remote.get("size", -1)) != len(meta_content.encode("utf-8")):
    fail("REMOTE_SIZE_MISMATCH", "remote metadata object could not be verified")
if meta_remote.get("sha256") != hashlib.sha256(meta_content.encode("utf-8")).hexdigest():
    fail("REMOTE_CHECKSUM_MISMATCH", "remote metadata checksum could not be verified")
checksum_content = local["sha256"] + "  " + os.path.basename(local["path"]) + "\n"
checksum_uploaded = provider.put_text(checksum_content, key + ".sha256")
checksum_remote = provider.head(key + ".sha256")
if checksum_remote is None or int(checksum_remote.get("size", -1)) != len(checksum_content.encode("utf-8")):
    fail("REMOTE_SIZE_MISMATCH", "remote sha256 object could not be verified")
if checksum_remote.get("sha256") != hashlib.sha256(checksum_content.encode("utf-8")).hexdigest():
    fail("REMOTE_CHECKSUM_MISMATCH", "remote sha256 object could not be verified")
payload = {"version": 1, "provider": provider_name, "bucket": os.environ.get("BACKUP_OBS_BUCKET", os.environ.get("BACKUP_OBJECT_BUCKET", "fake")), "object_key": key, "uploaded_at": datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"), "local_bytes": local["bytes"], "local_sha256": local["sha256"], "remote_bytes": int(remote["size"]), "remote_etag": remote.get("etag", ""), "remote_sha256": remote["sha256"], "verification": "OFFSITE_VERIFIED"}
offsite_path = local["path"] + ".offsite.json"
temporary_path = offsite_path + ".part"
try:
    with open(temporary_path, "w") as handle:
        json.dump(payload, handle, sort_keys=True, indent=2)
        handle.write("\n")
        handle.flush()
        os.fsync(handle.fileno())
    os.replace(temporary_path, offsite_path)
except Exception:
    try:
        os.unlink(temporary_path)
    except OSError:
        pass
    fail("OFFSITE_UPLOAD_FAILED", "offsite verification sidecar write failed")
if hasattr(provider, "close"):
    provider.close()
payload.update({"status": "OFFSITE_VERIFIED" if main_uploaded or meta_uploaded or checksum_uploaded else "ALREADY_OFFSITE_VERIFIED", "offsite_path": offsite_path, "uploaded": bool(main_uploaded or meta_uploaded or checksum_uploaded)})
out(payload)
PY
