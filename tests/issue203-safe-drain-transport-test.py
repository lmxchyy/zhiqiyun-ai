"""LOCAL Docker Linux/Desktop, UUID-owned real CLI transport replay; zero SKIP.

Run: python tests/issue203-safe-drain-transport-test.py
No production roots, resources, manifests or credentials. All logs use the
absolute ignored .evidence/issue203/safe-drain-transport path. Hard bounds:
build 300s, runner 900s. This proves priority2 only, not a release authorization.
"""
import ast
import datetime
import decimal
import hashlib
import hmac
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import types
import unittest
import uuid

ROOT = Path(__file__).resolve().parent.parent
EVIDENCE = ROOT / '.evidence/issue203/safe-drain-transport'
LABEL = 'issue203.safe-drain-transport.owner'
# Exact approved base bytes: #212 cold rollback fencing and the merged
# control-fixture inode-reuse correction superseded the stale P4 pins.
# Keep literal freezes: subsequent changes still reject, never recompute.
FROZEN = {'rollback.sh': 'b9fba73deee7e7d5135e9a7d6d951afe0e294eb1947b063f9d735d827b406184',
          'tests/issue203-control-plane-test.py': 'b3a799bf0fde1496f7362158d6fcfe5df1ca6cbaa0b5338dc5a187c9efce3b54'}
sys.dont_write_bytecode = True


# Only these fixed hash-family SELECTs have a canonical multiset contract.
# quarantine-live-snapshot._rows and _asset_family BOTH return
# sorted(projected, key=canonical). Their final SELECT has no ORDER BY.
# All other queries, including every ORDER BY query, compare in returned order.
CANONICAL_MULTISET_QUERIES = frozenset((
    '00a62133a363006646f4c11a7172d334c82cb001c2fce6f4691cff487aca1e78',  # aa)
    '11566e84e7a13e63175a7273943b600f3b8604c2261e16181b866f09f7f482b0',  # public.xz_user_wallets
    '122761d255d2571a693cdd2c3c41f60f76f0dfbf7ebf945f171854fd400257cc',  # ff)
    '27e980e1e05a940ccf97d0ab4de7a837d6df1555dc926386728b00ab895e84f2',  # public.xz_billing_events
    '5e2766950b3a40bf81639403ce28c5be128edce2ce4bdfd11153e136a699b928',  # public.xz_generation_tasks
    '694033b2df98465edcca4c6dc69b4ebb4ddb5c9ce3169ab97f1864a014c464e9',  # public.xz_wallet_ledger
    '6d8702519768359f1aba5bc660cd28875acb19a2f9c6ae60d880f4deb11043d7',  # rr)
    '7ad4efb385dcb2aeffad9b50aeb35becabe7d9ddf3a162423209be5b2b5d7a59',  # public.provider_execution_correlations
    '825cdf23ba3aeb34db027a175317353fe9b5509fe39e6af05c845b03feda41b1',  # jj)
    '903a54961ac99e9cd7e241bf1deae715fafea21c87bd9ef3e32665d8db9b7306',  # mm)
    'a4870757ad6d35e452f6651f18e0e677eb7cb0f048366b2bdeeddfc08e3efbf7',  # pp)
    'ab1a6f4d128fe411929251c44dde5d48ff2bde88f15fcb635fd76d39fb270d95',  # public.xz_personal_point_lots
    'b81b74b82c279e7de98b520269f07300a0c3e3afafd80e980de763b97e0a2ed4',  # public.provider_executions
    'cb0fc2509b0e2ed5cdc6dec28ebd12c81b8e240dbf6848d4fa6c044053ec3d2f',  # public.xz_point_accounts
    'ce08339fbb1967bdfe69df76aab82cc366e91eb31bcca88b5d635081a02484e9',  # public.xz_personal_point_lot_movements
    'cf8d07b052e00dd945cd46886bc7fc760589f75216b43e7605ea36f65074c4c0',  # public.xz_personal_point_reservations
    'da736af4280a3fb100ee733527028a1f36576cda515386cdae678a680fc32538',  # public.xz_billing_lifecycle_events
    'df4eea3a75c5217074dc6bbc3b5baf81dd62f30905e3216202b16b634bbc96bb',  # public.xz_personal_point_reservation_allocations
    'f184a45f409658b152f540171d13bcca99b85c96bd22a50f4275fc3381e946a0',  # cc)
))

def load(name, path):
    module = types.ModuleType(name)
    module.__file__ = str(path)
    exec(compile(path.read_bytes(), str(path), 'exec'), module.__dict__)
    return module


def command(args, **kwargs):
    kwargs.setdefault('stdout', subprocess.PIPE)
    kwargs.setdefault('stderr', subprocess.PIPE)
    kwargs.setdefault('timeout', 30)
    result = subprocess.run(args, **kwargs)
    if result.returncode:
        raise RuntimeError('fixture command failed: ' + args[0])
    return result.stdout.decode().strip()


def frozen():
    for name, expected in FROZEN.items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            raise RuntimeError('frozen source changed: ' + name)


class TransportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if sys.version_info[:3] != (3, 6, 8):
            raise RuntimeError('actual Python3.6.8 required, no SKIP')
        import psycopg2
        frozen()
        cls.owner = os.environ['ISSUE203_TRANSPORT_OWNER']
        cls.project = 'issue203transport' + cls.owner
        cls.work = Path('/work')
        cls.work.mkdir()
        cls.compose = cls.work / 'compose.json'
        cls.env = cls.work / 'fixture.env'
        cls.env.write_text('POSTGRES_USER=postgres\nPOSTGRES_DB=xianzhi_test\nPOSTGRES_PASSWORD=synthetic_transport_secret\n')
        queues = [dict(name='x.ai.generation.' + kind + '.canary', consumers=1,
                       messages_ready=0, messages_unacknowledged=0) for kind in ('image', 'video', 'ppt')]
        broker = "from http.server import BaseHTTPRequestHandler,HTTPServer\nclass H(BaseHTTPRequestHandler):\n def do_GET(self):\n  self.send_response(200);self.end_headers();self.wfile.write(" + repr(json.dumps(queues).encode()) + ")\n def log_message(self,*a): pass\nHTTPServer(('0.0.0.0',15672),H).serve_forever()"
        labels = {LABEL: cls.owner}
        cls.model = dict(name=cls.project, services={
            'postgres': dict(image='pgvector/pgvector:pg16', labels=labels,
                             environment={k: '${' + k + '}' for k in ('POSTGRES_USER', 'POSTGRES_DB', 'POSTGRES_PASSWORD')}),
            'xianzhi-ai': dict(image='python:3.6-alpine', labels=labels, command=['python3', '-c', broker],
                               environment={'RABBITMQ_URL': 'amqp://synthetic:synthetic@localhost/'}),
            'smartvideo-worker': dict(image='python:3.6-alpine', labels=labels, command=['sleep', '900']),
            'migrate': dict(image='pgvector/pgvector:pg16', labels=labels, command=['true'])},
            networks={'default': {'labels': labels}})
        cls.compose.write_text(json.dumps(cls.model))
        cls.cmd = ['docker', 'compose', '-f', str(cls.compose), '--env-file', str(cls.env)]
        command(cls.cmd + ['up', '-d', '--pull', 'never', 'postgres', 'xianzhi-ai', 'smartvideo-worker'], timeout=60)
        cls.cid = command(cls.cmd + ['ps', '-q', 'postgres'])
        cls.network = cls.project + '_default'
        command(['docker', 'network', 'connect', cls.network, os.environ['HOSTNAME']])
        for unused in range(40):
            try:
                cls.db = psycopg2.connect(host=cls.project + '-postgres-1', dbname='xianzhi_test', user='postgres', password='synthetic_transport_secret')
                break
            except psycopg2.OperationalError:
                time.sleep(.25)
        else:
            raise RuntimeError('owned PG failed to start')
        cls.db.autocommit = True
        paths = sorted(p for p in (ROOT / 'database/migrations').glob('[0-9][0-9][0-9]-*.sql') if not p.name.endswith('.down.sql'))
        sql = (ROOT / 'database/schema.sql').read_text() + '\n' + '\n'.join(p.read_text() for p in paths)
        # The real migration runner ledger is not in schema.sql. This fresh,
        # owner-labelled replay DB must reproduce it for historical observation.
        sql += "\nCREATE TABLE public.schema_migrations(filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());\nINSERT INTO public.schema_migrations(filename) VALUES('119-execution-generation-fencing.sql');"
        command(['docker', 'exec', '-i', cls.cid, 'psql', '-X', '-U', 'postgres', '-d', 'xianzhi_test', '-v', 'ON_ERROR_STOP=1'], input=sql.encode(), timeout=120)
        cls.live = load('live_tests', ROOT / 'tests/issue203-live-snapshot-test.py')
        cls.approval_tests = load('approval_tests', ROOT / 'tests/issue203-approval-test.py')
        cls.approval_tests.ApprovalTests.setUpClass()
        cls.live.CoreOnlyPostgresTests.approval_class = cls.approval_tests.ApprovalTests
        cls.live.CoreOnlyPostgresTests.db = cls.db
        cls.fixture = cls.live.CoreOnlyPostgresTests('test_internal_nine_row_stable_positive_and_privacy')
        cls.transport = load('transport', ROOT / 'ops/quarantine-psql-transport.py')
        cls.proof = cls.work / 'synthetic-proof.json'
        cls.manifest = cls.work / 'synthetic-approval.json'
        # P4 proof fixture names the ACTUAL locally packaged target, not our
        # intentionally inert old broker/worker observation containers.
        cls.capability = load('capability_fixture', ROOT / 'ops/verify-image-quarantine-capability.py')
        source_evidence = json.loads((ROOT / '.evidence/issue203/priority4-runtime-capability/synthetic-evidence-final.json').read_text())
        if source_evidence.get('identity_only_nonbehavior') is not True or source_evidence.get('synthetic_nonofficial') is not True:
            raise RuntimeError('identity-only NONOFFICIAL prerequisite required, not behavior evidence')
        source_hashes = source_evidence.get('source_sha256', {})
        expected_sources = load('ci_source_inventory', ROOT / 'tests/issue203-safe-drain-ci.py').source_hashes()
        if source_hashes != expected_sources:
            raise RuntimeError('packaged prerequisite source drift')
        cls.release = source_evidence['identity']['release_sha']
        cls.target_ref = source_evidence['identity']['repo_digests'][0]
        cls.target_id = source_evidence['identity']['local_image_id']
        if cls.capability.image_identity(cls.capability.Docker(), cls.target_ref, cls.release) != source_evidence['identity']:
            raise RuntimeError('immutable prerequisite inventory mismatch')
        for service, binary in (('xianzhi-ai', '/app/xianzhi-api'), ('smartvideo-worker', '/app/smartvideo-worker')):
            cls.model['services'][service]['image'] = cls.target_ref
            cls.model['services'][service]['command'] = [binary]
        cls.model['services']['xianzhi-ai']['environment'].update(XIANZHI_ENV='production', DATABASE_URL='postgres://postgres:synthetic_transport_secret@postgres:5432/xianzhi_test?sslmode=disable')
        cls.compose.write_text(json.dumps(cls.model))
        desired = json.loads(command(cls.cmd + ['config', '--format', 'json']))
        started = time.monotonic()
        cls.behavior_evidence = cls.capability.attest(cls.target_ref, cls.release, cls.capability.runtime_policy(desired), evidence_directory='/work/capability-owned')
        cls.capability.verify(cls.behavior_evidence, cls.target_ref, cls.release, cls.behavior_evidence['policy'])
        (cls.work / 'actual-strict-behavior-NONOFFICIAL.json').write_text(json.dumps(dict(fixture_nonofficial=True, elapsed_seconds=time.monotonic() - started, capability=cls.behavior_evidence), indent=2, sort_keys=True))
        print('FULL_PACKAGED_RESULT roles=2 blocked=2 allowed=2 unavailable=6 history=2 funded_controls=2 budget=300 NONOFFICIAL', flush=True)
        cls.secret = uuid.uuid4().hex + uuid.uuid4().hex
        os.environ['RELEASE_TRUST_SECRET'] = cls.secret
        print('ACTUAL_RUNTIME Python=' + sys.version.split()[0], flush=True)
        print('ACTUAL_PG ' + cls.cid, flush=True)
        info = json.loads(command(['docker', 'inspect', cls.cid]))[0]
        if info['HostConfig']['PortBindings']:
            raise RuntimeError('host DB ports are forbidden')
        with socket.socket() as sock:
            if sock.connect_ex(('127.0.0.1', 5432)) == 0:
                raise RuntimeError('runner localhost5432 is listening')
        print('NO_HOST_PORTS + LOCALHOST5432_CLOSED', flush=True)

    @classmethod
    def tearDownClass(cls):
        cls.db.close()
        cls.approval_tests.ApprovalTests.tearDownClass()
        # Host finally performs independent owner-label-scoped cleanup as well.

    def setUp(self):
        self.fixture.setUp()
        self.lock_dir = self.work / 'release.lock'
        self.lock_dir.mkdir(exist_ok=True)
        (self.lock_dir / 'owner_pid').write_text(str(os.getpid()))
        (self.lock_dir / 'owner_token').write_text('synthetic-owner-token')
        (self.lock_dir / 'release_sha').write_text(self.release)
        os.environ['OWNER_TOKEN'] = 'synthetic-owner-token'
        self.manifest.write_bytes(self.fixture.signed_canonical(release_sha=self.release))
        self.write_proof()

    def write_proof(self):
        t = self.transport
        manifest = self.work / 'synthetic-release.json'
        manifest.write_text('{"synthetic_only":true}')
        receipt = self.work / 'synthetic-receipt.json'
        image_id = self.target_id
        receipt.write_text(json.dumps(dict(previous_git_sha=self.release, previous_image_id=image_id, previous_image_reference=self.target_ref,
            rollback_manifest_path=str(manifest), rollback_manifest_sha256=t.digest_file(str(manifest)))))
        files = ['deploy.sh', 'rollback.sh', 'ops/verify-release-manifest.sh', 'ops/disk-guard.sh',
                 'ops/run-migrations.sh', 'ops/prestage-release.sh', 'ops/verify-prestage-proof.sh',
                 'ops/verify-release-runtime.py', 'ops/verify-safe-drain.py', 'ops/enroll-quarantine.py',
                 'ops/quarantine-approval.py', 'ops/create-quarantine-candidate.py',
                 'ops/quarantine-approval/registry.json',
                 'ops/quarantine-live-snapshot.py', 'ops/quarantine-psql-transport.py',
                 'ops/verify-image-quarantine-capability.py', 'ops/first-upgrade-cold.py',
                 'ops/auto_monitor_killswitch.py']
        config = json.loads(command(self.cmd + ['config', '--format', 'json']))
        proof = dict(git_sha=self.release, expires_at=(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat(),
            proof_nonce=uuid.uuid4().hex, compose_hash=t.digest_file(str(self.compose)), env_hash=t.digest_file(str(self.env)),
            bound_config_hash=hashlib.sha256(t.canonical(config)).hexdigest(), config_binding_version=2,
            deploy_scripts_hash={p: t.digest_file(str(ROOT / p)) for p in files},
            manifest_path=str(manifest), manifest_sha256=t.digest_file(str(manifest)),
            rollback_receipt_path=str(receipt), rollback_receipt_hash=t.digest_file(str(receipt)),
            image_reference=self.target_ref, local_image_id=image_id,
            runtime_capability=self.behavior_evidence, rollback_runtime_capability=self.behavior_evidence,
            fixture_nonofficial=True,
            github_provenance=dict(repository='SYNTHETIC-NOT-OFFICIAL', workflow='SYNTHETIC', run_id=1, artifact_id=1,
                                   head_sha=self.release, manifest_bytes_sha256=t.digest_file(str(manifest))),
            postgres_transport_binding=t.binding(str(self.compose), str(self.env)))
        proof['signature'] = hmac.new(self.secret.encode(), t.canonical(proof), hashlib.sha256).hexdigest()
        self.proof.write_text(json.dumps(proof))

    def target(self):
        return self.transport.Target(str(self.compose), str(self.env), str(self.proof), self.release)

    def cli(self, enroll=False, expected=0, compose=None, env=None, stop_containers=False):
        if stop_containers:
            command(self.cmd + ['stop', 'xianzhi-ai', 'smartvideo-worker'])
        args = [sys.executable, str(ROOT / 'ops' / ('enroll-quarantine.py' if enroll else 'verify-safe-drain.py')),
                str(compose or self.compose), str(env or self.env)]
        if enroll:
            args += [str(self.manifest), self.release, '--release-lock-dir', str(self.lock_dir)]
        else:
            args += ['0', '--manifest', str(self.manifest), '--release-sha', self.release]
        args += ['--expected-manifest-sha256', hashlib.sha256(self.manifest.read_bytes()).hexdigest(),
                 '--prestage-proof', str(self.proof)]
        try:
            result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=100)
            output = result.stdout + result.stderr
            self.assertNotIn(b'synthetic_transport_secret', output)
            self.assertEqual(result.returncode == 0, expected == 0, output.decode())
            print('CLI ' + ('enroll' if enroll else 'drain') + ' ' + ('PASS' if result.returncode == 0 else 'BLOCK'), flush=True)
            return result
        finally:
            if stop_containers:
                command(self.cmd + ['start', 'xianzhi-ai', 'smartvideo-worker'])

    def postgres_error_count(self, phrase):
        # Inspect only this UUID's database; never publish raw SQL/logs/secrets.
        observed = subprocess.run(['docker', 'logs', self.cid], stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, timeout=10)
        self.assertEqual(observed.returncode, 0)
        return (observed.stdout + observed.stderr).count(phrase)

    def count(self):
        with self.db.cursor() as cur:
            cur.execute('SELECT count(*) FROM public.provider_execution_quarantine')
            return cur.fetchone()[0]

    def test_01_real_cli_safe_drain_and_enrollment(self):
        self.cli()
        self.cli(enroll=True, stop_containers=True)
        self.assertEqual(self.count(), 9)

    def test_02_projector_equivalence_all_families(self):
        # Includes nonempty assets/storage/results and all personal financial families.
        self.fixture.stored_asset_fixture()
        expected = self.live.core.sample_canonical_live_read_only(self.db, self.fixture.entries)
        conn = self.target().connect()
        try:
            actual = self.live.core.sample_canonical_live_read_only(conn, self.fixture.entries)
            self.assertEqual(self.live.core.canonical(actual), self.live.core.canonical(expected))
        finally:
            conn.close()

    def test_03_positional_types_parameters_and_readonly(self):
        conn = self.target().connect()
        try:
            cur = conn.cursor()
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            hostile = "中文'; DROP TABLE public.provider_executions; --\n\\! echo bad\t\\.\""
            sql = "SELECT %s AS duplicate,%s AS duplicate,NULL AS nullable,true AS flag,9223372036854775807::bigint AS big,123456789012.123456::numeric AS amount,'2026-01-02T03:04:05.123456Z'::timestamptz AS clock_timestamp,%s AS arr,2=ANY(%s) AS empty,'{\"nested\":[1,null,true]}'::json AS nested"
            values = (hostile, '', [hostile, 'x'], [])
            # Synthetic decoder probes do not widen the production SELECT allowlist.
            with self.db.cursor() as native:
                native.execute(sql, values)
                expected = native.fetchall()
                oids = tuple(d.type_code for d in native.description)
                token = uuid.uuid4().hex
                native.execute(self.transport.select_statement(self.transport.bind(sql, values), token, oids))
                actual = self.transport.decode_frame(json.dumps(native.fetchone()[0]).encode(), token, oids)
                self.assertEqual(actual, expected)
                self.assertEqual([type(v) for v in actual[0]], [type(v) for v in expected[0]])
            with self.assertRaises(self.transport.TransportError):
                self.transport.query_types(sql)
            # A permitted enrollment INSERT is rejected by PostgreSQL, not a mock.
            readonly_error = b'cannot execute INSERT in a read-only transaction'
            errors_before = self.postgres_error_count(readonly_error)
            with self.assertRaises(self.transport.TransportError):
                tree = ast.parse((ROOT / 'ops/enroll-quarantine.py').read_bytes())
                insert = next(n.value.s for n in ast.walk(tree) if isinstance(n, ast.Assign) and
                              any(isinstance(t, ast.Name) and t.id == 'insert_sql' for t in n.targets))
                cur.execute(insert, (901, 'synthetic-task-0', 1, 1, 'a' * 64, 'b' * 64,
                                     'synthetic', self.release, '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z'))
            self.assertEqual(self.postgres_error_count(readonly_error), errors_before + 1)
            self.assertTrue(conn.closed)
        finally:
            conn.close()
        self.assertEqual(self.count(), 0)
        for value in ('nul\x00text', True, 1.5, {}, [None], [[1]]):
            with self.assertRaises(self.transport.TransportError):
                self.transport.parameter(value)

    def test_04_cli_hash_drift_and_schema_missing_zero_writes(self):
        self.fixture.sql("UPDATE public.provider_executions SET provider_request_id='drift' WHERE id=901")
        self.cli(expected=1)
        self.cli(enroll=True, expected=1)
        self.assertEqual(self.count(), 0)
        self.fixture.sql('ALTER TABLE public.xz_generation_tasks RENAME COLUMN prompt TO prompt_missing')
        try:
            self.cli(expected=1)
        finally:
            self.fixture.sql('ALTER TABLE public.xz_generation_tasks RENAME COLUMN prompt_missing TO prompt')

    def test_05_cli_readback_failure_rolls_back(self):
        self.fixture.sql("CREATE FUNCTION public.synthetic_readback() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.approval_id := 'synthetic-tamper'; RETURN NEW; END $$")
        self.fixture.sql('CREATE TRIGGER synthetic_readback BEFORE INSERT ON public.provider_execution_quarantine FOR EACH ROW EXECUTE FUNCTION public.synthetic_readback()')
        try:
            self.cli(enroll=True, expected=1, stop_containers=True)
            self.assertEqual(self.count(), 0)
        finally:
            self.fixture.sql('DROP TRIGGER synthetic_readback ON public.provider_execution_quarantine')
            self.fixture.sql('DROP FUNCTION public.synthetic_readback()')

    def test_06_paths_bytes_and_host_overrides(self):
        for path in (self.work / 'missing',):
            self.cli(expected=1, env=path)
            self.cli(expected=1, compose=path)
        wrong = self.work / 'wrong.env'
        wrong.write_bytes(self.env.read_bytes())
        self.cli(expected=1, env=wrong)
        old = self.env.read_bytes()
        self.env.write_bytes(old + b'# changed\n')
        try:
            self.cli(expected=1)
        finally:
            self.env.write_bytes(old)
        os.environ['POSTGRES_PASSWORD'] = 'untrusted-host-override'
        try:
            self.cli(expected=1)
        finally:
            del os.environ['POSTGRES_PASSWORD']

    def test_07_stopped_absent_and_restarted_container(self):
        command(self.cmd + ['stop', 'postgres'])
        try:
            self.cli(expected=1)
        finally:
            command(self.cmd + ['start', 'postgres'])
            import psycopg2
            for unused in range(40):
                try:
                    db = psycopg2.connect(host=self.project + '-postgres-1', dbname='xianzhi_test', user='postgres', password='synthetic_transport_secret')
                    db.autocommit = True
                    type(self).db = db
                    self.fixture.db = db
                    break
                except psycopg2.OperationalError:
                    time.sleep(.25)
            else:
                raise RuntimeError('owned postgres restart failed')
        self.cli(expected=1)  # same ID, changed StartedAt is also rejected
        model = dict(self.model, services={k: v for k, v in self.model['services'].items() if k != 'postgres'})
        self.compose.write_text(json.dumps(model))
        try:
            self.cli(expected=1)
        finally:
            self.compose.write_text(json.dumps(self.model))

    def test_08_psql_nonzero_malformed_truncated_and_missing_fields(self):
        # Replace only OUR container's executable, never production tools/source.
        psql = command(['docker', 'exec', self.cid, 'sh', '-c', 'command -v psql'])
        command(['docker', 'exec', self.cid, 'mv', psql, psql + '.owned-original'])
        try:
            for output in ("echo synthetic_transport_secret >&2; exit 7", "printf 'not-json\\n'", "printf '{\\\"token\\\":'", "printf '{}\\n'"):
                script = '#!/bin/sh\n' + output + '\n'
                command(['docker', 'exec', '-i', self.cid, 'sh', '-c', 'cat > "$1"; chmod 755 "$1"', 'sh', psql], input=script.encode())
                self.cli(expected=1)
        finally:
            command(['docker', 'exec', self.cid, 'mv', psql + '.owned-original', psql])
        self.assertEqual(self.count(), 0)

    def test_09_password_not_in_argv_and_disconnect_rollback(self):
        conn = self.target().connect()
        try:
            self.assertNotIn('synthetic_transport_secret', repr(conn.process.args))
            cur = conn.cursor()
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
            self.live.enroll.enroll_quarantine_in_transaction(cur, self.manifest.read_bytes(), self.release)
            self.assertEqual(self.count(), 0)  # other transaction cannot see inserts
            conn.close()  # host failure/EOF never commits
            self.assertEqual(self.count(), 0)
        finally:
            conn.close()


    def test_10_frames_and_parameter_arity_fail_closed(self):
        token = 'a' * 32
        valid = dict(token=token, rows=[['1']], count=1, columns=1, types=[20])
        self.assertEqual(self.transport.decode_frame(json.dumps(valid).encode(), token, (20,)), [(1,)])
        for fields in (dict(token='b' * 32), dict(count=2), dict(columns=2), dict(rows=[[]]),
                       dict(rows=[['1'], ['2']]), dict(count=10002), dict(types=[]),
                       dict(types=[1700]), dict(types=[True]), dict(rows=[[1]]), dict(rows=[['true']])):
            with self.assertRaises(self.transport.TransportError):
                self.transport.decode_frame(json.dumps(dict(valid, **fields)).encode(), token, (20,))
        for raw in (b'{}', b'{', b'{"token":"a","token":"a"}', b'NaN', b'[]'):
            with self.assertRaises(self.transport.TransportError):
                self.transport.decode_frame(raw, token, (20,))
        for oid in (0, 999999):
            with self.assertRaises(self.transport.TransportError):
                self.transport.decode_frame(json.dumps(dict(valid, types=[oid])).encode(), token, (oid,))
        for sql, values in (('SELECT %s', ()), ('SELECT %s', (1, 2)), ('SELECT %d', (1,)), ('SELECT 1;\\\\! echo bad', ())):
            with self.assertRaises(self.transport.TransportError):
                self.transport.bind(sql, values)

    def test_11_statement_timeout_after_insert_rolls_back(self):
        conn = self.target().connect()
        try:
            cur = conn.cursor()
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
            self.live.enroll.enroll_quarantine_in_transaction(cur, self.manifest.read_bytes(), self.release)
            # Lock a different fixture row: transport's fixed enrollment SELECT
            # genuinely waits for PostgreSQL statement_timeout, not whitelist denial.
            timeout_error = b'canceling statement due to statement timeout'
            errors_before = self.postgres_error_count(timeout_error)
            with self.db.cursor() as locker:
                locker.execute('BEGIN')
                locker.execute('SELECT id FROM public.provider_executions WHERE id=800 FOR UPDATE')
                started = time.monotonic()
                try:
                    with self.assertRaises(self.transport.TransportError):
                        cur.execute('SELECT id, task_id, attempt, task_execution_generation FROM public.provider_executions '
                                    'WHERE id = ANY(%s) ORDER BY id FOR UPDATE;', ([800],))
                finally:
                    locker.execute('ROLLBACK')
            self.assertEqual(self.postgres_error_count(timeout_error), errors_before + 1, 'actual PostgreSQL statement-timeout error required')
            self.assertLess(time.monotonic() - started, 20)
            self.assertTrue(conn.closed)
            self.assertEqual(self.count(), 0)
            print('STATEMENT_TIMEOUT_ROLLBACK_ZERO_WRITES', flush=True)
        finally:
            conn.close()


    def test_12_every_fixed_query_native_types_empty_metadata_and_hashes(self):
        test = self
        seen = set()
        class ComparedCursor:
            def __init__(self, cursor):
                self.native = cursor
                self.connection = cursor.connection
                self.rows = []
            def execute(self, sql, params=()):
                self.native.execute(sql, params)
                if not sql.strip().startswith('SELECT'):
                    return
                expected_types = test.transport.query_types(sql)
                native_types = tuple(d.type_code for d in self.native.description)
                test.assertEqual(expected_types, native_types)
                expected = self.native.fetchall()
                token = uuid.uuid4().hex
                text = test.transport.bind(sql, params)
                self.native.execute(test.transport.select_statement(text, token, expected_types))
                frame = self.native.fetchone()[0]
                actual = test.transport.decode_frame(json.dumps(frame).encode(), token, expected_types)
                test.assertEqual(len(actual), len(expected))
                fingerprint = hashlib.sha256(test.transport.re.sub(
                    r' AND e\.id NOT IN \([0-9]+(?:,[0-9]+)*\)',
                    ' AND e.id NOT IN (<execution_ids>)', sql.strip().rstrip(';')).encode()).hexdigest()
                if fingerprint not in test.transport.QUERY_TYPES:
                    fingerprint = hashlib.sha256(test.transport.re.sub(
                        r' AND e\.id NOT IN \([0-9]+(?:,[0-9]+)*\)',
                        ' AND e.id NOT IN (<execution_ids>)', sql.strip().rstrip(';')).replace('%%', '%').encode()).hexdigest()
                left, right = actual, expected
                if fingerprint in CANONICAL_MULTISET_QUERIES:
                    test.assertTrue(all(oid == 25 for oid in expected_types))
                    typed_row = lambda row: tuple((type(v).__name__, repr(v)) for v in row)
                    left, right = sorted(actual, key=typed_row), sorted(expected, key=typed_row)
                for row, original in zip(left, right):
                    test.assertEqual([type(v) for v in row], [type(v) for v in original])
                    for value, old in zip(row, original):
                        # clock_timestamp is intentionally volatile and excluded
                        # from business hashes; compare UTC type and bounded time.
                        if isinstance(value, datetime.datetime) and 'clock_timestamp()' in sql:
                            test.assertLess(abs((value - old).total_seconds()), 1)
                        else:
                            test.assertEqual(value, old)
                self.rows = actual
                if fingerprint not in seen:
                    # Force zero rows for EVERY reachable query, even aggregates:
                    # descriptor still checks arity and actual base PostgreSQL OIDs.
                    empty = 'SELECT * FROM (' + text + ') empty_projection WHERE false'
                    self.native.execute(test.transport.select_statement(empty, token, expected_types))
                    empty_frame = self.native.fetchone()[0]
                    test.assertEqual(empty_frame['columns'], len(expected_types))
                    test.assertEqual(test.transport.decode_frame(json.dumps(empty_frame).encode(), token, expected_types), [])
                    seen.add(fingerprint)
            def fetchall(self):
                rows, self.rows = self.rows, []
                return rows
            def fetchone(self):
                return self.rows.pop(0) if self.rows else None
        with self.db.cursor() as native:
            cur = ComparedCursor(native)
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            try:
                expected = self.live.core.project_canonical_live_snapshot_in_transaction(native, self.fixture.entries)
                actual = self.live.core.project_canonical_live_snapshot_in_transaction(cur, self.fixture.entries)
                again = self.live.core.project_canonical_live_snapshot_in_transaction(cur, self.fixture.entries)
                self.assertEqual(self.live.core.canonical(expected), self.live.core.canonical(actual))
                self.assertEqual(self.live.core.canonical(actual), self.live.core.canonical(again))
            finally:
                cur.execute('ROLLBACK')
        self.fixture.sql('CREATE TABLE IF NOT EXISTS public.schema_migrations(filename text PRIMARY KEY, applied_at timestamptz NOT NULL)')
        self.fixture.sql("INSERT INTO public.schema_migrations(filename,applied_at) VALUES('119-execution-generation-fencing.sql','2026-09-18T08:20:09.578256Z') ON CONFLICT (filename) DO UPDATE SET applied_at=EXCLUDED.applied_at")
        legacy = [dict(item, generation=None) for item in self.fixture.entries[:6]]
        pins = frozenset(self.live.core._approval.legacy_identity_sha256(item['execution_id'], item['task_id'], item['attempt']) for item in legacy)
        old_pins = self.live.core._approval.LEGACY_NULL_IDENTITY_PINS
        self.live.core._approval.LEGACY_NULL_IDENTITY_PINS = pins
        try:
            with self.db.cursor() as native:
                cur = ComparedCursor(native)
                cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                try:
                    native.execute("UPDATE public.xz_generation_tasks SET created_at='2026-09-03T10:09:43.350748981Z' WHERE id IN ('synthetic-task-0','synthetic-task-1','synthetic-task-2','synthetic-task-3','synthetic-task-4','synthetic-task-5')")
                    native.execute('DELETE FROM public.provider_execution_correlations WHERE execution_id=800')
                    native.execute('DELETE FROM public.provider_executions WHERE id=800')
                    native.execute("UPDATE public.provider_executions SET task_execution_generation=NULL,created_at='2026-09-03T10:09:43Z' WHERE id BETWEEN 901 AND 906")
                    self.assertEqual(self.live.core.canonical(
                        self.live.core.project_canonical_live_snapshot_in_transaction(native, legacy)),
                        self.live.core.canonical(
                            self.live.core.project_canonical_live_snapshot_in_transaction(cur, legacy)))
                finally:
                    cur.execute('ROLLBACK')
        finally:
            self.live.core._approval.LEGACY_NULL_IDENTITY_PINS = old_pins
        self.fixture.stored_asset_fixture()
        self.manifest.write_bytes(self.fixture.signed_canonical(release_sha=self.release))
        with self.db.cursor() as native:
            cur = ComparedCursor(native)
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
            try:
                self.live.enroll.enroll_quarantine_in_transaction(cur, self.manifest.read_bytes(), self.release)
            finally:
                cur.execute('ROLLBACK')
            drain = load('drain_source', ROOT / 'ops/verify-safe-drain.py')
            for ids in (None, [901, 902], [9223372036854775807]):
                cur.execute(drain.build_sql(ids))
            cur.execute(drain.HISTORY_KEYS_SQL)
            for ids in (None, [901, 902], [9223372036854775807]):
                cur.execute(drain.build_history_sql(ids))
        self.assertEqual(seen, set(self.transport.QUERY_TYPES), 'fixed-source variant coverage changed')
        self.assertTrue(CANONICAL_MULTISET_QUERIES.issubset(seen))
        self.assertEqual(self.count(), 0)
        print('FIXED_QUERY_NATIVE_EQUIVALENCE queries=%d empty_descriptors=%d; canonical_repeatability=PASS' % (len(seen), len(seen)), flush=True)

    def test_13_frame_order_truncation_eof_and_timeout_close_session(self):
        from unittest import mock
        import queue
        # Deterministic protocol faults after write: each closes a real session
        # containing nine uncommitted inserts, then another backend sees zero.
        for fault in ('order', 'truncation', 'eof', 'timeout'):
            with self.subTest(fault=fault):
                conn = self.target().connect()
                try:
                    cur = conn.cursor()
                    cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    self.live.enroll.enroll_quarantine_in_transaction(cur, self.manifest.read_bytes(), self.release)
                    with mock.patch.object(conn.messages, 'get') as receive:
                        if fault == 'timeout':
                            receive.side_effect = queue.Empty
                        else:
                            receive.return_value = {'order': b'END_wrong\n', 'truncation': b'{"token":', 'eof': b''}[fault]
                        with self.assertRaises(self.transport.TransportError):
                            cur.execute('SELECT id, task_id, attempt, task_execution_generation FROM public.provider_executions '
                                        'WHERE id = ANY(%s) ORDER BY id FOR UPDATE;', ([901],))
                    self.assertTrue(conn.closed)
                    self.assertEqual(self.count(), 0)
                finally:
                    conn.close()


    def test_14_scalar_numeric_and_nested_json_native_semantics(self):
        sql = """SELECT 0::numeric AS arbitrary,42::numeric AS arbitrary,
          123456789012345678901234567890.12345678901234567890::numeric AS money,
          9223372036854775807::bigint AS maximum, true AS flag, false AS flag,
          NULL::text AS nullable, %s AS clock_timestamp, ''::text AS lease_until,
          '2026-01-02T03:04:05.123456Z'::timestamptz AS unrelated_name,
          '{"fraction":0.25,"integer":0,"nested":[true,null,"中文",1.125]}'::jsonb AS nested,
          '{"repeat":1,"repeat":2,"items":[]}'::json AS duplicate_keys,
          ARRAY['中文',NULL,'']::text[] AS texts,
          ARRAY[9223372036854775807,NULL,0]::bigint[] AS numbers,
          ARRAY[]::text[] AS empty,'null'::json AS json_null"""
        token = uuid.uuid4().hex
        with self.db.cursor() as native:
            native.execute(sql, ('中文\nquoted \' text',))
            expected = native.fetchall()
            oids = tuple(d.type_code for d in native.description)
            text = self.transport.bind(sql, ('中文\nquoted \' text',))
            native.execute(self.transport.select_statement(text, token, oids))
            actual = self.transport.decode_frame(json.dumps(native.fetchone()[0]).encode(), token, oids)
            self.assertEqual(actual, expected)
            def types(value):
                if isinstance(value, (tuple, list)):
                    return (type(value), [types(v) for v in value])
                if isinstance(value, dict):
                    return (dict, {k: types(v) for k, v in value.items()})
                return type(value)
            self.assertEqual(types(actual), types(expected))
            self.assertIs(type(actual[0][0]), decimal.Decimal)
            self.assertIs(type(actual[0][10]['fraction']), float)
            # Wrong positional OID and arity also fail for actual empty results.
            for malformed in ('SELECT 1::text WHERE false', 'SELECT 1::bigint,2::bigint WHERE false'):
                native.execute(self.transport.select_statement(malformed, token, (20,)))
                with self.assertRaises(self.transport.TransportError):
                    self.transport.decode_frame(json.dumps(native.fetchone()[0]).encode(), token, (20,))

    def test_15_historical_native_transport_projection_and_drift(self):
        drain = load('historical_transport_drain', ROOT / 'ops/verify-safe-drain.py')
        target = self.target()
        self.assertTrue(target.history_enabled())
        # Start from ACTUAL packaged observations; reject protocol, generation,
        # image and rehashed request-identity substitutions, not fake receipts.
        for field in ('version', 'generation', 'fingerprint', 'image', 'version-bool', 'version-float', 'protocol-bool', 'protocol-float', 'history-protocol-bool', 'history-protocol-float', 'control-missing', 'control-no-inbox', 'control-no-capture', 'control-no-effects', 'control-protocol-bool', 'control-protocol-float'):
            bad = json.loads(json.dumps(self.behavior_evidence))
            if field == 'version':
                bad['history']['version'] = 2
            elif field == 'image':
                bad['identity']['reference'] = 'substituted'
            elif field in ('version-bool', 'version-float'):
                bad['history']['version'] = True if field.endswith('bool') else 1.0
            elif field in ('protocol-bool', 'protocol-float', 'history-protocol-bool', 'history-protocol-float'):
                key = 'history_protocol' if field.startswith('history-') else 'protocol'
                bad['history']['observations'][0]['response'][key] = True if field.endswith('bool') else 1.0
            elif field == 'control-missing':
                bad['history']['controls'] = []
            elif field.startswith('control-'):
                control = bad['history']['controls'][0]
                if field == 'control-no-inbox':
                    control['after']['consumer_inbox'] = []
                elif field == 'control-no-capture':
                    task = next(t for t in control['after']['xz_generation_tasks'] if t['id'] == control['owner'] + '-normal-control')
                    task['captured_points'] = 0
                elif field == 'control-no-effects':
                    control['effects'] = []
                else:
                    control['response']['protocol'] = True if field.endswith('bool') else 1.0
                control['after_sha256'] = self.capability.sha(self.capability.canonical(control['after']).encode('utf-8'))
            else:
                row = bad['history']['observations'][0]
                execution = next(e for e in row['before']['provider_executions'] if e['status'] == 'succeeded')
                execution['task_execution_generation' if field == 'generation' else 'request_fingerprint'] = None if field == 'generation' else '0' * 64
                row['after'] = row['before']
                row['before_sha256'] = row['after_sha256'] = self.capability.sha(self.capability.canonical(row['before']).encode('utf-8'))
            with self.assertRaises(self.capability.Refused):
                self.capability.verify(bad, self.target_ref, self.release, self.behavior_evidence['policy'])
        # Rehash deliberately truncated ACTUAL full snapshots, including funded
        # control storage/account/inbox rows; equal hashes cannot bless omissions.
        for family, checker in (('observations', self.capability.check_history_observation), ('controls', self.capability.check_history_control)):
            for baseline in self.behavior_evidence['history'][family]:
                checker(baseline, self.release)
                for table, rows in baseline['before'].items():
                    for index, row in enumerate(rows):
                        for field in row:
                            bad = json.loads(json.dumps(baseline))
                            del bad['before'][table][index][field]
                            bad['before_sha256'] = self.capability.sha(self.capability.canonical(bad['before']).encode('utf-8'))
                            if family == 'observations':
                                bad['after'] = bad['before']
                                bad['after_sha256'] = bad['before_sha256']
                            with self.subTest(family=family, table=table, field=field), self.assertRaises(self.capability.Refused):
                                checker(bad, self.release)
                for table, rows in baseline['after'].items():
                    for index, row in enumerate(rows):
                        for field, value in row.items():
                            if type(value) is not int:
                                continue
                            for wrong in (None, True, float(value)):
                                bad = json.loads(json.dumps(baseline))
                                bad['after'][table][index][field] = wrong
                                bad['after_sha256'] = self.capability.sha(self.capability.canonical(bad['after']).encode('utf-8'))
                                with self.subTest(family=family, table=table, field=field, wrong=wrong), self.assertRaises(self.capability.Refused):
                                    checker(bad, self.release)
        for field in ('source', 'expiry'):
            rejected = self.target()
            if field == 'source':
                rejected.proof['deploy_scripts_hash']['ops/verify-safe-drain.py'] = '0' * 64
            else:
                rejected.proof['expires_at'] = '2001-01-01T00:00:00Z'
            with self.assertRaises(self.transport.TransportError):
                rejected.history_enabled()
        ids = [entry['execution_id'] for entry in self.fixture.entries]
        task = self.owner + '-historical'
        orphan = self.owner + '-orphan'
        with self.db.cursor() as native:
            raw = dict(id=task, userId=self.owner, type='TEXT_TO_IMAGE', status='PROCESSING', model='', prompt='', billingAccountType='PERSONAL', params={})
            native.execute("INSERT INTO xz_generation_tasks(id,user_id,type,model,prompt,status,task_status,execution_generation,worker_id,lease_until,last_heartbeat_at,params,raw) VALUES(%s,%s,'TEXT_TO_IMAGE','','','PROCESSING','DISPATCHING',3,NULL,now()-interval '1 hour',now()-interval '2 hours','{}',%s::jsonb)", (task, self.owner, json.dumps(raw)))
            native.execute("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,error_class,request_fingerprint,task_execution_generation,result_metadata) VALUES(%s,'fixture','fixture','image',1,'succeeded','provider_succeeded',%s,2,'[{\"URL\":\"data:image/png;base64,aGVsbG8=\"}]')", (task, 'a' * 64))
            native.execute("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,error_class,request_fingerprint,task_execution_generation,created_at,updated_at) VALUES(%s,'fixture','fixture','image',1,'failed','definitive_not_submitted',%s,NULL,'2001-01-01','2001-01-02')", (orphan, 'b' * 64))
            native.execute(self.transport.bind(drain.build_history_sql(ids), ()))
            expected = native.fetchone()
        actual = drain.history_observation(target, ids)
        self.assertEqual(actual, expected)
        self.assertEqual(actual[0], 0)
        # Parity rejects the raw terminal branch before its inbox commit can be
        # treated as immutable history. Fixed source descriptor rejects drift.
        for tag, value in (('status', 'COMPLETED'), ('id', 'different'), ('type', 'TEXT_TO_VIDEO'), ('params', {'generation_dispatch_mode': 'canary'}), ('Status', 'COMPLETED')):
            with self.db.cursor() as native:
                native.execute('UPDATE xz_generation_tasks SET raw=raw||jsonb_build_object(%s,%s::jsonb) WHERE id=%s', (tag, json.dumps(value), task))
                native.execute(self.transport.bind(drain.build_history_sql(ids), ()))
                expected = native.fetchone()
            self.assertGreater(expected[0], 0)
            self.assertEqual(drain.history_observation(target, ids), expected)
            with self.db.cursor() as native:
                native.execute('UPDATE xz_generation_tasks SET raw=%s::jsonb WHERE id=%s', (json.dumps(raw), task))
        history_tests = load('historical_storage_fixtures', ROOT / 'tests/issue203-historical-drain-test.py')
        with self.db.cursor() as native:
            mutations = history_tests.seed_orphan_storage(native, self.owner + '-storage', orphan)
        self.cli()
        for sql, parameters in mutations:
            with self.db.cursor() as native:
                native.execute(self.transport.bind(drain.build_history_sql(ids), ()))
                baseline = native.fetchone()
                native.execute('SELECT to_jsonb(r) FROM public.' + ('xz_multipart_upload_parts' if 'xz_multipart_upload_parts' in sql else sql.split()[1]) + ' r WHERE ' + ('upload_id' if 'xz_multipart_upload_parts' in sql else 'file_id' if 'UPDATE xz_file_objects ' in sql else 'id') + '=%s', parameters)
                original = native.fetchone()[0]
                native.execute(sql, parameters)
                native.execute(self.transport.bind(drain.build_history_sql(ids), ()))
                expected = native.fetchone()
            actual = drain.history_observation(target, ids)
            self.assertEqual(expected, actual)
            self.assertEqual(baseline[0], actual[0])
            self.assertNotEqual(baseline[1], actual[1])
            # Real fixed-transport fresh-drain rejection: change exactly one
            # owned row between Docker observations, never verifier time/source.
            def mutate_between_observations(unused):
                with self.db.cursor() as native:
                    native.execute(sql, parameters)
            # Use verify's injectable pause solely as a test fixture driver;
            # actual native/fixed observations and unchanged Target are used.
            with self.db.cursor() as native:
                table = sql.split()[1]
                field = 'file_size' if table == 'xz_file_objects' else 'name' if table == 'xz_storage_configs' else 'relation_type' if table == 'xz_file_relations' else 'metadata' if table == 'xz_storage_jobs' else 'state' if table == 'xz_multipart_uploads' else 'etag'
                key = 'file_id' if table == 'xz_file_objects' else 'upload_id' if table == 'xz_multipart_upload_parts' else 'id'
                native.execute('UPDATE ' + table + ' SET ' + field + '=%s WHERE ' + key + '=%s', (json.dumps(original[field]) if field == 'metadata' else original[field], parameters[0]))
                if table == 'xz_storage_jobs':
                    native.execute('UPDATE xz_storage_jobs SET status=%s WHERE id=%s', (original['status'], parameters[0]))
            with self.assertRaisesRegex(drain.GateError, 'fresh drain required'):
                drain.verify(str(self.compose), str(self.env), 0, target=target, pause=mutate_between_observations, manifest_path=str(self.manifest), release_sha=self.release, expected_manifest_sha256=hashlib.sha256(self.manifest.read_bytes()).hexdigest())
        before_unlinked = drain.history_observation(target, ids)
        with self.db.cursor() as native:
            native.execute("UPDATE xz_storage_jobs SET status='COMPLETED' WHERE id=%s", (self.owner + '-storage-unlinked-job',))
            native.execute('UPDATE xz_file_objects SET file_size=6 WHERE file_id=%s', (self.owner + '-storage-unlinked',))
            native.execute(self.transport.bind(drain.build_history_sql(ids), ()))
            self.assertEqual(native.fetchone(), before_unlinked)
        self.assertEqual(drain.history_observation(target, ids), before_unlinked)
        self.cli()
        with self.db.cursor() as native:
            native.execute('UPDATE xz_generation_tasks SET worker_id=%s WHERE id=%s', ('fixture-changed-owner', task))
        drift = drain.history_observation(target, ids)
        self.assertEqual(drift[0], actual[0])
        self.assertNotEqual(drift[1], actual[1])
        # Source ClaimTx's real unbound NULL-metadata shape must block through
        # the fixed psql projection, without hashing unrelated normal work.
        event = self.owner + '-unbound-normal'
        with self.db.cursor() as native:
            native.execute("INSERT INTO consumer_inbox(consumer_name,event_id) VALUES('generation-image-normal-worker',%s)", (event,))
        pending = drain.history_observation(target, ids)
        self.assertEqual(pending[0], 1)
        self.assertEqual(pending[1], drift[1])
        with self.db.cursor() as native:
            native.execute("UPDATE consumer_inbox SET processed_at=now(),result='completed' WHERE event_id=%s", (event,))
        self.assertEqual(drain.history_observation(target, ids), drift)
        self.assertEqual(self.count(), 0)


