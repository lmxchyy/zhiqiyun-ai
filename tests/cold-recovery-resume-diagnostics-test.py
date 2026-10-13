#!/usr/bin/env python3
"""Actual prepare guards/entry with stdlib fixtures only; NOT certification."""
import ast
import copy
from contextlib import ExitStack
import io
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import threading
import time
import types
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
PATH = ROOT / 'ops/cold-recovery-resume.py'
r = types.ModuleType('diagnostic_resume')
r.__file__ = str(PATH)
exec(compile(PATH.read_bytes(), str(PATH), 'exec'), r.__dict__)
d = r.prepare_diagnostics
ACTUAL_VERIFY = r.cap.verify
GENERIC = 'COLD_RECOVERY_RESUME_REFUSED: hold retained; review authenticated state and lock\n'
CANARY = 'SECRET_DB_PASSWORD_ENV_ROW_PRIVATE_PATH_CANARY'


class NoString(Exception):
    def __str__(self):
        raise AssertionError('exception __str__ called')


def diagnostic():
    modules = (r, r.cap, r.cold, r.transport, r.runtime)
    sources = [(m.__dict__, m.__file__, 'ops/' + os.path.basename(m.__file__)) for m in modules]
    return d.Diagnostic(str(ROOT), sources, r.Refused, r.transport.TransportError, r.runtime.GateError)


def report_call(function):
    try:
        function()
    except BaseException as error:
        return diagnostic().report(error)
    raise AssertionError('fixture did not fail')


