import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { access, chmod, copyFile, mkdir, readFile, stat, writeFile } from "node:fs/promises";
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
{"services":{"xianzhi-ai":{"image":"xianzhi-ai-platform:prod"},"smartvideo-worker":{"image":"xianzhi-ai-platform:prod"}}}
EOF
          exit 0
        fi

        RESOLVED_REF="\${XIANZHI_IMAGE_REFERENCE:-}"
        if [ -z "$RESOLVED_REF" ] && [ -n "$ENV_FILE_PATH" ] && [ -f "$ENV_FILE_PATH" ]; then
          RESOLVED_REF="$(grep -E '^XIANZHI_IMAGE_REFERENCE=' "$ENV_FILE_PATH" 2>/dev/null | tail -n 1 | cut -d= -f2-)"
        fi
        RESOLVED_REF="\${RESOLVED_REF:-xianzhi-ai-platform:prod}"
        if printf '%s\n' "$*" | grep -q -- '--format json'; then
          printf '{"services":{"xianzhi-ai":{"image":"%s"},"smartvideo-worker":{"image":"%s"}}}\n' "$RESOLVED_REF" "$RESOLVED_REF"
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

test("immutable deploy persists image reference to env file so fresh compose invocations resolve the digest", async () => {
  const sandbox = await setupSandbox();
  const targetDigest = "sha256:" + "a".repeat(64);
  const expectedRef = `ghcr.io/lmxchyy/zhiqiyun-ai@${targetDigest}`;

  const { stdout } = await runDeploy(sandbox, {
    IMMUTABLE_RELEASE: "1",
    RELEASE_MANIFEST: "release-manifest.json"
  });
  assert.match(stdout, /Persisted XIANZHI_IMAGE_REFERENCE to \.env\.production/);

  const envFile = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.match(envFile, new RegExp(`^XIANZHI_IMAGE_REFERENCE=${expectedRef}$`, "m"));

  // Fresh compose invocation without XIANZHI_IMAGE_REFERENCE in env
  const freshEnv = { ...process.env, TARGET_PLATFORM: "linux/amd64" };
  delete freshEnv.XIANZHI_IMAGE_REFERENCE;
  const { stdout: composeOut } = await execFileAsync(
    bash,
    ["-c", `export PATH='${sandbox.bashBin}':"$PATH"; cd '${sandbox.bashDir}'; docker compose -f compose.prod.yml --env-file .env.production config`],
    { env: freshEnv }
  );
  assert.match(composeOut, new RegExp(`image: ${expectedRef}`));
  assert.doesNotMatch(composeOut, /xianzhi-ai-platform:prod/);
});

test("immutable deploy fails early when compose desired state diverges from verified manifest digest", async () => {
  const sandbox = await setupSandbox();
  await assert.rejects(
    runDeploy(sandbox, {
      IMMUTABLE_RELEASE: "1",
      RELEASE_MANIFEST: "release-manifest.json",
      MOCK_DESIRED_STATE_MISMATCH: "1"
    }),
    (error) => {
      assert.equal(error.code, 1);
      assert.match(error.stderr, /desired image mismatch/);
      return true;
    }
  );

  const envFile = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.doesNotMatch(envFile, /ghcr\.io\/lmxchyy\/zhiqiyun-ai@sha256:/);
});

test("immutable deploy fails when running container image ID does not match expected image ID", async () => {
  const sandbox = await setupSandbox();
  await assert.rejects(
    runDeploy(sandbox, {
      IMMUTABLE_RELEASE: "1",
      RELEASE_MANIFEST: "release-manifest.json",
      MOCK_RUNNING_IMAGE_MISMATCH: "1"
    }),
    (error) => {
      assert.equal(error.code, 1);
      assert.match(error.stderr, /is not running the manifest image digest/);
      return true;
    }
  );

  const envFile = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.doesNotMatch(envFile, /ghcr\.io\/lmxchyy\/zhiqiyun-ai@sha256:/);
});

test("immutable deploy fails when API or worker Config.Image is stale", async () => {
  const sandbox = await setupSandbox();
  await assert.rejects(
    runDeploy(sandbox, {
      IMMUTABLE_RELEASE: "1",
      RELEASE_MANIFEST: "release-manifest.json",
      MOCK_CONFIG_IMAGE_MISMATCH: "1"
    }),
    (error) => {
      assert.equal(error.code, 1);
      assert.match(error.stderr, /Config\.Image is not the manifest reference/);
      return true;
    }
  );

  const envFile = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.doesNotMatch(envFile, /ghcr\.io\/lmxchyy\/zhiqiyun-ai@sha256:/);
});

