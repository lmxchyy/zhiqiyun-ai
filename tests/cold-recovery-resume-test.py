#!/usr/bin/env python3
"""Recovery domain/adversarial mechanism tests; mocks are NOT certification."""
import ast
import copy
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import types
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
path = ROOT / 'ops/cold-recovery-resume.py'
r = types.ModuleType('resume')
r.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), r.__dict__)


def hold():
    return dict(version=1, state=r.STOPPED, release_sha=r.APP, proof_sha256=r.OLD_PROOF,
                project='zhiqiyun-ai-prod', image_ids=[r.IMAGE, r.OLD_IMAGE], owner='old-owner',
                proof_nonce='11111111-1111-1111-1111-111111111111')


def source():
    return dict(controller_source_sha='c' * 40, app_source_sha=r.APP, helper_sha256={'fixed': '1' * 64})


def candidate(h):
    now = int(time.time())
    return dict(version=1, purpose=r.DOMAIN + '-proof', nonce='e' * 32, not_before=now,
                expires_at=now + 800, hold_sha256=r.digest(h),
                snapshot=dict(source=source(), containers={role: {'cid': str(i) * 64} for i, role in enumerate(r.ROLES, 1)}),
                runtime_capability={'unit-only': True}, operations=['start-existing-api', 'start-existing-worker'])


class FakeDocker:
    def __init__(self):
        self.calls = []


class Backend:
    def __init__(self, c):
        self.c, self.docker, self.running = c, FakeDocker(), []
        self.drift_at, self.fail_health, self.fail_capability = None, False, False
        self.samples = 0

    def verify(self, evidence, snapshot):
        if self.fail_capability:
            raise r.Refused('expired/forged capability')

    def snapshot(self, running=()):
        self.samples += 1
        result = copy.deepcopy(self.c['snapshot'])
        if self.drift_at == self.samples:
            result['financial-drift'] = True
        return result

    def healthy(self, evidence):
        if self.fail_health:
            raise r.Refused('runtime/process/readiness timeout')


class FakeSupervisor:
    fail_start = None
    backend = None
    directory = None
    def __init__(self, directory, lock, docker, nonce, cids, lifetime, authority):
        self.directory, self.docker, self.cids = directory, docker, cids
        self.authority = authority
        self.index = 0
        self.closed = False

    def command(self, message):
        assert message == b'S'
        self.docker.calls.append(['start', self.cids[self.index]])
        self.index += 1
        if self.index == self.fail_start:
            raise r.Refused('partial start')

    def close(self, success=False):
        if self.closed:
            return
        self.closed = True
        if success:
            h = r.cold.read_hold(self.directory)
            h.update(state=r.SUCCESS, recovery_verified_cids=self.cids,
                     recovery_intentionally_stopped=['disk-monitor', 'postgres-backup', 'migrate'],
                     recovery_no_env_persistence=True)
            try:
                r.cold.write_hold(self.directory, h)
                return
            except Exception:
                pass
        h = r.cold.read_hold(self.directory)
        h['state'] = r.STOPPED
        r.cold.write_hold(self.directory, h)
        raise r.Refused('fenced')


