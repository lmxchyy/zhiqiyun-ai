#!/usr/bin/env python3
"""Read-only drain observations; supports approved quarantine manifest exemptions.

Compatible with Python 3.6. See docs/architecture/issue199-quarantine-drain.md.
"""
import datetime
import hashlib
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
# Source-only loading: approval helper is protected by the same Prestage proof.
_approval_path = os.path.join(os.path.dirname(__file__), 'quarantine-approval.py')
approval = types.ModuleType('quarantine_approval')
approval.__file__ = _approval_path
with open(_approval_path, 'rb') as _source:
    exec(compile(_source.read(), _approval_path, 'exec'), approval.__dict__)
# Complete canonical projection API; live approval still fails closed until the
# Carrier-bound authority registry is provisioned with the human approver key.
_snapshot_path = os.path.join(os.path.dirname(__file__), 'quarantine-live-snapshot.py')
live_snapshot = types.ModuleType('quarantine_live_snapshot')
live_snapshot.__file__ = _snapshot_path
with open(_snapshot_path, 'rb') as _source:
    exec(compile(_source.read(), _snapshot_path, 'exec'), live_snapshot.__dict__)
_runtime_path = os.path.join(os.path.dirname(__file__), 'verify-release-runtime.py')
runtime = types.ModuleType('release_runtime')
runtime.__file__ = _runtime_path
with open(_runtime_path, 'rb') as _source:
    exec(compile(_source.read(), _runtime_path, 'exec'), runtime.__dict__)
# Shared source-only psql transport; included in the verified Prestage proof.
_transport_path = os.path.join(os.path.dirname(__file__), 'quarantine-psql-transport.py')
transport = types.ModuleType('quarantine_psql_transport')
transport.__file__ = _transport_path
with open(_transport_path, 'rb') as _source:
    exec(compile(_source.read(), _transport_path, 'exec'), transport.__dict__)

GateError = runtime.GateError

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


def database_count(cmd, execute, sql_query=None, target=None):
    if sql_query is None:
        sql_query = SQL
    if target is not None:
        conn = None
        try:
            conn = target.connect()
            cur = conn.cursor()
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            cur.execute(sql_query)
            rows = cur.fetchall()
            if len(rows) != 1 or len(rows[0]) != 1 or type(rows[0][0]) is not int or rows[0][0] < 0:
                raise GateError('invalid drain count')
            cur.execute('ROLLBACK')
            target.check()
            return rows[0][0]
        except GateError:
            raise
        except Exception:
            raise GateError('PostgreSQL drain check query execution failed')
        finally:
            if conn is not None:
                conn.close()
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


def validate_manifest(cmd, execute, manifest_path, expected_release_sha, expected_manifest_sha256=None, cursor=None, target=None):
    if not os.path.isfile(manifest_path):
        raise GateError("quarantine manifest input missing")
    try:
        with open(manifest_path, "rb") as f:
            raw_bytes = f.read(approval.MAX_BYTES + 1)
            manifest = approval.decode(raw_bytes)
    except Exception as e:
        raise GateError("malformed quarantine manifest JSON")

    if expected_manifest_sha256:
        actual_hash = hashlib.sha256(raw_bytes).hexdigest()
        if actual_hash.lower() != expected_manifest_sha256.lower():
            raise GateError(f"manifest SHA256 mismatch: expected {expected_manifest_sha256}, got {actual_hash}")

    release_sha = manifest.get("release_sha", "")
    if str(release_sha).lower() == "f9cdf44ca79272ad7cead33dfb1d35fdf155f05f":
        raise GateError("PERMANENTLY_REJECTED_CARRIER: Base commit f9cdf44ca is permanently disqualified from production enrollment.")

    if release_sha != expected_release_sha:
        raise GateError("manifest release_sha mismatch")

    # Validate manifest and recompute live snapshot on read-only sampling cursor
    if cursor is not None:
        try:
            verified = live_snapshot.validate_live_snapshot_in_transaction(
                cursor, raw_bytes, expected_release_sha, 'drain-exemption', expected_manifest_sha256
            )
            return [e['execution_id'] for e in verified.get('executions', [])]
        except live_snapshot.SnapshotError as err:
            raise GateError(str(err))
        except Exception as err:
            raise GateError(f"live snapshot validation failed: {err}")

    # Host validation stays on the existing full projector, never a count-only fallback.
    if target is None:
        raise GateError('verified Prestage database target required')
    conn = None
    try:
        conn = target.connect()
        cur = conn.cursor()
        cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
        verified = live_snapshot.validate_live_snapshot_in_transaction(
            cur, raw_bytes, expected_release_sha, 'drain-exemption', expected_manifest_sha256)
        cur.execute('ROLLBACK')
        target.check()
        return [e['execution_id'] for e in verified['executions']]
    except live_snapshot.SnapshotError as err:
        raise GateError(str(err))
    except Exception:
        raise GateError('safe drain live snapshot sampling failed')
    finally:
        if conn is not None:
            conn.close()