class PrepareDiagnosticsTests(unittest.TestCase):
    def setUp(self):
        self.stack = ExitStack()
        self.addCleanup(self.stack.close)
        self.events = []
        self.hold = {'proof_nonce': 'opaque', 'project': r.PROJECT}
        self.snapshot = {
            'source': {'controller_source_sha': 'c' * 40},
            'containers': {role: {'policy_sha256': str(i) * 64} for i, role in enumerate(r.ROLES)},
            'database': {'quarantine_records': [{'password': CANARY}], 'history': [CANARY], 'migrations': [[CANARY]]},
            'policy': {'private': CANARY}, 'postgres': {'password': CANARY},
            'compose_sha256': '1' * 64, 'env_sha256': '2' * 64,
            'model_sha256': '3' * 64, 'image_identity': {'private_path': CANARY}}
        self.post = copy.deepcopy(self.snapshot)
        self.evidence = {'opaque': CANARY}
        self.published = None
        self.lock_failure = None
        self.sign_failure = None
        self.publish_failure = None
        self.snapshot_failure = None
        test = self

        class Lock:
            path = 'unit-lock'
            def __init__(self): test.events.append('lock')
            def release(self):
                test.events.append('release')
                if test.lock_failure is not None:
                    raise test.lock_failure

        class Backend(r.Backend):
            def __init__(self):
                test.events.append('backend')
                self.samples = 0
            def snapshot(self):
                self.samples += 1
                test.events.append('snapshot' + str(self.samples))
                if self.samples == 2 and test.snapshot_failure is not None:
                    raise test.snapshot_failure
                return test.snapshot if self.samples == 1 else test.post

        historical = {'git_sha': r.APP, 'image_reference': r.REF,
                      'proof_nonce': self.hold['proof_nonce'], 'compose_hash': 'compose', 'env_hash': 'env',
                      'cold_recovery_policy': {'baseline_identity': {'local_image_id': r.OLD_IMAGE},
                                               'project': self.hold['project']}}
        self.hold_reads = 0
        def stopped_hold():
            self.hold_reads += 1
            self.events.append('hold' + str(self.hold_reads))
            return self.hold if self.hold_reads == 1 else self.post_hold
        self.post_hold = self.hold
        def sign(candidate):
            self.events.append('sign')
            if self.sign_failure is not None:
                raise self.sign_failure
            return dict(candidate, signature='unit')
        def publish(path, value):
            self.events.append('publish')
            if self.publish_failure is not None:
                raise self.publish_failure
            self.assertEqual(path, r.CANDIDATE)
            self.published = value
        def attest(*args, **kwargs):
            self.events.append('attest')
            self.assertEqual(args, (r.REF, r.APP, self.snapshot['policy']))
            self.assertFalse(kwargs['synthetic'])
            self.assertTrue(kwargs['include_history'])
            return self.evidence
        def verify(*args):
            self.events.append('verify')
            return r.IMAGE
        def history(*args):
            self.events.append('verify_history')
            return True
        self.patch(r.sys, 'platform', 'linux')
        self.patch(r.os, 'geteuid', lambda: 0, create=True)
        self.patch(r.os, 'chdir', lambda _: None)
        self.stack.enter_context(mock.patch.dict(os.environ, {'RELEASE_TRUST_SECRET': '', 'DB_CANARY': CANARY}))
        self.patch(r, 'protected_path', lambda path, secret=False: path)
        self.patch(r, 'source_identity', lambda: self.events.append('source') or self.snapshot['source'])
        self.patch(r, 'read_stopped_hold', stopped_hold)
        self.patch(r, 'read', lambda _: historical)
        self.patch(r, 'authenticate', lambda value: value)
        self.patch(r.cold, 'file_hash', lambda path: {r.PROOF_PATH: r.OLD_PROOF, r.COMPOSE: 'compose', r.ENV: 'env'}[path])
        self.patch(r, 'ReleaseLock', Lock)
        self.patch(r, 'Backend', Backend)
        self.patch(r.cap, 'attest', attest)
        self.patch(r.cap, 'verify', verify)
        self.patch(r.cap, 'verify_history', history)
        self.patch(r.cold, 'signed', sign)
        self.patch(r.cold, 'atomic_json', publish)
        self.patch(r.os.path, 'exists', lambda _: True)
        self.persist = self.patch(d.Diagnostic, 'persist', mock.Mock())

    def patch(self, obj, name, value, **kwargs):
        return self.stack.enter_context(mock.patch.object(obj, name, value, **kwargs))

    def run_entry(self, args=('prepare',), failed=True):
        out, err = io.StringIO(), io.StringIO()
        with mock.patch.object(r.sys, 'argv', ['controller'] + list(args)), \
                mock.patch.object(r.sys, 'stdout', out), mock.patch.object(r.sys, 'stderr', err):
            if failed:
                with self.assertRaises(SystemExit) as caught:
                    r.entry()
                self.assertEqual(caught.exception.code, 1)
            else:
                r.entry()
        text = err.getvalue()
        self.assertNotIn(CANARY, text)
        if not failed:
            self.assertEqual(text, '')
            return out.getvalue()
        self.assertTrue(text.startswith(GENERIC))
        lines = text.splitlines()
        if len(lines) == 1:
            return None
        self.assertEqual(len(lines), 2)
        report = json.loads(lines[1].split(': ', 1)[1])
        self.assertEqual(set(report), {'reason_code', 'check_name', 'stage', 'source_file', 'function', 'line',
                                      'error_type', 'source_frames', 'summaries', 'exception_chain'})
        self.assertTrue(all(type(value) in (bool, int) for value in report['summaries'].values()))
        self.assertNotIn(str(ROOT), text)
        return report

    def test_actual_guard_caller_not_require_and_static_unique_line(self):
        report = self.run_entry(('prepare', '--operator', CANARY))
        line = next(i for i, text in enumerate(PATH.read_text().splitlines(), 1)
                    if "require(not args.operator and not args.confirmation, 'prepare cannot approve')" in text)
        self.assertEqual(report['function'], 'main')
        self.assertEqual(report['line'], line)
        self.assertEqual(report['reason_code'], 'CHECK_AT_SOURCE')
        self.assertEqual(report['check_name'], 'ops/cold-recovery-resume.py:main:L' + str(line))
        self.assertEqual(self.events[-1], 'release')
        self.assertNotIn('backend', self.events)

    def test_post_snapshot_mismatch_preserves_hold_shortcircuit(self):
        self.post['containers'][r.ROLES[0]]['policy_sha256'] = 'd' * 64
        report = self.run_entry()
        self.assertEqual(report['reason_code'], 'POST_SNAPSHOT_MISMATCH')
        self.assertTrue(report['check_name'].endswith(':snapshot_equal'))
        self.assertEqual(report['stage'], 'snapshot_equality')
        self.assertFalse(report['summaries']['snapshot_equal'])
        self.assertFalse(report['summaries'][r.ROLES[0] + '_policy_equal'])
        self.assertTrue(report['summaries']['database_equal'])
        self.assertEqual(report['summaries']['quarantine_records_initial_count'], 1)
        self.assertEqual(self.hold_reads, 1)
        self.assertNotIn('hold_equal', report['summaries'])
        self.assertNotIn('sign', self.events)
        self.assertEqual(self.events[-1], 'release')

    def test_hold_mismatch_distinct_check(self):
        self.post_hold = dict(self.hold, changed=CANARY)
        report = self.run_entry()
        self.assertEqual(report['reason_code'], 'STOPPED_HOLD_MISMATCH')
        self.assertTrue(report['check_name'].endswith(':hold_equal'))
        self.assertEqual(report['stage'], 'hold_equality')
        self.assertTrue(report['summaries']['snapshot_equal'])
        self.assertFalse(report['summaries']['hold_equal'])
        self.assertEqual(self.hold_reads, 2)
        self.assertNotIn('sign', self.events)

    def test_actual_probe_guard_and_no_false_attest_return(self):
        class Docker:
            def inspect(self, kind, ref):
                return {'Id': r.IMAGE, 'Config': {'Entrypoint': [CANARY]}}
        def attest(*args, **kwargs):
            return r.cap.image_identity(Docker(), r.REF, r.APP)
        self.patch(r.cap, 'attest', attest)
        report = self.run_entry()
        self.assertEqual(report['source_file'], 'ops/verify-image-quarantine-capability.py')
        self.assertEqual(report['function'], 'image_identity')
        self.assertEqual(report['stage'], 'attest')
        self.assertEqual(report['error_type'], 'Refused')
        self.assertNotIn('attest_returned', report['summaries'])
        self.assertNotIn('snapshot2', self.events)

    def test_actual_cap_verify_guard_attest_return_but_not_verify_return(self):
        # The original function, not a forensic-report model.
        self.patch(r.cap, 'verify', ACTUAL_VERIFY)
        self.evidence = None
        report = self.run_entry()
        self.assertEqual(report['function'], 'verify')
        self.assertEqual(report['stage'], 'capability_verify')
        self.assertEqual(report['source_file'], 'ops/verify-image-quarantine-capability.py')
        self.assertTrue(report['summaries']['attest_returned'])
        self.assertNotIn('verify_returned', report['summaries'])
        self.assertNotIn('snapshot2', self.events)

    def test_post_snapshot_io_error_not_invented_guard(self):
        self.snapshot_failure = PermissionError(CANARY, CANARY, '/' + CANARY)
        report = self.run_entry()
        self.assertEqual(report['stage'], 'post_snapshot')
        self.assertEqual(report['function'], 'main')
        self.assertEqual(report['error_type'], 'PermissionError')
        self.assertEqual(report['reason_code'], 'OPERATION_AT_SOURCE')
        self.assertTrue(report['summaries']['attest_returned'])
        self.assertTrue(report['summaries']['verify_returned'])
        self.assertNotIn('post_snapshot_returned', report['summaries'])
        self.assertEqual(self.hold_reads, 1)

    def test_signing_and_publication_io_type_and_location(self):
        for operation in ('sign', 'publish'):
            with self.subTest(operation=operation):
                self.events[:] = []
                self.hold_reads = 0
                self.sign_failure = OSError(CANARY) if operation == 'sign' else None
                self.publish_failure = OSError(CANARY) if operation == 'publish' else None
                report = self.run_entry()
                self.assertEqual(report['stage'], 'candidate_signing' if operation == 'sign' else 'candidate_publication')
                self.assertEqual(report['function'], 'main')
                self.assertEqual(report['error_type'], 'OSError')
                self.assertEqual(report['reason_code'], 'OPERATION_AT_SOURCE')
                self.assertEqual(self.events[-1], 'release')
                self.assertIsNone(self.published)

    def test_finally_replacement_retains_original_guard_context_and_stage(self):
        self.post = dict(self.post, different=CANARY)
        self.lock_failure = OSError(CANARY)
        report = self.run_entry()
        self.assertEqual(report['stage'], 'lock_release')
        self.assertEqual(report['error_type'], 'OSError')
        original = report['exception_chain'][1]
        self.assertEqual(original['relation'], 'context')
        self.assertEqual(original['function'], 'main')
        self.assertEqual(original['error_type'], 'Refused')
        self.assertEqual(original['stage'], 'snapshot_equality')
        self.assertEqual(self.hold_reads, 1)

    def test_write_failure_does_not_mask_refusal_or_cleanup(self):
        self.persist.side_effect = NoString(CANARY)
        self.post = {}
        report = self.run_entry()
        self.assertEqual(report['reason_code'], 'POST_SNAPSHOT_MISMATCH')
        self.assertEqual(self.events[-1], 'release')
        self.assertEqual(self.persist.call_count, 1)

    def test_observer_failure_never_changes_pass_or_refusal(self):
        self.patch(d.Diagnostic, 'observe', mock.Mock(side_effect=NoString(CANARY)))
        self.patch(d.Diagnostic, 'snapshots', mock.Mock(side_effect=NoString(CANARY)))
        self.run_entry(failed=False)
        self.assertIsNotNone(self.published)
        self.assertEqual(self.events[-1], 'release')
        self.events[:] = []
        self.hold_reads = 0
        self.post = {}
        report = self.run_entry()
        self.assertEqual(report['error_type'], 'Refused')
        self.assertEqual(self.events[-1], 'release')

    def test_pass_operation_sequence_candidate_phrase_and_no_diagnostic_write(self):
        output = self.run_entry(failed=False)
        self.assertEqual(self.events, ['source', 'hold1', 'lock', 'backend', 'snapshot1', 'attest',
                                      'verify', 'verify_history', 'snapshot2', 'hold2', 'sign', 'publish', 'release'])
        self.persist.assert_not_called()
        unsigned = dict(self.published)
        unsigned.pop('signature')
        self.assertEqual(output, r.phrase(unsigned) + '\n')
        self.assertEqual(unsigned['expires_at'] - unsigned['not_before'], 900)
        self.assertIs(unsigned['snapshot'], self.snapshot)
        self.assertIs(unsigned['runtime_capability'], self.evidence)
        self.assertEqual(set(unsigned), {'version', 'purpose', 'nonce', 'not_before', 'expires_at',
                                         'hold_sha256', 'snapshot', 'runtime_capability', 'operations'})
        self.assertTrue(r._prepare_diagnostic.summaries['verify_returned'])

    def test_approve_execute_inactive_even_if_sysargs_contain_prepare(self):
        for action in ('approve', 'execute'):
            with self.subTest(action=action):
                self.patch(r, 'protected_path', mock.Mock(side_effect=NoString(CANARY)))
                self.assertIsNone(self.run_entry((action, '--confirmation', 'prepare')))
                self.assertIsNone(r._prepare_diagnostic)
                self.assertFalse(r._prepare_active)
        self.persist.assert_not_called()

    def test_secret_exception_and_unknown_type_never_stringified(self):
        self.sign_failure = NoString(CANARY, {'password': CANARY})
        report = self.run_entry()
        self.assertEqual(report['error_type'], 'UNKNOWN')
        self.assertEqual(report['stage'], 'candidate_signing')
        text = json.dumps(report)
        self.assertNotIn('password', text)
        self.assertNotIn(CANARY, text)

    def test_observer_report_failure_uses_safe_fallback_preserves_cleanup(self):
        self.patch(d.Diagnostic, 'report', mock.Mock(side_effect=NoString(CANARY)))
        self.post = {}
        out = io.StringIO()
        with mock.patch.object(r.sys, 'argv', ['controller', 'prepare']), mock.patch.object(r.sys, 'stderr', out):
            with self.assertRaises(SystemExit) as caught:
                r.entry()
        self.assertEqual(caught.exception.code, 1)
        self.assertEqual(self.events[-1], 'release')
        self.assertNotIn(CANARY, out.getvalue())
        self.assertIn('diagnostic_unavailable', out.getvalue())


