#!/usr/bin/env python3
"""Read-only drain observations; supports approved quarantine manifest exemptions.

Compatible with Python 3.6. See docs/architecture/issue199-quarantine-drain.md.
"""
import datetime
import json
import math
import os
import re
import shlex
import sys
import time
import types

# Execute proof-bound source bytes, never an ignored/unbound .pyc cache.
sys.dont_write_bytecode = True
_runtime_path = os.path.join(os.path.dirname(__file__), 'verify-release-runtime.py')
runtime = types.ModuleType('release_runtime')
runtime.__file__ = _runtime_path
with open(_runtime_path, 'rb') as _source:
    exec(compile(_source.read(), _runtime_path, 'exec'), runtime.__dict__)
GateError = runtime.GateError


def build_sql(exempt_ids=None):
    if exempt_ids:
        clean_ids = [str(int(i)) for i in exempt_ids]
        not_in_clause = f" AND e.id NOT IN ({','.join(clean_ids)})"
    else:
        not_in_clause = ""
    return f"""
SELECT
 (SELECT count(*) FROM public.xz_generation_tasks
  WHERE lease_until > now()
     OR status IS NULL OR task_status IS NULL
     OR upper(status) NOT IN ('COMPLETED','SUCCEEDED','FAILED','CANCELLED')
     OR upper(task_status) NOT IN ('SUCCEEDED','FAILED','CANCELLED')
     OR (CASE upper(status) WHEN 'COMPLETED' THEN 'SUCCEEDED' ELSE upper(status) END)
        IS DISTINCT FROM upper(task_status)
     OR id IS NULL)
 + (SELECT count(*) FROM public.provider_executions e
    WHERE (e.status IS NULL OR e.status NOT IN ('succeeded','failed'){not_in_clause})
       OR e.id IS NULL OR e.task_id IS NULL OR e.attempt IS NULL OR e.attempt < 1
       OR (SELECT count(*) FROM public.xz_generation_tasks t WHERE t.id = e.task_id) <> 1)
 + (SELECT count(*) FROM (SELECT id FROM public.xz_generation_tasks GROUP BY id HAVING count(*) > 1) d)
 + (SELECT count(*) FROM (SELECT id FROM public.provider_executions GROUP BY id HAVING count(*) > 1) d)
 + (SELECT count(*) FROM (SELECT task_id, attempt FROM public.provider_executions
                         GROUP BY task_id, attempt HAVING count(*) > 1) d)
 + (SELECT count(*) FROM public.outbox_events
    WHERE status IS NULL OR status NOT IN ('published','failed'))
 + (SELECT count(*) FROM public.video_task_outbox
    WHERE state IS NULL OR state NOT IN ('published','failed'));
"""


SQL = build_sql()


def database_count(cmd, execute, sql_query=None):
    if sql_query is None:
        sql_query = SQL
    shell = ('PGPASSWORD="$POSTGRES_PASSWORD" '
             'PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=10000" '
             'psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" '
             '-v ON_ERROR_STOP=1 -t -A -c ' + shlex.quote(sql_query))
    try:
        result = execute(cmd + ['exec', '-T', 'postgres', 'sh', '-c', shell])
        if not re.fullmatch(r'[0-9]+', result):
            raise ValueError('invalid count')
        return int(result)
    except Exception:
        raise GateError('PostgreSQL drain check query execution failed')


