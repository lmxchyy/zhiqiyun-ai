import assert from "node:assert/strict";
import { execFile, execFileSync } from "node:child_process";
import { access, chmod, copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const root = new URL("../", import.meta.url);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";
const pythonBin = "python3";

function toBashPath(p) {
  const normalized = p.replaceAll("\\", "/");
  if (process.platform === "win32" && /^[A-Za-z]:/.test(normalized)) {
    return "/" + normalized[0].toLowerCase() + normalized.slice(2);
  }
  return normalized;
}

function createZipBuffer(filename, content) {
  const script = `import zipfile, io, sys
buf = io.BytesIO()
with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
    z.writestr(sys.argv[1], sys.argv[2])
sys.stdout.buffer.write(buf.getvalue())
`;
  return execFileSync(pythonBin, ["-c", script, filename, content]);
}

class MockGitHubTransport {
  constructor(stateDir) {
    this.stateDir = stateDir;
    this.stateFile = join(stateDir, ".mock_github_state.json");
    this.state = {
      status_code: 200,
      runs_by_sha: {},
      artifacts_by_run: {},
      zip_by_artifact: {}
    };
  }

  async save() {
    await writeFile(this.stateFile, JSON.stringify(this.state, null, 2), "utf8");
  }

  async setupCommit(sha, runId, artifactId, manifestObj, options = {}) {
    const branch = options.branch || "main";
    const event = options.event || "push";
    const status = options.status || "completed";
    const conclusion = options.conclusion || "success";
    const workflowPath = options.workflowPath || ".github/workflows/immutable-image-release.yml";
    const expired = options.expired !== undefined ? options.expired : false;
    const artifactName = options.artifactName || `release-manifest-${sha}`;

    const manifestStr = JSON.stringify(manifestObj, null, 2);
    const zipPath = join(this.stateDir, `artifact-${artifactId}.zip`);
    const zipBuf = createZipBuffer("release-manifest.json", manifestStr);
    await writeFile(zipPath, zipBuf);

    this.state.runs_by_sha[sha] = [
      {
        id: runId,
        name: "immutable-image-release",
        path: workflowPath,
        head_sha: sha,
        event: event,
        head_branch: branch,
        status: status,
        conclusion: conclusion,
        run_attempt: 1
      }
    ];

    this.state.artifacts_by_run[String(runId)] = [
      {
        id: artifactId,
        name: artifactName,
        expired: expired,
        workflow_run: { id: runId },
        archive_download_url: `https://api.github.com/repos/lmxchyy/zhiqiyun-ai/actions/artifacts/${artifactId}/zip`
      }
    ];

    this.state.zip_by_artifact[String(artifactId)] = zipPath;
    await this.save();
  }
}

const DEFAULT_SHA = "97361d7fe4cfcd32cce644b153532be480ad721a";
const PREV_SHA = "8888888888888888888888888888888888888888";
const DEFAULT_DIGEST = "sha256:" + "a".repeat(64);
const PREV_DIGEST = "sha256:" + "b".repeat(64);
const DEFAULT_IMAGE = "ghcr.io/lmxchyy/zhiqiyun-ai";
const DEFAULT_IMAGE_REF = `${DEFAULT_IMAGE}@${DEFAULT_DIGEST}`;
const PREV_IMAGE_REF = `${DEFAULT_IMAGE}@${PREV_DIGEST}`;
const DEFAULT_IMAGE_ID = "sha256:target_image_1111111111111111111111111111111111111111111111111111111111111111";
const PREV_IMAGE_ID = "sha256:prev_image_2222222222222222222222222222222222222222222222222222222222222222";

async function setupSandbox(options = {}) {
  const dir = await mkdtemp(join(tmpdir(), "xianzhi-prestage-test-"));
  const binDir = join(dir, "bin");
  const opsDir = join(dir, "ops");
  const pyFixtureDir = join(dir, "pyfixture");
  const migrationsDir = join(dir, "database", "migrations");
  const backupsDir = join(dir, "backups");
  await mkdir(binDir, { recursive: true });
  await mkdir(opsDir, { recursive: true });
  await mkdir(pyFixtureDir, { recursive: true });
  await mkdir(migrationsDir, { recursive: true });
  await mkdir(backupsDir, { recursive: true });

  const bashBin = toBashPath(binDir);
  const bashDir = toBashPath(dir);

  await copyFile(new URL("deploy.sh", root), join(dir, "deploy.sh"));
  await copyFile(new URL("rollback.sh", root), join(dir, "rollback.sh"));
  await copyFile(new URL("ops/verify-release-manifest.sh", root), join(opsDir, "verify-release-manifest.sh"));
  await copyFile(new URL("ops/disk-guard.sh", root), join(opsDir, "disk-guard.sh"));
  await copyFile(new URL("ops/run-migrations.sh", root), join(opsDir, "run-migrations.sh"));
  await copyFile(new URL("ops/prestage-release.sh", root), join(opsDir, "prestage-release.sh"));
  await copyFile(new URL("ops/verify-prestage-proof.sh", root), join(opsDir, "verify-prestage-proof.sh"));
  await copyFile(new URL("ops/verify-release-runtime.py", root), join(opsDir, "verify-release-runtime.py"));
  await copyFile(new URL("ops/verify-safe-drain.py", root), join(opsDir, "verify-safe-drain.py"));
  await copyFile(new URL("ops/enroll-quarantine.py", root), join(opsDir, "enroll-quarantine.py"));
  await copyFile(new URL("ops/quarantine-approval.py", root), join(opsDir, "quarantine-approval.py"));
  await copyFile(new URL("ops/quarantine-live-snapshot.py", root), join(opsDir, "quarantine-live-snapshot.py"));
  await copyFile(new URL("ops/quarantine-psql-transport.py", root), join(opsDir, "quarantine-psql-transport.py"));
  await copyFile(new URL("database/migrations/121-provider-execution-quarantine.sql", root), join(migrationsDir, "121-provider-execution-quarantine.sql"));

  await chmod(join(dir, "deploy.sh"), 0o755);
  await chmod(join(dir, "rollback.sh"), 0o755);
  await chmod(join(opsDir, "verify-release-manifest.sh"), 0o755);
  await chmod(join(opsDir, "disk-guard.sh"), 0o755);
  await chmod(join(opsDir, "run-migrations.sh"), 0o755);
  await chmod(join(opsDir, "prestage-release.sh"), 0o755);
  await chmod(join(opsDir, "verify-prestage-proof.sh"), 0o755);
  await chmod(join(opsDir, "enroll-quarantine.py"), 0o755);

  // In-memory HTTP transport interceptor (sitecustomize.py)
  // Intercepts requests strictly directed to https://api.github.com without any production code backdoors
  const siteCustomizeCode = `
import urllib.request, urllib.parse, json, os, io

_orig_open = urllib.request.OpenerDirector.open

def mock_open(self, fullurl, data=None, timeout=None):
    url = fullurl.full_url if hasattr(fullurl, 'full_url') else str(fullurl)
    state_file = os.environ.get('MOCK_GITHUB_STATE_FILE')
    if state_file and os.path.isfile(state_file) and 'api.github.com' in url:
        with open(state_file, 'r', encoding='utf-8') as f:
            state = json.load(f)
        status_code = state.get('status_code', 200)
        if status_code != 200:
            res = urllib.response.addinfourl(io.BytesIO(b'{"message":"Mock error"}'), {}, url)
            res.code = status_code
            res.msg = 'Mock error'
            return res

        if '/actions/runs' in url and '/artifacts' not in url:
            qs = urllib.parse.urlparse(url).query
            params = urllib.parse.parse_qs(qs)
            req_sha = params.get('head_sha', [''])[0]
            sha_runs = state.get('runs_by_sha', {}).get(req_sha)
            if sha_runs is not None:
                runs_list = sha_runs
            else:
                runs_list = state.get('runs_response', {}).get('workflow_runs', [])
            body = json.dumps({'workflow_runs': runs_list}).encode('utf-8')
            res = urllib.response.addinfourl(io.BytesIO(body), {'Content-Type': 'application/json'}, url)
            res.code = 200
            res.msg = 'OK'
            return res

        if '/artifacts' in url and '/zip' not in url:
            parts = url.split('/')
            run_id = parts[parts.index('runs') + 1] if 'runs' in parts else ''
            art_list = state.get('artifacts_by_run', {}).get(run_id)
            if art_list is not None:
                artifacts = art_list
            else:
                artifacts = state.get('artifacts_response', {}).get('artifacts', [])
            body = json.dumps({'artifacts': artifacts}).encode('utf-8')
            res = urllib.response.addinfourl(io.BytesIO(body), {'Content-Type': 'application/json'}, url)
            res.code = 200
            res.msg = 'OK'
            return res

        if '/zip' in url:
            parts = url.split('/')
            art_id = parts[parts.index('artifacts') + 1] if 'artifacts' in parts else ''
            z_path = state.get('zip_by_artifact', {}).get(art_id) or state.get('zip_path')
            zip_bytes = b''
            if z_path and os.path.isfile(z_path):
                with open(z_path, 'rb') as zf:
                    zip_bytes = zf.read()
            res = urllib.response.addinfourl(io.BytesIO(zip_bytes), {'Content-Type': 'application/zip'}, url)
            res.code = 200
            res.msg = 'OK'
            return res

    return _orig_open(self, fullurl, data=data, timeout=timeout)

urllib.request.OpenerDirector.open = mock_open
`;
  await writeFile(join(pyFixtureDir, "sitecustomize.py"), siteCustomizeCode, "utf8");

  const composeContent = `services:
  xianzhi-ai:
    image: \${XIANZHI_IMAGE_REFERENCE:-\${XIANZHI_IMAGE:-xianzhi-ai-platform}:\${IMAGE_TAG:-prod}}
    environment:
      VIDEO_STORAGE_PERSISTENCE_ENABLED: "true"
      MIGRATION_FILES: "\${MIGRATION_FILES:-001-init.sql}"
    volumes:
      - app-data:/app/data
  smartvideo-worker:
    image: \${XIANZHI_IMAGE_REFERENCE:-\${XIANZHI_IMAGE:-xianzhi-ai-platform}:\${IMAGE_TAG:-prod}}
    environment:
      VIDEO_STORAGE_PERSISTENCE_ENABLED: "true"
    volumes:
      - smartvideo-tmp:/tmp/smartvideo
  migrate:
    image: postgres:16-alpine
    environment:
      MIGRATION_FILES: "\${MIGRATION_FILES:-001-init.sql}"
volumes:
  app-data:
  smartvideo-tmp:
`;
  await writeFile(join(dir, "compose.prod.yml"), composeContent, "utf8");

  const envContent = options.envContent !== undefined ? options.envContent : `# Production Configuration
POSTGRES_DB=xianzhi
POSTGRES_USER=postgres
POSTGRES_PASSWORD=super_secret_pw
DATABASE_URL="postgres://postgres:super_secret_pw@10.0.0.1:5432/xianzhi"
STORAGE_MASTER_KEY="0123456789abcdef0123456789abcdef"
CONNECTOR_SECRET_ENCRYPTION_KEY="0123456789abcdef0123456789abcdef"
VIDEO_STORAGE_PERSISTENCE_ENABLED=true
XIANZHI_IMAGE_REFERENCE="${PREV_IMAGE_REF}"
MIGRATION_FILES="001-init.sql"
`;
  await writeFile(join(dir, ".env.production"), envContent, { mode: 0o600 });
  await chmod(join(dir, ".env.production"), 0o600);

  // Initial migration file
  await writeFile(join(migrationsDir, "001-init.sql"), "-- Initial migration\nSELECT 1;\n", "utf8");

  // Create .gitignore
  await writeFile(join(dir, ".gitignore"), ".prestage/\nbackups/\nbin/\npyfixture/\n.env\n.env.*\n*.zip\n.mock_github_state.json\n.mock_docker_*\n*.log\n", "utf8");

  // Previous release manifest (for rollback)
  const prevManifest = {
    git_sha: PREV_SHA,
    image: DEFAULT_IMAGE,
    digest: PREV_DIGEST,
    image_reference: PREV_IMAGE_REF,
    built_at: "2026-10-01T00:00:00Z",
    production_contract: "passed"
  };
  await writeFile(join(backupsDir, "release-manifest.json"), JSON.stringify(prevManifest, null, 2), "utf8");
  await writeFile(join(dir, "release-manifest.json"), JSON.stringify(prevManifest, null, 2), "utf8");

  // Mock df
  await writeFile(join(binDir, "df"), `#!/bin/sh
cat << 'EOF'
Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/root 100000000 10000000 90000000 10% /
EOF
`, { mode: 0o755 });

  // Mock docker
  await writeFile(join(binDir, "docker"), `#!/bin/sh
if [ -n "$MOCK_DOCKER_LOG" ]; then
  echo "$*" >> "$MOCK_DOCKER_LOG"
fi

STATE_DIR="\${MOCK_STATE_DIR:-${bashDir}}"

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
        RESOLVED_REF="\${XIANZHI_IMAGE_REFERENCE:-}"
        if [ -z "$RESOLVED_REF" ] && [ -n "$ENV_FILE_PATH" ] && [ -f "$ENV_FILE_PATH" ]; then
          RESOLVED_REF="$(grep -E '^[[:space:]]*XIANZHI_IMAGE_REFERENCE=' "$ENV_FILE_PATH" 2>/dev/null | tail -n 1 | cut -d= -f2- | tr -d '\"'\'')"
        fi
        RESOLVED_REF="\${RESOLVED_REF:-${PREV_IMAGE_REF}}"
        MIG_FILES="\${MIGRATION_FILES:-001-init.sql}"
        if printf '%s\n' "$@" | grep -q '^json$'; then
          printf '{"services":{"xianzhi-ai":{"image":"%s","environment":{"VIDEO_STORAGE_PERSISTENCE_ENABLED":"true","MIGRATION_FILES":"%s"},"volumes":[{"target":"/app/data"}]},"smartvideo-worker":{"image":"%s","environment":{"VIDEO_STORAGE_PERSISTENCE_ENABLED":"true"},"volumes":[{"target":"/tmp/smartvideo"}]},"migrate":{"image":"postgres:16-alpine","environment":{"MIGRATION_FILES":"%s"}}},"volumes":{"app-data":{},"smartvideo-tmp":{}}}\n' "$RESOLVED_REF" "$MIG_FILES" "$RESOLVED_REF" "$MIG_FILES" | python3 -c 'import json,os,sys;d=json.load(sys.stdin);d["name"]="mock-project";d["services"]["postgres"]={"image":"postgres:16-alpine","environment":{"POSTGRES_USER":"postgres","POSTGRES_DB":"xianzhi","POSTGRES_PASSWORD":"super_secret_pw"}};d["services"]["xianzhi-ai"]["environment"].update({k:os.environ[k] for k in ["DATABASE_URL","RABBITMQ_URL","GENERATION_ASYNC_CANARY_USERS","S3_BUCKET","S3_REGION","OBS_PREFIX"] if k in os.environ});print(json.dumps(d))'
        else
          cat << EOF
services:
  xianzhi-ai:
    image: $RESOLVED_REF
  smartvideo-worker:
    image: $RESOLVED_REF
  migrate:
    image: postgres:16-alpine
EOF
        fi
        exit 0
        ;;
      rm)
        exit 0
        ;;
      pull)
        if [ "\${MOCK_DOCKER_PULL_FAIL:-0}" = "1" ] || ( printf '%s\n' "$@" | grep -q "migrate" && [ "\${MOCK_PULL_MIGRATE_FAIL:-0}" = "1" ] ); then
          echo "Error response from daemon: pull access denied" >&2
          exit 1
        fi
        exit 0
        ;;
      stop)
        if [ "\${MOCK_STOP_FAIL:-0}" = "1" ]; then
          echo "Error stopping containers" >&2
          exit 1
        fi
        touch "$STATE_DIR/.mock_docker_stopped"
        exit 0
        ;;
      up)
        if printf '%s\n' "$@" | grep -q 'migrate'; then touch "$STATE_DIR/.mock_docker_migration"; fi
        rm -f "$STATE_DIR/.mock_docker_stopped"
        if ! printf '%s\n' "$@" | grep -q "migrate"; then
          touch "$STATE_DIR/.mock_docker_deployed"
        fi
        exit 0
        ;;
      ps)
        if printf '%s\\n' "$@" | grep -Fqx 'postgres'; then
          printf '%064d\\n' 1
          exit 0
        fi
        if [ "\${MOCK_POSTSTOP_PS_FAIL:-0}" = "1" ] && [ -f "$STATE_DIR/.mock_docker_stopped" ]; then exit 42; fi
        if [ "\${MOCK_DRAIN_PS_FAIL:-0}" = "1" ]; then
          echo "Error: docker compose ps command failed" >&2
          exit 1
        fi
        if [ "\${MOCK_NO_RUNNING_CONTAINERS:-0}" = "1" ]; then
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q "migrate"; then
          echo "mock_migrate_cid"
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q "smartvideo-worker"; then
          if printf '%s\n' "$@" | grep -Fqx "running" && [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
            exit 0
          fi
          echo "mock_worker_cid"
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q "xianzhi-ai"; then
          if printf '%s\n' "$@" | grep -Fqx "running" && [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
            exit 0
          fi
          echo "mock_api_cid"
          exit 0
        fi
        if printf '%s\n' "$@" | grep -Fqx "running"; then
          if [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
            exit 0
          fi
          echo "mock_api_cid"
          echo "mock_worker_cid"
          exit 0
        fi
        if printf '%s\n' "$@" | grep -Fqx "exited"; then
          if [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
            echo "mock_api_cid"
            echo "mock_worker_cid"
            exit 0
          fi
          exit 0
        fi
        if [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
          exit 0
        fi
        echo "mock_api_cid"
        echo "mock_worker_cid"
        exit 0
        ;;
      logs)
        echo "[mock logs] service logs output"
        exit 0
        ;;
      exec)
        if printf '%s\n' "$@" | grep -q '/metrics'; then
          if [ "\${MOCK_SCHEDULER_FAIL:-0}" = "1" ]; then exit 42; fi
          printf 'generation_scheduler_db_scrape_success 1\\ngeneration_scheduler_errors_total 0\\ngeneration_scheduler_dispatched_total 0\\ngeneration_scheduler_recovered_total 0\\n'
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q 'python3'; then
          if printf '%s\n' "$@" | grep -q 'urllib'; then
            if [ "\${MOCK_CONSUMER_FAIL:-0}" = "1" ]; then exit 42; fi
            python3 -c 'import json; print(json.dumps([dict(name="x.ai.generation."+n,consumers=1,messages_ready=0,messages_unacknowledged=0) for n in ["image.normal","image.canary","video.canary","ppt.canary"]]))'
          else
            echo '{"GENERATION_FAIR_SCHEDULER_ENABLED":"true","ASYNC_MESSAGING_ENABLED":"true"}'
          fi
          exit 0
        fi
        # Handle psql safe drain command and runtime barrier attestation
        if printf '%s\n' "$@" | grep -q 'provider_execution_quarantine'; then
          echo "1"
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q 'psql'; then
          if [ "\${MOCK_DRAIN_DB_FAIL:-0}" = "1" ]; then
            echo "psql: could not connect to server: connection refused" >&2
            exit 1
          fi
          if [ "\${MOCK_ACTIVE_LEASE:-0}" = "1" ]; then
            echo "1"
          else
            echo "0"
          fi
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q "/api/v1/health"; then
          if [ "\${MOCK_HEALTH_STATUS:-ok}" = "failing" ]; then
            echo '{"status":"error"}'
            exit 1
          fi
          echo '{"status":"ok","service":"xianzhi-ai-go-gin"}'
          exit 0
        fi
        if printf '%s\n' "$@" | grep -q "/api/v1/ready"; then
          if [ "\${MOCK_READY_STATUS:-ok}" = "failing" ]; then
            echo '{"ready":"false"}'
            exit 1
          fi
          if [ "\${MOCK_ASYNC_MESSAGING_NOT_READY:-0}" = "1" ]; then
            echo '{"status":"ok","ready":"true","asyncMessaging":"DEGRADED"}'
          else
            echo '{"status":"ok","ready":"true","asyncMessaging":"READY"}'
          fi
          exit 0
        fi
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
  exit 0
fi

if [ "$1" = "pull" ]; then
  if [ "\${MOCK_DOCKER_PULL_FAIL:-0}" = "1" ]; then
    echo "Error response from daemon: pull access denied" >&2
    exit 1
  fi
  exit 0
fi

if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  target="$3"
  if [ "\${MOCK_IMAGE_INSPECT_FAIL:-0}" = "1" ]; then
    echo "Error: No such image: $target" >&2
    exit 1
  fi
  if printf '%s\n' "$@" | grep -q 'RepoDigests'; then
    if [ "\${MOCK_RUNNING_IMAGE_DIGEST_MISMATCH:-0}" = "1" ]; then
      echo "ghcr.io/lmxchyy/zhiqiyun-ai@sha256:0000000000000000000000000000000000000000000000000000000000000000"
    else
      # Return both target and previous digests
      echo "\${XIANZHI_IMAGE_REFERENCE:-${DEFAULT_IMAGE_REF}}"
      echo "${PREV_IMAGE_REF}"
    fi
  else
    if [ "$target" = "${PREV_IMAGE_ID}" ] || [ "$target" = "${PREV_IMAGE_REF}" ]; then
      echo "${PREV_IMAGE_ID}"
    else
      echo "\${MOCK_IMAGE_ID:-${DEFAULT_IMAGE_ID}}"
    fi
  fi
  exit 0
fi

if [ "$1" = "ps" ]; then
  printf '%064d\\n' 1
  exit 0
fi

# Interactive framed psql mock: real Docker transport coverage lives in the
# UUID-owned Python3.6/PG suite, not in this release sequencing fixture.
if [ "$1" = "exec" ] && [ "$2" = "-i" ]; then
  if [ "\${MOCK_DRAIN_DB_FAIL:-0}" = "1" ]; then exit 42; fi
  exec python3 -u -c '
import json,os,sys
statement=""
for line in sys.stdin:
 if line.startswith(chr(92)+"echo END_"):
  token=line.strip().split("END_",1)[1]
  selected="WITH transport_rows" in statement
  rows=[[str(int(os.environ.get("MOCK_ACTIVE_LEASE","0")))]] if selected else []
  print(json.dumps(dict(token=token,rows=rows,count=len(rows),columns=1 if rows else 0,types=[20] if rows else [])),flush=True)
  print("END_"+token,flush=True)
  statement=""
 else: statement+=line
'
fi

if [ "$1" = "inspect" ]; then
  if [ "$2" = "$(printf '%064d' 1)" ]; then
    python3 -c 'import json;print(json.dumps([dict(Id="0"*63+"1",Image="sha256:mock-postgres",RestartCount=0,State=dict(Running=True,Paused=False,Restarting=False,StartedAt="fixed"),Config=dict(Image="postgres:16-alpine",Labels={"com.docker.compose.project":"mock-project","com.docker.compose.service":"postgres","com.docker.compose.oneoff":"False"},Env=["POSTGRES_USER=postgres","POSTGRES_DB=xianzhi","POSTGRES_PASSWORD=super_secret_pw"]))]))'
    exit 0
  fi
  if [ "\${MOCK_POSTSTOP_INSPECT_FAIL:-0}" = "1" ] && [ -f "$STATE_DIR/.mock_docker_stopped" ]; then exit 42; fi
  case "\${3:-}" in
    *Config.Image*)
      if [ "\${MOCK_RUNNING_IMAGE_DIGEST_MISMATCH:-0}" = "1" ]; then
        echo "ghcr.io/lmxchyy/zhiqiyun-ai@sha256:0000000000000000000000000000000000000000000000000000000000000000"
      else
        if [ -f "$STATE_DIR/.mock_docker_deployed" ]; then
          echo "\${XIANZHI_IMAGE_REFERENCE:-${DEFAULT_IMAGE_REF}}"
        else
          echo "${PREV_IMAGE_REF}"
        fi
      fi
      ;;
    *State.Health.Status*)
      if [ "\${MOCK_WORKER_UNHEALTHY:-0}" = "1" ]; then
        echo "unhealthy"
      else
        echo "healthy"
      fi
      ;;
    *State.Running*)
      if printf '%s\n' "$@" | grep -q "mock_migrate_cid"; then
        echo "false"
      elif [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
        echo "false"
      else
        echo "true"
      fi
      ;;
    *State.ExitCode*)
      if printf '%s\n' "$@" | grep -q "mock_migrate_cid"; then
        if [ "\${MOCK_MIGRATE_EXIT:-0}" = "1" ]; then
          echo "1"
        else
          echo "0"
        fi
      elif [ "\${MOCK_OLD_CONTAINER_UNCLEAN_EXIT:-0}" = "1" ]; then
        echo "137"
      else
        echo "0"
      fi
      ;;
    *State.Status*)
      if printf '%s\n' "$@" | grep -q "mock_migrate_cid"; then
        echo "exited"
      elif [ -f "$STATE_DIR/.mock_docker_stopped" ]; then
        echo "exited"
      else
        echo "running"
      fi
      ;;
    *)
      if [ "\${XIANZHI_IMAGE_REFERENCE:-}" = "${PREV_IMAGE_REF}" ]; then
        echo "${PREV_IMAGE_ID}"
      elif [ -f "$STATE_DIR/.mock_docker_deployed" ]; then
        echo "\${MOCK_IMAGE_ID:-${DEFAULT_IMAGE_ID}}"
      else
        echo "${PREV_IMAGE_ID}"
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

  // Create bare remote repository OUTSIDE sandbox dir
  const remoteDir = await mkdtemp(join(tmpdir(), "xianzhi-remote-"));
  await execFileAsync("git", ["init", "--bare", remoteDir]);

  // Initialize a REAL git repository in sandbox
  await execFileAsync("git", ["init"], { cwd: dir });
  await execFileAsync("git", ["config", "user.email", "ci@example.com"], { cwd: dir });
  await execFileAsync("git", ["config", "user.name", "CI Bot"], { cwd: dir });
  await execFileAsync("git", ["config", "core.autocrlf", "false"], { cwd: dir });
  await execFileAsync("git", ["config", "core.filemode", "false"], { cwd: dir });
  await execFileAsync("git", ["config", "commit.gpgsign", "false"], { cwd: dir });
  await execFileAsync("git", ["remote", "add", "origin", toBashPath(remoteDir)], { cwd: dir });

  // First commit represents previous running release (ancestor commit)
  await execFileAsync("git", ["add", "."], { cwd: dir });
  await execFileAsync("git", ["commit", "-m", "Previous release commit"], { cwd: dir });
  const { stdout: prevHeadShaOut } = await execFileAsync("git", ["rev-parse", "HEAD"], { cwd: dir });
  const prevHeadSha = prevHeadShaOut.trim();

  // Update previous manifest with real git SHA
  prevManifest.git_sha = prevHeadSha;
  await writeFile(join(backupsDir, "release-manifest.json"), JSON.stringify(prevManifest, null, 2), "utf8");
  await writeFile(join(dir, "release-manifest.json"), JSON.stringify(prevManifest, null, 2), "utf8");

  // Second commit represents target release commit
  await writeFile(join(dir, "commit-marker.txt"), "target-release\n", "utf8");
  await execFileAsync("git", ["add", "."], { cwd: dir });
  await execFileAsync("git", ["commit", "-m", "Target release commit"], { cwd: dir });
  await execFileAsync("git", ["branch", "-M", "main"], { cwd: dir });
  await execFileAsync("git", ["push", "-u", "origin", "main"], { cwd: dir });

  // Get current real HEAD commit
  const { stdout: headShaOut } = await execFileAsync("git", ["rev-parse", "HEAD"], { cwd: dir });
  const realHeadSha = headShaOut.trim();

  const transport = new MockGitHubTransport(dir);
  // Default setup for prevHeadSha (used for rollback verification)
  await transport.setupCommit(prevHeadSha, 10000, 20000, prevManifest);

  return { dir, binDir, bashBin, bashDir, realHeadSha, prevHeadSha, remoteDir, transport, pyFixtureDir };
}

async function runPrestage(sandbox, sha, env = {}, extraArgs = []) {
  const { bashBin, bashDir, pyFixtureDir } = sandbox;
  const script = `${bashDir}/ops/prestage-release.sh`;
  const quotedArgs = [sha, ...extraArgs].map(a => `'${a}'`).join(" ");
  const cleanEnv = { ...process.env };
  if (cleanEnv.GITHUB_ACTIONS === "true" && !("GITHUB_WORKFLOW" in env)) {
    delete cleanEnv.GITHUB_WORKFLOW;
  }
  return execFileAsync(
    bash,
    ["-c", `export PATH='${bashBin}':"$PATH"; cd '${bashDir}'; '${script}' ${quotedArgs}`],
    {
      env: {
        ...cleanEnv,
        TARGET_PLATFORM: "linux/amd64",
        PYTHONPATH: pyFixtureDir,
        MOCK_GITHUB_STATE_FILE: sandbox.transport.stateFile,
        ...env
      }
    }
  );
}

async function runDeploy(sandbox, env = {}, extraArgs = []) {
  const { bashBin, bashDir, pyFixtureDir } = sandbox;
  const script = `${bashDir}/deploy.sh`;
  const quotedArgs = extraArgs.map(a => `'${a}'`).join(" ");
  return execFileAsync(
    bash,
    ["-c", `export PATH='${bashBin}':"$PATH"; cd '${bashDir}'; '${script}' ${quotedArgs}`],
    {
      env: {
        ...process.env,
        TARGET_PLATFORM: "linux/amd64",
        PYTHONPATH: pyFixtureDir,
        ...env
      }
    }
  );
}

async function runRollback(sandbox, target, env = {}, extraArgs = []) {
  const { bashBin, bashDir, pyFixtureDir } = sandbox;
  const script = `${bashDir}/rollback.sh`;
  const quotedArgs = extraArgs.map(a => `'${a}'`).join(" ");
  return execFileAsync(
    bash,
    ["-c", `export PATH='${bashBin}':"$PATH"; cd '${bashDir}'; '${script}' '${target}' ${quotedArgs}`],
    {
      env: {
        ...process.env,
        TARGET_PLATFORM: "linux/amd64",
        PYTHONPATH: pyFixtureDir,
        ...env
      }
    }
  );
}

async function runVerifyProof(sandbox, proofPath, sha, env = {}, extraArgs = []) {
  const { bashBin, bashDir, pyFixtureDir } = sandbox;
  const script = `${bashDir}/ops/verify-prestage-proof.sh`;
  const quotedArgs = [proofPath, sha, ...extraArgs].map(a => `'${a}'`).join(" ");
  return execFileAsync(
    bash,
    ["-c", `export PATH='${bashBin}':"$PATH"; cd '${bashDir}'; '${script}' ${quotedArgs}`],
    {
      env: {
        ...process.env,
        TARGET_PLATFORM: "linux/amd64",
        PYTHONPATH: pyFixtureDir,
        ...env
      }
    }
  );
}

// -------------------------------------------------------------
// [T01] Normal PASS: Stage 1 verified provenance -> Stage 2 offline cutover
// -------------------------------------------------------------
test("[T01] prestaged release normal PASS: verified provenance -> stage 1 proof -> stage 2 cutover", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);

  // Stage 1: Prestage (in-memory transport verifies official https://api.github.com chain)
  const prestageResult = await runPrestage(sandbox, targetSha);
  assert.match(prestageResult.stdout, /Prestage proof atomically created at:/);

  // Verify proof structure & official provenance fields
  const proofPath = join(sandbox.dir, ".prestage", targetSha, "prestage-proof.json");
  const proof = JSON.parse(await readFile(proofPath, "utf8"));
  assert.equal(proof.git_sha, targetSha);
  assert.equal(proof.image_reference, DEFAULT_IMAGE_REF);
  assert.equal(proof.github_provenance.repository, "lmxchyy/zhiqiyun-ai");
  assert.equal(proof.github_provenance.workflow, ".github/workflows/immutable-image-release.yml");
  assert.equal(proof.github_provenance.run_id, 10001);
  assert.equal(proof.github_provenance.artifact_id, 20001);

  // Stage 2: Offline Cutover (zero git fetch/pull, safe drain passed, health verified)
  const cutoverResult = await runDeploy(sandbox, {}, ["--prestaged", targetSha]);
  assert.match(cutoverResult.stdout, /zero remote Git operations/);
  assert.match(cutoverResult.stdout, /Deployment completed successfully/);

  // Verify release ledger recorded consumption
  const ledgerPath = join(sandbox.dir, "backups", "release-ledger.json");
  const ledger = JSON.parse(await readFile(ledgerPath, "utf8"));
  assert.equal(ledger.entries.length, 1);
  assert.equal(ledger.entries[0].status, "CONSUMED");
});

// -------------------------------------------------------------
// [T02] Provenance - Self-reported fake manifest rejected
// -------------------------------------------------------------
test("[T02] prestage rejects self-reported fake manifest whose bytes differ from official GitHub artifact", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const officialManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, officialManifest);

  const forgedDir = await mkdtemp(join(tmpdir(), "xianzhi-forged-"));
  const forgedManifestPath = join(forgedDir, "forged-manifest.json");
  const forgedManifest = {
    ...officialManifest,
    forged_field: "hacked"
  };
  await writeFile(forgedManifestPath, JSON.stringify(forgedManifest, null, 2), "utf8");

  await assert.rejects(
    runPrestage(sandbox, targetSha, {}, ["--manifest", toBashPath(forgedManifestPath)]),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T03] Provenance - No official run found on GitHub rejected
// -------------------------------------------------------------
test("[T03] prestage rejects commit with no official successful run on GitHub", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  // Empty runs
  sandbox.transport.state.runs_by_sha[targetSha] = [];
  await sandbox.transport.save();

  await assert.rejects(
    runPrestage(sandbox, targetSha),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T04] Provenance - Mismatched branch, failed run, expired artifact, or spoofed workflow path rejected
// -------------------------------------------------------------
test("[T04] prestage rejects mismatched branch, failed run, expired artifact, or spoofed workflow path", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const manifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };

  // Case A: feature branch
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, manifest, { branch: "feat/untrusted" });
  await assert.rejects(
    runPrestage(sandbox, targetSha),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );

  // Case B: failed run conclusion
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, manifest, { conclusion: "failure" });
  await assert.rejects(
    runPrestage(sandbox, targetSha),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );

  // Case C: expired artifact
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, manifest, { expired: true });
  await assert.rejects(
    runPrestage(sandbox, targetSha),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );

  // Case D: spoofed workflow path ending with name
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, manifest, {
    workflowPath: ".github/workflows/malicious-immutable-image-release.yml"
  });
  await assert.rejects(
    runPrestage(sandbox, targetSha),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T05] Trust Root - Rejecting GITHUB_API_BASE_URL, GITHUB_REPOSITORY, GITHUB_WORKFLOW overrides
// -------------------------------------------------------------
test("[T05] prestage strictly rejects any attempt to override GITHUB_API_BASE_URL, repository, or workflow", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;

  // Untrusted HTTPS host
  await assert.rejects(
    runPrestage(sandbox, targetSha, { GITHUB_API_BASE_URL: "https://untrusted.example.invalid" }),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      assert.match(err.stderr, /GITHUB_API_BASE_URL override rejected/);
      return true;
    }
  );

  // HTTP localhost
  await assert.rejects(
    runPrestage(sandbox, targetSha, { GITHUB_API_BASE_URL: "http://127.0.0.1:9999" }),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      return true;
    }
  );

  // Repository override
  await assert.rejects(
    runPrestage(sandbox, targetSha, { GITHUB_REPOSITORY: "evil/zhiqiyun-ai" }),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      assert.match(err.stderr, /GITHUB_REPOSITORY override rejected/);
      return true;
    }
  );

  // Workflow override
  await assert.rejects(
    runPrestage(sandbox, targetSha, { GITHUB_ACTIONS: "false", GITHUB_WORKFLOW: "custom-pipeline.yml" }),
    (err) => {
      assert.match(err.stderr, /PROVENANCE_FAILED/);
      assert.match(err.stderr, /GITHUB_WORKFLOW override rejected/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T06] Stage 2 Cutover Fails Closed when HEAD != PRESTAGED_RELEASE_SHA
// -------------------------------------------------------------
test("[T06] prestaged cutover fails when real current HEAD does not match PRESTAGED_RELEASE_SHA", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  // Advance HEAD to a different commit
  await writeFile(join(sandbox.dir, "dummy.txt"), "advanced", "utf8");
  await execFileAsync("git", ["add", "dummy.txt"], { cwd: sandbox.dir });
  await execFileAsync("git", ["commit", "-m", "advance head"], { cwd: sandbox.dir });

  await assert.rejects(
    runDeploy(sandbox, {}, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /HEAD/);
      assert.match(err.stderr, /does not match/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T07] Dirty Working Tree Rejected Fail-Closed
// -------------------------------------------------------------
test("[T07] prestage and cutover reject real dirty working tree (uncommitted or untracked)", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;

  // Untracked file
  await writeFile(join(sandbox.dir, "untracked-leak.txt"), "leak", "utf8");

  await assert.rejects(
    runPrestage(sandbox, targetSha),
    (err) => {
      assert.match(err.stderr, /Working tree contains uncommitted or untracked changes/);
      return true;
    }
  );

  await assert.rejects(
    runDeploy(sandbox, {}, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /Working tree contains uncommitted or untracked changes/);
      return true;
    }
  );

  await rm(join(sandbox.dir, "untracked-leak.txt"));
});

// -------------------------------------------------------------
// [T08] Protected File Deletion / Script Tampering Rejected Fail-Closed
// -------------------------------------------------------------
test("[T08] cutover fails closed when migration SQL or ops script is deleted from disk after prestage", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  const proofPath = join(sandbox.dir, ".prestage", targetSha, "prestage-proof.json");

  // Case A: Delete migration file
  await rm(join(sandbox.dir, "database", "migrations", "001-init.sql"));
  await assert.rejects(
    runVerifyProof(sandbox, proofPath, targetSha),
    (err) => {
      assert.match(err.stderr, /PROTECTED_FILE_MISSING|MIGRATIONS_TAMPERED/);
      return true;
    }
  );

  // Restore migration file
  await writeFile(join(sandbox.dir, "database", "migrations", "001-init.sql"), "-- Initial migration\nSELECT 1;\n", "utf8");

  // Issue199: the executed drain helper is required and byte-bound in proof.
  await writeFile(join(sandbox.dir, "ops", "verify-safe-drain.py"), "raise SystemExit(0)\n", "utf8");
  await assert.rejects(
    runVerifyProof(sandbox, proofPath, targetSha),
    (err) => {
      assert.match(err.stderr, /DEPLOY_SCRIPT_TAMPERED/);
      return true;
    }
  );
  await copyFile(new URL("ops/verify-safe-drain.py", root), join(sandbox.dir, "ops", "verify-safe-drain.py"));

  // Issue203: authority source must be present, immutable, and in the proof.
  await rm(join(sandbox.dir, "ops", "quarantine-approval.py"));
  await assert.rejects(runVerifyProof(sandbox, proofPath, targetSha), (err) => {
    assert.match(err.stderr, /DEPLOY_SCRIPT_TAMPERED/);
    return true;
  });
  await writeFile(join(sandbox.dir, "ops", "quarantine-approval.py"), "raise SystemExit(0)\n", "utf8");
  await assert.rejects(runVerifyProof(sandbox, proofPath, targetSha), (err) => {
    assert.match(err.stderr, /DEPLOY_SCRIPT_TAMPERED/);
    return true;
  });
  await copyFile(new URL("ops/quarantine-approval.py", root), join(sandbox.dir, "ops", "quarantine-approval.py"));

  // CORE_ONLY snapshot source must also be required and byte-bound.
  await rm(join(sandbox.dir, "ops", "quarantine-live-snapshot.py"));
  await assert.rejects(runVerifyProof(sandbox, proofPath, targetSha), (err) => {
    assert.match(err.stderr, /DEPLOY_SCRIPT_TAMPERED/);
    return true;
  });
  await writeFile(join(sandbox.dir, "ops", "quarantine-live-snapshot.py"), "raise SystemExit(0)\n", "utf8");
  await assert.rejects(runVerifyProof(sandbox, proofPath, targetSha), (err) => {
    assert.match(err.stderr, /DEPLOY_SCRIPT_TAMPERED/);
    return true;
  });
  await copyFile(new URL("ops/quarantine-live-snapshot.py", root), join(sandbox.dir, "ops", "quarantine-live-snapshot.py"));

  // Case B: Tamper with ops script
  await writeFile(join(sandbox.dir, "ops", "run-migrations.sh"), "#!/bin/sh\nexit 0\n", "utf8");
  await assert.rejects(
    runVerifyProof(sandbox, proofPath, targetSha),
    (err) => {
      assert.match(err.stderr, /DEPLOY_SCRIPT_TAMPERED/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T09] Rollback Verification: Provenance Checked & Arbitrary Local JSON Rejected
// -------------------------------------------------------------
test("[T09] rollback verification: unverified local JSON rejected, and legal rollback with advanced HEAD succeeds", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);

  // Case A: Fake rollback manifest with forged sha not found on GitHub
  const forgedDir = await mkdtemp(join(tmpdir(), "xianzhi-forged-rb-"));
  const forgedFakeSha = "9999999999999999999999999999999999999999";
  const forgedManifestPath = join(forgedDir, "fake-rollback-manifest.json");
  await writeFile(forgedManifestPath, JSON.stringify({
    git_sha: forgedFakeSha,
    image: DEFAULT_IMAGE,
    digest: PREV_DIGEST,
    image_reference: PREV_IMAGE_REF,
    built_at: "2026-10-01T00:00:00Z",
    production_contract: "passed"
  }, null, 2), "utf8");

  // Prestage should reject using this forged rollback manifest because no GitHub run exists for forgedFakeSha
  await assert.rejects(
    runPrestage(sandbox, targetSha, {}, ["--rollback-manifest", toBashPath(forgedManifestPath)]),
    (err) => {
      assert.match(err.stderr, /ROLLBACK_RECEIPT_ERROR/);
      return true;
    }
  );

  await rm(forgedManifestPath);

  // Case B: Normal Prestage succeeds with official rollback manifest
  await runPrestage(sandbox, targetSha);

  // Deploy to targetSha
  await runDeploy(sandbox, {}, ["--prestaged", targetSha]);

  // Advance HEAD in git (simulating forward progress in repo)
  await writeFile(join(sandbox.dir, "feature.txt"), "feature", "utf8");
  await execFileAsync("git", ["add", "feature.txt"], { cwd: sandbox.dir });
  await execFileAsync("git", ["commit", "-m", "feature commit"], { cwd: sandbox.dir });

  // Rollback using the verified receipt
  const receiptPath = join(sandbox.dir, ".prestage", targetSha, "rollback-receipt.json");
  const rollbackResult = await runRollback(sandbox, toBashPath(receiptPath), {
    XIANZHI_IMAGE_REFERENCE: PREV_IMAGE_REF
  });
  assert.match(rollbackResult.stdout, /Using verified rollback receipt/);
  assert.match(rollbackResult.stdout, /Rollback to .* completed successfully/);

  // Verify env file persisted the rolled-back reference
  const envAfter = await readFile(join(sandbox.dir, ".env.production"), "utf8");
  assert.match(envAfter, /XIANZHI_IMAGE_REFERENCE="?ghcr\.io\/lmxchyy\/zhiqiyun-ai@sha256:b{64}"?/);
});

// -------------------------------------------------------------
// [T10] Safe Drain Fail-Closed & Post-Start Health Verification
// -------------------------------------------------------------
test("[T10] safe drain fail-closed: DB failure fails non-zero, and active leases block cutover", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  // Case A: PostgreSQL query fails -> MUST fail non-zero (NOT exit 0!)
  await assert.rejects(
    runDeploy(sandbox, { MOCK_DRAIN_DB_FAIL: "1", DRAIN_TIMEOUT_SECONDS: "2" }, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /SAFE_DRAIN_REJECTED/);
      assert.match(err.stderr, /PostgreSQL drain check query execution failed/);
      return true;
    }
  );

  // Case B: Container ps inspection fails -> MUST fail non-zero
  await assert.rejects(
    runDeploy(sandbox, { MOCK_DRAIN_PS_FAIL: "1", DRAIN_TIMEOUT_SECONDS: "2" }, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /SAFE_DRAIN_REJECTED/);
      assert.match(err.stderr, /Container inspection failed|API discovery failed/);
      return true;
    }
  );

  // Case C: Active leases in progress -> MUST reject cutover
  await assert.rejects(
    runDeploy(sandbox, { MOCK_ACTIVE_LEASE: "1", DRAIN_TIMEOUT_SECONDS: "2" }, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /SAFE_DRAIN_REJECTED/);
      assert.match(err.stderr, /active valid leases or in-flight operations in progress/);
      return true;
    }
  );

  // Side-effect boundary: docker compose stop must NOT be called when drain fails
  let stoppedAfterDrainFail = false;
  try {
    await access(join(sandbox.dir, ".mock_docker_stopped"));
    stoppedAfterDrainFail = true;
  } catch {
    stoppedAfterDrainFail = false;
  }
  assert.equal(stoppedAfterDrainFail, false, "docker compose stop must not be called when drain fails!");

  // Case D: Health/readiness failure blocks cutover and records NO ledger success entry
  await assert.rejects(
    runDeploy(sandbox, { MOCK_HEALTH_STATUS: "failing", HEALTH_CHECK_TIMEOUT_SECONDS: "2" }, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /CUTOVER_GATE_FAILED/);
      return true;
    }
  );

  // Release ledger must NOT contain a consumed entry
  const ledgerPath = join(sandbox.dir, "backups", "release-ledger.json");
  let hasConsumed = false;
  try {
    const ledger = JSON.parse(await readFile(ledgerPath, "utf8"));
    hasConsumed = ledger.entries && ledger.entries.length > 0;
  } catch {
    hasConsumed = false;
  }
  assert.equal(hasConsumed, false, "Ledger must not record consumed entry when health check fails!");
});

// -------------------------------------------------------------
// [T11] Concurrent Lock Owner Safety & Lifecycle
// -------------------------------------------------------------
test("[T11] concurrent lock owner safety: Process B failure/signals do not delete Process A's lock", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  const lockDir = join(sandbox.dir, ".prestage", "release.lock");
  await mkdir(lockDir, { recursive: true });

  // Simulate active owner Process A
  const ownerTokenA = "procA_token_999";
  await writeFile(join(lockDir, "owner_token"), ownerTokenA, "utf8");
  await writeFile(join(lockDir, "owner_pid"), String(process.pid), "utf8");

  // Attempt to run cutover as Process B while Process A is alive
  await assert.rejects(
    runDeploy(sandbox, {}, ["--prestaged", sandbox.realHeadSha]),
    (err) => {
      assert.match(err.stderr, /CONCURRENCY_LOCKED/);
      return true;
    }
  );

  // Verify that Process A's lock directory and owner_token were NOT deleted by Process B
  const tokenAfter = (await readFile(join(lockDir, "owner_token"), "utf8")).trim();
  assert.equal(tokenAfter, ownerTokenA, "Process A's lock must remain completely intact!");

  // Clean up lock
  await rm(lockDir, { recursive: true, force: true });
});

// -------------------------------------------------------------
// [T12] Zero Git Fetch and Pull during Prestaged Cutover
// -------------------------------------------------------------
test("[T12] prestaged cutover executes zero git fetch and zero git pull (works with broken remote)", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  // Break the remote origin
  await execFileAsync("git", ["remote", "set-url", "origin", "http://broken.invalid/repo.git"], { cwd: sandbox.dir });

  // Cutover should completely bypass git remote and succeed
  const cutoverResult = await runDeploy(sandbox, {}, ["--prestaged", targetSha]);
  assert.match(cutoverResult.stdout, /zero remote Git operations/);
  assert.match(cutoverResult.stdout, /Deployment completed successfully/);
});

// -------------------------------------------------------------
// [T13] Legacy Deploy Retains Online Git Fetch/Pull
// -------------------------------------------------------------
test("[T13] legacy deploy retains online git fetch/pull behavior", async () => {
  const sandbox = await setupSandbox();
  await assert.rejects(
    runDeploy(sandbox, {}, ["--skip-fetch"]),
    (err) => {
      assert.match(err.stderr, /Direct SKIP_FETCH without verified prestaged release proof is forbidden/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T14] Prestaged Deployment Passes --pull never to Docker Compose
// -------------------------------------------------------------
test("[T14] prestaged deployment passes --pull never to docker compose up", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  const dockerLog = join(sandbox.dir, "docker-invocations.log");
  await runDeploy(sandbox, { MOCK_DOCKER_LOG: toBashPath(dockerLog) }, ["--prestaged", targetSha]);

  const logContent = await readFile(dockerLog, "utf8");
  assert.match(logContent, /compose .* up .* --pull never/, "Docker compose up must be invoked with --pull never");
});

// -------------------------------------------------------------
// [T15] Anti-Replay: Proof Nonce Cannot Be Reused
// -------------------------------------------------------------
test("[T15] cutover rejects reusing an already consumed prestage proof", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  // Consume proof once
  await runDeploy(sandbox, {}, ["--prestaged", targetSha]);

  // Attempt to consume again with the same proof
  await assert.rejects(
    runDeploy(sandbox, {}, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /PRESTAGE_PROOF_ALREADY_CONSUMED/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T16] Rollback Rejects Non-Ancestor Commits
// -------------------------------------------------------------
test("[T16] rollback.sh rejects forward non-ancestor commits and forward manifests", async () => {
  const sandbox = await setupSandbox();
  const nonAncestorSha = "1234567890123456789012345678901234567890";
  await assert.rejects(
    runRollback(sandbox, nonAncestorSha),
    (err) => {
      assert.match(err.stderr, /ROLLBACK_FORWARD_REJECTED/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T17] Effective Configuration Binding: Process Env Overrides Bound
// -------------------------------------------------------------
test("[T17] effective configuration binding verifies process environment overrides and detects mutation", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);

  // Prestage with specific MIGRATION_FILES process env override
  await runPrestage(sandbox, targetSha, {
    MIGRATION_FILES: "001-init.sql"
  });

  const proofPath = join(sandbox.dir, ".prestage", targetSha, "prestage-proof.json");
  const proof = JSON.parse(await readFile(proofPath, "utf8"));
  assert.ok(proof.bound_config_hash, "Proof must include bound_config_hash");

  // Verify proof succeeds with matching process env
  await runVerifyProof(sandbox, proofPath, targetSha, {
    MIGRATION_FILES: "001-init.sql"
  });

  // Mutating MIGRATION_FILES in process env during Stage 2 must trigger CONFIG_MUTATED_AFTER_PRESTAGE
  await assert.rejects(
    runVerifyProof(sandbox, proofPath, targetSha, {
      MIGRATION_FILES: "999-tampered.sql"
    }),
    (err) => {
      assert.match(err.stderr, /CONFIG_MUTATED_AFTER_PRESTAGE/);
      return true;
    }
  );
});

// -------------------------------------------------------------
// [T18] Registry Selection: aliyun_acr selected and preserved
// -------------------------------------------------------------
test("[T18] registry selection: aliyun_acr reference selected from manifest", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const acrImage = "registry.cn-hangzhou.aliyuncs.com/zhiqiyun/xianzhi-ai";
  const acrRef = `${acrImage}@${DEFAULT_DIGEST}`;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    registries: {
      aliyun_acr: {
        image: acrImage,
        digest: DEFAULT_DIGEST,
        image_reference: acrRef
      }
    },
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);

  await runPrestage(sandbox, targetSha, {}, ["--registry", "aliyun_acr"]);

  const proofPath = join(sandbox.dir, ".prestage", targetSha, "prestage-proof.json");
  const proof = JSON.parse(await readFile(proofPath, "utf8"));
  assert.equal(proof.image_reference, acrRef, "Proof must select the aliyun_acr image reference");
  assert.equal(proof.selected_registry, "aliyun_acr");
});

// -------------------------------------------------------------
// [T19] Stale Lock Recovery & Concurrent Recovery Exclusion
// -------------------------------------------------------------
test("[T19] stale lock recovery: dead PID safely recovered, and active recovery lock blocks race", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);
  await runPrestage(sandbox, targetSha);

  const lockDir = join(sandbox.dir, ".prestage", "release.lock");
  const recoveryLock = join(sandbox.dir, ".prestage", "release.lock.recovering");
  await mkdir(lockDir, { recursive: true });

  // Simulate dead PID
  const deadPid = "99999999";
  await writeFile(join(lockDir, "owner_pid"), deadPid, "utf8");
  await writeFile(join(lockDir, "owner_token"), "dead_token_123", "utf8");

  // Case A: Active recovery lock blocks second recoverer
  await mkdir(recoveryLock, { recursive: true });
  await assert.rejects(
    runDeploy(sandbox, {}, ["--prestaged", targetSha]),
    (err) => {
      assert.match(err.stderr, /CONCURRENCY_LOCKED/);
      return true;
    }
  );
  await rm(recoveryLock, { recursive: true, force: true });

  // Case B: Without recovery lock, dead owner is safely recovered and deployment proceeds
  const deployResult = await runDeploy(sandbox, {}, ["--prestaged", targetSha]);
  assert.match(deployResult.stdout, /Stale lock safely recovered/);
  assert.match(deployResult.stdout, /Deployment completed successfully/);
});

// -------------------------------------------------------------
// [T20] Prestage Fails Closed when pull migrate fails
// -------------------------------------------------------------
test("[T20] prestage fails closed if pull migrate fails", async () => {
  const sandbox = await setupSandbox();
  const targetSha = sandbox.realHeadSha;
  const targetManifest = {
    git_sha: targetSha,
    image: DEFAULT_IMAGE,
    digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF,
    built_at: "2026-10-02T00:00:00Z",
    production_contract: "passed"
  };
  await sandbox.transport.setupCommit(targetSha, 10001, 20001, targetManifest);

  await assert.rejects(
    runPrestage(sandbox, targetSha, { MOCK_PULL_MIGRATE_FAIL: "1" }),
    (err) => {
      assert.match(err.stderr, /Failed to pull compose dependency \(migrate\)/);
      return true;
    }
  );
});

async function round4Staged() {
  const sandbox = await setupSandbox();
  const sha = sandbox.realHeadSha;
  await sandbox.transport.setupCommit(sha, 10001, 20001, {
    git_sha: sha, image: DEFAULT_IMAGE, digest: DEFAULT_DIGEST,
    image_reference: DEFAULT_IMAGE_REF, built_at: '2026-10-02T00:00:00Z', production_contract: 'passed'
  });
  await runPrestage(sandbox, sha);
  return sandbox;
}

async function assertNoMigrationOrSuccess(sandbox) {
  await assert.rejects(access(join(sandbox.dir, '.mock_docker_migration')));
  await assert.rejects(access(join(sandbox.dir, 'backups', 'release-ledger.json')));
}

test('[R4] actual deploy stops on post-stop status/inspect failure before migration', async () => {
  for (const key of ['MOCK_POSTSTOP_PS_FAIL', 'MOCK_POSTSTOP_INSPECT_FAIL', 'MOCK_STOP_FAIL']) {
    const s = await round4Staged();
    await assert.rejects(runDeploy(s, { [key]: '1' }, ['--prestaged', s.realHeadSha]), e => {
      assert.notEqual(e.code, 0);
      assert.match(e.stderr, /query failed|Failed to stop/);
      return true;
    });
    await assertNoMigrationOrSuccess(s);
  }
});

test('[R4] scheduler/consumer query failure forbids stop, migration and success', async () => {
  for (const key of ['MOCK_SCHEDULER_FAIL', 'MOCK_CONSUMER_FAIL']) {
    const s = await round4Staged();
    await assert.rejects(runDeploy(s, { [key]: '1' }, ['--prestaged', s.realHeadSha]), e => {
      assert.notEqual(e.code, 0);
      assert.match(e.stderr, /release-runtime.*FAIL/);
      return true;
    });
    await assert.rejects(access(join(s.dir, '.mock_docker_stopped')));
    await assertNoMigrationOrSuccess(s);
  }
});

test('[R4] effective endpoint, canary and storage drift invalidates proof before stop', async () => {
  const s = await round4Staged();
  for (const key of ['DATABASE_URL', 'RABBITMQ_URL', 'GENERATION_ASYNC_CANARY_USERS', 'S3_BUCKET', 'S3_REGION', 'OBS_PREFIX']) {
    await assert.rejects(runDeploy(s, { [key]: 'changed-test-value' }, ['--prestaged', s.realHeadSha]), e => {
      assert.notEqual(e.code, 0);
      assert.match(e.stderr, /CONFIG_MUTATED_AFTER_PRESTAGE/);
      return true;
    });
    await assert.rejects(access(join(s.dir, '.mock_docker_stopped')));
    await assertNoMigrationOrSuccess(s);
  }
  const proof = JSON.parse(await readFile(join(s.dir, '.prestage', s.realHeadSha, 'prestage-proof.json'), 'utf8'));
  assert.equal(proof.config_binding_version, 2);
  assert.equal(proof.bound_services_config, undefined, 'effective credentials must never be copied into proof');
});
