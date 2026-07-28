# RepoDigest residual → local-immutable amendment — 2026-07-29

## Verdict

**No usable container registry credentials found** (deep re-probe). Real registry RepoDigest **cannot** be produced. **Did not invent a digest.**

§1 recommendation: **`PASS-WITH-LOCAL-IMMUTABLE`** pending product-owner accept — see `AMENDMENT-LOCAL-IMMUTABLE.md`.

Until PO checkbox is signed, treat §1 as **residual / PENDING-PO-ACCEPT** (not full registry PASS).

## Immutable local identity (current prod)

```text
imageRef=local/xianzhi-ai-platform:a39485ef1
imageId=sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32
gitSha=a39485ef159dabf348a71059a0e922af4894ab5a
RepoDigests=[]
WECHAT_VIRTUAL_PAY_ENV=production
V2 flags=true/true/true
```

## Probe summary

| Item | Value |
|---|---|
| Host | `119.29.191.227` `/opt/zhiqiyun-ai` |
| `/root/.docker/config.json` | missing — no auths |
| Registry env / compose remote repos | none |
| `deploy.sh` / `install_panel.sh` push/login | none |
| docker.io | pull mirror only (`mirror.ccs.tencentyun.com`) |
| Workstation docker auths | empty |

Evidence: `probe-registry.sh`, `probe-registry-deep.sh`, `host-out/probe-registry.txt`, `host-out/probe-registry-deep.txt`, `AMENDMENT-LOCAL-IMMUTABLE.md`.

## What true RepoDigest still requires (ops follow-up)

1. Provision GHCR / TCR / Hub write credentials
2. `docker login` → tag `local/xianzhi-ai-platform:a39485ef1` → `docker push`
3. Record `docker image inspect --format '{{index .RepoDigests 0}}'`
4. Pin compose to digest; set `release-manifest.json.repoDigest`

Until then, local IMAGE_ID is the only honest immutability proof for this deploy model.