def validate_manifest(cmd, execute, manifest_path, expected_release_sha):
    if not os.path.isfile(manifest_path):
        raise GateError(f"quarantine manifest file not found: {manifest_path}")
    try:
        with open(manifest_path, "r", encoding="utf-8") as f:
            manifest = json.load(f)
    except Exception as e:
        raise GateError(f"malformed quarantine manifest JSON: {e}")

    release_sha = manifest.get("release_sha", "")
    if release_sha != expected_release_sha:
        raise GateError(f"manifest release_sha mismatch: expected {expected_release_sha}, got {release_sha}")

    executions = manifest.get("executions", [])
    if not isinstance(executions, list) or len(executions) == 0:
        raise GateError("manifest contains no executions")

    now = datetime.datetime.now(datetime.timezone.utc)
    exempt_ids = []
    for idx, item in enumerate(executions):
        eid = item.get("execution_id")
        if type(eid) is not int or eid <= 0:
            raise GateError(f"invalid execution_id in manifest item {idx}")
        exempt_ids.append(eid)
        nb_str = item.get("not_before", "")
        exp_str = item.get("expires_at", "")
        try:
            nb = datetime.datetime.fromisoformat(nb_str.replace("Z", "+00:00"))
            exp = datetime.datetime.fromisoformat(exp_str.replace("Z", "+00:00"))
        except Exception as e:
            raise GateError(f"invalid timestamp in manifest item {idx}: {e}")
        if not (nb <= now < exp):
            raise GateError(f"manifest item {idx} ({eid}) outside valid window ({nb_str} .. {exp_str})")

    # Verify against live DB state
    ids_str = ','.join(str(i) for i in exempt_ids)
    check_sql = f"""
    SELECT json_agg(json_build_object(
        'id', e.id,
        'task_id', e.task_id,
        'attempt', e.attempt,
        'execution_status', e.status,
        'task_status', t.status,
        'task_status_field', t.task_status,
        'lease_active', (t.lease_until IS NOT NULL AND t.lease_until > now())
    ))
    FROM provider_executions e
    JOIN xz_generation_tasks t ON t.id = e.task_id
    WHERE e.id IN ({ids_str});
    """
    shell = ('PGPASSWORD="$POSTGRES_PASSWORD" '
             'PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=10000" '
             'psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" '
             '-v ON_ERROR_STOP=1 -t -A -c ' + shlex.quote(check_sql))
    try:
        raw_res = execute(cmd + ['exec', '-T', 'postgres', 'sh', '-c', shell])
        db_rows = json.loads(raw_res)
    except Exception as e:
        raise GateError(f"database check of manifest executions failed: {e}")

    if not isinstance(db_rows, list) or len(db_rows) != len(exempt_ids):
        raise GateError(f"database execution count mismatch for manifest: expected {len(exempt_ids)}, got {len(db_rows) if isinstance(db_rows, list) else 0}")

    db_map = {r["id"]: r for r in db_rows}
    for item in executions:
        eid = item["execution_id"]
        if eid not in db_map:
            raise GateError(f"manifest execution {eid} missing from database")
        row = db_map[eid]
        if str(row["task_id"]) != str(item["task_id"]) or int(row["attempt"]) != int(item["attempt"]):
            raise GateError(f"manifest identity mismatch for execution {eid}")
        if str(row["execution_status"]) != str(item["execution_status"]):
            raise GateError(f"execution {eid} status changed: expected {item['execution_status']}, got {row['execution_status']}")
        if row["task_status"] != "FAILED" or row["task_status_field"] != "FAILED":
            raise GateError(f"task for execution {eid} is not terminal FAILED")
        if row["lease_active"]:
            raise GateError(f"task for execution {eid} has an active unexpired lease")

    return exempt_ids


def observe(cmd, execute, sql_query=None):
    if sql_query is None:
        sql_query = SQL
    ids = []
    for service in ('xianzhi-ai', 'smartvideo-worker'):
        try:
            cid = execute(cmd + ['ps', '-q', service])
        except Exception:
            raise GateError('Container inspection failed during drain check')
        if not re.fullmatch(r'[a-zA-Z0-9_-]+', cid or ''):
            raise GateError('Required old API/worker container is absent or ambiguous')
        ids.append(cid)
    count = database_count(cmd, execute, sql_query)
    queues = json.loads(execute(cmd + ['exec', '-T', 'xianzhi-ai', 'python3', '-c', runtime.BROKER]))
    runtime.queues_ok(queues, 'pre')
    return count, ids


def verify(compose, env, timeout, execute=runtime.run, pause=time.sleep, clock=time.monotonic, manifest_path=None, release_sha=None):
    timeout = float(timeout)
    if not math.isfinite(timeout) or timeout < 0 or timeout > 300:
        raise GateError('invalid drain timeout (must be 0..300 seconds)')
    cmd = ['docker', 'compose', '-f', compose, '--env-file', env]

    exempt_ids = None
    if manifest_path is not None and release_sha is not None:
        exempt_ids = validate_manifest(cmd, execute, manifest_path, release_sha)

    sql_query = build_sql(exempt_ids)

    deadline = clock() + timeout
    initial_ids = None
    while True:
        count, ids = observe(cmd, execute, sql_query)
        if initial_ids is not None and ids != initial_ids:
            raise GateError('runtime identity changed during drain observation')
        initial_ids = ids
        if count == 0:
            pause(1)
            final_count, final_ids = observe(cmd, execute, sql_query)
            if final_count or final_ids != ids:
                raise GateError('release observation changed; retry from a fresh drain')
            return
        if clock() >= deadline:
            raise GateError('active valid leases or in-flight operations in progress; safe drain blocked')
        pause(2)


def main():
    try:
        manifest_path = None
        release_sha = None
        args = sys.argv[1:]
        if len(args) == 3:
            compose, env, timeout = args
        elif len(args) == 7 and args[3] == '--manifest' and args[5] == '--release-sha':
            compose, env, timeout = args[0], args[1], args[2]
            manifest_path = args[4]
            release_sha = args[6]
        else:
            raise GateError('usage: verify-safe-drain.py compose env timeout [--manifest <path> --release-sha <sha>]')
        verify(compose, env, timeout, manifest_path=manifest_path, release_sha=release_sha)
    except Exception as error:
        detail = str(error) if isinstance(error, GateError) else 'invalid drain observation'
        print('[deploy] ERROR: SAFE_DRAIN_REJECTED: ' + detail, file=sys.stderr)
        return 1
    if manifest_path:
        print(f'[deploy] drain observations clear with approved quarantine exemptions (Release SHA: {release_sha})')
    else:
        print('[deploy] drain observations clear; no exemptions applied')
    return 0


if __name__ == '__main__':
    sys.exit(main())
