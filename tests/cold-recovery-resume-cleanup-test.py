#!/usr/bin/env python3
"""Exact-identity cleanup refuses foreign fixture resources before mutation."""
import ast
import copy
import json
from pathlib import Path
import types
import tempfile
import unittest

path = Path(__file__).resolve().with_name('cold-recovery-resume-cleanup.py')
c = types.ModuleType('owned_cleanup')
c.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), c.__dict__)


class CleanupTests(unittest.TestCase):
    def setUp(self):
        self.owner, self.cid = 'a' * 32, 'b' * 64
        self.project = 'resume-real-' + self.owner
        self.label = 'org.xianzhi.cold-resume.backend-owner'
        self.obj = dict(Id=self.cid, Config={'Labels': {self.label: self.owner, 'com.docker.compose.project': self.project}})
        fixture = self
        class Docker:
            def __init__(self): self.mutations = []
            def run(self, args, **kwargs):
                return types.SimpleNamespace(returncode=0, stdout=json.dumps([fixture.obj]).encode(), stderr=b'')
            def text(self, args): self.mutations.append(args)
        self.docker = Docker()

    def remove(self, kind='container', identity=None):
        return c.remove(self.docker, kind, identity or self.cid, self.label, self.owner, self.project)

    def test_only_exact_verified_id_removed(self):
        self.assertEqual(self.remove(), 'removed')
        self.assertEqual(self.docker.mutations, [['rm', '-f', '-v', self.cid]])

    def test_foreign_owner_project_or_cid_refused_without_mutation(self):
        before = copy.deepcopy(self.obj)
        for field in ('owner', 'project', 'cid'):
            self.obj = copy.deepcopy(before)
            if field == 'owner': self.obj['Config']['Labels'][self.label] = 'foreign'
            if field == 'project': self.obj['Config']['Labels']['com.docker.compose.project'] = 'foreign'
            if field == 'cid': self.obj['Id'] = 'c' * 64
            with self.assertRaises(RuntimeError): self.remove()
            self.assertEqual(self.docker.mutations, [])

    def test_short_id_or_unrelated_volume_refused(self):
        for kind, identity in [('container', self.cid[:12]), ('volume', 'foreign-volume'), ('image', self.cid)]:
            with self.assertRaises(RuntimeError): self.remove(kind, identity)
        self.assertEqual(self.docker.mutations, [])

    def test_missing_is_distinct_from_observer_unavailable(self):
        self.docker.run = lambda *a, **k: types.SimpleNamespace(returncode=1, stdout=b'', stderr=b'No such container')
        self.assertEqual(self.remove(), 'already-absent')
        self.docker.run = lambda *a, **k: types.SimpleNamespace(returncode=1, stdout=b'', stderr=b'daemon unavailable')
        with self.assertRaises(RuntimeError): self.remove()
        self.assertEqual(self.docker.mutations, [])


class SanitizationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        old_out, old_docker = c.OUT, c.cap.Docker
        self.addCleanup(setattr, c, 'OUT', old_out)
        self.addCleanup(setattr, c.cap, 'Docker', old_docker)
        c.OUT = Path(self.temp.name) / 'evidence'
        self.owner = 'a' * 32
        self.directory = c.OUT / 'backend' / self.owner
        self.directory.mkdir(parents=True)
        self.record = self.directory / 'resources.json'
        self.data = dict(owner=self.owner, project='resume-real-' + self.owner, resources=[])
        self.record.write_text(json.dumps(self.data), encoding='utf-8')
        for name in c.PRIVATE:
            (self.directory / name).write_text('fixture-only-private', encoding='utf-8')

    def report(self):
        return json.loads((c.OUT / 'fixture-sanitization.json').read_text(encoding='utf-8'))

    def assert_removed(self):
        self.assertTrue(all(not (self.directory / name).exists() for name in c.PRIVATE))
        self.assertEqual(self.report()['status'], 'passed')

    def test_empty_manifest_sanitizes_before_first_resource(self):
        c.cap.Docker = lambda: self.fail('sanitization must not need Docker')
        c.sanitize_private()
        self.assert_removed()
        self.assertEqual(len(self.report()['removed']), 3)

    def test_daemon_initialization_failure_still_sanitizes(self):
        def unavailable(): raise RuntimeError('daemon unavailable')
        c.cap.Docker = unavailable
        with self.assertRaises(RuntimeError): c.main()
        self.assert_removed()
        cleanup = json.loads((c.OUT / 'owned-cleanup.json').read_text(encoding='utf-8'))
        self.assertTrue(cleanup['status'].startswith('failed'))

    def test_resource_observation_failure_still_sanitizes(self):
        self.data['resources'] = [['container', 'b' * 64]]
        self.record.write_text(json.dumps(self.data), encoding='utf-8')
        class Unavailable:
            def run(self, *args, **kwargs):
                return types.SimpleNamespace(returncode=1, stdout=b'', stderr=b'daemon unavailable')
            def text(self, args): raise AssertionError('no mutation without identity')
        c.cap.Docker = Unavailable
        with self.assertRaises(RuntimeError): c.main()
        self.assert_removed()

    def test_missing_or_foreign_manifest_blocks_publication(self):
        self.record.unlink()
        with self.assertRaises(RuntimeError): c.sanitize_private()
        self.assertEqual(self.report()['status'], 'failed')
        self.assertTrue((self.directory / c.PRIVATE[0]).exists())
        self.data['project'] = 'foreign'
        self.record.write_text(json.dumps(self.data), encoding='utf-8')
        with self.assertRaises(RuntimeError): c.sanitize_private()
        self.assertEqual(self.report()['status'], 'failed')

    def test_backend_manifest_precedes_private_writes_and_psql_is_quiet(self):
        source = (c.ROOT / 'tests/cold-recovery-resume-backend-test.py').read_text(encoding='utf-8')
        tree = ast.parse(source)
        fixture = next(n for n in tree.body if isinstance(n, ast.ClassDef) and n.name == 'RealBackendTests')
        setup = next(n for n in fixture.body if isinstance(n, ast.FunctionDef) and n.name == 'setUp')
        manifest_lines = [n.lineno for n in ast.walk(setup) if isinstance(n, ast.Call)
                          and isinstance(n.func, ast.Attribute) and n.func.attr == 'write'
                          and n.args and isinstance(n.args[0], ast.Str) and n.args[0].s == 'resources.json']
        private_lines = [n.lineno for n in ast.walk(setup) if isinstance(n, ast.Call)
                         and isinstance(n.func, ast.Attribute) and n.func.attr in ('write_text', 'provision')]
        self.assertEqual(len(manifest_lines), 1)
        self.assertTrue(private_lines and manifest_lines[0] < min(private_lines))
        self.assertIn("'psql', '-X', '-q', '-U'", source)

    def test_workflow_requires_independent_sanitization_and_exclusions(self):
        text = (c.ROOT / '.github/workflows/cold-recovery-resume.yml').read_text(encoding='utf-8')
        self.assertIn("if: always() && steps.fixture_sanitization.outcome == 'success'", text)
        self.assertIn('--sanitize-only', text)
        for name in c.PRIVATE:
            self.assertIn('!.evidence/cold-recovery-resume/backend/**/' + name, text)


if __name__ == '__main__':
    unittest.main(verbosity=2)
