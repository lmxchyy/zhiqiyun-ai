import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { access, chmod, copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const root = new URL("../", import.meta.url);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

function toBashPath(p) {
  const normalized = p.replaceAll("\\", "/");
  if (process.platform === "win32" && /^[A-Za-z]:/.test(normalized)) {
    return "/" + normalized[0].toLowerCase() + normalized.slice(2);
  }
  return normalized;
}

async function source(path) {
  return (await readFile(new URL(path, root), "utf8")).replaceAll("\r\n", "\n");
}

async function setupSandbox(options = {}) {
  const dir = await mkdtemp(join(tmpdir(), "xianzhi-immutable-deploy-"));
  const binDir = join(dir, "bin");
  const opsDir = join(dir, "ops");
  await mkdir(binDir, { recursive: true });
  await mkdir(opsDir, { recursive: true });
  await mkdir(join(dir, ".git"), { recursive: true });

  const bashBin = toBashPath(binDir);
  const bashDir = toBashPath(dir);

  await copyFile(new URL("deploy.sh", root), join(dir, "deploy.sh"));
  await copyFile(new URL("rollback.sh", root), join(dir, "rollback.sh"));
  await copyFile(new URL("ops/verify-release-manifest.sh", root), join(opsDir, "verify-release-manifest.sh"));
  await copyFile(new URL("ops/disk-guard.sh", root), join(opsDir, "disk-guard.sh"));
  await chmod(join(dir, "deploy.sh"), 0o755);
  await chmod(join(dir, "rollback.sh"), 0o755);
  await chmod(join(opsDir, "verify-release-manifest.sh"), 0o755);
  await chmod(join(opsDir, "disk-guard.sh"), 0o755);

  const composeContent = `services:
  xianzhi-ai:
    image: \${XIANZHI_IMAGE_REFERENCE:-\${XIANZHI_IMAGE:-xianzhi-ai-platform}:\${IMAGE_TAG:-prod}}
  smartvideo-worker:
    image: \${XIANZHI_IMAGE_REFERENCE:-\${XIANZHI_IMAGE:-xianzhi-ai-platform}:\${IMAGE_TAG:-prod}}
`;
  await writeFile(join(dir, "compose.prod.yml"), composeContent, "utf8");

  const envContent = options.envContent !== undefined ? options.envContent : `# Production Database Secret
DATABASE_URL="postgres://user:super_secret_pw@10.0.0.1:5432/prod"
STORAGE_MASTER_KEY="0123456789abcdef0123456789abcdef"
CONNECTOR_SECRET_ENCRYPTION_KEY="0123456789abcdef0123456789abcdef"
# Immutable releases set this to the exact registry image@sha256:digest at deploy time
# XIANZHI_IMAGE_REFERENCE=
`;
  await writeFile(join(dir, ".env.production"), envContent, { mode: 0o600 });
  await chmod(join(dir, ".env.production"), 0o600);

  const manifest = options.manifest !== undefined ? options.manifest : {
    git_sha: "97361d7fe4cfcd32cce644b153532be480ad721a",
    image: "ghcr.io/lmxchyy/zhiqiyun-ai",
    digest: "sha256:" + "a".repeat(64),
    image_reference: "ghcr.io/lmxchyy/zhiqiyun-ai@sha256:" + "a".repeat(64),
    built_at: "2026-08-24T00:00:00Z",
    production_contract: "passed"
  };
  await writeFile(join(dir, "release-manifest.json"), typeof manifest === "string" ? manifest : JSON.stringify(manifest), "utf8");

  await writeFile(join(binDir, "df"), `#!/bin/sh
cat << 'EOF'
Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/root 100000000 10000000 90000000 10% /
EOF
`, { mode: 0o755 });

  await writeFile(join(binDir, "git"), `#!/bin/sh
case "$1" in
  diff) exit 0 ;;
  symbolic-ref) echo "main"; exit 0 ;;
  fetch|pull|checkout) exit 0 ;;
  rev-parse) echo "\${MOCK_GIT_SHA:-97361d7fe4cfcd32cce644b153532be480ad721a}"; exit 0 ;;
  *) exit 0 ;;
esac
`, { mode: 0o755 });

  await writeFile(join(binDir, "docker"), `#!/bin/sh
if [ "$1" = "compose" ]; then
  shift
  ENV_FILE_PATH=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --env-file)
        ENV_FILE_PATH="$2"
        shift 2
        ;;
      -f|--file)
        shift 2
        ;;
      version)
        echo "Docker Compose version v2.24.0"
        exit 0
        ;;
      config)
        if [ "\${MOCK_DESIRED_STATE_MISMATCH:-0}" = "1" ]; then
          cat << 'EOF'
{"services":{"xianzhi-ai":{"image":"xianzhi-ai-platform:prod","environment":{"VIDEO_STORAGE_PERSISTENCE_ENABLED":"true"}},"smartvideo-worker":{"image":"xianzhi-ai-platform:prod"}}}
EOF
          exit 0
        fi

        RESOLVED_REF="\${XIANZHI_IMAGE_REFERENCE:-}"
        if [ -z "$RESOLVED_REF" ] && [ -n "$ENV_FILE_PATH" ] && [ -f "$ENV_FILE_PATH" ]; then
          RESOLVED_REF="$(grep -E '^XIANZHI_IMAGE_REFERENCE=' "$ENV_FILE_PATH" 2>/dev/null | tail -n 1 | cut -d= -f2-)"
        fi
        RESOLVED_REF="\${RESOLVED_REF:-xianzhi-ai-platform:prod}"
        VIDEO_STORAGE_PERSISTENCE="\${MOCK_VIDEO_STORAGE_PERSISTENCE_ENABLED:-}"
        if [ -z "$VIDEO_STORAGE_PERSISTENCE" ] && [ -n "$ENV_FILE_PATH" ] && [ -f "$ENV_FILE_PATH" ]; then
          VIDEO_STORAGE_PERSISTENCE="$(grep -E '^VIDEO_STORAGE_PERSISTENCE_ENABLED=' "$ENV_FILE_PATH" 2>/dev/null | tail -n 1 | cut -d= -f2-)"
        fi
        VIDEO_STORAGE_PERSISTENCE="\${VIDEO_STORAGE_PERSISTENCE:-true}"
        if printf '%s\n' "$*" | grep -q -- '--format json'; then
          printf '{"services":{"xianzhi-ai":{"image":"%s","environment":{"VIDEO_STORAGE_PERSISTENCE_ENABLED":"%s"}},"smartvideo-worker":{"image":"%s"}}}\n' "$RESOLVED_REF" "$VIDEO_STORAGE_PERSISTENCE" "$RESOLVED_REF"
        else
          cat << EOF
services:
  xianzhi-ai:
    image: $RESOLVED_REF
  smartvideo-worker:
    image: $RESOLVED_REF
EOF
        fi
        exit 0
        ;;
      rm|pull|up)
        exit 0
        ;;
      ps)
        if [ "\${1:-}" = "-q" ] || [ "\${2:-}" = "-q" ]; then
          echo "mock_container_id"
        else
          echo "NAME IMAGE STATUS"
        fi
        exit 0
        ;;
      logs)
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
  exit 0
fi

if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  if printf '%s\n' "$*" | grep -q -- 'RepoDigests'; then
    printf '%s\n' "\${XIANZHI_IMAGE_REFERENCE:-ghcr.io/lmxchyy/zhiqiyun-ai@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
  else
    echo "sha256:target_image_1111111111111111111111111111111111111111111111111111111111111111"
  fi
  exit 0
fi

if [ "$1" = "inspect" ]; then
  case "\${3:-}" in
    *State.Running*)
      echo "false"
      ;;
    *State.Status*)
      echo "exited"
      ;;
    *Config.Image*)
      if [ "\${MOCK_CONFIG_IMAGE_MISMATCH:-0}" = "1" ]; then
        echo "ghcr.io/lmxchyy/zhiqiyun-ai:stale"
      else
        echo "\${XIANZHI_IMAGE_REFERENCE:-ghcr.io/lmxchyy/zhiqiyun-ai@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
      fi
      ;;
    *)
      if [ "\${MOCK_RUNNING_IMAGE_MISMATCH:-0}" = "1" ]; then
        echo "sha256:stale_mismatched_image_222222222222222222222222222222222222222222222222222222"
      else
        echo "sha256:target_image_1111111111111111111111111111111111111111111111111111111111111111"
      fi
      ;;
  esac
  exit 0
fi

if [ "$1" = "image" ] && [ "$2" = "prune" ]; then
  exit 0
fi

exit 0
`, { mode: 0o755 });

  return { dir, binDir, bashBin, bashDir };
}

async function runDeploy(sandbox, env = {}) {
  const { bashBin, bashDir } = sandbox;
  const script = `${bashDir}/deploy.sh`;
  return execFileAsync(bash, ["-c", `export PATH='${bashBin}':"$PATH"; cd '${bashDir}'; '${script}'`], {
    env: {
      ...process.env,
      TARGET_PLATFORM: "linux/amd64",
      ...env
    }
  });
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
  assert.match(deploy, /docker image inspect .*XIANZHI_IMAGE_REFERENCE/);
  assert.match(deploy, /docker inspect --format '\{\{\.Image\}\}'/);
  assert.match(deploy, /bash ops\/verify-release-manifest\.sh/);
  assert.match(rollback, /RELEASE_MANIFEST/);
  assert.match(rollback, /--no-build/);
  assert.match(rollback, /IMMUTABLE_RELEASE/);
  assert.match(rollback, /bash ops\/verify-release-manifest\.sh/);
});

