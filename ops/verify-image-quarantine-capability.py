#!/usr/bin/env python3
"""Owned, offline packaged-binary behavioral attestation (Python >=3.6).

No fixture target/provider/context overrides are accepted. Synthetic evidence
is deliberately not usable by production proof validation.
"""
import argparse
import datetime
import hashlib
import hmac
import json
import math
import os
import re
import shutil
import subprocess
import sys
import threading
import time
import types
import uuid

VERSION = 1
ROLES = {'api': '/app/xianzhi-api', 'generation-worker': '/app/generation-worker'}
BINARIES = ('/app/xianzhi-api', '/app/generation-worker', '/app/smartvideo-worker', '/app/video-backfill')
OPS = ('provider', 'video-provider', 'video-persistence', 'create', 'execution-claim', 'transition', 'result', 'correlation', 'status', 'lease', 'persistence', 'asset', 'reserve', 'capture', 'release')
TABLES = ('xz_generation_tasks', 'provider_executions', 'provider_execution_correlations', 'xz_assets', 'xz_file_objects', 'xz_storage_configs', 'xz_tenant_storage_quotas', 'xz_point_accounts', 'xz_personal_point_lots', 'xz_personal_point_reservations', 'xz_personal_point_reservation_allocations', 'xz_personal_point_lot_movements', 'xz_wallet_ledger')
HISTORY_VERSION = 1
HISTORY_OPS = ('text-normal-legacy', 'text-normal', 'image-normal-legacy', 'image-normal', 'text-canary-legacy', 'text-canary', 'image-canary-legacy', 'image-canary', 'orphan')
HISTORY_TABLES = TABLES + ('xz_user_wallets', 'xz_tenant_wallets', 'xz_file_relations', 'xz_storage_jobs', 'xz_multipart_uploads', 'xz_multipart_upload_parts', 'xz_billing_events', 'xz_billing_lifecycle_events', 'outbox_events', 'consumer_inbox', 'video_task_outbox', 'provider_execution_quarantine', 'schema_migrations')
LABEL = 'org.xianzhi.quarantine-capability-owner'
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SINK = '''import http.server,json
class Handler(http.server.BaseHTTPRequestHandler):
 def do_POST(self):
  body=self.rfile.read(int(self.headers.get('Content-Length','0')))
  with open('/tmp/effects','a') as f:f.write(json.dumps({'method':'POST','path':self.path,'bytes':len(body)})+'\\n')
  response={'data':[{'b64_json':'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aB9sAAAAASUVORK5CYII='}]} if self.path=='/consumer-provider' else {}
  self.send_response(200);self.end_headers();self.wfile.write(json.dumps(response).encode())
 def do_GET(self):
  with open('/tmp/effects','a') as f:f.write(json.dumps({'method':'GET','path':self.path,'bytes':0})+'\\n')
  self.send_response(200);self.end_headers();self.wfile.write(b'{}')
 def log_message(self,*args):pass
http.server.HTTPServer(('0.0.0.0',8080),Handler).serve_forever()
'''

class Refused(Exception):
    pass


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'))


def sha(value):
    return hashlib.sha256(value).hexdigest()


def history_request_fingerprint(task, canary=False):
    params = {'provider': 'fixture'}
    if canary:
        params['generation_async_canary'] = True
    # pe.Fingerprint hashes struct fields in this order, map fields sorted.
    return sha(('{"task_id":' + canonical(task) + ',"provider":"fixture","model":"fixture","capability":"image","params":' + canonical(params) + '}').encode('utf-8'))


def clean_env():
    # Never inherit test/DB/provider or docker context/remote daemon settings.
    return {k: v for k, v in os.environ.items() if not k.upper().startswith(('DOCKER_', 'COMPOSE_', 'POSTGRES', 'PG', 'DATABASE', 'XIANZHI_', 'QUARANTINE_', 'PROVIDER', 'CME_', 'OPENAI', 'REDIS', 'RABBITMQ'))}


class Docker:
    def __init__(self):
        self.env = clean_env()
        self.deadline = None
        self.evidence_dir = os.path.join(ROOT, '.evidence', 'issue203', 'priority4-runtime-capability')
        self.redactions = []
        self.cli = [shutil.which('docker') or 'docker']
        self.cli += ['--host', 'npipe:////./pipe/dockerDesktopLinuxEngine' if os.name == 'nt' else 'unix:///var/run/docker.sock']

    def redact(self, value):
        for secret in self.redactions:
            value = value.replace(secret, '[redacted]')
        return value

    def run(self, args, data=None, timeout=60, check=True, capture_limit=16 * 1024 * 1024):
        if self.deadline is not None:
            remaining = self.deadline - time.monotonic()
            if remaining <= 0:
                raise Refused('owned challenge deadline exceeded')
            timeout = min(timeout, remaining)
        if type(capture_limit) is not int or capture_limit <= 0:
            raise Refused('invalid Docker output capture limit')
        argv = self.cli + list(args)
        process = subprocess.Popen(argv, stdin=subprocess.PIPE if data is not None else None, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=self.env, cwd=ROOT)
        captured = {'stdout': bytearray(), 'stderr': bytearray()}
        truncated = {'stdout': False, 'stderr': False}

        def drain(name, pipe):
            while True:
                chunk = pipe.read(65536)
                if not chunk:
                    break
                buffer = captured[name]
                if len(chunk) >= capture_limit:
                    buffer[:] = chunk[-capture_limit:]
                    truncated[name] = True
                else:
                    overflow = len(buffer) + len(chunk) - capture_limit
                    if overflow > 0:
                        del buffer[:overflow]
                        truncated[name] = True
                    buffer.extend(chunk)
            pipe.close()

        readers = [threading.Thread(target=drain, args=('stdout', process.stdout)), threading.Thread(target=drain, args=('stderr', process.stderr))]
        for reader in readers:
            reader.daemon = True
            reader.start()
        writer = None
        if data is not None:
            def feed_input():
                try:
                    process.stdin.write(data)
                    process.stdin.flush()
                except (IOError, OSError):
                    pass
                finally:
                    try:
                        process.stdin.close()
                    except (IOError, OSError):
                        pass
            writer = threading.Thread(target=feed_input)
            writer.daemon = True
            writer.start()
        timed_out = False
        try:
            returncode = process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            process.kill()
            returncode = process.wait()
        if writer is not None:
            writer.join()
        for reader in readers:
            reader.join()
        result = subprocess.CompletedProcess(argv, returncode, bytes(captured['stdout']), bytes(captured['stderr']))
        result.stdout_truncated = truncated['stdout']
        result.stderr_truncated = truncated['stderr']
        if timed_out:
            raise subprocess.TimeoutExpired(argv, timeout, output=result.stdout, stderr=result.stderr)
        if check and result.returncode:
            stdout = self.redact(result.stdout.decode('utf-8', errors='replace'))[-4000:]
            stderr = self.redact(result.stderr.decode('utf-8', errors='replace'))[-4000:]
            diagnostic = {
                'argv': [self.redact(str(value)) for value in argv],
                'stdin_bytes': len(data) if data is not None else 0,
                'stdin_sha256': hashlib.sha256(data).hexdigest() if data is not None else None,
                'stdout_truncated': result.stdout_truncated,
                'stderr_truncated': result.stderr_truncated,
                'stdout': stdout,
                'stderr': stderr,
            }
            raise Refused('Docker operation failed: ' + args[0] + ' (exit ' + str(result.returncode) + '); diagnostic=' + json.dumps(diagnostic, sort_keys=True))
        if result.stdout_truncated or result.stderr_truncated:
            raise Refused('Docker output exceeded bounded capture limit')
        return result

    def text(self, args, **kw):
        return self.run(args, **kw).stdout.decode('utf-8').strip()

    def inspect(self, kind, identity):
        data = json.loads(self.text([kind, 'inspect', identity]))
        if len(data) != 1:
            raise Refused('ambiguous resource identity')
        return data[0]


def image_identity(docker, ref, release_sha, synthetic=False):
    if not re.match(r'^[0-9a-f]{40}$', release_sha):
        raise Refused('invalid release SHA')
    if not synthetic and not re.match(r'^.+@sha256:[0-9a-f]{64}$', ref):
        raise Refused('immutable image reference required')
    obj = docker.inspect('image', ref)
    image_id = obj['Id']
    if not re.match(r'^sha256:[0-9a-f]{64}$', image_id):
        raise Refused('invalid local image ID')
    if obj.get('Config', {}).get('Entrypoint'):
        raise Refused('packaged startup entrypoint must match explicit empty policy')
    digests = sorted(obj.get('RepoDigests') or [])
    if not synthetic and ref not in digests:
        raise Refused('RepoDigests membership missing')
    if obj.get('Os') != 'linux' or obj.get('Architecture') != 'amd64':
        raise Refused('unsupported platform: explicit linux/amd64 required')
    output = docker.text(['run', '--rm', '--pull', 'never', '--network', 'none', '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--entrypoint', '/usr/bin/sha256sum', image_id] + list(BINARIES))
    hashes = {}
    for line in output.splitlines():
        digest, path = line.split(None, 1)
        if path not in BINARIES or not re.match(r'^[0-9a-f]{64}$', digest):
            raise Refused('packaged binary identity invalid')
        hashes[path] = digest
    if set(hashes) != set(BINARIES):
        raise Refused('packaged binary inventory incomplete')
    return {'reference': ref, 'local_image_id': image_id, 'repo_digests': digests, 'platform': {'os': obj['Os'], 'architecture': obj['Architecture'], 'variant': obj.get('Variant', '')}, 'binary_sha256': hashes, 'release_sha': release_sha}


