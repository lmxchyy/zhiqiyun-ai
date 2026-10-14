#!/usr/bin/env python3
"""Pure forward controller contracts. Synthetic certificates are NOT certification."""
import ast
import copy
import datetime
import inspect
import json
import os
from pathlib import Path
import re
import sys
import types
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]


def load(name, path):
    module = types.ModuleType(name)
    module.__file__ = str(ROOT / path)
    exec(compile(Path(module.__file__).read_bytes(), module.__file__, 'exec'), module.__dict__)
    return module


f = load('forward_test', 'ops/recovery-forward-switch.py')
legacy = load('forward_behavior_fixture', 'tests/first-upgrade-cold-test.py')
history = load('forward_history_fixture', 'tests/issue203-historical-drain-test.py')


def synthetic_certificate(identity, policy):
    """Structurally complete historical precondition ONLY, never native evidence."""
    evidence = legacy.behavior(f.cap, identity, policy)
    evidence['history'] = {'version': f.cap.HISTORY_VERSION, 'observations': [], 'controls': []}
    for role in f.cap.ROLES:
        for key, factory in (('observations', history.history_item), ('controls', history.control_item)):
            item = factory()
            item['role'] = role
            item['response']['role'] = role
            item['response']['release_sha'] = identity['release_sha']
            evidence['history'][key].append(item)
    for item in evidence['observations']:
        if item['phase'] == 'allowed':
            item['effects'] = copy.deepcopy(item['effects']) + [{'method': 'GET', 'path': '/provider-query', 'bytes': 0}]
            item['response']['details']['provider-query'] = 'observed-get-control'
    return evidence


def fake_identity(app=f.APP, ref=f.REF, image='sha256:' + '7' * 64):
    return {'reference': ref, 'release_sha': app, 'local_image_id': image, 'repo_digests': [ref],
            'platform': {'os': 'linux', 'architecture': 'amd64', 'variant': ''},
            'binary_sha256': {p: 'a' * 64 for p in f.cap.BINARIES}}


class CapabilityTests(unittest.TestCase):
    def setUp(self):
        self.identity = fake_identity()
        self.policy = {'roles': {}}
        self.evidence = synthetic_certificate(self.identity, self.policy)

    def test_full_structural_parity_and_no_native_calls(self):
        with mock.patch.object(f.cap, 'image_identity', return_value=self.identity) as native:
            self.assertEqual(f.cap.verify(self.evidence, f.REF, f.APP, self.policy), self.identity['local_image_id'])
            native.assert_called_once()
        with mock.patch.object(f.cap, 'Docker', side_effect=AssertionError('native forbidden')):
            self.assertEqual(f.verify_prepared_capability(self.evidence, f.REF, f.APP, self.policy, self.identity), self.identity['local_image_id'])

    def test_exact_existing_verifier_guards_remain_discoverable(self):
        original = inspect.getsource(f.cap.verify)
        actual = inspect.getsource(f.verify_prepared_capability)
        # Assert every structural guard below native identity check remains exact.
        original = original[original.index('    observations ='):]
        actual = actual[actual.index('    observations ='):]
        self.assertEqual(original.replace('for r in ROLES for p in', 'for r in ROLES_CAPABILITY for p in'), actual)

    def test_new_capability_freshness_history_digest_binary_tamper(self):
        variants = []
        for key, value in [('synthetic_nonofficial', True), ('version', True), ('nonce', 'x'),
                           ('created_at_unix', 0), ('policy', {}), ('history', None), ('observations', [])]:
            bad = copy.deepcopy(self.evidence); bad[key] = value; variants.append(bad)
        bad = copy.deepcopy(self.evidence); bad['identity']['reference'] = f.OLD_REF; variants.append(bad)
        bad = copy.deepcopy(self.evidence); bad['identity']['binary_sha256'] = {}; variants.append(bad)
        bad = copy.deepcopy(self.evidence); bad['history']['controls'][0]['after']['xz_assets'] = []; variants.append(bad)
        for bad in variants:
            with self.subTest(keys=list(bad)), self.assertRaises(Exception):
                f.verify_prepared_capability(bad, f.REF, f.APP, self.policy, self.identity)

    def test_expired_history_identity_is_not_forward_authority(self):
        bad = copy.deepcopy(self.evidence)
        bad['created_at_unix'], bad['expires_at_unix'] = 1, 3601
        f.verify_prepared_capability(bad, f.REF, f.APP, self.policy, self.identity, historical=True)
        with self.assertRaises(f.Refused):
            f.verify_prepared_capability(bad, f.REF, f.APP, self.policy, self.identity)


