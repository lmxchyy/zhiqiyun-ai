#!/usr/bin/env bash
set -Eeuo pipefail

MANIFEST_PATH="${1:--}"
EXPECTED_GIT_SHA="${2:-}"
EXPECTED_IMAGE_REFERENCE="${3:-}"
SELECTED_REGISTRY="${4:-}"

command -v python3 >/dev/null 2>&1 || { printf '%s\n' '[release-manifest] ERROR: python3 is required.' >&2; exit 1; }

python3 -c '
import json
import re
import sys

path, expected_sha, expected_ref, selected_registry = sys.argv[1:5]
try:
    if path == "-":
        manifest = json.load(sys.stdin)
    else:
        with open(path, "r") as handle:
            manifest = json.load(handle)
except Exception as exc:
    raise SystemExit("invalid JSON: %s" % exc)

required = ("git_sha", "image", "digest", "image_reference", "built_at", "production_contract")
missing = [key for key in required if not manifest.get(key)]
if missing:
    raise SystemExit("missing fields: %s" % ", ".join(missing))
sha = manifest["git_sha"]
if selected_registry:
    registry_entry = manifest.get("registries", {}).get(selected_registry)
    if not isinstance(registry_entry, dict):
        raise SystemExit("registry is not present in manifest: %s" % selected_registry)
    image = registry_entry.get("image")
    digest = registry_entry.get("digest")
    image_reference = registry_entry.get("image_reference")
else:
    image = manifest["image"]
    digest = manifest["digest"]
    image_reference = manifest["image_reference"]
if not image or not digest or not image_reference:
    raise SystemExit("selected registry entry is incomplete")
if not re.match(r"^[0-9a-f]{40}$", sha):
    raise SystemExit("invalid git_sha")
if not re.match(r"^sha256:[0-9a-f]{64}$", digest):
    raise SystemExit("invalid digest")
if "@" in image or not re.match(r"^[A-Za-z0-9][A-Za-z0-9./_-]*$", image):
    raise SystemExit("invalid image")
if image_reference != image + "@" + digest:
    raise SystemExit("image_reference does not match image and digest")
if expected_sha and sha != expected_sha:
    raise SystemExit("git_sha does not match expected deployment commit")
if expected_ref and image_reference != expected_ref:
    raise SystemExit("image_reference does not match expected release")
if manifest["production_contract"] != "passed":
    raise SystemExit("production_contract is not passed")
print(image_reference)
' "$MANIFEST_PATH" "$EXPECTED_GIT_SHA" "$EXPECTED_IMAGE_REFERENCE" "$SELECTED_REGISTRY"
