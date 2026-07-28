# Gate amendment proposal — local-only immutable IMAGE_ID (§1)

**FilledAt:** 2026-07-29T07:50:00+08:00  
**Verdict:** registry credentials **CONFIRMED ABSENT** — cannot produce real `repository@sha256:...` RepoDigest without inventing one (forbidden).

## Deep probe (prod `root@119.29.191.227`)

| Check | Result |
|---|---|
| `/root/.docker/config.json` | **missing** (dir exists; only `buildx`) |
| Other `*/.docker/config.json` under `/home|/opt|/srv|/var/www|/root` | **none** |
| Env vars matching `REGISTRY|HARBOR|CCR|ACR|TCR|DOCKER_AUTH|…` | **none** |
| Compose / `.env*` registry hostnames (`ccr.|harbor|aliyuncs|tencentcloudcr|ghcr` as image repo) | **none** — only local `${XIANZHI_IMAGE}:${IMAGE_TAG}` |
| `XIANZHI_IMAGE` / `IMAGE_TAG` | `local/xianzhi-ai-platform` / `a39485ef1` |
| `docker login` / `docker push` in `/opt/zhiqiyun-ai/deploy.sh`, `/root/install_panel.sh` | **no hits** |
| Host `docker info` IndexConfigs | only `docker.io` with Tencent **pull mirror** `mirror.ccs.tencentyun.com` — **not** push credentials |
| Local workstation `~/.docker/config.json` | `auths: {}` / `credsStore=desktop` — empty |
| Repo CI (`.github`) CCR/ACR/GHCR push secrets/workflows for this image | **none found** |
| Running container | `zhiqiyun-ai-prod-xianzhi-ai-1` → `local/xianzhi-ai-platform:a39485ef1` |
| `RepoDigests` | **`[]`** |
| Image ID | `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` |
| V2 flags / pay env (unchanged this action) | `true` / `true` / `true` / `WECHAT_VIRTUAL_PAY_ENV=production` |

Raw: `host-out/probe-registry.txt`, `host-out/probe-registry-deep.txt`, `probe-registry-deep.sh`.

## Historical deploy model

Production has been shipping via **on-host build / local tag** (`local/xianzhi-ai-platform:<gitShort>`) for many prior releases (see local tag history in probe out). There is **no** established registry push path in compose, deploy.sh, or host docker auth.

## Recommendation (needs product-owner accept)

| Option | Meaning | Recommendation |
|---|---|---|
| A. Provision registry + push current IMAGE_ID → capture real RepoDigest | Strict Gate A wording | Best long-term; **blocked today** (no creds) |
| B. **PASS-WITH-LOCAL-IMMUTABLE** for §1 | Accept `local/xianzhi-ai-platform:a39485ef1` + `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` + git `a39485ef159dabf348a71059a0e922af4894ab5a` as immutable rollback identity for **this host-local deploy model** | **RECOMMENDED for §1 close** while deploy stays local-only |
| C. Keep §1 residual / NO-GO forever until registry exists | Strictest | Valid if PO refuses B |

**Do not** write a fake `repoDigest` string. `release-manifest.json.repoDigest` stays `null`.

### Close criteria if PO accepts B

§1 status becomes **`PASS-WITH-LOCAL-IMMUTABLE`** when all of:

1. Running image Id == `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32`
2. Tags include `local/xianzhi-ai-platform:a39485ef1` (and/or `…:git-a39485ef1`)
3. `RepoDigests=[]` documented as expected under local-only model
4. Evidence probe files retained under `evidence/20260729/repo-digest/`
5. Rollback runbook uses **IMAGE_ID + local tag**, not registry digest
6. Future registry adoption re-opens Gate A image row to require real RepoDigest

Overall GO/NO-GO still requires role-7 sign-off; this amendment only clears the **RepoDigest residual** as an accepted local-immutable substitute.

## Ask (product owner)

> Do you accept **local IMAGE_ID** `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` (+ tag `local/xianzhi-ai-platform:a39485ef1` / git `a39485ef159dabf348a71059a0e922af4894ab5a`) as §1 close criteria under status **`PASS-WITH-LOCAL-IMMUTABLE`**, given confirmed absence of registry credentials and historical local-only deploys?

- [ ] **ACCEPT** → update HANDOFF §1 / go-no-go rolling table to `PASS-WITH-LOCAL-IMMUTABLE`; keep `repoDigest: null`
- [ ] **REJECT** → §1 remains **residual / NO-GO** until registry write creds + push

**Agent recommendation:** ACCEPT (B) for this release train; schedule registry (GHCR/TCR) as follow-up hardening, not a blocker for already-validated ¥1 payment path + waived ¥996 policy.

Signer: __________  Date: __________
