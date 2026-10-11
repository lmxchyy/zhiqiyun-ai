#!/usr/bin/env python3
"""Owned Linux/Python3.6.8 native DB and post-start controller-death tests.

Mechanism containers use owned tooling bytes, NOT a substitute target image.
--certify-fixed additionally requires actual fixed official bytes and invokes
strict real attest. Missing prerequisites ERROR, never SKIP or fallback.
"""
import copy
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import types
import unittest
import uuid
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
path = ROOT / 'ops/cold-recovery-resume.py'
r = types.ModuleType('resume')
r.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), r.__dict__)
LABEL = 'org.xianzhi.cold-resume.owner'
OUT = ROOT / '.evidence/cold-recovery-resume'
FIXED_REF = 'ghcr.io/lmxchyy/zhiqiyun-ai@' + r.DIGEST


class OwnedDocker:
    def __init__(self, docker, owner, log):
        self.docker, self.owner, self.log = docker, owner, log
        self.fail_start = None
        self.timeout_start = False
        self.delayed_disconnect = None
        self.delayed_gate = None

    def inspect(self, kind, identity):
        obj = self.docker.inspect(kind, identity)
        if kind == 'container':
            if obj['Config']['Labels'].get(LABEL) != self.owner:
                raise AssertionError('foreign container')
        return obj

    def text(self, args, **kwargs):
        if args[:3] == ['ps', '-aq', '--no-trunc']:
            return self.docker.text(args + ['--filter', 'label=' + LABEL + '=' + self.owner])
        if args[0] in ('start', 'stop', 'update'):
            self.inspect('container', args[-1])
            with open(str(self.log), 'a') as stream:
                stream.write(json.dumps(args) + '\n')
            if args[0] == 'start' and self.delayed_disconnect:
                # Fault model ONLY: a NEW owned Docker client is gated until
                # after fencing, simulating unresolved late completion. This
                # does NOT prove real dockerd accepted the earlier request.
                # Only the already ownership-checked CID is used.
                code = ('import os,subprocess,time\nend=time.monotonic()+30\n'
                        'while not os.path.exists(' + repr(str(self.delayed_gate)) + '):\n'
                        ' if time.monotonic()>end: raise RuntimeError("fixture gate timeout")\n'
                        ' time.sleep(.05)\nsubprocess.check_call(' + repr(self.docker.cli + args) + ')')
                subprocess.Popen([sys.executable, '-c', code], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                if self.delayed_disconnect == 'timeout':
                    raise subprocess.TimeoutExpired(args, 1)
                raise r.Refused('owned NEW-client late-completion fault model; not accepted-daemon evidence')
            if args[0] == 'start' and args[-1] == self.fail_start:
                raise r.Refused('owned injected second start failure')
            if args[0] == 'start' and self.timeout_start:
                self.docker.text(args, **kwargs)
                raise subprocess.TimeoutExpired(args, 1)
        return self.docker.text(args, **kwargs)


class OwnedTarget:
    def __init__(self, docker, cid):
        self.docker, self.expected = docker, {'container_id': cid}
        self.check()

    def check(self):
        obj = self.docker.inspect('container', self.expected['container_id'])
        if obj['State']['Running'] is not True:
            raise AssertionError('owned PG stopped')


class NativeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if sys.platform != 'linux' or sys.version_info[:3] != (3, 6, 8):
            raise RuntimeError('actual native Linux Python3.6.8 required; zero skips')
        cls.raw = r.cap.Docker()
        cls.raw.text(['info'], timeout=20)
        cls.raw.inspect('image', 'pgvector/pgvector:pg16')
        for name, expected in r.FROZEN.items():
            if r.cold.file_hash(str(ROOT / name)) != expected:
                raise RuntimeError('frozen source changed')

    def setUp(self):
        self.owner = uuid.uuid4().hex
        self.project = 'resume-owned-' + self.owner
        self.temp = tempfile.TemporaryDirectory(prefix='cold-resume-owned-')
        self.directory = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)
        self.log = self.directory / 'commands.jsonl'
        self.docker = OwnedDocker(self.raw, self.owner, self.log)
        self.resources = []
        self.addCleanup(self.cleanup)
        self.secret = mock.patch.dict(os.environ, RELEASE_TRUST_SECRET='owned-test-key-not-release-authority')
        self.secret.start()
        self.addCleanup(self.secret.stop)
        (self.directory / r.cold.HOLD).mkdir()
        self.db = self.create('postgres', ['postgres'], running=True,
                              options=['--tmpfs', '/var/lib/postgresql/data', '-e', 'POSTGRES_USER=fixture_admin',
                                       '-e', 'POSTGRES_DB=owned_resume', '-e', 'POSTGRES_PASSWORD=owned_resume_only'])
        for unused in range(60):
            result = self.raw.run(['exec', self.db, 'psql', '-X', '-U', 'fixture_admin', '-d', 'owned_resume', '-Atqc', 'SELECT 1'], check=False)
            if result.returncode == 0 and result.stdout.strip() == b'1':
                break
            time.sleep(.25)
        else:
            raise RuntimeError('owned PG readiness failed')
        schema = (ROOT / 'database/schema.sql').read_text(encoding='utf-8')
        for path in sorted((ROOT / 'database/migrations').glob('[0-9][0-9][0-9]-*.sql')):
            if not path.name.endswith('.down.sql'):
                schema += '\n' + path.read_text(encoding='utf-8')
        self.sql(schema)
        self.anchor = r.read(str(ROOT / 'ops/cold-recovery-anchor.json'))
        fields = r.QUARANTINE[len('SELECT '):].split(' FROM ')[0].split(',')
        statements = ["INSERT INTO xz_users(id,name,role,status) VALUES('owned-user','owned','MEMBER','ACTIVE');",
                      'CREATE TABLE schema_migrations(filename text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now());']
        for row in self.anchor['quarantine_records']:
            statements.append("INSERT INTO xz_generation_tasks(id,user_id,type,status,task_status,params,raw) VALUES(%s,'owned-user','TEXT_TO_IMAGE','PROCESSING','RUNNING','{}','{}');" % r.transport.parameter(row['task_id']))
            statements.append("INSERT INTO provider_executions(id,task_id,provider,provider_model,capability,attempt,status,request_fingerprint,task_execution_generation) VALUES(%s,%s,'fixture','fixture','image',1,'prepared',%s,%s);" % (row['execution_id'], r.transport.parameter(row['task_id']), r.transport.parameter('a' * 64), r.transport.parameter(row['generation'])))
            statements.append('INSERT INTO provider_execution_quarantine(' + ','.join(fields) + ') VALUES(' + ','.join(r.transport.parameter(row[k]) + ('::timestamptz' if k in ('not_before', 'expires_at', 'created_at') else '') for k in fields) + ');')
        for path in sorted((ROOT / 'database/migrations').glob('[0-9][0-9][0-9]-*.sql')):
            if not path.name.endswith('.down.sql') and 108 <= int(path.name[:3]) <= 121:
                statements.append('INSERT INTO schema_migrations(filename) VALUES(' + r.transport.parameter(path.name) + ');')
        self.sql('\n'.join(statements))
        self.target = OwnedTarget(self.docker, self.db)
        conn = r.ReadOnlyConnection(self.target)
        try:
            cursor = conn.cursor()
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            history = []
            for query in r.HISTORY:
                cursor.execute(query)
                history.append(cursor.fetchone()[0])
            cursor.execute('ROLLBACK')
            self.anchor['history'] = history  # owned mechanism scope, never historical production claim
        finally:
            conn.close()
        self.binding = mock.patch.object(r, 'ReadOnlyTarget', return_value=self.target)
        self.binding.start()
        self.addCleanup(self.binding.stop)
        self.before = r.database_snapshot({}, self.anchor)
        # PID1 must handle TERM: preserve the production fence's 120s budget,
        # without spending that whole grace on a nonresponsive test sleep PID1.
        responsive = ['sh', '-c', "trap 'exit 0' TERM INT; while :; do sleep 1 & wait $!; done"]
        self.cids = [self.create(role, responsive) for role in r.ROLES]
        # Historical target was already started then cold-stopped. Match that
        # lifecycle: Docker may finalize anonymous mounts on the first start.
        for cid in self.cids:
            self.docker.inspect('container', cid)
            self.raw.text(['start', cid])
            self.raw.text(['stop', '-t', '1', cid])
        # Mechanism fixture is project-classified, not an invented app identity.
        # Exclude the shared PG tooling bytes from exact-business classification.
        self.h = dict(version=1, state=r.STARTING, owner='historical-owner', project=self.project,
                      release_sha=r.APP, proof_sha256=r.OLD_PROOF, image_ids=['sha256:' + 'b' * 64, 'sha256:' + 'a' * 64],
                      recovery_spent_nonces=[self.owner], recovery_authorized_cids=self.cids, recovery_attempt={'nonce': self.owner})
        r.cold.write_hold(str(self.directory), self.h)

    def create(self, role, command, running=False, options=()):
        args = ['create', '--pull', 'never', '--network', 'none', '--restart', 'no',
                '--label', LABEL + '=' + self.owner,
                '--label', 'com.docker.compose.project=' + self.project,
                '--label', 'com.docker.compose.service=' + role] + list(options) + ['pgvector/pgvector:pg16'] + command
        cid = self.raw.text(args)
        self.resources.append(cid)
        self.docker.inspect('container', cid)
        if running:
            self.raw.text(['start', cid])
        return cid

    def sql(self, sql):
        self.docker.inspect('container', self.db)
        return self.raw.run(['exec', '-i', self.db, 'psql', '-X', '-U', 'fixture_admin', '-d', 'owned_resume', '-v', 'ON_ERROR_STOP=1', '-At'], data=sql.encode(), timeout=180).stdout.decode().strip()

    def cleanup(self):
        errors = []
        for cid in reversed(self.resources):
            try:
                self.docker.inspect('container', cid)
                self.raw.text(['rm', '-f', '-v', cid])
            except Exception:
                errors.append(cid)
        if errors:
            raise RuntimeError('owned cleanup failed')

    def stopped(self):
        for cid in self.cids:
            obj = self.docker.inspect('container', cid)
            self.assertFalse(obj['State']['Running'])
            self.assertEqual(obj['HostConfig']['RestartPolicy']['Name'], 'no')
        h = r.cold.read_hold(str(self.directory))
        self.assertEqual(h['recovery_spent_nonces'], [self.owner])
        self.assertEqual(self.before, r.database_snapshot({}, self.anchor))

    def test_native_full_records_exact9_and_financial_drift(self):
        self.assertEqual(self.before, r.database_snapshot({}, self.anchor))
        for change in ('absent', 'extra', 'replaced', 'created_at', 'NULL', 'generation', 'evidence'):
            bad = copy.deepcopy(self.anchor)
            if change == 'absent': bad['quarantine_records'].pop()
            if change == 'extra': bad['quarantine_records'].append(dict(bad['quarantine_records'][0]))
            if change == 'replaced': bad['quarantine_records'][0]['execution_id'] = 10
            if change == 'created_at': bad['quarantine_records'][0]['created_at'] = '2026-10-10T17:01:04+00:00'
            if change == 'NULL': bad['quarantine_records'][0]['generation'] = 'NULL'
            if change == 'generation': bad['quarantine_records'][6]['generation'] = '2'
            if change == 'evidence': bad['quarantine_records'][0]['evidence_sha256'] = 'a' * 64
            with self.subTest(change=change):
                with self.assertRaises(r.Refused): r.database_snapshot({}, bad)
        self.sql("UPDATE xz_generation_tasks SET point_cost=1 WHERE id='task_000221';")
        with self.assertRaises(r.Refused): r.database_snapshot({}, self.anchor)

    def test_migration_incomplete_and_readonly_sql_authority(self):
        conn = r.ReadOnlyConnection(self.target)
        with self.assertRaises(r.Refused): conn.cursor().execute('COMMIT')
        self.assertTrue(conn.closed)
        self.sql("DELETE FROM schema_migrations WHERE filename='121-provider-execution-quarantine.sql';")
        with self.assertRaises(r.Refused): r.database_snapshot({}, self.anchor)

    def commit_authority(self):
        # Mechanism-only signed approval. Packaged cert is independently mandatory
        # in --certify-fixed; these mocks never become a production CLI mode.
        source = dict(controller_source_sha='d' * 40, app_source_sha=r.APP, helper_sha256={'owned': '1' * 64})
        original = dict(self.h, state=r.STOPPED, recovery_spent_nonces=[])
        original.pop('recovery_attempt')
        now = int(time.time())
        candidate = dict(version=1, purpose=r.DOMAIN + '-proof', nonce=self.owner,
                         not_before=now, expires_at=now + 800, hold_sha256=r.digest(original),
                         snapshot={'source': source, 'policy': {}, 'containers': {role: {'cid': cid} for role, cid in zip(r.ROLES, self.cids)}}, runtime_capability={'mechanism-only': True},
                         operations=['start-existing-api', 'start-existing-worker'])
        approval = r.approve(candidate, original, source, 'fixture.operator', r.phrase(candidate))
        for module, name, value in ((r, 'source_identity', source), (r.cap, 'verify', r.IMAGE), (r.cap, 'verify_history', True)):
            patch = mock.patch.object(module, name, return_value=value)
            patch.start()
            self.addCleanup(patch.stop)
        self.h['recovery_attempt'].update(candidate_sha256=r.digest(candidate), approval_sha256=r.digest(approval))
        r.cold.write_hold(str(self.directory), self.h)
        return {'hold': original, 'candidate': candidate, 'approval': approval}

    def start_for_commit(self, lifetime=300):
        authority = self.commit_authority()
        lock = r.ReleaseLock(str(self.directory))
        self.h['recovery_attempt']['owner'] = lock.owner
        r.cold.write_hold(str(self.directory), self.h)
        supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids,
                                         lifetime=lifetime, authority=authority)
        for unused in self.cids:
            supervisor.command(b'S')
        return lock, supervisor

    def test_real_exact_two_starts_success_ack_and_reentry_receipt(self):
        lock, supervisor = self.start_for_commit()
        try:
            for cid in self.cids:
                self.assertTrue(self.docker.inspect('container', cid)['State']['Running'])
            self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STARTING)
            supervisor.close(success=True)
            lock.release()
            self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.SUCCESS)
            calls = [json.loads(line) for line in self.log.read_text().splitlines()]
            self.assertEqual(calls, [['start', cid] for cid in self.cids])
            self.assertEqual(self.before, r.database_snapshot({}, self.anchor))
        finally:
            if not supervisor.finished:
                with self.assertRaises(r.Refused): supervisor.close()

    def test_actual_execute_owned_native_snapshot_and_exact_two_starts(self):
        authority = self.commit_authority()
        original, candidate = authority['hold'], authority['candidate']
        r.cold.write_hold(str(self.directory), original)
        fixture = self
        class OwnedBackend(r.Backend):
            # Production Backend paths/CIDs cannot be replaced via CLI. This
            # internal test adapter exercises execute, native DB and real start/
            # guardian primitives with explicitly NON-target fixture policies.
            def __init__(self):
                self.docker = fixture.docker
            def snapshot(self, running=()):
                containers = {}
                for role, cid in zip(r.ROLES, fixture.cids):
                    obj = self.docker.inspect('container', cid)
                    fixture.assertIs(obj['State']['Running'], role in running)
                    fixture.assertEqual(obj['HostConfig']['RestartPolicy']['Name'], 'no')
                    policy = {k: obj.get(k) for k in ('Id', 'Image', 'Config', 'HostConfig', 'Mounts', 'Path', 'Args')}
                    containers[role] = {'cid': cid, 'policy_sha256': r.digest(policy)}
                fixture.target.check()
                return {'source': candidate['snapshot']['source'], 'containers': containers,
                        'policy': {}, 'database': r.database_snapshot({}, fixture.anchor),
                        'scope': 'owned-mechanism-NOT-production-process-or-image-certification'}
            def verify(self, evidence, snapshot):
                fixture.assertEqual(evidence, {'mechanism-only': True})
            def healthy(self, evidence):
                for cid in fixture.cids:
                    fixture.assertTrue(self.docker.inspect('container', cid)['State']['Running'])
                    self.docker.text(['exec', cid, 'sh', '-c', 'kill -0 1'])
        backend = OwnedBackend()
        candidate['snapshot'] = backend.snapshot()
        approval = r.approve(candidate, original, candidate['snapshot']['source'], 'fixture.operator', r.phrase(candidate))
        lock = r.ReleaseLock(str(self.directory))
        def owned_stopped(directory):
            h = r.cold.read_hold(directory)
            if h['state'] != r.STOPPED or h['project'] != self.project:
                raise r.Refused('owned stopped hold only')
            return h
        with mock.patch.object(r, 'read_stopped_hold', side_effect=owned_stopped):
            r.execute(candidate, approval, backend, lock, str(self.directory))
        lock.release()
        h = r.cold.read_hold(str(self.directory))
        self.assertEqual(h['state'], r.SUCCESS)
        self.assertEqual(h['recovery_verified_cids'], self.cids)
        self.assertEqual(h['recovery_spent_nonces'], [self.owner])
        self.assertEqual([json.loads(line) for line in self.log.read_text().splitlines()], [['start', cid] for cid in self.cids])
        self.assertEqual(self.before, r.database_snapshot({}, self.anchor))

    def test_foreign_lock_attempt_cids_identity_never_fenced_or_overwritten(self):
        for change in ('lock', 'nonce', 'cids', 'project'):
            with self.subTest(change=change):
                r.cold.write_hold(str(self.directory), self.h)
                lock = r.ReleaseLock(str(self.directory))
                self.h['recovery_attempt']['owner'] = lock.owner
                r.cold.write_hold(str(self.directory), self.h)
                supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids)
                supervisor.command(b'S')
                foreign = r.cold.read_hold(str(self.directory))
                if change == 'lock':
                    Path(lock.path, 'owner_token').write_text('foreign-owner')
                if change == 'nonce': foreign['recovery_attempt']['nonce'] = 'foreign-nonce'
                if change == 'cids': foreign['recovery_authorized_cids'] = ['f' * 64]
                if change == 'project': foreign['project'] = 'foreign-project'
                r.cold.write_hold(str(self.directory), foreign)
                before_log = self.log.read_text()
                with self.assertRaises(r.Refused): supervisor.close()
                self.assertEqual(foreign, r.cold.read_hold(str(self.directory)))
                self.assertEqual(before_log, self.log.read_text())
                self.assertTrue(self.docker.inspect('container', self.cids[0])['State']['Running'])
                self.assertTrue(Path(lock.path).is_dir())
                # Only test-owned cleanup after known successful CLI completion.
                self.raw.text(['stop', '-t', '1', self.cids[0]])
                Path(lock.path, 'owner_token').write_text(lock.owner)
                lock.retained = False
                lock.release()
        self.assertEqual(self.before, r.database_snapshot({}, self.anchor))

    def test_start_sentinel_fsync_crosses_deadline_issues_zero_starts(self):
        original = r.cold.write_hold
        def slow_sentinel(directory, value):
            result = original(directory, value)
            if value['state'] == r.STARTING and value.get('recovery_indeterminate_operation'):
                time.sleep(2)
            return result
        lock = r.ReleaseLock(str(self.directory))
        self.h['recovery_attempt']['owner'] = lock.owner
        r.cold.write_hold(str(self.directory), self.h)
        with mock.patch.object(r.cold, 'write_hold', side_effect=slow_sentinel):
            supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids, lifetime=1)
            with self.assertRaises(r.Refused): supervisor.command(b'S')
            with self.assertRaises(r.Refused): supervisor.close()
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertFalse(any(call[0] == 'start' for call in calls))
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STOPPED)

    def test_final_commit_boundary_races_share_only_owned_pg_fixture(self):
        # Avoid replaying the full owned schema for each terminal boundary;
        # reset only our settled pair/hold/lock, retaining every assertion.
        for case in (
            self.case_final_commit_after_guard_deadline_cannot_overwrite_fence,
            self.case_final_commit_eof_cannot_write_success,
            self.case_final_commit_refuses_already_fenced_state,
            self.case_final_commit_audit_failure_fences,
            self.case_final_commit_deadline_during_validation_fences,
        ):
            with self.subTest(boundary=case.__name__):
                r.cold.write_hold(str(self.directory), self.h)
                case()
                self.stopped()
                self.assertFalse((self.directory / 'release.lock').exists())

    def case_final_commit_after_guard_deadline_cannot_overwrite_fence(self):
        lock, supervisor = self.start_for_commit(lifetime=5)
        time.sleep(6)
        with self.assertRaises(r.Refused): supervisor.close(success=True)
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STOPPED)

    def case_final_commit_eof_cannot_write_success(self):
        lock, supervisor = self.start_for_commit()
        with self.assertRaises(r.Refused): supervisor.close()
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STOPPED)

    def case_final_commit_refuses_already_fenced_state(self):
        lock, supervisor = self.start_for_commit()
        r.cold.fence(str(self.directory), 'fixture-commit-race', 'fixture-fence', self.docker)
        with self.assertRaises(r.Refused): supervisor.close(success=True)
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STOPPED)

    def case_final_commit_audit_failure_fences(self):
        # Fault installed before fork; only SUCCESS write fails, fencing remains
        # independently durable via the unchanged existing atomic primitive.
        original = r.cold.write_hold
        def write(directory, value):
            if value['state'] == r.SUCCESS:
                raise OSError('owned injected terminal fsync failure')
            return original(directory, value)
        with mock.patch.object(r.cold, 'write_hold', side_effect=write):
            lock, supervisor = self.start_for_commit()
            with self.assertRaises(r.Refused): supervisor.close(success=True)
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STOPPED)

    def case_final_commit_deadline_during_validation_fences(self):
        # Validation begins in-window but overruns it, deterministic guard race.
        authority = self.commit_authority()
        def slow_verify(*args):
            time.sleep(6)
            return r.IMAGE
        lock = r.ReleaseLock(str(self.directory))
        self.h['recovery_attempt']['owner'] = lock.owner
        r.cold.write_hold(str(self.directory), self.h)
        with mock.patch.object(r.cap, 'verify', side_effect=slow_verify):
            supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids,
                                             lifetime=5, authority=authority)
            for unused in self.cids: supervisor.command(b'S')
            with self.assertRaises(r.Refused): supervisor.close(success=True)
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], r.STOPPED)

    def test_partial_start_failure_fences_and_keeps_hold(self):
        self.docker.fail_start = self.cids[1]
        lock = r.ReleaseLock(str(self.directory))
        self.h['recovery_attempt']['owner'] = lock.owner
        r.cold.write_hold(str(self.directory), self.h)
        supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids)
        supervisor.command(b'S')
        with self.assertRaises(r.Refused): supervisor.command(b'S')
        with self.assertRaises(r.Refused): supervisor.close()
        self.stopped()
        self.assertEqual(r.cold.read_hold(str(self.directory))['state'], 'RECOVERY_REQUIRED_UNKNOWN')
        self.assertTrue(Path(lock.path).is_dir())

    def test_start_timeout_retains_indeterminate_unknown_lock(self):
        self.docker.timeout_start = True
        lock = r.ReleaseLock(str(self.directory))
        self.h['recovery_attempt']['owner'] = lock.owner
        r.cold.write_hold(str(self.directory), self.h)
        supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids)
        with self.assertRaises(r.Refused): supervisor.command(b'S')
        with self.assertRaises(r.Refused): supervisor.close()
        self.stopped()
        h = r.cold.read_hold(str(self.directory))
        self.assertEqual(h['state'], 'RECOVERY_REQUIRED_UNKNOWN')
        self.assertIn('recovery_indeterminate_operation', h)
        self.assertTrue(Path(lock.path).is_dir())

    def test_adversarial_delayed_new_client_preserves_unknown_fault_model(self):
        for failure in ('timeout', 'disconnect'):
            with self.subTest(failure=failure):
                r.cold.write_hold(str(self.directory), self.h)
                self.docker.delayed_disconnect = failure
                gate = self.directory / ('settled-fence-' + failure)
                self.docker.delayed_gate = gate
                lock = r.ReleaseLock(str(self.directory))
                self.h['recovery_attempt']['owner'] = lock.owner
                r.cold.write_hold(str(self.directory), self.h)
                supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids)
                with self.assertRaises(r.Refused): supervisor.command(b'S')
                with self.assertRaises(r.Refused): supervisor.close()
                h = r.cold.read_hold(str(self.directory))
                self.assertEqual(h['state'], 'RECOVERY_REQUIRED_UNKNOWN')
                self.assertIn('recovery_indeterminate_operation', h)
                self.assertTrue(Path(lock.path).is_dir())
                gate.write_text('fixture adversary may now complete accepted start')
                deadline = time.monotonic() + 15
                while time.monotonic() < deadline:
                    if self.docker.inspect('container', self.cids[0])['State']['Running']: break
                    time.sleep(.1)
                else: self.fail('delayed-start adversary was not actually reproduced')
                # UNKNOWN never promises stopped. This NEW-client model is not
                # proof of an earlier accepted daemon operation or a fundamental
                # production impossibility. Only test-owned cleanup settles it.
                self.raw.text(['stop', '-t', '1', self.cids[0]])
                self.assertEqual(h, r.cold.read_hold(str(self.directory)))
                # Fixture-only release AFTER independently settling our delayed
                # operation; production entry intentionally offers no override.
                lock.retained = False
                lock.release()
                self.assertEqual(self.before, r.database_snapshot({}, self.anchor))

    def test_controller_death_after_each_start_sigkill_term_int(self):
        for sig, count in ((signal.SIGKILL, 1), (signal.SIGKILL, 2), (signal.SIGTERM, 1), (signal.SIGINT, 2)):
            with self.subTest(signal=sig, count=count):
                r.cold.write_hold(str(self.directory), self.h)
                ready_r, ready_w = os.pipe()
                pid = os.fork()
                if pid == 0:
                    # A forked test controller must NEVER unwind into unittest,
                    # including lock/arm/start failures before the ready message.
                    try:
                        os.close(ready_r)
                        lock = r.ReleaseLock(str(self.directory))
                        self.h['recovery_attempt']['owner'] = lock.owner
                        r.cold.write_hold(str(self.directory), self.h)
                        supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids)
                        for unused in range(count): supervisor.command(b'S')
                        os.write(ready_w, b'R')
                        os.close(ready_w)
                        try:
                            time.sleep(100)
                        finally:
                            try: supervisor.close()
                            except BaseException: pass
                    except BaseException:
                        pass
                    finally:
                        os._exit(1)
                os.close(ready_w)
                self.assertEqual(os.read(ready_r, 1), b'R')
                os.close(ready_r)
                os.kill(pid, sig)
                os.waitpid(pid, 0)
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline:
                    h = r.cold.read_hold(str(self.directory))
                    if h['state'] == r.STOPPED and not (self.directory / 'release.lock').exists(): break
                    time.sleep(.1)
                else: self.fail('post-controller-death fence did not settle')
                self.stopped()

    def test_supervisor_lifetime_before_start_fences(self):
        lock = r.ReleaseLock(str(self.directory))
        self.h['recovery_attempt']['owner'] = lock.owner
        r.cold.write_hold(str(self.directory), self.h)
        supervisor = r.AttemptSupervisor(str(self.directory), lock, self.docker, self.owner, self.cids, lifetime=1)
        time.sleep(2)
        with self.assertRaises(r.Refused): supervisor.close()
        self.stopped()


