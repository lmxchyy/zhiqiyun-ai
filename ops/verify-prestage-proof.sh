#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$SCRIPT_DIR"

command -v python3 >/dev/null 2>&1 || { printf '%s\n' '[prestage-verify] ERROR: python3 is required.' >&2; exit 1; }

PROOF_PATH="${1:-}"
EXPECTED_GIT_SHA="${2:-}"
COMPOSE_FILE="${3:-compose.prod.yml}"
ENV_FILE="${4:-.env.production}"
TRUST_KEY_OR_FILE="${5:-}"
LEDGER_FILE="${6:-backups/release-ledger.json}"

if [ -z "$PROOF_PATH" ]; then
  echo "Usage: $0 <PROOF_PATH> <EXPECTED_GIT_SHA> [COMPOSE_FILE] [ENV_FILE] [TRUST_KEY] [LEDGER_FILE]" >&2
  exit 1
fi

python3 - "$PROOF_PATH" "$EXPECTED_GIT_SHA" "$COMPOSE_FILE" "$ENV_FILE" "$TRUST_KEY_OR_FILE" "$LEDGER_FILE" <<'PY'
import datetime
import hashlib
import hmac
import json
import os
import re
import shlex
import shutil
import subprocess
import sys

def fail(msg):
    sys.stderr.write(f"[prestage-verify] ERROR: {msg}\n")
    sys.exit(1)

def run_cmd(args, env_override=None):
    cmd_env = dict(os.environ)
    if env_override:
        cmd_env.update(env_override)
    if os.name == "nt":
        sh_bin = os.environ.get("SH_EXE") or shutil.which("sh") or "C:/Program Files/Git/bin/sh.exe"
        cmd_str = " ".join(shlex.quote(a) for a in args)
        return subprocess.check_output([sh_bin, "-c", cmd_str], stderr=subprocess.PIPE, env=cmd_env).decode("utf-8").strip()
    return subprocess.check_output(args, stderr=subprocess.PIPE, env=cmd_env).decode("utf-8").strip()

proof_path = sys.argv[1]
expected_sha = sys.argv[2] if len(sys.argv) > 2 else ""
compose_file = sys.argv[3] if len(sys.argv) > 3 and sys.argv[3] else "compose.prod.yml"
env_file = sys.argv[4] if len(sys.argv) > 4 and sys.argv[4] else ".env.production"
trust_key_input = sys.argv[5] if len(sys.argv) > 5 else ""
ledger_file = sys.argv[6] if len(sys.argv) > 6 and sys.argv[6] else os.environ.get("RELEASE_LEDGER_FILE", "backups/release-ledger.json")

# 1. Read and parse proof file
if not os.path.isfile(proof_path) or os.path.getsize(proof_path) == 0:
    fail(f"Proof file does not exist or is empty: {proof_path}")

try:
    with open(proof_path, "r", encoding="utf-8") as f:
        proof = json.load(f)
except Exception as e:
    fail(f"Invalid JSON in proof file: {e}")

if not isinstance(proof, dict):
    fail("Proof root must be a JSON object")

# 2. Check git_sha
proof_sha = proof.get("git_sha", "")
if not re.match(r"^[0-9a-f]{40}$", str(proof_sha)):
    fail("Proof contains invalid git_sha format")
if expected_sha and proof_sha != expected_sha:
    fail(f"HEAD_MISMATCH: proof git_sha ({proof_sha}) does not match expected ({expected_sha})")

# 3. Check expiration
expires_at_str = proof.get("expires_at", "")
if not expires_at_str:
    fail("Proof missing expires_at field")
try:
    if hasattr(datetime.datetime, "fromisoformat"):
        iso_input = expires_at_str
        if iso_input.endswith("Z"):
            iso_input = iso_input[:-1] + "+00:00"
        expires_at = datetime.datetime.fromisoformat(iso_input)
    else:
        # Python 3.6 compatibility fallback: parse ISO 8601 UTC string
        clean_str = expires_at_str.split(".")[0].replace("Z", "").replace("+00:00", "")
        expires_at = datetime.datetime.strptime(clean_str, "%Y-%m-%dT%H:%M:%S").replace(tzinfo=datetime.timezone.utc)
    now = datetime.datetime.now(datetime.timezone.utc)
    if now > expires_at:
        fail(f"PRESTAGE_PROOF_EXPIRED: proof expired at {proof.get('expires_at')} (now is {now.isoformat()})")