class ReuseTests(unittest.TestCase):
    def row(self):
        return [113, 'task_000334', 1, 'succeeded', 3,
                datetime.datetime(2026, 10, 13, 3, 51, 10, 958975, tzinfo=datetime.timezone.utc),
                [{'URL': 'data:image/png;base64,aGVsbG8='}], 'PROCESSING', 'RUNNING', 3]

    def test_native_null_bigint_bool_manifest_status_drift(self):
        expected = f.validate_reuse([self.row()])
        for index, value in [(0, True), (2, '1'), (4, None), (4, 2), (9, 4), (6, []),
                             (6, [{}]), (3, 'processing'), (7, 'FAILED'), (5, None)]:
            bad = self.row(); bad[index] = value
            with self.subTest(index=index), self.assertRaises(f.Refused): f.validate_reuse([bad])
        done = self.row(); done[7:9] = ['SUCCEEDED', 'SUCCEEDED']
        self.assertEqual(f.validate_reuse([done], completed=True), expected)
        with self.assertRaises(f.Refused): f.validate_reuse([done])
        with self.assertRaises(f.Refused): f.validate_reuse([self.row(), self.row()])

    def test_heartbeat_and_transport_history_not_frozen(self):
        self.assertNotIn('last_heartbeat', f.REUSE)
        self.assertNotIn('consumer_inbox', f.REUSE)
        self.assertEqual(f.REUSE_OIDS, (20, 25, 23, 25, 20, 1184, 3802, 25, 25, 20))
        conn = types.SimpleNamespace(status=2, closed=False, process=types.SimpleNamespace(poll=lambda: None), close=lambda: None)
        cursor = f.ReuseCursor(conn)
        for sql in ('UPDATE provider_executions SET status=\'failed\'', 'SELECT 1', f.REUSE + ' FOR UPDATE'):
            with self.assertRaises(f.Refused): cursor.execute(sql)


