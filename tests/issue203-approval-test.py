"""Synthetic approval tests; run ONLY in an owned, network-none root container.

No test key/registry is a production trust root. The source mount is read-only.
"""
import base64
import copy
import datetime
import hashlib
import json
import os
from pathlib import Path
import py_compile
import shutil
import subprocess
import sys
import tempfile
import types
import unittest

ROOT = Path(__file__).resolve().parent.parent
sys.dont_write_bytecode = True
approval = types.ModuleType('approval')
source = ROOT / 'ops/quarantine-approval.py'
approval.__file__ = str(source)
exec(compile(source.read_bytes(), str(source), 'exec'), approval.__dict__)
NOW = datetime.datetime.now(datetime.timezone.utc)


def ts(value):
    return value.strftime('%Y-%m-%dT%H:%M:%S.%fZ')


def openssl(args, data=None):
    proc = subprocess.run(['openssl'] + args, input=data, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE)
    if proc.returncode:
        raise RuntimeError('synthetic OpenSSL fixture failed')
    return proc.stdout


class ApprovalTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if os.name != 'posix' or os.getuid() != 0 or not os.environ.get('ISSUE203_OWNED_APPROVAL_CONTAINER'):
            raise RuntimeError('required owned root container unavailable (not SKIP)')
        cls.work = Path(tempfile.mkdtemp(prefix='synthetic-approval-', dir='/root'))
        cls.root = cls.work / 'trusted-registry'
        if cls.root.exists():
            raise RuntimeError('refuse pre-existing/unowned root')
        cls.root.mkdir(parents=True, mode=0o700)
        approval.REGISTRY_PATH = str(cls.root / 'registry.json')
        cls.private = cls.work / 'synthetic-private.pem'
        cls.public = cls.root / 'synthetic-public.pem'
        openssl(['genpkey', '-algorithm', 'RSA', '-pkeyopt', 'rsa_keygen_bits:3072', '-out', str(cls.private)])
        cls.public.write_bytes(openssl(['pkey', '-in', str(cls.private), '-pubout']))
        cls.authority = dict(authority_id=approval.AUTHORITY_ID, key_id='synthetic-3072',
                             purpose=approval.PURPOSE, algorithm=approval.ALGORITHM,
                             public_key_file=cls.public.name, revoked=False,
                             public_key_sha256=hashlib.sha256(openssl([
                                 'pkey', '-pubin', '-in', str(cls.public), '-outform', 'DER'])).hexdigest(),
                             not_before=ts(NOW - datetime.timedelta(days=1)),
                             expires_at=ts(NOW + datetime.timedelta(days=1)))

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(str(cls.root))  # owned container/test-created subtree only
        shutil.rmtree(str(cls.work))

    def setUp(self):
        self.registry = dict(version=1, authorities=[copy.deepcopy(self.authority)])
        self.write_registry()
        nb, exp = ts(NOW - datetime.timedelta(minutes=5)), ts(NOW + datetime.timedelta(minutes=30))
        evidence = dict(approval_id='synthetic-approval-1', snapshot_sha256='a' * 64,
                        review_sha256=hashlib.sha256(b'synthetic reviewed content').hexdigest())
        item = dict(execution_id=901, task_id='synthetic-task', attempt=1,
                    generation=1, task_generation=2, approval_id=evidence['approval_id'],
                    snapshot_sha256=evidence['snapshot_sha256'], evidence=evidence,
                    evidence_sha256=hashlib.sha256(approval.canonical(evidence)).hexdigest(),
                    release_sha='b' * 40, not_before=nb, expires_at=exp)
        self.manifest = dict(manifest_version='2.0', purpose=approval.PURPOSE,
                             algorithm=approval.ALGORITHM, authority_id=self.authority['authority_id'],
                             key_id=self.authority['key_id'], operations=approval.OPERATIONS,
                             release_sha='b' * 40, approved_count=1, not_before=nb,
                             expires_at=exp, executions=[item])

    def write_registry(self):
        Path(approval.REGISTRY_PATH).write_bytes(approval.canonical(self.registry))
        os.chmod(approval.REGISTRY_PATH, 0o644)

    def signed(self, manifest=None, key=None):
        data = copy.deepcopy(manifest or self.manifest)
        data.pop('signature', None)
        data['signature'] = base64.b64encode(openssl([
            'dgst', '-sha256', '-sign', str(key or self.private),
            '-sigopt', 'rsa_padding_mode:pkcs1'], approval.canonical(data))).decode('ascii')
        return approval.canonical(data)

    def verify(self, raw=None):
        return approval.verify(raw or self.signed(), 'b' * 40, 'enroll', NOW)

    def rejected(self, raw=None):
        with self.assertRaisesRegex(approval.ApprovalError, '^QUARANTINE_APPROVAL_[A-Z_]+$'):
            self.verify(raw)

    def test_valid_pinned_3072_signature_and_both_operations(self):
        raw = self.signed()
        self.assertEqual(self.verify(raw)['approved_count'], 1)
        self.assertEqual(approval.verify(raw, 'b' * 40, 'drain-exemption', NOW)['approved_count'], 1)

    def test_unsigned_candidate_and_review_binding_have_no_signing_capability(self):
        identity = dict(execution_id=901, task_id='synthetic-task', attempt=1,
                        generation=1, task_generation=2)
        snapshot = dict(version=approval.LIVE_SNAPSHOT_VERSION,
                        scope='CANONICAL_LIVE_SNAPSHOT', execution_id=901,
                        task_id='synthetic-task', attempt=1, generation=1,
                        task_generation=2, core={'hashed': 'a'*64},
                        financial={'hashed': 'b'*64},
                        asset_storage={'hashed': 'c'*64})
        candidate = approval.build_unsigned_candidate(
            [identity], {901: snapshot}, 'b'*40, self.authority['key_id'],
            self.manifest['not_before'], self.manifest['expires_at'], NOW)
        self.assertEqual(candidate['authority_id'], approval.AUTHORITY_ID)
        self.assertEqual(candidate['status'], 'UNSIGNED_REQUIRES_HUMAN_REVIEW')
        self.assertNotIn('signature', candidate)
        self.assertNotIn('approval_id', candidate['records'][0])
        raw_unsigned = approval.unsigned_manifest_bytes(candidate, [
            dict(execution_id=901, approval_id='human-review-1', review_sha256='d'*64)])
        unsigned = approval.decode(raw_unsigned)
        self.assertNotIn('signature', unsigned)
        with self.assertRaises(approval.ApprovalError):
            approval.verify(raw_unsigned, 'b'*40, 'enroll', NOW)
        signed = self.signed(unsigned)
        self.assertEqual(self.verify(signed)['executions'][0]['snapshot_sha256'],
                         candidate['records'][0]['snapshot_sha256'])
        tampered = copy.deepcopy(candidate)
        tampered['records'][0]['snapshot']['task_id'] = 'other-task'
        with self.assertRaisesRegex(approval.ApprovalError, 'QUARANTINE_CANDIDATE_SNAPSHOT_INVALID'):
            approval.unsigned_manifest_bytes(tampered, [
                dict(execution_id=901, approval_id='human-review-1', review_sha256='d'*64)])

    def test_forged_signature_and_modified_signed_content(self):
        m = approval.decode(self.signed())
        m['signature'] = base64.b64encode(b'\0' * 384).decode('ascii')
        self.rejected(approval.canonical(m))
        m = approval.decode(self.signed())
        m['executions'][0]['generation'] = 999
        self.rejected(approval.canonical(m))

    def test_wrongpurpose_manifest_and_registry(self):
        self.manifest['purpose'] = 'prestage-proof'
        self.rejected()
        self.manifest['purpose'] = approval.PURPOSE
        self.registry['authorities'][0]['purpose'] = 'prestage-proof'
        self.write_registry()
        self.rejected()

    def test_carrier_registry_cannot_select_external_public_key_path(self):
        self.registry['authorities'][0]['public_key_file'] = '../outside.pem'
        self.write_registry()
        self.rejected()
        self.registry['authorities'] = []
        self.write_registry()
        self.rejected()  # Empty, unprovisioned registry is fail-closed.

    def test_missing_unknown_authority_and_revocation(self):
        for field, value in [('authority_id', 'arbitrary-authority'), ('key_id', 'arbitrary-key')]:
            with self.subTest(field=field):
                old = self.manifest[field]
                self.manifest[field] = value
                self.rejected()
                self.manifest[field] = old
        self.registry['authorities'][0]['revoked'] = True
        self.write_registry()
        self.rejected()
        Path(approval.REGISTRY_PATH).unlink()
        os.environ['RELEASE_TRUST_SECRET'] = 'synthetic-no-fallback'
        self.rejected()

    def test_duplicate_authority_and_unknown_authority_fields(self):
        self.registry['authorities'].append(copy.deepcopy(self.authority))
        self.write_registry()
        self.rejected()
        self.registry['authorities'] = [dict(self.authority, approved=True)]
        self.write_registry()
        self.rejected()

    def test_root_key_substitution_forged_key_and_weak_key(self):
        other = self.work / 'other-private.pem'
        openssl(['genpkey', '-algorithm', 'RSA', '-pkeyopt', 'rsa_keygen_bits:3072', '-out', str(other)])
        self.rejected(self.signed(key=other))
        original = self.public.read_bytes()
        try:
            self.public.write_bytes(openssl(['pkey', '-in', str(other), '-pubout']))
            self.rejected(self.signed(key=other))  # valid signature, wrong pinned fingerprint
            openssl(['genpkey', '-algorithm', 'RSA', '-pkeyopt', 'rsa_keygen_bits:2048', '-out', str(other)])
            self.public.write_bytes(openssl(['pkey', '-in', str(other), '-pubout']))
            self.registry['authorities'][0]['public_key_sha256'] = hashlib.sha256(openssl([
                'pkey', '-pubin', '-in', str(self.public), '-outform', 'DER'])).hexdigest()
            self.write_registry()
            self.rejected(self.signed(key=other))
        finally:
            self.public.write_bytes(original)

    def test_root_and_key_permissions_symlinks_and_ownership(self):
        for path in (Path(approval.REGISTRY_PATH), self.public, self.root, self.root.parent):
            with self.subTest(path=path.name):
                original_mode = path.stat().st_mode & 0o777
                os.chmod(str(path), 0o777)
                self.rejected()
                os.chmod(str(path), original_mode)
        os.chown(str(self.public), 12345, 0)
        self.rejected()
        os.chown(str(self.public), 0, 0)
        original = self.public.read_bytes()
        self.public.unlink()
        destination = self.work / 'public.pem'
        destination.write_bytes(original)
        self.public.symlink_to(destination)
        self.rejected()
        self.public.unlink()
        self.public.write_bytes(original)

    def test_window_precision_bounds_expiry_and_registry_window(self):
        for field, value in [('not_before', ts(NOW + datetime.timedelta(microseconds=1))),
                             ('expires_at', ts(NOW)), ('expires_at', '2026-10-10T00:00:00Z')]:
            with self.subTest(field=field, value=value):
                old = self.manifest[field]
                self.manifest[field] = value
                self.manifest['executions'][0][field] = value
                self.rejected()
                self.manifest[field] = old
                self.manifest['executions'][0][field] = old
        self.registry['authorities'][0]['expires_at'] = ts(NOW + datetime.timedelta(seconds=1))
        self.write_registry()
        self.rejected()

    def test_per_entry_release_window_count_duplicates_null_generation(self):
        for field, value in [('release_sha', 'c' * 40), ('not_before', ts(NOW)),
                             ('expires_at', ts(NOW + datetime.timedelta(seconds=10))),
                             ('generation', None), ('task_generation', None)]:
            with self.subTest(field=field):
                old = self.manifest['executions'][0][field]
                self.manifest['executions'][0][field] = value
                self.rejected()
                self.manifest['executions'][0][field] = old
        self.manifest['approved_count'] = 2
        self.rejected()
        self.manifest['executions'].append(copy.deepcopy(self.manifest['executions'][0]))
        self.rejected()

    def test_arbitrary_evidence_hash_and_modified_evidence(self):
        for field, value in [('evidence_sha256', '0' * 64), ('snapshot_sha256', '0' * 64)]:
            old = self.manifest['executions'][0][field]
            self.manifest['executions'][0][field] = value
            self.rejected()
            self.manifest['executions'][0][field] = old
        self.manifest['executions'][0]['evidence']['review_sha256'] = 'f' * 64
        self.rejected()

    def test_json_duplicate_fields_nonfinite_wrong_operation(self):
        raw = self.signed()
        self.rejected(raw.replace(b'"approved_count":1', b'"approved_count":1,"approved_count":1'))
        self.rejected(raw.replace(b'"approved_count":1', b'"approved_count":NaN'))
        with self.assertRaises(approval.ApprovalError):
            approval.verify(raw, 'b' * 40, 'rollback', NOW)

    def test_missing_unknown_runtime_dependencies(self):
        raw = self.signed()
        old_path = os.environ['PATH']
        try:
            os.environ['PATH'] = '/synthetic-missing-openssl'
            self.rejected(raw)
            shim = self.work / 'runtime-shim'
            shim.mkdir(exist_ok=True)
            (shim / 'openssl').write_text('#!/bin/sh\necho LibreSSL-unknown\n')
            os.chmod(str(shim / 'openssl'), 0o755)
            os.environ['PATH'] = str(shim)
            self.rejected(raw)
        finally:
            os.environ['PATH'] = old_path

    def test_hostile_identity_fields_are_signed_data_never_shell(self):
        for value in ["single'quote", 'double"quote', '$(touch /tmp/ISSUE203_PWNED)',
                      '`touch /tmp/ISSUE203_PWNED`', ';touch /tmp/ISSUE203_PWNED',
                      'line1\nline2', '中文', 'json\\escape']:
            with self.subTest(value=value):
                self.manifest['executions'][0]['task_id'] = value
                self.assertEqual(self.verify()['executions'][0]['task_id'], value)
        self.assertFalse(Path('/tmp/ISSUE203_PWNED').exists())
        self.manifest['executions'][0]['task_id'] = 'x' * 257
        self.rejected()

    def test_source_loader_ignores_timestamp_valid_approval_pyc(self):
        directory = Path(tempfile.mkdtemp(prefix='approval-source-only-', dir=str(self.work)))
        for name in ('enroll-quarantine.py', 'quarantine-approval.py', 'quarantine-live-snapshot.py', 'quarantine-psql-transport.py'):
            shutil.copy2(str(ROOT / 'ops' / name), str(directory / name))
        source = directory / 'quarantine-approval.py'
        original, original_stat = source.read_bytes(), source.stat()
        marker = directory / 'UNBOUND_BYTECODE_EXECUTED'
        payload = ('open(' + repr(str(marker)) + ', "w").write("unbound")\nraise SystemExit(0)\n').encode('ascii')
        source.write_bytes(payload + b'#' * (len(original) - len(payload)))
        os.utime(str(source), (original_stat.st_atime, original_stat.st_mtime))
        py_compile.compile(str(source), doraise=True)
        source.write_bytes(original)
        os.utime(str(source), (original_stat.st_atime, original_stat.st_mtime))
        control = ('import importlib.util,sys;sys.dont_write_bytecode=True;'
                   's=importlib.util.spec_from_file_location("probe",sys.argv[1]);'
                   'm=importlib.util.module_from_spec(s);s.loader.exec_module(m)')
        proc = subprocess.run([sys.executable, '-c', control, str(source)],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(proc.returncode, 0)
        self.assertTrue(marker.exists())
        marker.unlink()
        before = sorted(str(p) for p in directory.rglob('*'))
        proc = subprocess.run([sys.executable, str(directory / 'enroll-quarantine.py')],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(proc.returncode, 1)
        self.assertIn(b'Usage: enroll-quarantine.py', proc.stderr)
        self.assertFalse(marker.exists())
        self.assertEqual(sorted(str(p) for p in directory.rglob('*')), before)

    def test_cli_missing_root_and_forged_signature_have_distinct_redacted_errors(self):
        path = self.work / 'cli-negative.json'
        path.write_bytes(self.signed())
        cli_ops = self.work / 'cli-source' / 'ops'
        cli_trust = cli_ops / 'quarantine-approval'
        cli_trust.mkdir(parents=True, mode=0o755)
        for name in ('enroll-quarantine.py', 'quarantine-approval.py',
                     'quarantine-live-snapshot.py', 'quarantine-psql-transport.py'):
            shutil.copyfile(str(ROOT / 'ops' / name), str(cli_ops / name))
        source_registry = Path(approval.REGISTRY_PATH)
        shutil.copyfile(str(source_registry), str(cli_trust / 'registry.json'))
        shutil.copyfile(str(self.public), str(cli_trust / self.public.name))
        root = cli_trust / 'registry.json'
        registry = root.read_bytes()
        root.unlink()
        cli_script = cli_ops / 'enroll-quarantine.py'
        proc = subprocess.run([sys.executable, str(cli_script),
                               'dummy', 'dummy', str(path), 'b' * 40],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertIn(b'QUARANTINE_APPROVAL_ROOT_INVALID', proc.stderr)
        root.write_bytes(registry)
        manifest = approval.decode(self.signed())
        manifest['signature'] = base64.b64encode(b'\0' * 384).decode('ascii')
        path.write_bytes(approval.canonical(manifest))
        proc = subprocess.run([sys.executable, str(cli_script),
                               'dummy', 'dummy', str(path), 'b' * 40],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertIn(b'QUARANTINE_APPROVAL_SIGNATURE_INVALID', proc.stderr)
        self.assertNotIn(b'synthetic-task', proc.stderr)

    def test_exact_signed_bytes_pin_detects_canonical_equivalent_rewrite(self):
        path = self.work / 'signed-bytes.json'
        raw = self.signed()
        path.write_bytes(raw + b'\n')
        for script, args in [('enroll-quarantine.py', ['dummy', 'dummy', str(path), 'b' * 40]),
                             ('verify-safe-drain.py', ['dummy', 'dummy', '0', '--manifest', str(path),
                                                      '--release-sha', 'b' * 40])]:
            proc = subprocess.run([sys.executable, str(ROOT / 'ops' / script)] + args +
                                  ['--expected-manifest-sha256', hashlib.sha256(raw).hexdigest()],
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertEqual(proc.returncode, 1)
            self.assertIn(b'SHA256 mismatch', proc.stderr)

    def test_cli_docker_failure_fails_closed(self):
        path = self.work / 'manifest.json'
        path.write_bytes(self.signed())
        shim = self.work / 'bin'
        shim.mkdir(exist_ok=True)
        (shim / 'docker').write_text('#!/bin/sh\nexit 91\n')
        os.chmod(str(shim / 'docker'), 0o755)
        env = dict(os.environ, PATH=str(shim) + ':' + os.environ['PATH'],
                   RELEASE_TRUST_SECRET='synthetic-legacy-secret-cannot-authorize')
        for script, args in [('enroll-quarantine.py', ['dummy', 'dummy', str(path), 'b' * 40]),
                             ('verify-safe-drain.py', ['dummy', 'dummy', '0', '--manifest', str(path),
                                                      '--release-sha', 'b' * 40])]:
            with self.subTest(script=script):
                proc = subprocess.run([sys.executable, str(ROOT / 'ops' / script)] + args,
                                      env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(proc.returncode, 1)
                self.assertIn(b'ERROR:', proc.stderr)


if __name__ == '__main__':
    unittest.main()
