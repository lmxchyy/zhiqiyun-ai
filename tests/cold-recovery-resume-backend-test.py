#!/usr/bin/env python3
"""Actual fixed-byte Backend topology, internal fixture identities only.

No Backend/observer/supervisor replacement, no production CLI bypass. Missing
fixed bytes or dirty controller fails. Provisioning ends before signed hold;
recovery never migrates/re-enrolls. All infrastructure is disposable and internal.
"""
import copy
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import types
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[1]
r = types.ModuleType('resume_backend')
r.__file__ = str(ROOT / 'ops/cold-recovery-resume.py')
exec(compile(Path(r.__file__).read_bytes(), r.__file__, 'exec'), r.__dict__)
LABEL = 'org.xianzhi.cold-resume.backend-owner'
OUT = ROOT / '.evidence/cold-recovery-resume/backend'
FIXED_REF = 'ghcr.io/lmxchyy/zhiqiyun-ai@' + r.DIGEST


def emit(path, kind, **fields):
    fields.update(event=kind, monotonic=time.monotonic(), wall=time.time())
    fd = os.open(str(path), os.O_CREAT | os.O_APPEND | os.O_WRONLY, 0o600)
    try:
        os.write(fd, (json.dumps(fields, sort_keys=True) + '\n').encode())
    finally:
        os.close(fd)


def lifecycle(obj):
    return dict(cid=obj['Id'], image_id=obj['Image'], image_reference=obj['Config']['Image'],
                restart_count=obj['RestartCount'], restart_policy=obj['HostConfig']['RestartPolicy'],
                state={k: obj['State'].get(k) for k in ('Running', 'Status', 'Restarting', 'Paused', 'StartedAt', 'FinishedAt', 'ExitCode')})


class TracedDocker(r.cap.Docker):
    """Only command logging/owned fault injection; never manufactures a result."""
    def __init__(self, trace, owner):
        super().__init__()
        self.trace, self.owner = trace, owner
        self.phase = 'provisioning'
        self.fault = None
        self.target_cids = []
        self.rabbit = None
        self.controller_pid = None
    def run(self, args, **kwargs):
        emit(self.trace, 'command', phase=self.phase, argv=[self.redact(str(a)) for a in args],
             stdin_sha256=hashlib.sha256(kwargs['data']).hexdigest() if kwargs.get('data') else None)
        if self.phase == 'recovery' and args[0] == 'start':
            if args[-1] not in self.target_cids:
                raise AssertionError('non-target start during recovery')
            obj = self.inspect('container', args[-1])
            labels = obj['Config']['Labels']
            if (labels.get(LABEL) != self.owner or labels.get('com.docker.compose.project') != 'resume-real-' + self.owner or obj['Image'] != r.IMAGE):
                raise AssertionError('foreign mutation refused before request')
        result = super().run(args, **kwargs)
        if self.phase == 'recovery' and args[0] == 'start':
            obj = self.inspect('container', args[-1])
            emit(self.trace, 'original-start-ack', phase=self.phase, cid=obj['Id'], postimage=lifecycle(obj))
            if self.fault == 'broker' and args[-1] == self.target_cids[1]:
                rabbit = self.inspect('container', self.rabbit)
                if rabbit['Config']['Labels'].get(LABEL) != self.owner:
                    raise AssertionError('foreign broker fault')
                super().run(['exec', self.rabbit, 'rabbitmqctl', 'delete_user', 'fixture_admin'])
                emit(self.trace, 'owned-broker-user-deleted-after-two-acks')
            if self.fault == 'controller-death' and args[-1] == self.target_cids[0]:
                emit(self.trace, 'controller-sigkill-after-first-ack', pid=self.controller_pid)
                os.kill(self.controller_pid, signal.SIGKILL)
        return result


class RealBackendTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if sys.platform != 'linux' or sys.version_info[:3] != (3, 6, 8):
            raise RuntimeError('native Linux Python3.6.8 mandatory; zero skips')
        raw = r.cap.Docker()
        # Fail before resource acquisition: no synthetic image, rebuild or token.
        obj = raw.inspect('image', FIXED_REF)
        if obj['Id'] != r.IMAGE or FIXED_REF not in obj.get('RepoDigests', []) or (obj['Os'], obj['Architecture']) != ('linux', 'amd64'):
            raise RuntimeError('exact fixed packaged bytes absent')
        r.source_identity()  # actual clean committed controller, no replacement
        OUT.mkdir(parents=True, exist_ok=True)

    def setUp(self):
        self.owner = uuid.uuid4().hex
        self.path = OUT / self.owner
        self.path.mkdir(mode=0o700)
        self.trace = self.path / 'commands.jsonl'
        self.docker = TracedDocker(self.trace, self.owner)
        self.resources = []
        self.addCleanup(self.cleanup)
        self.originals = {name: getattr(r, name) for name in ('PROJECT', 'ANCHOR_PATH', 'COMPOSE', 'ENV', 'WORK', 'DIR', 'REF', 'OLD_PROOF')}
        self.old_env = {name: os.environ.get(name) for name in ('RELEASE_TRUST_KEY_FILE', 'RELEASE_TRUST_SECRET')}
        self.addCleanup(self.restore)
        self.project = 'resume-real-' + self.owner
        # Persist ownership before ANY private file; EXIT sanitization can find
        # a fixture interrupted before acquiring its first Docker resource.
        self.write('resources.json', dict(owner=self.owner, project=self.project, resources=[]))
        r.PROJECT, r.REF = self.project, FIXED_REF
        r.COMPOSE, r.ENV = str(self.path / 'compose.json'), str(self.path / 'fixture.env')
        r.ANCHOR_PATH = str(self.path / 'anchor.json')
        r.DIR, r.WORK = str(self.path / 'prestage'), str(self.path / 'prestage' / r.cold.HOLD)
        Path(r.WORK).mkdir(parents=True, mode=0o700)
        key = self.path / 'fixture-release-trust.key'
        key.write_text(uuid.uuid4().hex + uuid.uuid4().hex + '\n', encoding='utf-8')
        key.chmod(0o600)
        os.environ.pop('RELEASE_TRUST_SECRET', None)
        os.environ['RELEASE_TRUST_KEY_FILE'] = str(key)
        self.addCleanup(self.capture_final)
        self.provision()

    def restore(self):
        for name, value in self.originals.items(): setattr(r, name, value)
        for name, value in self.old_env.items():
            if value is None: os.environ.pop(name, None)
            else: os.environ[name] = value

    def record(self, kind, identity):
        obj = self.docker.inspect(kind, identity)
        labels = obj.get('Config', {}).get('Labels', {}) if kind == 'container' else obj.get('Labels', {})
        if labels.get(LABEL) != self.owner:
            raise AssertionError('unowned resource acquisition')
        identity = obj.get('Id') if kind != 'volume' else obj['Name']
        self.resources.append((kind, identity))
        self.write('resources.json', dict(owner=self.owner, project=self.project, resources=self.resources))
        return identity

    def cleanup(self):
        # Exact recorded IDs/names AND owner label. Never compose down/prune.
        failures = []
        for kind, identity in reversed(self.resources):
            try:
                obj = self.docker.inspect(kind, identity)
                labels = obj.get('Config', {}).get('Labels', {}) if kind == 'container' else obj.get('Labels', {})
                if labels.get(LABEL) != self.owner:
                    raise AssertionError('foreign cleanup refused')
                if kind == 'container': self.docker.text(['rm', '-f', '-v', identity])
                else: self.docker.text([kind, 'rm', identity])
            except Exception:
                failures.append([kind, identity])
        for name in ('fixture.env', 'compose.json', 'fixture-release-trust.key'):
            private = self.path / name
            if private.exists(): private.unlink()
        self.write('cleanup.json', dict(failures=failures, removed=len(self.resources) - len(failures), fixture_secrets_removed=True))
        if failures: raise RuntimeError('owned fixture cleanup incomplete')

    def write(self, name, value):
        (self.path / name).write_text(json.dumps(value, indent=2, sort_keys=True) + '\n', encoding='utf-8')

    def compose(self, *args, **kwargs):
        return self.docker.text(['compose', '-f', r.COMPOSE, '--env-file', r.ENV] + list(args), **kwargs)

    def sql(self, text):
        return self.docker.text(['exec', '-i', self.services['postgres'], 'psql', '-X', '-q', '-U', 'fixture_admin', '-d', 'fixture_only', '-v', 'ON_ERROR_STOP=1', '-At'], data=text.encode('utf-8'), timeout=180)

    def wait_healthy(self, roles):
        end = time.monotonic() + 120
        while time.monotonic() < end:
            if all(self.docker.inspect('container', self.services[role])['State'].get('Health', {}).get('Status') == 'healthy' for role in roles):
                return
            time.sleep(.5)
        raise RuntimeError('owned provisioning health deadline')

    def provision(self):
        started = time.monotonic()
        import re
        required = set(re.findall(r'\$\{([A-Z0-9_]+):\?', (ROOT / 'compose.prod.yml').read_text(encoding='utf-8')))
        secret = uuid.uuid4().hex + uuid.uuid4().hex
        values = {name: secret for name in required}
        values.update(COMPOSE_PROJECT_NAME=self.project, TARGET_PLATFORM='linux/amd64', XIANZHI_IMAGE_REFERENCE=FIXED_REF,
                      MINIO_IMAGE='docker.m.daocloud.io/minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e',
                      POSTGRES_USER='fixture_admin', POSTGRES_DB='fixture_only', POSTGRES_PASSWORD=secret,
                      REDIS_PASSWORD=secret, RABBITMQ_DEFAULT_USER='fixture_admin', RABBITMQ_DEFAULT_PASS=secret,
                      MINIO_ROOT_USER='fixture_admin', MINIO_ROOT_PASSWORD=secret,
                      DATABASE_URL='postgres://fixture_admin:' + secret + '@postgres:5432/fixture_only?sslmode=disable',
                      REDIS_URL='redis://:' + secret + '@redis:6379/0', RABBITMQ_URL='amqp://fixture_admin:' + secret + '@rabbitmq:5672/',
                      S3_ENDPOINT='http://minio:9000', STORAGE_PUBLIC_ENDPOINT='http://minio:9000', S3_ACCESS_KEY='fixture_admin',
                      S3_SECRET_KEY=secret, S3_BUCKET='fixture-bucket', STORAGE_MASTER_KEY=secret,
                      CONNECTOR_SECRET_ENCRYPTION_KEY=secret, CONNECTOR_CALLBACK_BASE_URL='http://fixture.invalid',
                      ASYNC_MESSAGING_ENABLED='true', GENERATION_FAIR_SCHEDULER_ENABLED='true', PROVIDER_EXECUTION_SAFETY_ENABLED='true',
                      GENERATION_ASYNC_CANARY_ENABLED='true', VIDEO_ASYNC_CANARY_ENABLED='true', PPT_ASYNC_CANARY_ENABLED='true',
                      OPENAI_BASE_URL='http://fixture.invalid', FEISHU_API_BASE_URL='http://fixture.invalid',
                      FEISHU_ACCOUNTS_BASE_URL='http://fixture.invalid', WECHAT_OPEN_API_BASE_URL='http://fixture.invalid',
                      WECHAT_OPEN_AUTHORIZE_BASE_URL='http://fixture.invalid')
        baselines = sorted((ROOT / 'database/migrations').glob('108-*.sql'))
        values['MIGRATION_FILES'] = ' '.join(p.name for p in baselines if not p.name.endswith('.down.sql'))
        Path(r.ENV).write_text('\n'.join(k + '=' + values[k] for k in sorted(values)) + '\n', encoding='utf-8')
        Path(r.ENV).chmod(0o600)
        self.docker.redactions.append(secret)
        model, unused = r.cap.rendered_policy(self.docker, str(ROOT / 'compose.prod.yml'), r.ENV, FIXED_REF)
        model['name'] = self.project
        model['services'].pop('backup-uploader', None)  # inactive profiled service, not enrolled topology
        for role, cfg in model['services'].items():
            cfg.pop('build', None)
            cfg.pop('container_name', None)
            cfg.pop('ports', None)
            cfg['labels'] = dict(cfg.get('labels') or {}, **{LABEL: self.owner})
            if role not in r.cold.INFRASTRUCTURE: cfg['restart'] = 'no'
        model.setdefault('networks', {}).setdefault('default', {})['internal'] = True
        model['networks']['default']['labels'] = {LABEL: self.owner}
        for cfg in model.get('volumes', {}).values(): cfg['labels'] = {LABEL: self.owner}
        model['services']['postgres']['command'] = ['postgres', '-c', 'log_statement=all', '-c', 'log_line_prefix=%m [%p] %a ']
        # Bind sources must name the host checkout. CI mounts checkout at its
        # identical absolute path; do not remap production transport observers.
        for role in ('postgres-backup',):
            for mount in model['services'][role].get('volumes', []):
                if mount['type'] == 'bind':
                    mount['source'] = str(self.path / 'backups')
                    Path(mount['source']).mkdir(exist_ok=True)
        # Re-rendered Compose JSON is parsed as Compose input: escape literal $
        # in normalized shell commands so credentials are not interpolated twice.
        def escape(value):
            if isinstance(value, str): return value.replace('$', '$$')
            if isinstance(value, list): return [escape(v) for v in value]
            if isinstance(value, dict): return {k: escape(v) for k, v in value.items()}
            return value
        Path(r.COMPOSE).write_text(json.dumps(escape(model), sort_keys=True) + '\n', encoding='utf-8')
        Path(r.COMPOSE).chmod(0o600)
        # Precreate owned networks/volumes by exact model names before containers.
        for name, cfg in model['networks'].items():
            self.record('network', self.docker.text(['network', 'create', '--internal', '--label', LABEL + '=' + self.owner,
                '--label', 'com.docker.compose.project=' + self.project, '--label', 'com.docker.compose.network=' + name, cfg['name']]))
        for name, cfg in model['volumes'].items():
            self.record('volume', self.docker.text(['volume', 'create', '--label', LABEL + '=' + self.owner,
                '--label', 'com.docker.compose.project=' + self.project, '--label', 'com.docker.compose.volume=' + name, cfg['name']]))
        self.services = {}
        try:
            self.compose('create', '--no-build', '--pull', 'never', timeout=120)
        finally:
            # Even a partial create failure enrolls every exact owned resource
            # before cleanup. No broad project delete is used.
            for cid in self.compose('ps', '--all', '-q').splitlines():
                cid = self.record('container', cid)
                role = self.docker.inspect('container', cid)['Config']['Labels']['com.docker.compose.service']
                self.services[role] = cid
        self.assertEqual(set(self.services), set(r.cold.INFRASTRUCTURE) | set(r.ROLES) | {'migrate', 'disk-monitor', 'postgres-backup'})
        self.docker.target_cids = [self.services[role] for role in r.ROLES]
        self.docker.rabbit = self.services['rabbitmq']
        for role in r.cold.INFRASTRUCTURE: self.docker.text(['start', self.services[role]])
        self.wait_healthy(r.cold.INFRASTRUCTURE)
        # Schema + all pre-baseline SQL establish the genuinely existing baseline.
        # Completed migrate container then runs the original runner through121.
        parts = []
        for p in sorted((ROOT / 'database/migrations').glob('[0-9][0-9][0-9]-*.sql')):
            if not p.name.endswith('.down.sql') and int(p.name[:3]) <= 108:
                parts.append(p.read_text(encoding='utf-8'))
        self.sql('\n'.join(parts))
        self.docker.text(['start', '-a', self.services['migrate']], timeout=180)
        self.assertEqual(self.docker.inspect('container', self.services['migrate'])['State']['ExitCode'], 0)
        self.seed()
        self.bucket(secret)
        anchor = r.read(str(ROOT / 'ops/cold-recovery-anchor.json'))
        # Same exact9 enrollment shape, now independently read fixture timestamps,
        # financial fingerprints and actual CIDs. Never claim production history.
        anchor['quarantine_records'] = json.loads(self.sql("SET TIME ZONE 'UTC'; SELECT json_agg(t ORDER BY execution_id) FROM provider_execution_quarantine t;"))
        # PostgreSQL JSON UTC timestamp spelling is identical to native isoformat.
        for row in anchor['quarantine_records']:
            for field in ('created_at', 'not_before', 'expires_at'):
                row[field] = r.transport.decode_cell(json.dumps(row[field]), 1184).isoformat()
        anchor['history'] = [self.sql(query) for query in r.HISTORY]
        anchor['services'] = {role: dict(cid=cid, image_id=self.docker.inspect('container', cid)['Image'],
                                        image_reference=self.docker.inspect('container', cid)['Config']['Image'],
                                        started_at=self.docker.inspect('container', cid)['State']['StartedAt']) for role, cid in self.services.items()}
        self.write('anchor.json', anchor)
        proof = r.cold.signed(dict(scope='fixture-history-NOT-production-authority', git_sha=r.APP, image_reference=FIXED_REF,
                                   proof_nonce=str(uuid.uuid4()), project=self.project))
        self.write('fixture-historical-proof.json', proof)
        r.OLD_PROOF = r.cold.file_hash(str(self.path / 'fixture-historical-proof.json'))
        hold = dict(version=1, state=r.STOPPED, release_sha=r.APP, proof_sha256=r.OLD_PROOF,
                    owner='fixture-historical-owner', project=self.project, image_ids=[r.IMAGE, r.OLD_IMAGE], proof_nonce=proof['proof_nonce'])
        r.cold.write_hold(r.DIR, hold)
        self.backend = r.Backend()  # Actual class, methods and real default Docker.
        self.backend.docker = self.docker  # command trace only, no observer outputs
        self.before = {role: lifecycle(self.docker.inspect('container', cid)) for role, cid in self.services.items()}
        self.write('bindings.json', dict(scope='owned topology NOT production authority', substitutions={
            name: getattr(r, name) for name in self.originals}, trust='generated fixture-only file key; no secret env',
            unchanged=['ROOT/source_identity', 'APP', 'DIGEST', 'IMAGE', 'OLD_IMAGE', 'Backend methods', 'observers', 'execute', 'AttemptSupervisor'],
            budgets=dict(history=300, legacy=180, host=900, human=300, candidate=900), provisioning_seconds=time.monotonic() - started))
        self.write('before.json', self.before)

    def seed(self):
        template = r.read(str(ROOT / 'ops/cold-recovery-anchor.json'))
        fields = r.QUARANTINE[len('SELECT '):].split(' FROM ')[0].split(',')
        statements = ["INSERT INTO xz_users(id,name,role,status) VALUES('fixture-user','fixture','MEMBER','ACTIVE');",
                      "INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw) VALUES('fixture-account','fixture-user',100,10,'{}');"]
        for row in template['quarantine_records']:
            task = row['task_id']
            raw = json.dumps(dict(id=task, userId='fixture-user', type='TEXT_TO_IMAGE', status='PROCESSING', params={}))
            raw_expression = r.transport.parameter(raw) + '::jsonb'
            statements.append("INSERT INTO xz_generation_tasks(id,user_id,type,status,task_status,execution_generation,worker_id,lease_until,last_heartbeat_at,params,raw) VALUES(%s,'fixture-user','TEXT_TO_IMAGE','PROCESSING','RUNNING',%s,'fixture-lease',now()+interval '1 day',now(),'{}',%s);" % (r.transport.parameter(task), r.transport.parameter(row['generation'] or 1), raw_expression))
            statements.append("INSERT INTO provider_executions(id,task_id,provider,provider_model,capability,attempt,status,request_fingerprint,task_execution_generation) VALUES(%s,%s,'fixture','fixture','image',1,'prepared',%s,%s);" % (row['execution_id'], r.transport.parameter(task), r.transport.parameter('a' * 64), r.transport.parameter(row['generation'])))
            statements.append('INSERT INTO provider_execution_quarantine(' + ','.join(fields) + ') VALUES(' + ','.join(r.transport.parameter(row[k]) + ('::timestamptz' if k in ('not_before', 'expires_at', 'created_at') else '') for k in fields) + ');')
        for task, state in [('task_000313', 'SUCCEEDED'), ('task_000322', 'CANCELLED')]:
            statements.append("INSERT INTO xz_generation_tasks(id,user_id,type,status,task_status,params,raw) VALUES('%s','fixture-user','TEXT_TO_IMAGE','%s','%s','{}','{}');" % (task, state, state))
        task = template['quarantine_records'][0]['task_id']
        q = r.transport.parameter(task)
        statements += ["INSERT INTO xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,idempotency_key,status) VALUES('fixture-reservation','fixture-account','fixture-user','GENERATION_TASK',%s,10,10,'fixture-reserve','RESERVED');" % q,
                       "INSERT INTO xz_wallet_ledger(id,account_id,user_id,task_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) VALUES('fixture-ledger','fixture-account','fixture-user',%s,'RESERVE',10,110,100,0,10,'fixture-ledger');" % q,
                       "INSERT INTO xz_billing_lifecycle_events(id,task_id,user_id,event_type,billing_status,points,idempotency_key) VALUES('fixture-lifecycle',%s,'fixture-user','RESERVE','RESERVED',10,'fixture-lifecycle');" % q,
                       "INSERT INTO xz_billing_events(id,task_id,user_id,point_cost,status) VALUES('fixture-billing',%s,'fixture-user',10,'RESERVED');" % q]
        self.sql('\n'.join(statements))
        proof = json.loads(self.sql(
            "SELECT json_build_object('schema_oid', a.atttypid::bigint, 'schema_type', a.atttypid::regtype::text, "
            "'expression_type', pg_typeof(%s)::text, 'expression_value', %s) "
            "FROM pg_catalog.pg_attribute a WHERE a.attrelid='public.xz_generation_tasks'::regclass "
            "AND a.attname='raw' AND NOT a.attisdropped;" % (raw_expression, raw_expression)))
        self.assertEqual(proof['schema_oid'], 3802)
        self.assertEqual(proof['schema_type'], 'jsonb')
        self.assertEqual(proof['expression_type'], 'jsonb')
        self.assertEqual(proof['expression_value'], json.loads(raw))
        proof['stored_value'] = json.loads(self.sql(
            "SELECT raw FROM public.xz_generation_tasks WHERE id=%s;" % r.transport.parameter(json.loads(raw)['id'])))
        self.assertEqual(proof['stored_value'], json.loads(raw))
        self.write('fixture-raw-type-regression.json', proof)

    def bucket(self, secret):
        # Real MinIO bucket provisioning using SigV4, only generated fixture keys.
        script = """import datetime,hashlib,hmac,urllib.request
now=datetime.datetime.utcnow(); stamp=now.strftime('%Y%m%dT%H%M%SZ'); day=stamp[:8]
payload=hashlib.sha256(b'').hexdigest(); scope=day+'/us-east-1/s3/aws4_request'
headers='host:minio:9000\\nx-amz-content-sha256:'+payload+'\\nx-amz-date:'+stamp+'\\n'
signed='host;x-amz-content-sha256;x-amz-date'
canonical='PUT\\n/fixture-bucket\\n\\n'+headers+'\\n'+signed+'\\n'+payload
string='AWS4-HMAC-SHA256\\n'+stamp+'\\n'+scope+'\\n'+hashlib.sha256(canonical.encode()).hexdigest()
def sign(k,v): return hmac.new(k,v.encode(),hashlib.sha256).digest()
import os
key=sign(sign(sign(sign(('AWS4'+os.environ['FIXTURE_SECRET']).encode(),day),'us-east-1'),'s3'),'aws4_request')
auth='AWS4-HMAC-SHA256 Credential=fixture_admin/'+scope+', SignedHeaders='+signed+', Signature='+hmac.new(key,string.encode(),hashlib.sha256).hexdigest()
request=urllib.request.Request('http://minio:9000/fixture-bucket',data=b'',method='PUT',headers={'Authorization':auth,'x-amz-date':stamp,'x-amz-content-sha256':payload})
with urllib.request.urlopen(request,timeout=10) as response: assert response.status==200
"""
        cid = self.record('container', self.docker.text(['create', '--pull', 'never', '--network', self.project + '_default',
                   '--label', LABEL + '=' + self.owner, '--label', 'com.docker.compose.project=' + self.project,
                   '--label', 'com.docker.compose.service=fixture-bucket-provisioner',
                   '--env', 'FIXTURE_SECRET=' + secret, '--entrypoint', 'python3', FIXED_REF, '-c', script]))
        self.docker.text(['start', '-a', cid])
        self.assertEqual(self.docker.inspect('container', cid)['State']['ExitCode'], 0)
        self.docker.text(['rm', '-v', cid])
        self.resources.remove(('container', cid))

    def candidate(self, hold, evidence):
        snapshot = self.backend.snapshot()
        self.backend.verify(evidence, snapshot)
        now = int(time.time())
        candidate = dict(version=1, purpose=r.DOMAIN + '-proof', nonce=uuid.uuid4().hex, not_before=now, expires_at=now + 900,
                         hold_sha256=r.digest(hold), snapshot=snapshot, runtime_capability=evidence,
                         operations=['start-existing-api', 'start-existing-worker'])
        approval = r.approve(candidate, hold, r.source_identity(), 'fixture.operator', r.phrase(candidate))
        return candidate, approval

    def audit(self, name, candidate, elapsed):
        hold = r.cold.read_hold(r.DIR)
        after = {role: lifecycle(self.docker.inspect('container', cid)) for role, cid in self.services.items()}
        self.write(name + '.json', dict(hold=hold, after=after, execution_seconds=elapsed, candidate_nonce=candidate['nonce'],
                                        database=r.database_snapshot(r.transport.binding(r.COMPOSE, r.ENV, r.REF), self.backend.anchor)))
        for role in self.services:
            self.assertEqual(after[role]['cid'], self.before[role]['cid'])
            self.assertEqual(after[role]['image_id'], self.before[role]['image_id'])
            if role in r.cold.INFRASTRUCTURE:
                self.assertEqual(after[role]['state']['StartedAt'], self.before[role]['state']['StartedAt'])
            elif role not in r.ROLES:
                self.assertEqual(after[role], self.before[role])
        self.assertIn(candidate['nonce'], hold['recovery_spent_nonces'])
        self.assertLess(elapsed, 300)
        return hold

    def capture_final(self):
        # Retain SQL read/write facts even when the runtime/timing gate fails.
        services = getattr(self, 'services', {})
        if 'postgres' in services:
            logs = self.docker.run(['logs', services['postgres']], check=False).stderr.decode('utf-8', errors='replace')
            (self.path / 'postgres-statements.log').write_text(self.docker.redact(logs), encoding='utf-8')
        hold_path = Path(r.WORK, 'state.json')
        self.write('final.json', dict(hold=r.cold.read_hold(r.DIR) if hold_path.exists() else None,
                                     lifecycle={role: lifecycle(self.docker.inspect('container', cid)) for role, cid in services.items()},
                                     missing_hold='provisioning incomplete' if not hold_path.exists() else None))

    def test_real_backend_full_hold_failures_and_success(self):
        initial = self.backend.snapshot()
        self.assertEqual([row['execution_id'] for row in initial['database']['quarantine_records']], [1, 12, 14, 22, 25, 31, 100, 102, 103])
        self.write('actual-preimage-snapshot.json', initial)
        started = time.monotonic()
        evidence = self.backend.certify(initial)  # actual nonsynthetic/history300
        self.write('certification.json', dict(scope='actual Backend.certify fixed packaged full-history; NOT production authorization',
                                              evidence=evidence, source=initial['source'], image_identity=initial['image_identity'],
                                              policy=initial['policy'], elapsed=time.monotonic() - started))
        self.assertEqual(self.backend.snapshot(), initial)
        hold = r.read_stopped_hold(r.DIR)
        candidate, approval = self.candidate(hold, evidence)
        lock = r.ReleaseLock(r.DIR)
        anchor = copy.deepcopy(self.backend.anchor)
        self.backend.anchor['history'][0] += '-owned-drift'
        try:
            with self.assertRaises(r.Refused): r.execute(candidate, approval, self.backend, lock, r.DIR)
            self.assertEqual(r.cold.read_hold(r.DIR), hold)
        finally:
            self.backend.anchor = anchor
            lock.release()
        # No recreate capability exists. Parser rejects before any authority read.
        result = subprocess.run([sys.executable, '-B', r.__file__, 'recreate'], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(r.cold.read_hold(r.DIR), hold)
        self.write('pre-mutation-refusals.json', dict(anchor_drift='refused unchanged authentic hold', recreate_exit=result.returncode))
        self.sql("SELECT 'recovery-trace-start-%s';" % self.owner)
        self.docker.phase = 'recovery'
        # Genuine parent death after first ACK: actual guard observes pipe EOF.
        self.docker.fault = 'controller-death'
        started = time.monotonic()
        pid = os.fork()
        if pid == 0:
            try:
                self.docker.controller_pid = os.getpid()
                lock = r.ReleaseLock(r.DIR)
                r.execute(candidate, approval, self.backend, lock, r.DIR)
            finally:
                os._exit(1)
        unused, status = os.waitpid(pid, 0)
        self.assertTrue(os.WIFSIGNALED(status) and os.WTERMSIG(status) == signal.SIGKILL)
        end = time.monotonic() + 150
        while time.monotonic() < end:
            if r.cold.read_hold(r.DIR)['state'] == r.STOPPED and not Path(r.DIR, 'release.lock').exists(): break
            time.sleep(.1)
        else: self.fail('actual guard did not finish owned death fence')
        self.assertEqual(self.audit('controller-death', candidate, time.monotonic() - started)['state'], r.STOPPED)
        self.docker.fault = None
        # A spent approval is refused, without a new start or enrollment.
        lock = r.ReleaseLock(r.DIR)
        try:
            with self.assertRaises(r.Refused): r.execute(candidate, approval, self.backend, lock, r.DIR)
        finally: lock.release()
        hold = r.read_stopped_hold(r.DIR)
        candidate, approval = self.candidate(hold, evidence)
        self.docker.fault = 'broker'
        started = time.monotonic()
        lock = r.ReleaseLock(r.DIR)
        try:
            with self.assertRaises((r.Refused, r.runtime.GateError)):
                r.execute(candidate, approval, self.backend, lock, r.DIR)
        finally:
            if Path(lock.path).exists(): lock.release()
        self.assertEqual(self.audit('real-broker-failure', candidate, time.monotonic() - started)['state'], r.STOPPED)
        # Fixture repair only after both acknowledged starts have been fenced.
        self.docker.phase, self.docker.fault = 'fixture-repair', None
        password = dict(line.split('=', 1) for line in Path(r.ENV).read_text().splitlines())['RABBITMQ_DEFAULT_PASS']
        for args in (['add_user', 'fixture_admin', password], ['set_user_tags', 'fixture_admin', 'administrator'], ['set_permissions', '-p', '/', 'fixture_admin', '.*', '.*', '.*']):
            self.docker.text(['exec', self.services['rabbitmq'], 'rabbitmqctl'] + args)
        hold = r.read_stopped_hold(r.DIR)
        candidate, approval = self.candidate(hold, evidence)
        self.docker.phase = 'recovery'
        started = time.monotonic()
        lock = r.ReleaseLock(r.DIR)
        try:
            r.execute(candidate, approval, self.backend, lock, r.DIR)
        finally:
            if Path(lock.path).exists(): lock.release()
        after = self.audit('actual-success', candidate, time.monotonic() - started)
        self.assertEqual(after['state'], r.SUCCESS)
        self.assertEqual(after['recovery_verified_cids'], self.docker.target_cids)
        self.assertTrue(after['recovery_no_env_persistence'])
        commands = [json.loads(line) for line in self.trace.read_text().splitlines()]
        starts = [v['cid'] for v in commands if v['event'] == 'original-start-ack' and v['phase'] == 'recovery']
        self.assertEqual(starts, [self.docker.target_cids[0]] + self.docker.target_cids * 2)
        # Facts independently logged by PG; no post-hold migration/enrollment SQL.
        logs = self.docker.run(['logs', self.services['postgres']], check=False).stderr.decode('utf-8', errors='replace')
        recovery = logs.split('recovery-trace-start-' + self.owner, 1)[1]
        import re
        self.assertIsNone(re.search(r'(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+(?:public\.)?(?:schema_migrations|provider_execution_quarantine)\b', recovery, re.I))
        self.write('assertions.json', dict(actual_backend=True, real_pid1_env_mount_binary=True, scheduler_readiness_broker_get=True,
                    recovery_starts=starts, per_attempt_start_counts=[1, 2, 2], zero_remigration=True, zero_reenrollment=True,
                    ancillary_held=True, source=r.source_identity(), production_engine_applicability='UNKNOWN'))


if __name__ == '__main__':
    unittest.main(verbosity=2)