class SourceAndGuardTests(unittest.TestCase):
    def test_fixed_manifest_python36_and_ordinary_sources_unchanged(self):
        self.assertEqual(f.manifest_identity(), f.MANIFEST_HASH)
        source = Path(f.__file__).read_text(encoding='utf-8')
        try: ast.parse(source, feature_version=(3, 6))
        except TypeError: ast.parse(source)  # actual3.6 CI independently mandatory
        self.assertNotIn('cold.fence(', source)
        self.assertNotIn('cap.verify(', source)
        self.assertNotIn('cap.image_identity(', source)
        import subprocess
        self.assertEqual(subprocess.check_output(['git', 'diff', '--name-only', 'd6593ad98f7fc35e78a4bd6459fb79e7ab7f73f2', '--',
                          'deploy.sh', 'ops/cold-recovery-resume.py', 'ops/first-upgrade-cold.py', 'database', 'backend-go',
                          'compose.prod.yml', 'ops/cold-recovery-anchor.json'], cwd=str(ROOT)), b'')

    def test_protected_snapshot_only_excludes_replaced_pair(self):
        base = {'containers': {r: {'cid': r} for r in f.ROLES + ('postgres', 'migrate')}, 'reuse113': {'generation': 3}}
        variant = copy.deepcopy(base)
        variant['containers'][f.ROLES[0]]['cid'] = 'new'
        self.assertEqual(f.protected_snapshot(base), f.protected_snapshot(variant))
        variant['containers']['migrate']['cid'] = 'foreign'
        self.assertNotEqual(f.protected_snapshot(base), f.protected_snapshot(variant))
        self.assertIn(f.ROLES[0], base['containers'])

    def test_predicted_names_and_phrase_disclose_stop_remove_recreate(self):
        names = f.predicted_names('a' * 32)
        self.assertEqual(set(names), set(f.ROLES))
        candidate = {'hold_sha256': 'b' * 64, 'snapshot': {'source': {'controller_source_sha': 'c' * 40}}}
        phrase = f.phrase(candidate)
        self.assertIn('STOP_REMOVE_RECREATE_PAIR RETAIN_HOLD', phrase)
        self.assertIn('new_digest=' + f.DIGEST, phrase)
        self.assertIn('NO_ROLLBACK SINGLE_USE', phrase)

    def test_uncertain_native_acceptance_never_resends_and_fence_never_global(self):
        guard = object.__new__(f.Guard)
        guard.nonce, guard.deadline, guard.uncertain = 'a' * 32, f.time.monotonic() + 30, False
        hold = {}
        guard.check_live = lambda: None
        guard.check_owner = lambda: hold
        guard.docker = types.SimpleNamespace(text=mock.Mock(side_effect=OSError('accepted but response lost')))
        with mock.patch.object(f.cold, 'write_hold') as write:
            with self.assertRaises(OSError): guard.mutate('start-new', ['start', 'c' * 64], 'c' * 64)
            self.assertTrue(guard.uncertain)
            self.assertEqual(hold['forward_indeterminate_operation']['operation'], 'start-new')
            guard.docker.text.assert_called_once()
            with self.assertRaises(f.Refused): guard.mutate('start-new', ['start', 'c' * 64], 'c' * 64)
            guard.docker.text.assert_called_once()
            write.assert_called_once()

    def test_stale_budget_after_fsync_does_not_issue_start(self):
        guard = object.__new__(f.Guard)
        guard.nonce, guard.deadline, guard.uncertain = 'a' * 32, f.time.monotonic() + 30, False
        guard.check_live = lambda: None
        hold = {}; guard.check_owner = lambda: hold
        guard.docker = types.SimpleNamespace(text=mock.Mock())
        def expire(*args): guard.deadline = f.time.monotonic() - 1
        with mock.patch.object(f.cold, 'write_hold', side_effect=expire):
            with self.assertRaises(f.Refused): guard.mutate('start-new', ['start', 'c' * 64], 'c' * 64)
        guard.docker.text.assert_not_called()

    def test_foreign_lock_refuses_before_marker_or_native_operation(self):
        guard = object.__new__(f.Guard)
        guard.lock = types.SimpleNamespace(check=mock.Mock(side_effect=f.Refused('foreign owner')))
        guard.docker = types.SimpleNamespace(text=mock.Mock())
        with mock.patch.object(f.cold, 'write_hold') as write:
            with self.assertRaises(f.Refused): guard.check_owner()
            write.assert_not_called(); guard.docker.text.assert_not_called()

    def test_journal_new_cid_bound_before_start(self):
        guard = object.__new__(f.Guard)
        guard.new, guard.removed = {}, []
        hold = {'forward_attempt': {}}
        guard.check_owner = lambda: hold
        with mock.patch.object(f.cold, 'write_hold') as write:
            guard.write_attempt(new={f.ROLES[0]: 'c' * 64})
            self.assertEqual(hold['forward_attempt']['new_cids'], guard.new)
            write.assert_called_once()


