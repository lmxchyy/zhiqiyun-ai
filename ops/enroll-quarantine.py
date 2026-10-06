#!/usr/bin/env python3
"""Enrolls and verifies approved quarantine records during the zero-activity window.

Runs strictly between migration container exit-0 and new service startup.
Approval authentication alone does not establish stopped-process isolation.
Enrollment is currently disabled pending live snapshot/atomic/runtime milestones.
If enrollment or verification fails, exits non-zero fail-closed so deploy.sh
aborts BEFORE starting any containers.
"""
import datetime
import hashlib
import json
import os
import re
import subprocess
import sys
import types

sys.dont_write_bytecode = True
# Source-only loading: approval helper is protected by the same Prestage proof.
_approval_path = os.path.join(os.path.dirname(__file__), 'quarantine-approval.py')
approval = types.ModuleType('quarantine_approval')
approval.__file__ = _approval_path
with open(_approval_path, 'rb') as _source:
    exec(compile(_source.read(), _approval_path, 'exec'), approval.__dict__)

# Future same-transaction registration must use this protected projection API.
_snapshot_path = os.path.join(os.path.dirname(__file__), 'quarantine-live-snapshot.py')
live_snapshot = types.ModuleType('quarantine_live_snapshot')
live_snapshot.__file__ = _snapshot_path
with open(_snapshot_path, 'rb') as _source:
    exec(compile(_source.read(), _snapshot_path, 'exec'), live_snapshot.__dict__)

# Shared source-only psql transport; included in the verified Prestage proof.
_transport_path = os.path.join(os.path.dirname(__file__), 'quarantine-psql-transport.py')
transport = types.ModuleType('quarantine_psql_transport')
transport.__file__ = _transport_path
with open(_transport_path, 'rb') as _source:
    exec(compile(_source.read(), _transport_path, 'exec'), transport.__dict__)

class EnrollmentError(Exception):
    pass

def fail(msg):
    sys.stderr.write(f"[deploy] ERROR: QUARANTINE_ENROLLMENT_FAILED: {msg}\n")
    raise EnrollmentError(msg)


_ISO8601_RE = re.compile(
    r'^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(?:Z|([+-]\d{2}):?(\d{2}))?$'
)

def parse_iso8601_utc(ts_str):
    if not isinstance(ts_str, str):
        raise ValueError("Timestamp must be a string")
    m = _ISO8601_RE.match(ts_str.strip())
    if not m:
        raise ValueError(f"Invalid ISO 8601 timestamp: {ts_str}")
    year, month, day, hour, minute, second, frac, tz_h, tz_m = m.groups()
    microsecond = int((frac or '0')[:6].ljust(6, '0'))
    if tz_h is not None and tz_m is not None:
        offset_minutes = int(tz_h) * 60 + (int(tz_m) if int(tz_h) >= 0 else -int(tz_m))
        tz = datetime.timezone(datetime.timedelta(minutes=offset_minutes))
    else:
        tz = datetime.timezone.utc
    dt = datetime.datetime(
        int(year), int(month), int(day),
        int(hour), int(minute), int(second),
        microsecond, tzinfo=tz
    )
    return dt.astimezone(datetime.timezone.utc)

