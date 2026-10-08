#!/usr/bin/env python3
"""Actual release shell/helper paths with owned stub transport; no production.

Actual Docker/PG coverage: tests/first-upgrade-cold-docker-test.py.
Missing Docker is a failure, never a runtime PASS. Python 3.6 compatible.
"""
import copy
import hashlib
import hmac
import json
import os
from pathlib import Path
import shutil
import shlex
import signal
import subprocess
import sys
import tempfile
import time
import types
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[1]
BASH = 'C:/Program Files/Git/bin/bash.exe' if os.name == 'nt' else 'bash'
BASELINE = 'b45e72613dff3863d9eacc13cc99e4d320cf284f'
TARGET = 'e7df2d6f07416f7c08fc39810e81327fa8003ed0'
SECRET = 'isolated-cold-test-only'
REF = 'ghcr.io/lmxchyy/zhiqiyun-ai@sha256:' + 'a' * 64
OLDREF = 'ghcr.io/lmxchyy/zhiqiyun-ai@sha256:' + 'b' * 64
IMAGE = 'sha256:' + 'c' * 64
OLDIMAGE = 'sha256:' + 'd' * 64


def module(path):
    result = types.ModuleType(path.stem.replace('-', '_'))
    result.__file__ = str(path)
    exec(compile(path.read_bytes(), str(path), 'exec'), result.__dict__)
    return result


def save(path, value):
    path.write_text(json.dumps(value, sort_keys=True), encoding='utf-8')


# This transport only handles the disposable state file. Unexpected operations
# fail closed. The actual production Python helpers and release shell are copied
# unchanged, including their HMAC, source pins, policy and capability verifier.
DOCKER_STUB = r'''import json,os,sys,time,io
sys.stdout=io.TextIOWrapper(sys.stdout.buffer,encoding='utf-8',newline='\n')
from pathlib import Path
path=Path(__file__).resolve().parent.parent/'docker-state.json'
s=json.loads(path.read_text()); a=sys.argv[1:]
if a[:1]==['--host']: a=a[2:]
s['commands'].append(a)
path.write_text(json.dumps(s))
def done(value='', code=0):
 path.write_text(json.dumps(s)); print(value) if value != '' else None; sys.exit(code)
def object_(cid):
 if cid not in s['containers']: done(code=1)
 return s['containers'][cid]
def inspect(cid,fmt=None):
 o=object_(cid)
 if s.get('fault')=='inspect' and o['State']['Running'] is False: done(code=1)
 if fmt:
  fields={'{{.Config.Image}}':o['Config']['Image'],'{{.Image}}':o['Image'],'{{.State.Running}}':str(o['State']['Running']).lower(),'{{.State.Status}}':o['State']['Status'],'{{.State.ExitCode}}':str(o['State']['ExitCode']),'{{.State.Health.Status}}':'healthy'}
  done(fields.get(fmt,''))
 done(json.dumps([o]))
if a[:2]==['compose','version']: done('fixture v2')
if a[:2]==['image','prune']: done()
if a[:2]==['image','inspect']:
 identity=a[2]; o=s['images'].get(identity)
 if not o: done(code=1)
 if '--format' in a:
  fmt=a[a.index('--format')+1]; done(o['Id'] if fmt=='{{.Id}}' else '\n'.join(o['RepoDigests']))
 done(json.dumps([o]))
if a[:1]==['run'] and '/usr/bin/sha256sum' in a:
 done('\n'.join('e'*64+'  '+p for p in ['/app/xianzhi-api','/app/generation-worker','/app/smartvideo-worker','/app/video-backfill']))
if a[:2]==['container','inspect']: inspect(a[2])
if a[:1]==['inspect']:
 inspect(a[-1],a[a.index('--format')+1] if '--format' in a else None)
if a[:1]==['ps']: done('\n'.join(s['containers']))
if a[:1]==['update']:
 cid=a[-1]
 if s.get('fault')=='update' and object_(cid)['Config']['Labels']['com.docker.compose.service']=='generation-worker': done(code=1)
 object_(cid)['HostConfig']['RestartPolicy']['Name']='no'; done(cid)
if a[:1]==['stop']:
 cid=a[-1]; o=object_(cid)
 if s.get('fault')=='stop' and o['Config']['Labels']['com.docker.compose.service']=='generation-worker': done(code=1)
 o['State'].update(Running=False,Status='exited',Restarting=False,ExitCode=0); done(cid)
if a[:1]==['exec']:
 cid=a[1]; o=object_(cid)
 if a[-1]=='/proc/1/cmdline': sys.stdout.buffer.write(b'\0'.join(x.encode() for x in (['/bin/false'] if s.get('fault')=='pid1' else o['Config']['Cmd']))+b'\0'); sys.exit(0)
 if a[-1]=='/proc/1/environ': sys.stdout.buffer.write(b'\0'.join(x.encode() for x in o['Config']['Env']+['HOSTNAME=fixture','HOME=/root'])+b'\0'); sys.exit(0)
 if a[-1]=='/etc/passwd': done('root:x:0:0:root:/root:/bin/sh')
 if '/usr/bin/sha256sum' in a:
  if s.get('fault')=='lock-drift': Path('.prestage/release.lock/owner_token').write_text('other')
  done('e'*64+'  /proc/1/exe')
 done(code=1)
if a[:1]==['compose']:
 config=a[a.index('-f')+1]; model=json.loads(Path(config).read_text())
 pos=a.index('--env-file')+2; cmd=a[pos:]
 if cmd[:1]==['config']:
  model['services']['xianzhi-ai']['image']=os.environ.get('XIANZHI_IMAGE_REFERENCE',model['services']['xianzhi-ai']['image'])
  for name in ('generation-worker','smartvideo-worker'):
   model['services'][name]['image']=model['services']['xianzhi-ai']['image']
  done(json.dumps(model))
 if cmd[:1]==['ps']:
  names=[x for x in cmd[1:] if x in model['services']]
  cids=[cid for cid,o in s['containers'].items() if o['Config']['Labels'].get('com.docker.compose.project')==model['name'] and o['Config']['Labels'].get('com.docker.compose.service') in names and ('-a' in cmd or o['State']['Running'])]
  done('\n'.join(cids))
 if cmd[:1]==['stop']:
  for o in s['containers'].values():
   if (o['Config']['Labels'].get('com.docker.compose.project') == model['name'] and
       o['Config']['Labels'].get('com.docker.compose.service') in cmd): o['State'].update(Running=False,Status='exited',ExitCode=0)
  done()
 if cmd[:1] in (['rm'],['logs']): done()
 if cmd[:1]==['exec']:
  if s.get('fault')=='health' and 'curl' in cmd: done(code=1)
  if 'curl' in cmd: done('{"status":"ok","ready":true}')
  done(code=1)
 if cmd[:1]==['up']:
  names=[x for x in cmd[1:] if x in model['services']]
  if not names: names=list(model['services'])
  for name in names:
   cfg=model['services'][name]; cid='new-'+name
   if name=='postgres': continue
   s['containers']={k:v for k,v in s['containers'].items() if not (
    v['Config']['Labels'].get('com.docker.compose.project')==model['name'] and
    v['Config']['Labels'].get('com.docker.compose.service')==name)}
   cmdline=cfg.get('command') or ['/app/xianzhi-api']
   env=dict(cfg.get('environment') or {})
   s['containers'][cid]={'Id':cid,'Image':s['images'][cfg['image']]['Id'],'Config':{'Image':cfg['image'],'Cmd':cmdline,'Entrypoint':[],'Env':[k+'='+str(v) for k,v in env.items()],'Hostname':'fixture','User':'','Labels':{'com.docker.compose.project':model['name'],'com.docker.compose.service':name}},'HostConfig':{'RestartPolicy':{'Name':cfg.get('restart','always')}},'State':{'Running':name!='migrate','Status':'exited' if name=='migrate' else 'running','ExitCode':1 if s.get('fault')=='migration' and name=='migrate' else 0,'Restarting':False,'Health':{'Status':'healthy'}},'Mounts':[]}
  path.write_text(json.dumps(s))
  if s.get('fault') in ('signal','sigkill') and 'migrate' in names:
   Path('signal-ready').write_text('ready'); time.sleep(3)
  if s.get('fault')=='ancillary' and 'xianzhi-ai' in names:
   s['containers']['new-proxy']['State'].update(Running=False,Status='exited')
  if s.get('fault')=='start' and 'xianzhi-ai' in names: done(code=1)
  done()
 done(code=1)
done(code=1)
'''


