"""CI-owned, network-none PostgreSQL replay of the #207 fixed psql query contract.

This is intentionally smaller than the local Issue 203 full transport harness: it
executes the three new SELECTs through the real docker-exec/psql Connection.
"""
import datetime
import importlib.util
import os
from pathlib import Path
import stat
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
LABEL = 'issue207.psql-transport.owner'
IMAGE = 'pgvector/pgvector:pg16'  # Already pulled by the user-core CI service.


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, str(ROOT / path))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def command(args, data=None, timeout=30):
    result = subprocess.run(args, input=data, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout)
    if result.returncode:
        raise RuntimeError('owned fixture command failed: ' + args[0])
    return result.stdout.decode('utf-8').strip()


class OwnedTarget:
    def __init__(self, container_id):
        self.expected = {'container_id': container_id}

    def check(self):
        # This test owns and rechecks its UUID-labelled container, not a
        # production Compose binding or a release authorization.
        rows = command(['docker', 'inspect', '--format', '{{.Id}}|{{index .Config.Labels "' + LABEL + '"}}',
                        self.expected['container_id']]).split('|')
        if rows != [self.expected['container_id'], self.owner]:
            raise RuntimeError('owned fixture identity changed')


def main():
    if (os.name != 'posix' or os.environ.get('GITHUB_ACTIONS') != 'true' or
            os.environ.get('DOCKER_HOST') or os.environ.get('DOCKER_CONTEXT')):
        raise RuntimeError('CI local Docker only; never use a remote endpoint')
    if command(['docker', 'context', 'show']) != 'default':
        raise RuntimeError('default local Docker context required')
    docker_socket = Path('/var/run/docker.sock')
    if not docker_socket.exists() or not stat.S_ISSOCK(docker_socket.stat().st_mode):
        raise RuntimeError('local Docker socket required')
    owner = uuid.uuid4().hex
    name = 'issue207-transport-' + owner
    container_id = None
    try:
        container_id = command(['docker', 'run', '-d', '--pull', 'never', '--network', 'none',
                                '--name', name, '--label', LABEL + '=' + owner,
                                '-e', 'POSTGRES_USER=issue207', '-e', 'POSTGRES_PASSWORD=synthetic-only',
                                '-e', 'POSTGRES_DB=issue207', IMAGE])
        target = OwnedTarget(container_id)
        target.owner = owner
        target.check()
        for _ in range(40):
            ready = subprocess.run(['docker', 'exec', container_id, 'pg_isready', '-q', '-U', 'issue207'],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5)
            if ready.returncode == 0:
                break
            time.sleep(.25)
        else:
            raise RuntimeError('owned PG not ready')
        sql = """CREATE TABLE public.schema_migrations(filename text PRIMARY KEY, applied_at timestamptz NOT NULL);
CREATE TABLE public.xz_generation_tasks(id text PRIMARY KEY, created_at text NOT NULL);
CREATE TABLE public.provider_executions(id bigint PRIMARY KEY,task_id text,attempt integer,
 task_execution_generation bigint,status text,provider text,provider_channel text,
 provider_model text,capability text,request_fingerprint char(64),created_at timestamptz);
INSERT INTO public.schema_migrations VALUES('119-execution-generation-fencing.sql','2026-09-18T08:20:09.578256Z');
INSERT INTO public.xz_generation_tasks VALUES('synthetic-legacy','2026-09-03T10:09:43.350748981Z');
INSERT INTO public.provider_executions VALUES(901,'synthetic-legacy',1,NULL,'unknown',
 'synthetic','synthetic','synthetic','image',repeat('a',64),'2026-09-03T10:09:43.405950Z');
"""
        command(['docker', 'exec', '-i', container_id, 'psql', '-X', '-v', 'ON_ERROR_STOP=1',
                 '-U', 'issue207', '-d', 'issue207'], sql.encode('utf-8'))
        transport = module('issue207_transport', 'ops/quarantine-psql-transport.py')
        live = module('issue207_live', 'ops/quarantine-live-snapshot.py')
        identity = dict(execution_id=901, task_id='synthetic-legacy', attempt=1,
                        generation=None, task_generation=1)
        previous = live._approval.LEGACY_NULL_IDENTITY_PINS
        live._approval.LEGACY_NULL_IDENTITY_PINS = frozenset((
            live._approval.legacy_identity_sha256(901, 'synthetic-legacy', 1),))
        try:
            connection = transport.Connection(target)
            try:
                cursor = connection.cursor()
                cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
                # The source's exact SELECT, wrapped by _query, must be on the
                # transport whitelist and decode timestamp/null/char(64) types.
                rows = live._query(cursor, """SELECT id,task_id,attempt,task_execution_generation,status,provider,provider_channel,
       provider_model,capability,request_fingerprint,created_at
FROM public.provider_executions WHERE task_id=%s ORDER BY id""", ('synthetic-legacy',))
                assert len(rows) == 1 and rows[0][0] == 901 and rows[0][3] is None
                assert rows[0][10] == datetime.datetime(2026, 9, 3, 10, 9, 43, 405950,
                                                        tzinfo=datetime.timezone.utc)
                evidence = live._legacy_generation_evidence(cursor, identity, rows[0][10])
                assert evidence['identity_sha256'] == live._approval.legacy_identity_sha256(
                    901, 'synthetic-legacy', 1)
                assert evidence['migration119_applied_at'] == '2026-09-18T08:20:09.578256Z'
                cursor.execute('ROLLBACK')
            finally:
                connection.close()
                live._approval.LEGACY_NULL_IDENTITY_PINS = previous
        finally:
            target.check()
        print('ISSUE207_OWNED_PSQL_TRANSPORT PASS: fixed queries, typed NULL/timestamps, RR read-only')
    finally:
        if container_id is not None:
            info = subprocess.run(['docker', 'inspect', '--format', '{{.Id}}|{{index .Config.Labels "' + LABEL + '"}}',
                                   container_id], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
            if info.returncode == 0 and info.stdout.decode().strip() == container_id + '|' + owner:
                command(['docker', 'rm', '-f', container_id])


if __name__ == '__main__':
    main()