except Exception as e:
    if "PRESTAGE_PROOF_EXPIRED" in str(e):
        fail(str(e))
    fail(f"Invalid expires_at in proof: {e}")

# 4. Resolve Trust Key
trust_key = ""
if trust_key_input:
    if os.path.isfile(trust_key_input):
        with open(trust_key_input, "r", encoding="utf-8") as f:
            trust_key = f.read().strip()
    else:
        trust_key = trust_key_input.strip()
elif os.environ.get("RELEASE_TRUST_SECRET"):
    trust_key = os.environ.get("RELEASE_TRUST_SECRET").strip()
elif os.environ.get("RELEASE_TRUST_KEY_FILE") and os.path.isfile(os.environ.get("RELEASE_TRUST_KEY_FILE")):
    with open(os.environ.get("RELEASE_TRUST_KEY_FILE"), "r", encoding="utf-8") as f:
        trust_key = f.read().strip()
elif os.path.isfile("/etc/zhiqiyun/release-trust.key"):
    with open("/etc/zhiqiyun/release-trust.key", "r", encoding="utf-8") as f:
        trust_key = f.read().strip()
elif os.path.isfile(".prestage/release-trust.key"):
    with open(".prestage/release-trust.key", "r", encoding="utf-8") as f:
        trust_key = f.read().strip()
else:
    alt_key = os.path.join(os.path.dirname(proof_path), "release-trust.key")
    if os.path.isfile(alt_key):
        with open(alt_key, "r", encoding="utf-8") as f:
            trust_key = f.read().strip()

if not trust_key:
    fail("RELEASE_TRUST_KEY_MISSING: no valid release trust key found")

# 5. Verify HMAC-SHA256 signature
signature = proof.get("signature")
if not signature:
    fail("Proof missing signature")

payload = {k: v for k, v in proof.items() if k != "signature"}
canonical_json = json.dumps(payload, sort_keys=True, separators=(',', ':'))
expected_sig = hmac.new(trust_key.encode("utf-8"), canonical_json.encode("utf-8"), hashlib.sha256).hexdigest()

if not hmac.compare_digest(signature, expected_sig):
    fail("SIGNATURE_VERIFICATION_FAILED: proof signature does not match (tampered proof or invalid key)")

# 6. Anti-replay ledger check
nonce = proof.get("proof_nonce")
if not nonce:
    fail("Proof missing proof_nonce")

if os.path.isfile(ledger_file):
    try:
        with open(ledger_file, "r", encoding="utf-8") as f:
            ledger = json.load(f)
        entries = ledger.get("entries", [])
        for entry in entries:
            if entry.get("nonce") == nonce and entry.get("status") == "CONSUMED":
                fail(f"PRESTAGE_PROOF_ALREADY_CONSUMED: proof nonce {nonce} was already consumed at {entry.get('consumed_at')}")
    except Exception as e:
        if "PRESTAGE_PROOF_ALREADY_CONSUMED" in str(e):
            fail(str(e))
        fail(f"Failed to inspect release ledger: {e}")

# 7. Check compose_hash (Fail Closed on missing file)
if not os.path.isfile(compose_file) or os.path.getsize(compose_file) == 0:
    fail(f"PROTECTED_FILE_MISSING: Compose file does not exist or is empty: {compose_file}")
with open(compose_file, "rb") as f:
    actual_compose_hash = hashlib.sha256(f.read()).hexdigest()
if actual_compose_hash != proof.get("compose_hash"):
    fail(f"COMPOSE_TAMPERED: {compose_file} hash mismatch (expected {proof.get('compose_hash')}, got {actual_compose_hash})")

# 8. Check env_hash & effective Compose configuration binding (Fail Closed)
if not os.path.isfile(env_file) or os.path.getsize(env_file) == 0:
    fail(f"PROTECTED_FILE_MISSING: Env file does not exist or is empty: {env_file}")
with open(env_file, "rb") as f:
    actual_env_hash = hashlib.sha256(f.read()).hexdigest()
if actual_env_hash != proof.get("env_hash"):
    fail(f"CONFIG_MUTATED_AFTER_PRESTAGE: {env_file} was modified after prestage (expected {proof.get('env_hash')}, got {actual_env_hash})")