def behavior(capability, identity, policy):
    before = {table: [] for table in capability.TABLES}
    owner = 'test-owned'
    for op in ('lease', 'video-persistence'):
        if op == 'lease':
            before['xz_generation_tasks'].append({'id': owner + '-lease', 'lease_until': 'a'})
        else:
            before['xz_file_objects'].append({'file_id': owner + '-video-file', 'business_id': owner + '-video-persistence'})
    after = copy.deepcopy(before)
    for op in capability.OPS:
        after['xz_generation_tasks'].append({'id': owner + '-' + op, 'status': 'PROCESSING', 'task_status': 'RUNNING', 'worker_id': 'fixture-worker', 'execution_generation': 2, 'lease_until': 'z', 'last_heartbeat_at': 'z'})
    after['xz_generation_tasks'] = [r for r in after['xz_generation_tasks'] if r.get('lease_until') != 'a']
    for op, state in (('provider', 'succeeded'), ('video-provider', 'succeeded'), ('execution-claim', 'submitting'), ('transition', 'submitting'), ('result', 'succeeded'), ('create', 'prepared'), ('correlation', 'prepared')):
        after['provider_executions'].append({'id': op, 'task_id': owner + '-' + op, 'status': state, 'attempt': 2, 'result_metadata': {'fixture': True}})
    after['provider_execution_correlations'] = [{'execution_id': 'correlation' if i == 0 else 'provider', 'kind': 'submit'} for i in range(4)]
    after['xz_assets'] = [{'task_id': owner + '-asset'}]
    after['xz_file_objects'].append({'business_id': owner + '-persistence', 'status': 'ACTIVE'})
    for op, available, frozen in (('reserve', 90, 10), ('capture', 100, 5), ('release', 105, 5)):
        after['xz_point_accounts'].append({'id': owner + '-' + op, 'available': available, 'frozen': frozen})
    for table in capability.TABLES:
        if not after[table] and table != 'xz_storage_configs':
            after[table] = [{'fixture': i} for i in range(3)]
    effects = [{'path': p, 'bytes': 1} for p in ('/object', '/provider', '/provider-video')]
    capability.check_allowed(before, after, owner, effects)
    observations = []
    for role in capability.ROLES:
        for phase in ('allowed', 'blocked'):
            response = {'release_sha': identity['release_sha'], 'nil_dependency_checks': 6,
                        'operations': {op: 'EXECUTION_QUARANTINED_READONLY' if phase == 'blocked' else 'ok' for op in capability.OPS},
                        'details': {'video-persistence': {'positive_mode': 'existing_durable_reuse', 'file_id': owner + '-video-file', 'task_id': owner + '-video-persistence', 'new_files': 0}}}
            b, a = before, after if phase == 'allowed' else before
            observations.append({'role': role, 'phase': phase, 'owner': owner, 'before': b, 'after': a,
                                 'before_sha256': capability.sha(capability.canonical(b).encode()), 'after_sha256': capability.sha(capability.canonical(a).encode()),
                                 'effects': effects if phase == 'allowed' else [], 'response': response})
    unavailable = [{'role': r, 'mode': m, 'before_sha256': '1', 'after_sha256': '1', 'effects': [],
                    'response': {'release_sha': identity['release_sha'], 'operations': {op: 'quarantine barrier unavailable' for op in capability.OPS}}}
                   for r in capability.ROLES for m in ('missing', 'denied', 'timeout')]
    now = int(time.time())
    return {'version': 1, 'synthetic_nonofficial': False, 'nonce': str(uuid.uuid4()), 'created_at_unix': now,
            'expires_at_unix': now + 3600, 'identity': identity, 'policy': policy,
            'observations': observations, 'unavailable_observations': unavailable}


class ShellTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='owned-cold-shell-')
        self.root = Path(self.temp.name)
        for name in ('ops', 'ops/quarantine-approval', 'database/migrations', 'bin', '.prestage'):
            (self.root / name).mkdir(parents=True, exist_ok=True)
        helpers = ['deploy.sh', 'rollback.sh'] + ['ops/' + p.name for p in (ROOT / 'ops').iterdir() if p.is_file()]
        helpers += ['ops/quarantine-approval/registry.json']
        for name in helpers:
            shutil.copyfile(str(ROOT / name), str(self.root / name))
            (self.root / name).chmod(0o755)
        # Unrelated runtime/drain gates are owned no-op fixtures; cold, proof,
        # image/source identity and actual capability verification remain REAL.
        (self.root / 'ops/verify-release-runtime.py').write_text(
            'import sys\ndef verify_quarantine_barrier_read_only(compose, env, execute):\n'
            '    with open("catalog-observed", "a") as stream: stream.write("read-only barrier\\n")\n'
            'if __name__ == "__main__": sys.exit(0)\n')
        (self.root / 'ops/verify-safe-drain.py').write_text('import sys\nsys.exit(0)\n')
        (self.root / 'ops/disk-guard.sh').write_text('#!/bin/sh\nexit 0\n')
        (self.root / 'database/migrations/001-fixture.sql').write_text('SELECT 1;\n')
        (self.root / '.env.production').write_text('fixture=owned\n')
        services = {}
        for role, binary in (('xianzhi-ai', 'xianzhi-api'), ('generation-worker', 'generation-worker'), ('smartvideo-worker', 'smartvideo-worker'), ('migrate', 'migrate'), ('video-backfill', 'video-backfill')):
            services[role] = {'image': REF, 'command': ['/app/' + binary], 'environment': {'XIANZHI_ENV': 'production', 'DATABASE_URL': 'owned-fixture', 'VIDEO_STORAGE_PERSISTENCE_ENABLED': 'true'}, 'restart': 'always'}
        # Match production: migrations run in a separate PostgreSQL image,
        # not in the digest-pinned application image.
        services['migrate']['image'] = 'postgres:16-alpine'
        services['proxy'] = {'image': 'fixture-proxy', 'command': ['/proxy'], 'restart': 'always'}
        services['postgres'] = {'image': 'fixture-postgres'}
        self.model = {'name': 'owned-cold-project', 'services': services}
        save(self.root / 'compose.prod.yml', self.model)
        images = {}
        for ref, image in ((REF, IMAGE), (OLDREF, OLDIMAGE)):
            obj = {'Id': image, 'RepoDigests': [ref], 'Os': 'linux', 'Architecture': 'amd64', 'Config': {'Entrypoint': [], 'Env': []}}
            images[ref] = obj
            images[image] = obj
        images['fixture-proxy'] = {'Id': 'proxy-image', 'Config': {'Env': [], 'Entrypoint': []}}
        images['postgres:16-alpine'] = {'Id': 'migration-runner-image', 'Config': {'Env': [], 'Entrypoint': []}}
        containers = {}
        for role in services:
            cid = 'old-' + role
            containers[cid] = {'Id': cid, 'Image': OLDIMAGE if role != 'postgres' else 'infra',
                               'Config': {'Image': OLDREF if role != 'postgres' else 'fixture-postgres', 'Labels': {'com.docker.compose.project': self.model['name'], 'com.docker.compose.service': role}},
                               'HostConfig': {'RestartPolicy': {'Name': 'always'}},
                               'State': {'Running': True, 'Status': 'running', 'ExitCode': 0, 'Restarting': False}}
        containers['unrelated'] = {'Id': 'unrelated', 'Image': 'unrelated-image', 'Config': {'Labels': {'com.docker.compose.project': 'other', 'com.docker.compose.service': 'xianzhi-ai'}}, 'HostConfig': {'RestartPolicy': {'Name': 'always'}}, 'State': {'Running': True}}
        save(self.root / 'docker-state.json', {'images': images, 'containers': containers, 'commands': [], 'fault': ''})
        python = shutil.which('python3') or sys.executable
        (self.root / 'bin/docker').write_text('#!/bin/sh\nexec "' + python.replace('\\', '/') + '" "$(dirname "$0")/docker-stub.py" "$@"\n')
        (self.root / 'bin/docker-stub.py').write_text(DOCKER_STUB)
        git = shutil.which('git')
        # Real trusted source reads, isolated HEAD/clean/ancestry transport.
        (self.root / 'bin/git').write_text('#!/bin/sh\ncase "$*" in\n "status "*) exit 0;;\n "rev-parse HEAD") echo ' + TARGET + '; exit 0;;\n "merge-base "*) exit 0;;\n "rev-parse --verify "*) echo ' + BASELINE + '; exit 0;;\n "rev-parse HEAD:database/migrations") exit 1;;\nesac\nexec "' + git.replace('\\', '/') + '" -C "' + str(ROOT).replace('\\', '/') + '" "$@"\n')
        if os.name == 'nt':
            # Windows Python uses PATHEXT; Bash uses extensionless shell launchers.
            (self.root / 'bin/docker.cmd').write_text('@echo off\n"' + python + '" "%~dp0docker-stub.py" %*\n')
            (self.root / 'bin/git-stub.py').write_text('import subprocess,sys\na=[x.replace("{tree}","^{tree}") if "{tree}" in x and "^" not in x else x for x in sys.argv[1:]]\nif a[:2]==["rev-parse","--verify"]: print(' + repr(BASELINE) + ');sys.exit(0)\nsys.exit(subprocess.call([' + repr(git) + ',"-C",' + repr(str(ROOT)) + ']+a))\n')
            (self.root / 'bin/git.cmd').write_text('@echo off\n"' + python + '" "%~dp0git-stub.py" %*\n')
        for path in (self.root / 'bin').iterdir():
            path.write_bytes(path.read_bytes().replace(b'\r\n', b'\n'))
            path.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.root / 'bin') + os.pathsep + os.environ['PATH'], RELEASE_TRUST_SECRET=SECRET,
                        PYTHONDONTWRITEBYTECODE='1', DEPLOY_MIN_FREE_BYTES='0', HEALTH_CHECK_TIMEOUT_SECONDS='1', MIGRATE_TIMEOUT_SECONDS='1')
        self.old_env = dict(os.environ)
        os.environ.update(self.env)
        self.cold = module(self.root / 'ops/first-upgrade-cold.py')
        self.capability = self.cold.capability
        save(self.root / 'rollback-manifest.json', {'git_sha': BASELINE, 'image_reference': OLDREF})
        self.receipt = {'receipt_version': '1.0', 'target_git_sha': TARGET, 'previous_git_sha': BASELINE, 'previous_image_reference': OLDREF,
                        'previous_image_id': OLDIMAGE, 'rollback_manifest_path': 'rollback-manifest.json', 'rollback_manifest_sha256': self.cold.file_hash(str(self.root / 'rollback-manifest.json')),
                        'rollback_provenance': {'repository': 'lmxchyy/zhiqiyun-ai', 'workflow': '.github/workflows/immutable-image-release.yml', 'head_sha': BASELINE, 'run_id': 1, 'artifact_id': 2}}
        save(self.root / 'receipt.json', self.receipt)
        save(self.root / 'manifest.json', {'git_sha': TARGET, 'image_reference': REF})
        self.cwd = os.getcwd()
        os.chdir(str(self.root))
        self.addCleanup(os.chdir, self.cwd)
        docker = self.capability.Docker()
        self.policy = self.cold.create_policy('receipt.json', self.cold.file_hash('receipt.json'), self.model, '.prestage/cold-compose.json', docker)
        identity = self.capability.image_identity(docker, REF, TARGET)
        runtime_policy = self.capability.runtime_policy(self.model)
        self.proof = {'git_sha': TARGET, 'expires_at': '2099-01-01T00:00:00Z', 'proof_nonce': str(uuid.uuid4()),
                      'image_reference': REF, 'local_image_id': IMAGE, 'runtime_capability': behavior(self.capability, identity, runtime_policy),
                      'rollback_runtime_capability': None, 'cold_recovery_policy': self.policy,
                      'rollback_receipt_path': 'receipt.json', 'rollback_receipt_hash': self.cold.file_hash('receipt.json'),
                      'compose_hash': self.cold.file_hash('compose.prod.yml'), 'env_hash': self.cold.file_hash('.env.production'),
                      'bound_config_hash': self.capability.sha(self.capability.canonical(self.model).encode()), 'config_binding_version': 2,
                      'manifest_path': 'manifest.json', 'manifest_sha256': self.cold.file_hash('manifest.json'),
                      'github_provenance': {'repository': 'lmxchyy/zhiqiyun-ai', 'workflow': '.github/workflows/immutable-image-release.yml', 'run_id': 3, 'artifact_id': 4, 'head_sha': TARGET},
                      'deploy_scripts_hash': {name: self.cold.file_hash(name) for name in helpers}}
        self.proof['github_provenance']['manifest_bytes_sha256'] = self.proof['manifest_sha256']
        self.sign()

    def tearDown(self):
        os.chdir(self.cwd)
        os.environ.clear()
        os.environ.update(self.old_env)
        self.temp.cleanup()

    def sign(self):
        self.proof.pop('signature', None)
        self.proof['signature'] = hmac.new(SECRET.encode(), self.capability.canonical(self.proof).encode(), hashlib.sha256).hexdigest()
        save(self.root / 'proof.json', self.proof)

    def shell(self, script, *args):
        fixture = self.root.as_posix()
        if os.name == 'nt':
            fixture = '/' + fixture[0].lower() + fixture[2:]
        command = 'export PATH=' + shlex.quote(fixture + '/bin') + ':"$PATH"; exec bash ' + shlex.quote(fixture + '/' + script) + ' ' + ' '.join(shlex.quote(a) for a in args)
        process = subprocess.Popen([BASH, '-c', command], cwd=str(self.root), env=self.env,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   universal_newlines=True, encoding='utf-8', errors='replace')
        try:
            stdout, stderr = process.communicate(timeout=600 if os.name == 'nt' else 70)
        except subprocess.TimeoutExpired:
            # Kill only this owned fixture tree; a Windows Bash wrapper alone
            # can leave descendants holding captured pipes indefinitely.
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            else:
                process.kill()
            process.communicate(timeout=20)
            raise
        return subprocess.CompletedProcess(process.args, process.returncode, stdout, stderr)

    def deploy(self, opt=True):
        return self.shell('deploy.sh', '--prestaged', TARGET, '--proof', 'proof.json', *(['--first-upgrade-cold'] if opt else []))

    def state(self):
        return json.loads((self.root / 'docker-state.json').read_text())

    def fault(self, kind):
        state = self.state()
        state['fault'] = kind
        save(self.root / 'docker-state.json', state)

    def stopped(self):
        state = self.state()
        for cid, obj in state['containers'].items():
            if cid not in ('unrelated', 'old-postgres'):
                self.assertFalse(obj['State']['Running'], cid)
                self.assertEqual(obj['HostConfig']['RestartPolicy']['Name'], 'no', cid)
        self.assertTrue(state['containers']['unrelated']['State']['Running'])
        self.assertEqual(state['containers']['unrelated']['HostConfig']['RestartPolicy']['Name'], 'always')
        self.assertTrue(state['containers']['old-postgres']['State']['Running'])
        self.assertFalse(any(cmd[0] == 'exec' and 'psql' in cmd for cmd in state['commands']))

    def test_default_disabled_and_offline_never_cold(self):
        result = self.deploy(False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('COLD_POLICY_OPT_IN_MISMATCH', result.stderr)
        result = self.shell('rollback.sh', '--offline', '--receipt', 'receipt.json', '--capability-proof', 'proof.json')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())
        self.assertFalse(any(cmd[0] in ('stop', 'update') for cmd in self.state()['commands']))

    def test_explicit_cold_rollback_repeat_and_unauthorized_reentry(self):
        for unused in range(2):
            result = self.shell('rollback.sh', '--first-upgrade-cold', '--receipt', 'receipt.json', '--capability-proof', 'proof.json')
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('RECOVERY_REQUIRED', result.stdout + result.stderr)
            self.stopped()
        before = self.state()['commands'][:]
        self.assertNotEqual(self.deploy().returncode, 0)
        self.assertNotEqual(self.shell('ops/prestage-release.sh', TARGET).returncode, 0)
        self.assertEqual(before, self.state()['commands'])
        monitor = module(self.root / 'ops/auto_monitor_killswitch.py')
        self.assertTrue(monitor.release_lock_present(self.root))
        with self.assertRaises(monitor.ReleaseLocked):
            monitor.trigger_kill_switch('owned-fixture')

    def test_migration_and_target_start_failure_real_exit_fence(self):
        initial_state = self.state()
        for fault in ('migration', 'start'):
            with self.subTest(fault=fault):
                if (self.root / '.prestage/cold-recovery-required').exists():
                    # Owned fixture reset ONLY, not an operational resume path.
                    shutil.rmtree(str(self.root / '.prestage/cold-recovery-required'))
                save(self.root / 'docker-state.json', copy.deepcopy(initial_state))
                self.fault(fault)
                result = self.deploy()
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.stopped()
                hold = self.cold.read_hold('.prestage')
                self.assertEqual(hold['state'], 'STOPPED_RECOVERY_REQUIRED')
                self.assertEqual(hold['release_sha'], TARGET)
                self.assertIn(hold['failure_stage'], ('migration', 'target-start', 'first-stop'))
                for command in self.state()['commands']:
                    if command[0] == 'compose' and 'up' in command and 'xianzhi-ai' in command:
                        self.assertIn('.prestage/cold-compose.json', command)

    def test_partial_stop_inspection_and_restart_failure_never_stopped_pass(self):
        initial_state = self.state()
        for fault in ('stop', 'inspect', 'update'):
            with self.subTest(fault=fault):
                if (self.root / '.prestage/cold-recovery-required').exists():
                    shutil.rmtree(str(self.root / '.prestage/cold-recovery-required'))
                save(self.root / 'docker-state.json', copy.deepcopy(initial_state))
                self.fault(fault)
                result = self.deploy()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.cold.read_hold('.prestage')['state'], 'RECOVERY_REQUIRED_UNKNOWN')
                self.assertNotIn('STOPPED_RECOVERY_REQUIRED', result.stdout)

    def test_tampered_policy_receipt_config_and_helper(self):
        original = copy.deepcopy(self.proof)
        for field in ('signature', 'receipt', 'config', 'helper'):
            with self.subTest(field=field):
                if field == 'signature':
                    self.proof['cold_recovery_policy']['project'] = 'other'
                    save(self.root / 'proof.json', self.proof)
                elif field == 'receipt':
                    (self.root / 'receipt.json').write_text('{}')
                elif field == 'config':
                    (self.root / '.prestage/cold-compose.json').write_text('{}')
                else:
                    with (self.root / 'ops/first-upgrade-cold.py').open('a') as stream:
                        stream.write('\n# tampered\n')
                result = self.deploy()
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())
                self.proof = copy.deepcopy(original)
                self.sign()
                save(self.root / 'receipt.json', self.receipt)
                save(self.root / '.prestage/cold-compose.json', self.cold.cold_model(self.model))

    def test_unknown_capable_and_alternate_baseline_rejected(self):
        for sha_value in (TARGET, '1' * 40):
            receipt = dict(self.receipt, previous_git_sha=sha_value)
            with self.assertRaises(self.cold.Refused):
                self.cold.baseline_identity(receipt, self.capability.Docker())
        self.proof['rollback_runtime_capability'] = self.proof['runtime_capability']
        self.sign()
        self.assertNotEqual(self.deploy().returncode, 0)
        self.proof['rollback_runtime_capability'] = None
        self.sign()
        state = self.state()
        state['images'][OLDREF]['Id'] = IMAGE
        save(self.root / 'docker-state.json', state)
        self.assertNotEqual(self.deploy().returncode, 0)
        self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())

    def test_target_evidence_mandatory(self):
        self.proof['runtime_capability']['observations'] = []
        self.sign()
        self.assertNotEqual(self.deploy().returncode, 0)
        self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())

    def test_success_independently_verifies_and_audits_no_restart(self):
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())
        audit = json.loads(next((self.root / '.prestage').glob('cold-forward-*.json')).read_text())
        self.assertEqual(audit['state'], 'FORWARD_VERIFIED')
        self.assertIn('proxy', audit['restored_services'])
        self.assertTrue((self.root / 'catalog-observed').exists())  # ordinary post-start gate
        for cid, obj in self.state()['containers'].items():
            if cid.startswith('new-'):
                self.assertEqual(obj['HostConfig']['RestartPolicy']['Name'], 'no')
        calls = self.state()['commands']
        self.assertGreaterEqual(sum(c[0] == 'exec' and '/proc/1/cmdline' in c for c in calls), 6)

    def test_pid1_health_and_required_proxy_failure_keeps_hold(self):
        initial = self.state()
        for fault in ('pid1', 'health', 'ancillary'):
            with self.subTest(fault=fault):
                shutil.rmtree(str(self.root / '.prestage/cold-recovery-required'), ignore_errors=True)
                save(self.root / 'docker-state.json', copy.deepcopy(initial))
                self.fault(fault)
                result = self.deploy()
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.stopped()
                self.assertEqual(self.cold.read_hold('.prestage')['state'], 'STOPPED_RECOVERY_REQUIRED')

    def fixture_arm_and_target(self):
        lock = self.root / '.prestage/release.lock'
        lock.mkdir()
        (lock / 'owner_token').write_text('owned')
        args = types.SimpleNamespace(proof='proof.json', receipt=None, compose_file='compose.prod.yml',
                                     env_file='.env.production', prestage_dir='.prestage', owner='owned', stage='first-stop', ledger='backups/release-ledger.json')
        docker = self.capability.Docker()
        self.cold.arm(args, docker)
        self.cold.fence('.prestage', 'first-stop', 'exit-failure', docker, True, 'owned')
        docker.text(['compose', '-f', '.prestage/cold-compose.json', '--env-file', '.env.production', 'up', '-d'])
        with self.assertRaises(Exception): self.cold.complete(args, docker)  # no consumed entry/persisted desired image
        self.assertTrue((self.root / '.prestage/cold-recovery-required').exists())
        (self.root / 'backups').mkdir(exist_ok=True)
        self.ledger = {'ledger_version': '1.0', 'entries': [{'nonce': self.proof['proof_nonce'], 'git_sha': TARGET,
                       'image_reference': REF, 'status': 'CONSUMED', 'consumed_at': 'owned-fixture'}]}
        save(self.root / args.ledger, self.ledger)
        (self.root / '.env.production').write_text('fixture=owned\nXIANZHI_IMAGE_REFERENCE=' + REF + '\n')
        return args, docker

    def test_complete_rejects_unauthorized_failed_drift_and_unknown_state(self):
        args, docker = self.fixture_arm_and_target()
        hold = self.cold.read_hold('.prestage')
        initial = self.state()
        for fault in ('owner', 'lock', 'failed', 'proof', 'pid1', 'restart', 'image', 'unknown', 'proxy', 'health', 'worker', 'migration', 'migration-image', 'migration-project', 'migration-tag-drift', 'ledger', 'desired', 'lock-drift'):
            with self.subTest(fault=fault):
                args.owner = 'other' if fault == 'owner' else 'owned'
                (self.root / '.prestage/release.lock/owner_token').write_text('other' if fault == 'lock' else 'owned')
                save(self.root / args.ledger, self.ledger)
                (self.root / '.env.production').write_text('fixture=owned\nXIANZHI_IMAGE_REFERENCE=' + (OLDREF if fault == 'desired' else REF) + '\n')
                if fault == 'ledger': save(self.root / args.ledger, {'entries': [dict(self.ledger['entries'][0], git_sha=BASELINE)]})
                current = dict(hold)
                if fault == 'failed': current['state'] = 'STOPPED_RECOVERY_REQUIRED'
                if fault == 'proof': current['proof_sha256'] = '0' * 64
                self.cold.write_hold('.prestage', current)
                state = copy.deepcopy(initial)
                if fault in ('pid1', 'health', 'lock-drift'): state['fault'] = fault
                if fault == 'worker': state['containers']['new-smartvideo-worker']['State']['Health']['Status'] = 'unhealthy'
                if fault == 'migration': state['containers']['new-migrate']['State']['ExitCode'] = 1
                if fault == 'migration-image': state['containers']['new-migrate']['Image'] = IMAGE
                if fault == 'migration-project': state['containers']['new-migrate']['Config']['Labels']['com.docker.compose.project'] = 'other'
                if fault == 'migration-tag-drift':
                    state['images']['postgres:16-alpine']['Id'] = 'unapproved-runner-image'
                    state['containers']['new-migrate']['Image'] = 'unapproved-runner-image'
                if fault == 'restart': state['containers']['new-xianzhi-ai']['HostConfig']['RestartPolicy']['Name'] = 'always'
                if fault == 'image': state['containers']['new-xianzhi-ai']['Image'] = OLDIMAGE
                if fault == 'unknown': state['containers']['new-proxy']['State']['Status'] = 'dead'
                if fault == 'proxy': state['containers']['new-proxy']['State'].update(Running=False,Status='exited')
                save(self.root / 'docker-state.json', state)
                with self.assertRaises(Exception): self.cold.complete(args, docker)
                self.assertTrue((self.root / '.prestage/cold-recovery-required').exists())
        self.assertFalse((self.root / 'catalog-observed').exists())  # zero SQL in cold completion
        args.owner = 'owned'
        (self.root / '.prestage/release.lock/owner_token').write_text('owned')
        save(self.root / 'docker-state.json', initial)
        save(self.root / args.ledger, self.ledger)
        (self.root / '.env.production').write_text('fixture=owned\nXIANZHI_IMAGE_REFERENCE=' + REF + '\n')
        original_proof = copy.deepcopy(self.proof)
        self.proof['runtime_capability']['expires_at_unix'] = int(time.time()) - 1
        self.sign()
        stale_hold = dict(hold, proof_sha256=self.cold.file_hash('proof.json'))
        self.cold.write_hold('.prestage', stale_hold)
        with self.assertRaises(self.cold.Refused): self.cold.complete(args, docker)
        self.assertTrue((self.root / '.prestage/cold-recovery-required').exists())
        self.proof = original_proof
        self.sign()
        tampered = dict(hold, owner='other', signature='0' * 64)
        save(self.root / '.prestage/cold-recovery-required/state.json', tampered)
        with self.assertRaises(self.cold.Refused): self.cold.complete(args, docker)
        self.assertTrue((self.root / '.prestage/cold-recovery-required').exists())
        self.assertFalse((self.root / 'catalog-observed').exists())
        args.owner = 'owned'
        (self.root / '.prestage/release.lock/owner_token').write_text('owned')
        self.cold.write_hold('.prestage', hold)
        save(self.root / 'docker-state.json', initial)
        model = dict(self.model, _compose_file='compose.prod.yml', _env_file='.env.production')
        self.capability.verify_running(docker, model, self.proof['runtime_capability'], REF)
        self.assertEqual((self.root / 'catalog-observed').read_text().count('read-only barrier'), 1)
        self.cold.complete(args, docker)
        self.assertEqual((self.root / 'catalog-observed').read_text().count('read-only barrier'), 1)
        self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())

    def test_signals_fence_and_sigkill_creation_policy_and_reentry(self):
        initial = self.state()
        for sig in ('TERM', 'INT', 'KILL'):
            with self.subTest(signal=sig):
                shutil.rmtree(str(self.root / '.prestage/cold-recovery-required'), ignore_errors=True)
                shutil.rmtree(str(self.root / '.prestage/release.lock'), ignore_errors=True)
                if (self.root / 'signal-ready').exists(): (self.root / 'signal-ready').unlink()
                save(self.root / 'docker-state.json', copy.deepcopy(initial))
                self.fault('sigkill' if sig == 'KILL' else 'signal')
                fixture = self.root.as_posix()
                if os.name == 'nt': fixture = '/' + fixture[0].lower() + fixture[2:]
                command = 'export PATH=' + shlex.quote(fixture + '/bin') + ':"$PATH"; exec bash deploy.sh --prestaged ' + TARGET + ' --proof proof.json --first-upgrade-cold'
                process = subprocess.Popen([BASH, '-c', command], cwd=str(self.root), env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                try:
                    deadline = time.monotonic() + (180 if os.name == 'nt' else 60)
                    while not (self.root / 'signal-ready').exists():
                        if process.poll() is not None or time.monotonic() > deadline:
                            self.fail('signal fixture did not reach owned migration: ' + str(process.communicate(timeout=5)))
                        time.sleep(0.1)
                    pid = (self.root / '.prestage/release.lock/owner_pid').read_text().strip()
                    killed = subprocess.run([BASH, '-c', 'kill -' + sig + ' ' + pid], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                    self.assertEqual(killed.returncode, 0, killed.stderr)
                    process.communicate(timeout=180 if os.name == 'nt' else 20)
                    self.assertNotEqual(process.returncode, 0)
                finally:
                    if process.poll() is None:
                        process.kill()
                        process.communicate(timeout=10)
                hold = self.cold.read_hold('.prestage')
                if sig == 'KILL':
                    self.assertEqual(hold['state'], 'ARMED_RECOVERY_REQUIRED')
                    self.assertEqual(self.state()['containers']['new-migrate']['HostConfig']['RestartPolicy']['Name'], 'no')
                    time.sleep(3)  # owned stub transport exits; no stranded writer
                else:
                    self.assertEqual(hold['trigger_reason'], 'signal-' + sig)
                    self.assertEqual(hold['state'], 'STOPPED_RECOVERY_REQUIRED')
                    self.stopped()
                before = self.state()['commands'][:]
                self.assertNotEqual(self.deploy().returncode, 0)
                self.assertEqual(before, self.state()['commands'])

    def test_arm_persisted_before_return_signals_are_owned_and_fenced(self):
        initial = self.state()
        path = self.root / 'ops/first-upgrade-cold.py'
        original = path.read_text(encoding='utf-8')
        seam = "    write_hold(args.prestage_dir, hold)\n    print(cold['config_path'])"
        for script, sig in (('deploy.sh', 'TERM'), ('deploy.sh', 'INT'), ('rollback.sh', 'TERM'), ('rollback.sh', 'INT')):
            with self.subTest(script=script, signal=sig):
                shutil.rmtree(str(self.root / '.prestage/cold-recovery-required'), ignore_errors=True)
                save(self.root / 'docker-state.json', copy.deepcopy(initial))
                # Fault injection lives ONLY in this disposable copied helper,
                # with its bytes rebound into the signed owned fixture proof.
                injected = "    write_hold(args.prestage_dir, hold)\n    with open(os.path.join(args.prestage_dir, 'release.lock', 'owner_pid')) as stream:\n        pid = stream.read().strip()\n    subprocess.check_call([" + repr(BASH) + ", '-c', 'kill -" + sig + " ' + pid])\n    time.sleep(1)\n    print(cold['config_path'])"
                self.assertIn(seam, original)
                path.write_text(original.replace(seam, injected), encoding='utf-8')
                self.proof['deploy_scripts_hash']['ops/first-upgrade-cold.py'] = self.cold.file_hash(str(path))
                self.sign()
                if script == 'deploy.sh': result = self.deploy()
                else: result = self.shell(script, '--first-upgrade-cold', '--receipt', 'receipt.json', '--capability-proof', 'proof.json')
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                hold = self.cold.read_hold('.prestage')
                self.assertEqual(hold['state'], 'STOPPED_RECOVERY_REQUIRED', result.stdout + result.stderr)
                self.assertEqual(hold['trigger_reason'], 'signal-' + sig)
                self.stopped()

    def test_arm_validation_before_hold_and_foreign_owner_cleanup_no_mutations(self):
        path = self.root / 'ops/first-upgrade-cold.py'
        original = path.read_text(encoding='utf-8')
        seam = '    os.mkdir(os.path.join(args.prestage_dir, HOLD), 0o700)'
        self.assertIn(seam, original)
        path.write_text(original.replace(seam, "    raise Refused('owned injected arm refusal')\n" + seam), encoding='utf-8')
        self.proof['deploy_scripts_hash']['ops/first-upgrade-cold.py'] = self.cold.file_hash(str(path))
        self.sign()
        for script in ('deploy.sh', 'rollback.sh'):
            result = self.deploy() if script == 'deploy.sh' else self.shell(script, '--first-upgrade-cold', '--receipt', 'receipt.json', '--capability-proof', 'proof.json')
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((self.root / '.prestage/cold-recovery-required').exists())
            self.assertNotIn('STOPPED_RECOVERY_REQUIRED', result.stdout)
            self.assertFalse(any(c[0] in ('update', 'stop') for c in self.state()['commands']))
        path.write_text(original, encoding='utf-8')
        self.proof['deploy_scripts_hash']['ops/first-upgrade-cold.py'] = self.cold.file_hash(str(path))
        self.sign()
        lock = self.root / '.prestage/release.lock'
        lock.mkdir()
        (lock / 'owner_token').write_text('owned')
        args = types.SimpleNamespace(proof='proof.json', receipt=None, compose_file='compose.prod.yml', env_file='.env.production', prestage_dir='.prestage', owner='owned', stage='first-stop')
        self.cold.arm(args, self.capability.Docker())
        before = self.state()['commands'][:]
        hold = self.cold.read_hold('.prestage')
        result = subprocess.run([sys.executable, '-B', 'ops/first-upgrade-cold.py', 'fence', '--owner', 'foreign', '--proof', 'proof.json'], env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(before, self.state()['commands'])
        self.assertEqual(hold, self.cold.read_hold('.prestage'))
        with self.assertRaises(self.cold.Refused):
            self.cold.fence('.prestage', 'cleanup', 'exit-failure', self.capability.Docker(), owner='owned', proof_path='manifest.json')
        self.assertEqual(before, self.state()['commands'])

    def test_audit_write_failure_is_not_pass(self):
        lock = self.root / '.prestage/release.lock'
        lock.mkdir()
        (lock / 'owner_token').write_text('owned')
        result = subprocess.run(
            [sys.executable, '-B', 'ops/first-upgrade-cold.py', 'arm', '--proof', 'proof.json', '--owner', 'owned'], env=self.env, stdout=subprocess.PIPE)
        self.assertEqual(result.returncode, 0)
        state = self.root / '.prestage/cold-recovery-required/state.json'
        state.unlink()
        state.mkdir()  # exact owned audit path cannot be read/written
        result = subprocess.run([sys.executable, '-B', 'ops/first-upgrade-cold.py', 'fence'], env=self.env, stdout=subprocess.PIPE)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(b'STOPPED', result.stdout)


if __name__ == '__main__':
    unittest.main()
