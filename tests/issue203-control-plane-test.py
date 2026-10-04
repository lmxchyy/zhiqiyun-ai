"""Issue #203 Control Plane Automated Verification Suite.

Tests safe drain, quarantine boundary, safe PSQL STDIN transport,
Python 3.6 compatibility, rollback scoping, capability determination,
adversarial injection resistance, and container fencing.
"""
import copy
import datetime
import hashlib
import hmac
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
import uuid

ROOT = Path(__file__).resolve().parent.parent
sys.dont_write_bytecode = True
TEST_TRUST_SECRET = b"ci-synthetic-test-trust-secret-only"

def find_bash():
    for p in ["C:/Program Files/Git/bin/bash.exe", "C:/Program Files/Git/usr/bin/bash.exe"]:
        if os.path.isfile(p):
            return p
    return shutil.which("bash") or "bash"

BASH_EXE = find_bash()

def to_bash_path(path):
    p = Path(path).resolve().as_posix()
    if len(p) >= 2 and p[1] == ":":
        return "/" + p[0].lower() + p[2:]
    return p

def run_cmd(args, **kwargs):
    if "capture_output" in kwargs:
        kwargs.pop("capture_output")
        kwargs["stdout"] = subprocess.PIPE
        kwargs["stderr"] = subprocess.PIPE
    if "text" in kwargs:
        kwargs["universal_newlines"] = kwargs.pop("text")
    return subprocess.run(args, **kwargs)

def sign_manifest(manifest_dict, secret_bytes=TEST_TRUST_SECRET):
    m = copy.deepcopy(manifest_dict)
    m.pop("signature", None)
    canonical = json.dumps(m, sort_keys=True, separators=(',', ':')).encode("utf-8")
    sig = hmac.new(secret_bytes, canonical, hashlib.sha256).hexdigest()
    m["signature"] = sig
    return m

