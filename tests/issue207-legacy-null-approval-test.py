"""Offline #207 guards; no production database, signing key or Docker required."""
import datetime
import importlib.util
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


def source(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


approval = source('issue207_approval', 'ops/quarantine-approval.py')
snapshot = source('issue207_snapshot', 'ops/quarantine-live-snapshot.py')


class MigrationCursor:
    def __init__(self, applied, task_created, duplicate=False):
        self.applied, self.task_created, self.duplicate = applied, task_created, duplicate

    def execute(self, sql, parameters):
        if 'schema_migrations' in sql:
            self.rows = [(self.applied,)] * (2 if self.duplicate else 1)
        elif 'xz_generation_tasks' in sql:
            self.rows = [(self.task_created,)]
        else:
            raise AssertionError('unexpected query')

    def fetchall(self):
        return self.rows


class LegacyNullApprovalTests(unittest.TestCase):
    def setUp(self):
        self.entries = [dict(execution_id=901 + i, task_id='synthetic-task-%d' % i,
                             attempt=1, generation=None, task_generation=2) for i in range(6)]
        pins = frozenset(approval.legacy_identity_sha256(item['execution_id'], item['task_id'], 1)
                         for item in self.entries)
        self.old_approval = approval.LEGACY_NULL_IDENTITY_PINS
        self.old_snapshot = snapshot._approval.LEGACY_NULL_IDENTITY_PINS
        approval.LEGACY_NULL_IDENTITY_PINS = pins
        snapshot._approval.LEGACY_NULL_IDENTITY_PINS = pins
        self.applied = datetime.datetime(2026, 9, 18, 8, 20, 9, 578256,
                                         tzinfo=datetime.timezone.utc)
        self.created = datetime.datetime(2026, 9, 3, 10, 9, 43, 405950,
                                         tzinfo=datetime.timezone.utc)

    def tearDown(self):
        approval.LEGACY_NULL_IDENTITY_PINS = self.old_approval
        snapshot._approval.LEGACY_NULL_IDENTITY_PINS = self.old_snapshot

    def evidence(self, entry):
        return snapshot._legacy_generation_evidence(
            MigrationCursor(self.applied, '2026-09-03T10:09:43.350748981Z'),
            entry, self.created)

    def record(self, entry):
        return dict(version=approval.LIVE_SNAPSHOT_VERSION,
                    scope='CANONICAL_LIVE_SNAPSHOT', **entry,
                    task_execution_generation=None, legacy_generation_unverifiable=True,
                    generation_resolution_reason=approval.LEGACY_GENERATION_REASON,
                    generation_resolution_evidence=self.evidence(entry))

    def candidate(self, entries, snapshots):
        now = datetime.datetime.now(datetime.timezone.utc)
        fmt = '%Y-%m-%dT%H:%M:%S.%fZ'
        return approval.build_unsigned_candidate(
            entries, snapshots, 'b' * 40, 'UNPROVISIONED',
            (now - datetime.timedelta(minutes=1)).strftime(fmt),
            (now + datetime.timedelta(hours=1)).strftime(fmt), now)

    def test_exact_six_nulls_bind_unsigned_candidate_not_an_approval(self):
        self.assertEqual(len(self.old_approval), 6, 'production source must pin six exact identities')
        records = {item['execution_id']: self.record(item) for item in self.entries}
        candidate = self.candidate(self.entries, records)
        self.assertEqual(candidate['record_count'], 6)
        self.assertEqual([r['identity']['generation'] for r in candidate['records']], [None] * 6)
        bindings = [dict(execution_id=i['execution_id'], approval_id='review-%d' % i['execution_id'],
                         review_sha256='a' * 64) for i in self.entries]
        manifest = approval.decode(approval.unsigned_manifest_bytes(candidate, bindings))
        self.assertEqual([e['generation'] for e in manifest['executions']], [None] * 6)
        self.assertNotIn('signature', manifest)
        with self.assertRaises(approval.ApprovalError):
            approval.verify(approval.canonical(manifest), 'b' * 40, 'enroll',
                            datetime.datetime.now(datetime.timezone.utc))

    def test_unlisted_post119_and_forged_generation_fail_closed(self):
        entry = self.entries[0]
        unlisted = dict(entry, execution_id=1000)
        with self.assertRaises(approval.ApprovalError):
            self.candidate([unlisted], {1000: dict(self.record(entry), execution_id=1000)})
        with self.assertRaisesRegex(snapshot.SnapshotError, 'LEGACY_IDENTITY_NOT_PINNED'):
            snapshot._entries([unlisted])
        with self.assertRaisesRegex(snapshot.SnapshotError, 'LEGACY_MIGRATION_PROOF_INVALID'):
            snapshot._legacy_generation_evidence(
                MigrationCursor(self.applied, '2026-09-19T00:00:00.000000000Z'),
                entry, self.created)
        with self.assertRaisesRegex(snapshot.SnapshotError, 'LEGACY_MIGRATION_PROOF_INVALID'):
            snapshot._legacy_generation_evidence(
                MigrationCursor(self.applied, '2026-09-03T10:09:43Z'),
                entry, self.applied + datetime.timedelta(seconds=1))
        with self.assertRaisesRegex(snapshot.SnapshotError, 'AMBIGUOUS_COUNT'):
            snapshot._legacy_generation_evidence(
                MigrationCursor(self.applied, '2026-09-03T10:09:43Z', duplicate=True),
                entry, self.created)
        fabricated = dict(entry, generation=1)
        with self.assertRaises(approval.ApprovalError):
            self.candidate([fabricated], {entry['execution_id']: dict(self.record(entry), generation=1)})

    def test_snapshot_identity_and_evidence_tampering_rejected(self):
        entry = self.entries[0]
        original = self.record(entry)
        for mutation in (dict(original, task_id='other'),
                         dict(original, legacy_generation_unverifiable=False),
                         dict(original, generation_resolution_reason='guessed-generation'),
                         dict(original, generation_resolution_evidence=dict(
                             original['generation_resolution_evidence'], identity_sha256='0' * 64))):
            with self.subTest(mutation=mutation):
                with self.assertRaises(approval.ApprovalError):
                    self.candidate([entry], {entry['execution_id']: mutation})


if __name__ == '__main__':
    unittest.main()
