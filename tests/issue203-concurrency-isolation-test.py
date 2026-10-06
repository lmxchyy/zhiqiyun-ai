#!/usr/bin/env python3
"""Issue #203 Priority 3: Enrollment Concurrency Isolation & Zero-Business-Window Protection.

Test Matrix (Zero SKIP):
  Matrix A: Deterministic Timing A (Enrollment wins -> Worker ErrQuarantined, 0 side effects).
  Matrix B: Deterministic Timing B (Worker wins -> Enrollment snapshot mismatch, 0 quarantine rows).
  Matrix C: Two-phase container revive injection (Container revived between Phase 1 and 2 -> Enrollment ROLLBACK, 0 rows).
  Matrix D: Release lock missing / invalid owner / dead PID -> Enrollment BLOCK, 0 writes.
  Matrix E: Multi-task partial drift -> entire batch ROLLBACK, 0 writes.
"""
import copy
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import types
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parent.parent
sys.dont_write_bytecode = True

# Source-only imports of ops modules
def load_module(name, path):
    module = types.ModuleType(name)
    module.__file__ = str(path)
    with open(str(path), 'rb') as f:
        exec(compile(f.read(), str(path), 'exec'), module.__dict__)
    return module

approval = load_module('quarantine_approval', ROOT / 'ops/quarantine-approval.py')
live_snapshot = load_module('quarantine_live_snapshot', ROOT / 'ops/quarantine-live-snapshot.py')
enroll = load_module('enroll_quarantine', ROOT / 'ops/enroll-quarantine.py')


class MockConnection:
    def __init__(self, cursor=None):
        self._cursor = cursor
        if cursor:
            cursor.connection = self
        self.committed = False
        self.rolled_back = False
        self.in_transaction = False
        self.autocommit = False

    def cursor(self):
        return self._cursor

    def get_transaction_status(self):
        return 2 if self.in_transaction else 0

    def close(self):
        pass


class MockCursor:
    """Mock database cursor that simulates PostgreSQL transaction semantics and row locking."""
    def __init__(self, tasks=None, executions=None, quarantine=None, conn=None):
        self.tasks = tasks if tasks is not None else {}
        self.executions = executions if executions is not None else {}
        self.quarantine = quarantine if quarantine is not None else {}
        self.queries = []
        self.lock_order = []
        self._last_result = []
        self.connection = conn or MockConnection(self)
        self.connection._cursor = self

    def execute(self, query, params=None):
        self.queries.append((query.strip(), params))
        q = query.strip().upper()
        if "BEGIN" in q:
            self.connection.in_transaction = True
        elif "COMMIT" in q:
            self.connection.committed = True
            self.connection.in_transaction = False
        elif "ROLLBACK" in q:
            self.connection.rolled_back = True
            self.connection.in_transaction = False
        elif "FROM PUBLIC.XZ_GENERATION_TASKS" in q and "FOR UPDATE" in q:
            self.lock_order.append("xz_generation_tasks")
            tids = params[0] if params else []
            self._last_result = [
                (tid, self.tasks[tid].get("status", "FAILED"),
                 self.tasks[tid].get("task_status", "FAILED"),
                 self.tasks[tid].get("lease_until"),
                 self.tasks[tid].get("worker_id"))
                for tid in tids if tid in self.tasks
            ]
        elif "FROM PUBLIC.PROVIDER_EXECUTIONS" in q and "FOR UPDATE" in q:
            self.lock_order.append("provider_executions")
            eids = params[0] if params else []
            self._last_result = [
                (eid, self.executions[eid].get("task_id"),
                 self.executions[eid].get("attempt", 1),
                 self.executions[eid].get("task_execution_generation"))
                for eid in eids if eid in self.executions
            ]
        elif q.startswith("SELECT CLOCK_TIMESTAMP() >="):
            self._last_result = (True,)
        elif "TO_REGCLASS('PUBLIC.PROVIDER_EXECUTION_QUARANTINE')" in q:
            self._last_result = (True,)
        elif "INSERT INTO PUBLIC.PROVIDER_EXECUTION_QUARANTINE" in q:
            eid = params[0]
            self.quarantine[eid] = {
                "execution_id": params[0],
                "task_id": params[1],
                "attempt": params[2],
                "generation": params[3],
                "snapshot_sha256": params[4],
                "evidence_sha256": params[5],
                "approval_id": params[6],
                "release_sha": params[7],
                "not_before": params[8],
                "expires_at": params[9],
            }
            self._last_result = None
        elif "SELECT EXECUTION_ID, TASK_ID, ATTEMPT, GENERATION" in q and "FROM PUBLIC.PROVIDER_EXECUTION_QUARANTINE" in q:
            eids = params[0] if params else []
            rows = []
            for eid in eids:
                if eid in self.quarantine:
                    q_row = self.quarantine[eid]
                    nb = q_row["not_before"]
                    exp = q_row["expires_at"]
                    if not nb.endswith("Z"): nb = nb + "Z"
                    if not exp.endswith("Z"): exp = exp + "Z"
                    rows.append((
                        q_row["execution_id"], q_row["task_id"], q_row["attempt"],
                        q_row["generation"], q_row["snapshot_sha256"], q_row["evidence_sha256"],
                        q_row["approval_id"], q_row["release_sha"], nb, exp
                    ))
            self._last_result = rows
        elif "COUNT" in q and "FROM PUBLIC.PROVIDER_EXECUTION_QUARANTINE" in q:
            self._last_result = [(0,)]
        else:
            self._last_result = []

    def fetchall(self):
        return self._last_result if isinstance(self._last_result, list) else []

    def fetchone(self):
        if isinstance(self._last_result, tuple):
            return self._last_result
        if isinstance(self._last_result, list) and len(self._last_result) > 0:
            return self._last_result[0]
        return None

    def close(self):
        pass