def observe(cmd, execute, sql_query=None, target=None):
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
    count = database_count(cmd, execute, sql_query, target)
    queues = json.loads(execute(cmd + ['exec', '-T', 'xianzhi-ai', 'python3', '-c', runtime.BROKER]))
    runtime.queues_ok(queues, 'pre')
    return count, ids


def verify(compose, env, timeout, execute=runtime.run, pause=time.sleep, clock=time.monotonic, manifest_path=None, release_sha=None, expected_manifest_sha256=None, target=None):
    timeout = float(timeout)
    if not math.isfinite(timeout) or timeout < 0 or timeout > 300:
        raise GateError('invalid drain timeout (must be 0..300 seconds)')
    cmd = ['docker', 'compose', '-f', compose, '--env-file', env]

    exempt_ids = None
    if (manifest_path is None) != (release_sha is None):
        raise GateError('manifest and release must be supplied together')
    if manifest_path is not None:
        if expected_manifest_sha256 is None:
            try:
                with open(manifest_path, 'rb') as stream:
                    expected_manifest_sha256 = hashlib.sha256(stream.read(approval.MAX_BYTES + 1)).hexdigest()
            except OSError:
                raise GateError('manifest input failed')
        exempt_ids = validate_manifest(cmd, execute, manifest_path, release_sha, expected_manifest_sha256=expected_manifest_sha256, target=target)

    sql_query = build_sql(exempt_ids)

    deadline = clock() + timeout
    initial_ids = None
    while True:
        if manifest_path is not None:
            validate_manifest(cmd, execute, manifest_path, release_sha,
                              expected_manifest_sha256=expected_manifest_sha256, target=target)
        count, ids = observe(cmd, execute, sql_query, target)
        if initial_ids is not None and ids != initial_ids:
            raise GateError('runtime identity changed during drain observation')
        initial_ids = ids
        if count == 0:
            pause(1)
            if manifest_path is not None:
                validate_manifest(cmd, execute, manifest_path, release_sha,
                                  expected_manifest_sha256=expected_manifest_sha256, target=target)
            final_count, final_ids = observe(cmd, execute, sql_query, target)
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
        expected_manifest_sha256 = None
        args = sys.argv[1:]
        proof_path = None
        if len(args) >= 2 and args[-2] == '--prestage-proof':
            proof_path = args[-1]
            args = args[:-2]
        if len(args) == 3:
            compose, env, timeout = args
        elif len(args) >= 7 and args[3] == '--manifest' and args[5] == '--release-sha':
            compose, env, timeout = args[0], args[1], args[2]
            manifest_path = args[4]
            release_sha = args[6]
            if len(args) == 9 and args[7] == '--expected-manifest-sha256':
                expected_manifest_sha256 = args[8]
            elif len(args) != 7:
                raise GateError('usage: verify-safe-drain.py compose env timeout [--manifest <path> --release-sha <sha> [--expected-manifest-sha256 <sha>]]')
        else:
            raise GateError('usage: verify-safe-drain.py compose env timeout [--manifest <path> --release-sha <sha> [--expected-manifest-sha256 <sha>]]')
        if manifest_path and expected_manifest_sha256:
            with open(manifest_path, 'rb') as stream:
                actual_hash = hashlib.sha256(stream.read(approval.MAX_BYTES + 1)).hexdigest()
            if actual_hash != expected_manifest_sha256:
                raise GateError('manifest SHA256 mismatch')
        if str(release_sha).lower() == 'f9cdf44ca79272ad7cead33dfb1d35fdf155f05f':
            raise GateError('PERMANENTLY_REJECTED_CARRIER: Base commit f9cdf44ca is permanently disqualified from production enrollment.')
        if proof_path is None:
            raise GateError('verified --prestage-proof is required')
        with open(proof_path, 'r', encoding='utf-8') as stream:
            proof_release = json.load(stream)['git_sha']
        target = transport.Target(compose, env, proof_path, release_sha or proof_release)
        verify(compose, env, timeout, manifest_path=manifest_path, release_sha=release_sha, expected_manifest_sha256=expected_manifest_sha256, target=target)
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
