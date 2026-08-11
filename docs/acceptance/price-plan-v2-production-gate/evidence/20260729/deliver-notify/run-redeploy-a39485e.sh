#!/bin/bash
set -euo pipefail
cd /opt/zhiqiyun-ai
EVID=/tmp/deliver-notify-deploy
mkdir -p "$EVID" backups/compose
BRANCH=codex/channel-ecosystem-v132-phase3
TARGET=a39485ef159dabf348a71059a0e922af4894ab5a

git fetch origin "+refs/heads/${BRANCH}:refs/remotes/origin/${BRANCH}"
git rev-parse "origin/${BRANCH}" | tee "$EVID/redeploy.meta"
test "$(git rev-parse origin/${BRANCH})" = "$TARGET"

git reset --hard HEAD || true
git clean -fd -e .env.production -e backups -e data -e 'admin-vue/dist' -e compose.oc009.override.yml -e ops-evidence || true
git checkout -f -B "$BRANCH" "$TARGET"
git reset --hard "$TARGET"
test "$(git rev-parse HEAD)" = "$TARGET"
SHORT=$(git rev-parse --short=9 HEAD)
echo "HEAD=$(git rev-parse HEAD) SHORT=$SHORT" | tee -a "$EVID/redeploy.meta"

# Ensure flags present and true in env file used for compose interpolation
ENV_FILE=.env.production
cp "$ENV_FILE" "backups/compose/.env.production.pre-redeploy-deliver-${SHORT}.bak"
for k in SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED PRICE_PLAN_TEST_ENTRY_ENABLED; do
  if grep -q "^${k}=" "$ENV_FILE"; then
    sed -i "s/^${k}=.*/${k}=true/" "$ENV_FILE"
  else
    echo "${k}=true" >> "$ENV_FILE"
  fi
done
if grep -q '^IMAGE_TAG=' "$ENV_FILE"; then
  sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=${SHORT}/" "$ENV_FILE"
else
  echo "IMAGE_TAG=${SHORT}" >> "$ENV_FILE"
fi
grep -E '^IMAGE_TAG=|^SNAPSHOT_V2_|^PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED|^PRICE_PLAN_TEST_ENTRY_ENABLED|^WECHAT_VIRTUAL_PAY_ENV=' "$ENV_FILE" | tee -a "$EVID/redeploy.meta"

# Confirm compose wires the flags
grep -n 'SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED\|PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED\|PRICE_PLAN_TEST_ENTRY_ENABLED' compose.prod.yml | tee -a "$EVID/redeploy.meta"

IMAGE_REF="local/xianzhi-ai-platform:git-${SHORT}"
export TARGET_PLATFORM=linux/amd64
echo "BUILD_START $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$EVID/redeploy.meta"
docker build --pull \
  --platform linux/amd64 \
  --label "org.opencontainers.image.revision=${TARGET}" \
  --tag "$IMAGE_REF" \
  --tag "local/xianzhi-ai-platform:${SHORT}" \
  --file Dockerfile .
echo "BUILD_END $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$EVID/redeploy.meta"
REV=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMAGE_REF")
echo "LABEL_REVISION=$REV" | tee -a "$EVID/redeploy.meta"
test "$REV" = "$TARGET"

docker compose -f compose.prod.yml --env-file "$ENV_FILE" up -d --no-deps --force-recreate xianzhi-ai
sleep 12
docker compose -f compose.prod.yml --env-file "$ENV_FILE" ps xianzhi-ai | tee -a "$EVID/redeploy.meta"
echo "CONTAINER_ENV" | tee -a "$EVID/redeploy.meta"
docker inspect zhiqiyun-ai-prod-xianzhi-ai-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E 'SNAPSHOT_V2|PRICE_PLAN_MEMBER_AGENT_CREATION|PRICE_PLAN_TEST_ENTRY|WECHAT_VIRTUAL_PAY_ENV' \
  | tee -a "$EVID/redeploy.meta"
# Must be true
docker inspect zhiqiyun-ai-prod-xianzhi-ai-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=true$'
docker inspect zhiqiyun-ai-prod-xianzhi-ai-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=true$'
docker inspect zhiqiyun-ai-prod-xianzhi-ai-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^PRICE_PLAN_TEST_ENTRY_ENABLED=true$'
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 sh -lc 'strings /app/xianzhi-api 2>/dev/null | grep -F notify_provide_goods | head -3' | tee -a "$EVID/redeploy.meta" || true
echo REDEPLOY_DONE | tee -a "$EVID/redeploy.meta"