def host():
    frozen()
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    owner = uuid.uuid4().hex
    log_path = EVIDENCE / (owner + '.log')
    clean = {k: v for k, v in os.environ.items() if not k.startswith(('XIANZHI_TEST_', 'POSTGRES_', 'PG', 'DOCKER_'))}
    if os.name == 'nt':
        clean['DOCKER_CONTEXT'] = 'desktop-linux'
        endpoint = command(['docker', 'context', 'inspect', 'desktop-linux', '--format', '{{.Endpoints.docker.Host}}'], env=clean)
        if endpoint != 'npipe:////./pipe/dockerDesktopLinuxEngine':
            raise RuntimeError('refuse nonlocal Docker endpoint')
    else:
        endpoint = 'unix:///var/run/docker.sock'
        if not Path('/var/run/docker.sock').is_socket():
            raise RuntimeError('local Linux Docker socket required')
        clean['DOCKER_HOST'] = endpoint
    with socket.socket() as sock:
        if sock.connect_ex(('127.0.0.1', 5432)) == 0:
            raise RuntimeError('host localhost5432 must not listen')
    image = 'issue203-transport-' + owner
    network = image + '-runner'
    container = image + '-runner'
    result = 1
    try:
        with log_path.open('wb') as log:
            log.write(('ENDPOINT ' + endpoint + '\nHOST_LOCALHOST5432_CLOSED\n').encode()); log.flush()
            build = subprocess.run(['docker', 'build', '--label', LABEL + '=' + owner,
                                    '-t', image, '-f', str(ROOT / 'tests/issue203-safe-drain-transport.Dockerfile'), str(ROOT)],
                                   env=clean, stdout=log, stderr=log, timeout=300)
            if build.returncode:
                raise RuntimeError('bounded build failed; see ' + str(log_path))
            image_id = command(['docker', 'image', 'inspect', image, '--format', '{{.Id}}'], env=clean)
            command(['docker', 'network', 'create', '--label', LABEL + '=' + owner, network], env=clean)
            run = subprocess.run(['docker', 'run', '--name', container, '--label', LABEL + '=' + owner,
                                  '--network', network, '-v', str(ROOT) + ':/source:ro',
                                  '-v', '/var/run/docker.sock:/var/run/docker.sock',
                                  '-e', 'ISSUE203_TRANSPORT_OWNER=' + owner, image_id,
                                  'python', 'tests/issue203-safe-drain-transport-test.py', '--inside'],
                                 env=clean, stdout=log, stderr=log, timeout=900)
            result = run.returncode
    finally:
        # Persist runner observations before removing only this UUID's resources.
        inspect = subprocess.run(['docker', 'inspect', container], env=clean, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
        if inspect.returncode == 0:
            info = json.loads(inspect.stdout)[0]
            if info.get('Config', {}).get('Labels', {}).get(LABEL) != owner:
                raise RuntimeError('runner ownership changed')
            copied = subprocess.run(['docker', 'cp', container + ':/work', str(EVIDENCE / (owner + '-work'))], env=clean, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
            if copied.returncode and result == 0:
                result = 1
        # Only this UUID's labels qualify; inherited service/DB variables ignored.
        for kind, listing, remove in [('container', ['ps', '-aq'], ['rm', '-f', '-v']),
                                      ('network', ['network', 'ls', '-q'], ['network', 'rm']),
                                      ('image', ['image', 'ls', '-q'], ['image', 'rm'])]:
            ids = command(['docker'] + listing + ['--filter', 'label=' + LABEL + '=' + owner], env=clean).splitlines()
            for resource in set(ids):
                removed = subprocess.run(['docker'] + remove + [resource], env=clean, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
                if removed.returncode:
                    result = 1
        frozen()
        print('EVIDENCE ' + str(log_path))
    output = log_path.read_text(errors='replace')
    if ('TRANSPORT_RESULT tests=15 failures=0 errors=0 skipped=0' not in output or
            'FULL_PACKAGED_RESULT roles=2 blocked=2 allowed=2 unavailable=6 history=2 funded_controls=2 budget=300 NONOFFICIAL' not in output):
        result = 1
    (EVIDENCE / (owner + '-result.json')).write_text(json.dumps(dict(fixture_nonofficial=True, exit_code=result, log=str(log_path), hard_host_seconds=900)))
    return result


if __name__ == '__main__':
    if sys.argv[1:] == ['--inside']:
        result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(TransportTests))
        print('TRANSPORT_RESULT tests=%d failures=%d errors=%d skipped=%d' % (result.testsRun, len(result.failures), len(result.errors), len(result.skipped)), flush=True)
        sys.exit(0 if result.wasSuccessful() and result.testsRun == 15 and not result.skipped else 1)
    else:
        sys.exit(host())
