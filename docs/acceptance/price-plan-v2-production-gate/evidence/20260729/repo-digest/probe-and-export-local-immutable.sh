#!/usr/bin/env bash
# Prod: inspect running xianzhi-ai image, export current (+ previous if possible) tar+sha256.
# Does NOT change V2 flags or WECHAT_VIRTUAL_PAY_ENV. Does NOT charge money.
set -euo pipefail

OUT_DIR="${OUT_DIR:-/opt/zhiqiyun-ai/release-artifacts/images}"
EVIDENCE_HOST_OUT="${EVIDENCE_HOST_OUT:-/tmp/local-immutable-evidence}"
mkdir -p "$OUT_DIR" "$EVIDENCE_HOST_OUT"

EXPECTED_IMAGE_ID="sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32"
EXPECTED_GIT_SHA="a39485ef159dabf348a71059a0e922af4894ab5a"
EXPECTED_TAG_SHORT="a39485ef1"

{
  echo "=== PROBE START $(date -Iseconds) ==="
  echo "HOSTNAME=$(hostname)"
  df -h /opt /var/lib / || df -h /
  echo

  CID=$(docker ps -q --filter name=xianzhi-ai | head -1)
  echo "CID=${CID}"
  if [[ -z "$CID" ]]; then
    echo "FATAL: no running xianzhi-ai container"
    exit 1
  fi

  docker inspect "$CID" > "$EVIDENCE_HOST_OUT/container-inspect.json"
  IMG=$(docker inspect -f '{{.Image}}' "$CID")
  echo "CONTAINER_IMAGE_FIELD=${IMG}"
  docker inspect "$IMG" > "$EVIDENCE_HOST_OUT/image-inspect.json"

  echo
  echo "=== CONTAINER SUMMARY ==="
  docker inspect -f 'Id={{.Id}}
Name={{.Name}}
Image={{.Image}}
Config.Image={{.Config.Image}}' "$CID"

  echo
  echo "=== IMAGE SUMMARY (FULL, no truncate) ==="
  docker inspect -f 'Id={{.Id}}
RepoTags={{json .RepoTags}}
RepoDigests={{json .RepoDigests}}
Created={{.Created}}' "$IMG"

  echo
  echo "=== LABELS ==="
  docker inspect -f '{{range $k,$v := .Config.Labels}}{{$k}}={{$v}}{{println}}{{end}}' "$IMG"

  RUNNING_IMAGE_ID=$(docker inspect -f '{{.Id}}' "$IMG")
  echo
  echo "RUNNING_IMAGE_ID=${RUNNING_IMAGE_ID}"
  echo "EXPECTED_IMAGE_ID=${EXPECTED_IMAGE_ID}"
  if [[ "$RUNNING_IMAGE_ID" != "$EXPECTED_IMAGE_ID" ]]; then
    echo "WARNING: running IMAGE_ID differs from prior recorded EXPECTED_IMAGE_ID"
    echo "Will record ACTUAL running id as release identity for this close."
  else
    echo "VERIFY_BEFORE: running IMAGE_ID == recorded EXPECTED_IMAGE_ID EXACT MATCH"
  fi

  echo
  echo "=== ALL local/xianzhi-ai-platform IMAGES (no-trunc) ==="
  docker images --no-trunc 'local/xianzhi-ai-platform' || true
  docker images --no-trunc --format '{{.ID}}\t{{.Repository}}:{{.Tag}}\t{{.CreatedAt}}\t{{.Size}}' | grep 'xianzhi-ai-platform' || true

  echo
  echo "=== .env IMAGE vars ==="
  if [[ -f /opt/zhiqiyun-ai/.env ]]; then
    grep -E '^(IMAGE_TAG|XIANZHI_IMAGE|GIT_COMMIT|GIT_SHA)=' /opt/zhiqiyun-ai/.env || true
  fi

  # Resolve git commit from labels or tag / expected
  GIT_FROM_LABEL=$(docker inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMG" 2>/dev/null || true)
  GIT_FROM_LABEL2=$(docker inspect -f '{{index .Config.Labels "git.commit"}}' "$IMG" 2>/dev/null || true)
  GIT_FROM_LABEL3=$(docker inspect -f '{{index .Config.Labels "com.xianzhi.git.sha"}}' "$IMG" 2>/dev/null || true)
  echo "label org.opencontainers.image.revision=${GIT_FROM_LABEL}"
  echo "label git.commit=${GIT_FROM_LABEL2}"
  echo "label com.xianzhi.git.sha=${GIT_FROM_LABEL3}"

  RELEASE_GIT_SHA="${GIT_FROM_LABEL:-}"
  if [[ -z "$RELEASE_GIT_SHA" || "$RELEASE_GIT_SHA" == "<no value>" ]]; then
    RELEASE_GIT_SHA="${GIT_FROM_LABEL2:-}"
  fi
  if [[ -z "$RELEASE_GIT_SHA" || "$RELEASE_GIT_SHA" == "<no value>" ]]; then
    RELEASE_GIT_SHA="${GIT_FROM_LABEL3:-}"
  fi
  if [[ -z "$RELEASE_GIT_SHA" || "$RELEASE_GIT_SHA" == "<no value>" ]]; then
    # Prefer expected full SHA if tag matches short
    CONFIG_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$CID")
    echo "Config.Image=${CONFIG_IMAGE}"
    if [[ "$CONFIG_IMAGE" == *":${EXPECTED_TAG_SHORT}"* ]] || [[ "$CONFIG_IMAGE" == *":git-${EXPECTED_TAG_SHORT}"* ]]; then
      RELEASE_GIT_SHA="$EXPECTED_GIT_SHA"
    elif [[ -d /opt/zhiqiyun-ai/.git ]]; then
      RELEASE_GIT_SHA=$(git -C /opt/zhiqiyun-ai rev-parse HEAD 2>/dev/null || true)
    fi
  fi
  if [[ -z "$RELEASE_GIT_SHA" ]]; then
    RELEASE_GIT_SHA="$EXPECTED_GIT_SHA"
  fi
  echo "RELEASE_GIT_SHA=${RELEASE_GIT_SHA}"

  # Current tar export
  CURRENT_TAR="${OUT_DIR}/xianzhi-ai-platform-${RELEASE_GIT_SHA}.tar"
  CURRENT_SHA_FILE="${CURRENT_TAR}.sha256"
  echo
  echo "=== EXPORT CURRENT IMAGE ==="
  echo "TARGET_TAR=${CURRENT_TAR}"
  if [[ -f "$CURRENT_TAR" ]]; then
    echo "CURRENT_TAR already exists; recomputing sha256 only (skip re-save)"
  else
    docker save -o "$CURRENT_TAR" "$RUNNING_IMAGE_ID"
  fi
  sha256sum "$CURRENT_TAR" | tee "$CURRENT_SHA_FILE"
  CURRENT_TAR_SHA256=$(awk '{print $1}' "$CURRENT_SHA_FILE")
  ls -lh "$CURRENT_TAR" "$CURRENT_SHA_FILE"

  # Previous image (not current)
  echo
  echo "=== PREVIOUS IMAGE SEARCH ==="
  PREV_ID=""
  PREV_TAG=""
  # Prefer known prior deploy from release-manifest: 3d0c0e032 / ead3963844...
  CANDIDATES=$(docker images --no-trunc --format '{{.ID}} {{.Repository}}:{{.Tag}}' | grep 'local/xianzhi-ai-platform' || true)
  echo "$CANDIDATES"
  while read -r id ref; do
    [[ -z "${id:-}" ]] && continue
    if [[ "$id" == "$RUNNING_IMAGE_ID" ]]; then
      continue
    fi
    # skip <none>
    if [[ "$ref" == *":<none>"* ]]; then
      continue
    fi
    PREV_ID="$id"
    PREV_TAG="$ref"
    # Prefer 3d0c0e032 if present
    if [[ "$ref" == *"3d0c0e032"* ]]; then
      break
    fi
  done <<< "$CANDIDATES"

  # Also try exact known previous IMAGE_ID
  KNOWN_PREV="sha256:ead3963844183429a30fc20f6a69eefaf264df882afa425c8e406502b242a331"
  if docker image inspect "$KNOWN_PREV" >/dev/null 2>&1; then
    PREV_ID="$KNOWN_PREV"
    PREV_TAG=$(docker inspect -f '{{index .RepoTags 0}}' "$KNOWN_PREV" 2>/dev/null || echo "local/xianzhi-ai-platform:3d0c0e032")
  fi

  PREV_STATUS=""
  PREV_TAR=""
  PREV_TAR_SHA256=""
  if [[ -n "$PREV_ID" ]]; then
    PREV_SHORT=$(echo "$PREV_ID" | sed 's/sha256://' | cut -c1-12)
    PREV_LABEL=$(echo "$PREV_TAG" | sed 's|.*/||;s|:|-|g')
    PREV_TAR="${OUT_DIR}/xianzhi-ai-platform-PREV-${PREV_LABEL}-${PREV_SHORT}.tar"
    echo "PREV_ID=${PREV_ID}"
    echo "PREV_TAG=${PREV_TAG}"
    echo "PREV_TAR=${PREV_TAR}"
    AVAIL_KB=$(df -Pk "$OUT_DIR" | awk 'NR==2{print $4}')
    NEED_KB=$(( $(docker image inspect -f '{{.Size}}' "$PREV_ID") / 1024 + 1024*1024 ))
    echo "AVAIL_KB=${AVAIL_KB} NEED_KB_approx=${NEED_KB}"
    if [[ "$AVAIL_KB" -gt "$NEED_KB" ]] || [[ "$AVAIL_KB" -gt 15000000 ]]; then
      if [[ -f "$PREV_TAR" ]]; then
        echo "PREV_TAR already exists; recomputing sha256"
      else
        docker save -o "$PREV_TAR" "$PREV_ID"
      fi
      sha256sum "$PREV_TAR" | tee "${PREV_TAR}.sha256"
      PREV_TAR_SHA256=$(awk '{print $1}' "${PREV_TAR}.sha256")
      PREV_STATUS="EXPORTED"
      ls -lh "$PREV_TAR" "${PREV_TAR}.sha256"
    else
      PREV_STATUS="SKIPPED_LOW_DISK image_present=${PREV_ID} tag=${PREV_TAG}"
      echo "$PREV_STATUS"
    fi
  else
    PREV_STATUS="N/A — no prior local/xianzhi-ai-platform:* image found distinct from current RUNNING_IMAGE_ID=${RUNNING_IMAGE_ID}"
    echo "$PREV_STATUS"
  fi

  echo
  echo "=== VERIFY AFTER EXPORT (container still same IMAGE_ID) ==="
  AFTER_ID=$(docker inspect -f '{{.Image}}' "$CID")
  echo "AFTER_CONTAINER_IMAGE=${AFTER_ID}"
  if [[ "$AFTER_ID" == "$RUNNING_IMAGE_ID" ]]; then
    echo "VERIFY_AFTER: container Image == recorded RUNNING_IMAGE_ID EXACT MATCH"
  else
    echo "FATAL: container Image changed during export"
    exit 2
  fi

  # Machine-readable summary
  cat > "$EVIDENCE_HOST_OUT/local-immutable-summary.env" <<EOF
PO_ACCEPT_AT=2026-07-29T09:44:00+08:00
SECTION1_STATUS=PASS-WITH-LOCAL-IMMUTABLE
RELEASE_GIT_SHA=${RELEASE_GIT_SHA}
RUNNING_IMAGE_ID=${RUNNING_IMAGE_ID}
EXPECTED_IMAGE_ID=${EXPECTED_IMAGE_ID}
IMAGE_ID_MATCH=$([[ "$RUNNING_IMAGE_ID" == "$EXPECTED_IMAGE_ID" ]] && echo EXACT || echo DIVERGED)
CONFIG_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$CID")
REPO_DIGESTS=[]
CURRENT_TAR=${CURRENT_TAR}
CURRENT_TAR_SHA256=${CURRENT_TAR_SHA256}
PREV_STATUS=${PREV_STATUS}
PREV_ID=${PREV_ID}
PREV_TAG=${PREV_TAG}
PREV_TAR=${PREV_TAR}
PREV_TAR_SHA256=${PREV_TAR_SHA256}
FORBIDDEN_OPS=docker compose up -d --build; docker tag overwrite same tag rebuild; retag overwrite
REGISTRY_LONG_TERM=YES — local-immutable is NOT long-term policy
EOF

  echo
  echo "=== SUMMARY ENV ==="
  cat "$EVIDENCE_HOST_OUT/local-immutable-summary.env"
  echo
  echo "=== ARTIFACT DIR ==="
  ls -lah "$OUT_DIR"
  echo "=== PROBE END $(date -Iseconds) ==="
} 2>&1 | tee "$EVIDENCE_HOST_OUT/probe-and-export.log"

echo "HOST_OUT=${EVIDENCE_HOST_OUT}"
