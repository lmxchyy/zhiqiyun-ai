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
import io
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
from unittest.mock import MagicMock, patch
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

    @classmethod
    def get_preflight_python_code(cls):
        rollback_src = (ROOT / "rollback.sh").read_text(encoding="utf-8")
        m = re.search(r"preflight_check_quarantine_revocation\(\)\s*\{.*?python3 - .*?<<['\"]PY['\"].*?\n(.*?)\nPY\n\}", rollback_src, re.DOTALL)
        assert m, "Could not extract preflight python code from rollback.sh"
        return m.group(1)

    def test_preflight_and_cleanup_credentials_stay_out_of_argv_and_output(self):
        source = (ROOT / "rollback.sh").read_text(encoding="utf-8")
        cleanup_source = source.split("cleanup_quarantine_for_rollback() {", 1)[1]
        cleanup_code = cleanup_source.split("<<'PY'", 1)[1].split("\n", 1)[1].split("\nPY\n}", 1)[0]
        for phase, code in (("preflight", self.get_preflight_python_code()), ("cleanup", cleanup_code)):
            for branch in ("compose", "test_container"):
                for configured in (True, False):
                    for failure in (True, False):
                        with self.subTest(phase=phase, branch=branch, configured=configured, failure=failure):
                            secret = "SYNTHETIC_ONLY_SECRET_WITHOUT_PASSWORD_PREFIX"
                            ambient = "SYNTHETIC_AMBIENT_PGPASSWORD"
                            env_path = Path(self.tmpdir) / "credentials-fixture.env"
                            env_path.write_text("POSTGRES_PASSWORD=" + secret + "\n" if configured else "", encoding="utf-8")
                            env = {"PGPASSWORD": ambient, "QUARANTINE_BACKUP_DIR": str(Path(self.tmpdir) / "credential-probe")}
                            if branch == "test_container":
                                env["XIANZHI_TEST_CONTAINER"] = "inert-container-only"
                                if configured:
                                    env["POSTGRES_PASSWORD"] = secret
                            captured = []
                            def fake_popen(argv, **kwargs):
                                captured.append((argv, kwargs))
                                proc = MagicMock()
                                proc.returncode = 1 if failure else 0
                                # Missing table allows cleanup to exit BEFORE any snapshot or TRUNCATE.
                                output = b"f|t\n" if phase == "preflight" else b"f\n"
                                proc.communicate.return_value = (output, ("RAW_DB_ERROR " + secret + ambient).encode())
                                return proc
                            stdout, stderr = io.StringIO(), io.StringIO()
                            argv = ["python", "synthetic-compose.yml", str(env_path), "a" * 40, "synthetic-timestamp"]
                            with patch.dict(os.environ, env, clear=True), patch("sys.argv", argv), patch("sys.stdout", stdout), patch("sys.stderr", stderr), patch("subprocess.Popen", side_effect=fake_popen):
                                with self.assertRaises(SystemExit) as stopped:
                                    exec(compile(code, "rollback-credential-regression", "exec"), {"__name__": "__main__"})
                            self.assertEqual(stopped.exception.code, 1 if failure else 0)
                            self.assertEqual(len(captured), 1)
                            child_argv, child_options = captured[0]
                            self.assertNotIn(secret, " ".join(child_argv))
                            self.assertNotIn(ambient, " ".join(child_argv))
                            self.assertEqual(child_options["env"].get("PGPASSWORD"), secret if configured else None)
                            if configured:
                                self.assertEqual(child_argv[child_argv.index("-e") + 1], "PGPASSWORD")
                            for forbidden in (secret, ambient, "RAW_DB_ERROR"):
                                self.assertNotIn(forbidden, stdout.getvalue() + stderr.getvalue())

    def test_preflight_unit_test_container_branch_argv_env_no_secret_leak(self):
        code = self.get_preflight_python_code()
        captured_popen = []
        def mock_popen(*args, **kwargs):
            captured_popen.append((args, kwargs))
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        backup_dir = os.path.join(self.tmpdir, "backup_test_branch")
        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "POSTGRES_USER": "test_pg_user",
            "POSTGRES_DB": "test_pg_db",
            "POSTGRES_PASSWORD": "super_secret_test_pw_456",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "mock-compose.yml", "mock.env", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        try:
                            exec(code, {"__name__": "__main__"})
                            exit_code = 0
                        except SystemExit as se:
                            exit_code = se.code

        self.assertEqual(exit_code, 0)
        self.assertEqual(len(captured_popen), 1)
        argv, kwargs = captured_popen[0]
        psql_base = argv[0]
        child_env = kwargs.get("env", {})

        # Assert value-free -e PGPASSWORD forwarding
        self.assertIn("-e", psql_base)
        e_idx = psql_base.index("-e")
        self.assertEqual(psql_base[e_idx + 1], "PGPASSWORD")

        # Assert secret is NOT in argv
        for arg in psql_base:
            self.assertNotIn("super_secret_test_pw_456", arg)

        # Assert child env has the secret
        self.assertEqual(child_env.get("PGPASSWORD"), "super_secret_test_pw_456")

    def test_preflight_unit_compose_branch_argv_env_no_secret_leak(self):
        code = self.get_preflight_python_code()
        captured_popen = []
        def mock_popen(*args, **kwargs):
            captured_popen.append((args, kwargs))
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        env_file = os.path.join(self.tmpdir, "prod.env")
        with open(env_file, "w", encoding="utf-8") as f:
            f.write("POSTGRES_USER=prod_user\nPOSTGRES_DB=prod_db\nPOSTGRES_PASSWORD=super_secret_prod_pw_789\n")

        backup_dir = os.path.join(self.tmpdir, "backup_compose_branch")
        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", env_file, "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        try:
                            exec(code, {"__name__": "__main__"})
                            exit_code = 0
                        except SystemExit as se:
                            exit_code = se.code

        self.assertEqual(exit_code, 0)
        self.assertEqual(len(captured_popen), 1)
        argv, kwargs = captured_popen[0]
        psql_base = argv[0]
        child_env = kwargs.get("env", {})

        # Assert value-free -e PGPASSWORD forwarding
        self.assertIn("-e", psql_base)
        e_idx = psql_base.index("-e")
        self.assertEqual(psql_base[e_idx + 1], "PGPASSWORD")

        # Assert secret is NOT in argv
        for arg in psql_base:
            self.assertNotIn("super_secret_prod_pw_789", arg)

        # Assert child env has the secret
        self.assertEqual(child_env.get("PGPASSWORD"), "super_secret_prod_pw_789")

    def test_preflight_unit_synthetic_error_no_secret_or_raw_stderr_leak(self):
        code = self.get_preflight_python_code()
        def mock_popen_fail(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 1
            proc.communicate.return_value = (b"", b"FATAL: password authentication failed for user 'prod_user' with password=raw_super_secret_password")
            return proc

        env_file = os.path.join(self.tmpdir, "prod_fail.env")
        with open(env_file, "w", encoding="utf-8") as f:
            f.write("POSTGRES_PASSWORD=raw_super_secret_password\n")

        backup_dir = os.path.join(self.tmpdir, "backup_fail")
        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", env_file, "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen_fail):
                        try:
                            exec(code, {"__name__": "__main__"})
                            exit_code = 0
                        except SystemExit as se:
                            exit_code = se.code

        self.assertEqual(exit_code, 1)
        stderr_out = captured_stderr.getvalue()
        self.assertIn("ERROR: Failed to inspect database relations", stderr_out)
        self.assertNotIn("raw_super_secret_password", stderr_out)
        self.assertNotIn("FATAL: password authentication failed", stderr_out)

    def test_preflight_unit_sentinel_preservation_and_exclusive_cleanup(self):
        code = self.get_preflight_python_code()

        backup_dir = os.path.join(self.tmpdir, "backup_sentinel")
        os.makedirs(backup_dir, exist_ok=True)

        sentinel_file = os.path.join(backup_dir, ".preflight_probe_424242")
        with open(sentinel_file, "w", encoding="utf-8") as f:
            f.write("sentinel_initial_data_must_not_be_overwritten_or_deleted")

        unrelated_file = os.path.join(backup_dir, "prior_backup.json")
        with open(unrelated_file, "w", encoding="utf-8") as f:
            f.write('{"unrelated": true}')

        # Optional symlink sentinel
        symlink_created = False
        symlink_path = os.path.join(backup_dir, ".preflight_probe_symlink")
        try:
            os.symlink(unrelated_file, symlink_path)
            symlink_created = True
        except (OSError, NotImplementedError):
            pass

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("os.getpid", return_value=424242):
                        with patch("subprocess.Popen", side_effect=mock_popen):
                            try:
                                exec(code, {"__name__": "__main__"})
                                exit_code = 0
                            except SystemExit as se:
                                exit_code = se.code

        self.assertEqual(exit_code, 0)
        # Assert sentinel file preserved
        self.assertTrue(os.path.isfile(sentinel_file))
        with open(sentinel_file, "r", encoding="utf-8") as f:
            self.assertEqual(f.read(), "sentinel_initial_data_must_not_be_overwritten_or_deleted")

        # Assert unrelated file preserved
        self.assertTrue(os.path.isfile(unrelated_file))
        with open(unrelated_file, "r", encoding="utf-8") as f:
            self.assertEqual(f.read(), '{"unrelated": true}')

        # Assert symlink preserved
        if symlink_created:
            self.assertTrue(os.path.islink(symlink_path))
            with open(symlink_path, "r", encoding="utf-8") as f:
                self.assertEqual(f.read(), '{"unrelated": true}')

        # Assert only own probe was cleaned up: no new .preflight_probe_* files left behind
        all_entries = os.listdir(backup_dir)
        allowed = {".preflight_probe_424242", "prior_backup.json"}
        if symlink_created:
            allowed.add(".preflight_probe_symlink")
        for entry in all_entries:
            self.assertIn(entry, allowed, f"Unexpected leftover file in backup dir: {entry}")

    def test_preflight_unit_table_presence_false_vs_permission_failure(self):
        code = self.get_preflight_python_code()

        # Case A: Table presence false (f|t) -> exits 0 (no revocation needed)
        def mock_popen_not_found(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"f|t\n", b"")
            return proc

        backup_dir_a = os.path.join(self.tmpdir, "backup_not_found")
        captured_stderr_a = io.StringIO()
        test_env_a = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir_a,
        }
        with patch.dict(os.environ, test_env_a, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr_a):
                    with patch("subprocess.Popen", side_effect=mock_popen_not_found):
                        try:
                            exec(code, {"__name__": "__main__"})
                            exit_code_a = 0
                        except SystemExit as se:
                            exit_code_a = se.code

        self.assertEqual(exit_code_a, 0)

        # Case B: Table presence true, but permissions missing (t|f) -> exits 1
        def mock_popen_no_privs(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|f\n", b"")
            return proc

        backup_dir_b = os.path.join(self.tmpdir, "backup_no_privs")
        captured_stderr_b = io.StringIO()
        test_env_b = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir_b,
        }
        with patch.dict(os.environ, test_env_b, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr_b):
                    with patch("subprocess.Popen", side_effect=mock_popen_no_privs):
                        try:
                            exec(code, {"__name__": "__main__"})
                            exit_code_b = 0
                        except SystemExit as se:
                            exit_code_b = se.code

        self.assertEqual(exit_code_b, 1)
        self.assertIn("Insufficient privileges", captured_stderr_b.getvalue())

    def test_probe_cleanup_detects_ordinary_file_replacement_and_blocks(self):
        code = self.get_preflight_python_code()
        backup_dir = os.path.join(self.tmpdir, "backup_probe_ordinary_repl")
        os.makedirs(backup_dir, exist_ok=True)
        sentinel_file = os.path.join(backup_dir, "sentinel.txt")
        with open(sentinel_file, "w", encoding="utf-8") as f:
            f.write("SENTINEL_DATA_MUST_NOT_BE_DELETED")

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        real_close = os.close
        tampered_path = []
        def adversary_close(fd):
            real_close(fd)
            probes = [os.path.join(backup_dir, f) for f in os.listdir(backup_dir) if f.startswith(".preflight_probe_")]
            if probes:
                probe_p = probes[0]
                tampered_path.append(probe_p)
                os.remove(probe_p)
                os.rename(sentinel_file, probe_p)

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        with patch("os.close", side_effect=adversary_close):
                            with self.assertRaises(SystemExit) as cm:
                                exec(code, {"__name__": "__main__"})

        self.assertEqual(cm.exception.code, 1)
        self.assertIn("file identity mismatch", captured_stderr.getvalue())
        # Assert sentinel was renamed to probe_p, but MUST NOT have been deleted!
        self.assertTrue(os.path.exists(tampered_path[0]))
        with open(tampered_path[0], "r", encoding="utf-8") as f:
            self.assertEqual(f.read(), "SENTINEL_DATA_MUST_NOT_BE_DELETED")

    def test_probe_cleanup_detects_inode_mismatch_with_identical_token_and_blocks(self):
        code = self.get_preflight_python_code()
        backup_dir = os.path.join(self.tmpdir, "backup_probe_inode_mismatch")
        os.makedirs(backup_dir, exist_ok=True)

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        real_close = os.close
        real_lstat = os.lstat
        tampered_path = []

        def adversary_close(fd):
            real_close(fd)
            probes = [os.path.join(backup_dir, f) for f in os.listdir(backup_dir) if f.startswith(".preflight_probe_")]
            if probes:
                probe_p = probes[0]
                tampered_path.append(probe_p)
                with open(probe_p, "rb") as pf:
                    token = pf.read()
                os.remove(probe_p)
                # Recreate with identical content; inode reuse is filesystem-dependent.
                with open(probe_p, "wb") as pf:
                    pf.write(token)

        def replacement_lstat(path):
            st = real_lstat(path)
            if tampered_path and path == tampered_path[0]:
                values = list(st)
                values[1] = st.st_ino + 1
                return os.stat_result(values)
            return st

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        with patch("os.close", side_effect=adversary_close):
                            with patch("os.lstat", side_effect=replacement_lstat):
                                with self.assertRaises(SystemExit) as cm:
                                    exec(code, {"__name__": "__main__"})

        self.assertEqual(cm.exception.code, 1)
        self.assertIn("file identity mismatch", captured_stderr.getvalue())
        # Replacement file must NOT have been deleted
        self.assertTrue(os.path.exists(tampered_path[0]))

    def test_probe_cleanup_detects_same_inode_token_change_and_blocks(self):
        code = self.get_preflight_python_code()
        backup_dir = os.path.join(self.tmpdir, "backup_probe_token_change")
        os.makedirs(backup_dir, exist_ok=True)

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        real_close = os.close
        tampered_path = []
        def adversary_close(fd):
            real_close(fd)
            probes = [os.path.join(backup_dir, f) for f in os.listdir(backup_dir) if f.startswith(".preflight_probe_")]
            if probes:
                probe_p = probes[0]
                tampered_path.append(probe_p)
                # Overwrite content in place (same inode) with tampered token
                with open(probe_p, "wb") as pf:
                    pf.write(b"tampered_token_payload\n")

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        with patch("os.close", side_effect=adversary_close):
                            with self.assertRaises(SystemExit) as cm:
                                exec(code, {"__name__": "__main__"})

        self.assertEqual(cm.exception.code, 1)
        self.assertIn("token/content mismatch", captured_stderr.getvalue())
        # File must NOT have been deleted
        self.assertTrue(os.path.exists(tampered_path[0]))
        with open(tampered_path[0], "rb") as pf:
            self.assertEqual(pf.read(), b"tampered_token_payload\n")

    def test_probe_cleanup_detects_symlink_substitution_and_blocks(self):
        code = self.get_preflight_python_code()
        backup_dir = os.path.join(self.tmpdir, "backup_probe_symlink_sub")
        os.makedirs(backup_dir, exist_ok=True)
        sentinel_file = os.path.join(backup_dir, "sentinel.txt")
        with open(sentinel_file, "w", encoding="utf-8") as f:
            f.write("SENTINEL_NEVER_DELETE")

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        real_close = os.close
        tampered_path = []
        symlink_supported = [True]

        def adversary_close(fd):
            real_close(fd)
            probes = [os.path.join(backup_dir, f) for f in os.listdir(backup_dir) if f.startswith(".preflight_probe_")]
            if probes:
                probe_p = probes[0]
                tampered_path.append(probe_p)
                os.remove(probe_p)
                try:
                    os.symlink(sentinel_file, probe_p)
                except (OSError, NotImplementedError):
                    symlink_supported[0] = False

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        with patch("os.close", side_effect=adversary_close):
                            if symlink_supported[0]:
                                try:
                                    exec(code, {"__name__": "__main__"})
                                    exit_code = 0
                                except SystemExit as se:
                                    exit_code = se.code
                            else:
                                exit_code = 1

        if symlink_supported[0] and tampered_path and os.path.islink(tampered_path[0]):
            self.assertEqual(exit_code, 1)
            self.assertIn("substituted with a symlink", captured_stderr.getvalue())
            self.assertTrue(os.path.islink(tampered_path[0]))
            self.assertTrue(os.path.exists(sentinel_file))
        else:
            captured_stderr_mock = io.StringIO()
            with patch.dict(os.environ, test_env, clear=True):
                with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                    with patch("sys.stderr", captured_stderr_mock):
                        with patch("subprocess.Popen", side_effect=mock_popen):
                            with patch("os.path.islink", return_value=True):
                                with self.assertRaises(SystemExit) as cm:
                                    exec(code, {"__name__": "__main__"})
            self.assertEqual(cm.exception.code, 1)
            self.assertIn("substituted with a symlink", captured_stderr_mock.getvalue())

    def test_probe_cleanup_normal_own_file_succeeds(self):
        code = self.get_preflight_python_code()
        backup_dir = os.path.join(self.tmpdir, "backup_probe_normal")
        os.makedirs(backup_dir, exist_ok=True)

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        try:
                            exec(code, {"__name__": "__main__"})
                            exit_code = 0
                        except SystemExit as se:
                            exit_code = se.code

        self.assertEqual(exit_code, 0)
        remaining = [f for f in os.listdir(backup_dir) if f.startswith(".preflight_probe_")]
        self.assertEqual(remaining, [])

    def test_probe_cleanup_no_false_positive_inode_reuse_blocks(self):
        code = self.get_preflight_python_code()
        backup_dir = os.path.join(self.tmpdir, "backup_probe_inode_reuse")
        os.makedirs(backup_dir, exist_ok=True)

        def mock_popen(*args, **kwargs):
            proc = MagicMock()
            proc.returncode = 0
            proc.communicate.return_value = (b"t|t\n", b"")
            return proc

        real_close = os.close
        tampered_path = []
        captured_orig_stat = []

        def adversary_close(fd):
            st = os.fstat(fd)
            captured_orig_stat.append(st)
            real_close(fd)
            probes = [os.path.join(backup_dir, f) for f in os.listdir(backup_dir) if f.startswith(".preflight_probe_")]
            if probes:
                probe_p = probes[0]
                tampered_path.append(probe_p)
                os.remove(probe_p)
                # Attacker creates replacement file with attacker token
                with open(probe_p, "wb") as pf:
                    pf.write(b"attacker_arbitrary_content\n")

        real_lstat = os.lstat
        def simulated_reuse_lstat(path):
            lst = real_lstat(path)
            if tampered_path and path == tampered_path[0] and captured_orig_stat:
                # Simulate exact inode reuse: dev and ino match original probe
                mock_st = MagicMock(wraps=lst)
                mock_st.st_dev = captured_orig_stat[0].st_dev
                mock_st.st_ino = captured_orig_stat[0].st_ino
                mock_st.st_mode = lst.st_mode
                return mock_st
            return lst

        captured_stderr = io.StringIO()
        test_env = {
            "PATH": os.environ.get("PATH", ""),
            "XIANZHI_TEST_CONTAINER": "mock-test-container",
            "QUARANTINE_BACKUP_DIR": backup_dir,
        }
        with patch.dict(os.environ, test_env, clear=True):
            with patch("sys.argv", ["python", "docker-compose.yml", "none", "target_sha_123"]):
                with patch("sys.stderr", captured_stderr):
                    with patch("subprocess.Popen", side_effect=mock_popen):
                        with patch("os.close", side_effect=adversary_close):
                            with patch("os.lstat", side_effect=simulated_reuse_lstat):
                                with self.assertRaises(SystemExit) as cm:
                                    exec(code, {"__name__": "__main__"})

        self.assertEqual(cm.exception.code, 1)
        # Even with identical inode/dev (simulated reuse), unpredictable token prevents false-positive cleanup
        self.assertIn("token/content mismatch", captured_stderr.getvalue())
        self.assertTrue(os.path.exists(tampered_path[0]))
        with open(tampered_path[0], "rb") as pf:
            self.assertEqual(pf.read(), b"attacker_arbitrary_content\n")


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
        cls.fixture_owner = str(uuid.uuid4())
        cls.container = "issue203-pg-test-" + cls.fixture_owner.replace('-', '')[:10]
        image_id = subprocess.check_output(["docker", "image", "inspect", "pgvector/pgvector:pg16", "--format", "{{.Id}}"], stderr=subprocess.PIPE).decode().strip()
        cls.fixture_id = subprocess.check_output([
            "docker", "run", "-d", "--pull", "never", "--network", "none",
            "--label", "issue203.control-plane.owner=" + cls.fixture_owner,
            "--tmpfs", "/var/lib/postgresql/data", "--name", cls.container,
            "-e", "POSTGRES_PASSWORD=test_secret_pass",
            "-e", "POSTGRES_USER=postgres",
            "-e", "POSTGRES_DB=xianzhi_test", image_id
        ], stdin=subprocess.DEVNULL, stderr=subprocess.PIPE).decode().strip()
        cls.own_container = True
        record = ROOT / '.evidence/issue203/priority4-runtime-capability' / ('control-owned-' + cls.fixture_owner + '.json')
        record.parent.mkdir(parents=True, exist_ok=True)
        record.write_text(json.dumps({'owner': cls.fixture_owner, 'container_id': cls.fixture_id, 'image_id': image_id, 'pid': os.getpid(), 'started_at_unix': time.time()}, sort_keys=True), encoding='utf-8')
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
            cls.cleanup_owned_fixture()
            raise

    @classmethod
    def cleanup_owned_fixture(cls):
        if getattr(cls, "own_container", False):
            obj = json.loads(subprocess.check_output(["docker", "inspect", cls.fixture_id], stderr=subprocess.PIPE))[0]
            if obj['Id'] != cls.fixture_id or obj['Config']['Labels'].get('issue203.control-plane.owner') != cls.fixture_owner or obj['HostConfig']['NetworkMode'] != 'none':
                raise RuntimeError('test fixture ownership mismatch; no cleanup')
            subprocess.run(["docker", "rm", "-f", "-v", cls.fixture_id], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
            cls.own_container = False

    @classmethod
    def tearDownClass(cls):
        cls.cleanup_owned_fixture()

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

    def test_full_immutable_old_schema_without_runtime_proof_rejected(self):
        # Named P4 contract change: Migration121/source capability is no longer
        # enough to authorize rollback, even with a healthy quarantine table.
        self.seed_quarantine_record(execution_id=501, task_id="task_imm_pos")
        sb = self.create_rollback_sandbox()
        receipt = self.create_receipt_fixture(sb, git_sha="79a3d7cf493453adbcdc03234b62fc5f099f776a")
        r = self.run_rollback(sb, ["--receipt", to_bash_path(receipt), "--compose-file", to_bash_path(sb["compose_file"]), "--env-file", to_bash_path(sb["env_file"])])
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUNTIME_CAPABILITY_PROOF_MISSING", r.stderr.decode("utf-8", "replace"))
        self.assertIn("1", self.psql("SELECT count(*) FROM provider_execution_quarantine;").stdout.decode())
        self.assert_no_cutover_actions(sb)

    def test_full_legacy_emergency_rejected_before_stop(self):
        # Named P4 contract change: stop->revoke->build is intentionally removed.
        self.seed_quarantine_record(execution_id=502, task_id="task_leg_pos")
        sb = self.create_rollback_sandbox(extra_env={"EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1"})
        r = self.run_rollback(sb, ["--compose-file", to_bash_path(sb["compose_file"]), "--env-file", to_bash_path(sb["env_file"]), "0183f267a"])
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUNTIME_CAPABILITY_PROOF_MISSING", r.stderr.decode("utf-8", "replace"))
        self.assertIn("1", self.psql("SELECT count(*) FROM provider_execution_quarantine;").stdout.decode())
        self.assert_no_cutover_actions(sb)
        self.assertEqual(list(sb["quarantine_backup_dir"].glob("revoked-*.json")), [])

    def assert_no_cutover_actions(self, sb):
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            for forbidden in ("docker compose stop", "docker compose rm", "docker compose pull", "docker compose up", "git checkout"):
                self.assertNotIn(forbidden, actions)

    def run_rollback_safety_helper(self, sb, helper):
        # Existing cleanup and indeterminate-stop safety remain independently
        # exercised, but no longer imply permission for legacy cutover.
        source = (ROOT / "rollback.sh").read_text(encoding="utf-8")
        start = source.index(helper + "() {")
        end = source.index("\n}\n", start) + 3
        script = '''set -Eeuo pipefail
log(){ printf '%s\\n' "$*"; }
fail(){ printf '%s\\n' "$*" >&2; exit 1; }
'''
        script += source[start:end] + "\n"
        script += "cleanup_quarantine_for_rollback 0183f267a 0\n" if helper == "cleanup_quarantine_for_rollback" else "stop_services_fail_closed\n"
        path = sb["dir"] / "safety-helper.sh"
        path.write_bytes(script.encode("utf-8"))
        env = dict(sb["env"], COMPOSE_FILE=to_bash_path(sb["compose_file"]), ENV_FILE=to_bash_path(sb["env_file"]), TIMESTAMP="synthetic")
        return subprocess.run([BASH_EXE, to_bash_path(sb["runner_file"]), BASH_EXE, to_bash_path(path)], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)

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

        # Failed preflight must cause ZERO stop, recreate, pull, checkout, up, TRUNCATE
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

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
        # Unknown target must fail before stop even if ledger allows ancestry: ZERO stop/rm/pull/up/checkout
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

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
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

    def test_negative_4_snapshot_failure_no_truncate(self):
        self.seed_quarantine_record(execution_id=504, task_id="task_snap_fail")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "SIMULATE_SNAPSHOT_FAILURE": "1"
        })

        r = self.run_rollback_safety_helper(sb, "cleanup_quarantine_for_rollback")
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

        r = self.run_rollback_safety_helper(sb, "cleanup_quarantine_for_rollback")
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("READBACK_VERIFICATION_FAILED", stderr)

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("git checkout", actions)
            self.assertNotIn("docker compose up", actions)

    def test_negative_6_container_stop_failure(self):
        # Case A: Stop command failure (with authorized preflight)
        sb_a = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "MOCK_STOP_FAIL": "1"
        })
        r_a = self.run_rollback_safety_helper(sb_a, "stop_services_fail_closed")
        stderr_a = r_a.stderr.decode("utf-8", "replace")
        self.assertEqual(r_a.returncode, 1)
        self.assertIn("CONTAINER_STOP_FAILED", stderr_a)

        # Case B: Container fails to reach exited state (with authorized preflight)
        sb_b = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "MOCK_CONTAINER_NOT_EXITED": "1"
        })
        r_b = self.run_rollback_safety_helper(sb_b, "stop_services_fail_closed")
        stderr_b = r_b.stderr.decode("utf-8", "replace")
        self.assertEqual(r_b.returncode, 1)
        self.assertIn("FENCING_FAILED", stderr_b)

    def test_negative_1_immutable_legacy_unauthorized(self):
        self.seed_quarantine_record(execution_id=506, task_id="task_imm_leg_unauth")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox()
        sb["env"].pop("EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION", None)
        receipt = self.create_receipt_fixture(sb, git_sha="0183f267a")

        r = self.run_rollback(sb, [
            "--receipt", to_bash_path(receipt),
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"])
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("LEGACY_TARGET_BARRIER_UNSUPPORTED", stderr)

        # Table must remain INTACT
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        # Failed preflight must cause ZERO stop/rm/pull/up/checkout/TRUNCATE in immutable path
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

    def test_negative_2_immutable_unknown_capability(self):
        sb = self.create_rollback_sandbox()
        ledger_path = sb["dir"] / "backups" / "release-ledger.json"
        ledger_path.parent.mkdir(parents=True, exist_ok=True)
        ledger_path.write_text('["unknown_nonexistent_sha_99999"]', encoding="utf-8")
        receipt = self.create_receipt_fixture(sb, git_sha="unknown_nonexistent_sha_99999")

        r = self.run_rollback(sb, [
            "--receipt", to_bash_path(receipt),
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"])
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("TARGET_CAPABILITY_UNKNOWN", stderr)

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

    def test_negative_7_preflight_db_inspection_failure(self):
        self.seed_quarantine_record(execution_id=507, task_id="task_preflight_db_fail")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "SIMULATE_PREFLIGHT_DB_INSPECTION_FAILURE": "1"
        })

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "0183f267a"
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("QUARANTINE_REVOCATION_PREFLIGHT_FAILED", stderr)

        # Table must remain INTACT
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

    def test_negative_8_backup_dir_preflight_failure_mutable_path(self):
        self.seed_quarantine_record(execution_id=508, task_id="task_preflight_backup_fail_mut")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "SIMULATE_PREFLIGHT_BACKUP_DIR_FAILURE": "1"
        })

        r = self.run_rollback(sb, [
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"]),
            "0183f267a"
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("QUARANTINE_REVOCATION_PREFLIGHT_FAILED", stderr)

        # Table must remain INTACT (zero TRUNCATE)
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

    def test_negative_8_backup_dir_preflight_failure_immutable_path(self):
        self.seed_quarantine_record(execution_id=509, task_id="task_preflight_backup_fail_imm")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={
            "EMERGENCY_ALLOW_LEGACY_QUARANTINE_REVOCATION": "1",
            "SIMULATE_PREFLIGHT_BACKUP_DIR_FAILURE": "1"
        })
        receipt = self.create_receipt_fixture(sb, git_sha="0183f267a")

        r = self.run_rollback(sb, [
            "--receipt", to_bash_path(receipt),
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"])
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("QUARANTINE_REVOCATION_PREFLIGHT_FAILED", stderr)

        # Table must remain INTACT (zero TRUNCATE)
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)

    def test_negative_rollback_receipt_manifest_identity_mismatch_zero_stop(self):
        self.seed_quarantine_record(execution_id=510, task_id="task_identity_mismatch")
        count_pre = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_pre.stdout.decode())

        sb = self.create_rollback_sandbox(extra_env={"MOCK_STOP_FAIL": "1"})

        # Parent reproduction:
        # Receipt SHA: 79a3d7cf493453adbcdc03234b62fc5f099f776a
        # Manifest SHA: 0183f267a3faa63e9dec0c14d871ad136fcfe779
        # Rehashed receipt manifest hash
        manifest_path = sb["dir"] / "rollback-manifest.json"
        manifest_dict = {
            "git_sha": "0183f267a3faa63e9dec0c14d871ad136fcfe779",
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
            "previous_git_sha": "79a3d7cf493453adbcdc03234b62fc5f099f776a",
            "previous_image_reference": sb["expected_ref"],
            "previous_image_id": sb["expected_image_id"],
            "rollback_manifest_path": to_bash_path(manifest_path),
            "rollback_manifest_sha256": manifest_hash
        }
        receipt_path.write_text(json.dumps(receipt_dict, indent=2), encoding="utf-8")

        r = self.run_rollback(sb, [
            "--receipt", to_bash_path(receipt_path),
            "--compose-file", to_bash_path(sb["compose_file"]),
            "--env-file", to_bash_path(sb["env_file"])
        ])
        stderr = r.stderr.decode("utf-8", "replace")
        self.assertEqual(r.returncode, 1)
        self.assertIn("ROLLBACK_IDENTITY_MISMATCH", stderr)
        self.assertNotIn("CONTAINER_STOP_FAILED", stderr)

        # Table must remain INTACT
        count_post = self.psql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertIn("1", count_post.stdout.decode())

        # STOP invocation count must be EXACTLY 0
        if sb["action_log"].exists():
            actions = sb["action_log"].read_text(encoding="utf-8")
            self.assertNotIn("docker compose stop", actions)
            self.assertNotIn("docker compose rm", actions)
            self.assertNotIn("docker compose pull", actions)
            self.assertNotIn("docker compose up", actions)
            self.assertNotIn("git checkout", actions)


if __name__ == "__main__":
    unittest.main(verbosity=2)