class ResumeTests(unittest.TestCase):
    def setUp(self):
        self.secret = mock.patch.dict(os.environ, RELEASE_TRUST_SECRET='owned-unit-fixture-only')
        self.secret.start()
        self.addCleanup(self.secret.stop)
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = self.temp.name
        os.mkdir(os.path.join(self.directory, r.cold.HOLD))
        self.h = hold()
        r.cold.write_hold(self.directory, self.h)
        self.c = candidate(self.h)
        self.a = r.approve(self.c, self.h, source(), 'root.operator', r.phrase(self.c))
        self.backend = Backend(self.c)
        self.lock = r.ReleaseLock(self.directory)
        self.addCleanup(self.unlock)
        FakeSupervisor.fail_start = None
        self.identity = mock.patch.object(r, 'source_identity', side_effect=source)
        self.identity.start()
        self.addCleanup(self.identity.stop)

    def unlock(self):
        if os.path.isdir(self.lock.path):
            self.lock.release()

    def execute(self):
        return r.execute(self.c, self.a, self.backend, self.lock, self.directory, FakeSupervisor)

    def test_success_exact_two_existing_cids_keeps_terminal_hold_no_env(self):
        self.execute()
        h = r.cold.read_hold(self.directory)
        self.assertEqual(h['state'], r.SUCCESS)
        self.assertEqual(h['recovery_spent_nonces'], [self.c['nonce']])
        self.assertEqual(h['recovery_original_hold'], r.cold.signed(self.h))
        self.assertEqual(h['owner'], self.h['owner'])
        self.assertTrue(h['recovery_no_env_persistence'])
        self.assertEqual(h['recovery_intentionally_stopped'], ['disk-monitor', 'postgres-backup', 'migrate'])
        self.assertEqual(self.backend.docker.calls, [['start', str(i) * 64] for i in (1, 2)])
        with self.assertRaises(r.Refused):
            self.execute()
        self.assertEqual(len(self.backend.docker.calls), 2)

    def test_hold_states_signatures_and_fixed_identity(self):
        for field, values in {'state': ['ARMED_RECOVERY_REQUIRED', r.STARTING, r.SUCCESS, 'RECOVERY_REQUIRED_UNKNOWN'],
                              'version': [True, 2], 'release_sha': ['c' * 40], 'proof_sha256': ['0' * 64],
                              'project': ['other'], 'image_ids': [[r.IMAGE], ['a', r.IMAGE], [r.IMAGE, 'sha256:' + 'a' * 64]],
                              'proof_nonce': ['', None], 'owner': ['', None]}.items():
            for value in values:
                with self.subTest(field=field, value=value):
                    h = copy.deepcopy(self.h)
                    h[field] = value
                    r.cold.write_hold(self.directory, h)
                    with self.assertRaises(r.Refused):
                        r.read_stopped_hold(self.directory)
        forged = r.cold.signed(self.h)
        forged['owner'] = 'forged'
        r.cold.atomic_json(os.path.join(self.directory, r.cold.HOLD, 'state.json'), forged)
        with self.assertRaises(r.Refused):
            r.read_stopped_hold(self.directory)

    def test_approval_domain_forgery_expiry_bindings_and_types_before_mutation(self):
        a = r.authenticate(self.a)
        cases = [('purpose', 'quarantine-enrollment'), ('candidate_sha256', '0' * 64),
                 ('hold_sha256', '0' * 64), ('nonce', 'f' * 32), ('operator', ''),
                 ('confirmation', 'yes'), ('version', True), ('approved_at', True),
                 ('not_before', int(time.time()) + 100), ('expires_at', int(time.time()) - 1)]
        for field, value in cases:
            with self.subTest(field=field):
                mutated = dict(a)
                mutated[field] = value
                self.a = r.cold.signed(mutated)
                with self.assertRaises(r.Refused):
                    self.execute()
                self.assertEqual(self.backend.docker.calls, [])
                self.assertEqual(r.cold.read_hold(self.directory), self.h)
        self.a = dict(self.a, signature='0' * 64)
        with self.assertRaises(r.Refused):
            self.execute()

    def test_explicit_operator_phrase_no_auto_approval(self):
        for operator, phrase in [('', r.phrase(self.c)), ('root.operator', 'yes'), ('root.operator', r.phrase(self.c).replace(r.APP, 'c' * 40))]:
            with self.assertRaises(r.Refused):
                r.approve(self.c, self.h, source(), operator, phrase)

    def test_candidate_separates_app_control_and_strict_schema(self):
        for mutation in ('app', 'controller', 'source', 'hold', 'purpose', 'bool', 'generic', 'expired', 'operations', 'nonce'):
            with self.subTest(mutation=mutation):
                c = copy.deepcopy(self.c)
                if mutation == 'app': c['snapshot']['source']['app_source_sha'] = 'c' * 40
                if mutation == 'controller': c['snapshot']['source']['controller_source_sha'] = r.APP
                if mutation == 'source': c['snapshot']['source']['helper_sha256'] = {}
                if mutation == 'hold': c['hold_sha256'] = '0' * 64
                if mutation == 'purpose': c['purpose'] = 'prestage'
                if mutation == 'bool': c['version'] = True
                if mutation == 'generic': c['git_sha'] = r.APP
                if mutation == 'expired': c['expires_at'] = 0
                if mutation == 'operations': c['operations'].append('migrate')
                if mutation == 'nonce': c['nonce'] = 'replay'
                with self.assertRaises(r.Refused):
                    r.validate_candidate(c, self.h, source())

    def test_replay_concurrency_foreign_owner_fail_closed(self):
        with self.assertRaises(FileExistsError):
            r.ReleaseLock(self.directory)
        with open(os.path.join(self.lock.path, 'owner_token'), 'w') as stream:
            stream.write('foreign')
        with self.assertRaises(r.Refused):
            self.lock.release()
        with open(os.path.join(self.lock.path, 'owner_token'), 'w') as stream:
            stream.write(self.lock.owner)
        h = dict(self.h, recovery_spent_nonces=[self.c['nonce']])
        with self.assertRaises(r.Refused):
            r.validate_candidate(self.c, h, source())

    def test_snapshot_drift_each_start_and_post_health_fences_spent_nonce(self):
        for sample in (1, 2, 3, 4):
            with self.subTest(sample=sample):
                r.cold.write_hold(self.directory, self.h)
                self.backend = Backend(self.c)
                self.backend.drift_at = sample
                with self.assertRaises(r.Refused):
                    self.execute()
                h = r.cold.read_hold(self.directory)
                self.assertEqual(h['state'], r.STOPPED)
                if sample != 1:
                    self.assertEqual(h['recovery_spent_nonces'], [self.c['nonce']])
                self.assertLessEqual(len(self.backend.docker.calls), 2)

    def test_partial_start_runtime_failure_and_capability_failure(self):
        for failure in ('first', 'second', 'health', 'capability'):
            with self.subTest(failure=failure):
                r.cold.write_hold(self.directory, self.h)
                self.backend = Backend(self.c)
                FakeSupervisor.fail_start = {'first': 1, 'second': 2}.get(failure)
                self.backend.fail_health = failure == 'health'
                self.backend.fail_capability = failure == 'capability'
                with self.assertRaises(r.Refused): self.execute()
                self.assertEqual(r.cold.read_hold(self.directory)['state'], r.STOPPED)
                if failure == 'capability': self.assertEqual(self.backend.docker.calls, [])

    def test_durable_success_write_failure_fences(self):
        original = r.cold.write_hold
        def write(directory, h):
            if h['state'] == r.SUCCESS:
                raise OSError('audit storage failed')
            original(directory, h)
        with mock.patch.object(r.cold, 'write_hold', side_effect=write):
            with self.assertRaises(r.Refused): self.execute()
        self.assertEqual(r.cold.read_hold(self.directory)['state'], r.STOPPED)
        self.assertEqual(r.cold.read_hold(self.directory)['recovery_spent_nonces'], [self.c['nonce']])

    def test_strict_json_duplicate_nonfinite(self):
        for text in ('{"purpose":"a","purpose":"b"}', '{"expiry":NaN}'):
            path = os.path.join(self.directory, 'bad.json')
            with open(path, 'w') as stream: stream.write(text)
            with self.assertRaises(r.Refused): r.read(path)

    def test_readonly_domain_rejects_writes_locks_and_ordinary_descriptors(self):
        class Connection:
            closed = False
            def close(self): self.closed = True
        forbidden = ['COMMIT', 'BEGIN ISOLATION LEVEL REPEATABLE READ', 'DELETE FROM provider_execution_quarantine',
                     'SELECT 1', r.QUARANTINE + ' FOR UPDATE', 'INSERT INTO provider_execution_quarantine VALUES (1)',
                     'UPDATE xz_wallet_ledger SET points=0', 'SELECT pg_sleep(10)']
        for query in forbidden:
            conn = Connection()
            with self.assertRaises(r.Refused): r.ReadOnlyCursor(conn).execute(query)
            self.assertTrue(conn.closed)
        self.assertEqual(len(r.transport.QUERY_TYPES), 103)
        self.assertEqual(len(r.DESCRIPTORS), 9)

    def test_native_quarantine_null_and_oid_replacement_rejected(self):
        expected = r.DESCRIPTORS[r.QUARANTINE]
        fields = r.QUARANTINE[len('SELECT '):].split(' FROM ')[0].split(',')
        anchor = r.read(str(ROOT / 'ops/cold-recovery-anchor.json'))
        cells = [None if anchor['quarantine_records'][0][k] is None else json.dumps(anchor['quarantine_records'][0][k]) for k in fields]
        frame = dict(token='owned', count=1, columns=11, types=list(expected), rows=[cells])
        rows = r.transport.decode_frame(json.dumps(frame).encode(), 'owned', expected)
        self.assertIsNone(rows[0][3])
        self.assertIs(type(rows[0][0]), int)
        self.assertIs(type(rows[0][10]), datetime.datetime)
        for generation in ('"NULL"', 'true', '"2"'):
            bad = copy.deepcopy(frame)
            bad['rows'][0][3] = generation
            with self.assertRaises(r.transport.TransportError): r.transport.decode_frame(json.dumps(bad).encode(), 'owned', expected)
        frame['types'][3] = 25
        with self.assertRaises(r.transport.TransportError): r.transport.decode_frame(json.dumps(frame).encode(), 'owned', expected)

    def observer_fixture(self):
        anchor = r.read(str(ROOT / 'ops/cold-recovery-anchor.json'))
        objects = {}
        for role, a in anchor['services'].items():
            objects[a['cid']] = dict(Id=a['cid'], Image=a['image_id'],
                Config={'Image': a['image_reference'], 'Labels': {'com.docker.compose.project': self.h['project'],
                        'com.docker.compose.service': role}, 'Env': [], 'Cmd': ['/app/xianzhi-api' if role == r.ROLES[0] else '/app/smartvideo-worker'],
                        'Entrypoint': [], 'User': ''},
                HostConfig={'RestartPolicy': {'Name': a['restart']}}, Mounts=[], Path='owned-policy', Args=[],
                NetworkSettings={'Networks': {'owned': {}}}, RestartCount=0,
                State={'Running': role in r.cold.INFRASTRUCTURE, 'Restarting': False, 'Paused': False,
                       'Status': 'running' if role in r.cold.INFRASTRUCTURE else 'exited', 'ExitCode': 0,
                       'StartedAt': a['started_at'], 'Health': {'Status': 'healthy'}})
        class Docker:
            def text(self, args):
                assert args == ['ps', '-aq', '--no-trunc']
                return '\n'.join(objects)
            def inspect(self, kind, identity):
                if kind == 'image': return {'Config': {'Env': [], 'User': ''}}
                return copy.deepcopy(objects[identity])
        return anchor, objects, Docker()

    def test_production_observe_wrong_old_cid_unclassified_config_and_unhealthy_infra(self):
        anchor, objects, docker = self.observer_fixture()
        work = os.path.join(self.directory, r.cold.HOLD)
        with mock.patch.object(r, 'WORK', work):
            self.assertEqual(set(r.observe(docker, anchor)), set(anchor['services']))
            cases = ('wrong-cid', 'old-cid', 'old-external-owner', 'unclassified-owner', 'wrong-target',
                     'restart-drift', 'unhealthy-infra', 'replaced-infra-epoch', 'stopped-infra')
            baseline = copy.deepcopy(objects)
            for change in cases:
                objects.clear(); objects.update(copy.deepcopy(baseline))
                api = anchor['services'][r.ROLES[0]]['cid']
                pg = anchor['services']['postgres']['cid']
                if change == 'wrong-cid': objects[api]['Id'] = 'f' * 64
                if change == 'old-cid': objects[api]['Image'] = r.OLD_IMAGE
                if change == 'old-external-owner':
                    objects['e' * 64] = copy.deepcopy(objects[api])
                    objects['e' * 64]['Image'] = r.OLD_IMAGE
                    objects['e' * 64]['Config']['Labels']['com.docker.compose.project'] = 'foreign'
                if change == 'unclassified-owner':
                    objects['e' * 64] = copy.deepcopy(objects[api])
                    objects['e' * 64]['Config']['Labels']['com.docker.compose.service'] = 'unclassified'
                if change == 'wrong-target': objects[api]['Config']['Image'] = 'arbitrary:tag'
                if change == 'restart-drift': objects[api]['HostConfig']['RestartPolicy']['Name'] = 'always'
                if change == 'unhealthy-infra': objects[pg]['State']['Health']['Status'] = 'unhealthy'
                if change == 'replaced-infra-epoch': objects[pg]['State']['StartedAt'] = 'changed'
                if change == 'stopped-infra': objects[pg]['State']['Running'] = False
                with self.subTest(change=change):
                    with self.assertRaises(r.Refused): r.observe(docker, anchor)

    def test_production_target_config_validator_command_env_user_and_mount_drift(self):
        anchor, objects, docker = self.observer_fixture()
        model = {'services': {role: {'environment': {}, 'volumes': []} for role in r.ROLES}}
        policy = {'roles': {role: {'command': objects[anchor['services'][role]['cid']]['Config']['Cmd'],
                                  'entrypoint': [], 'user': ''} for role in r.ROLES}}
        containers = {role: {'cid': anchor['services'][role]['cid']} for role in r.ROLES}
        r.verify_container_config(docker, model, containers, policy)
        baseline = copy.deepcopy(objects)
        for change in ('command', 'entrypoint', 'env', 'user', 'mount'):
            objects.clear(); objects.update(copy.deepcopy(baseline))
            obj = objects[containers[r.ROLES[0]]['cid']]
            if change == 'command': obj['Config']['Cmd'] = ['sh', '-c', 'other']
            if change == 'entrypoint': obj['Config']['Entrypoint'] = ['sh']
            if change == 'env': obj['Config']['Env'] = ['DATABASE_URL=changed']
            if change == 'user': obj['Config']['User'] = 'foreign'
            if change == 'mount': obj['Mounts'] = [{'Destination': '/app', 'Type': 'bind', 'Source': '/replacement', 'RW': True}]
            with self.subTest(change=change):
                with self.assertRaises(r.Refused): r.verify_container_config(docker, model, containers, policy)

    def test_actual_capability_validator_rejects_missing_expired_synthetic_forged(self):
        now = int(time.time())
        for evidence in (None, {}, {'version': r.cap.VERSION, 'synthetic_nonofficial': True},
                         {'version': r.cap.VERSION, 'synthetic_nonofficial': False, 'created_at_unix': now - 3601, 'expires_at_unix': now - 1}):
            with self.assertRaises(r.Refused): r.cap.verify(evidence, r.REF, r.APP, {})
        evidence = {'version': r.cap.VERSION, 'synthetic_nonofficial': False, 'created_at_unix': now,
                    'expires_at_unix': now + 3600, 'nonce': '11111111-1111-1111-1111-111111111111', 'policy': {}, 'identity': {'forged': True}}
        with mock.patch.object(r.cap, 'image_identity', return_value={'actual': True}):
            with self.assertRaises(r.Refused): r.cap.verify(evidence, r.REF, r.APP, {})

    def test_frozen_original_files_and_python36_ast(self):
        for name, expected in r.FROZEN.items():
            self.assertEqual(hashlib.sha256((ROOT / name).read_bytes()).hexdigest(), expected)
        if sys.version_info >= (3, 8):
            ast.parse(path.read_text(encoding='utf-8'), feature_version=(3, 6))
        for name in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh', 'ops/first-upgrade-cold.py',
                     'ops/quarantine-psql-transport.py', 'tests/issue203-control-plane-test.py'):
            current = (ROOT / name).read_bytes()
            base = subprocess.check_output(['git', 'show', r.APP + ':' + name], cwd=str(ROOT))
            self.assertEqual(current, base)


if __name__ == '__main__':
    unittest.main(verbosity=2)