class MetadataAndFileTests(unittest.TestCase):
    def test_readonly_wrapper_preserves_actual_guard_caller_chain(self):
        class Connection:
            closed = False
            def close(self): self.closed = True
        conn = Connection()
        report = report_call(lambda: r.ReadOnlyCursor(conn).execute(CANARY))
        self.assertTrue(conn.closed)
        self.assertEqual(report['function'], 'execute')
        self.assertEqual(report['reason_code'], 'REFUSAL_AT_SOURCE')
        original = report['exception_chain'][1]
        self.assertEqual(original['function'], 'execute')
        self.assertEqual(original['reason_code'], 'CHECK_AT_SOURCE')
        self.assertNotEqual(original['line'], report['line'])
        self.assertNotIn(CANARY, json.dumps(report))

    def test_transport_block_caller_and_chained_close_replacement(self):
        report = report_call(lambda: r.transport.decode_frame(b'{}', CANARY, (20,)))
        self.assertEqual(report['source_file'], 'ops/quarantine-psql-transport.py')
        self.assertEqual(report['function'], 'decode_frame')
        self.assertEqual(report['reason_code'], 'CHECK_AT_SOURCE')
        class Connection:
            closed = False
            def close(self): raise PermissionError(CANARY)
        report = report_call(lambda: r.ReadOnlyCursor(Connection()).execute(CANARY))
        self.assertEqual(report['error_type'], 'PermissionError')
        self.assertEqual(report['reason_code'], 'OPERATION_AT_SOURCE')
        self.assertEqual(report['exception_chain'][1]['reason_code'], 'CHECK_AT_SOURCE')

    def test_explicit_cause_context_suppression_cycles_and_untrusted_paths(self):
        def fail():
            try:
                r.authenticate(None)
            except r.Refused as prior:
                try:
                    raise NoString(CANARY) from prior
                except NoString:
                    raise
        # Install an adversarial cycle only after unwinding. Older Python's raise
        # context-linking itself can loop if handed a cycle during re-raise.
        captured = None
        try:
            fail()
        except NoString as error:
            captured = error
        # Leave ALL active except blocks before creating an artificial cycle:
        # Python3.6 can loop linking an internal AttributeError to that cycle.
        captured.__cause__.__context__ = captured
        report = diagnostic().report(captured)
        self.assertEqual(report['error_type'], 'UNKNOWN')
        self.assertEqual(report['source_file'], 'UNKNOWN')
        self.assertEqual(len(report['exception_chain']), 2)
        self.assertEqual(report['exception_chain'][1]['relation'], 'cause')
        self.assertEqual(report['exception_chain'][1]['function'], 'authenticate')
        self.assertNotIn(CANARY, json.dumps(report))
        def suppressed():
            try:
                r.authenticate(None)
            except r.Refused:
                raise NoString(CANARY) from None
        report = report_call(suppressed)
        self.assertEqual(report['exception_chain'][1]['relation'], 'context')
        self.assertNotIn(str(ROOT), json.dumps(report))

    def test_actual_runtime_barrier_gate_type_and_location(self):
        report = report_call(lambda: r.runtime.verify_quarantine_barrier_read_only(
            'unit-compose', 'unit-env', execute=lambda args: '0'))
        self.assertEqual(report['error_type'], 'GateError')
        self.assertEqual(report['reason_code'], 'REFUSAL_AT_SOURCE')
        self.assertEqual(report['source_file'], 'ops/verify-release-runtime.py')
        self.assertEqual(report['function'], 'verify_quarantine_barrier_read_only')

    def test_untrusted_filename_cannot_impersonate_trusted_namespace(self):
        namespace = {'require': r.require, 'canary': CANARY}
        exec(compile('def impostor():\n require(False, canary)\n', r.__file__, 'exec'), namespace)
        report = report_call(namespace['impostor'])
        self.assertEqual(report['check_name'], 'UNKNOWN')
        self.assertNotIn(CANARY, json.dumps(report))

    def test_writer_fixed_namespace_private_exclusive_nofollow_no_repairs(self):
        observer = diagnostic()
        info = types.SimpleNamespace(st_uid=0, st_mode=stat.S_IFDIR | 0o700)
        stream = mock.MagicMock()
        stream.__enter__.return_value = stream
        with mock.patch.object(d.os, 'lstat', return_value=info), \
                mock.patch.object(d.os, 'O_NOFOLLOW', getattr(d.os, 'O_NOFOLLOW', 0x20000), create=True), \
                mock.patch.object(d.os, 'open', return_value=7) as opened, \
                mock.patch.object(d.os, 'fdopen', return_value=stream), mock.patch.object(d.os, 'fsync'):
            observer.persist('{}')
            filename, flags, mode = opened.call_args[0]
            self.assertEqual(os.path.dirname(filename), str(ROOT / '.prestage'))
            self.assertTrue(os.path.basename(filename).startswith('prepare-diagnostic-'))
            self.assertNotIn(r.WORK, filename)
            self.assertEqual(mode, 0o600)
            self.assertEqual(flags, os.O_WRONLY | os.O_CREAT | os.O_EXCL | d.os.O_NOFOLLOW)
        # Native permissions/exclusivity receipt is exercised on the disposable Linux runner.
        if os.name == 'posix':
            with tempfile.TemporaryDirectory() as temp:
                observer.directory = os.path.join(temp, '.prestage')
                os.mkdir(observer.directory, 0o700)
                with mock.patch.object(d.os, 'lstat', return_value=info), \
                        mock.patch.object(d.uuid, 'uuid4', return_value=types.SimpleNamespace(hex='f' * 32)):
                    observer.persist('{}')
                    filename = os.path.join(observer.directory, 'prepare-diagnostic-' + 'f' * 32 + '.json')
                    self.assertEqual(stat.S_IMODE(os.stat(filename).st_mode), 0o600)
                    with self.assertRaises(FileExistsError): observer.persist('{}')
                    os.unlink(filename)
                    target = os.path.join(temp, 'foreign')
                    with open(target, 'w') as stream: stream.write(CANARY)
                    os.symlink(target, filename)
                    with self.assertRaises(OSError): observer.persist('{}')
                    with open(target) as stream: self.assertEqual(stream.read(), CANARY)

    def test_symlink_writable_foreign_ancestor_or_io_failure_only_falls_back(self):
        observer = diagnostic()
        failure = report_call(lambda: r.authenticate(None))
        # Capture the original exception without ever stringifying it.
        try:
            r.authenticate(None)
        except r.Refused as error:
            for mode, uid in ((stat.S_IFLNK | 0o700, 0), (stat.S_IFDIR | 0o722, 0), (stat.S_IFDIR | 0o700, 1)):
                with self.subTest(mode=mode, uid=uid), mock.patch.object(d.os, 'lstat',
                        return_value=types.SimpleNamespace(st_uid=uid, st_mode=mode)), mock.patch.object(d.os, 'open') as opened:
                    stderr = io.StringIO()
                    observer.emit(error, stderr)
                    opened.assert_not_called()
                    self.assertIn(failure['check_name'].split(':L')[0], stderr.getvalue())
                    self.assertNotIn(CANARY, stderr.getvalue())
            with mock.patch.object(d.os, 'lstat', side_effect=NoString(CANARY)):
                stderr = io.StringIO()
                observer.emit(error, stderr)
                self.assertIn('CHECK_AT_SOURCE', stderr.getvalue())
            observer.emit(error, types.SimpleNamespace(write=mock.Mock(side_effect=NoString(CANARY))))

    def test_python36_syntax(self):
        if sys.version_info >= (3, 8):
            for path in (PATH, ROOT / 'ops/cold-recovery-prepare-diagnostics.py', Path(__file__)):
                ast.parse(path.read_text(encoding='utf-8'), feature_version=(3, 6))


