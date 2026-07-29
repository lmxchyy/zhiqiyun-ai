# RepoDigest residual → local-immutable — 2026-07-29

## Verdict

**No usable container registry credentials** (deep probe). Real registry RepoDigest **cannot** be produced. **Did not invent a digest.**

§1 status: **`PASS-WITH-LOCAL-IMMUTABLE`** — product owner **ACCEPTED** at **2026-07-29T09:44:00+08:00** with strict conditions (full SHAs, IMAGE_ID before/after verify, tar+SHA256, previous tar, forbidden `--build`/retag overwrite, registry long-term note). See `AMENDMENT-LOCAL-IMMUTABLE.md`.

## Immutable local identity (FULL — never truncate in gate records)

```text
imageRef=local/xianzhi-ai-platform:a39485ef1
imageId=sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32
gitSha=a39485ef159dabf348a71059a0e922af4894ab5a
RepoDigests=[]
WECHAT_VIRTUAL_PAY_ENV=production
V2 flags=true/true/true
```

## Tar artifacts (prod)

| Role | Path | SHA256 |
|---|---|---|
| Current | `/opt/zhiqiyun-ai/release-artifacts/images/xianzhi-ai-platform-a39485ef159dabf348a71059a0e922af4894ab5a.tar` | `4341d6b1cdac84d83fb2962729ac654684d2ec0ff90660f527da992016378d09` |
| Previous | `/opt/zhiqiyun-ai/release-artifacts/images/xianzhi-ai-platform-PREV-xianzhi-ai-platform-3d0c0e032-ead396384418.tar` | `4c08c51a41d6fc527e854c4e4c64988a35e9773622b5506433eb4d5a76f9a9ee` |

Previous IMAGE_ID: `sha256:ead3963844183429a30fc20f6a69eefaf264df882afa425c8e406502b242a331` (`local/xianzhi-ai-platform:3d0c0e032`).

## Forbidden (this release path)

- `docker compose up -d --build`（现场重构镜像禁止）
- Retag/overwrite same already-used image tag
- Inventing RepoDigest

Deploy MUST use pre-built immutable tag + IMAGE_ID exact match (or `docker load` from saved tar). See `release-freeze-runbook.md` §5b.

## Registry long-term

Registry `repository@sha256:...` remains the **long-term standard**. Local-immutable is **not** standing policy.

## Evidence files

- `AMENDMENT-LOCAL-IMMUTABLE.md`
- `probe-and-export-local-immutable.sh`
- `host-out/probe-and-export.txt` (same content as prod `/tmp/.../probe-and-export.log`; `.log` gitignored)
- `host-out/local-immutable-summary.env`
- `host-out/*.tar.sha256`
- `host-out/container-inspect.json`, `host-out/image-inspect.json`
- Earlier: `probe-registry*.sh`, `host-out/probe-registry*.txt`
