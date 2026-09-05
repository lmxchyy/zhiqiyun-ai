# Immutable Image Release

Phase 2 changes production application releases from server-side builds to a
CI-built, digest-pinned image.

```text
commit -> CI build once -> production-contract test
       -> registry push -> release manifest
       -> Gitee sync -> backup -> deploy by digest -> migration -> smoke
```

The API and `smartvideo-worker` use the same `XIANZHI_IMAGE_REFERENCE`. In an
immutable release it must contain a full `@sha256:<64 hex characters>` digest.
`deploy.sh` validates the manifest, checks the rendered Compose desired state,
pulls with Compose, starts with `--no-build`, and verifies both running
containers' `Config.Image`, image ID, and `RepoDigests` against the manifest.
Upon successful verification, `XIANZHI_IMAGE_REFERENCE` is atomically persisted
to `.env.production` and checked again through a fresh Compose render, so future
invocations preserve the immutable digest.

Pull requests build and test the image without publishing a release. A push to
`main` or `master` pushes the already-tested image to both GHCR and Aliyun ACR
without rebuilding. CI compares the remote OCI config and layer digests before
publishing a machine-readable `release-manifest.json` artifact. The manifest
records both registry references and digests, has no credentials, and can
select `aliyun_acr` as the preferred production registry.

An immutable deployment requires a manifest for the exact checked-out commit:

```bash
IMMUTABLE_RELEASE=1 \
RELEASE_REGISTRY=aliyun_acr \
RELEASE_MANIFEST=/opt/zhiqiyun-ai/release-manifest.json \
bash ./deploy.sh
```

`rollback.sh` accepts a previous release manifest when
`IMMUTABLE_RELEASE=1`; it pulls and starts that digest without rebuilding.
Image rollback is not database rollback. If a release ran a migration, schema
compatibility must be assessed before using an older image.

The legacy local build path remains available when `IMMUTABLE_RELEASE` is not
`1`, but it is not the production immutable release path. ACR credentials are
GitHub Actions Secrets for CI and a separately managed read-only registry
credential on production; they are never stored in the repository or manifest.