class HistoricalSessionTests(unittest.TestCase):
    def setUp(self):
        self.env = mock.patch.dict(os.environ, {'RELEASE_TRUST_SECRET': 'synthetic-forward-unit-only'})
        self.env.start(); self.addCleanup(self.env.stop)
        self.hashes = mock.patch.object(f.cold, 'file_hash', return_value='a' * 64)
        self.hashes.start(); self.addCleanup(self.hashes.stop)
        self.original = dict(version=1, state=f.r.STOPPED, owner='original', project=f.PROJECT,
                             release_sha=f.OLD_APP, proof_sha256=f.r.OLD_PROOF, proof_nonce='original-proof',
                             image_ids=[f.OLD_IMAGE, f.r.OLD_IMAGE])
        self.latest = dict(self.original, errors=['prior-completely-fenced'], recovery_spent_nonces=['1' * 32])
        self.anchor = {'services': {role: {'cid': str(i) * 64} for i, role in enumerate(f.ROLES, 2)},
                       'quarantine_records': [], 'history': ['history']}
        identity = fake_identity(f.OLD_APP, f.OLD_REF, f.OLD_IMAGE)
        policy = {'roles': {}}
        snapshot = dict(source={'app_source_sha': f.OLD_APP, 'controller_source_sha': 'c' * 40},
                        database={'quarantine_records': [], 'history': ['history']},
                        compose_sha256='a' * 64, env_sha256='a' * 64, policy=policy, image_identity=identity,
                        containers={role: {'cid': self.anchor['services'][role]['cid']} for role in f.ROLES})
        self.candidate = dict(version=1, purpose=f.r.DOMAIN + '-proof', nonce='2' * 32,
                              not_before=100, expires_at=1000, hold_sha256=f.digest(self.latest), snapshot=snapshot,
                              runtime_capability=synthetic_certificate(identity, policy),
                              operations=['start-existing-api', 'start-existing-worker'])
        self.human = dict(version=1, purpose=f.r.DOMAIN + '-human-approval',
                          candidate_sha256=f.digest(self.candidate), hold_sha256=f.digest(self.latest),
                          nonce=self.candidate['nonce'], operator='unit.operator', confirmation=f.r.phrase(self.candidate),
                          not_before=200, expires_at=500, approved_at=200)
        self.approval = f.cold.signed(self.human)
        cids = [self.anchor['services'][role]['cid'] for role in f.ROLES]
        self.hold = dict(self.original, state=f.r.SUCCESS, recovery_original_hold=f.cold.signed(self.original),
                         recovery_spent_nonces=['1' * 32, '2' * 32], recovery_authorized_cids=cids,
                         recovery_verified_cids=cids, recovery_verified_at=300, recovery_no_env_persistence=True,
                         recovery_intentionally_stopped=['disk-monitor', 'postgres-backup', 'migrate'],
                         recovery_attempt={'nonce': self.candidate['nonce'], 'owner': 'latest-owner',
                                           'operator': self.human['operator'], 'candidate_sha256': f.digest(self.candidate),
                                           'approval_sha256': f.digest(self.approval)})
        self.proof = dict(git_sha=f.OLD_APP, image_reference=f.OLD_REF, proof_nonce='original-proof',
                          cold_recovery_policy={'baseline_identity': {'local_image_id': f.r.OLD_IMAGE}, 'project': f.PROJECT},
                          compose_hash='a' * 64, env_hash='a' * 64)

    def validate(self, hold=None, candidate=None, approval=None):
        return f.historical_session(hold or self.hold, self.proof, candidate or self.candidate,
                                    approval or self.approval, self.anchor)

    def test_failed_then_successful_latest_attempt_keeps_earliest_ancestry(self):
        self.assertNotEqual(self.candidate['hold_sha256'], f.digest(self.original))
        self.assertEqual(self.validate()['candidate_sha256'], f.digest(self.candidate))

    def test_latest_session_hash_nonce_operator_window_cids_tamper(self):
        for field, value in [('nonce', '3' * 32), ('operator', 'foreign'), ('candidate_sha256', 'f' * 64),
                             ('approval_sha256', 'f' * 64)]:
            bad = copy.deepcopy(self.hold); bad['recovery_attempt'][field] = value
            with self.subTest(field=field), self.assertRaises(f.Refused): self.validate(hold=bad)
        for field, value in [('recovery_verified_at', 500), ('recovery_verified_cids', []),
                             ('recovery_spent_nonces', ['1' * 32, '2' * 32, '2' * 32])]:
            bad = dict(self.hold, **{field: value})
            with self.subTest(field=field), self.assertRaises(f.Refused): self.validate(hold=bad)
        bad = dict(self.human, hold_sha256=f.digest(self.original))
        with self.assertRaises(f.Refused): self.validate(approval=f.cold.signed(bad))
        bad = dict(self.hold); bad['recovery_original_hold'] = dict(self.hold['recovery_original_hold'], owner='foreign')
        with self.assertRaises(f.Refused): self.validate(hold=bad)
        bad = dict(self.candidate, hold_sha256='f' * 64)
        with self.assertRaises(f.Refused): self.validate(candidate=bad)