def runtime_policy(model):
    # Full effective Compose config is also separately HMAC-bound. This
    # additional policy rejects binary replacement/challenge startup paths.
    services = model.get('services', {})
    roles = {}
    for service, binary in (('xianzhi-ai', '/app/xianzhi-api'), ('generation-worker', '/app/generation-worker'), ('smartvideo-worker', '/app/smartvideo-worker')):
        if service not in services:
            if service == 'generation-worker':
                continue
            raise Refused('required process missing: ' + service)
        cfg = services[service]
        command = cfg.get('command') or [binary]
        if isinstance(command, str):
            command = command.split()
        entrypoint = cfg.get('entrypoint') or []
        if entrypoint or command != [binary]:
            raise Refused('unexpected process policy: ' + service)
        for mount in cfg.get('volumes') or []:
            target = mount.get('target', '') if isinstance(mount, dict) else mount.split(':')[-2]
            if any(path == target or path.startswith(target.rstrip('/') + '/') for path in BINARIES):
                raise Refused('binary replacement mount forbidden')
        env = cfg.get('environment') or {}
        if service != 'smartvideo-worker' and (env.get('XIANZHI_ENV') != 'production' or not env.get('DATABASE_URL')):
            raise Refused('generation production PostgreSQL policy required')
        if any(k.startswith('QUARANTINE_FIXTURE') for k in env):
            raise Refused('challenge config forbidden in normal service')
        roles[service] = {'binary': binary, 'command': command, 'entrypoint': entrypoint, 'environment_sha256': sha(canonical(env).encode('utf-8')), 'volumes': cfg.get('volumes') or [], 'user': cfg.get('user') or ''}
    return {'version': VERSION, 'roles': roles, 'applicable_generation_roles': sorted(ROLES), 'excluded_roles': {'smartvideo-worker': 'separate smartvideo job pipeline, not generation quarantine coverage', 'video-backfill': 'maintenance command, not an enrolled generation consumer'}}