class MountPolicyTests(unittest.TestCase):
    """Actual observe producer, no Docker/SQL/probes or reconstructed history."""
    def setUp(self):
        self.objects = {}
        self.anchor = {'services': {}}
        self.hold = {'release_sha': r.APP, 'proof_sha256': r.OLD_PROOF,
                     'image_ids': [r.IMAGE], 'project': r.PROJECT}
        for i, role in enumerate(('xianzhi-ai', 'smartvideo-worker', 'postgres', 'redis',
                                 'rabbitmq', 'minio', 'disk-monitor', 'postgres-backup', 'migrate')):
            cid = '{:064x}'.format(i + 1)
            infra = role in r.cold.INFRASTRUCTURE
            obj = {'Id': cid, 'Image': r.IMAGE, 'RestartCount': 0,
                   'Config': {'Image': r.REF, 'Labels': {'com.docker.compose.service': role,
                              'com.docker.compose.project': r.PROJECT},
                              'Env': ['K=first', 'K=last'], 'Entrypoint': ['/bin/sh', '-c']},
                   'HostConfig': {'RestartPolicy': {'Name': 'unless-stopped' if infra else 'no'},
                                  'Binds': ['one:/one:ro', 'two:/two:rw']},
                   'Mounts': [{'Type': 'bind', 'Source': 'fixture-one', 'Destination': '/one',
                               'Driver': '', 'Name': '', 'Mode': 'ro', 'RW': False,
                               'Propagation': 'rprivate', 'Unknown': {'value': 1}},
                              {'Type': 'volume', 'Source': 'fixture-two', 'Destination': '/two',
                               'Driver': 'local', 'Name': 'fixture-volume', 'Mode': 'rw',
                               'RW': True, 'Propagation': ''}],
                   'Path': '/bin/sh', 'Args': ['-c', 'fixture-command'],
                   'NetworkSettings': {'Networks': {'fixture-network': {'IPAddress': 'fixture-ip'}}},
                   'State': {'Running': infra, 'Paused': False, 'Restarting': False,
                             'Status': 'running' if infra else 'exited', 'ExitCode': 0,
                             'Health': {'Status': 'healthy'}, 'StartedAt': 'fixed-epoch'}}
            self.objects[cid] = obj
            self.anchor['services'][role] = {'cid': cid, 'image_id': r.IMAGE,
                                             'image_reference': r.REF, 'started_at': 'fixed-epoch'}
        self.cid = self.anchor['services']['migrate']['cid']
        self.obj = self.objects[self.cid]

    def observe(self):
        calls = []
        test = self
        class Docker:
            def text(self, args):
                calls.append(('text', args))
                return '\n'.join(test.objects)
            def inspect(self, kind, identity):
                calls.append((kind, identity))
                return test.objects[identity]
        with mock.patch.object(r, 'read', return_value=self.hold), \
                mock.patch.object(r, 'authenticate', side_effect=lambda value: value):
            result = r.observe(Docker(), self.anchor)
        self.assertEqual(calls[0], ('text', ['ps', '-aq', '--no-trunc']))
        self.assertEqual(calls[1:], [('container', cid) for cid in self.objects])
        return result

    def test_mount_permutations_equal_without_input_mutation(self):
        import itertools
        self.obj['Mounts'].append(copy.deepcopy(self.obj['Mounts'][0]))
        original = copy.deepcopy(self.objects)
        expected = self.observe()
        for permutation in itertools.permutations(original[self.cid]['Mounts']):
            self.obj['Mounts'] = list(copy.deepcopy(permutation))
            before = copy.deepcopy(self.objects)
            self.assertEqual(self.observe(), expected)
            self.assertEqual(self.objects, before)

    def test_every_mount_field_and_duplicate_count_still_bound(self):
        expected = self.observe()
        baseline = copy.deepcopy(self.obj['Mounts'])
        for name in baseline[0]:
            with self.subTest(field=name):
                self.obj['Mounts'] = copy.deepcopy(baseline)
                self.obj['Mounts'][0][name] = {'different': True}
                self.assertNotEqual(self.observe()['migrate']['policy_sha256'],
                                    expected['migrate']['policy_sha256'])
        for mounts in (baseline[:1], baseline + [copy.deepcopy(baseline[0])], None, []):
            self.obj['Mounts'] = copy.deepcopy(mounts)
            self.assertNotEqual(self.observe()['migrate']['policy_sha256'],
                                expected['migrate']['policy_sha256'])

    def test_semantic_arrays_and_configuration_remain_order_sensitive(self):
        baseline = copy.deepcopy(self.obj)
        expected = self.observe()['migrate']['policy_sha256']
        for root, name in (('Config', 'Env'), ('Config', 'Entrypoint'), ('HostConfig', 'Binds')):
            with self.subTest(field=root + '.' + name):
                self.objects[self.cid] = copy.deepcopy(baseline)
                self.objects[self.cid][root][name].reverse()
                self.assertNotEqual(self.observe()['migrate']['policy_sha256'], expected)
        for name, value in (('Args', ['fixture-command', '-c']), ('Path', 'changed-path'),
                            ('Config', dict(baseline['Config'], Image='changed-image-reference')),
                            ('HostConfig', dict(baseline['HostConfig'], NetworkMode='changed-network')),
                            ('NetworkSettings', {'Networks': {'another-network': {}}})):
            with self.subTest(field=name):
                self.objects[self.cid] = copy.deepcopy(baseline)
                self.objects[self.cid][name] = value
                if name == 'Config':
                    with self.assertRaises(r.Refused): self.observe()
                else:
                    self.assertNotEqual(self.observe()['migrate']['policy_sha256'], expected)

    def test_identity_and_restart_guards_not_weakened(self):
        baseline = copy.deepcopy(self.obj)
        for field, value in (('Id', 'changed-cid'), ('Image', 'changed-image')):
            self.objects[self.cid] = copy.deepcopy(baseline)
            self.objects[self.cid][field] = value
            with self.assertRaises(r.Refused): self.observe()
        self.objects[self.cid] = copy.deepcopy(baseline)
        self.objects[self.cid]['HostConfig']['RestartPolicy']['Name'] = 'always'
        with self.assertRaises(r.Refused): self.observe()


