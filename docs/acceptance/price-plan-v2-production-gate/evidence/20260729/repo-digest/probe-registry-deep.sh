#!/bin/bash
set -euo pipefail
echo "=== docker config ==="
if [ -f /root/.docker/config.json ]; then
  sed -E 's/("auth"[[:space:]]*:[[:space:]]*")[^"]+/\1***REDACTED***/g; s/("identitytoken"[[:space:]]*:[[:space:]]*")[^"]+/\1***REDACTED***/g; s/("password"[[:space:]]*:[[:space:]]*")[^"]+/\1***REDACTED***/g' /root/.docker/config.json
  python3 - <<'PY'
import json
d=json.load(open("/root/.docker/config.json"))
print("AUTH_KEYS:", sorted((d.get("auths") or {}).keys()))
print("credsStore=", d.get("credsStore"))
print("credHelpers=", list((d.get("credHelpers") or {}).keys()))
PY
else
  echo "NO_/root/.docker/config.json"
fi
echo "=== ls /root/.docker ==="
ls -la /root/.docker/ 2>/dev/null || true
echo "=== find other docker configs ==="
find /home /opt /srv /var/www /root -maxdepth 5 -path "*/.docker/config.json" 2>/dev/null || true
echo "=== env registry var NAMES ==="
env | awk -F= 'BEGIN{IGNORECASE=1} $1 ~ /REGISTRY|HARBOR|CCR|ACR|DOCKER_AUTH|TENCENTCLOUDCR|ALIYUNCR|CONTAINER_REGISTRY|CCE_|TCR_/ {print $1"=***PRESENT***"}' || true
echo "=== compose/.env registry hits (redacted) ==="
while IFS= read -r f; do
  hits=$(grep -nEi 'registry|harbor|ccr\.|acr\.|tencentcloudcr|aliyuncs\.com|docker\.io|REPO_|IMAGE_' "$f" 2>/dev/null | sed -E 's/(PASSWORD|SECRET|TOKEN|KEY|AUTH|PASSWD)[=:].*/\1=***REDACTED***/Ig' | head -25 || true)
  if [ -n "${hits:-}" ]; then echo "FILE:$f"; echo "$hits"; echo "---"; fi
done < <(find /opt /root /home /srv /var/www /data -maxdepth 6 \( -name 'docker-compose*.yml' -o -name 'docker-compose*.yaml' -o -name 'compose*.yml' -o -name 'compose*.yaml' -o -name '.env' -o -name '.env.*' -o -name '*.env' \) 2>/dev/null | head -80)
echo "=== push/login script paths ==="
grep -RIlE 'docker (push|login)|ccr\.ccs|tencentcloudcr|aliyuncs|harbor' /opt /root /home /srv 2>/dev/null | head -40 || true
echo "=== running containers ==="
docker ps --format '{{.Names}}\t{{.Image}}\t{{.ID}}' 2>/dev/null | head -40
echo "=== xianzhi images ==="
docker images --format '{{.Repository}}:{{.Tag}}\t{{.ID}}\t{{.Digest}}' 2>/dev/null | grep -iE 'xianzhi|local/|git-' | head -40 || true
echo "=== inspect candidates ==="
for img in local/xianzhi-ai-platform local/xianzhi-ai-platform:latest git-a39485ef1; do
  if docker image inspect "$img" >/dev/null 2>&1; then
    echo "FOUND:$img"
    docker inspect "$img" --format 'Id={{.Id}} RepoTags={{json .RepoTags}} RepoDigests={{json .RepoDigests}}'
  else
    echo "MISS:$img"
  fi
done
echo "=== by IMAGE_ID prefix 1bd6777d ==="
docker images -a --no-trunc --format '{{.Repository}}:{{.Tag}} {{.ID}}' | grep -i '1bd6777d' | head -10 || true
echo "=== docker info IndexConfigs ==="
docker info -f '{{json .RegistryConfig.IndexConfigs}}' 2>/dev/null | head -c 2000 || true
echo
echo "DONE"