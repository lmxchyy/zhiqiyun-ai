#!/usr/bin/env python3
"""Enrolls and verifies approved quarantine records during the zero-activity window.

Runs strictly between migration container exit-0 and new service startup.
Services are dead; no concurrency is possible.
If enrollment or verification fails, exits non-zero fail-closed so deploy.sh
aborts BEFORE starting any containers.
"""
import datetime
import json
import os
import re
import shlex
import sys
import time

sys.dont_write_bytecode = True

def fail(msg):
    sys.stderr.write(f"[deploy] ERROR: QUARANTINE_ENROLLMENT_FAILED: {msg}\n")
    sys.exit(1)

def run_cmd(args):
    import subprocess
    try:
        return subprocess.check_output(args, stderr=subprocess.PIPE).decode("utf-8").strip()
    except subprocess.CalledProcessError as e:
        fail(f"Command failed ({' '.join(args[:4])}): {e.stderr.decode('utf-8', 'replace').strip()}")
    except Exception as e:
        fail(f"Execution error: {e}")

def main():
    if len(sys.argv) != 5:
        fail("Usage: enroll-quarantine.py <compose_file> <env_file> <manifest_path> <expected_release_sha>")

    compose_file, env_file, manifest_path, expected_release_sha = sys.argv[1:5]

    if not os.path.isfile(manifest_path):
        fail(f"Quarantine manifest file not found: {manifest_path}")

    with open(manifest_path, "r", encoding="utf-8") as f:
        try:
            manifest = json.load(f)
        except Exception as e:
            fail(f"Malformed manifest JSON: {e}")

    release_sha = manifest.get("release_sha", "")
    if release_sha != expected_release_sha:
        fail(f"Manifest release_sha mismatch: expected {expected_release_sha}, got {release_sha}")

    executions = manifest.get("executions", [])
    if not isinstance(executions, list) or len(executions) == 0:
        fail("Manifest contains no executions")

    # Validate timing window
    now = datetime.datetime.now(datetime.timezone.utc)
    for idx, item in enumerate(executions):
        nb_str = item.get("not_before", "")
        exp_str = item.get("expires_at", "")
        try:
            nb = datetime.datetime.fromisoformat(nb_str.replace("Z", "+00:00"))
            exp = datetime.datetime.fromisoformat(exp_str.replace("Z", "+00:00"))
        except Exception as e:
            fail(f"Invalid timestamp in execution item {idx}: {e}")
        if not (nb <= now < exp):
            fail(f"Execution item {idx} ({item.get('execution_id')}) is outside valid window ({nb_str} .. {exp_str})")

    cmd = ["docker", "compose", "-f", compose_file, "--env-file", env_file]

    # Verify table and trigger exist
    check_ddl_sql = """
    SELECT (
      SELECT to_regclass('public.provider_execution_quarantine') IS NOT NULL
    ) AND (
      SELECT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'trg_provider_execution_quarantine_immutable'
      )
    );
    """
    ddl_ok = run_cmd(cmd + [
        "exec", "-T", "postgres", "sh", "-c",
        f'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -t -A -c "{check_ddl_sql}"'
    ])
    if ddl_ok.strip().lower() != "t":
        fail("provider_execution_quarantine table or immutable trigger is missing after migration")

    # Enroll in a single atomic transaction
    statements = ["BEGIN;"]
    for item in executions:
        eid = int(item["execution_id"])
        tid = str(item["task_id"]).strip()
        attempt = int(item["attempt"])
        gen = "NULL" if item.get("generation") is None else str(int(item["generation"]))
        snap = str(item["snapshot_sha256"]).strip()
        evid = str(item["evidence_sha256"]).strip()
        appr = str(item["approval_id"]).strip()
        rel = str(item["release_sha"]).strip()
        nb_s = str(item["not_before"]).strip()
        exp_s = str(item["expires_at"]).strip()

        stmt = f"""
        INSERT INTO provider_execution_quarantine (
            execution_id, task_id, attempt, generation,
            snapshot_sha256, evidence_sha256, approval_id, release_sha,
            not_before, expires_at
        ) VALUES (
            {eid}, {shlex.quote(tid)}, {attempt}, {gen},
            {shlex.quote(snap)}, {shlex.quote(evid)}, {shlex.quote(appr)}, {shlex.quote(rel)},
            {shlex.quote(nb_s)}::timestamptz, {shlex.quote(exp_s)}::timestamptz
        );
        """
        statements.append(stmt)
    statements.append("COMMIT;")

    full_sql = "\n".join(statements)
    run_cmd(cmd + [
        "exec", "-T", "postgres", "sh", "-c",
        f'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "{full_sql}"'
    ])

    # Read back and verify each record
    verify_sql = """
    SELECT json_agg(json_build_object(
        'execution_id', execution_id,
        'task_id', task_id,
        'attempt', attempt,
        'generation', generation,
        'snapshot_sha256', snapshot_sha256,
        'evidence_sha256', evidence_sha256,
        'approval_id', approval_id,
        'release_sha', release_sha
    ) ORDER BY execution_id) FROM provider_execution_quarantine;
    """
    raw_db = run_cmd(cmd + [
        "exec", "-T", "postgres", "sh", "-c",
        f'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -t -A -c "{verify_sql}"'
    ])
    try:
        db_rows = json.loads(raw_db)
    except Exception as e:
        fail(f"Failed to read back quarantine records: {e}")

    if not isinstance(db_rows, list) or len(db_rows) != len(executions):
        fail(f"Quarantine row count mismatch: manifest has {len(executions)}, DB has {len(db_rows) if isinstance(db_rows, list) else 0}")

    db_by_id = {r["execution_id"]: r for r in db_rows}
    for item in executions:
        eid = item["execution_id"]
        if eid not in db_by_id:
            fail(f"Enrolled execution {eid} missing from DB readback")
        db_item = db_by_id[eid]
        for field in ("task_id", "attempt", "generation", "snapshot_sha256", "evidence_sha256", "approval_id", "release_sha"):
            expected_val = str(item.get(field)) if item.get(field) is not None else None
            actual_val = str(db_item.get(field)) if db_item.get(field) is not None else None
            if expected_val != actual_val:
                fail(f"Field mismatch for execution {eid} field {field}: expected {expected_val}, got {actual_val}")

    print(f"[deploy] Quarantine enrollment verified for {len(executions)} executions (Release SHA: {release_sha}).")
    return 0

if __name__ == "__main__":
    main()