class Fixture:
    def __init__(self, docker):
        self.docker = docker
        self.owner = str(uuid.uuid4())
        self.password = uuid.uuid4().hex + uuid.uuid4().hex
        self.name = 'p4_' + self.owner.replace('-', '')
        self.docker.redactions.extend((self.password, self.owner, self.name))
        self.resources = []
        self.record_path = os.path.join(docker.evidence_dir, 'owned-' + self.owner + '.json')
        self.network = None
        self.db = None
        self.sink = None

    def record(self, kind, identity):
        obj = self.docker.inspect(kind, identity)
        labels = obj.get('Labels') if kind == 'network' else obj.get('Config', {}).get('Labels')
        if (labels or {}).get(LABEL) != self.owner:
            raise Refused('resource owner mismatch')
        self.resources.append((kind, obj['Id']))
        os.makedirs(os.path.dirname(self.record_path), exist_ok=True)
        with open(self.record_path, 'w', encoding='utf-8') as stream:
            json.dump({'owner': self.owner, 'resources': self.resources}, stream, sort_keys=True)
        return obj['Id']

    def owned(self, kind, identity):
        if (kind, identity) not in self.resources:
            raise Refused('unrecorded resource')
        obj = self.docker.inspect(kind, identity)
        labels = obj.get('Labels') if kind == 'network' else obj.get('Config', {}).get('Labels')
        if obj['Id'] != identity or (labels or {}).get(LABEL) != self.owner:
            raise Refused('resource ownership changed')
        return obj

    def sql(self, query):
        self.owned('container', self.db)
        try:
            return self.docker.text(['exec', '-i', self.db, 'psql', '-X', '-U', 'fixture_admin', '-d', self.name, '-v', 'ON_ERROR_STOP=1', '-At'], data=query.encode('utf-8'), timeout=90)
        except Refused as failure:
            context = {}
            owned_for_diagnostics = False
            try:
                obj = self.owned('container', self.db)
                owned_for_diagnostics = True
                state = obj.get('State', {})
                context['container_state'] = {key: state.get(key) for key in ('Status', 'Running', 'Restarting', 'OOMKilled', 'ExitCode', 'StartedAt', 'FinishedAt')}
                if state.get('Error'):
                    context['container_state']['Error'] = self.docker.redact(str(state['Error']))[-1000:]
                context['restart_count'] = obj.get('RestartCount')
            except Exception as error:
                context['container_inspect_error'] = self.docker.redact(str(error))[-1000:]
            if owned_for_diagnostics:
                try:
                    logs = self.docker.run(['logs', '--tail', '100', self.db], timeout=20, check=False, capture_limit=32768)
                    context['container_logs'] = self.docker.redact((logs.stdout + logs.stderr).decode('utf-8', errors='replace'))[-12000:]
                    context['container_logs_exit'] = logs.returncode
                except Exception as error:
                    context['container_logs_error'] = self.docker.redact(str(error))[-1000:]
            else:
                context['container_logs'] = '[skipped: owned-container revalidation failed]'
            raise Refused(str(failure) + '; fixture_database=' + json.dumps(context, sort_keys=True))

    def provision(self, image_id):
        started = time.monotonic()
        remaining = 'unset' if self.docker.deadline is None else '%.3f' % (self.docker.deadline - started)
        print('CAPABILITY_PHASE provision-start remaining_seconds=' + remaining, flush=True)
        pg = self.docker.inspect('image', 'pgvector/pgvector:pg16')['Id']
        self.network = self.record('network', self.docker.text(['network', 'create', '--internal', '--label', LABEL + '=' + self.owner, 'p4-' + self.owner]))
        args = ['run', '-d', '--pull', 'never', '--label', LABEL + '=' + self.owner, '--network', self.network, '--network-alias', 'fixture-db', '--tmpfs', '/var/lib/postgresql/data', '-e', 'POSTGRES_USER=fixture_admin', '-e', 'POSTGRES_PASSWORD=' + self.password, '-e', 'POSTGRES_DB=' + self.name, pg]
        self.db = self.record('container', self.docker.text(args))
        for attempt in range(40):
            ready = self.docker.run(['exec', self.db, 'pg_isready', '-U', 'fixture_admin', '-d', self.name], check=False)
            if ready.returncode == 0:
                probe = self.docker.run(['exec', self.db, 'psql', '-X', '-U', 'fixture_admin', '-d', self.name, '-v', 'ON_ERROR_STOP=1', '-Atqc', 'SELECT 1'], check=False, timeout=10)
                if probe.returncode == 0 and probe.stdout.strip() == b'1':
                    break
            time.sleep(0.5)
        else:
            raise Refused('fixture database readiness timeout')
        # Source SQL is sent only to our recorded disposable database, never
        # through production transport or host PostgreSQL ports.
        with open(os.path.join(ROOT, 'database/schema.sql'), 'r', encoding='utf-8') as stream:
            schema_parts = [stream.read()]
        directory = os.path.join(ROOT, 'database/migrations')
        for path in sorted(os.listdir(directory)):
            if re.match(r'^[0-9]{3}-.*\.sql$', path) and not path.endswith('.down.sql'):
                with open(os.path.join(directory, path), 'r', encoding='utf-8') as stream:
                    schema_parts.append(stream.read())
        print('CAPABILITY_PHASE migration-replay-start elapsed_seconds=%.3f' % (time.monotonic() - started), flush=True)
        self.sql('\n'.join(schema_parts))
        self.sql("CREATE ROLE {0} LOGIN PASSWORD '{1}'; GRANT USAGE ON SCHEMA public TO {0}; GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA public TO {0}; GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO {0}; CREATE TABLE quarantine_capability_fixture(owner text PRIMARY KEY); INSERT INTO quarantine_capability_fixture VALUES ('{2}'); GRANT SELECT ON quarantine_capability_fixture TO {0};".format(self.name, self.password, self.owner))
        self.sink = self.record('container', self.docker.text(['run', '-d', '--pull', 'never', '--label', LABEL + '=' + self.owner, '--network', self.network, '--network-alias', 'fixture-sink', '--read-only', '--tmpfs', '/tmp', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--entrypoint', '/usr/bin/python3', image_id, '-u', '-c', SINK]))

        print('CAPABILITY_PHASE provision-complete elapsed_seconds=%.3f' % (time.monotonic() - started), flush=True)

    def seed(self, blocked):
        statements = []
        # Independent fixture state; no candidate is allowed to provision it.
        statements.append("INSERT INTO xz_users(id,name,role,status) VALUES('{0}','fixture','MEMBER','ACTIVE');".format(self.owner))
        for op in OPS:
            task = self.owner + '-' + op
            queued = op == 'status'
            statements.append("INSERT INTO xz_generation_tasks(id,user_id,type,status,task_status,execution_generation,worker_id,lease_until,params,raw) VALUES('{0}','{1}','TEXT_TO_IMAGE','{2}','{3}',1,'fixture-worker',now()+interval '30 seconds','{{}}','{{}}');".format(task, self.owner, 'QUEUED' if queued else 'PROCESSING', 'QUEUED' if queued else 'RUNNING'))
            if queued:
                statements.append("UPDATE xz_generation_tasks SET worker_id=NULL,lease_until=NULL WHERE id='{0}';".format(task))
            if op.startswith('video-'):
                statements.append("UPDATE xz_generation_tasks SET type='TEXT_TO_VIDEO' WHERE id='{0}';".format(task))
            if blocked or op not in ('provider', 'video-provider', 'status'):
                status = 'failed' if op == 'create' and not blocked else ('submitting' if op == 'result' else 'prepared')
                statements.append("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,request_fingerprint,task_execution_generation) VALUES('{0}','fixture','fixture','image',1,'{1}','{2}',1);".format(task, status, '1' * 64))
            if blocked:
                statements.append("INSERT INTO provider_execution_quarantine(execution_id,task_id,attempt,generation,snapshot_sha256,evidence_sha256,approval_id,release_sha,not_before,expires_at) SELECT id,task_id,1,1,'{0}','{0}','synthetic-nonofficial','{1}',now()-interval '1 minute',now()+interval '1 hour' FROM provider_executions WHERE task_id='{2}';".format('1' * 64, '1' * 40, task))
        video_task = self.owner + '-video-persistence'
        statements.append("INSERT INTO xz_storage_configs(id,tenant_id,name,provider,endpoint,bucket) VALUES('{0}-video-config','tenant_default','owned synthetic existing video','s3','http://fixture-sink:8080','fixture'); INSERT INTO xz_file_objects(file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name,mime_type,file_size,business_type,business_id,visibility,status) VALUES('{0}-video-file','tenant_default','{0}','{0}-video-config','s3','fixture','owned-video','{1}-01.mp4','{1}-01.mp4','video/mp4',5,'generation_result','{1}','PRIVATE','ACTIVE');".format(self.owner, video_task))
        # Seed lots/reservations through fixed disposable fixture SQL, never
        # grant/override/admin operations in the candidate.
        for op in ('reserve', 'capture', 'release'):
            task = self.owner + '-' + op
            statements.append("INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw) VALUES('{0}','{1}',100,{2},'{{\"totalGranted\":110}}'); INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_id,original_points,available_points,reserved_points,idempotency_key,status,granted_at) VALUES('{0}','{0}','{1}','RECHARGE','fixture',100+{2},100,{2},'fixture','ACTIVE',now());".format(task, self.owner, 0 if op == 'reserve' else 10))
            if op != 'reserve':
                statements.append("INSERT INTO xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,idempotency_key,status) VALUES('{0}','{0}','{1}','GENERATION_TASK','{0}',10,10,'fixture','RESERVED'); INSERT INTO xz_personal_point_reservation_allocations(id,reservation_id,lot_id,account_id,user_id,allocated_points,reserved_points,status) VALUES('{0}','{0}','{0}','{0}','{1}',10,10,'RESERVED');".format(task, self.owner))

        self.sql("\n".join(statements))

    def seed_history(self):
        statements = ["CREATE TABLE IF NOT EXISTS public.schema_migrations(filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());",
                      "INSERT INTO public.schema_migrations(filename) VALUES('119-execution-generation-fencing.sql') ON CONFLICT DO NOTHING;",
                      "INSERT INTO xz_users(id,name,role,status) VALUES('{0}','fixture','MEMBER','ACTIVE');".format(self.owner)]
        for op in HISTORY_OPS:
            task = self.owner + '-' + op
            if op == 'orphan':
                statements.append("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,error_class,request_fingerprint,task_execution_generation,created_at,updated_at) VALUES('{0}','fixture','fixture','image',1,'failed','definitive_not_submitted','{1}',NULL,'2001-01-01','2001-01-02');".format(task, history_request_fingerprint(task)))
                continue
            params = {'provider': 'fixture'}
            if not op.endswith('legacy'):
                params['generation_dispatch_mode'] = 'canary' if 'canary' in op else 'normal'
            if 'canary' in op:
                params['generation_async_canary'] = True
            kind = 'IMAGE_TO_IMAGE' if op.startswith('image') else 'TEXT_TO_IMAGE'
            raw = dict(id=task, userId=self.owner, type=kind, status='PROCESSING', model='', prompt='', billingAccountType='PERSONAL', params=params, updatedAt='2001-01-01T00:00:00Z')
            worker = 'NULL' if op == 'text-normal-legacy' else ("''" if op == 'image-normal-legacy' else "'fixture-unrelated-worker'")
            statements.append("INSERT INTO xz_generation_tasks(id,user_id,type,model,prompt,status,task_status,execution_generation,worker_id,lease_until,last_heartbeat_at,updated_at,params,raw,billing_status) VALUES('{0}','{1}','{2}','','','PROCESSING','DISPATCHING',3,{5},now()-interval '1 hour',now()-interval '2 hours','2001-01-01T00:00:00Z','{3}','{4}','RESERVED');".format(task, self.owner, kind, canonical(params), canonical(raw), worker))
            manifest = [{'URL': 'data:image/png;base64,aGVsbG8=', 'ContentType': 'image/png'}]
            fingerprint = history_request_fingerprint(task, 'canary' in op)
            statements.append("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,error_class,request_fingerprint,task_execution_generation,result_metadata) VALUES('{0}','fixture','fixture','image',1,'succeeded','provider_succeeded','{1}',2,'{2}');".format(task, fingerprint, canonical(manifest)))
        self.sql('\n'.join(statements))

    def seed_history_control(self):
        # Separate funded user/account from historical owners. This is a valid
        # normal dispatch consumed by the actual packaged worker entrypath.
        task, user = self.owner + '-normal-control', self.owner + '-control-user'
        account, reservation, lot = task + '-account', task + '-reservation', task + '-lot'
        channel = task + '-channel'
        params = {'generation_dispatch_mode': 'normal', '_generation_dispatch_owner': 'fixture-control-dispatch', 'billingReserved': True, 'billingReservationPointCost': 10, 'provider': channel}
        channel_raw = dict(id=channel, name=channel, baseUrl='http://fixture-sink:8080', protocol='openai', status='ACTIVE', models=['gpt-image-2'], imageGenerationEndpoint='/consumer-provider')
        key_raw = dict(id=channel + '-key', customer=channel, secret='synthetic-consumer-fixture-key', status='ACTIVE')
        raw = dict(id=task, userId=user, type='TEXT_TO_IMAGE', model='gpt-image-2', prompt='funded consumer control', status='PROCESSING', params=params, personalPointAccountId=account, personalPointReservationId=reservation, billingEngine='PERSONAL_LOT_V1', billingStatus='RESERVED', pointCost=10)
        self.sql("""
INSERT INTO xz_api_channels(id,name,base_url,protocol,status,raw) VALUES('{channel}','{channel}','http://fixture-sink:8080','openai','ACTIVE','{channel_raw}');
INSERT INTO xz_api_keys(id,customer,status,raw) VALUES('{channel}-key','{channel}','ACTIVE','{key_raw}');
INSERT INTO xz_users(id,name,role,status) VALUES('{user}','fixture normal consumer','MEMBER','ACTIVE');
INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw) VALUES('{account}','{user}',100,10,'{{"totalGranted":110}}');
INSERT INTO xz_user_wallets(user_id,token_balance,frozen_token,total_token_granted) VALUES('{user}',100,10,110);
INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_id,original_points,available_points,reserved_points,idempotency_key,status,granted_at) VALUES('{lot}','{account}','{user}','RECHARGE','fixture',110,100,10,'fixture','ACTIVE',now());
INSERT INTO xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,idempotency_key,status) VALUES('{reservation}','{account}','{user}','GENERATION_TASK','{task}',10,10,'generation:reserve:{task}','RESERVED');
INSERT INTO xz_personal_point_reservation_allocations(id,reservation_id,lot_id,account_id,user_id,allocated_points,reserved_points,status) VALUES('{reservation}','{reservation}','{lot}','{account}','{user}',10,10,'RESERVED');
INSERT INTO xz_generation_tasks(id,user_id,type,model,prompt,status,task_status,execution_generation,worker_id,params,raw,point_cost,reserved_points,billing_status) VALUES('{task}','{user}','TEXT_TO_IMAGE','gpt-image-2','funded consumer control','PROCESSING','DISPATCHING',1,'fixture-control-dispatch','{params}','{raw}',10,10,'RESERVED');
""".format(task=task, user=user, account=account, reservation=reservation, lot=lot, params=canonical(params), raw=canonical(raw), channel=channel, channel_raw=canonical(channel_raw), key_raw=canonical(key_raw)))

    def snapshot(self, tables=TABLES):
        parts = []
        for table in tables:
            parts += ["'" + table + "'", "(SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]') FROM " + table + ' t)']
        return json.loads(self.sql('SELECT jsonb_build_object(' + ','.join(parts) + ');'))

    def effects(self):
        self.owned('container', self.sink)
        text = self.docker.text(['exec', self.sink, '/bin/sh', '-c', 'cat /tmp/effects 2>/dev/null || true'])
        return [json.loads(line) for line in text.splitlines()]

    def challenge(self, image_id, binary, phase):
        started = time.monotonic()
        remaining = 'unset' if self.docker.deadline is None else '%.3f' % (self.docker.deadline - started)
        print('CAPABILITY_PHASE challenge-start role=%s phase=%s remaining_seconds=%s' % (binary, phase, remaining), flush=True)
        identity = self.record('container', self.docker.text(['create', '--pull', 'never', '--label', LABEL + '=' + self.owner, '--network', self.network, '--read-only', '--tmpfs', '/tmp', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--pids-limit', '64', '--memory', '256m', '--env', 'QUARANTINE_FIXTURE_PASSWORD=' + self.password, '--entrypoint', binary, image_id, '--quarantine-capability-v1', self.owner, phase]))
        network = self.owned('network', self.network)
        self.owned('container', self.db)
        self.owned('container', self.sink)
        if not network['Internal'] or set(network.get('Containers', {})) - {self.db, self.sink}:
            raise Refused('fixture network is not exclusively owned/internal')
        obj = self.owned('container', identity)
        if obj['Image'] != image_id or obj.get('Mounts') or obj['HostConfig'].get('PortBindings') or obj['HostConfig']['NetworkMode'] != self.network or not obj['HostConfig']['ReadonlyRootfs']:
            raise Refused('candidate isolation mismatch')
        result = self.docker.run(['start', '-a', identity], timeout=40, check=False, capture_limit=65536)
        obj = self.owned('container', identity)
        diagnostic = self.docker.redact((result.stdout + result.stderr).decode('utf-8', errors='replace'))
        self.diagnostic = diagnostic[-4000:]
        if result.stdout_truncated or result.stderr_truncated or len(result.stdout) > 65536:
            raise Refused('packaged challenge output exceeds 65536-byte limit')
        print('CAPABILITY_PHASE challenge-complete role=%s phase=%s exit_code=%s elapsed_seconds=%.3f' % (binary, phase, obj['State']['ExitCode'], time.monotonic() - started), flush=True)
        return obj['State']['ExitCode'], result.stdout.decode('utf-8', errors='replace')

    def begin_history_observation(self, identity, role, phase):
        self.observation_path = os.path.join(os.path.dirname(self.record_path),
                                             'observation-' + self.owner + '-' + phase + '.json')
        self.observation = dict(fixture_nonofficial=True, owner_label=LABEL, owner=self.owner,
                                role=role, phase=phase, image_identity=identity, status='incomplete',
                                before=None, after=None, effects=None, exit_code=None, stdout=None,
                                diagnostic='', unavailable={field: 'not observed' for field in
                                ('before', 'after', 'effects', 'exit_code', 'stdout', 'validation')})
        self.write_history_observation()

    def write_history_observation(self):
        self.observation['resources'] = list(self.resources)
        os.makedirs(os.path.dirname(self.observation_path), exist_ok=True)
        with open(self.observation_path, 'w', encoding='utf-8') as stream:
            json.dump(self.observation, stream, indent=2, sort_keys=True)

    def finish_history_observation(self, error=None):
        if not hasattr(self, 'observation'):
            return
        self.observation['status'] = 'failed' if error is not None else 'validated'
        if error is None:
            self.observation['unavailable'].pop('validation', None)
        else:
            self.observation['error'] = self.docker.redact(type(error).__name__ + ': ' + str(error))[-4000:]
            self.observation['unavailable']['validation'] = 'not accepted'
        self.write_history_observation()

    def observe_history_challenge(self, image_id, binary, phase):
        # Persist independently sampled bytes BEFORE the candidate can fail or
        # cleanup can destroy the DB. Missing after/effects remain explicit,
        # never empty snapshots that could imply zero side effects.
        before = self.snapshot(HISTORY_TABLES)
        self.observation['before'] = before
        self.observation['unavailable'].pop('before')
        self.write_history_observation()
        failure = None
        code, output = None, None
        try:
            code, output = self.challenge(image_id, binary, phase)
            self.observation.update(exit_code=code, stdout=self.docker.redact(output)[-4000:],
                                    diagnostic=self.docker.redact(getattr(self, 'diagnostic', ''))[-4000:])
            for field in ('exit_code', 'stdout'):
                self.observation['unavailable'].pop(field)
            self.write_history_observation()
        except Exception as error:
            failure = error
            self.observation['diagnostic'] = self.docker.redact(type(error).__name__ + ': ' + str(error))[-4000:]
            captured = getattr(error, 'output', None)
            if isinstance(captured, bytes):
                captured = captured.decode('utf-8', errors='replace')
            if isinstance(captured, str):
                self.observation['stdout'] = self.docker.redact(captured)[-4000:]
                self.observation['unavailable'].pop('stdout', None)
            self.write_history_observation()
        for field, observe in (('after', lambda: self.snapshot(HISTORY_TABLES)), ('effects', self.effects)):
            try:
                self.observation[field] = observe()  # original absolute deadline still applies
                self.observation['unavailable'].pop(field)
            except Exception as error:
                self.observation['unavailable'][field] = self.docker.redact(type(error).__name__ + ': ' + str(error))[-4000:]
                if failure is None:
                    failure = error
            self.write_history_observation()
        if failure is not None:
            raise failure
        return before, self.observation['after'], self.observation['effects'], code, output

    def cleanup(self):
        deadline = self.docker.deadline
        self.docker.deadline = None  # cleanup must still run after timeout
        try:
            errors = []
            for kind, identity in reversed(self.resources):
                try:
                    self.owned(kind, identity)
                    self.docker.run([kind, 'rm'] + (['-f', '-v'] if kind == 'container' else []) + [identity])
                except Exception:
                    errors.append(identity)
            os.makedirs(os.path.dirname(self.record_path), exist_ok=True)
            with open(self.record_path, 'w', encoding='utf-8') as stream:
                json.dump({'owner': self.owner, 'resources': self.resources, 'cleanup': 'failed' if errors else 'complete', 'errors': errors}, stream, sort_keys=True)
            if errors:
                raise Refused('owned fixture cleanup failed: ' + ','.join(errors))
        finally:
            # One whole-attestation budget, not a new budget after each fixture.
            self.docker.deadline = deadline


def check_allowed(before, after, owner, effects):
    def row(table, identifier):
        rows = [r for r in after[table] if r.get('id') == identifier]
        if len(rows) != 1:
            raise Refused('normal fixture identity missing: ' + table)
        return rows[0]
    tasks = {op: row('xz_generation_tasks', owner + '-' + op) for op in OPS}
    status = tasks['status']
    if status['status'] != 'PROCESSING' or status['task_status'] != 'RUNNING' or status['worker_id'] != 'fixture-worker' or status['execution_generation'] != 2:
        raise Refused('normal status/ownership effects missing')
    previous_lease = next(r for r in before['xz_generation_tasks'] if r['id'] == owner + '-lease')
    if tasks['lease']['lease_until'] <= previous_lease['lease_until'] or not tasks['lease']['last_heartbeat_at']:
        raise Refused('normal lease effect missing')
    for op, status in (('provider', 'succeeded'), ('video-provider', 'succeeded'), ('execution-claim', 'submitting'), ('transition', 'submitting'), ('result', 'succeeded')):
        rows = [r for r in after['provider_executions'] if r['task_id'] == owner + '-' + op]
        if len(rows) != 1 or rows[0]['status'] != status:
            raise Refused('normal execution effect missing: ' + op)
        if op == 'result' and rows[0]['result_metadata'] != {'fixture': True}:
            raise Refused('normal result metadata missing')
    created = [r for r in after['provider_executions'] if r['task_id'] == owner + '-create' and r['attempt'] == 2 and r['status'] == 'prepared']
    correlations = after['provider_execution_correlations']
    if len(created) != 1 or len(correlations) != 4:
        raise Refused('normal create/correlation effects missing')
    cid = next(r['id'] for r in after['provider_executions'] if r['task_id'] == owner + '-correlation')
    if len([r for r in correlations if r['execution_id'] == cid and r['kind'] == 'submit']) != 1:
        raise Refused('normal explicit correlation missing')
    if len([r for r in after['xz_assets'] if r['task_id'] == owner + '-asset']) != 1 or len([r for r in after['xz_file_objects'] if r['business_id'] == owner + '-persistence' and r['status'] == 'ACTIVE']) != 1:
        raise Refused('normal asset/private persistence effect missing')
    for op, available, frozen in (('reserve', 90, 10), ('capture', 100, 5), ('release', 105, 5)):
        account = row('xz_point_accounts', owner + '-' + op)
        if (account['available'], account['frozen']) != (available, frozen):
            raise Refused('normal billing balance effect missing: ' + op)
    if len(after['xz_wallet_ledger']) != len(before['xz_wallet_ledger']) + 3 or len(after['xz_personal_point_lot_movements']) != len(before['xz_personal_point_lot_movements']) + 3:
        raise Refused('normal billing ledger/movement effects missing')
    existing_before = next(r for r in before['xz_file_objects'] if r['file_id'] == owner + '-video-file')
    existing_after = next(r for r in after['xz_file_objects'] if r['file_id'] == owner + '-video-file')
    if existing_before != existing_after:
        raise Refused('video durable reuse unexpectedly mutated existing file')
    if sorted(effect['path'] for effect in effects) not in (['/object', '/provider', '/provider-video'], ['/object', '/provider', '/provider-query', '/provider-video']) or any(effect['bytes'] <= 0 for effect in effects if effect['path'] != '/provider-query'):
        raise Refused('normal independent synthetic sink effect missing')


def challenge_budget(synthetic, include_history):
    return 900 if synthetic else (300 if include_history else 180)


def attest(ref, release_sha, policy, synthetic=False, evidence_directory=None, include_history=True):
    docker = Docker()
    if evidence_directory is not None:
        docker.evidence_dir = os.path.abspath(evidence_directory)
    # Synthetic CI replay provisions isolated PostgreSQL fixtures and applies
    # the full migration history repeatedly; give it a bounded but realistic
    # budget. Authenticated runtime verification does not use this path.
    deadline = time.monotonic() + challenge_budget(synthetic, include_history)
    docker.deadline = deadline
    identity = image_identity(docker, ref, release_sha, synthetic)
    observations = []
    for role, binary in sorted(ROLES.items()):
        for phase in ('blocked', 'allowed'):
            docker.deadline = deadline
            fixture = Fixture(docker)
            try:
                fixture.provision(identity['local_image_id'])
                fixture.seed(phase == 'blocked')
                before = fixture.snapshot()
                code, output = fixture.challenge(identity['local_image_id'], binary, phase)
                after = fixture.snapshot()
                effects = fixture.effects()
                observation_path = os.path.join(os.path.dirname(fixture.record_path), 'observation-' + fixture.owner + '.json')
                with open(observation_path, 'w', encoding='utf-8') as stream:
                    json.dump({'role': role, 'phase': phase, 'image_identity': identity, 'before': before, 'after': after, 'effects': effects, 'exit_code': code, 'stdout': docker.redact(output)[-4000:]}, stream, indent=2, sort_keys=True)
                if phase == 'blocked' and (before != after or effects):
                    raise Refused('independently observed forbidden effects')
                if code:
                    raise Refused('packaged production challenge rejected: ' + fixture.diagnostic[-4000:])
                response = json.loads(output.strip().splitlines()[-1])
                if response.get('owner') != fixture.owner or response.get('role') != role or response.get('release_sha') != release_sha or response.get('phase') != phase or set(response.get('operations', {})) != set(OPS) or response.get('nil_dependency_checks') != 6:
                    raise Refused('behavior protocol/release mismatch')
                if phase == 'blocked' and any(value != 'EXECUTION_QUARANTINED_READONLY' for value in response['operations'].values()):
                    raise Refused('actual quarantine error absent')
                if phase == 'allowed':
                    check_allowed(before, after, fixture.owner, effects)
                    if before == after or sorted(effect['path'] for effect in effects) not in (['/object', '/provider', '/provider-video'], ['/object', '/provider', '/provider-query', '/provider-video']):
                        raise Refused('normal control effects absent')
                    for table in TABLES:
                        if table == 'xz_storage_configs':
                            continue  # an existing config may legitimately be reused
                        if before[table] == after[table]:
                            raise Refused('normal control table unchanged: ' + table)
                observations.append({'role': role, 'phase': phase, 'owner': fixture.owner, 'before_sha256': sha(canonical(before).encode('utf-8')), 'after_sha256': sha(canonical(after).encode('utf-8')), 'effects': effects, 'response': response, 'before': before, 'after': after})
            finally:
                fixture.cleanup()
    unavailable = []
    for role, binary in sorted(ROLES.items()):
        for mode in ('missing', 'denied', 'timeout'):
            docker.deadline = deadline
            fixture = Fixture(docker)
            try:
                fixture.provision(identity['local_image_id'])
                fixture.seed(False)
                if mode == 'missing':
                    fixture.sql('DROP TABLE provider_execution_quarantine CASCADE;')
                elif mode == 'denied':
                    fixture.sql('REVOKE SELECT ON provider_execution_quarantine FROM ' + fixture.name + ';')
                else:
                    fixture.sql("ALTER ROLE " + fixture.name + " SET statement_timeout='50ms';")
                    docker.run(['exec', '-d', fixture.db, 'psql', '-X', '-U', 'fixture_admin', '-d', fixture.name, '-c', "BEGIN; LOCK TABLE provider_execution_quarantine IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(20); ROLLBACK;"])
                    for attempt in range(20):
                        if fixture.sql("SELECT count(*) FROM pg_locks WHERE relation='provider_execution_quarantine'::regclass AND mode='AccessExclusiveLock' AND granted;") == '1':
                            break
                        time.sleep(0.1)
                    else:
                        raise Refused('timeout fault was not independently installed')
                before = fixture.snapshot()
                code, output = fixture.challenge(identity['local_image_id'], binary, 'unavailable')
                after = fixture.snapshot()
                effects = fixture.effects()
                if before != after or effects or code:
                    raise Refused('required database dependency failed open: ' + mode + ': ' + getattr(fixture, 'diagnostic', '')[-1000:])
                response = json.loads(output.strip().splitlines()[-1])
                if any(x != 'quarantine barrier unavailable' for x in response.get('operations', {}).values()) or set(response.get('operations', {})) != set(OPS) or response.get('release_sha') != release_sha or response.get('nil_dependency_checks') != 6:
                    raise Refused('required database failure rejection missing')
                unavailable.append({'role': role, 'mode': mode, 'owner': fixture.owner, 'before_sha256': sha(canonical(before).encode('utf-8')), 'after_sha256': sha(canonical(after).encode('utf-8')), 'effects': effects, 'response': response})
            finally:
                fixture.cleanup()
    docker.deadline = deadline
    if image_identity(docker, ref, release_sha, synthetic) != identity:
        raise Refused('image identity changed during challenge')
    history = []
    history_controls = []
    if include_history:
        docker.deadline = deadline
        for role, binary in sorted(ROLES.items()):
            docker.deadline = deadline
            fixture = Fixture(docker)
            try:
                fixture.begin_history_observation(identity, role, 'history')
                fixture.provision(identity['local_image_id'])
                fixture.seed_history()
                before, after, effects, code, output = fixture.observe_history_challenge(identity['local_image_id'], binary, 'history')
                if code or before != after or effects:
                    raise Refused('non-enrolled history changed or rejection absent: ' + fixture.diagnostic[-4000:])
                response = json.loads(output.strip().splitlines()[-1])
                item = dict(role=role, owner=fixture.owner, before=before, after=after,
                            before_sha256=sha(canonical(before).encode('utf-8')),
                            after_sha256=sha(canonical(after).encode('utf-8')), effects=effects, response=response)
                check_history_observation(item, release_sha)
                fixture.finish_history_observation()
                history.append(item)
                fixture.begin_history_observation(identity, role, 'history-control')
                fixture.seed_history_control()
                control_before, control_after, effects, code, output = fixture.observe_history_challenge(identity['local_image_id'], binary, 'history-control')
                if code:
                    raise Refused('funded normal consumer control rejected: ' + fixture.diagnostic[-4000:])
                control = dict(role=role, owner=fixture.owner, before=control_before, after=control_after, effects=effects,
                               before_sha256=sha(canonical(control_before).encode('utf-8')),
                               after_sha256=sha(canonical(control_after).encode('utf-8')), response=json.loads(output.strip().splitlines()[-1]))
                check_history_control(control, release_sha)
                fixture.finish_history_observation()
                history_controls.append(control)
            except Exception as error:
                fixture.finish_history_observation(error)
                raise
            finally:
                fixture.cleanup()
    docker.deadline = deadline
    if image_identity(docker, ref, release_sha, synthetic) != identity:
        raise Refused('image identity changed after history challenge')
    now = int(time.time())
    evidence = {'version': VERSION, 'synthetic_nonofficial': synthetic, 'nonce': str(uuid.uuid4()), 'created_at_unix': now, 'expires_at_unix': now + 3600, 'identity': identity, 'policy': policy, 'observations': observations, 'unavailable_observations': unavailable}
    if include_history:
        evidence['history'] = {'version': HISTORY_VERSION, 'observations': history, 'controls': history_controls}
    return evidence


_history_schema = None


def history_schema():
    global _history_schema
    if _history_schema is None:
        # Reuse exact source-bound catalog descriptors, not evidence-supplied
        # fieldsets or database-discovered schemas that could bless truncation.
        path = os.path.join(ROOT, 'ops/verify-safe-drain.py')
        source = types.ModuleType('capability_history_schema')
        source.__file__ = path
        with open(path, 'rb') as stream:
            exec(compile(stream.read(), path, 'exec'), source.__dict__)
        schema = {}
        for fields in (source.live_snapshot.SCHEMA, source.live_snapshot.FINANCIAL_SCHEMA,
                       source.live_snapshot.ENTERPRISE_FINANCIAL_SCHEMA,
                       source.live_snapshot.ASSET_STORAGE_SCHEMA, source.HISTORY_SCHEMA):
            schema.update(fields)
        # Migration046; quota effects belong to packaged fixture observations.
        schema['xz_tenant_storage_quotas'] = [
            ['tenant_id', 'text', 'NO'], ['quota_bytes', 'int8', 'NO'],
            ['used_bytes', 'int8', 'NO'], ['reserved_bytes', 'int8', 'NO'],
            ['file_count', 'int8', 'NO'], ['warning_percent', 'int4', 'NO'],
            ['critical_percent', 'int4', 'NO'], ['updated_at', 'timestamptz', 'NO']]
        _history_schema = {table: schema[table] for table in HISTORY_TABLES}
    return _history_schema


def history_timestamp(value):
    if not isinstance(value, str):
        raise Refused('historical timestamp type invalid')
    match = re.fullmatch(r'(\d{4}-\d\d-\d\d)[T ](\d\d:\d\d:\d\d)(?:\.(\d{1,6}))?(Z|[+-]\d\d:\d\d)', value)
    if not match:
        raise Refused('historical timestamp incomplete')
    try:
        base = datetime.datetime.strptime(match.group(1) + 'T' + match.group(2), '%Y-%m-%dT%H:%M:%S')
        zone = match.group(4)
        minutes = 0 if zone == 'Z' else (int(zone[1:3]) * 60 + int(zone[4:])) * (1 if zone[0] == '+' else -1)
        if zone != 'Z' and (int(zone[1:3]) > 23 or int(zone[4:]) > 59):
            raise ValueError('timezone')
        return base.replace(microsecond=int((match.group(3) or '').ljust(6, '0')), tzinfo=datetime.timezone(datetime.timedelta(minutes=minutes)))
    except ValueError:
        raise Refused('historical timestamp invalid')


def check_history_snapshot(snapshot):
    if not isinstance(snapshot, dict) or set(snapshot) != set(HISTORY_TABLES):
        raise Refused('historical snapshot table fieldset incomplete')
    def json_value(value):
        if value is None or type(value) in (str, bool, int):
            return True
        if type(value) is float:
            return math.isfinite(value)
        if type(value) is list:
            return all(json_value(v) for v in value)
        return type(value) is dict and all(type(k) is str and json_value(v) for k, v in value.items())
    for table, fields in history_schema().items():
        if type(snapshot[table]) is not list:
            raise Refused('historical snapshot rows invalid: ' + table)
        for row in snapshot[table]:
            if type(row) is not dict or set(row) != {field[0] for field in fields}:
                raise Refused('historical snapshot row fieldset incomplete: ' + table)
            for field in fields:
                name, kind, nullable = field[:3]
                value = row[name]
                if value is None:
                    valid = nullable == 'YES' or kind == 'jsonb'  # JSON null is a valid non-SQL-NULL jsonb value.
                elif kind in ('int8', 'int4'):
                    bits = 64 if kind == 'int8' else 32
                    valid = type(value) is int and -(2 ** (bits - 1)) <= value < 2 ** (bits - 1)
                elif kind == 'numeric':
                    valid = type(value) in (int, float) and math.isfinite(value)
                elif kind == 'bool':
                    valid = type(value) is bool
                elif kind == 'jsonb':
                    valid = json_value(value)
                elif kind == 'timestamptz':
                    history_timestamp(value)
                    valid = True
                elif kind in ('text', 'varchar', 'bpchar'):
                    valid = type(value) is str
                    if valid and len(field) > 3 and field[3] is not None:
                        valid = len(value) <= field[3]
                else:
                    raise Refused('unsupported historical source column type: ' + kind)
                if valid and kind == 'jsonb' and name in ('raw', 'params', 'metadata', 'policy_snapshot'):
                    valid = type(value) is dict or (value is None and nullable == 'YES')
                if valid and table == 'xz_generation_tasks' and name == 'result_ids':
                    valid = type(value) is list and all(type(v) is str for v in value)
                if not valid:
                    raise Refused('historical snapshot column type invalid: ' + table + '.' + name)


def check_history_observation(item, release_sha):
    check_history_snapshot(item.get('before'))
    check_history_snapshot(item.get('after'))
    response = item.get('response', {})
    before, after = item.get('before'), item.get('after')
    if (not isinstance(before, dict) or set(before) != set(HISTORY_TABLES) or before != after or
            item.get('effects') != [] or
            sha(canonical(before).encode('utf-8')) != item.get('before_sha256') or
            item.get('before_sha256') != item.get('after_sha256') or
            type(response.get('history_protocol')) is not int or response.get('history_protocol') != HISTORY_VERSION or
            type(response.get('protocol')) is not int or response.get('protocol') != VERSION or
            response.get('release_sha') != release_sha or response.get('phase') != 'history' or
            response.get('role') != item.get('role') or response.get('owner') != item.get('owner') or
            set(response.get('operations', {})) != set(HISTORY_OPS) or
            any(v != 'read-only-rejected' for v in response['operations'].values()) or
            before.get('provider_execution_quarantine') != []):
        raise Refused('historical behavior evidence incomplete or changed')
    owner = item['owner']
    tasks = before['xz_generation_tasks']
    executions = before['provider_executions']
    if len(tasks) != 8 or len(executions) != 9:
        raise Refused('historical challenge fixture coverage incomplete')
    migrations = [r for r in before['schema_migrations'] if r['filename'] == '119-execution-generation-fencing.sql']
    if len(migrations) != 1:
        raise Refused('historical migration fixture evidence absent')
    for name in HISTORY_OPS:
        tid = owner + '-' + name
        es = [e for e in executions if e['task_id'] == tid]
        ts = [t for t in tasks if t['id'] == tid]
        if len(es) != 1 or es[0]['id'] <= 0 or es[0]['attempt'] != 1 or es[0]['capability'] != 'image' or es[0]['provider'] != 'fixture' or es[0]['provider_model'] != 'fixture' or es[0]['error_code'] is not None:
            raise Refused('historical execution fixture invalid')
        e = es[0]
        if e['request_fingerprint'] != history_request_fingerprint(tid, 'canary' in name):
            raise Refused('historical real request identity mismatch')
        if name == 'orphan':
            if ts or e['status'] != 'failed' or e['error_class'] != 'definitive_not_submitted' or any(e[k] is not None for k in ('task_execution_generation', 'provider_request_id', 'result_metadata', 'next_check_at')) or not (history_timestamp(e['created_at']) < history_timestamp(migrations[0]['applied_at']) and history_timestamp(e['updated_at']) < history_timestamp(migrations[0]['applied_at'])):
                raise Refused('historical orphan fixture invalid')
        elif (len(ts) != 1 or ts[0]['type'] != ('IMAGE_TO_IMAGE' if name.startswith('image') else 'TEXT_TO_IMAGE') or
                ts[0]['status'] != 'PROCESSING' or ts[0]['task_status'] != 'DISPATCHING' or ts[0]['execution_generation'] != 3 or
                e['status'] != 'succeeded' or e['task_execution_generation'] != 2 or e['error_class'] != 'provider_succeeded' or
                e['result_metadata'] != [{'URL': 'data:image/png;base64,aGVsbG8=', 'ContentType': 'image/png'}] or e['next_check_at'] is not None or
                type(ts[0]['params']) is not dict or type(ts[0]['raw']) is not dict or ts[0]['result_ids'] != [] or ts[0]['error'] is not None or
                ts[0]['user_id'] != owner or ts[0]['billing_status'] != 'RESERVED' or
                ts[0]['lease_until'] is None or ts[0]['last_heartbeat_at'] is None or ts[0]['updated_at'] != '2001-01-01T00:00:00Z' or
                history_timestamp(ts[0]['lease_until']) >= history_timestamp(e['created_at']) or
                history_timestamp(ts[0]['last_heartbeat_at']) >= history_timestamp(ts[0]['lease_until']) or
                ts[0]['raw'] != dict(id=tid, userId=owner, type=ts[0]['type'], status='PROCESSING', model='', prompt='', billingAccountType='PERSONAL', params=ts[0]['params'], updatedAt=ts[0]['updated_at']) or
                ts[0]['params'].get('provider') != 'fixture' or
                ts[0]['params'].get('generation_async_canary') != (True if 'canary' in name else None) or
                '_generation_dispatch_owner' in ts[0]['params'] or
                ts[0]['params'].get('generation_dispatch_mode', '') != ('' if name.endswith('legacy') else ('canary' if 'canary' in name else 'normal'))):
            raise Refused('historical success fixture invalid')
        if name.endswith('normal-legacy') and any(key in ts[0]['params'] for key in ('generation_async_canary', '_generation_fair_scheduled', '_generation_dispatch_owner')):
            raise Refused('historical empty-mode fixture markers present')
    workers = {str(t['worker_id']) for t in tasks if t['id'].endswith('normal-legacy')}
    if workers != {'None', ''}:
        raise Refused('historical owner-independent coverage incomplete')


def check_history_control(item, release_sha):
    check_history_snapshot(item.get('before'))
    check_history_snapshot(item.get('after'))
    before, after, response = item.get('before'), item.get('after'), item.get('response', {})
    if (not isinstance(before, dict) or set(before) != set(HISTORY_TABLES) or not isinstance(after, dict) or set(after) != set(HISTORY_TABLES) or
            sha(canonical(before).encode('utf-8')) != item.get('before_sha256') or sha(canonical(after).encode('utf-8')) != item.get('after_sha256') or
            type(response.get('protocol')) is not int or response.get('protocol') != VERSION or
            type(response.get('history_protocol')) is not int or response.get('history_protocol') != HISTORY_VERSION or
            response.get('phase') != 'history-control' or response.get('role') != item.get('role') or
            response.get('owner') != item.get('owner') or response.get('release_sha') != release_sha or
            response.get('operations') != {'normal-consumer': 'completed'}):
        raise Refused('funded consumer control evidence incomplete')
    task = item['owner'] + '-normal-control'
    account, reservation, lot = task + '-account', task + '-reservation', task + '-lot'
    def one(snapshot, table, key, identity):
        rows = [row for row in snapshot[table] if row[key] == identity]
        if len(rows) != 1:
            raise Refused('funded consumer control identity absent/ambiguous: ' + table)
        return rows[0]
    t0, t1 = one(before, 'xz_generation_tasks', 'id', task), one(after, 'xz_generation_tasks', 'id', task)
    a0, a1 = one(before, 'xz_point_accounts', 'id', account), one(after, 'xz_point_accounts', 'id', account)
    r0, r1 = one(before, 'xz_personal_point_reservations', 'id', reservation), one(after, 'xz_personal_point_reservations', 'id', reservation)
    l1 = one(after, 'xz_personal_point_lots', 'id', lot)
    execution = one(after, 'provider_executions', 'task_id', task)
    asset = one(after, 'xz_assets', 'task_id', task)
    file = one(after, 'xz_file_objects', 'business_id', task)
    inbox = one(after, 'consumer_inbox', 'event_id', task + '-event')
    captures = [row for row in after['xz_wallet_ledger'] if row['task_id'] == task and row['entry_type'] == 'CAPTURE']
    movements = [row for row in after['xz_personal_point_lot_movements'] if row['reservation_id'] == reservation]
    expected_params = {'generation_dispatch_mode': 'normal', '_generation_dispatch_owner': 'fixture-control-dispatch', 'billingReserved': True, 'billingReservationPointCost': 10, 'provider': task + '-channel'}
    if (t0['user_id'] != item['owner'] + '-control-user' or t0['type'] != 'TEXT_TO_IMAGE' or
            t0['model'] != 'gpt-image-2' or t0['prompt'] != 'funded consumer control' or t0['execution_generation'] != 1 or
            t0['worker_id'] != 'fixture-control-dispatch' or t0['params'] != expected_params or
            any(t0['raw'].get(tag) != t0[column] for tag, column in (('id', 'id'), ('userId', 'user_id'), ('type', 'type'), ('status', 'status'), ('model', 'model'), ('prompt', 'prompt'), ('params', 'params'))) or
            t0['raw'].get('personalPointAccountId') != account or t0['raw'].get('personalPointReservationId') != reservation or t0['raw'].get('billingEngine') != 'PERSONAL_LOT_V1' or
            t0['status'] != 'PROCESSING' or t0['task_status'] != 'DISPATCHING' or t0['point_cost'] != 10 or t0['result_ids'] != [] or
            t1['status'] not in ('COMPLETED', 'SUCCEEDED') or t1['task_status'] != 'SUCCEEDED' or t1['billing_status'] != 'CAPTURED' or
            t1['result_ids'] != [asset['id']] or t1['captured_points'] != 10 or t1['released_points'] != 0 or
            any(t1['raw'].get(tag) != t1[column] for tag, column in (('id', 'id'), ('userId', 'user_id'), ('type', 'type'), ('status', 'status'), ('model', 'model'), ('prompt', 'prompt'), ('params', 'params'), ('resultIds', 'result_ids'))) or
            a0['available'] != 100 or a0['frozen'] != 10 or a1['available'] != 100 or a1['frozen'] != 0 or
            r0['reserved_points'] != 10 or r0['captured_points'] != 0 or r1['reserved_points'] != 0 or r1['captured_points'] != 10 or r1['released_points'] != 0 or
            l1['available_points'] != 100 or l1['reserved_points'] != 0 or l1['consumed_points'] != 10 or
            execution['status'] != 'succeeded' or execution['attempt'] != 1 or execution['task_execution_generation'] != t1['execution_generation'] or
            file['status'] != 'ACTIVE' or file['visibility'] != 'PRIVATE' or file['file_size'] <= 0 or asset['url'] != '' or type(asset['metadata']) is not dict or asset['metadata'].get('storageFileId') != file['file_id'] or
            inbox['consumer_name'] != 'generation-image-normal-worker' or inbox['processed_at'] is None or inbox['result'] != 'completed' or type(inbox['metadata']) is not dict or inbox['metadata'].get('task_id') != task or
            len(captures) != 1 or captures[0]['points'] != 10 or len(movements) != 1 or movements[0]['movement_type'] != 'CAPTURE' or movements[0]['points'] != 10):
        raise Refused('funded consumer completion/storage/capture/inbox control absent')
    if any(row['task_id'] == task for row in before['provider_executions']) or any(row['event_id'] == task + '-event' for row in before['consumer_inbox']):
        raise Refused('funded consumer work was pre-completed')
    if sorted((effect.get('method'), effect.get('path')) for effect in item.get('effects', [])) != [('POST', '/consumer-provider'), ('POST', '/object')] or any(effect['bytes'] <= 0 for effect in item['effects']):
        raise Refused('funded consumer independent provider/storage effects absent')
    for table, key in [('xz_generation_tasks', 'id'), ('provider_executions', 'task_id')]:
        if [row for row in before[table] if row[key] != task] != [row for row in after[table] if row[key] != task]:
            raise Refused('normal consumer mutated historical rows')
    for table in ('provider_execution_quarantine', 'outbox_events', 'video_task_outbox', 'schema_migrations'):
        if before[table] != after[table]:
            raise Refused('normal consumer unexpected historical transport/schema mutation')


def verify_history(evidence, release_sha):
    history = evidence.get('history')
    if history is None:
        return False  # Frozen v1 remains strict: no history classification.
    if not isinstance(history, dict) or type(history.get('version')) is not int or history.get('version') != HISTORY_VERSION:
        raise Refused('historical protocol unknown')
    rows = history.get('observations', [])
    if len(rows) != len(ROLES) or {r.get('role') for r in rows} != set(ROLES):
        raise Refused('historical packaged role coverage incomplete')
    for row in rows:
        check_history_observation(row, release_sha)
    controls = history.get('controls', [])
    if len(controls) != len(ROLES) or {r.get('role') for r in controls} != set(ROLES):
        raise Refused('funded normal consumer packaged role coverage incomplete')
    for control in controls:
        check_history_control(control, release_sha)
    return True


def verify(evidence, ref, release_sha, policy):
    if not isinstance(evidence, dict) or type(evidence.get('version')) is not int or evidence.get('version') != VERSION or evidence.get('synthetic_nonofficial') is not False:
        raise Refused('runtime capability evidence missing/unknown/nonofficial')
    now = int(time.time())
    start, end = evidence.get('created_at_unix'), evidence.get('expires_at_unix')
    if type(start) is not int or type(end) is not int or start > now or end <= now or end - start != 3600:
        raise Refused('runtime capability stale: restage before stop')
    if not re.match(r'^[0-9a-f-]{36}$', evidence.get('nonce', '')) or evidence.get('policy') != policy:
        raise Refused('runtime capability nonce/policy mismatch')
    if evidence.get('identity') != image_identity(Docker(), ref, release_sha):
        raise Refused('runtime capability immutable identity mismatch')
    observations = evidence.get('observations', [])
    if len(observations) != 4 or {(x.get('role'), x.get('phase')) for x in observations} != {(r, p) for r in ROLES for p in ('allowed', 'blocked')}:
        raise Refused('runtime behavior coverage incomplete')
    for item in observations:
        response = item.get('response', {})
        if response.get('release_sha') != release_sha or set(response.get('operations', {})) != set(OPS) or response.get('nil_dependency_checks') != 6:
            raise Refused('runtime behavior response incomplete')
        if item['phase'] == 'blocked' and (item.get('effects') or item.get('before_sha256') != item.get('after_sha256') or any(x != 'EXECUTION_QUARANTINED_READONLY' for x in response['operations'].values())):
            raise Refused('runtime forbidden effects present')
        if item['phase'] == 'allowed' and (item.get('before_sha256') == item.get('after_sha256') or sorted(x['path'] for x in item.get('effects', [])) not in (['/object', '/provider', '/provider-video'], ['/object', '/provider', '/provider-query', '/provider-video'])):
            raise Refused('runtime normal control missing')
        if sha(canonical(item.get('before')).encode('utf-8')) != item.get('before_sha256') or sha(canonical(item.get('after')).encode('utf-8')) != item.get('after_sha256'):
            raise Refused('runtime independent snapshot hash mismatch')
        if item['phase'] == 'allowed':
            check_allowed(item['before'], item['after'], item['owner'], item['effects'])
            detail = response.get('details', {}).get('video-persistence', {})
            if detail.get('positive_mode') != 'existing_durable_reuse' or detail.get('file_id') != item['owner'] + '-video-file' or detail.get('task_id') != item['owner'] + '-video-persistence' or detail.get('new_files') != 0:
                raise Refused('video reuse branch evidence missing')
    unavailable = evidence.get('unavailable_observations', [])
    if len(unavailable) != 6 or {(x.get('role'), x.get('mode')) for x in unavailable} != {(r, p) for r in ROLES for p in ('missing', 'denied', 'timeout')}:
        raise Refused('required dependency coverage incomplete')
    for item in unavailable:
        response = item.get('response', {})
        if item.get('effects') or item.get('before_sha256') != item.get('after_sha256') or set(response.get('operations', {})) != set(OPS) or any(x != 'quarantine barrier unavailable' for x in response.get('operations', {}).values()) or response.get('release_sha') != release_sha:
            raise Refused('required dependency rejection invalid')
    if verify_history(evidence, release_sha):
        for item in observations:
            if item['phase'] == 'allowed' and (item['response'].get('details', {}).get('provider-query') != 'observed-get-control' or [x for x in item['effects'] if x.get('method') == 'GET' and x['path'] == '/provider-query'] != [dict(method='GET', path='/provider-query', bytes=0)]):
                raise Refused('independent GET query control absent')
    return evidence['identity']['local_image_id']


def authenticated_proof(path):
    with open(path, 'r', encoding='utf-8') as stream:
        proof = json.load(stream)
    key = os.environ.get('RELEASE_TRUST_SECRET', '')
    key_path = os.environ.get('RELEASE_TRUST_KEY_FILE', '')
    if not key:
        for candidate in (key_path, '/etc/zhiqiyun/release-trust.key', '.prestage/release-trust.key'):
            if candidate and os.path.isfile(candidate):
                with open(candidate, 'r', encoding='utf-8') as stream:
                    key = stream.read().strip()
                break
    payload = {k: v for k, v in proof.items() if k != 'signature'}
    if not key or not hmac.compare_digest(proof.get('signature', ''), hmac.new(key.strip().encode('utf-8'), canonical(payload).encode('utf-8'), hashlib.sha256).hexdigest()):
        raise Refused('authenticated capability Proof required')
    expiration = re.match(r'^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,6}))?(?:Z|\+00:00)$', proof.get('expires_at', ''))
    if not expiration:
        raise Refused('authenticated Proof expiry missing/invalid')
    expires = datetime.datetime.strptime(expiration.group(1), '%Y-%m-%dT%H:%M:%S').replace(tzinfo=datetime.timezone.utc, microsecond=int((expiration.group(2) or '').ljust(6, '0')))
    if expires <= datetime.datetime.now(datetime.timezone.utc):
        raise Refused('authenticated Proof expired: restage before stop')
    scripts = proof.get('deploy_scripts_hash') or {}
    for name in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh', 'ops/verify-prestage-proof.sh', 'ops/verify-image-quarantine-capability.py', 'ops/verify-release-runtime.py', 'ops/first-upgrade-cold.py', 'ops/auto_monitor_killswitch.py'):
        with open(os.path.join(ROOT, name), 'rb') as stream:
            if scripts.get(name) != sha(stream.read()):
                raise Refused('protected capability helper bytes changed')
    return proof


def rendered_policy(docker, compose, env_file, ref):
    docker.env['XIANZHI_IMAGE_REFERENCE'] = ref
    model = json.loads(docker.text(['compose', '-f', compose, '--env-file', env_file, 'config', '--format', 'json']))
    return model, runtime_policy(model)


def verify_pid1_policy(docker, cid, obj, policy, expected_env):
    if (obj['Config'].get('Entrypoint') or []) != policy['entrypoint']:
        raise Refused('actual running entrypoint policy mismatch')
    command = docker.run(['exec', cid, '/bin/cat', '/proc/1/cmdline']).stdout
    environment = docker.run(['exec', cid, '/bin/cat', '/proc/1/environ']).stdout
    if not command or not environment or not command.endswith(b'\0') or not environment.endswith(b'\0'):
        raise Refused('actual PID1 policy unreadable or malformed')
    if [x.decode('utf-8') for x in command[:-1].split(b'\0')] != policy['command']:
        raise Refused('actual PID1 command policy mismatch')
    actual = {}
    for item in environment[:-1].split(b'\0'):
        key, value = item.decode('utf-8').split('=', 1)
        if not key or key in actual:
            raise Refused('actual PID1 environment malformed')
        actual[key] = value
    expected = dict(expected_env)
    # Docker adds these defaults at execve even when absent from Config.Env.
    expected.setdefault('HOSTNAME', obj['Config']['Hostname'])
    if 'HOME' not in expected:
        user = (obj['Config'].get('User') or '0').split(':', 1)[0]
        passwd = docker.text(['exec', cid, '/bin/cat', '/etc/passwd'])
        homes = [line.split(':')[5] for line in passwd.splitlines() if len(line.split(':')) == 7 and (line.split(':')[0] == user or line.split(':')[2] == user)]
        if len(homes) != 1:
            raise Refused('actual PID1 default home policy unavailable')
        expected['HOME'] = homes[0]
    if obj['Config'].get('Tty'):
        expected.setdefault('TERM', 'xterm')
    if actual != expected:
        raise Refused('actual PID1 environment policy mismatch')


def verify_running_processes(docker, model, evidence, ref):
    """Internal process-only observation; no SQL and no CLI skip gate."""
    identity = evidence['identity']
    image = docker.inspect('image', identity['local_image_id'])
    for service, policy in evidence['policy']['roles'].items():
        cid = docker.text(['compose', '-f', model['_compose_file'], '--env-file', model['_env_file'], 'ps', '-q', service])
        if not cid or '\n' in cid:
            raise Refused('running process singleton missing')
        obj = docker.inspect('container', cid)
        cfg = model['services'][service]
        expected_env = dict(x.split('=', 1) for x in image['Config'].get('Env', []))
        expected_env.update({k: str(v) for k, v in (cfg.get('environment') or {}).items()})
        actual_env = dict(x.split('=', 1) for x in obj['Config'].get('Env', []))
        if obj['Image'] != identity['local_image_id'] or obj['Config']['Image'] != ref or not obj['State']['Running'] or obj['Config']['Cmd'] != policy['command'] or (obj['Config'].get('Entrypoint') or []) != policy['entrypoint'] or actual_env != expected_env or obj['Config'].get('User', '') != (policy['user'] or image['Config'].get('User', '')):
            raise Refused('actual running image/process/env policy mismatch')
        verify_pid1_policy(docker, cid, obj, policy, expected_env)
        expected_mounts = {}
        for mount in cfg.get('volumes') or []:
            if not isinstance(mount, dict):
                raise Refused('normalized mount policy required')
            source = mount['source']
            if mount['type'] == 'volume':
                source = model.get('volumes', {}).get(source, {}).get('name', source)
            expected_mounts[mount['target']] = (mount['type'], source, not mount.get('read_only', False))
        actual_mounts = {m['Destination']: (m['Type'], m.get('Name') if m['Type'] == 'volume' else m['Source'], m['RW']) for m in obj.get('Mounts', [])}
        if actual_mounts != expected_mounts:
            raise Refused('actual running mount policy mismatch')
        binary_hash = docker.text(['exec', cid, '/usr/bin/sha256sum', '/proc/1/exe']).split()[0]
        if binary_hash != identity['binary_sha256'][policy['binary']]:
            raise Refused('actual PID1 binary mismatch')
    postgres = docker.text(['compose', '-f', model['_compose_file'], '--env-file', model['_env_file'], 'ps', '-q', 'postgres'])
    if not postgres or '\n' in postgres or not docker.inspect('container', postgres)['State']['Running']:
        raise Refused('actual barrier database singleton unavailable')
    if image_identity(docker, ref, identity['release_sha']) != identity:
        raise Refused('image substituted during running observation')


def verify_running(docker, model, evidence, ref):
    verify_running_processes(docker, model, evidence, ref)
    # Complementary catalog health on the actual Compose singleton, with the
    # original READ ONLY/statement_timeout and credentials-in-container policy.
    # Never inherit the legacy test-container override in this production gate.
    runtime_path = os.path.join(ROOT, 'ops/verify-release-runtime.py')
    import types
    runtime = types.ModuleType('runtime_health')
    runtime.__file__ = runtime_path
    with open(runtime_path, 'rb') as stream:
        exec(compile(stream.read(), runtime_path, 'exec'), runtime.__dict__)
    inherited_test = os.environ.pop('XIANZHI_TEST_CONTAINER', None)
    try:
        runtime.verify_quarantine_barrier_read_only(model['_compose_file'], model['_env_file'], execute=lambda args: docker.text(args[1:]))
    finally:
        if inherited_test is not None:
            os.environ['XIANZHI_TEST_CONTAINER'] = inherited_test
    if image_identity(docker, ref, evidence['identity']['release_sha']) != evidence['identity']:
        raise Refused('image substituted during running observation')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--image', required=True)
    parser.add_argument('--release-sha', required=True)
    parser.add_argument('--output')
    parser.add_argument('--synthetic-nonofficial', action='store_true')
    parser.add_argument('--verify-proof')
    parser.add_argument('--rollback', action='store_true')
    parser.add_argument('--receipt-sha256')
    parser.add_argument('--compose-file', default='compose.prod.yml')
    parser.add_argument('--env-file', default='.env.production')
    parser.add_argument('--post-start', action='store_true')
    args = parser.parse_args()
    if args.verify_proof:
        if args.synthetic_nonofficial or args.output:
            raise Refused('synthetic mode cannot validate authenticated production Proof')
        proof = authenticated_proof(args.verify_proof)
        docker = Docker()
        model, policy = rendered_policy(docker, args.compose_file, args.env_file, args.image)
        for path, field in ((args.compose_file, 'compose_hash'), (args.env_file, 'env_hash')):
            with open(path, 'rb') as stream:
                if sha(stream.read()) != proof.get(field):
                    raise Refused('authenticated config bytes changed before stop')
        bound_model = json.loads(canonical(model))
        if args.rollback:
            for cfg in bound_model.get('services', {}).values():
                if cfg.get('image') == args.image:
                    cfg['image'] = proof.get('image_reference')
        if sha(canonical(bound_model).encode('utf-8')) != proof.get('bound_config_hash'):
            raise Refused('authenticated full runtime configuration changed')
        evidence = proof.get('rollback_runtime_capability' if args.rollback else 'runtime_capability')
        if args.rollback and (not args.receipt_sha256 or proof.get('rollback_receipt_hash') != args.receipt_sha256):
            raise Refused('frozen original rollback Receipt not bound to Proof')
        verified_id = verify(evidence, args.image, args.release_sha, policy)
        if args.post_start:
            model['_compose_file'], model['_env_file'] = args.compose_file, args.env_file
            verify_running(docker, model, evidence, args.image)
        print(verified_id)
        return
    if not args.output or args.post_start or args.rollback:
        raise Refused('challenge output required; malformed arguments refused')
    evidence = attest(args.image, args.release_sha, {}, args.synthetic_nonofficial)
    with open(args.output, 'w', encoding='utf-8') as stream:
        json.dump(evidence, stream, indent=2, sort_keys=True)
        stream.write('\n')

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        sys.stderr.write('RUNTIME_CAPABILITY_REFUSED: ' + str(error) + '\n')
        sys.exit(1)