def post_diagnostic():
    modules = (r, r.cap, r.cold, r.transport, r.runtime)
    sources = [(m.__dict__, m.__file__, 'ops/' + os.path.basename(m.__file__)) for m in modules]
    return d.PostStartDiagnostic(str(ROOT), sources, r.Refused, r.transport.TransportError, r.runtime.GateError)


class PostStartDiagnosticsTests(unittest.TestCase):
    def runtime_failure(self, ready=None, worker_health='starting'):
        calls = []
        def execute(args):
            calls.append(args)
            if args[-1] == r.runtime.FLAGS:
                return json.dumps(dict(GENERATION_FAIR_SCHEDULER_ENABLED='true', ASYNC_MESSAGING_ENABLED='true', secret=CANARY))
            if args[-1].endswith('/metrics'):
                return '\n'.join(name + ' 1' for name in ('generation_scheduler_db_scrape_success', 'generation_scheduler_errors_total', 'generation_scheduler_dispatched_total', 'generation_scheduler_recovered_total'))
            if args[-1].endswith('/ready'):
                return json.dumps(ready or dict(ready='true', asyncMessaging='READY', secret=CANARY))
            if args[-3:-1] == ['ps', '-q']:
                return 'worker-cid' if args[-1] == 'smartvideo-worker' else 'api-cid'
            if '{{.State.Running}}' in args:
                return 'true'
            if '{{.State.Health.Status}}' in args:
                return worker_health if args[-1] == 'worker-cid' else 'healthy'
            raise AssertionError('unexpected observer call')
        try:
            r.runtime.verify('post', 'compose', 'env', execute=execute, pause=lambda _: None)
        except r.runtime.GateError as error:
            return error, calls
        raise AssertionError('fixture did not refuse')

    def test_actual_worker_starting_check_has_expected_actual_and_source(self):
        error, calls = self.runtime_failure()
        diag = post_diagnostic(); diag.stage_at('post_start_health')
        data = diag.details(error)
        self.assertEqual(data['reason_code'], 'POST_START_RUNTIME_HEALTH_FAILED')
        self.assertEqual(data['source_file'], 'ops/verify-release-runtime.py')
        self.assertEqual(data['function'], 'verify')
        self.assertEqual(data['expected']['healthy'], 'healthy')
        self.assertEqual(data['actual']['healthy'], 'starting')
        self.assertEqual(data['actual']['service'], 'smartvideo-worker')
        self.assertEqual(len(calls), 10)
        self.assertNotIn(CANARY, json.dumps(data))

    def test_actual_ready_failure_only_emits_bounded_returned_enums(self):
        error, calls = self.runtime_failure(dict(ready='false', asyncMessaging='DEGRADED', password=CANARY))
        diag = post_diagnostic(); diag.stage_at('post_start_health'); data = diag.details(error)
        self.assertEqual(data['reason_code'], 'POST_START_READY_FAILED')
        self.assertEqual(data['expected']['asyncMessaging'], 'READY')
        self.assertEqual(data['actual']['asyncMessaging'], 'DEGRADED')
        self.assertEqual(len(calls), 4)
        self.assertNotIn(CANARY, json.dumps(data))

    def test_official_dynamic_runtime_namespace_matches_code_not_fake_filename(self):
        module = types.ModuleType('runtime_health'); module.__file__ = r.runtime.__file__
        exec(compile(Path(module.__file__).read_bytes(), module.__file__, 'exec'), module.__dict__)
        try:
            module.verify_quarantine_barrier_read_only('compose', 'env', execute=lambda _: '0')
        except BaseException as error:
            data = post_diagnostic().details(error)
        self.assertEqual(data['source_file'], 'ops/verify-release-runtime.py')
        self.assertEqual(data['guard_label'], 'quarantine barrier relation or immutable trigger unhealthy or absent')
        fake = {}; exec(compile('def fake():\n raise ValueError("' + CANARY + '")\n', r.runtime.__file__, 'exec'), fake)
        try:
            fake['fake']()
        except BaseException as error:
            data = post_diagnostic().details(error)
        self.assertEqual(data['check_name'], 'UNKNOWN')
        self.assertNotIn(CANARY, json.dumps(data))

    def test_hostile_error_and_summary_values_never_escape(self):
        diag = post_diagnostic(); diag.stage_at(CANARY)
        try:
            raise NoString(CANARY)
        except BaseException as error:
            data = diag.details(error)
        self.assertNotIn(CANARY, json.dumps(data))
        error, _ = self.runtime_failure(dict(ready=CANARY, asyncMessaging=CANARY, env=CANARY))
        data = diag.details(error)
        self.assertNotIn(CANARY, json.dumps(data))
        self.assertEqual(data['actual']['ready'], 'UNRECOGNIZED')
        self.assertEqual(data['actual']['asyncMessaging'], 'UNRECOGNIZED')

    def test_first_exclusive_private_record_survives_other_process_copy_and_io_fail(self):
        error, _ = self.runtime_failure()
        with tempfile.TemporaryDirectory() as directory:
            child = post_diagnostic(); child.directory = directory; child.stage_at('post_start_commit')
            parent = post_diagnostic(); parent.directory = directory; parent.filename = child.filename; parent.stage_at('post_start_health')
            output = io.StringIO()
            # Protected ancestor metadata mocked only for owned unit tempdir; actual O_EXCL/mode/fsync are real.
            with mock.patch.object(d.os, 'lstat', return_value=types.SimpleNamespace(st_uid=0, st_mode=stat.S_IFDIR | 0o700)), mock.patch.object(d.os, 'O_NOFOLLOW', getattr(d.os, 'O_NOFOLLOW', 0), create=True):
                if hasattr(os, 'fork'):
                    pid = os.fork()
                    if pid == 0:
                        try:
                            child.emit_first(error, io.StringIO())
                        finally:
                            os._exit(0)
                    _, status = os.waitpid(pid, 0)
                    self.assertEqual(status, 0)
                    # Child stderr is separate in this fixture; the saved first record is shared.
                    output.write('COLD_RECOVERY_POST_START_DIAGNOSTIC: child\n')
                else:
                    child.emit_first(error, output)
                saved = (Path(directory) / child.filename).read_bytes()
                parent.emit_first(NoString(CANARY), output)
            self.assertEqual((Path(directory) / child.filename).read_bytes(), saved)
            self.assertEqual(output.getvalue().count('COLD_RECOVERY_POST_START_DIAGNOSTIC: '), 1)
            if os.name != 'nt': self.assertEqual((Path(directory) / child.filename).stat().st_mode & 0o777, 0o600)
            failing = post_diagnostic(); failing.stage_at('post_start_health'); output = io.StringIO()
            with mock.patch.object(failing, 'persist', side_effect=PermissionError(CANARY)):
                failing.emit_first(error, output)
            self.assertIn('POST_START_RUNTIME_HEALTH_FAILED', output.getvalue())
            self.assertNotIn(CANARY, output.getvalue())

    def test_path_guards_are_private_no_symlink_no_new_authority(self):
        diag = post_diagnostic()
        for uid, mode in ((1, stat.S_IFDIR | 0o700), (0, stat.S_IFLNK | 0o700), (0, stat.S_IFDIR | 0o777)):
            with mock.patch.object(d.os, 'lstat', return_value=types.SimpleNamespace(st_uid=uid, st_mode=mode)):
                with self.assertRaises(PermissionError): diag.persist('{}')
        self.assertTrue(diag.filename.startswith('post-start-diagnostic-'))

    def execute_case(self):
        path = ROOT / 'tests/cold-recovery-resume-test.py'
        module = types.ModuleType('existing_execute_fixture'); module.__file__ = str(path)
        exec(compile(path.read_bytes(), str(path), 'exec'), module.__dict__)
        case = module.ResumeTests(); case.setUp(); self.addCleanup(case.doCleanups)
        return module, case

    def test_actual_execute_preserves_first_health_error_before_cleanup_mask(self):
        module, case = self.execute_case(); error, _ = self.runtime_failure(); records = []; done = threading.Event()
        def save(text):
            records.append(text); done.set()
        with mock.patch.object(case.backend, 'healthy', side_effect=error), mock.patch.object(module.r.prepare_diagnostics.PostStartDiagnostic, 'persist', side_effect=save), mock.patch.object(module.r.sys, 'stderr', io.StringIO()):
            with self.assertRaises(module.r.Refused): case.execute()
            self.assertTrue(done.wait(5))  # Fixture wait only, not native cleanup.
        self.assertEqual(len(records), 1)
        data = json.loads(records[0])
        self.assertEqual(data['stage'], 'post_start_health')
        self.assertEqual(data['reason_code'], 'POST_START_RUNTIME_HEALTH_FAILED')
        self.assertEqual(len(case.backend.docker.calls), 2)
        self.assertEqual(case.backend.samples, 3)
        self.assertEqual(module.r.cold.read_hold(case.directory)['state'], module.r.STOPPED)
        self.assertNotIn(CANARY, records[0])

    def test_post_snapshot_summary_keeps_original_strict_equality_and_call_count(self):
        module, case = self.execute_case(); case.backend.drift_at = 4; records = []; done = threading.Event()
        def save(text):
            records.append(text); done.set()
        with mock.patch.object(module.r.prepare_diagnostics.PostStartDiagnostic, 'persist', side_effect=save), mock.patch.object(module.r.sys, 'stderr', io.StringIO()):
            with self.assertRaises(module.r.Refused): case.execute()
            self.assertTrue(done.wait(5))
        data = json.loads(records[0]); self.assertEqual(data['reason_code'], 'POST_START_PROTECTED_SNAPSHOT_MISMATCH')
        self.assertFalse(data['actual']['snapshot_equal'])
        self.assertEqual(case.backend.samples, 4)
        self.assertEqual(len(case.backend.docker.calls), 2)

    def test_success_and_diagnostic_failure_do_not_change_existing_calls(self):
        module, case = self.execute_case()
        with mock.patch.object(module.r.prepare_diagnostics.PostStartDiagnostic, 'persist', side_effect=AssertionError('should not write')):
            case.execute()
        self.assertEqual(case.backend.samples, 4)
        self.assertEqual(len(case.backend.docker.calls), 2)
        self.assertEqual(module.r.cold.read_hold(case.directory)['state'], module.r.SUCCESS)

    def watch_failure(self, output, persist, blocked=False):
        diag = post_diagnostic(); events = []; entered = threading.Event(); complete = threading.Event()
        def save(text):
            self.assertEqual(events, ['fence', 'release'])
            entered.set()
            persist(text)
            complete.set()

        lock = types.SimpleNamespace(owner='owner', retained=False, release=lambda: events.append('release'))
        hold = {'state': r.STARTING, 'errors': []}
        authority = {'candidate': {'runtime_capability': {}, 'snapshot': {'policy': {}}}, 'approval': {}, 'hold': {}}
        docker = types.SimpleNamespace(text=lambda *args, **kwargs: 'started')
        with ExitStack() as stack:
            for target, name, value in ((r, '_post_start_diagnostic', diag), (r.sys, 'stderr', output)):
                stack.enter_context(mock.patch.object(target, name, value))
            stack.enter_context(mock.patch.object(diag, 'persist', side_effect=save))
            stack.enter_context(mock.patch.object(r, 'check_owned_attempt', return_value=hold))
            stack.enter_context(mock.patch.object(r.cold, 'read_hold', return_value=hold))
            stack.enter_context(mock.patch.object(r.cold, 'write_hold'))
            stack.enter_context(mock.patch.object(r.cold, 'fence', side_effect=lambda *a, **k: events.append('fence')))
            stack.enter_context(mock.patch.object(r, 'validate_approval', return_value={}))
            stack.enter_context(mock.patch.object(r, 'source_identity', return_value={}))
            stack.enter_context(mock.patch.object(r, 'authenticate', return_value={}))
            stack.enter_context(mock.patch.object(r, 'check_window'))
            stack.enter_context(mock.patch.object(r.cap, 'verify', return_value=r.IMAGE))
            stack.enter_context(mock.patch.object(r.cap, 'verify_history', return_value=False))
            stack.enter_context(mock.patch.object(r.signal, 'signal'))
            stack.enter_context(mock.patch.object(r.select, 'select', return_value=([10], [], [])))
            stack.enter_context(mock.patch.object(r.os, 'read', side_effect=[b'S', b'S', b'C']))
            stack.enter_context(mock.patch.object(r.os, 'write'))
            start = time.monotonic()
            self.assertEqual(r.AttemptSupervisor.watch('unused', lock, docker, 'nonce', ['one', 'two'], 10, 11, 300, authority), 1)
            self.assertLess(time.monotonic() - start, 2)
            self.assertTrue(entered.wait(5))  # Unit fixture only; keep patches until observer starts.
            if not blocked:
                self.assertTrue(complete.wait(5))
        self.assertEqual(events, ['fence', 'release'])
        return diag

    def test_real_watch_commit_failure_captures_before_but_emits_after_fence(self):
        output = io.StringIO(); records = []
        diag = self.watch_failure(output, records.append)
        self.assertEqual(len(records), 1)
        data = json.loads(output.getvalue().split(': ', 1)[1])
        self.assertEqual(data['stage'], 'post_start_commit')
        self.assertEqual(data['guard_label'], 'commit capability unavailable')
        self.assertEqual(data['function'], 'watch')
        self.assertIsNotNone(diag.captured)
        # Parent ACK cannot be mislabeled as the unique guard failure, even if
        # guard output was dropped/stalled and its exclusive file is missing.
        parent = post_diagnostic(); parent.stage_at('post_start_commit')
        with mock.patch.object(r, '_post_start_diagnostic', parent):
            r.post_start_failure(NoString(CANARY))
        self.assertIsNone(parent.captured)

    def test_stalled_persistence_cannot_delay_watch_fence_or_return(self):
        entered = threading.Event(); release = threading.Event(); done = threading.Event()
        def stalled(_):
            entered.set()
            release.wait()
            done.set()
        try:
            diag = self.watch_failure(io.StringIO(), stalled, blocked=True)
            self.assertTrue(entered.wait(1))
            self.assertFalse(done.is_set())
            self.assertTrue(diag.dispatched)
        finally:
            release.set()
            self.assertTrue(done.wait(2))

    def test_undrained_pipe_output_cannot_delay_watch_fence_or_return(self):
        entered = threading.Event(); release = threading.Event(); done = threading.Event()
        read_fd, write_fd = os.pipe()
        # Large fixture-only write fills a real pipe on Linux/Windows. Production
        # sends only the bounded diagnostic; a previously full pipe behaves the same.
        class PipeOutput:
            def write(self, text):
                entered.set()
                try:
                    remaining = 1024 * 1024
                    while remaining:
                        remaining -= os.write(write_fd, b'x' * remaining)
                except OSError:
                    pass
                finally:
                    done.set()
            def flush(self): pass
        try:
            self.watch_failure(PipeOutput(), lambda _: None)
            self.assertTrue(entered.wait(1))
            self.assertFalse(done.is_set())
        finally:
            os.close(read_fd)  # Release only this fixture's blocked writer.
            self.assertTrue(done.wait(2))
            os.close(write_fd)

    def test_capture_first_does_no_io_and_postcleanup_dispatch_is_once(self):
        error, _ = self.runtime_failure(); diag = post_diagnostic()
        diag.stage_at('post_start_health'); records = []
        with mock.patch.object(diag, 'details', side_effect=AssertionError('pre-fence IO')):
            diag.capture_first(error)
            diag.capture_first(NoString(CANARY))
        done = threading.Event()
        def save(text):
            records.append(text); done.set()
        with mock.patch.object(diag, 'persist', side_effect=save):
            diag.emit_after_cleanup(io.StringIO())
            diag.emit_after_cleanup(io.StringIO())
            self.assertTrue(done.wait(5))
        self.assertEqual(len(records), 1)
        self.assertEqual(json.loads(records[0])['reason_code'], 'POST_START_RUNTIME_HEALTH_FAILED')


