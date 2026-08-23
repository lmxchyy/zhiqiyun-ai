import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);

async function source(path) {
  return (await readFile(new URL(path, root), "utf8")).replaceAll("\r\n", "\n");
}

test("production compose exposes one digest-capable image reference for API and worker", async () => {
  const compose = await source("compose.prod.yml");
  assert.match(compose, /XIANZHI_IMAGE_REFERENCE/);
  assert.equal((compose.match(/image: \$\{XIANZHI_IMAGE_REFERENCE/g) || []).length, 2);
});

test("immutable deploy and rollback reject rebuild paths", async () => {
  const deploy = await source("deploy.sh");
  const rollback = await source("rollback.sh");
  assert.match(deploy, /IMMUTABLE_RELEASE/);
  assert.match(deploy, /--no-build/);
  assert.match(deploy, /RepoDigests/);
  assert.match(rollback, /RELEASE_MANIFEST/);
  assert.match(rollback, /--no-build/);
  assert.match(rollback, /IMMUTABLE_RELEASE/);
});

test("release manifest validator and CI workflow exist", async () => {
  await access(new URL("ops/verify-release-manifest.sh", root));
  await access(new URL(".github/workflows/immutable-image-release.yml", root));
  const workflow = await source(".github/workflows/immutable-image-release.yml");
  assert.match(workflow, /docker\/build-push-action|docker buildx build/);
  assert.match(workflow, /production-contract/);
  assert.match(workflow, /digest/);
});
