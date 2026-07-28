#!/bin/bash
# Deploy deliver-notify fix; KEEP V2 flags true (unlike older gate scripts that forced false).
set -euo pipefail
cd /opt/zhiqiyun-ai
EVID=/tmp/deliver-notify-deploy
mkdir -p "$EVID"
BRANCH=codex/channel-ecosystem-v132-phase3

git fetch origin "$BRANCH"
TARGET=$(git rev-parse "origin/$BRANCH")
SHORT=$(git rev-parse --short=9 "$TARGET")
echo "TARGET=$TARGET SHORT=$SHORT" | tee "$EVID/build.meta"

# Preserve local secrets/data; sync code to TARGET
git reset --hard HEAD || true
git clean -fd -e .env.production -e backups -e data -e 'admin-vue/dist' -e compose.oc009.override.yml -e ops-evidence || true
git checkout -f -B "$BRANCH" "$TARGET"
git reset --hard "$TARGET"
test "$(git rev-parse HEAD)" = "$TARGET"
git status -sb | tee -a "$EVID/build.meta"

# Capture V2 flags BEFORE so we can restore if compose rewrite touches them
ENV_FILE=.env.production
cp "$ENV_FILE" "backups/compose/.env.production.pre-deliver-notify-${SHORT}.bak" 2>/dev/null || mkdir -p backups/compose && cp "$ENV_FILE" "backups/compose/.env.production.pre-deliver-notify-${SHORT}.bak"
echo "FLAGS_BEFORE" | tee -a "$EVID/build.meta"
grep -E '^SNAPSHOT_V2_|^PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED|^PRICE_PLAN_TEST_ENTRY_ENABLED|^WECHAT_VIRTUAL_PAY_ENV=' "$ENV_FILE" | tee -a "$EVID/build.meta" || true

IMAGE_REF="local/xianzhi-ai-platform:git-${SHORT}"
export TARGET_PLATFORM=linux/amd64
echo "BUILD_START $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$EVID/build.meta"
docker build --pull \
  --platform linux/amd64 \
  --label "org.opencontainers.image.revision=${TARGET}" \
  --tag "$IMAGE_REF" \
  --tag "local/xianzhi-ai-platform:${SHORT}" \
  --file Dockerfile .
echo "BUILD_END $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$EVID/build.meta"

IMAGE_ID=$(docker image inspect --format '{{.Id}}' "$IMAGE_REF")
REV=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMAGE_REF")
{
  echo "IMAGE_REF=$IMAGE_REF"
  echo "IMAGE_ID=$IMAGE_ID"
  echo "LABEL_REVISION=$REV"
} | tee -a "$EVID/build.meta"
test "$REV" = "$TARGET"

if grep -q '^IMAGE_TAG=' "$ENV_FILE"; then
  sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=${SHORT}/" "$ENV_FILE"
else
  echo "IMAGE_TAG=${SHORT}" >> "$ENV_FILE"
fi

# Force V2 flags to remain true (user requirement for this remediation)
for k in SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED PRICE_PLAN_TEST_ENTRY_ENABLED; do
  if grep -q "^${k}=" "$ENV_FILE"; then
    sed -i "s/^${k}=.*/${k}=true/" "$ENV_FILE"
  else
    echo "${k}=true" >> "$ENV_FILE"
  fi
done
echo "FLAGS_AFTER" | tee -a "$EVID/build.meta"
grep -E '^IMAGE_TAG=|^SNAPSHOT_V2_|^PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED|^PRICE_PLAN_TEST_ENTRY_ENABLED|^WECHAT_VIRTUAL_PAY_ENV=' "$ENV_FILE" | tee -a "$EVID/build.meta"

docker compose -f compose.prod.yml --env-file "$ENV_FILE" up -d --no-deps --force-recreate xianzhi-ai
sleep 10
docker compose -f compose.prod.yml --env-file "$ENV_FILE" ps xianzhi-ai | tee -a "$EVID/build.meta"
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV | tee -a "$EVID/build.meta"
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 ls /app 2>/dev/null | head -5 | tee -a "$EVID/build.meta" || true
# Confirm binary includes notify symbol if strings available
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 sh -lc 'strings /app/xianzhi-ai 2>/dev/null | grep -F notify_provide_goods | head -3' | tee -a "$EVID/build.meta" || true
echo DONE | tee -a "$EVID/build.meta"