class PendingFenceTests(unittest.TestCase):
    def test_pending_stop_old_still_running_never_resends(self):
        guard = object.__new__(f.Guard)
        guard.armed, guard.uncertain, guard.nonce = True, False, 'a' * 32
        guard.new, guard.removed = {}, []
        guard.old = {f.ROLES[0]: 'b' * 64, f.ROLES[1]: 'c' * 64}
        guard.identity = {'local_image_id': 'new'}
        guard.candidate = {'snapshot': {'containers': {role: {'policy_sha256': 'policy'} for role in f.ROLES}}}
        pending = guard.old[f.ROLES[0]]
        hold = {'forward_indeterminate_operation': {'nonce': guard.nonce, 'operation': 'stop-old', 'target': pending}}
        guard.check_owner = lambda: hold
        guard.lock = types.SimpleNamespace(retained=False, release=mock.Mock())
        def inspect(kind, cid):
            role = next(role for role in f.ROLES if guard.old[role] == cid)
            return {'Id': cid, 'Image': f.OLD_IMAGE, 'Config': {'Labels': {
                    'com.docker.compose.project': f.PROJECT, 'com.docker.compose.service': role}},
                    'State': {'Running': cid == pending, 'Paused': False, 'Restarting': False, 'Status': 'exited'}}
        guard.docker = types.SimpleNamespace(inspect=mock.Mock(side_effect=inspect), text=mock.Mock())
        with mock.patch.object(f.cold, 'write_hold'), mock.patch.object(f, 'policy_projection', return_value='policy'):
            guard.fence()
        self.assertEqual(hold['state'], 'RECOVERY_REQUIRED_UNKNOWN')
        self.assertEqual(hold['forward_indeterminate_operation']['target'], pending)
        self.assertTrue(guard.lock.retained)
        guard.lock.release.assert_not_called()
        guard.docker.text.assert_not_called()
        self.assertNotIn(mock.call('container', pending), guard.docker.inspect.call_args_list)


@unittest.skipUnless(sys.platform == 'linux', 'local Linux process-group/pipe evidence required; CI must run without skips')
class NativeBudgetTests(unittest.TestCase):
    def test_owned_descendant_retaining_pipes_timeout_is_bounded_unknown_one_request(self):
        import tempfile
        import time
        with tempfile.TemporaryDirectory() as directory:
            path = os.path.join(directory, 'requests.jsonl')
            code = ("import subprocess,sys,time,json; "
                    "open(" + repr(path) + ",'a').write(json.dumps(sys.argv[1:])+'\\n'); "
                    "subprocess.Popen([sys.executable,'-c','import time; time.sleep(20)']); time.sleep(20)")
            compile(code, '<owned-descendant>', 'exec')
            docker = f.NativeDocker(); docker.cli = [sys.executable, '-c', code]
            guard = object.__new__(f.Guard)
            guard.nonce, guard.deadline, guard.uncertain = 'a' * 32, time.monotonic() + 1, False
            guard.docker = docker; docker.deadline = guard.deadline
            hold = {}; guard.check_owner = lambda: hold; guard.check_live = lambda: None
            started = time.monotonic()
            import subprocess
            processes = []
            real_popen = subprocess.Popen
            def owned_popen(*args, **kwargs):
                process = real_popen(*args, **kwargs); processes.append(process)
                return process
            with mock.patch.object(f.cold, 'write_hold'), mock.patch.object(f.subprocess, 'Popen', side_effect=owned_popen):
                with self.assertRaises((f.Refused, subprocess.TimeoutExpired)) as failure: guard.mutate('stop-old', ['stop', 'b' * 64], 'b' * 64)
                self.assertTrue(Path(path).exists(), 'request was not issued: ' + repr(failure.exception))
                self.assertTrue(guard.uncertain)
                guard.armed, guard.new, guard.removed = True, {}, []
                guard.old = {f.ROLES[0]: 'b' * 64}
                guard.lock = types.SimpleNamespace(retained=False, release=mock.Mock())
                guard.fence()  # pending exact target has no second inspect/stop
            # Test-only reap of the exact owned local CLI, never Engine settlement.
            for process in processes: process.wait(timeout=.5)
            self.assertLess(time.monotonic() - started, 2)
            self.assertEqual(hold['state'], 'RECOVERY_REQUIRED_UNKNOWN')
            self.assertTrue(guard.lock.retained)
            self.assertEqual(len(Path(path).read_text().splitlines()), 1)

    def final_sql_stall(self, eof=False):
        import subprocess
        import time
        docker = f.NativeDocker()
        read_fd, write_fd = os.pipe()
        docker.parent_fd = read_fd
        docker.deadline = time.monotonic() + (.4 if not eof else 10)
        conn = object.__new__(f.ReadOnlyConnection)
        conn.docker, conn.closed, conn.buffer = docker, False, bytearray()
        conn.process = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(20)'],
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, start_new_session=True)
        os.set_blocking(conn.process.stdin.fileno(), False)
        os.set_blocking(conn.process.stdout.fileno(), False)
        if eof:
            os.close(write_fd); write_fd = None
        started = time.monotonic()
        try:
            with self.assertRaises(f.Refused): conn.exchange(b'read-only-final-observation\n')
        finally:
            conn.close()
            os.close(read_fd)
            if write_fd is not None: os.close(write_fd)
        self.assertLess(time.monotonic() - started, 2)
        # Test-only reap, outside the expired guard budget. Production close
        # never waits past authority merely to reap a killed local CLI.
        conn.process.wait(timeout=.5)
        self.assertIsNotNone(conn.process.poll())

    def test_final_postgresql_frame_deadline(self):
        self.final_sql_stall()

    def test_final_postgresql_frame_parent_eof(self):
        self.final_sql_stall(eof=True)

    def test_native_capture_and_redacted_failure(self):
        docker = f.NativeDocker(); docker.cli = [sys.executable, '-c', "import sys; print('secret'); sys.exit(1)"]
        docker.redactions = ['secret']
        with self.assertRaises(f.Refused) as error: docker.run(['observation'])
        self.assertNotIn('secret', str(error.exception)); self.assertIn('[redacted]', str(error.exception))
        docker.cli = [sys.executable, '-c', "print('x'*1024)"]
        with self.assertRaises(f.Refused): docker.run(['observation'], capture_limit=32)