def make_valid_manifest(release_sha="79a3d7cf493453adbcdc03234b62fc5f099f776a", eid=1, tid="task_1", gen=10):
    manifest = {
        "manifest_version": "1.0",
        "release_sha": release_sha,
        "authority_id": "ops-control-root-prod",
        "not_before": (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M:%SZ'),
        "expires_at": (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M:%SZ'),
        "executions": [
            {
                "execution_id": eid,
                "task_id": tid,
                "attempt": 1,
                "generation": gen,
                "execution_status": "unknown",
                "provider_request_id": None,
                "task_status": "FAILED",
                "task_status_v2": "FAILED",
                "lease_until": "2026-10-01T00:00:00Z",
                "owner": None,
                "approval_id": "appr_synthetic_1",
                "release_sha": release_sha,
                "not_before": (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M:%SZ'),
                "expires_at": (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M:%SZ'),
                "snapshot_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
                "evidence_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
            }
        ]
    }
    return sign_manifest(manifest)


class ControlPlaneUnitTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        os.environ["RELEASE_TRUST_SECRET"] = TEST_TRUST_SECRET.decode("utf-8")

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)
        os.environ.pop("RELEASE_TRUST_SECRET", None)

    def test_anti_f9_rejection(self):
        f9_sha = "f9cdf44ca79272ad7cead33dfb1d35fdf155f05f"
        manifest = make_valid_manifest(release_sha=f9_sha)
        m_file = os.path.join(self.tmpdir, "manifest_f9.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(manifest, f)

        # verify-safe-drain rejection
        res_drain = run_cmd(
            [sys.executable, str(ROOT / "ops/verify-safe-drain.py"),
             "dummy_compose", "dummy_env", "15",
             "--manifest", m_file, "--release-sha", f9_sha],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res_drain.returncode, 0)
        self.assertIn("PERMANENTLY_REJECTED_CARRIER", res_drain.stderr)

        # enroll-quarantine rejection
        res_enroll = run_cmd(
            [sys.executable, str(ROOT / "ops/enroll-quarantine.py"),
             "dummy_compose", "dummy_env", m_file, f9_sha],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res_enroll.returncode, 0)
        self.assertIn("PERMANENTLY_REJECTED_CARRIER", res_enroll.stderr)

    def test_unauthenticated_manifest_rejected(self):
        valid = make_valid_manifest()
        valid.pop("signature")
        m_file = os.path.join(self.tmpdir, "unsigned.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(valid, f)

        res = run_cmd(
            [sys.executable, str(ROOT / "ops/enroll-quarantine.py"),
             "dummy_compose", "dummy_env", m_file, valid["release_sha"]],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("QUARANTINE_APPROVAL_INVALID", res.stderr)

    def test_tampered_manifest_signature_rejected(self):
        valid = make_valid_manifest()
        valid["signature"] = "0" * 64
        m_file = os.path.join(self.tmpdir, "tampered_sig.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(valid, f)

        res = run_cmd(
            [sys.executable, str(ROOT / "ops/enroll-quarantine.py"),
             "dummy_compose", "dummy_env", m_file, valid["release_sha"]],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("QUARANTINE_APPROVAL_INVALID", res.stderr)

    def test_legacy_hmac_without_independent_authority_fails_closed(self):
        os.environ.pop("RELEASE_TRUST_SECRET", None)
        valid = make_valid_manifest()
        m_file = os.path.join(self.tmpdir, "valid.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(valid, f)

        res = run_cmd(
            [sys.executable, str(ROOT / "ops/enroll-quarantine.py"),
             "dummy_compose", "dummy_env", m_file, valid["release_sha"]],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("QUARANTINE_APPROVAL_INVALID", res.stderr)

    def test_manifest_sha256_mismatch_fails_closed(self):
        valid = make_valid_manifest()
        m_file = os.path.join(self.tmpdir, "valid.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(valid, f)

        wrong_hash = "f" * 64
        res = run_cmd(
            [sys.executable, str(ROOT / "ops/enroll-quarantine.py"),
             "dummy_compose", "dummy_env", m_file, valid["release_sha"],
             "--expected-manifest-sha256", wrong_hash],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Manifest SHA256 mismatch", res.stderr)

    def test_legacy_hmac_cannot_authorize_even_duplicate_entries(self):
        valid = make_valid_manifest()
        item = copy.deepcopy(valid["executions"][0])
        valid["executions"].append(item)
        signed = sign_manifest(valid)
        m_file = os.path.join(self.tmpdir, "duplicate.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(signed, f)

        res = run_cmd(
            [sys.executable, str(ROOT / "ops/enroll-quarantine.py"),
             "dummy_compose", "dummy_env", m_file, signed["release_sha"]],
            capture_output=True, text=True, encoding="utf-8", errors="replace"
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("QUARANTINE_APPROVAL_INVALID", res.stderr)

    def test_python36_regex_timestamp_parser(self):
        import importlib.util
        spec = importlib.util.spec_from_file_location("enroll", str(ROOT / "ops/enroll-quarantine.py"))
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)

        cases = [
            ("2026-10-03T10:00:00Z", 2026, 10, 3, 10, 0, 0),
            ("2026-10-03T10:00:00+00:00", 2026, 10, 3, 10, 0, 0),
            ("2026-10-03 10:00:00.500000Z", 2026, 10, 3, 10, 0, 500000),
            ("2026-10-03T18:00:00+08:00", 2026, 10, 3, 10, 0, 0),  # converted to UTC
        ]
        for ts_str, y, m, d, h, mn, s in cases:
            dt = mod.parse_iso8601_utc(ts_str)
            self.assertEqual(dt.year, y)
            self.assertEqual(dt.month, m)
            self.assertEqual(dt.day, d)
            self.assertEqual(dt.hour, h)
            self.assertEqual(dt.minute, mn)

        with self.assertRaises(ValueError):
            mod.parse_iso8601_utc("invalid-timestamp")


class RollbackSyntaxTests(unittest.TestCase):
    def test_rollback_syntax_and_scoping(self):
        r = subprocess.run(["shellcheck", "rollback.sh"], cwd=ROOT, capture_output=True, text=True, encoding="utf-8", errors="replace")
        self.assertEqual(r.returncode, 0, f"shellcheck failed: {r.stdout} {r.stderr}")


class IsolatedPostgresControlPlaneTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # This suite destroys its schema. Never reuse an inherited service container,
        # even when CI or the caller supplies XIANZHI_TEST_CONTAINER.
        cls.own_container = False
        cls.container = "issue203-pg-test-" + uuid.uuid4().hex[:10]
        subprocess.run([
            "docker", "run", "-d", "--rm", "--network", "none",
            "--name", cls.container,
            "-e", "POSTGRES_PASSWORD=test_secret_pass",
            "-e", "POSTGRES_USER=postgres",
            "-e", "POSTGRES_DB=xianzhi_test",
            "pgvector/pgvector:pg16"
        ], check=True, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL)
        cls.own_container = True
        try:
            for _ in range(40):
                r = subprocess.run(["docker", "exec", cls.container, "psql", "-X", "-U", "postgres", "-d", "xianzhi_test", "-c", "SELECT 1;"],
                                   stdin=subprocess.DEVNULL, capture_output=True)
                if r.returncode == 0:
                    break
                time.sleep(0.5)
            else:
                raise RuntimeError("PostgreSQL test container failed to become ready")
            # Full real schema + all numbered migrations, only in OUR container.
            migration_paths = sorted(path for path in (ROOT / "database/migrations").glob("[0-9][0-9][0-9]-*.sql")
                                     if not path.name.endswith(".down.sql"))
            replay_sql = (ROOT / "database/schema.sql").read_text(encoding="utf-8") + "\n"
            replay_sql += "\n".join(path.read_text(encoding="utf-8") for path in migration_paths)
            result = subprocess.run(["docker", "exec", "-i", cls.container, "psql", "-X", "-U", "postgres",
                                     "-d", "xianzhi_test", "-v", "ON_ERROR_STOP=1"],
                                    input=replay_sql.encode("utf-8"), stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            if result.returncode:
                # Schema errors are data-redacted even for synthetic fixtures.
                message = result.stderr.decode('utf-8', 'replace')
                errors = [line for line in message.splitlines() if 'ERROR:' in line]
                diagnostic = re.sub(r'\"[^\"]*\"|\'[^\']*\'|https?://\S+', '[REDACTED]', errors[-1] if errors else 'unknown')
                raise RuntimeError("owned full schema/migration replay failed; no SKIP: " + diagnostic)
            cls.migration_count = len(migration_paths)
            print("[synthetic fixture] real schema + %d migrations replayed; not snapshot acceptance" % cls.migration_count)
            # Keep real migrated DDL for per-test clean schemas without replaying
            # seeded product data. No queries return prompts/URLs/payloads.
            dump = subprocess.run(["docker", "exec", cls.container, "pg_dump", "-U", "postgres", "-d", "xianzhi_test",
                                   "--schema-only", "--no-owner", "--no-privileges"],
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            if dump.returncode:
                raise RuntimeError("owned schema-only dump failed; no SKIP")
            cls.migrated_schema = dump.stdout.decode("utf-8")
        except Exception:
            subprocess.run(["docker", "rm", "-f", cls.container],
                           stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            cls.own_container = False
            raise

    @classmethod
    def tearDownClass(cls):
        if getattr(cls, "own_container", False):
            subprocess.run(["docker", "rm", "-f", cls.container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def psql(self, sql_text):
        args = ["docker", "exec", "-i", self.container, "psql", "-X", "-U", "postgres", "-d", "xianzhi_test", "-v", "ON_ERROR_STOP=1"]
        return subprocess.run(args, input=sql_text.encode("utf-8"), capture_output=True)

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.saved_env = {key: os.environ.get(key) for key in (
            "RELEASE_TRUST_SECRET", "XIANZHI_TEST_CONTAINER", "POSTGRES_USER",
            "POSTGRES_DB", "POSTGRES_PASSWORD")}
        self.addCleanup(self.restore_environment)
        os.environ["RELEASE_TRUST_SECRET"] = TEST_TRUST_SECRET.decode("utf-8")
        os.environ["XIANZHI_TEST_CONTAINER"] = self.container
        os.environ["POSTGRES_USER"] = "postgres"
        os.environ["POSTGRES_DB"] = "xianzhi_test"
        os.environ["POSTGRES_PASSWORD"] = "test_secret_pass"

        # Real migration tables, not invented five-column live-snapshot models.
        setup_sql = "DROP SCHEMA public CASCADE; CREATE SCHEMA public;\n" + self.migrated_schema
        setup_sql += """
SET search_path = public;
ALTER TABLE provider_executions ALTER COLUMN provider SET DEFAULT 'synthetic-provider';
ALTER TABLE provider_executions ALTER COLUMN capability SET DEFAULT 'synthetic-image';
ALTER TABLE provider_executions ALTER COLUMN request_fingerprint SET DEFAULT repeat('a', 64);
"""
        r = self.psql(setup_sql)
        self.assertEqual(r.returncode, 0, f"Schema setup failed: {r.stderr.decode()}")

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def restore_environment(self):
        for key, value in self.saved_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value

    def test_destructive_replay_owns_container(self):
        self.assertTrue(self.own_container)
        self.assertTrue(self.container.startswith("issue203-pg-test-"))
        inherited = self.saved_env["XIANZHI_TEST_CONTAINER"]
        if inherited is not None:
            self.assertNotEqual(self.container, inherited)
        self.assertEqual(os.environ["XIANZHI_TEST_CONTAINER"], self.container)

    def execute_transport_fixture(self, entries):
        # Executes only the legacy SQL/COPY builder against OUR owned container.
        # This is not a trust override or enrollment CLI bypass; it cannot prove
        # authenticated live snapshot/atomic-readback/runtime acceptance.
        spec = importlib.util.spec_from_file_location("enroll_fixture", str(ROOT / "ops/enroll-quarantine.py"))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return self.psql(module.build_enrollment_sql(entries))

    def test_transport_only_psql_stdin_adversarial_injection_resistance(self):
        # Seed matching execution and task
        hostile_task_id = "task_'; DROP TABLE provider_execution_quarantine; --"
        sql_task_id = hostile_task_id.replace("'", "''")
        hostile_approval_id = "appr_$(whoami)`touch /tmp/pwned`'\"\\;"
        self.psql(f"""
INSERT INTO xz_generation_tasks (id, status, task_status, lease_until, worker_id) VALUES ('{sql_task_id}', 'FAILED', 'FAILED', now() - interval '1 hour', NULL);
INSERT INTO provider_executions (id, task_id, attempt, status, task_execution_generation) VALUES (200, '{sql_task_id}', 1, 'unknown', 5);
""")
        manifest = make_valid_manifest(eid=200, tid=hostile_task_id, gen=5)
        manifest["executions"][0]["approval_id"] = hostile_approval_id
        signed = sign_manifest(manifest)

        m_file = os.path.join(self.tmpdir, "hostile.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(signed, f)

        res = self.execute_transport_fixture(signed["executions"])
        self.assertEqual(res.returncode, 0, f"Adversarial enrollment failed: {res.stderr}")

        # Check /tmp/pwned does NOT exist inside postgres container
        check_pwned = run_cmd(["docker", "exec", self.container, "ls", "/tmp/pwned"], capture_output=True)
        self.assertNotEqual(check_pwned.returncode, 0, "COMMAND SUBSTITUTION EXECUTED IN CONTAINER!")

        # Check table still exists and data was inserted verbatim
        check_row = self.psql("SELECT task_id, approval_id FROM provider_execution_quarantine WHERE execution_id=200;")
        self.assertEqual(check_row.returncode, 0)
        output = check_row.stdout.decode()
        self.assertIn(hostile_task_id, output)
        self.assertIn(hostile_approval_id, output)

    def test_transport_only_hostile_field_matrix(self):
        values = ["single'quote", 'double"quote', '$(touch /tmp/ISSUE203_TRANSPORT_PWNED)',
                  '`touch /tmp/ISSUE203_TRANSPORT_PWNED`', '; DROP TABLE provider_execution_quarantine;',
                  'line1\nline2', '中文', 'json\\escape', 'x' * 512]
        for index, value in enumerate(values):
            with self.subTest(case=index):
                execution_id = 1000 + index
                task_id = 'synthetic-%d-' % index + value
                sql_task_id = task_id.replace("'", "''")
                seed = self.psql("INSERT INTO xz_generation_tasks (id,status,task_status,lease_until,worker_id) "
                                 "VALUES ('%s','FAILED','FAILED',now()-interval '1 hour',NULL);\n"
                                 "INSERT INTO provider_executions (id,task_id,attempt,status,task_execution_generation) "
                                 "VALUES (%d,'%s',1,'unknown',1);" % (sql_task_id, execution_id, sql_task_id))
                self.assertEqual(seed.returncode, 0)
                entry = make_valid_manifest(eid=execution_id, tid=task_id, gen=1)['executions'][0]
                entry['approval_id'] = 'synthetic-' + value
                result = self.execute_transport_fixture([entry])
                self.assertEqual(result.returncode, 0)
                row = self.psql("SELECT json_build_object('task_id',task_id,'approval_id',approval_id) "
                                "FROM provider_execution_quarantine WHERE execution_id=%d;" % execution_id)
                self.assertEqual(row.returncode, 0)
                # psql formatted output is safe because all returned data is
                # explicitly synthetic; compare JSON content without logging it.
                lines = row.stdout.decode('utf-8').splitlines()
                encoded = next(line.strip() for line in lines if line.strip().startswith('{'))
                self.assertEqual(json.loads(encoded), dict(task_id=task_id, approval_id=entry['approval_id']))
        marker = run_cmd(['docker', 'exec', self.container, 'test', '-e', '/tmp/ISSUE203_TRANSPORT_PWNED'], capture_output=True)
        self.assertNotEqual(marker.returncode, 0)

    def test_legacy_sql_constraint_only_task_not_failed(self):
        self.psql("""
INSERT INTO xz_generation_tasks (id, status, task_status, lease_until, worker_id) VALUES ('task_running', 'RUNNING', 'RUNNING', now() - interval '1 hour', NULL);
INSERT INTO provider_executions (id, task_id, attempt, status, task_execution_generation) VALUES (301, 'task_running', 1, 'unknown', 1);
""")
        manifest = make_valid_manifest(eid=301, tid="task_running", gen=1)
        m_file = os.path.join(self.tmpdir, "m301.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(manifest, f)

        res = self.execute_transport_fixture(manifest["executions"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn(b"TASK_MISMATCH", res.stderr)

        # Confirm zero rows enrolled
        count = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("0", count.stdout.decode())

    def test_legacy_sql_constraint_only_execution_generation_mismatch(self):
        self.psql("""
INSERT INTO xz_generation_tasks (id, status, task_status, lease_until, worker_id) VALUES ('task_gen', 'FAILED', 'FAILED', now() - interval '1 hour', NULL);
INSERT INTO provider_executions (id, task_id, attempt, status, task_execution_generation) VALUES (302, 'task_gen', 1, 'unknown', 99);
""")
        # Manifest expects generation 10, but DB has 99
        manifest = make_valid_manifest(eid=302, tid="task_gen", gen=10)
        m_file = os.path.join(self.tmpdir, "m302.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(manifest, f)

        res = self.execute_transport_fixture(manifest["executions"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn(b"EXECUTION_MISMATCH", res.stderr)

    def test_legacy_sql_constraint_only_active_lease(self):
        self.psql("""
INSERT INTO xz_generation_tasks (id, status, task_status, lease_until, worker_id) VALUES ('task_lease', 'FAILED', 'FAILED', now() + interval '1 hour', NULL);
INSERT INTO provider_executions (id, task_id, attempt, status, task_execution_generation) VALUES (303, 'task_lease', 1, 'unknown', 1);
""")
        manifest = make_valid_manifest(eid=303, tid="task_lease", gen=1)
        m_file = os.path.join(self.tmpdir, "m303.json")
        with open(m_file, "w", encoding="utf-8") as f:
            json.dump(manifest, f)

        res = self.execute_transport_fixture(manifest["executions"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn(b"TASK_MISMATCH", res.stderr)

    def seed_quarantine_record(self, execution_id=501, task_id="task_rb_test"):
        self.psql(f"""
INSERT INTO xz_generation_tasks (id, status, task_status, lease_until, worker_id) VALUES ('{task_id}', 'FAILED', 'FAILED', now() - interval '1 hour', NULL) ON CONFLICT (id) DO NOTHING;
INSERT INTO provider_executions (id, task_id, attempt, status, task_execution_generation) VALUES ({execution_id}, '{task_id}', 1, 'unknown', 1) ON CONFLICT (id) DO NOTHING;
INSERT INTO provider_execution_quarantine (
    execution_id, task_id, attempt, generation,
    snapshot_sha256, evidence_sha256, approval_id, release_sha,
    not_before, expires_at
) VALUES (
    {execution_id}, '{task_id}', 1, 1,
    '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
    '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
    'appr_rb_test', '79a3d7cf493453adbcdc03234b62fc5f099f776a',
    now() - interval '1 hour', now() + interval '1 hour'
);
""")

    def create_rollback_sandbox(self, extra_env=None):
        sb_dir = Path(tempfile.mkdtemp(prefix="issue203-rb-sb-"))
        self.addCleanup(lambda: shutil.rmtree(sb_dir, ignore_errors=True))
        action_log = sb_dir / "actions.log"

        target_digest = "sha256:" + "1" * 64
        expected_ref = f"ghcr.io/lmxchyy/zhiqiyun-ai@{target_digest}"
        expected_image_id = "sha256:" + "2" * 64

        env_file = sb_dir / ".env.production"
        env_file.write_text(
            f"DATABASE_URL=\"postgres://postgres:test_secret_pass@127.0.0.1:5432/xianzhi_test\"\n"
            f"STORAGE_MASTER_KEY=\"0123456789abcdef0123456789abcdef\"\n"
            f"CONNECTOR_SECRET_ENCRYPTION_KEY=\"0123456789abcdef0123456789abcdef\"\n"
            f"POSTGRES_USER=\"postgres\"\n"
            f"POSTGRES_DB=\"xianzhi_test\"\n"
            f"POSTGRES_PASSWORD=\"test_secret_pass\"\n"
            f"XIANZHI_IMAGE_REFERENCE={expected_ref}\n",
            encoding="utf-8"
        )

        compose_file = sb_dir / "compose.prod.yml"
        compose_file.write_text(
            "services:\n"
            "  xianzhi-ai:\n"
            "    image: ${XIANZHI_IMAGE_REFERENCE:-xianzhi-ai-platform:prod}\n"
            "  smartvideo-worker:\n"
            "    image: ${XIANZHI_IMAGE_REFERENCE:-xianzhi-ai-platform:prod}\n",
            encoding="utf-8"
        )

        prestage_dir = sb_dir / ".prestage"
        prestage_dir.mkdir()
        quarantine_backup_dir = sb_dir / "backups" / "quarantine"
        quarantine_backup_dir.mkdir(parents=True)
        env_backup_dir = sb_dir / "backups" / "env"
        env_backup_dir.mkdir(parents=True)

        runner_script = f"""#!/usr/bin/env bash
git() {{
  if [ "$1" = "checkout" ]; then
    if [ -n "$ACTION_LOG" ]; then
      (IFS=' '; printf 'git checkout %s\\n' "$*") >> "$ACTION_LOG"
    fi
    return 0
  fi
  command git "$@"
}}
export -f git

docker() {{
  local action_log="${{ACTION_LOG:-}}"
  if [ "$1" = "exec" ]; then
    command docker "$@"
    return $?
  fi
  if [ "$1" = "compose" ]; then
    shift
    while [ $# -gt 0 ]; do
      case "$1" in
        -f|--file|--env-file)
          shift 2
          ;;
        version)
          echo "Docker Compose version v2.24.0"
          return 0
          ;;
        config)
          shift
          local is_json=0
          for arg in "$@"; do
            [ "$arg" = "json" ] && is_json=1
          done
          local img="${{XIANZHI_IMAGE_REFERENCE:-$EXPECTED_REF}}"
          if [ "$is_json" = "1" ]; then
            printf '{{\"services\":{{\"xianzhi-ai\":{{\"image\":\"%s\"}},\"smartvideo-worker\":{{\"image\":\"%s\"}}}}}}\\n' "$img" "$img"
          else
            printf 'services:\\n  xianzhi-ai:\\n    image: %s\\n  smartvideo-worker:\\n    image: %s\\n' "$img" "$img"
          fi
          return 0
          ;;
        stop)
          if [ -n "$action_log" ]; then
            (IFS=' '; printf 'docker compose stop %s\\n' "$*") >> "$action_log"
          fi
          if [ "${{MOCK_STOP_FAIL:-0}}" = "1" ]; then
            return 1
          fi
          return 0
          ;;
        ps)
          shift
          local is_status_running=0 is_q=0
          for arg in "$@"; do
            if [ "$arg" = "--status" ]; then is_status_running=1; fi
            if [ "$arg" = "-q" ]; then is_q=1; fi
          done
          if [ "$is_status_running" = "1" ]; then
            if [ "${{MOCK_UNFENCED:-0}}" = "1" ]; then
              echo "cid_rogue"
            fi
            return 0
          fi
          if [ "$is_q" = "1" ]; then
            if [ "${{MOCK_NO_CONTAINERS:-0}}" = "1" ]; then
              return 0
            fi
            echo "cid_mock_1"
            return 0
          fi
          echo "NAME IMAGE STATUS"
          return 0
          ;;
        rm)
          if [ -n "$action_log" ]; then
            (IFS=' '; printf 'docker compose rm %s\\n' "$*") >> "$action_log"
          fi
          return 0
          ;;
        pull)
          if [ -n "$action_log" ]; then
            (IFS=' '; printf 'docker compose pull %s\\n' "$*") >> "$action_log"
          fi
          return 0
          ;;
        up)
          if [ -n "$action_log" ]; then
            (IFS=' '; printf 'docker compose up %s\\n' "$*") >> "$action_log"
          fi
          return 0
          ;;
        *)
          shift
          ;;
      esac
    done
    return 0
  fi
  if [ "$1" = "inspect" ]; then
    if [ "${{MOCK_CONTAINER_NOT_EXITED:-0}}" = "1" ]; then
      if [ "$3" = "{{{{.State.Running}}}}" ]; then echo "true"; return 0; fi
      if [ "$3" = "{{{{.State.Status}}}}" ]; then echo "running"; return 0; fi
    fi
    if [ "$3" = "{{{{.State.Running}}}}" ]; then echo "false"; return 0; fi
    if [ "$3" = "{{{{.State.Status}}}}" ]; then echo "exited"; return 0; fi
    if [ "$3" = "{{{{.Config.Image}}}}" ]; then
      echo "${{XIANZHI_IMAGE_REFERENCE:-$EXPECTED_REF}}"
      return 0
    fi
    if [ "$3" = "{{{{.Image}}}}" ]; then
      echo "$EXPECTED_IMAGE_ID"
      return 0
    fi
    return 0
  fi
  if [ "$1" = "image" ]; then
    if [ "$2" = "inspect" ]; then
      if printf '%s\\n' "$*" | grep -q -- 'RepoDigests'; then
        echo "${{XIANZHI_IMAGE_REFERENCE:-$EXPECTED_REF}}"
        return 0
      fi
      echo "$EXPECTED_IMAGE_ID"
      return 0
    fi
    if [ "$2" = "prune" ]; then
      return 0
    fi
  fi
  command docker "$@"
}}
export -f docker

exec "$@"
"""
        runner_file = sb_dir / "runner.sh"
        runner_file.write_bytes(runner_script.encode("utf-8"))

        env = os.environ.copy()
        env["ACTION_LOG"] = to_bash_path(action_log)
        env["EXPECTED_REF"] = expected_ref
        env["EXPECTED_IMAGE_ID"] = expected_image_id
        env["XIANZHI_TEST_CONTAINER"] = self.container
        env["POSTGRES_USER"] = "postgres"
        env["POSTGRES_DB"] = "xianzhi_test"
        env["POSTGRES_PASSWORD"] = "test_secret_pass"
        env["PRESTAGE_DIR"] = to_bash_path(prestage_dir)
        env["QUARANTINE_BACKUP_DIR"] = to_bash_path(quarantine_backup_dir)
        env["ENV_BACKUP_DIR"] = to_bash_path(env_backup_dir)
        env["RELEASE_LEDGER_FILE"] = to_bash_path(sb_dir / "backups" / "release-ledger.json")
        if extra_env:
            env.update(extra_env)

        return {
            "dir": sb_dir,
            "runner_file": runner_file,
            "action_log": action_log,
            "env_file": env_file,
            "compose_file": compose_file,
            "prestage_dir": prestage_dir,
            "quarantine_backup_dir": quarantine_backup_dir,
            "expected_ref": expected_ref,
            "expected_image_id": expected_image_id,
            "env": env
        }

    def run_rollback(self, sb, args):
        cmd = [
            BASH_EXE, to_bash_path(sb["runner_file"]),
            BASH_EXE, to_bash_path(ROOT / "rollback.sh")
        ] + args
        return subprocess.run(cmd, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=sb["env"])

    def create_receipt_fixture(self, sb, git_sha="79a3d7cf493453adbcdc03234b62fc5f099f776a"):
        manifest_path = sb["dir"] / "rollback-manifest.json"
        manifest_dict = {
            "git_sha": git_sha,
            "image": "ghcr.io/lmxchyy/zhiqiyun-ai",
            "digest": sb["expected_ref"].split("@")[1],
            "image_reference": sb["expected_ref"]
        }
        manifest_bytes = json.dumps(manifest_dict, sort_keys=True).encode("utf-8")
        manifest_path.write_bytes(manifest_bytes)
        manifest_hash = hashlib.sha256(manifest_bytes).hexdigest()

        receipt_path = sb["dir"] / "rollback-receipt.json"
        receipt_dict = {
            "receipt_version": "1.0",
            "previous_git_sha": git_sha,
            "previous_image_reference": sb["expected_ref"],
            "previous_image_id": sb["expected_image_id"],
            "rollback_manifest_path": to_bash_path(manifest_path),
            "rollback_manifest_sha256": manifest_hash
        }
        receipt_path.write_text(json.dumps(receipt_dict, indent=2), encoding="utf-8")
        return receipt_path

    def test_full_immutable_path_positive(self):
        self.seed_quarantine_record(execution_id=501, task_id="task_imm_pos")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox()
        receipt = self.create_receipt_fixture(sb, git_sha="79a3d7cf493453adbcdc03234b62fc5f099f776a")

        r = self.run_rollback(sb, [
            "--receipt", to_bash_path(receipt),
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"])
        ])
        stdout = r.stdout.decode("utf-8", "replace")
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 0, f"Immutable rollback failed (exit {r.returncode}):\nSTDOUT: {stdout}\nSTDERR: {stderr}")
        self.assertIn("QUARANTINE_PRESERVED", stdout)
        self.assertIn("Running services match the rollback release digest", stdout)
        self.assertIn("completed successfully", stdout)

        # Quarantine table must be preserved in DB
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        # Verify action log: stopped containers and started with --pull never
        actions = sb["action_log"].read_text(encoding="utf-8")
        self.assertIn("docker compose stop", actions)
        self.assertIn("--pull never", actions)
        self.assertNotIn("git checkout", actions)

        # Verify env file updated
        env_content = sb["env_file"].read_text(encoding="utf-8")
        self.assertIn(f"XIANZHI_IMAGE_REFERENCE={sb['expected_ref']}", env_content)

    def test_full_legacy_path_positive(self):
        self.seed_quarantine_record(execution_id=502, task_id="task_leg_pos")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={"EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1"})

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "0183f267a"
        ])
        stdout = r.stdout.decode("utf-8", "replace")
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 0, f"Legacy rollback failed (exit {r.returncode}):\nSTDOUT: {stdout}\nSTDERR: {stderr}")
        self.assertIn("QUARANTINE_REVOKED", stdout)
        self.assertIn("Legacy rollback to 0183f267a completed", stdout)

        # Verify table truncated & readback 0
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("0", count_post.stdout.decode())

        # Verify snapshot created and intact
        backups = list(sb["quarantine_backup_dir"].glob("revoked-*-0183f267a*.json"))
        self.assertGreater(len(backups), 0, "Quarantine snapshot file not found")
        snap_data = json.loads(backups[0].read_text(encoding="utf-8"))
        self.assertEqual(snap_data["count"], 1)
        self.assertEqual(snap_data["records"][0]["execution_id"], 502)

        # Verify action log: stopped, checked out, and started with --build
        actions = sb["action_log"].read_text(encoding="utf-8")
        self.assertIn("docker compose stop", actions)
        self.assertIn("git checkout", actions)
        self.assertIn("docker compose up", actions)
        self.assertIn("--build", actions)

    def test_negative_1_legacy_unauthorized(self):
        self.seed_quarantine_record(execution_id=503, task_id="task_leg_unauth")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox()
        sb["env"].pop("EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION", None)

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "0183f267a"
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("LEGACY_TARGET_BARRIER_UNSUPPORTED", stderr)

        # Table must remain INTACT
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        # Neither checkout nor compose up may be called
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("git checkout", actions)
            self.assertNotIn("docker compose up", actions)

    def test_negative_2_unknown_capability(self):
        sb = self.create_rollback_sandbox()
        # Put target in release ledger so it passes forward-release guard and tests capability resolution
        ledger_path = sb["dir"] / "backups" / "release-ledger.json"
        ledger_path.parent.mkdir(parents=True, exist_ok=True)
        ledger_path.write_text('["unknown_nonexistent_sha_99999"]', encoding="utf-8")

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "unknown_nonexistent_sha_99999"
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("TARGET_CAPABILITY_UNKNOWN", stderr)
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("git checkout", actions)
            self.assertNotIn("docker compose up", actions)

    def test_negative_3_forward_release_rejected(self):
        sb = self.create_rollback_sandbox()
        tree_id = subprocess.check_output(["git", "write-tree"], cwd=ROOT).decode("utf-8").strip()
        git_env = dict(os.environ,
                       GIT_AUTHOR_NAME="ci", GIT_AUTHOR_EMAIL="ci@example.com",
                       GIT_COMMITTER_NAME="ci", GIT_COMMITTER_EMAIL="ci@example.com")
        non_ancestor = subprocess.check_output(
            ["git", "commit-tree", tree_id, "-m", "synthetic non-ancestor commit"],
            cwd=ROOT, env=git_env
        ).decode("utf-8").strip()

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            non_ancestor
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("ROLLBACK_FORWARD_REJECTED", stderr)
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("git checkout", actions)
            self.assertNotIn("docker compose up", actions)

    def test_negative_4_snapshot_failure_no_truncate(self):
        self.seed_quarantine_record(execution_id=504, task_id="task_snap_fail")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "SIMULATE_SNAPSHOT_FAILURE": "1"
        })

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "0183f267a"
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("SNAPSHOT_WRITE_FAILED", stderr)

        # TRUNCATE must NEVER have been executed: count remains 1!
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("git checkout", actions)
            self.assertNotIn("docker compose up", actions)

    def test_negative_5_post_truncate_readback_mismatch(self):
        self.seed_quarantine_record(execution_id=505, task_id="task_readback_fail")
        sb = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "SIMULATE_POST_TRUNCATE_READBACK_MISMATCH": "1"
        })

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "0183f267a"
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("READBACK_VERIFICATION_FAILED", stderr)

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("git checkout", actions)
            self.assertNotIn("docker compose up", actions)

    def test_negative_6_container_stop_failure(self):
        # Case A: Stop command failure
        sb_a = self.create_rollback_sandbox(extra_env={"MOCK_STOP_FAIL": "1"})
        r_a = self.run_rollback(sb_a, [
            "--compose-file", to_bash_path(sb_a["compose_file"]),
            "--env-file", to_bash_path(sb_a["env_file"]),
            "0183f267a"
        ])
        stderr_a = r_a.stderr.decode("utf-8", "replace")
        self.assertEqual(r_a.returncode, 1)
        self.assertIn("CONTAINER_STOP_FAILED", stderr_a)

        # Case B: Container fails to reach exited state
        sb_b = self.create_rollback_sandbox(extra_env={"MOCK_CONTAINER_NOT_EXITED": "1"})
        r_b = self.run_rollback(sb_b, [
            "--compose-file", to_bash_path(sb_b["compose_file"]),
            "--env-file", to_bash_path(sb_b["env_file"]),
            "0183f267a"
        ])
        stderr_b = r_b.stderr.decode("utf-8", "replace")
        self.assertEqual(r_b.returncode, 1)
        self.assertIn("FENCING_FAILED", stderr_b)


if __name__ == "__main__":
    unittest.main(verbosity=2)
