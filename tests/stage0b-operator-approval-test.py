"""Tests for Stage 0B Operator Release Authorization and Quarantine Approval."""
import copy
import datetime
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def source(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


approval = source('quarantine_approval', 'ops/quarantine-approval.py')
TEST_TRUST_SECRET = "ci-test-release-trust-secret-only"


class Stage0BOperatorApprovalTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp(prefix='test-operator-approval-')
        os.environ['RELEASE_TRUST_SECRET'] = TEST_TRUST_SECRET
        self.now = datetime.datetime.now(datetime.timezone.utc)
        self.fmt = '%Y-%m-%dT%H:%M:%S.%fZ'
        self.nb = (self.now - datetime.timedelta(minutes=5)).strftime(self.fmt)
        self.exp = (self.now + datetime.timedelta(hours=1)).strftime(self.fmt)
        self.release_sha = 'a' * 40

        self.entries = [
            {'execution_id': 901, 'task_id': 'task-1', 'attempt': 1, 'generation': 1, 'task_generation': 1},
            {'execution_id': 902, 'task_id': 'task-2', 'attempt': 1, 'generation': 1, 'task_generation': 1},
        ]
        self.snapshots = {
            901: {'version': approval.LIVE_SNAPSHOT_VERSION, 'scope': 'CANONICAL_LIVE_SNAPSHOT',
                  'execution_id': 901, 'task_id': 'task-1', 'attempt': 1, 'generation': 1, 'task_generation': 1,
                  'core': {'hash': '1' * 64}},
            902: {'version': approval.LIVE_SNAPSHOT_VERSION, 'scope': 'CANONICAL_LIVE_SNAPSHOT',
                  'execution_id': 902, 'task_id': 'task-2', 'attempt': 1, 'generation': 1, 'task_generation': 1,
                  'core': {'hash': '2' * 64}},
        }

    def tearDown(self):
        os.environ.pop('RELEASE_TRUST_SECRET', None)
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_build_unsigned_candidate_defaults_to_operator_algorithm(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        self.assertEqual(candidate['status'], 'UNSIGNED_REQUIRES_HUMAN_REVIEW')
        self.assertEqual(candidate['algorithm'], approval.OPERATOR_ALGORITHM)
        self.assertEqual(candidate['authority_id'], approval.OPERATOR_AUTHORITY_ID)
        self.assertNotIn('signature', candidate)
        self.assertEqual(candidate['record_count'], 2)

    def test_approve_candidate_requires_operator_identity(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        with self.assertRaises(approval.ApprovalError):
            approval.approve_candidate(candidate, operator_identity='')

    def test_approve_candidate_requires_trust_key(self):
        os.environ.pop('RELEASE_TRUST_SECRET', None)
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        with self.assertRaises(approval.ApprovalError):
            approval.approve_candidate(candidate, operator_identity='operator@ssh-1.2.3.4')

    def test_approve_and_verify_success(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        signed_bytes = approval.approve_candidate(
            candidate, operator_identity='admin@ssh-10.0.0.1',
            approval_id='test-approval-001', secret=TEST_TRUST_SECRET, now=self.now
        )
        manifest = approval.decode(signed_bytes)
        self.assertEqual(manifest['approved_count'], 2)
        self.assertIn('signature', manifest)
        self.assertIn('approval_audit', manifest)
        self.assertEqual(manifest['approval_audit']['operator'], 'admin@ssh-10.0.0.1')
        self.assertEqual(manifest['approval_audit']['approval_id'], 'test-approval-001')

        # Verify passes with valid release_sha and clock
        verified = approval.verify(signed_bytes, self.release_sha, 'enroll', self.now)
        self.assertEqual(verified['approved_count'], 2)

        # Also verifies for drain-exemption operation
        verified_drain = approval.verify(signed_bytes, self.release_sha, 'drain-exemption', self.now)
        self.assertEqual(verified_drain['approved_count'], 2)

    def test_verify_rejects_wrong_release_sha(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        signed_bytes = approval.approve_candidate(candidate, operator_identity='admin@ssh')
        with self.assertRaises(approval.ApprovalError):
            approval.verify(signed_bytes, 'b' * 40, 'enroll', self.now)

    def test_verify_rejects_tampered_signature(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        signed_bytes = approval.approve_candidate(candidate, operator_identity='admin@ssh')
        manifest = approval.decode(signed_bytes)
        manifest['signature'] = '0' * 64  # tampered
        with self.assertRaises(approval.ApprovalError):
            approval.verify(approval.canonical(manifest), self.release_sha, 'enroll', self.now)

    def test_verify_rejects_tampered_execution(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        signed_bytes = approval.approve_candidate(candidate, operator_identity='admin@ssh')
        manifest = approval.decode(signed_bytes)
        manifest['executions'][0]['task_id'] = 'tampered-task'
        with self.assertRaises(approval.ApprovalError):
            approval.verify(approval.canonical(manifest), self.release_sha, 'enroll', self.now)

    def test_verify_rejects_tampered_audit(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        signed_bytes = approval.approve_candidate(candidate, operator_identity='admin@ssh')
        manifest = approval.decode(signed_bytes)
        manifest['approval_audit']['operator'] = 'impersonated-user'
        with self.assertRaises(approval.ApprovalError):
            approval.verify(approval.canonical(manifest), self.release_sha, 'enroll', self.now)

    def test_verify_rejects_outside_window(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        signed_bytes = approval.approve_candidate(candidate, operator_identity='admin@ssh')
        # Before window
        before = self.now - datetime.timedelta(hours=2)
        with self.assertRaises(approval.ApprovalError):
            approval.verify(signed_bytes, self.release_sha, 'enroll', before)
        # After window
        after = self.now + datetime.timedelta(hours=2)
        with self.assertRaises(approval.ApprovalError):
            approval.verify(signed_bytes, self.release_sha, 'enroll', after)

    def test_cli_approve_and_verify(self):
        candidate = approval.build_unsigned_candidate(
            self.entries, self.snapshots, self.release_sha, 'release-trust-key',
            self.nb, self.exp, self.now
        )
        cand_path = os.path.join(self.tmpdir, 'candidate.json')
        with open(cand_path, 'wb') as f:
            f.write(approval.canonical(candidate))

        manifest_path = os.path.join(self.tmpdir, 'manifest.json')

        # CLI approve without --confirm APPROVE fails
        proc_fail = subprocess.run([
            sys.executable, str(ROOT / 'ops/quarantine-approval.py'), 'approve',
            '--candidate', cand_path,
            '--release-sha', self.release_sha,
            '--confirm', 'NOT-APPROVED',
            '--output', manifest_path,
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertNotEqual(proc_fail.returncode, 0)

        # CLI approve with --confirm APPROVE succeeds
        proc_ok = subprocess.run([
            sys.executable, str(ROOT / 'ops/quarantine-approval.py'), 'approve',
            '--candidate', cand_path,
            '--release-sha', self.release_sha,
            '--operator', 'root@mxcp',
            '--confirm', 'APPROVE',
            '--output', manifest_path,
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(proc_ok.returncode, 0)
        out_json = json.loads(proc_ok.stdout.decode('utf-8'))
        self.assertEqual(out_json['status'], 'APPROVED')
        self.assertEqual(out_json['approved_count'], 2)

        # CLI verify succeeds
        proc_verify = subprocess.run([
            sys.executable, str(ROOT / 'ops/quarantine-approval.py'), 'verify',
            '--manifest', manifest_path,
            '--release-sha', self.release_sha,
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(proc_verify.returncode, 0)
        verify_json = json.loads(proc_verify.stdout.decode('utf-8'))
        self.assertEqual(verify_json['status'], 'VALID')

    def test_legacy_null_generation_approved_and_verified(self):
        # Verify that pinned legacy null generation records can be approved and verified
        entry = dict(execution_id=999, task_id='task-legacy', attempt=1, generation=None, task_generation=1)
        old_pins = approval.LEGACY_NULL_IDENTITY_PINS
        approval.LEGACY_NULL_IDENTITY_PINS = frozenset((
            approval.legacy_identity_sha256(999, 'task-legacy', 1),
        ))
        try:
            snapshot = {
                'version': approval.LIVE_SNAPSHOT_VERSION, 'scope': 'CANONICAL_LIVE_SNAPSHOT',
                'execution_id': 999, 'task_id': 'task-legacy', 'attempt': 1, 'generation': None, 'task_generation': 1,
                'task_execution_generation': None, 'legacy_generation_unverifiable': True,
                'generation_resolution_reason': approval.LEGACY_GENERATION_REASON,
                'generation_resolution_evidence': {
                    'identity_sha256': approval.legacy_identity_sha256(999, 'task-legacy', 1),
                    'migration119_applied_at': '2026-09-18T08:20:09.578256Z',
                    'execution_created_at': '2026-09-03T10:09:43.000000Z',
                    'task_created_at': '2026-09-03T10:09:43.000000Z',
                }
            }
            candidate = approval.build_unsigned_candidate(
                [entry], {999: snapshot}, self.release_sha, 'release-trust-key',
                self.nb, self.exp, self.now
            )
            self.assertEqual(candidate['records'][0]['identity']['generation'], None)
            signed_bytes = approval.approve_candidate(candidate, operator_identity='admin@ssh')
            verified = approval.verify(signed_bytes, self.release_sha, 'enroll', self.now)
            self.assertEqual(verified['executions'][0]['generation'], None)
            self.assertEqual(verified['approved_count'], 1)
        finally:
            approval.LEGACY_NULL_IDENTITY_PINS = old_pins


if __name__ == '__main__':
    unittest.main()