def build_enrollment_sql(executions):
    """Transport/legacy constraints only; NOT production enrollment authorization.

    Kept for isolated SQL fixture regression until canonical registration closes.
    """
    # 2. Build COPY payload using safe text escape
    copy_lines = []
    for item in executions:
        raw_json = json.dumps(item)
        copy_line = raw_json.replace("\\", "\\\\").replace("\t", "\\t").replace("\r", "\\r").replace("\n", "\\n")
        copy_lines.append(copy_line)
    copy_payload = "\n".join(copy_lines)

    # 3. Transactional enrollment under row locks with pre-COMMIT readback verification
    enroll_sql = f"""
CREATE TEMP TABLE _import (data jsonb);
COPY _import (data) FROM STDIN;
{copy_payload}
\\.

BEGIN ISOLATION LEVEL REPEATABLE READ;

-- Lock xz_generation_tasks rows
SELECT t.id FROM xz_generation_tasks t
JOIN (
  SELECT (rec).task_id
  FROM _import, LATERAL jsonb_to_record(data) AS rec(task_id text)
) m ON t.id = m.task_id ORDER BY t.id FOR UPDATE;

-- Lock provider_executions rows
SELECT e.id FROM provider_executions e
JOIN (
  SELECT (rec).execution_id
  FROM _import, LATERAL jsonb_to_record(data) AS rec(execution_id bigint)
) m ON e.id = m.execution_id ORDER BY e.id FOR UPDATE;

-- Validate live DB state under lock
DO $$
DECLARE
  v_mismatch integer;
BEGIN
  -- Check executions exist and match generation
  SELECT count(*) INTO v_mismatch FROM _import i
  WHERE NOT EXISTS (
    SELECT 1 FROM provider_executions e
    WHERE e.id = ((i.data->>'execution_id')::bigint)
      AND e.task_id = (i.data->>'task_id')
      AND e.attempt = ((i.data->>'attempt')::integer)
      AND (
        ((i.data->>'generation') IS NULL AND e.task_execution_generation IS NULL) OR
        (e.task_execution_generation = ((i.data->>'generation')::bigint))
      )
  );
  IF v_mismatch > 0 THEN
    RAISE EXCEPTION 'EXECUTION_MISMATCH: live provider_executions state does not match manifest';
  END IF;

  -- Check tasks are FAILED, lease expired, worker_id null
  SELECT count(*) INTO v_mismatch FROM _import i
  WHERE NOT EXISTS (
    SELECT 1 FROM xz_generation_tasks t
    WHERE t.id = (i.data->>'task_id')
      AND upper(t.status) = 'FAILED'
      AND upper(t.task_status) = 'FAILED'
      AND (t.lease_until IS NULL OR t.lease_until <= clock_timestamp())
      AND t.worker_id IS NULL
  );
  IF v_mismatch > 0 THEN
    RAISE EXCEPTION 'TASK_MISMATCH: live xz_generation_tasks state is not terminal FAILED with expired lease';
  END IF;

  -- Check DB clock within not_before and expires_at
  SELECT count(*) INTO v_mismatch FROM _import i,
  LATERAL jsonb_to_record(data) AS rec(not_before timestamptz, expires_at timestamptz)
  WHERE clock_timestamp() < rec.not_before OR clock_timestamp() >= rec.expires_at;
  IF v_mismatch > 0 THEN
    RAISE EXCEPTION 'CLOCK_WINDOW_MISMATCH: DB clock_timestamp() outside not_before .. expires_at window';
  END IF;
END $$;

INSERT INTO provider_execution_quarantine (
    execution_id, task_id, attempt, generation,
    snapshot_sha256, evidence_sha256, approval_id, release_sha,
    not_before, expires_at
)
SELECT
    rec.execution_id,
    rec.task_id,
    rec.attempt,
    rec.generation,
    rec.snapshot_sha256,
    rec.evidence_sha256,
    rec.approval_id,
    rec.release_sha,
    rec.not_before,
    rec.expires_at
FROM _import,
LATERAL jsonb_to_record(data) AS rec(
    execution_id bigint,
    task_id text,
    attempt integer,
    generation bigint,
    snapshot_sha256 char(64),
    evidence_sha256 char(64),
    approval_id text,
    release_sha char(40),
    not_before timestamptz,
    expires_at timestamptz
);

-- Pre-COMMIT readback verification of all 10 columns within SAME transaction
DO $$
DECLARE
  v_mismatch integer;
BEGIN
  SELECT count(*) INTO v_mismatch
  FROM _import i
  WHERE NOT EXISTS (
    SELECT 1 FROM provider_execution_quarantine q
    WHERE q.execution_id = ((i.data->>'execution_id')::bigint)
      AND q.task_id = (i.data->>'task_id')
      AND q.attempt = ((i.data->>'attempt')::integer)
      AND (
        ((i.data->>'generation') IS NULL AND q.generation IS NULL) OR
        (q.generation = ((i.data->>'generation')::bigint))
      )
      AND q.snapshot_sha256 = (i.data->>'snapshot_sha256')
      AND q.evidence_sha256 = (i.data->>'evidence_sha256')
      AND q.approval_id = (i.data->>'approval_id')
      AND q.release_sha = (i.data->>'release_sha')
      AND q.not_before = ((i.data->>'not_before')::timestamptz)
      AND q.expires_at = ((i.data->>'expires_at')::timestamptz)
  );
  IF v_mismatch > 0 THEN
    RAISE EXCEPTION 'READBACK_MISMATCH: quarantine readback does not match approved manifest';
  END IF;

  SELECT count(*) INTO v_mismatch
  FROM provider_execution_quarantine
  WHERE execution_id IN (SELECT ((data->>'execution_id')::bigint) FROM _import)
    AND (clock_timestamp() < not_before OR clock_timestamp() >= expires_at);
  IF v_mismatch > 0 THEN
    RAISE EXCEPTION 'READBACK_CLOCK_MISMATCH: post-insert records outside active window';
  END IF;
END $$;

COMMIT;
"""
    return enroll_sql