class NewFixtureShapeTests(unittest.TestCase):
    def test_explicit_fixture_lock_and_protected_parent_before_setup(self):
        source = (ROOT / 'tests/recovery-forward-switch-backend-test.py').read_text(encoding='utf-8')
        self.assertNotIn('ReleaseLock()', source)
        self.assertEqual(source.count('f.r.ReleaseLock(f.DIR)'), 2)
        self.assertIn('OUT.mkdir(mode=0o700, parents=True, exist_ok=True)', source)
        self.assertIn('f.r.protected_path(str(OUT))', source)
        self.assertIn('class ForwardDocker(f.NativeDocker)', source)
        self.assertIn("self.assertEqual(obj['Config']['Image'], MINIO_REF)", source)
        workflow = (ROOT / '.github/workflows/recovery-forward-switch.yml').read_text(encoding='utf-8')
        self.assertNotIn('docker pull --platform linux/amd64 docker.m.daocloud.io', workflow)
        self.assertIn('docker.io/minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e', workflow)
        self.assertIn('git -C "$root/source" rev-parse HEAD', workflow)
        self.assertIn('os.WNOHANG', inspect.getsource(f.execute))


class OwnedCleanupTests(unittest.TestCase):
    def setUp(self):
        self.cleanup = load('forward_cleanup_unit', 'tests/recovery-forward-switch-cleanup.py')

    def test_exact_label_project_required_before_remove(self):
        owner, cid = 'a' * 32, 'b' * 64
        project = 'resume-real-' + owner
        obj = {'Id': cid, 'Config': {'Labels': {self.cleanup.BACKEND_LABEL: owner,
                                                'com.docker.compose.project': project}}}
        docker = types.SimpleNamespace(run=mock.Mock(), text=mock.Mock())
        docker.run.return_value = types.SimpleNamespace(returncode=0, stdout=json.dumps([obj]).encode())
        self.assertEqual(self.cleanup.remove(docker, 'container', cid, self.cleanup.BACKEND_LABEL, owner, project), 'removed')
        docker.text.assert_called_once_with(['rm', '-f', '-v', cid])
        for bad in ('different-owner', 'different-project', 'different-id'):
            modified = copy.deepcopy(obj)
            if bad == 'different-owner': modified['Config']['Labels'][self.cleanup.BACKEND_LABEL] = 'foreign'
            if bad == 'different-project': modified['Config']['Labels']['com.docker.compose.project'] = 'foreign'
            if bad == 'different-id': modified['Id'] = 'c' * 64
            docker.run.return_value.stdout = json.dumps([modified]).encode()
            docker.text.reset_mock()
            with self.assertRaises(RuntimeError): self.cleanup.remove(docker, 'container', cid, self.cleanup.BACKEND_LABEL, owner, project)
            docker.text.assert_not_called()

    def test_mixed_attestation_journals_and_observations(self):
        import tempfile
        with tempfile.TemporaryDirectory() as root:
            out = Path(root); attest = out / 'attest'; attest.mkdir()
            owner = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
            (attest / ('owned-' + owner + '.json')).write_text(json.dumps({'owner': owner, 'resources': []}))
            observation = dict(role='api', phase='allowed', image_identity={}, before={}, after={},
                               effects=[], exit_code=0, stdout='')
            (attest / ('observation-' + owner + '.json')).write_text(json.dumps(observation))
            (attest / 'certification.json').write_text('{}')
            docker = types.SimpleNamespace(text=mock.Mock(return_value=''))
            with mock.patch.object(self.cleanup, 'OUT', out), mock.patch.object(self.cleanup.cap, 'Docker', return_value=docker):
                self.cleanup.attest_cleanup()
                self.assertEqual(docker.text.call_count, 2)
                bad = attest / ('owned-' + 'b' * 32 + '.json'); bad.write_text('{}')
                with self.assertRaisesRegex(RuntimeError, 'unowned'): self.cleanup.attest_cleanup()
                bad.unlink()
                (attest / 'unexpected.json').write_text('{}')
                with self.assertRaisesRegex(RuntimeError, 'unknown'): self.cleanup.attest_cleanup()

    def test_sanitized_export_allowlist_and_failed_gate(self):
        import tempfile
        with tempfile.TemporaryDirectory() as root:
            out = Path(root) / 'evidence'; out.mkdir()
            dest = Path(root) / 'public'; dest.mkdir()
            (out / 'fixture-sanitization.json').write_text('{"status":"passed"}')
            (out / 'native.log').write_text('bounded fixture result')
            (out / 'fixture.env').write_text('private-do-not-export')
            with mock.patch.object(self.cleanup, 'OUT', out):
                self.cleanup.export_sanitized(dest)
                self.assertEqual((dest / 'native.log').read_text(), 'bounded fixture result')
                self.assertFalse((dest / 'fixture.env').exists())
                (out / 'fixture-sanitization.json').write_text('{"status":"failed"}')
                with self.assertRaisesRegex(RuntimeError, 'sanitization'): self.cleanup.export_sanitized(dest)

    def test_unrecorded_resources_refuse_broad_cleanup_and_private_files_removed(self):
        import tempfile
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / ('a' * 32); path.mkdir()
            (path / 'resources.json').write_text(json.dumps({'owner': path.name, 'project': 'resume-real-' + path.name, 'resources': []}))
            (path / 'fixture.env').write_text('private-unit-only')
            prestage = path / 'prestage'; prestage.mkdir()
            (prestage / 'forward-compose-nonce.json').write_text('private-unit-only')
            docker = types.SimpleNamespace(text=mock.Mock(return_value='b' * 64), run=mock.Mock())
            with mock.patch.object(self.cleanup.cap, 'Docker', return_value=docker):
                with self.assertRaisesRegex(RuntimeError, 'unrecorded'): self.cleanup.cleanup_backend(path)
            docker.run.assert_not_called()
            self.assertFalse((path / 'fixture.env').exists())
            self.assertFalse((prestage / 'forward-compose-nonce.json').exists())
            self.assertFalse((path / 'cleanup.json').exists())


if __name__ == '__main__':
    run = unittest.main(verbosity=2, exit=False)
    zero_skip = sys.platform != 'linux' or not run.result.skipped
    sys.exit(0 if run.result.wasSuccessful() and zero_skip else 1)
