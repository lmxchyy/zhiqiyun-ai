#!/usr/bin/env python3
"""Actual owned Docker/PG cold-fence replay, never a stub/runtime PASS on absence.

Local daemon only; UUID project/images, network-none PostgreSQL, no host ports.
Setup SQL belongs exclusively to this disposable fixture. Recovery makes no SQL
calls. This is not official image capability/provenance or production evidence.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import types
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[1]
LABEL = 'org.xianzhi.cold-recovery-test-owner'


def module(path):
    result = types.ModuleType(path.stem.replace('-', '_'))
    result.__file__ = str(path)
    exec(compile(path.read_bytes(), str(path), 'exec'), result.__dict__)
    return result


cold = module(ROOT / 'ops/first-upgrade-cold.py')


class OwnedDocker:
    def __init__(self, docker, owner):
        self.docker, self.owner = docker, owner
        self.fault, self.calls = None, []

    def inspect(self, kind, identity):
        obj = self.docker.inspect(kind, identity)
        if (kind == 'container' and self.fault == ('inspect', identity) and
                not obj['State']['Running']):
            raise cold.Refused('owned inspection fault')
        return obj

    def text(self, args, **kwargs):
        self.calls.append(list(args))
        if args[0] in ('stop', 'update'):
            obj = self.docker.inspect('container', args[-1])
            if obj['Config']['Labels'].get(LABEL) != self.owner:
                raise AssertionError('mutation outside fixture owner')
            if self.fault == (args[0], args[-1]):
                raise cold.Refused('owned stop/update fault')
        return self.docker.text(args, **kwargs)


class DockerTests(unittest.TestCase):
    def setUp(self):
        self.docker = cold.capability.Docker()
        # Do not honor an alternate current Docker context or remote endpoint.
        self.docker.cli += ['--host', 'unix:///var/run/docker.sock'] if os.name != 'nt' else []
        self.docker.text(['info'], timeout=20)  # unavailable daemon is ERROR, not skip
        self.temp = tempfile.TemporaryDirectory(prefix='owned-cold-docker-')
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.owner = uuid.uuid4().hex
        self.project = 'cold-' + self.owner
        self.resources, self.images = [], []
        self.addCleanup(self.cleanup_resources)
        self.old_secret = os.environ.get('RELEASE_TRUST_SECRET')
        os.environ['RELEASE_TRUST_SECRET'] = 'owned-cold-docker-fixture-only'
        self.addCleanup(self.restore_secret)
        self.transport = OwnedDocker(self.docker, self.owner)
        # Build unique image IDs so global exact-image fencing cannot match any
        # shared postgres/business image outside this owned fixture.
        base_ref = 'pgvector/pgvector:pg16'
        base = self.docker.inspect('image', base_ref)['Id']
        ids = []
        for role in ('old', 'target'):
            tag = self.project + ':' + role
            context = self.directory / role
            context.mkdir()
            # BuildKit resolves FROM as an image reference, not an image ID.
            # Use the already-pulled local tag and recheck its exact identity.
            (context / 'Dockerfile').write_text('FROM ' + base_ref + '\nLABEL ' + LABEL + '="' + self.owner + '" fixture.role="' + role + '"\n')
            self.assertEqual(self.docker.inspect('image', base_ref)['Id'], base)
            self.docker.text(['build', '--pull=false', '--network', 'none', '-t', tag, str(context)], timeout=90)
            self.assertEqual(self.docker.inspect('image', base_ref)['Id'], base)
            obj = self.docker.inspect('image', tag)
            self.assertEqual(obj['Config']['Labels'][LABEL], self.owner)
            self.images.append((tag, obj['Id']))
            ids.append(obj['Id'])
        self.old_image, self.target_image = ids
        self.db = self.run_owned('postgres', base, ['-e', 'POSTGRES_USER=fixture_admin', '-e', 'POSTGRES_PASSWORD=owned-test', '-e', 'POSTGRES_DB=owned_test', '--tmpfs', '/var/lib/postgresql/data'])
        for unused in range(60):
            result = self.docker.run(['exec', self.db, 'psql', '-X', '-U', 'fixture_admin', '-d', 'owned_test', '-Atqc', 'SELECT 1'], check=False)
            if result.returncode == 0 and result.stdout.strip() == b'1': break
            time.sleep(0.5)
        else: self.fail('owned PostgreSQL did not become ready')
        # Reuse the existing independently observed business fixture seed.
        self.fixture = cold.capability.Fixture(self.docker)
        self.fixture.owner, self.fixture.name, self.fixture.db = self.owner, 'owned_test', self.db
        self.fixture.resources = [('container', self.db)]
        self.fixture.record_path = str(self.directory / 'fixture.json')
        # Fixture.owned checks its existing label; DB has both fixture labels.
        schema = (ROOT / 'database/schema.sql').read_text(encoding='utf-8')
        for path in sorted((ROOT / 'database/migrations').glob('*.sql')):
            if re.match(r'^[0-9]{3}-', path.name) and not path.name.endswith('.down.sql'):
                schema += '\n' + path.read_text(encoding='utf-8')
        self.sql(schema)
        self.fixture.seed(True)
        self.before = self.snapshot()
        self.old_roles = []
        for role in ('xianzhi-ai', 'generation-worker', 'smartvideo-worker', 'video-backfill', 'proxy'):
            self.old_roles.append(self.run_owned(role, self.old_image, ['--restart', 'always', '--entrypoint', '/bin/sleep'], ['infinity']))
        # A globally exact business image outside Compose must also be fenced.
        self.global_role = self.run_owned('exact-global', self.old_image, ['--restart', 'always', '--entrypoint', '/bin/sleep'], ['infinity'], project='other-' + self.owner)
        self.hold = {'version': 1, 'state': 'ARMED_RECOVERY_REQUIRED', 'owner': self.owner,
                     'release_sha': 'owned-synthetic-not-official', 'proof_sha256': '0' * 64,
                     'project': self.project, 'image_ids': ids, 'stopped_services': [], 'observed_state': []}
        (self.directory / cold.HOLD).mkdir()
        cold.write_hold(str(self.directory), self.hold)

    def restore_secret(self):
        if self.old_secret is None: os.environ.pop('RELEASE_TRUST_SECRET', None)
        else: os.environ['RELEASE_TRUST_SECRET'] = self.old_secret

    def run_owned(self, role, image, options, command=None, project=None):
        args = ['run', '-d', '--pull', 'never', '--network', 'none', '--label', LABEL + '=' + self.owner,
                '--label', cold.capability.LABEL + '=' + self.owner,
                '--label', 'com.docker.compose.project=' + (project or self.project),
                '--label', 'com.docker.compose.service=' + role] + options + [image] + (command or [])
        cid = self.docker.text(args)
        obj = self.docker.inspect('container', cid)
        self.assertEqual(obj['Config']['Labels'][LABEL], self.owner)
        self.resources.append(obj['Id'])
        return obj['Id']

    def cleanup_resources(self):
        errors = []
        # Include partially created Compose resources even if creation/ps failed.
        owned = self.docker.text(['ps', '-aq', '--no-trunc', '--filter', 'label=' + LABEL + '=' + self.owner]).splitlines()
        for cid in owned:
            if cid not in self.resources: self.resources.append(cid)
        for cid in reversed(self.resources):
            try:
                obj = self.docker.inspect('container', cid)
                if obj['Id'] != cid or obj['Config']['Labels'].get(LABEL) != self.owner:
                    raise AssertionError('cleanup owner mismatch')
                self.docker.text(['rm', '-f', cid])
            except Exception as error: errors.append(str(error))
        for tag, image in reversed(self.images):
            try:
                obj = self.docker.inspect('image', tag)
                if obj['Id'] != image or obj['Config']['Labels'].get(LABEL) != self.owner:
                    raise AssertionError('image cleanup owner mismatch')
                self.docker.text(['image', 'rm', tag])  # never delete shared base ID
            except Exception as error: errors.append(str(error))
        if errors: raise AssertionError('owned cleanup failed: ' + '; '.join(errors))

    def sql(self, query):
        obj = self.docker.inspect('container', self.db)
        self.assertEqual(obj['Config']['Labels'][LABEL], self.owner)
        return self.docker.run(['exec', '-i', self.db, 'psql', '-X', '-U', 'fixture_admin', '-d', 'owned_test', '-v', 'ON_ERROR_STOP=1', '-At'], data=query.encode('utf-8'), timeout=180).stdout.decode().strip()

    def snapshot(self):
        tables = self.sql("SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename;").splitlines()
        return {table: self.sql('BEGIN READ ONLY; SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),\'[]\') FROM "' + table + '" t; COMMIT;') for table in tables}

    def test_actual_pg_no_writes_creation_no_restart_failure_and_reentry(self):
        # Actual normalized Compose creation: poisonous business commands would
        # write if recovery erroneously started them. No helper start action exists.
        poison = "PGPASSWORD=owned-test psql -h 127.0.0.1 -U fixture_admin -d owned_test -c \"UPDATE xz_generation_tasks SET status='FAILED';\""
        model = {'name': self.project, 'services': {}}
        for role in ('xianzhi-ai', 'generation-worker', 'smartvideo-worker', 'video-backfill', 'proxy'):
            model['services'][role] = {'image': self.target_image, 'restart': 'always',
                'network_mode': 'container:' + self.db, 'entrypoint': ['/bin/sh', '-c'], 'command': [poison],
                'labels': {LABEL: self.owner, cold.capability.LABEL: self.owner}}
        config = self.directory / 'cold-compose.json'
        cold.atomic_json(str(config), cold.cold_model(model))
        # Different project avoids Compose replacing the running old-role fixture.
        created_model = cold.read_json(str(config))
        created_model['name'] = 'new-' + self.project
        cold.atomic_json(str(config), created_model)
        self.docker.text(['compose', '-f', str(config), 'create', '--no-build', '--pull', 'never'], timeout=60)
        created = self.docker.text(['compose', '-f', str(config), 'ps', '-aq']).splitlines()
        self.assertEqual(len(created), 5)
        for cid in created:
            obj = self.docker.inspect('container', cid)
            self.assertEqual(obj['Config']['Labels'][LABEL], self.owner)
            self.resources.append(obj['Id'])
            self.assertEqual(obj['State']['Status'], 'created')
            self.assertEqual(obj['HostConfig']['RestartPolicy']['Name'], 'no')
        # Real stop/update/inspection failures may never produce a stopped PASS.
        for fault in ('update', 'stop', 'inspect'):
            self.transport.fault = (fault, self.old_roles[1])
            with self.assertRaises(cold.Refused):
                cold.fence(str(self.directory), 'isolated-fault', 'exit-failure', self.transport)
            self.assertEqual(cold.read_hold(str(self.directory))['state'], 'RECOVERY_REQUIRED_UNKNOWN')
            self.assertEqual(self.before, self.snapshot())
        self.transport.fault = None
        for reason in ('exit-failure', 'reentry'):
            cold.fence(str(self.directory), 'isolated-replay', reason, self.transport)
            hold = cold.read_hold(str(self.directory))
            self.assertEqual(hold['state'], 'STOPPED_RECOVERY_REQUIRED')
            self.assertEqual(hold['release_sha'], 'owned-synthetic-not-official')
            for cid in self.old_roles + [self.global_role] + created:
                obj = self.docker.inspect('container', cid)
                self.assertFalse(obj['State']['Running'])
                self.assertEqual(obj['HostConfig']['RestartPolicy']['Name'], 'no')
            self.assertTrue(self.docker.inspect('container', self.db)['State']['Running'])
            self.assertEqual(self.before, self.snapshot())
        # SIGKILL of a NEW process whose policy was no at creation does not restart.
        new = self.run_owned('new-kill-probe', self.target_image, ['--restart', 'no', '--entrypoint', '/bin/sleep'], ['infinity'])
        self.docker.text(['kill', '--signal', 'KILL', new])
        time.sleep(2)
        obj = self.docker.inspect('container', new)
        self.assertFalse(obj['State']['Running'])
        self.assertEqual(obj['RestartCount'], 0)
        self.assertEqual(self.before, self.snapshot())
        self.assertTrue(all(c[0] in ('ps', 'stop', 'update') for c in self.transport.calls))
        self.assertFalse(any(c[0] in ('exec', 'run', 'start', 'restart', 'compose') for c in self.transport.calls))


if __name__ == '__main__':
    unittest.main()