# Re-render effective Compose configuration to verify environment variables / overrides
try:
    target_ref = proof.get("image_reference", "")
    actual_compose_raw = run_cmd(
        ["docker", "compose", "-f", compose_file, "--env-file", env_file, "config", "--format", "json"],
        env_override={"XIANZHI_IMAGE_REFERENCE": target_ref} if target_ref else None
    )
    actual_compose_data = json.loads(actual_compose_raw)
except Exception as e:
    fail(f"Failed to render effective Docker Compose configuration: {e}")

services_data = actual_compose_data.get("services", {})
for s_name in ("xianzhi-ai", "smartvideo-worker", "migrate"):
    if not services_data.get(s_name):
        fail("CRITICAL_SERVICE_MISSING: " + s_name)
actual_bound_str = json.dumps(actual_compose_data, sort_keys=True, separators=(',', ':'))
actual_bound_hash = hashlib.sha256(actual_bound_str.encode("utf-8")).hexdigest()

expected_bound_hash = proof.get("bound_config_hash")
if proof.get("config_binding_version") != 2 or not expected_bound_hash:
    fail("CONFIG_BINDING_MISSING: restage with the full effective configuration binding")
if actual_bound_hash != expected_bound_hash:
    fail(f"CONFIG_MUTATED_AFTER_PRESTAGE: effective Compose configuration altered after prestage (expected {expected_bound_hash}, got {actual_bound_hash})")

# 9. Check migrations directory & disk content (Fail Closed)
migrations_dir = os.path.join("database", "migrations")
if not os.path.isdir(migrations_dir):
    fail(f"PROTECTED_FILE_MISSING: database/migrations directory missing on disk: {migrations_dir}")

disk_sql_files = sorted([f for f in os.listdir(migrations_dir) if f.endswith(".sql") and not f.endswith(".down.sql")])
if not disk_sql_files:
    fail("PROTECTED_FILE_MISSING: database/migrations directory contains no valid SQL migration files")

expected_count = proof.get("migrations_files_count")
if expected_count is not None and len(disk_sql_files) != expected_count:
    fail(f"MIGRATIONS_TAMPERED: migration files count mismatch (expected {expected_count}, got {len(disk_sql_files)})")

dir_hasher = hashlib.sha256()
for fn in disk_sql_files:
    fp = os.path.join(migrations_dir, fn)
    if not os.path.isfile(fp) or os.path.getsize(fp) == 0:
        fail(f"PROTECTED_FILE_MISSING: migration SQL file empty or missing: {fn}")
    with open(fp, "rb") as mf:
        content = mf.read()
    f_hash = hashlib.sha256(content).hexdigest()
    dir_hasher.update(f"{fn}:{f_hash}\n".encode("utf-8"))
actual_migrations_content_hash = dir_hasher.hexdigest()

expected_content_hash = proof.get("migrations_content_hash")
if expected_content_hash and actual_migrations_content_hash != expected_content_hash:
    fail(f"MIGRATIONS_TAMPERED: database/migrations disk content mismatch (expected {expected_content_hash}, got {actual_migrations_content_hash})")

expected_migrations_tree = proof.get("migrations_tree_hash")
if expected_migrations_tree:
    actual_tree = ""
    try:
        actual_tree = run_cmd(["git", "rev-parse", "HEAD:database/migrations"])
    except Exception:
        pass
    if actual_tree and actual_tree != expected_migrations_tree:
        fail(f"MIGRATIONS_TAMPERED: database/migrations git tree hash mismatch (expected {expected_migrations_tree}, got {actual_tree})")

# 10. Check deploy_scripts_hash (Fail Closed on missing scripts)
scripts_hash = proof.get("deploy_scripts_hash", {})
if not scripts_hash:
    fail("PROTECTED_FILE_MISSING: deploy_scripts_hash is empty in proof")

for script_name, expected_script_hash in scripts_hash.items():
    if not os.path.isfile(script_name) or os.path.getsize(script_name) == 0:
        fail(f"DEPLOY_SCRIPT_TAMPERED: required script missing or empty on disk: {script_name}")
    with open(script_name, "rb") as sf:
        actual_script_hash = hashlib.sha256(sf.read()).hexdigest()
    if actual_script_hash != expected_script_hash:
        fail(f"DEPLOY_SCRIPT_TAMPERED: {script_name} hash mismatch (expected {expected_script_hash}, got {actual_script_hash})")

