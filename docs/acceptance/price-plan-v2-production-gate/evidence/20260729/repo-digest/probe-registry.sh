#!/bin/bash
set -euo pipefail
cd /opt/zhiqiyun-ai
CID=$(docker ps -q --filter name=zhiqiyun-ai-prod-xianzhi-ai | head -n1)
IMG=$(docker inspect -f '{{.Image}}' "$CID")
echo "CID=$CID"
echo "CONTAINER_IMAGE_ID=$IMG"
echo "CONFIG_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$CID")"
echo "=== IMAGE INSPECT ==="
docker image inspect -f 'Id={{.Id}}
RepoTags={{json .RepoTags}}
RepoDigests={{json .RepoDigests}}
Created={{.Created}}
Architecture={{.Architecture}}
Os={{.Os}}' "$IMG"
echo "=== FLAGS ==="
docker exec "$CID" printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV
echo "=== ENV IMAGE ==="
grep -E '^(IMAGE_TAG|XIANZHI_IMAGE)=' .env.production
echo "=== DOCKER AUTH ==="
if [ -f /root/.docker/config.json ]; then
  python3 - <<'PY'
import json
d=json.load(open("/root/.docker/config.json"))
print("auths:", sorted(d.get("auths",{}).keys()))
print("credsStore:", d.get("credsStore"))
print("credHelpers:", d.get("credHelpers"))
PY
else
  echo "NO_DOCKER_CONFIG"
fi
echo "=== LOCAL TAGS ==="
docker images --format '{{.Repository}}:{{.Tag}}\t{{.ID}}\t{{.Digest}}' | grep -E 'xianzhi-ai-platform|zhiqiyun' | head -n 40
echo "=== GIT ==="
git rev-parse HEAD
git log -1 --oneline
echo "=== COMPOSE IMAGE LINE ==="
grep -n 'image:.*XIANZHI_IMAGE\|IMAGE_TAG' compose.prod.yml | head -n 5
echo "=== SANDBOX KEYS ==="
grep -E '^WECHAT_VIRTUAL_PAY_(ENV|OFFER_ID|MODE)=' .env.production | sed 's/=.*/=***/'
grep -E '^WECHAT_VIRTUAL_PAY_APP_KEY=' .env.production | sed 's/=.*/=PRESENT/'
grep -E '^WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY=' .env.production | sed 's/=.*/=PRESENT/'
# Check for empty sandbox key
python3 - <<'PY'
import pathlib
text=pathlib.Path('/opt/zhiqiyun-ai/.env.production').read_text(encoding='utf-8', errors='ignore')
for line in text.splitlines():
    if line.startswith('WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY='):
        v=line.split('=',1)[1].strip().strip('"').strip("'")
        print('SANDBOX_APP_KEY_LEN=', len(v))
        print('SANDBOX_APP_KEY_EMPTY=', len(v)==0)
    if line.startswith('WECHAT_VIRTUAL_PAY_APP_KEY='):
        v=line.split('=',1)[1].strip().strip('"').strip("'")
        print('PROD_APP_KEY_LEN=', len(v))
        print('PROD_APP_KEY_EMPTY=', len(v)==0)
    if line.startswith('WECHAT_VIRTUAL_PAY_OFFER_ID='):
        print('OFFER_ID=', line.split('=',1)[1].strip())
    if line.startswith('WECHAT_VIRTUAL_PAY_MODE='):
        print('MODE=', line.split('=',1)[1].strip())
PY
echo "=== DONE ==="