test("immutable deploy rejects manifest missing or invalid digest before modifying environment", async () => {
  const invalidManifest = {
    git_sha: "97361d7fe4cfcd32cce644b153532be480ad721a",
    image: "ghcr.io/lmxchyy/zhiqiyun-ai",
    digest: "sha256:invalid-short",
    image_reference: "ghcr.io/lmxchyy/zhiqiyun-ai@sha256:invalid-short",
    built_at: "2026-08-24T00:00:00Z",
    production_contract: "passed"
  };
  const sandbox = await setupSandbox({ manifest: invalidManifest });
  const initialEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");

  await assert.rejects(
    runDeploy(sandbox, {
      IMMUTABLE_RELEASE: "1",
      RELEASE_MANIFEST: "release-manifest.json"
    }),
    (error) => {
      assert.equal(error.code, 1);
      assert.match(error.stderr, /invalid digest/);
      return true;
    }
  );

  const finalEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.equal(finalEnv, initialEnv);
});

test("immutable deploy is idempotent when re-run with identical manifest digest", async () => {
  const sandbox = await setupSandbox();
  await runDeploy(sandbox, {
    IMMUTABLE_RELEASE: "1",
    RELEASE_MANIFEST: "release-manifest.json"
  });
  const firstEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  const targetDigest = "sha256:" + "a".repeat(64);
  const expectedRef = `ghcr.io/lmxchyy/zhiqiyun-ai@${targetDigest}`;

  // Second deployment with the exact same manifest
  await runDeploy(sandbox, {
    IMMUTABLE_RELEASE: "1",
    RELEASE_MANIFEST: "release-manifest.json"
  });
  const secondEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");

  assert.equal(secondEnv, firstEnv);
  const matches = secondEnv.match(new RegExp(`^XIANZHI_IMAGE_REFERENCE=${expectedRef}$`, "gm")) || [];
  assert.equal(matches.length, 1);
});

test("immutable deploy preserves existing env secrets, comments, and file permissions without leaking to stdout", async () => {
  const secretDatabaseUrl = "postgres://user:super_secret_pw@10.0.0.1:5432/prod";
  const masterKey = "0123456789abcdef0123456789abcdef";
  const customComment = "# Vital production database credential - do not expose";
  const envContent = `${customComment}
DATABASE_URL="${secretDatabaseUrl}"
STORAGE_MASTER_KEY="${masterKey}"
CONNECTOR_SECRET_ENCRYPTION_KEY="${masterKey}"
# Immutable releases set this to the exact registry image@sha256:digest at deploy time
# XIANZHI_IMAGE_REFERENCE=
`;
  const sandbox = await setupSandbox({ envContent });

  const { stdout, stderr } = await runDeploy(sandbox, {
    IMMUTABLE_RELEASE: "1",
    RELEASE_MANIFEST: "release-manifest.json"
  });

  // Secrets must NEVER appear in stdout or stderr
  assert.doesNotMatch(stdout, /super_secret_pw/);
  assert.doesNotMatch(stderr, /super_secret_pw/);
  assert.doesNotMatch(stdout, new RegExp(masterKey));
  assert.doesNotMatch(stderr, new RegExp(masterKey));

  // Secrets and comments must remain intact in .env.production
  const updatedEnv = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.match(updatedEnv, new RegExp(customComment, "m"));
  assert.match(updatedEnv, new RegExp(`DATABASE_URL="${secretDatabaseUrl}"`, "m"));
  assert.match(updatedEnv, new RegExp(`STORAGE_MASTER_KEY="${masterKey}"`, "m"));

  // File permissions check (on platforms supporting POSIX mode)
  if (process.platform !== "win32") {
    const fileStat = await stat(join(sandbox.dir, ".env.production"));
    assert.equal(fileStat.mode & 0o777, 0o600);
  }
});

test("immutable rollback verifies running container image ID and persists target digest to env file", async () => {
  const rollbackDigest = "sha256:" + "b".repeat(64);
  const rollbackRef = `ghcr.io/lmxchyy/zhiqiyun-ai@${rollbackDigest}`;
  const rollbackManifest = {
    git_sha: "97361d7fe4cfcd32cce644b153532be480ad721a",
    image: "ghcr.io/lmxchyy/zhiqiyun-ai",
    digest: rollbackDigest,
    image_reference: rollbackRef,
    built_at: "2026-08-24T00:00:00Z",
    production_contract: "passed"
  };
  const sandbox = await setupSandbox({ manifest: rollbackManifest });

  const { stdout } = await execFileAsync(
    bash,
    ["-c", `export PATH='${sandbox.bashBin}':"$PATH"; cd '${sandbox.bashDir}'; ./rollback.sh release-manifest.json`],
    {
      env: {
        ...process.env,
        IMMUTABLE_RELEASE: "1",
        TARGET_PLATFORM: "linux/amd64"
      }
    }
  );

  assert.match(stdout, /Persisted XIANZHI_IMAGE_REFERENCE to \.env\.production/);
  const envFile = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.match(envFile, new RegExp(`^XIANZHI_IMAGE_REFERENCE=${rollbackRef}$`, "m"));
});