# 11. Check release manifest hash (Fail Closed)
manifest_path = proof.get("manifest_path")
if not manifest_path or not os.path.isfile(manifest_path) or os.path.getsize(manifest_path) == 0:
    fail(f"PROTECTED_FILE_MISSING: release manifest missing or empty: {manifest_path}")
with open(manifest_path, "rb") as mf:
    actual_manifest_hash = hashlib.sha256(mf.read()).hexdigest()
if actual_manifest_hash != proof.get("manifest_sha256"):
    fail(f"MANIFEST_TAMPERED: release manifest hash mismatch (expected {proof.get('manifest_sha256')}, got {actual_manifest_hash})")

# 12. Check rollback receipt & previous image (Fail Closed)
receipt_path = proof.get("rollback_receipt_path")
if not receipt_path or not os.path.isfile(receipt_path) or os.path.getsize(receipt_path) == 0:
    fail(f"ROLLBACK_RECEIPT_INVALID: rollback receipt missing or empty on disk: {receipt_path}")
with open(receipt_path, "rb") as rf:
    actual_receipt_hash = hashlib.sha256(rf.read()).hexdigest()
if actual_receipt_hash != proof.get("rollback_receipt_hash"):
    fail(f"ROLLBACK_RECEIPT_INVALID: rollback receipt hash mismatch (expected {proof.get('rollback_receipt_hash')}, got {actual_receipt_hash})")

try:
    with open(receipt_path, "r", encoding="utf-8") as rf:
        receipt_data = json.load(rf)
    prev_id = receipt_data.get("previous_image_id", "")
    prev_ref = receipt_data.get("previous_image_reference", "")
    if not prev_id or prev_id == "none" or not prev_ref or prev_ref == "none":
        fail("ROLLBACK_RECEIPT_INVALID: previous image reference and ID must not be empty or 'none'")
    inspect_out = run_cmd(["docker", "image", "inspect", prev_id, "--format", "{{.Id}}"])
    if not inspect_out:
        fail("ROLLBACK_IMAGE_MISSING: previous image has been pruned from local docker")
    rb_m_path = receipt_data.get("rollback_manifest_path", "")
    if not rb_m_path or not os.path.isfile(rb_m_path) or os.path.getsize(rb_m_path) == 0:
        fail(f"ROLLBACK_MANIFEST_MISSING: rollback manifest missing on disk: {rb_m_path}")
    with open(rb_m_path, "rb") as rbmf:
        actual_rb_m_hash = hashlib.sha256(rbmf.read()).hexdigest()
    if actual_rb_m_hash != receipt_data.get("rollback_manifest_sha256"):
        fail("ROLLBACK_MANIFEST_TAMPERED: rollback manifest hash mismatch")
except Exception as e:
    if "ROLLBACK_IMAGE_MISSING" in str(e) or "ROLLBACK_RECEIPT_INVALID" in str(e) or "ROLLBACK_MANIFEST" in str(e):
        fail(str(e))
    fail(f"ROLLBACK_RECEIPT_INVALID: failed to inspect rollback receipt: {e}")

# 13. Check target Docker image locally
image_reference = proof.get("image_reference")
if not image_reference:
    fail("Proof missing image_reference")
expected_local_id = proof.get("local_image_id")
if expected_local_id:
    try:
        actual_local_id = run_cmd(["docker", "image", "inspect", image_reference, "--format", "{{.Id}}"])
        if actual_local_id != expected_local_id:
            fail(f"LOCAL_IMAGE_MISMATCH: docker image ID mismatch (expected {expected_local_id}, got {actual_local_id})")
    except Exception as e:
        fail(f"LOCAL_IMAGE_MISMATCH: cannot inspect local image {image_reference}: {e}")

# 14. Check GitHub official provenance
gov = proof.get("github_provenance")
if not isinstance(gov, dict):
    fail("PROVENANCE_MISSING: proof missing verified github_provenance object")
if gov.get("manifest_bytes_sha256") != proof.get("manifest_sha256"):
    fail("PROVENANCE_MISMATCH: provenance manifest_bytes_sha256 does not match proof manifest_sha256")
for req_field in ("repository", "workflow", "run_id", "artifact_id", "head_sha"):
    if not gov.get(req_field):
        fail(f"PROVENANCE_INCOMPLETE: missing required provenance field: {req_field}")

# All checks passed! Output image reference
print(image_reference)
PY
