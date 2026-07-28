# RepoDigest residual — 2026-07-29

## Verdict

**§1 RepoDigest = residual / NOT PASS.** Did **not** invent a digest.

## Probe (prod `119.29.191.227` / `/opt/zhiqiyun-ai`)

| Item | Value |
|---|---|
| Running image ref | `local/xianzhi-ai-platform:a39485ef1` |
| Image ID (immutable local) | `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` |
| RepoTags | `local/xianzhi-ai-platform:a39485ef1`, `local/xianzhi-ai-platform:git-a39485ef1` |
| RepoDigests | **`[]` (empty)** |
| Deploy git SHA | `a39485ef159dabf348a71059a0e922af4894ab5a` |
| Compose image | `${XIANZHI_IMAGE:-xianzhi-ai-platform}:${IMAGE_TAG:-prod}` → local tag only |
| `.env.production` | `XIANZHI_IMAGE=local/xianzhi-ai-platform` / `IMAGE_TAG=a39485ef1` |
| `/root/.docker/config.json` | **missing** — no registry auths |
| GHCR / Docker Hub / Tencent TCR creds on host | **not found** (env + docker config) |

## Attempted path

Tried to locate a pushable registry that would yield `repository@sha256:...`.

Blocked by: no logged-in registry, no `auths` file, compose path is local-tag only. Pushing to anonymous Docker Hub / GHCR without credentials is impossible.

## What true RepoDigest requires (ops)

1. Provision registry write credentials (GHCR / TCR / Docker Hub) on the deploy host or CI.
2. `docker login <registry>`
3. Tag current immutable image:
   ```bash
   docker tag local/xianzhi-ai-platform:a39485ef1 <registry>/<repo>:a39485ef1
   docker push <registry>/<repo>:a39485ef1
   ```
4. Record:
   ```bash
   docker image inspect --format '{{index .RepoDigests 0}}' <registry>/<repo>:a39485ef1
   ```
5. Point compose/`XIANZHI_IMAGE` at that registry repo (still pin by digest in release manifest).
6. Update `release-manifest.json` `repoDigest` and HANDOFF §1.

## Residual substitute (NOT a digest)

Until registry exists, ops may track:

```text
imageId=sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32
gitSha=a39485ef159dabf348a71059a0e922af4894ab5a
tag=local/xianzhi-ai-platform:a39485ef1
```

This proves local immutability for the current host only — **not** Gate A registry RepoDigest.

Evidence files: `probe-registry.sh`, `probe-registry.out` (copied from host `/tmp/probe-registry.out`).
