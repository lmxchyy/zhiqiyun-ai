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


if __name__ == '__main__':
    unittest.main(verbosity=2)