def certify_fixed():
    if sys.version_info[:3] != (3, 6, 8) or sys.platform != 'linux':
        raise RuntimeError('Python3.6.8 Linux mandatory; zero skips')
    docker = r.cap.Docker()
    identity = r.cap.image_identity(docker, FIXED_REF, r.APP)
    if identity['local_image_id'] != r.IMAGE:
        raise RuntimeError('fixed target local image identity mismatch')
    OUT.mkdir(parents=True, exist_ok=True)
    # Rendering-only placeholder config: never attach certification fixtures to
    # these endpoints or read a production env. attest owns all actual endpoints.
    import re
    text = (ROOT / 'compose.prod.yml').read_text(encoding='utf-8')
    required = set(re.findall(r'\$\{([A-Z0-9_]+):\?', text))
    values = {name: 'owned-certification-placeholder' for name in required}
    values.update(TARGET_PLATFORM='linux/amd64', POSTGRES_USER='fixture_admin', POSTGRES_DB='fixture_only',
                  DATABASE_URL='postgres://fixture:fixture@fixture.invalid/fixture_only',
                  REDIS_URL='redis://fixture.invalid:6379', RABBITMQ_URL='amqp://fixture:fixture@fixture.invalid/',
                  S3_ENDPOINT='http://fixture.invalid', STORAGE_PUBLIC_ENDPOINT='http://fixture.invalid',
                  CONNECTOR_CALLBACK_BASE_URL='http://fixture.invalid', XIANZHI_IMAGE_REFERENCE=FIXED_REF)
    fixture_env = OUT / 'certification-fixture.env'
    fixture_env.write_text('\n'.join(name + '=' + values[name] for name in sorted(values)) + '\n')
    model, policy = r.cap.rendered_policy(docker, str(ROOT / 'compose.prod.yml'), str(fixture_env), FIXED_REF)
    evidence = r.cap.attest(FIXED_REF, r.APP, policy, synthetic=False, evidence_directory=str(OUT), include_history=True)
    if r.cap.verify(evidence, FIXED_REF, r.APP, policy) != r.IMAGE or not r.cap.verify_history(evidence, r.APP):
        raise RuntimeError('fixed packaged full-history certification failed')
    (OUT / 'fixed-packaged-certification.json').write_text(json.dumps({'scope': 'owned-fixed-image-certification-NOT-human-recovery-authorization', 'evidence': evidence}, indent=2, sort_keys=True))


if __name__ == '__main__':
    if '--certify-fixed' in sys.argv:
        sys.argv.remove('--certify-fixed')
        certify_fixed()
    unittest.main(verbosity=2)