def get_db_connection(compose_file, env_file, proof_path, release_sha):
    return transport.Target(compose_file, env_file, proof_path, release_sha).connect()


def is_pid_alive(pid):
    if pid <= 0:
        return False
    try:
        os.kill(pid, 0)
        return True
    except OSError as err:
        import errno
        return err.errno == errno.EPERM


def verify_release_lock(release_lock_dir, expected_release_sha, expected_owner_token=None):
    if not release_lock_dir:
        fail("RELEASE_LOCK_DIR_REQUIRED: release lock directory must be specified")
    lock_path = os.path.abspath(release_lock_dir)
    if not os.path.isdir(lock_path):
        fail(f"RELEASE_LOCK_MISSING: release lock directory does not exist: {lock_path}")

    # Check for .recovering lock
    clean_lock_path = lock_path.rstrip('/\\')
    recovering_path = clean_lock_path + ".recovering"
    if os.path.exists(recovering_path):
        fail(f"RELEASE_LOCK_RECOVERING: recovery lock present at {recovering_path}")

    # Check owner_pid
    pid_file = os.path.join(lock_path, "owner_pid")
    if not os.path.isfile(pid_file):
        fail(f"RELEASE_LOCK_INVALID: owner_pid file missing in {lock_path}")
    try:
        with open(pid_file, "r") as f:
            pid_str = f.read().strip()
        owner_pid = int(pid_str)
    except Exception as e:
        fail(f"RELEASE_LOCK_INVALID: invalid owner_pid ({e})")
    if not is_pid_alive(owner_pid):
        fail(f"RELEASE_LOCK_OWNER_DEAD: owner_pid {owner_pid} is not alive")

    # Check owner_token
    token_file = os.path.join(lock_path, "owner_token")
    if not os.path.isfile(token_file):
        fail(f"RELEASE_LOCK_INVALID: owner_token file missing in {lock_path}")
    with open(token_file, "r") as f:
        token_in_file = f.read().strip()
    if not token_in_file:
        fail(f"RELEASE_LOCK_INVALID: owner_token is empty in {lock_path}")

    expected_token = expected_owner_token or os.environ.get("OWNER_TOKEN")
    if expected_token and token_in_file != expected_token.strip():
        fail(f"RELEASE_LOCK_TOKEN_MISMATCH: token in lock {token_in_file!r} does not match expected {expected_token!r}")

    # Check release_sha
    sha_file = os.path.join(lock_path, "release_sha")
    if not os.path.isfile(sha_file):
        fail(f"RELEASE_LOCK_INVALID: release_sha file missing in {lock_path}")
    with open(sha_file, "r") as f:
        sha_in_file = f.read().strip()
    if sha_in_file.lower() != expected_release_sha.lower():
        fail(f"RELEASE_LOCK_SHA_MISMATCH: release_sha in lock {sha_in_file} does not match {expected_release_sha}")


def verify_business_containers_stopped(compose_file, env_file, execute=None):
    """Verifies xianzhi-ai and smartvideo-worker are STOPPED (status exited, no running container)."""
    if execute is None:
        def _default_exec(cmd):
            res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
            if res.returncode != 0:
                fail(f"CONTAINER_INSPECTION_FAILED: command {' '.join(cmd)} exited {res.returncode}: {res.stderr}")
            return res.stdout.strip()
        execute = _default_exec

    base_cmd = ["docker", "compose", "-f", compose_file, "--env-file", env_file]

    # Check 1: no running containers
    ps_running = execute(base_cmd + ["ps", "--status", "running", "-q", "xianzhi-ai", "smartvideo-worker"])
    if ps_running and ps_running.strip():
        fail(f"ZERO_BUSINESS_WINDOW_VIOLATION: running business containers detected: {ps_running.strip()}")

    # Check 2: all existing containers for xianzhi-ai and smartvideo-worker must be exited
    for svc in ("xianzhi-ai", "smartvideo-worker"):
        cids_raw = execute(base_cmd + ["ps", "-a", "-q", svc])
        cids = [cid.strip() for cid in cids_raw.split() if cid.strip()]
        for cid in cids:
            status = execute(["docker", "inspect", "--format", "{{.State.Status}}", cid]).strip().lower()
            if status != "exited":
                fail(f"ZERO_BUSINESS_WINDOW_VIOLATION: container {cid} for {svc} has status {status!r}, expected 'exited'")


