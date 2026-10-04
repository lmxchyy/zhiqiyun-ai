"""Local-only tests. Docker replay creates/removes its own network-isolated DB.

python3 tests/issue199-quarantine-drain-test.py
ISSUE199_DOCKER_REPLAY=1 python3 tests/issue199-quarantine-drain-test.py
"""
import ast
import copy
import importlib.util
import json
import os
from pathlib import Path
import py_compile
import subprocess
import sys
import tempfile
import shutil
import time
import unittest
import uuid

ROOT = Path(__file__).resolve().parent.parent
sys.dont_write_bytecode = True


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, str(ROOT / 'ops' / filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


gate = load('drain', 'verify-safe-drain.py')
model = load('model', 'quarantine-drain-model.py')


def entry(eid=1):
    e = dict(execution_id=eid, task_id=eid + 1000, attempt=1, generation=1,
             execution_status='unknown', provider_request_id=None,
             task_status='FAILED', task_status_v2='FAILED', lease_until=1,
             owner=None, approval=dict(identity='synthetic-reviewer', time=10,
                                      reference='synthetic-approval-not-real'),
             release_sha='a' * 40, not_before=10, expires_at=100)
    for key in ('financial', 'assets', 'storage', 'results', 'correlation', 'logs'):
        e[key] = dict(source='synthetic-' + key, sha256=model.digest({'synthetic': key}))
    e['snapshot_sha256'] = model.snapshot_digest(e)
    return e


def oracle(e):
    return {e['execution_id']: dict(entry_sha256=model.digest(e), recovery_isolated=True)}


def queues():
    return [dict(name='x.ai.generation.' + name, consumers=1,
                 messages_ready=0, messages_unacknowledged=0)
            for name in ('image.normal', 'image.canary', 'video.canary', 'ppt.canary')]


class FakeRuntime:
    def __init__(self, counts=('0', '0'), queue_data=None, error=None):
        self.counts = iter(counts)
        self.queues = queues() if queue_data is None else queue_data
        self.error = error
        self.calls = []

    def __call__(self, args):
        self.calls.append(args)
        if 'ps' in args:
            if self.error == 'ps':
                raise RuntimeError('SECRET sentinel')
            return 'local-' + args[-1]
        if args[-1] == gate.runtime.BROKER:
            if self.error == 'broker':
                raise RuntimeError('SECRET sentinel')
            return json.dumps(self.queues)
        if 'psql' in args[-1]:
            if self.error == 'db':
                raise RuntimeError('SECRET sentinel')
            return next(self.counts, '1')
        raise AssertionError('unexpected command')


class DrainTests(unittest.TestCase):
    def check(self, runner):
        return gate.verify('synthetic-compose', 'synthetic-env', 0, runner, lambda _: None)

    def test_clear_observations_and_read_only_commands(self):
        runner = FakeRuntime()
        self.check(runner)
        sql_calls = [c[-1] for c in runner.calls if 'psql' in c[-1]]
        self.assertEqual(len(sql_calls), 2)
        for command in sql_calls:
            self.assertIn('default_transaction_read_only=on', command)
            self.assertIn('psql -X', command)
        for forbidden in ('UPDATE ', 'DELETE ', 'INSERT ', 'ALTER ', 'CREATE '):
            self.assertNotIn(forbidden, gate.SQL.upper())
        self.assertNotIn('INNER JOIN', gate.SQL.upper())
        for command in runner.calls:
            self.assertFalse(set(command) & {'stop', 'up', 'ack', 'requeue', 'get'})
        self.assertIn('NoRedirect', gate.runtime.BROKER)
        requests = [node for node in ast.walk(ast.parse(gate.runtime.BROKER))
                    if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute)
                    and node.func.attr == 'Request']
        self.assertEqual(len(requests), 1)
        self.assertEqual(len(requests[0].args), 1)
        self.assertEqual({kw.arg for kw in requests[0].keywords}, {'headers', 'method'})
        method = next(kw.value for kw in requests[0].keywords if kw.arg == 'method')
        self.assertEqual(ast.literal_eval(method), 'GET')

    def test_observation_errors_and_bad_counts_block(self):
        for error in ('db', 'broker', 'ps'):
            with self.subTest(error=error), self.assertRaises(Exception):
                self.check(FakeRuntime(error=error))
        for value in ('', '-1', 'NaN', '0\n0', 'null', '1'):
            with self.subTest(value=value), self.assertRaises(gate.GateError):
                self.check(FakeRuntime(counts=[value]))

    def test_ready_unacked_pending_retry_and_metadata_block(self):
        for key in ('messages_ready', 'messages_unacknowledged'):
            for name in ('image.normal', 'image.canary', 'video.canary', 'ppt.canary', 'image.normal.retry'):
                q = queues()
                q.append(dict(name='x.ai.generation.' + name + '.test', consumers=0,
                              messages_ready=0, messages_unacknowledged=0))
                q[-1][key] = 1
                with self.subTest(key=key, name=name), self.assertRaises(gate.GateError):
                    self.check(FakeRuntime(queue_data=q))
        for q in ([], {}, queues() + [queues()[0]], [dict(name='x.ai.test')]):
            with self.subTest(q=q), self.assertRaises(Exception):
                self.check(FakeRuntime(queue_data=q))

    def test_existing_dlq_scope_is_not_changed(self):
        q = queues() + [dict(name='x.ai.generation.image.canary.dlq', consumers=0,
                             messages_ready=62, messages_unacknowledged=0)]
        self.check(FakeRuntime(queue_data=q))
        # DLQ count does not remove unresolved DB executions.
        with self.assertRaises(gate.GateError):
            self.check(FakeRuntime(counts=['9'], queue_data=q))

    def test_no_bytecode_or_dirty_files_on_helper_import(self):
        with tempfile.TemporaryDirectory(prefix='issue199-import-') as directory:
            target = Path(directory)
            for name in ('verify-safe-drain.py', 'verify-release-runtime.py', 'quarantine-approval.py', 'quarantine-live-snapshot.py'):
                shutil.copyfile(str(ROOT / 'ops' / name), str(target / name))
            before = sorted(p.name for p in target.iterdir())
            result = subprocess.run([sys.executable, str(target / 'verify-safe-drain.py')],
                                    capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)  # Missing args: no runtime contact.
            self.assertEqual(sorted(p.name for p in target.iterdir()), before)

    def test_existing_timestamp_valid_cache_cannot_execute_or_bypass_gate(self):
        with tempfile.TemporaryDirectory(prefix='issue199-poisoned-cache-') as directory:
            target = Path(directory)
            source = target / 'verify-release-runtime.py'
            helper = target / 'verify-safe-drain.py'
            for name in (source.name, helper.name, 'quarantine-approval.py', 'quarantine-live-snapshot.py'):
                shutil.copy2(str(ROOT / 'ops' / name), str(target / name))
            original = source.read_bytes()
            original_stat = source.stat()
            marker = target / 'LOCAL_ONLY_CACHE_MARKER'
            # Same source size and timestamp: this is a valid cache even with -B.
            payload = ('from pathlib import Path\nPath(' + repr(str(marker)) +
                       ').write_text("unbound cache executed")\nraise SystemExit(0)\n').encode('utf-8')
            self.assertLess(len(payload), len(original))
            source.write_bytes(payload + b'#' * (len(original) - len(payload)))
            os.utime(str(source), ns=(original_stat.st_atime_ns, original_stat.st_mtime_ns))
            py_compile.compile(str(source), doraise=True,
                               invalidation_mode=py_compile.PycInvalidationMode.TIMESTAMP)
            source.write_bytes(original)
            os.utime(str(source), ns=(original_stat.st_atime_ns, original_stat.st_mtime_ns))
            # Positive control proves that a normal loader accepts this poisoned cache.
            control = ('import importlib.util,sys;sys.dont_write_bytecode=True;'
                       's=importlib.util.spec_from_file_location("probe",sys.argv[1]);'
                       'm=importlib.util.module_from_spec(s);s.loader.exec_module(m)')
            result = subprocess.run([sys.executable, '-c', control, str(source)],
                                    stdin=subprocess.DEVNULL, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0)
            self.assertTrue(marker.exists())
            marker.unlink()
            # Missing CLI args must be rejected by trusted source, not exit(0) in cache.
            result = subprocess.run([sys.executable, str(helper)],
                                    stdin=subprocess.DEVNULL, capture_output=True, text=True)
            self.assertEqual(result.returncode, 1)
            self.assertIn('usage: verify-safe-drain.py', result.stderr)
            self.assertFalse(marker.exists())
            self.assertEqual(source.read_bytes(), original)

    def test_toctou_and_identity_change_block(self):
        with self.assertRaisesRegex(gate.GateError, 'observation changed'):
            self.check(FakeRuntime(counts=['0', '1']))
        runner = FakeRuntime()
        def restart(args):
            result = runner(args)
            if len(runner.calls) > 4 and 'ps' in args:
                return result + '-restarted'
            return result
        with self.assertRaisesRegex(gate.GateError, 'observation changed'):
            self.check(restart)

    def test_invalid_timeout_blocks(self):
        for timeout in ('nan', 'inf', '-1', '301'):
            with self.subTest(timeout=timeout), self.assertRaises(gate.GateError):
                gate.verify('c', 'e', timeout, FakeRuntime())

    def test_cli_rejects_activation_without_contacting_runtime(self):
        result = subprocess.run([sys.executable, str(ROOT / 'ops/verify-safe-drain.py'),
                                 'c', 'e', '0', '--approve-quarantine', 'forged.json'],
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('usage: verify-safe-drain.py', result.stderr)
        self.assertNotIn('SECRET', result.stderr)

    def test_helpers_bound_and_proof_omission_rejected(self):
        prestage = (ROOT / 'ops/prestage-release.sh').read_text(encoding='utf-8')
        verify = (ROOT / 'ops/verify-prestage-proof.sh').read_text(encoding='utf-8')
        start = verify.index('scripts_hash = proof.get(')
        end = verify.index('for script_name, expected_script_hash', start)
        block = verify[start:end]
        def fail(msg):
            raise ValueError(msg)
        scripts = ('deploy.sh', 'rollback.sh', 'ops/verify-release-manifest.sh', 'ops/disk-guard.sh',
                   'ops/run-migrations.sh', 'ops/prestage-release.sh', 'ops/verify-prestage-proof.sh',
                   'ops/verify-release-runtime.py', 'ops/verify-safe-drain.py', 'ops/enroll-quarantine.py',
                   'ops/quarantine-approval.py', 'ops/quarantine-live-snapshot.py')
        self.assertIn('"ops/verify-safe-drain.py"', prestage)
        self.assertIn('"ops/enroll-quarantine.py"', prestage)
        self.assertIn('"ops/quarantine-approval.py"', prestage)
        self.assertIn('"ops/quarantine-live-snapshot.py"', prestage)
        hashes = {name: 'synthetic' for name in scripts}
        exec(block, {'proof': {'deploy_scripts_hash': hashes}, 'fail': fail})
        for missing in scripts:
            changed = dict(hashes)
            del changed[missing]
            with self.subTest(missing=missing), self.assertRaises(ValueError):
                exec(block, {'proof': {'deploy_scripts_hash': changed}, 'fail': fail})

    def test_python36_syntax_and_no_model_deployment_import(self):
        for filename in ('verify-safe-drain.py', 'enroll-quarantine.py', 'quarantine-approval.py', 'quarantine-live-snapshot.py', 'quarantine-drain-model.py'):
            text = (ROOT / 'ops' / filename).read_text(encoding='utf-8')
            ast.parse(text, feature_version=(3, 6))
        for filename in ('deploy.sh', 'ops/verify-safe-drain.py', 'ops/enroll-quarantine.py', 'ops/prestage-release.sh'):
            self.assertNotIn('quarantine-drain-model', (ROOT / filename).read_text(encoding='utf-8'))


class ModelTests(unittest.TestCase):
    def evaluate(self, entries, ids, trusted=None, sha='a' * 40, now=20):
        result = model.evaluate_hypothetical(entries, ids, trusted or {}, sha, now)
        self.assertEqual(result['deployment'], 'BLOCKED')
        return result

    def test_hypothetical_only_excludes_own_entry(self):
        e = entry()
        result = self.evaluate([e], [1], oracle(e))
        self.assertEqual(result['model'], 'HYPOTHETICAL_ONLY')
        self.assertEqual(result['excluded'], [1])
        result = self.evaluate([e, entry(2)], [1], oracle(e))
        self.assertEqual(result['excluded'], [1])
        self.assertEqual(result['remaining'], [2])
        self.assertEqual(result['model'], 'BLOCKED')

    def test_no_approval_and_self_attested_forgery_block(self):
        e = entry()
        self.assertEqual(self.evaluate([e], [1])['excluded'], [])
        for field in ('approved', 'recovery_verified', 'quarantine_enabled'):
            forged = copy.deepcopy(e)
            forged[field] = True
            self.assertEqual(self.evaluate([forged], [1], oracle(forged))['excluded'], [])
        for path in ('operator-api', 'watchdog', 'startup', 'scheduler', 'worker',
                     'callback', 'billing', 'artifact-recovery', 'in-flight-provider'):
            trusted = oracle(e)
            trusted[1]['recovery_isolated'] = False
            with self.subTest(path=path):
                self.assertEqual(self.evaluate([e], [1], trusted)['excluded'], [])

    def test_all_evidence_mutations_even_rehashed_block(self):
        original = entry()
        for key in model.FIELDS:
            changed = copy.deepcopy(original)
            value = changed[key]
            if type(value) is int:
                changed[key] += 1
            elif isinstance(value, dict):
                changed[key] = dict(value, forged=True)
            else:
                changed[key] = 'changed'
            # An attacker can recompute a digest, not the independent oracle.
            changed['snapshot_sha256'] = model.snapshot_digest(changed)
            with self.subTest(field=key):
                if key == 'snapshot_sha256':
                    changed[key] = '0' * 64
                self.assertEqual(self.evaluate([changed], [1], oracle(original))['excluded'], [])

    def test_missing_null_duplicate_extra_expired_and_release_mismatch(self):
        e = entry()
        for key in model.FIELDS:
            changed = copy.deepcopy(e)
            del changed[key]
            with self.subTest(missing=key):
                self.assertEqual(self.evaluate([changed], [1], oracle(e))['excluded'], [])
        for entries, ids in (([e, e], [1]), ([e], [1, 1]), ([e], [1, 999]),
                             ([dict(e, execution_status=None)], [1])):
            self.assertEqual(self.evaluate(entries, ids, oracle(e))['excluded'], [])
        for now in (9, 100, 101):
            self.assertEqual(self.evaluate([e], [1], oracle(e), now=now)['excluded'], [])
        self.assertEqual(self.evaluate([e], [1], oracle(e), sha='b' * 40)['excluded'], [])

    def test_valid_lease_and_active_owner_block_even_hypothetical_oracle(self):
        for changes in ({'lease_until': 21}, {'owner': 'worker'}, {'task_status_v2': 'RUNNING'}):
            e = dict(entry(), **changes)
            e['snapshot_sha256'] = model.snapshot_digest(e)
            self.assertEqual(self.evaluate([e], [1], oracle(e))['excluded'], [])

    def test_actual9_remain_blocked_without_fabricated_evidence(self):
        pairs = [(103, 324), (102, 323), (100, 321), (31, 251), (25, 245),
                 (22, 242), (14, 234), (12, 232), (1, 221)]
        # Identity/status-only redaction, intentionally incomplete, NOT approvals.
        entries = [dict(execution_id=e, task_id=t,
                        execution_status='submitted' if e == 14 else 'unknown') for e, t in pairs]
        result = self.evaluate(entries, [e for e, _ in pairs])
        self.assertEqual(result['excluded'], [])
        self.assertEqual(result['model'], 'BLOCKED')


@unittest.skipUnless(os.environ.get('ISSUE199_DOCKER_REPLAY') == '1', 'explicit local Docker replay not requested')
class PostgresReplay(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        endpoint = subprocess.check_output(['docker', 'context', 'inspect', '--format',
                                            '{{.Endpoints.docker.Host}}'], text=True).strip()
        if not endpoint.startswith(('npipe:', 'unix:')):
            raise RuntimeError('Only a local Docker endpoint is permitted')
        cls.container = 'issue199-drain-test-' + uuid.uuid4().hex[:10]
        subprocess.run(['docker', 'run', '--pull=never', '-d', '--network', 'none',
                        '--name', cls.container, '-e', 'POSTGRES_PASSWORD=synthetic-local-only',
                        'postgres:16-alpine'], check=True, stdout=subprocess.DEVNULL)
        cls.addClassCleanup(lambda: subprocess.run(['docker', 'rm', '-f', cls.container],
                                                  stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        for _ in range(40):
            r = subprocess.run(['docker', 'exec', cls.container, 'pg_isready', '-U', 'postgres'],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if r.returncode == 0:
                return
            time.sleep(.5)
        raise RuntimeError('isolated PostgreSQL did not start')

    def sql(self, text, readonly=False):
        args = ['docker', 'exec', '-i']
        if readonly:
            args += ['-e', 'PGOPTIONS=-c default_transaction_read_only=on']
        args += [self.container, 'psql', '-X', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-Atq']
        return subprocess.run(args, input=text, text=True, capture_output=True)

    def setUp(self):
        # Relaxed constraints deliberately allow corruption/unsupported-schema cases.
        result = self.sql('''DROP SCHEMA public CASCADE; CREATE SCHEMA public;
CREATE TABLE xz_generation_tasks(id text, status text, task_status text, lease_until timestamptz);
CREATE TABLE provider_executions(id bigint, task_id text, attempt int, status text);
CREATE TABLE outbox_events(status text);
CREATE TABLE video_task_outbox(state text);
INSERT INTO xz_generation_tasks VALUES ('t','FAILED','FAILED',now()-interval '1 day');
INSERT INTO provider_executions VALUES (1,'t',1,'failed');''')
        self.assertEqual(result.returncode, 0, 'isolated fixture setup failed')

    def count(self):
        result = self.sql(gate.SQL, readonly=True)
        self.assertEqual(result.returncode, 0, 'read-only drain query failed')
        return int(result.stdout.strip())

    def test_clear_and_no_database_writes(self):
        self.assertEqual(self.count(), 0)
        write = self.sql("UPDATE provider_executions SET status='succeeded';", readonly=True)
        self.assertNotEqual(write.returncode, 0)
        self.assertEqual(self.count(), 0)

    def test_blocking_records(self):
        cases = [
            "UPDATE provider_executions SET status=NULL",
            "UPDATE provider_executions SET status='unexpected'",
            "UPDATE provider_executions SET status='unknown'",
            "UPDATE provider_executions SET status='submitted'",
            "UPDATE provider_executions SET task_id='missing'",
            "UPDATE provider_executions SET attempt=NULL",
            "UPDATE xz_generation_tasks SET status=NULL",
            "UPDATE xz_generation_tasks SET task_status=NULL",
            "UPDATE xz_generation_tasks SET task_status='RUNNING'",
            "UPDATE xz_generation_tasks SET status='RUNNING'",
            "UPDATE xz_generation_tasks SET lease_until=now()+interval '1 hour'",
            "INSERT INTO provider_executions SELECT * FROM provider_executions",
            "INSERT INTO xz_generation_tasks SELECT * FROM xz_generation_tasks",
            "INSERT INTO outbox_events VALUES ('pending')",
            "INSERT INTO outbox_events VALUES ('publishing')",
            "INSERT INTO outbox_events VALUES (NULL)",
            "INSERT INTO video_task_outbox VALUES ('pending')",
            "INSERT INTO video_task_outbox VALUES ('publishing')",
            "INSERT INTO video_task_outbox VALUES (NULL)",
        ]
        for sql in cases:
            with self.subTest(sql=sql):
                self.setUp()
                self.assertEqual(self.sql(sql).returncode, 0)
                self.assertGreater(self.count(), 0)

    def test_legal_outbox_terminal_states_remain_clear(self):
        self.assertEqual(self.sql("INSERT INTO outbox_events VALUES ('published'),('failed');"
                                  "INSERT INTO video_task_outbox VALUES ('published'),('failed');").returncode, 0)
        self.assertEqual(self.count(), 0)

    def test_missing_schema_query_failure(self):
        for sql in ('DROP TABLE provider_executions', 'ALTER TABLE xz_generation_tasks DROP COLUMN task_status'):
            with self.subTest(sql=sql):
                self.setUp()
                self.assertEqual(self.sql(sql).returncode, 0)
                self.assertNotEqual(self.sql(gate.SQL, readonly=True).returncode, 0)

    def test_actual9_and_release_toctou(self):
        self.assertEqual(self.count(), 0)
        pairs = [(103,324),(102,323),(100,321),(31,251),(25,245),(22,242),(14,234),(12,232),(1,221)]
        self.assertEqual(self.sql('TRUNCATE provider_executions, xz_generation_tasks;').returncode, 0)
        for eid, tid in pairs:
            self.assertEqual(self.sql("INSERT INTO xz_generation_tasks VALUES ('%s','FAILED','FAILED',now()-interval '1 day');"
                                      "INSERT INTO provider_executions VALUES (%s,'%s',1,'%s');" %
                                      (tid, eid, tid, 'submitted' if eid == 14 else 'unknown')).returncode, 0)
        # Without exemption: strictly 9
        self.assertEqual(self.count(), 9)
        # With approved exemption: count becomes 0
        e_ids = [e for e, _ in pairs]
        exempt_sql = gate.build_sql(e_ids)
        res = self.sql(exempt_sql, readonly=True)
        self.assertEqual(res.returncode, 0)
        self.assertEqual(int(res.stdout.strip()), 0)
        # If an unapproved 10th execution exists: count is 1
        self.assertEqual(self.sql("INSERT INTO xz_generation_tasks VALUES ('t10','FAILED','FAILED',now()-interval '1 day');"
                                  "INSERT INTO provider_executions VALUES (999,'t10',1,'unknown');").returncode, 0)
        res10 = self.sql(exempt_sql, readonly=True)
        self.assertEqual(int(res10.stdout.strip()), 1)
        # If one of the 9 has an active lease: count is 2 (task lease + unapproved 10th)
        self.assertEqual(self.sql("UPDATE xz_generation_tasks SET lease_until=now()+interval '1 hour' WHERE id='324';").returncode, 0)
        res_lease = self.sql(exempt_sql, readonly=True)
        self.assertGreater(int(res_lease.stdout.strip()), 1)

    def test_quarantine_table_immutability_and_rollback_truncate(self):
        # Create quarantine table and trigger
        ddl = (ROOT / 'database/migrations/121-provider-execution-quarantine.sql').read_text(encoding='utf-8')
        # Setup prerequisite tables with required primary keys
        self.assertEqual(self.sql('''
DROP SCHEMA public CASCADE; CREATE SCHEMA public;
CREATE TABLE xz_generation_tasks(id text PRIMARY KEY, status text, task_status text, lease_until timestamptz);
CREATE TABLE provider_executions(id bigint PRIMARY KEY, task_id text, attempt int, status text);
CREATE TABLE outbox_events(status text);
CREATE TABLE video_task_outbox(state text);
''' + ddl).returncode, 0)
        # Insert a quarantine row
        insert_sql = """
INSERT INTO provider_executions VALUES (1, 't1', 1, 'unknown');
INSERT INTO provider_execution_quarantine (
    execution_id, task_id, attempt, generation,
    snapshot_sha256, evidence_sha256, approval_id, release_sha,
    not_before, expires_at
) VALUES (
    1, 't1', 1, NULL,
    '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
    '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
    'approval-test', '79a3d7cf493453adbcdc03234b62fc5f099f776a',
    now() - interval '1 hour', now() + interval '1 hour'
);
"""
        self.assertEqual(self.sql(insert_sql).returncode, 0)
        # Assert UPDATE fails due to immutable trigger
        self.assertNotEqual(self.sql("UPDATE provider_execution_quarantine SET approval_id='tampered';").returncode, 0)
        # Assert DELETE fails due to immutable trigger
        self.assertNotEqual(self.sql("DELETE FROM provider_execution_quarantine WHERE execution_id=1;").returncode, 0)
        # Assert TRUNCATE succeeds (used exclusively by rollback on release failure)
        self.assertEqual(self.sql("TRUNCATE TABLE provider_execution_quarantine;").returncode, 0)
        count_res = self.sql("SELECT count(*) FROM provider_execution_quarantine;")
        self.assertEqual(count_res.returncode, 0)
        self.assertEqual(int(count_res.stdout.strip()), 0)


if __name__ == '__main__':
    unittest.main(verbosity=2)
