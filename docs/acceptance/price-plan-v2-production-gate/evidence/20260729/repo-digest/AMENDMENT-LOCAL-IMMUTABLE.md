# Gate amendment — local-only immutable IMAGE_ID (§1)

**FilledAt:** 2026-07-29T09:46:00+08:00  
**PO_ACCEPT_AT:** 2026-07-29T09:44:00+08:00  
**Verdict:** §1 = **`PASS-WITH-LOCAL-IMMUTABLE`** (product owner **ACCEPTED** with strict conditions below).  
Registry credentials remain **CONFIRMED ABSENT** — `repoDigest` stays `null` (never invent).

## PO accept (strict conditions — ALL enforced)

| # | Condition | Status |
|---|---|---|
| 1 | Record **FULL** release commit SHA and **FULL** IMAGE_ID (never truncate in acceptance record) | **DONE** |
| 2 | Verify running container IMAGE_ID matches recorded release IMAGE_ID exactly (before + after export) | **EXACT MATCH** |
| 3 | **FORBIDDEN** `docker compose up -d --build` for this release path（现场重构镜像禁止） | **DOCUMENTED + ENFORCED** |
| 4 | **FORBIDDEN** retagging/overwriting the same image tag（同一镜像标签禁止被重新覆盖）；tags immutable | **DOCUMENTED + ENFORCED** |
| 5 | Export current running image to tar on prod; SHA256 of tar as restore/rollback artifact | **DONE** |
| 6 | Preserve previous runnable image tar + SHA256 (or N/A with reason) | **DONE — previous EXPORTED** |
| 7 | Explicitly: **registry is the long-term standard**; local-immutable is **NOT** long-term policy | **STATED** |

## Immutable identity (FULL — do not shorten in gate records)

```text
releaseGitSha (FULL) = a39485ef159dabf348a71059a0e922af4894ab5a
imageId       (FULL) = sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32
imageRef             = local/xianzhi-ai-platform:a39485ef1
alsoTagged           = local/xianzhi-ai-platform:git-a39485ef1
RepoDigests          = []
label org.opencontainers.image.revision = a39485ef159dabf348a71059a0e922af4894ab5a
container            = zhiqiyun-ai-prod-xianzhi-ai-1
Config.Image         = local/xianzhi-ai-platform:a39485ef1
```

### Before / after IMAGE_ID verification (2026-07-29T09:45:26+08 → 09:45:31+08)

| Check | Value |
|---|---|
| BEFORE container `.Image` | `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` |
| Recorded release IMAGE_ID | `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` |
| AFTER container `.Image` (post `docker save`) | `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` |
| Result | **EXACT MATCH** (before and after) |

## Image tar artifacts (prod durable path)

| Role | Path | Tar SHA256 (FULL) |
|---|---|---|
| **Current** (running) | `/opt/zhiqiyun-ai/release-artifacts/images/xianzhi-ai-platform-a39485ef159dabf348a71059a0e922af4894ab5a.tar` | `4341d6b1cdac84d83fb2962729ac654684d2ec0ff90660f527da992016378d09` |
| **Previous** (rollback) | `/opt/zhiqiyun-ai/release-artifacts/images/xianzhi-ai-platform-PREV-xianzhi-ai-platform-3d0c0e032-ead396384418.tar` | `4c08c51a41d6fc527e854c4e4c64988a35e9773622b5506433eb4d5a76f9a9ee` |

Previous image identity:

```text
prevImageId (FULL) = sha256:ead3963844183429a30fc20f6a69eefaf264df882afa425c8e406502b242a331
prevTag            = local/xianzhi-ai-platform:3d0c0e032
prevAlsoTagged     = local/xianzhi-ai-platform:git-3d0c0e032
```

Checksum sidecars sit beside each tar (`*.tar.sha256`). Host evidence: `host-out/probe-and-export.log`, `host-out/local-immutable-summary.env`, `host-out/*.tar.sha256`.

## Forbidden operations (this release path)

1. **`docker compose up -d --build`** — 禁止现场重构镜像；deploy MUST use pre-built immutable tag + IMAGE_ID check.
2. **Retag / overwrite same tag** — 禁止对已使用的 tag（如 `a39485ef1` / `git-a39485ef1`）重新 `docker build` 后覆盖；新构建必须使用新的 full-commit-derived tag.
3. **Inventing** `repository@sha256:...` RepoDigest — forbidden while registry absent.

Allowed deploy pattern: load/use pre-built `local/xianzhi-ai-platform:<immutable-tag>` whose `docker inspect` Id equals the recorded FULL IMAGE_ID; or `docker load` from the saved tar and verify Id.

See also: `release-freeze-runbook.md` §5b (local-immutable path).

## Registry long-term note

**Container registry with real `repository@sha256:...` RepoDigest remains the long-term standard.**  
`PASS-WITH-LOCAL-IMMUTABLE` is a **host-local bridge** for this release train only (no registry write credentials). When registry credentials exist, Gate A image row **re-opens** to require a real RepoDigest; local-immutable is **not** standing policy.

## Deep probe (earlier; still valid)

| Check | Result |
|---|---|
| `/root/.docker/config.json` | **missing** (dir exists; only `buildx`) |
| Registry env / compose remote repos | **none** — only local `${XIANZHI_IMAGE}:${IMAGE_TAG}` |
| Host `docker info` IndexConfigs | docker.io pull mirror only — **not** push credentials |
| V2 flags / pay env (unchanged this action) | `true` / `true` / `true` / `WECHAT_VIRTUAL_PAY_ENV=production` |

Raw: `host-out/probe-registry.txt`, `host-out/probe-registry-deep.txt`, `probe-registry-deep.sh`, `probe-and-export-local-immutable.sh`.

## Ask (product owner) — CLOSED

> Do you accept **local IMAGE_ID** `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` (+ tag `local/xianzhi-ai-platform:a39485ef1` / git `a39485ef159dabf348a71059a0e922af4894ab5a`) as §1 close criteria under status **`PASS-WITH-LOCAL-IMMUTABLE`**, given confirmed absence of registry credentials and historical local-only deploys?

- [x] **ACCEPT** → HANDOFF §1 / go-no-go rolling table = `PASS-WITH-LOCAL-IMMUTABLE`; keep `repoDigest: null`; enforce tar + IMAGE_ID + forbidden ops
- [ ] **REJECT** → §1 remains **residual / NO-GO** until registry write creds + push

**PO_ACCEPT_AT:** 2026-07-29T09:44:00+08:00  
**Signer:** Product Owner (user message: ACCEPTED PASS-WITH-LOCAL-IMMUTABLE with STRICT conditions)  
**ExecutedBy:** Codex agent on prod `root@119.29.191.227`

Overall GO/NO-GO still requires other sections; this amendment **only** closes the **RepoDigest residual** under local-immutable substitute. **Do not invent overall GO** if §5/other remain PARTIAL.