class WorkerStartingWaitTests(unittest.TestCase):
    def setUp(self):
        self.clock = 0.0; self.calls = []; self.sleeps = []
        self.health = ['starting']; self.states = ['true']; self.advance_state = 0
        self.backend = object.__new__(r.Backend)
        self.backend.anchor = {'services': {'smartvideo-worker': {'cid': 'worker-cid'}}}
        self.backend.docker = types.SimpleNamespace(text=self.text)
        self.query = ['docker', 'inspect', '--format', '{{.State.Health.Status}}', 'worker-cid']
        self.addCleanup(mock.patch.stopall)
        mock.patch.object(r.time, 'monotonic', side_effect=lambda: self.clock).start()
        mock.patch.object(r.time, 'sleep', side_effect=self.sleep).start()

    def sleep(self, seconds):
        self.assertGreater(seconds, 0); self.assertLessEqual(seconds, 2)
        self.sleeps.append(seconds); self.clock += seconds

    def text(self, args, **kwargs):
        self.calls.append((args, kwargs))
        if 'timeout' in kwargs:
            self.assertGreater(kwargs['timeout'], 0); self.assertLessEqual(kwargs['timeout'], 5)
        if args == self.query[1:]:
            return self.health.pop(0) if len(self.health) > 1 else self.health[0]
        if args == ['inspect', '--format', '{{.State.Running}}', 'worker-cid']:
            self.clock += self.advance_state
            return self.states.pop(0) if len(self.states) > 1 else self.states[0]
        return 'delegated-observation'

    def observe(self):return self.backend.recovery_runtime_observation(self.query)

    def test_healthy_has_original_single_observation_no_wait(self):
        self.health = ['healthy']; self.assertEqual(self.observe(), 'healthy')
        self.assertEqual(len(self.calls), 1); self.assertEqual(self.sleeps, [])

    def test_initial_unhealthy_or_unknown_never_retried(self):
        for value in ('unhealthy', '', 'UNRECOGNIZED'):
            self.health = [value]; self.assertEqual(self.observe(), value)
        self.assertEqual(len(self.calls), 3); self.assertEqual(self.sleeps, [])

    def test_starting_converges_healthy_on_same_pinned_cid(self):
        self.health = ['starting', 'starting', 'healthy']
        self.assertEqual(self.observe(), 'healthy'); self.assertEqual(self.sleeps, [2, 2])
        self.assertEqual(len(self.calls), 6)
        self.assertTrue(all(args[-1] == 'worker-cid' for args, _ in self.calls))

    def test_transition_unhealthy_fails_without_another_sleep_or_query(self):
        self.health = ['starting', 'unhealthy']; self.assertEqual(self.observe(), 'unhealthy')
        self.assertEqual(self.sleeps, [2]); self.assertEqual(len(self.calls), 3)

    def test_initial_stopped_worker_immediately_refuses(self):
        self.states = ['false']
        with self.assertRaises(r.GateError):self.observe()
        self.assertEqual(self.sleeps, []); self.assertEqual(len(self.calls), 2)

    def test_worker_stops_as_health_converges_still_refuses(self):
        self.health = ['starting', 'healthy']; self.states = ['true', 'false']
        with self.assertRaises(r.GateError):self.observe()
        self.assertEqual(self.sleeps, [2]); self.assertEqual(len(self.calls), 4)

    def test_timeout_starting_has_unique_expected_actual_and_no_nonpositive_rpc(self):
        try:self.observe()
        except r.GateError as error:
            diag = post_diagnostic(); diag.stage_at('post_start_health'); data = diag.details(error)
        self.assertEqual(self.clock, 60); self.assertEqual(sum(self.sleeps), 60)
        self.assertEqual(data['reason_code'], 'POST_START_WORKER_HEALTH_TIMEOUT')
        self.assertEqual(data['expected']['healthy'], 'healthy')
        self.assertEqual(data['expected']['starting_wait_seconds'], 60)
        self.assertEqual(data['actual']['healthy'], 'starting')
        self.assertTrue(data['actual']['wait_budget_exhausted'])
        self.assertEqual(data['actual']['service'], 'smartvideo-worker')
        self.assertEqual(data['function'], 'recovery_runtime_observation')

    def test_observation_consumes_remaining_budget_no_success_after_deadline(self):
        self.health = ['starting', 'healthy']; self.advance_state = 60
        with self.assertRaises(r.GateError):self.observe()
        self.assertEqual(self.clock, 60); self.assertEqual(len(self.calls), 2)
        self.assertEqual(self.sleeps, [])

    def test_api_readiness_broker_barrier_other_cids_are_delegated_unchanged(self):
        for args in (self.query[:-1] + ['api-cid'], self.query[:-1] + ['replacement-cid'],
                     ['docker', 'compose', 'exec', 'api', 'curl', '/ready'],
                     ['docker', 'compose', 'exec', 'postgres', 'sh', '-c', 'barrier'],
                     ['docker', 'compose', 'exec', 'api', 'python3', '-c', 'broker']):
            self.assertEqual(self.backend.recovery_runtime_observation(args), 'delegated-observation')
            self.assertEqual(self.calls[-1], (args[1:], {}))
        self.assertEqual(self.sleeps, [])

    def test_actual_backend_healthy_wires_wait_without_changing_other_gate_order(self):
        self.health = ['starting', 'healthy']; events = []
        def verify(phase, compose, env, execute):
            events.append('runtime')
            self.assertEqual(phase, 'post'); self.assertEqual(execute(self.query), 'healthy')
        with mock.patch.object(r.cap, 'rendered_policy', return_value=({}, {})), \
             mock.patch.object(r.cap, 'verify_running', side_effect=lambda *args: events.append('pid1-policy')), \
             mock.patch.object(r.runtime, 'verify', side_effect=verify), \
             mock.patch.object(r.cold, 'read_health', side_effect=lambda *args: events.append('api-health') or {'status': 'ok'}):
            self.backend.healthy({})
        self.assertEqual(events, ['pid1-policy', 'runtime', 'api-health'])
        self.assertEqual(self.sleeps, [2])


if __name__ == '__main__':
    unittest.main(verbosity=2)