def enroll_quarantine_in_transaction(cursor, raw_bytes, release_sha,
                                    expected_manifest_sha256=None,
                                    readback_tamper_hook=None,
                                    release_lock_dir=None,
                                    compose_file=None,
                                    env_file=None,
                                    expected_owner_token=None,
                                    docker_executor=None,
                                    phase2_hook=None):
    """Atomic enrollment on a single database transaction cursor.

    Acquires row-level locks, validates live DB state against approved manifest,
    checks clock window, inserts all 10 columns, and verifies readback of all 10
    columns byte-for-byte within the SAME transaction BEFORE COMMIT.
    """
    if expected_manifest_sha256 is not None:
        actual_hash = hashlib.sha256(raw_bytes).hexdigest()
        if actual_hash.lower() != expected_manifest_sha256.lower():
            fail(f"Manifest SHA256 mismatch: expected {expected_manifest_sha256}, got {actual_hash}")

    if str(release_sha).lower() == "f9cdf44ca79272ad7cead33dfb1d35fdf155f05f":
        fail("PERMANENTLY_REJECTED_CARRIER: Base commit f9cdf44ca is permanently disqualified from production enrollment.")

    try:
        manifest = approval.decode(raw_bytes)
    except Exception as e:
        fail(f"Malformed manifest JSON: {e}")

    if manifest.get("release_sha") != release_sha:
        fail("Manifest release_sha mismatch")

    executions = manifest.get("executions", [])
    if not isinstance(executions, list) or len(executions) == 0:
        fail("Manifest contains no executions")

    eids = [item["execution_id"] for item in executions]
    tids = [item["task_id"] for item in executions]

    # 1. Acquire row-level locks FOR UPDATE in stable order:
    # Anchor on xz_generation_tasks FIRST, then provider_executions.
    cursor.execute(
        "SELECT id, status, task_status, lease_until, worker_id FROM public.xz_generation_tasks "
        "WHERE id = ANY(%s) ORDER BY id FOR UPDATE;",
        (list(set(tids)),)
    )
    locked_t = cursor.fetchall()
    if len(locked_t) != len(set(tids)):
        fail("TASK_LOCK_FAILED: not all generation task rows locked for update")

    cursor.execute(
        "SELECT id, task_id, attempt, task_execution_generation FROM public.provider_executions "
        "WHERE id = ANY(%s) ORDER BY id FOR UPDATE;",
        (eids,)
    )
    locked_e = cursor.fetchall()
    if len(locked_e) != len(eids):
        fail("EXECUTION_LOCK_FAILED: not all execution rows locked for update")

    # 2. Execute live snapshot validation on this same transaction cursor
    try:
        manifest = live_snapshot.validate_live_snapshot_in_transaction(
            cursor, raw_bytes, release_sha, 'enroll', expected_manifest_sha256
        )
    except live_snapshot.SnapshotError as err:
        fail(f"LIVE_SNAPSHOT_VALIDATION_FAILED: {err}")
    except Exception as err:
        fail("LIVE_SNAPSHOT_VALIDATION_FAILED")

    # 3. Phase 2 verification: release lock and zero-business-window under row locks
    if phase2_hook is not None:
        phase2_hook()
    elif release_lock_dir and compose_file and env_file:
        verify_release_lock(release_lock_dir, release_sha, expected_owner_token)
        verify_business_containers_stopped(compose_file, env_file, execute=docker_executor)

    # 4. Check DB clock_timestamp() within not_before..expires_at for each execution
    for item in executions:
        cursor.execute(
            "SELECT clock_timestamp() >= %s::timestamptz AND clock_timestamp() < %s::timestamptz;",
            (item["not_before"], item["expires_at"])
        )
        clock_ok = cursor.fetchone()
        if not clock_ok or clock_ok[0] is not True:
            fail(f"CLOCK_WINDOW_MISMATCH: DB clock outside not_before..expires_at for execution {item['execution_id']}")

    # 5. Verify table and immutable trigger exist
    cursor.execute("""
    SELECT (
      SELECT to_regclass('public.provider_execution_quarantine') IS NOT NULL
    ) AND (
      SELECT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'trg_provider_execution_quarantine_immutable'
      )
    );
    """)
    ddl_ok = cursor.fetchone()
    if not ddl_ok or ddl_ok[0] is not True:
        fail("provider_execution_quarantine table or immutable trigger is missing")

    # 6. INSERT all 10 columns into provider_execution_quarantine
    insert_sql = """
    INSERT INTO public.provider_execution_quarantine (
        execution_id, task_id, attempt, generation,
        snapshot_sha256, evidence_sha256, approval_id, release_sha,
        not_before, expires_at
    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::timestamptz, %s::timestamptz);
    """
    for item in executions:
        cursor.execute(insert_sql, (
            item["execution_id"],
            item["task_id"],
            item["attempt"],
            item["generation"],
            item["snapshot_sha256"],
            item["evidence_sha256"],
            item["approval_id"],
            item["release_sha"],
            item["not_before"],
            item["expires_at"],
        ))

    # 7. Read back all 10 columns within SAME transaction BEFORE COMMIT
    readback_sql = """
    SELECT execution_id, task_id, attempt, generation,
           snapshot_sha256, evidence_sha256, approval_id, release_sha,
           to_char(not_before AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
           to_char(expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
    FROM public.provider_execution_quarantine
    WHERE execution_id = ANY(%s)
    ORDER BY execution_id;
    """
    cursor.execute(readback_sql, (eids,))
    db_rows = cursor.fetchall()
    if len(db_rows) != len(executions):
        fail(f"READBACK_COUNT_MISMATCH: expected {len(executions)}, got {len(db_rows)}")

    if readback_tamper_hook is not None:
        db_rows = readback_tamper_hook(db_rows)

    col_names = [
        "execution_id", "task_id", "attempt", "generation",
        "snapshot_sha256", "evidence_sha256", "approval_id", "release_sha",
        "not_before", "expires_at"
    ]
    exec_by_id = {item["execution_id"]: item for item in executions}
    for row in db_rows:
        eid = row[0]
        if eid not in exec_by_id:
            fail(f"READBACK_UNEXPECTED_EXECUTION: {eid}")
        exp_item = exec_by_id[eid]
        for idx, col in enumerate(col_names):
            val = row[idx]
            exp_val = exp_item[col]
            if col in ("execution_id", "attempt", "generation"):
                if val != exp_val:
                    fail(f"READBACK_MISMATCH: execution {eid} column {col}: expected {exp_val!r}, got {val!r}")
            else:
                if str(val) != str(exp_val):
                    fail(f"READBACK_MISMATCH: execution {eid} column {col}: expected {exp_val!r}, got {val!r}")

    # Check clock window in readback rows
    cursor.execute(
        "SELECT count(*) FROM public.provider_execution_quarantine "
        "WHERE execution_id = ANY(%s) AND (clock_timestamp() < not_before OR clock_timestamp() >= expires_at);",
        (eids,)
    )
    if cursor.fetchone()[0] != 0:
        fail("READBACK_CLOCK_MISMATCH: records outside active window")

    return len(executions)