test("release manifest validator and CI workflow exist", async () => {
  await access(new URL("ops/verify-release-manifest.sh", root));
  await access(new URL(".github/workflows/immutable-image-release.yml", root));
  const workflow = await source(".github/workflows/immutable-image-release.yml");
  assert.match(workflow, /docker\/build-push-action|docker buildx build/);
  assert.match(workflow, /production-contract/);
  assert.match(workflow, /digest/);
});

test("main release pushes the tested image to GHCR and ACR without rebuilding", async () => {
  const workflow = await source(".github/workflows/immutable-image-release.yml");
  assert.match(workflow, /ALIYUN_REGISTRY/);
  assert.match(workflow, /ALIYUN_USERNAME/);
  assert.match(workflow, /ALIYUN_PASSWORD/);
  assert.match(workflow, /docker login/);
  assert.match(workflow, /docker tag \"\$IMAGE:\$IMAGE_TAG\"/);
  assert.match(workflow, /docker push \"\$ACR_IMAGE:\$IMAGE_TAG\"/);
  assert.match(workflow, /imagetools inspect/);
  assert.match(workflow, /acr_digest/);
  assert.equal((workflow.match(/docker buildx build/g) || []).length, 1);
});

test("immutable deploy preserves registry selection for manifest verification", async () => {
  const deploy = await source("deploy.sh");
  assert.match(deploy, /RELEASE_REGISTRY/);
  assert.match(deploy, /verify-release-manifest/);
});

test("immutable deployment verifies both container configuration and image digest", async () => {
  const deploy = await source("deploy.sh");
  const rollback = await source("rollback.sh");
  assert.match(deploy, /Config\.Image/);
  assert.match(deploy, /RepoDigests/);
  assert.match(deploy, /PARTIAL_RELEASE_DETECTED/);
  assert.match(rollback, /update_env_file_key/);
  assert.match(rollback, /--no-build/);
});

test("legacy deploy entrypoints stop before cutover without authenticated runtime proof", async () => {
  // These lightweight sandboxes intentionally contain no signed Prestage proof.
  // Post-proof immutable image, compose, and persistence behavior is exercised by
  // the Docker production-contract harness; do not stub the production verifier.
  const sandbox = await setupSandbox();
  const initialEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");

  await assert.rejects(
    runDeploy(sandbox, {
      IMMUTABLE_RELEASE: "1",
      RELEASE_MANIFEST: "release-manifest.json"
    }),
    (error) => {
      assert.equal(error.code, 1);
      assert.match(error.stderr, /RUNTIME_CAPABILITY_PROOF_REQUIRED/);
      return true;
    }
  );

  assert.equal(await readFile(join(sandbox.dir, ".env.production"), "utf8"), initialEnv);
});

test("legacy rollback entrypoint stops before mutation without authenticated runtime proof", async () => {
  const sandbox = await setupSandbox();
  const initialEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");

  await assert.rejects(
    execFileAsync(
      bash,
      ["-c", `export PATH='${sandbox.bashBin}':"$PATH"; cd '${sandbox.bashDir}'; ./rollback.sh release-manifest.json`],
      { env: { ...process.env, IMMUTABLE_RELEASE: "1", TARGET_PLATFORM: "linux/amd64" } }
    ),
    (error) => {
      assert.equal(error.code, 1);
      assert.match(error.stderr, /RUNTIME_CAPABILITY_PROOF_MISSING/);
      return true;
    }
  );

  assert.equal(await readFile(join(sandbox.dir, ".env.production"), "utf8"), initialEnv);
});