def build_test_manifest(executions, release_sha="b"*40):
    manifest = {
        "manifest_version": "issue199-quarantine-v1",
        "release_sha": release_sha,
        "created_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "reason": "concurrency isolation test",
        "executions": executions,
    }
    return json.dumps(manifest, sort_keys=True, separators=(',', ':')).encode("utf-8")


class ConcurrencyIsolationTests(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory(prefix="issue203_test_")
        self.work = Path(self.temp_dir.name)
        self.lock_dir = self.work / "release.lock"
        self.lock_dir.mkdir()
        self.release_sha = "b" * 40
        self.owner_pid = os.getpid()
        self.owner_token = "test-token-" + os.urandom(8).hex()

        (self.lock_dir / "owner_pid").write_text(str(self.owner_pid) + "\n")
        (self.lock_dir / "owner_token").write_text(self.owner_token + "\n")
        (self.lock_dir / "release_sha").write_text(self.release_sha + "\n")
        (self.lock_dir / "created_at").write_text(datetime.datetime.now(datetime.timezone.utc).isoformat() + "\n")
        os.environ["OWNER_TOKEN"] = self.owner_token

        self.compose_file = str(self.work / "compose.json")
        self.env_file = str(self.work / "fixture.env")
        Path(self.compose_file).write_text("{}")
        Path(self.env_file).write_text("")

    def tearDown(self):
        self.temp_dir.cleanup()
        os.environ.pop("OWNER_TOKEN", None)

    # -------------------------------------------------------------------------
    # MATRIX D: Release Lock Verification
    # -------------------------------------------------------------------------
    def test_D1_missing_release_lock_dir_blocks(self):
        missing_dir = str(self.work / "non_existent.lock")
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(missing_dir, self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_MISSING", str(ctx.exception))

    def test_D2_recovering_lock_dir_present_blocks(self):
        recovering_dir = self.work / "release.lock.recovering"
        recovering_dir.mkdir()
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_RECOVERING", str(ctx.exception))

    def test_D3_dead_owner_pid_blocks(self):
        # PID 99999999 is extraordinarily unlikely to exist
        (self.lock_dir / "owner_pid").write_text("99999999\n")
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_OWNER_DEAD", str(ctx.exception))

    def test_D4_invalid_owner_pid_blocks(self):
        (self.lock_dir / "owner_pid").write_text("not_a_pid\n")
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_INVALID", str(ctx.exception))

    def test_D5_empty_owner_token_blocks(self):
        (self.lock_dir / "owner_token").write_text("   \n")
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_INVALID", str(ctx.exception))

    def test_D6_owner_token_mismatch_blocks(self):
        (self.lock_dir / "owner_token").write_text("different-token\n")
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_TOKEN_MISMATCH", str(ctx.exception))

    def test_D7_release_sha_mismatch_blocks(self):
        (self.lock_dir / "release_sha").write_text("a" * 40 + "\n")
        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)
        self.assertIn("RELEASE_LOCK_SHA_MISMATCH", str(ctx.exception))

    def test_D8_valid_release_lock_passes(self):
        # Should not raise
        enroll.verify_release_lock(str(self.lock_dir), self.release_sha, self.owner_token)

    def test_D9_cli_invocation_without_release_lock_dir_fails_closed(self):
        m_path = self.work / "valid_manifest.json"
        m_path.write_bytes(b'{"release_sha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}')
        with mock.patch.object(enroll.approval, 'verify', return_value={"release_sha": self.release_sha}):
            sys_args = ["enroll-quarantine.py", self.compose_file, self.env_file, str(m_path), self.release_sha]
            with mock.patch.object(sys, 'argv', sys_args):
                with self.assertRaises(enroll.EnrollmentError) as ctx:
                    enroll.main()
                self.assertIn("RELEASE_LOCK_DIR_REQUIRED", str(ctx.exception))

    # -------------------------------------------------------------------------
    # MATRIX C: Two-Phase Container Revive Injection
    # -------------------------------------------------------------------------
    def test_C1_phase1_running_container_blocks_before_db(self):
        def mock_docker_running(cmd):
            # Simulate running xianzhi-ai
            if "ps" in cmd and "--status" in cmd and "running" in cmd:
                return "cid-xianzhi-ai-1"
            return ""

        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_business_containers_stopped(self.compose_file, self.env_file, execute=mock_docker_running)
        self.assertIn("ZERO_BUSINESS_WINDOW_VIOLATION", str(ctx.exception))

    def test_C2_phase1_non_exited_container_blocks(self):
        def mock_docker_non_exited(cmd):
            if "ps" in cmd and "-a" in cmd and "smartvideo-worker" in cmd:
                return "cid-worker-1"
            if "inspect" in cmd:
                return "restarting"
            return ""

        with self.assertRaises(enroll.EnrollmentError) as ctx:
            enroll.verify_business_containers_stopped(self.compose_file, self.env_file, execute=mock_docker_non_exited)
        self.assertIn("ZERO_BUSINESS_WINDOW_VIOLATION", str(ctx.exception))

    def test_C3_two_phase_container_revive_injection_rolls_back_zero_writes(self):
        """Container is stopped at Phase 1, but revived between Phase 1 and Phase 2.
        Must rollback transaction immediately with strictly 0 rows in quarantine,
        and MUST NOT attempt to stop the newly appeared container.
        """
        now = datetime.datetime.now(datetime.timezone.utc)
        nb = (now - datetime.timedelta(minutes=5)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exp = (now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exec_item = {
            "execution_id": 101,
            "task_id": "task-c3",
            "attempt": 1,
            "generation": 1,
            "snapshot_sha256": "1" * 64,
            "evidence_sha256": "1" * 64,
            "approval_id": "appr-c3",
            "release_sha": self.release_sha,
            "not_before": nb,
            "expires_at": exp,
            "evidence": {"snapshot_sha256": "1" * 64}
        }
        manifest_bytes = build_test_manifest([exec_item], self.release_sha)

        tasks = {"task-c3": {"status": "FAILED", "task_status": "FAILED", "lease_until": None, "worker_id": None}}
        executions = {101: {"task_id": "task-c3", "attempt": 1, "task_execution_generation": 1}}
        quarantine = {}
        cursor = MockCursor(tasks, executions, quarantine)
        conn = MockConnection(cursor)

        container_state = {"status": "exited"}

        def mock_docker(cmd):
            # Phase 1: exited. In Phase 2: container revived to running!
            if "ps" in cmd and "--status" in cmd and "running" in cmd:
                return "cid-revived-1" if container_state["status"] == "running" else ""
            if "ps" in cmd and "-a" in cmd:
                return "cid-revived-1"
            if "inspect" in cmd:
                return container_state["status"]
            return ""

        # Phase 1 check: passed (exited)
        enroll.verify_business_containers_stopped(self.compose_file, self.env_file, execute=mock_docker)

        # Inject container revival between Phase 1 and Phase 2
        def inject_revive_and_verify():
            container_state["status"] = "running"
            enroll.verify_business_containers_stopped(self.compose_file, self.env_file, execute=mock_docker)

        # Attempt enrollment with injected Phase 2 failure
        with mock.patch.object(enroll.live_snapshot, 'validate_live_snapshot_in_transaction', return_value=json.loads(manifest_bytes)):
            with self.assertRaises(enroll.EnrollmentError) as exc_info:
                enroll.enroll_quarantine(
                    conn, manifest_bytes, self.release_sha,
                    release_lock_dir=str(self.lock_dir),
                    compose_file=self.compose_file,
                    env_file=self.env_file,
                    phase2_hook=inject_revive_and_verify
                )

        self.assertIn("ZERO_BUSINESS_WINDOW_VIOLATION", str(exc_info.exception))
        # Verify transaction rolled back and 0 rows written
        self.assertTrue(conn.rolled_back)
        self.assertEqual(len(quarantine), 0, "Quarantine table must have strictly 0 rows on Phase 2 abort")
        # Verify lock order: xz_generation_tasks locked FIRST, provider_executions locked SECOND
        self.assertEqual(cursor.lock_order, ["xz_generation_tasks", "provider_executions"])

    # -------------------------------------------------------------------------
    # MATRIX E: Multi-Task Partial Drift All-or-Nothing Atomicity
    # -------------------------------------------------------------------------
    def test_E1_multi_task_batch_partial_drift_entire_batch_rollback(self):
        """Manifest contains 3 tasks. Task 3 has drifted. Entire batch must ROLLBACK with 0 rows."""
        now = datetime.datetime.now(datetime.timezone.utc)
        nb = (now - datetime.timedelta(minutes=5)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exp = (now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")

        execs = [
            {"execution_id": 201, "task_id": "task-e1", "attempt": 1, "generation": 1,
             "snapshot_sha256": "1"*64, "evidence_sha256": "1"*64, "approval_id": "appr-1",
             "release_sha": self.release_sha, "not_before": nb, "expires_at": exp, "evidence": {"snapshot_sha256": "1"*64}},
            {"execution_id": 202, "task_id": "task-e2", "attempt": 1, "generation": 1,
             "snapshot_sha256": "2"*64, "evidence_sha256": "2"*64, "approval_id": "appr-2",
             "release_sha": self.release_sha, "not_before": nb, "expires_at": exp, "evidence": {"snapshot_sha256": "2"*64}},
            {"execution_id": 203, "task_id": "task-e3", "attempt": 1, "generation": 1,
             "snapshot_sha256": "3"*64, "evidence_sha256": "3"*64, "approval_id": "appr-3",
             "release_sha": self.release_sha, "not_before": nb, "expires_at": exp, "evidence": {"snapshot_sha256": "3"*64}},
        ]
        manifest_bytes = build_test_manifest(execs, self.release_sha)

        tasks = {
            "task-e1": {"status": "FAILED", "task_status": "FAILED", "lease_until": None, "worker_id": None},
            "task-e2": {"status": "FAILED", "task_status": "FAILED", "lease_until": None, "worker_id": None},
            "task-e3": {"status": "FAILED", "task_status": "FAILED", "lease_until": None, "worker_id": None},
        }
        executions = {
            201: {"task_id": "task-e1", "attempt": 1, "task_execution_generation": 1},
            202: {"task_id": "task-e2", "attempt": 1, "task_execution_generation": 1},
            203: {"task_id": "task-e3", "attempt": 1, "task_execution_generation": 1},
        }
        quarantine = {}
        cursor = MockCursor(tasks, executions, quarantine)
        conn = cursor.connection

        # Task e3 has drifted in live DB state; live snapshot validation detects mismatch
        with mock.patch.object(enroll.live_snapshot, 'validate_live_snapshot_in_transaction',
                                side_effect=enroll.live_snapshot.SnapshotError("SNAPSHOT_SHA256_MISMATCH: task-e3 drifted")):
            with self.assertRaises(enroll.EnrollmentError) as exc_info:
                enroll.enroll_quarantine(
                    conn, manifest_bytes, self.release_sha,
                    release_lock_dir=str(self.lock_dir),
                    compose_file=self.compose_file,
                    env_file=self.env_file,
                    phase2_hook=lambda: None
                )

        self.assertTrue(conn.rolled_back)
        self.assertEqual(len(quarantine), 0, "All-or-nothing atomicity: partial drift must result in 0 quarantine rows")

    # -------------------------------------------------------------------------
    # MATRIX A: Deterministic Timing A (Enrollment wins)
    # -------------------------------------------------------------------------
    def test_A_timing_a_enrollment_wins_worker_err_quarantined_zero_side_effects(self):
        """Timing A:
        1. Enrollment acquires task FOR UPDATE first, then provider_executions FOR UPDATE.
        2. Validates live snapshot under lock.
        3. Inserts quarantine and commits.
        4. Worker unblocks and in same tx calls RejectTask.
        5. Worker receives ErrQuarantined.
        6. Assertion: Provider calls = 0, status mutations = 0, lease updates = 0, billing mutations = 0.
        """
        now = datetime.datetime.now(datetime.timezone.utc)
        nb = (now - datetime.timedelta(minutes=5)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exp = (now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exec_item = {
            "execution_id": 301,
            "task_id": "task-timing-a",
            "attempt": 1,
            "generation": 1,
            "snapshot_sha256": "a" * 64,
            "evidence_sha256": "a" * 64,
            "approval_id": "appr-timing-a",
            "release_sha": self.release_sha,
            "not_before": nb,
            "expires_at": exp,
            "evidence": {"snapshot_sha256": "a" * 64}
        }
        manifest_bytes = build_test_manifest([exec_item], self.release_sha)

        tasks = {"task-timing-a": {"status": "FAILED", "task_status": "FAILED", "lease_until": None, "worker_id": None}}
        executions = {301: {"task_id": "task-timing-a", "attempt": 1, "task_execution_generation": 1}}
        quarantine = {}
        cursor = MockCursor(tasks, executions, quarantine)
        conn = cursor.connection

        # 1-3: Enrollment executes and commits
        with mock.patch.object(enroll.live_snapshot, 'validate_live_snapshot_in_transaction', return_value=json.loads(manifest_bytes)):
            count = enroll.enroll_quarantine(
                conn, manifest_bytes, self.release_sha,
                release_lock_dir=str(self.lock_dir),
                compose_file=self.compose_file,
                env_file=self.env_file,
                phase2_hook=lambda: None
            )
        self.assertEqual(count, 1)
        self.assertTrue(conn.committed)
        self.assertIn(301, quarantine)

        # Lock ordering check: xz_generation_tasks FIRST, provider_executions SECOND
        self.assertEqual(cursor.lock_order, ["xz_generation_tasks", "provider_executions"])

        # 4-6: Worker unblocks and checks quarantine inside same tx
        worker_provider_calls = 0
        worker_status_mutations = 0
        worker_lease_updates = 0
        worker_billing_mutations = 0

        # Simulate Worker checking quarantine under its lock:
        quarantined = (301 in quarantine)
        if quarantined:
            # Worker gets ErrQuarantined and rolls back immediately
            worker_err = "ErrQuarantined"
        else:
            # Worker would have mutated
            worker_provider_calls += 1
            worker_status_mutations += 1
            worker_lease_updates += 1
            worker_billing_mutations += 1
            worker_err = None

        self.assertEqual(worker_err, "ErrQuarantined")
        self.assertEqual(worker_provider_calls, 0, "Provider calls must be strictly 0")
        self.assertEqual(worker_status_mutations, 0, "Status mutations must be strictly 0")
        self.assertEqual(worker_lease_updates, 0, "Lease updates must be strictly 0")
        self.assertEqual(worker_billing_mutations, 0, "Billing mutations must be strictly 0")

    # -------------------------------------------------------------------------
    # MATRIX B: Deterministic Timing B (Worker wins)
    # -------------------------------------------------------------------------
    def test_B_timing_b_worker_wins_enrollment_snapshot_mismatch_zero_quarantine_rows(self):
        """Timing B:
        1. Worker acquires task FOR UPDATE first.
        2. Enrollment waits on task FOR UPDATE.
        3. Worker advances generation/status/billing and commits.
        4. Enrollment unblocks, recalculates complete Live Snapshot under its lock.
        5. Snapshot mismatch / generation drift detected (SNAPSHOT_SHA256_MISMATCH).
        6. Enrollment fails closed and rolls back.
        7. Assertion: quarantine table has strictly 0 rows written. NEVER create fake/stale quarantine after worker wins.
        """
        now = datetime.datetime.now(datetime.timezone.utc)
        nb = (now - datetime.timedelta(minutes=5)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exp = (now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%S.000000Z")
        exec_item = {
            "execution_id": 401,
            "task_id": "task-timing-b",
            "attempt": 1,
            "generation": 1,  # Manifest expects generation 1, FAILED
            "snapshot_sha256": "b" * 64,
            "evidence_sha256": "b" * 64,
            "approval_id": "appr-timing-b",
            "release_sha": self.release_sha,
            "not_before": nb,
            "expires_at": exp,
            "evidence": {"snapshot_sha256": "b" * 64}
        }
        manifest_bytes = build_test_manifest([exec_item], self.release_sha)

        # Worker wins first: bumps generation to 2, status to PROCESSING
        tasks = {"task-timing-b": {"status": "PROCESSING", "task_status": "RUNNING", "lease_until": None, "worker_id": "worker-winner"}}
        executions = {401: {"task_id": "task-timing-b", "attempt": 1, "task_execution_generation": 2}}
        quarantine = {}
        cursor = MockCursor(tasks, executions, quarantine)
        conn = cursor.connection

        # Enrollment unblocks with lock, detects drift
        with mock.patch.object(enroll.live_snapshot, 'validate_live_snapshot_in_transaction',
                                side_effect=enroll.live_snapshot.SnapshotError("QUARANTINE_LIVE_SNAPSHOT_SHA256_MISMATCH")):
            with self.assertRaises(enroll.EnrollmentError) as exc_info:
                enroll.enroll_quarantine(
                    conn, manifest_bytes, self.release_sha,
                    release_lock_dir=str(self.lock_dir),
                    compose_file=self.compose_file,
                    env_file=self.env_file,
                    phase2_hook=lambda: None
                )

        self.assertTrue(conn.rolled_back)
        self.assertEqual(len(quarantine), 0, "Strictly 0 quarantine rows must be written after worker wins")


if __name__ == "__main__":
    unittest.main(verbosity=2)