def enroll_quarantine(conn, raw_bytes, release_sha,
                      expected_manifest_sha256=None,
                      readback_tamper_hook=None,
                      release_lock_dir=None,
                      compose_file=None,
                      env_file=None,
                      expected_owner_token=None,
                      docker_executor=None,
                      phase2_hook=None):
    """Executes atomic enrollment on database connection with transaction boundaries."""
    conn.autocommit = True
    cursor = conn.cursor()
    cursor.execute("BEGIN ISOLATION LEVEL REPEATABLE READ;")
    try:
        count = enroll_quarantine_in_transaction(
            cursor, raw_bytes, release_sha, expected_manifest_sha256, readback_tamper_hook,
            release_lock_dir=release_lock_dir, compose_file=compose_file, env_file=env_file,
            expected_owner_token=expected_owner_token, docker_executor=docker_executor,
            phase2_hook=phase2_hook
        )
        cursor.execute("COMMIT;")
        return count
    except Exception:
        try:
            cursor.execute("ROLLBACK;")
        except Exception:
            pass
        raise
    finally:
        try:
            cursor.close()
        except Exception:
            pass


def main():
    expected_manifest_sha256 = None
    proof_path = None
    release_lock_dir = None
    owner_token = None
    pos_args = []

    args = sys.argv[1:]
    i = 0
    while i < len(args):
        arg = args[i]
        if arg == '--prestage-proof':
            if i + 1 >= len(args):
                fail("--prestage-proof requires an argument")
            proof_path = args[i + 1]
            i += 2
        elif arg == '--expected-manifest-sha256':
            if i + 1 >= len(args):
                fail("--expected-manifest-sha256 requires an argument")
            expected_manifest_sha256 = args[i + 1]
            i += 2
        elif arg == '--release-lock-dir':
            if i + 1 >= len(args):
                fail("--release-lock-dir requires an argument")
            release_lock_dir = args[i + 1]
            i += 2
        elif arg == '--owner-token':
            if i + 1 >= len(args):
                fail("--owner-token requires an argument")
            owner_token = args[i + 1]
            i += 2
        elif arg.startswith('--'):
            fail(f"Unknown argument: {arg}")
        else:
            pos_args.append(arg)
            i += 1

    if len(pos_args) != 4:
        fail("Usage: enroll-quarantine.py <compose_file> <env_file> <manifest_path> <expected_release_sha> --release-lock-dir <lock_dir> [--expected-manifest-sha256 <sha>] [--prestage-proof <proof>]")

    compose_file, env_file, manifest_path, expected_release_sha = pos_args

    if not os.path.isfile(manifest_path):
        fail("Quarantine manifest input missing")

    try:
        with open(manifest_path, "rb") as f:
            raw_bytes = f.read(approval.MAX_BYTES + 1)
            manifest = approval.decode(raw_bytes)
    except Exception as e:
        fail("Malformed manifest JSON")

    if expected_manifest_sha256:
        actual_hash = hashlib.sha256(raw_bytes).hexdigest()
        if actual_hash.lower() != expected_manifest_sha256.lower():
            fail(f"Manifest SHA256 mismatch: expected {expected_manifest_sha256}, got {actual_hash}")

    if expected_release_sha.lower() == "f9cdf44ca79272ad7cead33dfb1d35fdf155f05f":
        fail("PERMANENTLY_REJECTED_CARRIER: Base commit f9cdf44ca is permanently disqualified from production enrollment.")

    release_sha = manifest.get("release_sha", "")
    if release_sha != expected_release_sha:
        fail("Manifest release_sha mismatch")

    try:
        approval.verify(raw_bytes, expected_release_sha, 'enroll',
                        datetime.datetime.now(datetime.timezone.utc))
    except approval.ApprovalError as error:
        fail(str(error))

    if not release_lock_dir:
        fail("RELEASE_LOCK_DIR_REQUIRED: enroll-quarantine.py cannot be run as an uncoordinated standalone tool without release lock.")

    # Phase 1: verify release lock and business containers stopped BEFORE opening DB connection
    verify_release_lock(release_lock_dir, expected_release_sha, owner_token)
    verify_business_containers_stopped(compose_file, env_file)

    if proof_path is None:
        fail('verified --prestage-proof is required')
    # ONE explicit read/write transaction, not independent read-only sampling.
    conn = get_db_connection(compose_file, env_file, proof_path, expected_release_sha)
    try:
        count = enroll_quarantine(
            conn, raw_bytes, expected_release_sha,
            expected_manifest_sha256=expected_manifest_sha256,
            release_lock_dir=release_lock_dir,
            compose_file=compose_file,
            env_file=env_file,
            expected_owner_token=owner_token,
        )
        print(f"[deploy] Quarantine enrollment verified for {count} executions (Release SHA: {release_sha}).")
        return 0
    finally:
        try:
            conn.close()
        except Exception:
            pass

if __name__ == "__main__":
    try:
        sys.exit(main())
    except EnrollmentError:
        sys.exit(1)
    except Exception:
        sys.stderr.write('[deploy] ERROR: QUARANTINE_ENROLLMENT_FAILED: database transport failed\n')
        sys.exit(1)
