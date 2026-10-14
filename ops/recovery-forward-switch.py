#!/usr/bin/env python3
"""One fixed release213 API/Worker replacement under retained hold (Python3.6).

No production execution authority is conveyed by this source. No rollback,
ordinary deploy, migration, enrollment, desired-image persistence or task writes.
"""
import argparse
import copy
import datetime
import json
import os
import re
import select
import signal
import stat
import subprocess
import sys
import time
import uuid

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
# Import an isolated namespace; never change the old entry's globals or CLI.
import types
r = types.ModuleType('forward_read_only')
r.__file__ = os.path.join(ROOT, 'ops/cold-recovery-resume.py')
with open(r.__file__, 'rb') as stream:
    exec(compile(stream.read(), r.__file__, 'exec'), r.__dict__)
cold, cap, runtime, transport = r.cold, r.cap, r.runtime, r.transport
Refused, require, digest, authenticate = r.Refused, r.require, r.digest, r.authenticate
OLD_APP, OLD_DIGEST, OLD_IMAGE, OLD_REF = r.APP, r.DIGEST, r.IMAGE, r.REF
APP = '071ccc00136459b612c2c274be6f65bfdc72b76d'
DIGEST = 'sha256:bc6da7a08a53907f7f4c3d302d616012a718163c42e136cd34c70feca6226cbd'
REF = 'crpi-kvtl7houjvj5ftba.cn-hangzhou.personal.cr.aliyuncs.com/zhiqiyun-ai/zhiqiyun-ai@' + DIGEST
CANONICAL_REF = REF
MANIFEST_HASH = '62d18f187f143bf713b6eb47cbf61ac1f4ba7a042ed67d0ba4d333bf181e22c8'
MANIFEST = os.path.join(ROOT, 'ops/forward-switch-manifest.json')
ROLES, PROJECT, DIR, WORK = r.ROLES, r.PROJECT, r.DIR, r.WORK
COMPOSE, ENV, ANCHOR_PATH = r.COMPOSE, r.ENV, r.ANCHOR_PATH
DOMAIN = 'release213-fixed-forward-switch'
STARTING = 'FORWARD_API_WORKER_REPLACING'
SUCCESS = 'FORWARD_API_WORKER_VERIFIED_ANCILLARY_HELD'
CANDIDATE = os.path.join(WORK, 'forward-candidate.json')
APPROVAL = os.path.join(WORK, 'forward-approval.json')
LABEL = 'org.xianzhi.release213.forward-nonce'
OPERATIONS = ['stop-old-pair', 'remove-exact-stopped-old-pair', 'create-fixed-new-pair', 'start-fixed-new-pair']


def manifest_identity():
    require(cold.file_hash(MANIFEST) == MANIFEST_HASH, 'official manifest bytes changed')
    value = r.read(MANIFEST)
    require(value['git_sha'] == APP and value['digest'] == DIGEST and
            value['production_contract'] == 'passed' and
            value['registries']['aliyun_acr']['image_reference'] == CANONICAL_REF,
            'fixed official carrier mismatch')
    return MANIFEST_HASH


def source_identity():
    value = r.source_identity()
    require(value['controller_source_sha'] not in (OLD_APP, APP), 'separate controller required')
    # Old source identity includes every tracked ops file. Include this narrow CI,
    # docs/tests too; publication must precede production candidate preparation.
    import subprocess
    paths = subprocess.check_output(['git', 'ls-files'], cwd=ROOT).decode().splitlines()
    selected = [p for p in paths if 'forward-switch' in p]
    require('ops/recovery-forward-switch.py' in selected, 'published forward controller required')
    value['forward_sha256'] = {p: cold.file_hash(os.path.join(ROOT, p)) for p in selected}
    value['target_app_source_sha'] = APP
    value['official_manifest_sha256'] = manifest_identity()
    return value


def private_json(path, value):
    if os.path.lexists(path):
        r.protected_path(path, secret=True)
    cold.atomic_json(path, value)
    r.protected_path(path, secret=True)


def historical_session(hold, historical, candidate, signed_approval, anchor):
    """Expired signed documents establish ancestry ONLY, never fresh authority."""
    require(hold.get('version') == 1 and type(hold['version']) is int and
            hold.get('state') == r.SUCCESS and not hold.get('errors') and
            not hold.get('indeterminate') and not hold.get('recovery_indeterminate_operation') and
            not hold.get('forward_attempt'), 'only original successful recovery hold')
    require(hold.get('release_sha') == OLD_APP and hold.get('proof_sha256') == r.OLD_PROOF and
            hold.get('project') == PROJECT and hold.get('image_ids') == [OLD_IMAGE, r.OLD_IMAGE] and
            type(hold.get('owner')) is str and bool(hold['owner']), 'historical fixed hold identity')
    # Earliest stopped hold proves stable ancestry, not the latest pre-start
    # hash: completely fenced recovery attempts retain this original via setdefault.
    original = authenticate(hold['recovery_original_hold'])
    require(original['state'] == r.STOPPED and
            all(original.get(k) == hold.get(k) for k in
                ('owner', 'project', 'image_ids', 'release_sha', 'proof_sha256', 'proof_nonce')),
            'original stopped ancestry mismatch')
    require(set(candidate) == {'version', 'purpose', 'nonce', 'not_before', 'expires_at',
                              'hold_sha256', 'snapshot', 'runtime_capability', 'operations'} and
            type(candidate['version']) is int and candidate['version'] == 1 and
            candidate['purpose'] == r.DOMAIN + '-proof' and
            candidate['operations'] == ['start-existing-api', 'start-existing-worker'] and
            type(candidate['nonce']) is str and re.fullmatch('[0-9a-f]{32}', candidate['nonce']) and
            re.fullmatch('[0-9a-f]{64}', candidate['hold_sha256']), 'historical recovery candidate mismatch')
    human = authenticate(signed_approval)
    require(set(human) == {'version', 'purpose', 'candidate_sha256', 'hold_sha256', 'nonce',
                          'operator', 'confirmation', 'not_before', 'expires_at', 'approved_at'} and
            type(human['version']) is int and human['version'] == 1 and
            human['purpose'] == r.DOMAIN + '-human-approval' and
            human['candidate_sha256'] == digest(candidate) and human['hold_sha256'] == candidate['hold_sha256'] and
            human['nonce'] == candidate['nonce'] and human['confirmation'] == r.phrase(candidate) and
            type(human['operator']) is str and re.fullmatch('[A-Za-z0-9_.@-]{3,80}', human['operator']),
            'historical human binding mismatch')
    for doc, bound in ((candidate, 900), (human, 300)):
        require(type(doc['not_before']) is int and type(doc['expires_at']) is int and
                0 < doc['expires_at'] - doc['not_before'] <= bound, 'historical window malformed')
    require(type(human['approved_at']) is int and human['approved_at'] == human['not_before'] and
            candidate['not_before'] <= human['not_before'] < human['expires_at'] <= candidate['expires_at'],
            'historical approval window mismatch')
    cids = [anchor['services'][role]['cid'] for role in ROLES]
    attempt = hold.get('recovery_attempt', {})
    require(attempt.get('nonce') == candidate['nonce'] and attempt.get('owner') and
            attempt.get('operator') == human['operator'] and
            attempt.get('candidate_sha256') == digest(candidate) and
            attempt.get('approval_sha256') == digest(signed_approval) and
            hold.get('recovery_spent_nonces', []).count(candidate['nonce']) == 1 and
            hold.get('recovery_authorized_cids') == cids == hold.get('recovery_verified_cids') and
            cids == [candidate['snapshot']['containers'][role]['cid'] for role in ROLES] and
            hold.get('recovery_intentionally_stopped') == ['disk-monitor', 'postgres-backup', 'migrate'] and
            hold.get('recovery_no_env_persistence') is True and
            type(hold.get('recovery_verified_at')) is int and
            human['not_before'] <= hold['recovery_verified_at'] < human['expires_at'],
            'successful spent original session required')
    require(historical.get('git_sha') == OLD_APP and historical.get('image_reference') == OLD_REF and
            historical.get('proof_nonce') == hold.get('proof_nonce') and
            historical.get('cold_recovery_policy', {}).get('baseline_identity', {}).get('local_image_id') == r.OLD_IMAGE and
            historical.get('cold_recovery_policy', {}).get('project') == PROJECT and
            historical.get('compose_hash') == cold.file_hash(COMPOSE) and
            historical.get('env_hash') == cold.file_hash(ENV), 'original historical proof drift')
    prior = candidate['snapshot']
    require(prior['source']['app_source_sha'] == OLD_APP and
            re.fullmatch('[0-9a-f]{40}', prior['source']['controller_source_sha']) and
            prior['source']['controller_source_sha'] != OLD_APP and
            prior['database']['quarantine_records'] == anchor['quarantine_records'] and
            prior['database']['history'] == anchor['history'] and
            prior['compose_sha256'] == cold.file_hash(COMPOSE) and
            prior['env_sha256'] == cold.file_hash(ENV), 'prior snapshot history/config mismatch')
    evidence = candidate['runtime_capability']
    identity = prior['image_identity']
    require(identity == evidence['identity'] and identity['local_image_id'] == OLD_IMAGE and
            identity['reference'] == OLD_REF and identity['release_sha'] == OLD_APP,
            'prior packaged old identity required')
    verify_prepared_capability(evidence, OLD_REF, OLD_APP, prior['policy'], identity, historical=True)
    return {'old_identity': identity, 'old_policy': prior['policy'],
            'candidate_sha256': digest(candidate), 'approval_sha256': digest(signed_approval)}


def read_success_hold():
    return authenticate(r.read(os.path.join(WORK, 'state.json')))


def history_binding(hold):
    for path in (r.PROOF_PATH, r.CANDIDATE, r.APPROVAL, ANCHOR_PATH):
        r.protected_path(path)
    require(cold.file_hash(r.PROOF_PATH) == r.OLD_PROOF, 'original proof bytes changed')
    return historical_session(hold, authenticate(r.read(r.PROOF_PATH)),
                              authenticate(r.read(r.CANDIDATE)), r.read(r.APPROVAL), r.read(ANCHOR_PATH))


# Pure capability validation aliases (no Docker access in these functions).
VERSION, ROLES_CAPABILITY, OPS = cap.VERSION, cap.ROLES, cap.OPS
sha, canonical = cap.sha, cap.canonical
check_allowed, verify_history = cap.check_allowed, cap.verify_history

def verify_prepared_capability(evidence, ref, release_sha, policy, identity, historical=False):
    """Exact structural verifier parity; authenticated history does not renew TTL.

    Native image hash RUN is replaced ONLY by an inspect-bound prepared identity.
    All behavior/effect/history guards below remain identical to cap.verify.
    """
    if not isinstance(evidence, dict) or type(evidence.get('version')) is not int or evidence.get('version') != VERSION or evidence.get('synthetic_nonofficial') is not False:
        raise Refused('runtime capability evidence missing/unknown/nonofficial')
    require(identity.get('reference') == ref and identity.get('release_sha') == release_sha and
            re.fullmatch('sha256:[0-9a-f]{64}', identity.get('local_image_id', '')) and
            ref in identity.get('repo_digests', []) and
            set(identity.get('binary_sha256', {})) == set(cap.BINARIES) and
            all(re.fullmatch('[0-9a-f]{64}', h) for h in identity['binary_sha256'].values()),
            'prepared capability identity malformed')
    require(cap.verify_history(evidence, release_sha), 'full packaged history required')
    now = int(time.time())
    start, end = evidence.get('created_at_unix'), evidence.get('expires_at_unix')
    if type(start) is not int or type(end) is not int or end - start != 3600 or (not historical and (start > now or end <= now)):
        raise Refused('runtime capability stale: restage before stop')
    if not re.match(r'^[0-9a-f-]{36}$', evidence.get('nonce', '')) or evidence.get('policy') != policy:
        raise Refused('runtime capability nonce/policy mismatch')
    if evidence.get('identity') != identity:
        raise Refused('runtime capability immutable identity mismatch')
    observations = evidence.get('observations', [])
    if len(observations) != 4 or {(x.get('role'), x.get('phase')) for x in observations} != {(r, p) for r in ROLES_CAPABILITY for p in ('allowed', 'blocked')}:
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
    if len(unavailable) != 6 or {(x.get('role'), x.get('mode')) for x in unavailable} != {(r, p) for r in ROLES_CAPABILITY for p in ('missing', 'denied', 'timeout')}:
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


def attest_owned(ref, app, policy):
    """Private attester namespace: real packaged fixture behavior, empty journal first.

    Production namespace and shared helper source are untouched. Hash-only RUNs
    have their own UUID name/owner premanifest; fixture records are private0600.
    A leftover/unrecorded resource refuses candidate publication.
    """
    module = r.load('forward_owned_attester', 'verify-image-quarantine-capability.py')
    out = os.path.join(ROOT, '.evidence/forward-switch/attest')
    os.makedirs(out, mode=0o700, exist_ok=True)
    r.protected_path(out)
    base_docker, base_fixture = module.Docker, module.Fixture
    fixtures, helpers = [], []

    class OwnedDocker(base_docker):
        def run(self, args, **kwargs):
            if args[:2] != ['run', '--rm']:
                return super().run(args, **kwargs)
            owner = uuid.uuid4().hex
            name = 'forward-hash-' + owner
            path = os.path.join(out, 'hash-' + owner + '.json')
            private_json(path, {'owner': owner, 'name': name, 'resources': [], 'purpose': 'network-none packaged hash'})
            helpers.append((owner, name, path))
            cid = None
            try:
                create = ['create', '--name', name, '--label', LABEL + '=' + owner] + args[2:]
                cid = super().run(create, **kwargs).stdout.decode().strip()
                obj = self.inspect('container', cid)
                require(obj['Id'] == cid and re.fullmatch('[0-9a-f]{64}', cid) and
                        (obj['Config'].get('Labels') or {}).get(LABEL) == owner,
                        'hash helper exact owner required')
                private_json(path, {'owner': owner, 'name': name, 'resources': [('container', cid)]})
                result = super().run(['start', '-a', cid], **kwargs)
                require(self.inspect('container', cid)['State']['ExitCode'] == 0, 'hash helper failed')
                return result
            finally:
                if cid is not None:
                    saved = self.deadline
                    self.deadline = None
                    try:
                        obj = self.inspect('container', cid)
                        require(obj['Id'] == cid and (obj['Config'].get('Labels') or {}).get(LABEL) == owner,
                                'hash helper foreign cleanup refused')
                        super().run(['rm', '-f', cid], timeout=20)
                        private_json(path, {'owner': owner, 'name': name, 'resources': [('container', cid)], 'removed': True})
                    finally:
                        self.deadline = saved

    class OwnedFixture(base_fixture):
        def __init__(self, docker):
            super().__init__(docker)
            self.record_path = os.path.join(out, 'owned-' + self.owner + '.json')
            private_json(self.record_path, {'owner': self.owner, 'resources': []})
            fixtures.append(self)

        def record(self, kind, identity):
            obj = self.docker.inspect(kind, identity)
            labels = obj.get('Labels') if kind == 'network' else obj.get('Config', {}).get('Labels')
            require((labels or {}).get(module.LABEL) == self.owner, 'fixture exact owner required')
            self.resources.append((kind, obj['Id']))
            private_json(self.record_path, {'owner': self.owner, 'resources': self.resources})
            return obj['Id']

    module.Docker, module.Fixture = OwnedDocker, OwnedFixture
    evidence = module.attest(ref, app, policy, synthetic=False,
                             evidence_directory=out, include_history=True)
    docker = OwnedDocker()
    for fixture in fixtures:
        require(not docker.text(['ps', '-aq', '--no-trunc', '--filter', 'label=' + module.LABEL + '=' + fixture.owner]),
                'owned attestation container cleanup incomplete')
        require(not docker.text(['network', 'ls', '-q', '--no-trunc', '--filter', 'label=' + module.LABEL + '=' + fixture.owner]),
                'owned attestation network cleanup incomplete')
        private_json(fixture.record_path, {'owner': fixture.owner, 'resources': fixture.resources, 'removed': True})
    for owner, name, path in helpers:
        require(r.read(path).get('removed') is True, 'hash helper cleanup incomplete')
    private_json(os.path.join(out, 'certification.json'), evidence)
    return evidence


def inspect_identity(docker, identity):
    obj = docker.inspect('image', identity['local_image_id'])
    require(obj['Id'] == identity['local_image_id'] and
            sorted(obj.get('RepoDigests') or []) == identity['repo_digests'] and
            identity['reference'] in obj.get('RepoDigests', []) and
            {'os': obj['Os'], 'architecture': obj['Architecture'], 'variant': obj.get('Variant', '')} == identity['platform'] and
            identity['platform']['os'] == 'linux' and identity['platform']['architecture'] == 'amd64' and
            not obj.get('Config', {}).get('Entrypoint'), 'prepared immutable inspect identity drift')
    return obj


# Fixed native READ ONLY descriptor; mutable334 heartbeat/events are intentionally
# absent. The durable succeeded result and generation/status reuse preconditions
# are checked, not immutable snapshots of its normal completion side effects.
REUSE = ("SELECT e.id,e.task_id,e.attempt,e.status,e.task_execution_generation,e.succeeded_at,"
         "e.result_metadata,t.status,t.task_status,t.execution_generation "
         "FROM public.provider_executions e JOIN public.xz_generation_tasks t ON t.id=e.task_id "
         "WHERE e.task_id='task_000334' ORDER BY e.id")
REUSE_OIDS = (20, 25, 23, 25, 20, 1184, 3802, 25, 25, 20)


class NativeDocker(cap.Docker):
    """Entry-local native adapter: owns CLI group, not Engine settlement.

    Nonblocking pipe I/O shares one fixed deadline. A plugin retaining a pipe
    cannot obstruct UNKNOWN/fencing; killing local processes never cancels an
    accepted daemon request. Ordinary cap.Docker is deliberately unchanged.
    """
    def __init__(self):
        super().__init__()
        self.parent_fd = None

    def check_budget(self):
        if self.deadline is not None:
            require(time.monotonic() < self.deadline, 'native guard deadline')
        if self.parent_fd is not None:
            require(not select.select([self.parent_fd], [], [], 0)[0], 'native controller EOF')

    def run(self, args, data=None, timeout=60, check=True, capture_limit=16 * 1024 * 1024, env=None):
        self.check_budget()
        require(type(capture_limit) is int and capture_limit > 0 and timeout > 0, 'native bounds required')
        end = time.monotonic() + timeout
        if self.deadline is not None:
            end = min(end, self.deadline)
        argv = self.cli + list(args)
        process = subprocess.Popen(argv, stdin=subprocess.PIPE if data is not None else subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=self.env if env is None else env,
                                   cwd=ROOT, start_new_session=True)
        outputs = {process.stdout.fileno(): bytearray(), process.stderr.fileno(): bytearray()}
        truncated = {fd: False for fd in outputs}
        active = set(outputs)
        input_fd = process.stdin.fileno() if data is not None else None
        sent = 0
        for fd in list(active) + ([input_fd] if input_fd is not None else []):
            os.set_blocking(fd, False)
        try:
            while active or input_fd is not None or process.poll() is None:
                self.check_budget()
                remaining = end - time.monotonic()
                if remaining <= 0:
                    raise subprocess.TimeoutExpired(argv, timeout)
                reads = list(active) + ([self.parent_fd] if self.parent_fd is not None else [])
                writes = [input_fd] if input_fd is not None else []
                ready, writable, _ = select.select(reads, writes, [], min(.1, remaining))
                self.check_budget()
                for fd in ready:
                    if fd not in active:
                        continue
                    chunk = os.read(fd, 65536)
                    if not chunk:
                        active.remove(fd)
                        continue
                    buffer = outputs[fd]
                    overflow = len(buffer) + len(chunk) - capture_limit
                    if overflow > 0:
                        del buffer[:overflow]
                        truncated[fd] = True
                    buffer.extend(chunk[-capture_limit:])
                if writable:
                    try:
                        sent += os.write(input_fd, data[sent:sent + 65536])
                    except BrokenPipeError:
                        sent = len(data)
                    if sent == len(data):
                        process.stdin.close()
                        input_fd = None
            result = subprocess.CompletedProcess(argv, process.returncode,
                    bytes(outputs[process.stdout.fileno()]), bytes(outputs[process.stderr.fileno()]))
            result.stdout_truncated = truncated[process.stdout.fileno()]
            result.stderr_truncated = truncated[process.stderr.fileno()]
            if check and result.returncode:
                diagnostic = {'argv': [self.redact(str(v)) for v in argv],
                              'stdin_bytes': len(data) if data is not None else 0,
                              'stdin_sha256': cap.sha(data) if data is not None else None,
                              'stdout_truncated': result.stdout_truncated, 'stderr_truncated': result.stderr_truncated,
                              'stdout': self.redact(result.stdout.decode('utf-8', errors='replace'))[-4000:],
                              'stderr': self.redact(result.stderr.decode('utf-8', errors='replace'))[-4000:]}
                raise Refused('Docker operation failed: ' + args[0] + ' (exit ' + str(result.returncode) + '); diagnostic=' + json.dumps(diagnostic, sort_keys=True))
            require(not result.stdout_truncated and not result.stderr_truncated, 'Docker output exceeded bounded capture limit')
            return result
        finally:
            # SIGKILL the owned session even when its leader has exited. No pipe
            # joins/communicate or unbounded wait; leaked Engine acceptance is UNKNOWN.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=min(.5, max(0, end - time.monotonic())))
            except subprocess.TimeoutExpired:
                pass
            for pipe in (process.stdin, process.stdout, process.stderr):
                if pipe is not None:
                    pipe.close()


def postgres_binding(docker):
    # Reuse the exact native binding verifier with an entry-local run binding;
    # never patch the shared transport module or its ordinary callers.
    def run(args, env=None):
        require(args[0] == 'docker', 'read-only native Docker required')
        return docker.run(args[1:], env=env, timeout=transport.TIMEOUT,
                          capture_limit=transport.MAX_BYTES).stdout.decode('utf-8').strip()
    namespace = dict(transport.binding.__globals__, run=run)
    binding = types.FunctionType(transport.binding.__code__, namespace)
    return binding(COMPOSE, ENV, OLD_REF)


class ReadOnlyTarget:
    def __init__(self, expected, docker):
        self.expected, self.docker = expected, docker
        self.check()

    def check(self):
        require(postgres_binding(self.docker) == self.expected, 'PostgreSQL identity drift')


class ReadOnlyCursor(transport.Cursor):
    def execute(self, sql, parameters=()):
        conn = self.connection
        try:
            require(not parameters and sql in tuple(r.DESCRIPTORS) + r.SETTINGS + (REUSE,), 'non-forward SQL refused')
            require(not conn.closed and conn.process.poll() is None, 'read-only backend unavailable')
            token = uuid.uuid4().hex
            expected = REUSE_OIDS if sql == REUSE else r.DESCRIPTORS.get(sql, ())
            if expected:
                require(conn.status == 2, 'read-only transaction required')
                statement = transport.select_statement(sql, token, expected)
            else:
                statement = sql + ";\nSELECT json_build_object('token','" + token + "','count',0,'columns',0,'types','[]'::json,'rows','[]'::json);\n"
            raw, marker = conn.exchange((statement + '\\echo END_' + token + '\n').encode())
            self.rows = transport.decode_frame(raw, token, expected)
            require(marker.strip() == ('END_' + token).encode() and conn.process.poll() is None, 'native frame incomplete')
            if sql.startswith('BEGIN '):
                conn.status = 2
            elif sql == 'ROLLBACK':
                conn.status = 0
        except Exception:
            conn.close()
            raise Refused('forward read-only transport refused')


class ReadOnlyConnection(transport.Connection):
    def __init__(self, target):
        self.target, self.docker = target, target.docker
        self.autocommit, self.closed, self.status = True, False, 0
        self.process = None
        self.buffer = bytearray()
        self.docker.check_budget()
        shell = ('unset PGHOST PGHOSTADDR PGPORT PGSERVICE PGSERVICEFILE PGPASSFILE; '
                 'export PGPASSWORD="$POSTGRES_PASSWORD" PGCLIENTENCODING=UTF8; '
                 'export PGOPTIONS="-c statement_timeout=10000 -c idle_in_transaction_session_timeout=60000 -c timezone=UTC"; '
                 'exec psql -X -q -t -A -h /var/run/postgresql -p 5432 '
                 '-U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1')
        try:
            self.process = subprocess.Popen(self.docker.cli + ['exec', '-i', target.expected['container_id'], 'sh', '-c', shell],
                    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                    env=self.docker.env, cwd=ROOT, start_new_session=True)
            os.set_blocking(self.process.stdin.fileno(), False)
            os.set_blocking(self.process.stdout.fileno(), False)
            self.cursor().execute(r.SETTINGS[0])
            target.check()
        except Exception:
            self.close()
            raise Refused('forward PostgreSQL connection refused')

    def cursor(self):
        require(not self.closed, 'connection closed')
        return ReadOnlyCursor(self)

    def exchange(self, data):
        end = time.monotonic() + transport.TIMEOUT
        if self.docker.deadline is not None:
            end = min(end, self.docker.deadline)
        sent, lines = 0, []
        while len(lines) < 2:
            self.docker.check_budget()
            require(time.monotonic() < end, 'forward SQL frame deadline')
            reads = [self.process.stdout.fileno()]
            if self.docker.parent_fd is not None:
                reads.append(self.docker.parent_fd)
            writes = [self.process.stdin.fileno()] if sent < len(data) else []
            ready, writable, _ = select.select(reads, writes, [], min(.1, max(0, end - time.monotonic())))
            self.docker.check_budget()
            if writable:
                sent += os.write(writes[0], data[sent:sent + 65536])
            if self.process.stdout.fileno() in ready:
                chunk = os.read(self.process.stdout.fileno(), 65536)
                require(bool(chunk), 'forward SQL EOF')
                self.buffer.extend(chunk)
                require(len(self.buffer) <= transport.MAX_BYTES, 'forward SQL bounded frame')
                while b'\n' in self.buffer and len(lines) < 2:
                    index = self.buffer.index(b'\n') + 1
                    lines.append(bytes(self.buffer[:index]))
                    del self.buffer[:index]
        self.docker.check_budget()
        return lines

    def close(self):
        self.closed = True
        if self.process is not None:
            try:
                os.killpg(self.process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                remaining = .5 if self.docker.deadline is None else max(0, self.docker.deadline - time.monotonic())
                self.process.wait(timeout=min(.5, remaining))
            except subprocess.TimeoutExpired:
                pass
            self.process.stdin.close()
            self.process.stdout.close()


def database_snapshot(pg, anchor, docker):
    # Preserve exact9/OID/migration/history checks with entry-local transport.
    namespace = dict(r.database_snapshot.__globals__,
                     ReadOnlyTarget=lambda expected: ReadOnlyTarget(expected, docker),
                     ReadOnlyConnection=ReadOnlyConnection)
    return types.FunctionType(r.database_snapshot.__code__, namespace)(pg, anchor)


# Descriptor test seam.
ReuseCursor = ReadOnlyCursor

def validate_reuse(rows, completed=False):
    require(len(rows) == 1, 'single succeeded113 reuse record required')
    row = rows[0]
    require(len(row) == 10 and type(row[0]) is int and row[0] == 113 and row[1] == 'task_000334' and
            type(row[2]) is int and row[2] == 1 and row[3] == 'succeeded' and
            type(row[4]) is int and row[4] == 3 and type(row[9]) is int and row[9] == 3 and
            type(row[5]) is datetime.datetime and
            row[5].isoformat() == '2026-10-13T03:51:10.958975+00:00' and
            type(row[6]) is list and len(row[6]) == 1 and type(row[6][0]) is dict and
            type(row[6][0].get('URL')) is str and bool(row[6][0]['URL']), 'durable113 manifest/generation drift')
    allowed = [('PROCESSING', 'RUNNING')]
    if completed:
        allowed.append(('SUCCEEDED', 'SUCCEEDED'))
    require((row[7], row[8]) in allowed, 'task334 reuse status drift')
    return {'execution_id': row[0], 'task_id': row[1], 'attempt': row[2], 'status': row[3],
            'generation': row[4], 'succeeded_at': row[5].isoformat(), 'manifest_sha256': digest(row[6])}


def reuse_precondition(pg, completed=False, docker=None):
    conn = ReadOnlyConnection(ReadOnlyTarget(pg, docker or NativeDocker()))
    try:
        cursor = conn.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
        for setting in r.SETTINGS[3:]:
            cursor.execute(setting)
        cursor.execute(REUSE)
        result = validate_reuse(cursor.fetchall(), completed)
        cursor.execute('ROLLBACK')
        conn.target.check()
        return result
    finally:
        conn.close()


def policy_projection(obj):
    value = {k: obj.get(k) for k in ('Id', 'Image', 'Config', 'HostConfig', 'Mounts', 'Path', 'Args')}
    if type(value['Mounts']) is list:
        value['Mounts'] = sorted(value['Mounts'], key=cap.canonical)
    value['networks'] = sorted(obj.get('NetworkSettings', {}).get('Networks', {}))
    return digest(value)


def epoch(obj):
    return {k: obj['State'].get(k) for k in ('Running', 'Status', 'StartedAt', 'FinishedAt', 'ExitCode')}


def verify_process(docker, obj, cfg, policy, identity, model):
    image = inspect_identity(docker, identity)
    expected = dict(x.split('=', 1) for x in image['Config'].get('Env', []))
    expected.update({k: str(v) for k, v in (cfg.get('environment') or {}).items()})
    require(obj['Image'] == identity['local_image_id'] and obj['Config']['Image'] == identity['reference'] and
            obj['Config']['Cmd'] == policy['command'] and (obj['Config'].get('Entrypoint') or []) == policy['entrypoint'] and
            dict(x.split('=', 1) for x in obj['Config'].get('Env', [])) == expected and
            obj['Config'].get('User', '') == (policy['user'] or image['Config'].get('User', '')) and
            obj['HostConfig']['RestartPolicy'] == {'Name': 'no', 'MaximumRetryCount': 0}, 'effective pair policy drift')
    mounts = {}
    for mount in cfg.get('volumes') or []:
        require(type(mount) is dict, 'normalized mounts required')
        source = mount['source']
        if mount['type'] == 'volume':
            source = model.get('volumes', {}).get(source, {}).get('name', source)
        require(mount['target'] not in mounts, 'duplicate mount target')
        mounts[mount['target']] = (mount['type'], source, not mount.get('read_only', False))
    actual = {m['Destination']: (m['Type'], m.get('Name') if m['Type'] == 'volume' else m['Source'], m['RW']) for m in obj.get('Mounts', [])}
    require(actual == mounts and len(actual) == len(obj.get('Mounts', [])), 'pair mounts drift')
    require(set(obj.get('NetworkSettings', {}).get('Networks', {})) ==
            {model['networks'][n]['name'] for n in cfg.get('networks', {})}, 'pair networks drift')
    if obj['State']['Running']:
        cap.verify_pid1_policy(docker, obj['Id'], obj, policy, expected)
        require(docker.text(['exec', obj['Id'], '/usr/bin/sha256sum', '/proc/1/exe']).split()[0] ==
                identity['binary_sha256'][policy['binary']], 'live PID1 binary drift')


class Backend:
    def __init__(self):
        self.docker = NativeDocker()
        self.anchor = r.read(ANCHOR_PATH)

    def snapshot(self, ancestry, completed=False, replacement=None):
        model, policy = cap.rendered_policy(self.docker, COMPOSE, ENV, OLD_REF)
        require(model['name'] == PROJECT and set(policy['roles']) == set(ROLES) and
                policy == ancestry['old_policy'], 'original fixed project/policy drift')
        image_ids = [OLD_IMAGE, r.OLD_IMAGE]
        if replacement is not None:
            image_ids.append(replacement['identity']['local_image_id'])
        # Final health convergence precedes strict inspect health equality.
        runtime.verify('post', COMPOSE, ENV, execute=self.runtime_observation, pause=self.observation_pause)
        found = {}
        for cid in self.docker.text(['ps', '-aq', '--no-trunc']).splitlines():
            obj = self.docker.inspect('container', cid)
            labels = obj['Config'].get('Labels') or {}
            project, role = labels.get('com.docker.compose.project'), labels.get('com.docker.compose.service')
            if obj['Image'] not in image_ids and project != PROJECT:
                continue
            require(project == PROJECT and role in self.anchor['services'] and role not in found,
                    'foreign/extraneous/duplicate owner')
            expected = replacement['cids'][role] if replacement and role in ROLES else self.anchor['services'][role]['cid']
            require(cid == obj['Id'] == expected and re.fullmatch('[0-9a-f]{64}', cid), 'anchor CID drift')
            state = obj['State']
            running = role in cold.INFRASTRUCTURE or role in ROLES
            require(state.get('Running') is running and not state.get('Paused') and not state.get('Restarting'), 'service activity drift')
            if running:
                require(state.get('Health', {}).get('Status') == 'healthy', 'service health drift')
            if role in ROLES:
                identity = replacement['identity'] if replacement else ancestry['old_identity']
                cfg = replacement['model']['services'][role] if replacement else model['services'][role]
                verify_process(self.docker, obj, cfg, policy['roles'][role], identity,
                               replacement['model'] if replacement else model)
            else:
                anchor = self.anchor['services'][role]
                require(obj['Image'] == anchor['image_id'] and obj['Config']['Image'] == anchor['image_reference'] and
                        state['StartedAt'] == anchor['started_at'], 'ancillary identity/epoch drift')
                if role not in cold.INFRASTRUCTURE:
                    require(obj['HostConfig']['RestartPolicy']['Name'] == 'no', 'held ancillary restart drift')
                if role == 'migrate':
                    require(state['Status'] == 'exited' and type(state['ExitCode']) is int and state['ExitCode'] == 0 and
                            state['FinishedAt'] == anchor['finished_at'], 'completed migration runner drift')
            found[role] = {'cid': cid, 'policy_sha256': policy_projection(obj),
                           'restart_count': obj['RestartCount'], 'epoch': epoch(obj)}
        require(set(found) == set(self.anchor['services']), 'service coverage drift')
        pg = postgres_binding(self.docker)
        runtime.verify_quarantine_barrier_read_only(COMPOSE, ENV, execute=lambda args: self.docker.text(args[1:]))
        database = database_snapshot(pg, self.anchor, self.docker)
        reuse = reuse_precondition(pg, completed, self.docker)
        return {'source': source_identity(), 'containers': found, 'postgres': pg, 'database': database,
                'reuse113': reuse, 'compose_sha256': cold.file_hash(COMPOSE), 'env_sha256': cold.file_hash(ENV),
                'model_sha256': digest(model), 'policy': policy}

    def observation_pause(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.docker.check_budget()
            remaining = end - time.monotonic()
            if self.docker.deadline is not None:
                remaining = min(remaining, self.docker.deadline - time.monotonic())
            time.sleep(min(.1, max(0, remaining)))
        self.docker.check_budget()

    def runtime_observation(self, args):
        # Only Worker starting may converge, same old-entry bound; guard's Docker
        # deadline still bounds every RPC and the subsequent terminal commit.
        if args[:3] == ['docker', 'inspect', '--format'] and args[3:4] == ['{{.State.Health.Status}}']:
            obj = self.docker.inspect('container', args[-1])
            if (obj['Config'].get('Labels') or {}).get('com.docker.compose.service') == 'smartvideo-worker':
                deadline = time.monotonic() + 60
                while obj['State'].get('Health', {}).get('Status') == 'starting':
                    require(obj['State']['Running'] and time.monotonic() < deadline, 'worker startup health deadline')
                    self.observation_pause(min(2, max(0, deadline - time.monotonic())))
                    obj = self.docker.inspect('container', args[-1])
                require(time.monotonic() < deadline, 'worker startup health deadline')
                return obj['State'].get('Health', {}).get('Status', 'unknown')
        return self.docker.text(args[1:])

    def prepare_target(self, snapshot):
        self.docker.text(['pull', '--platform', 'linux/amd64', REF], timeout=300)
        evidence = attest_owned(REF, APP, snapshot['policy'])
        identity = evidence['identity']
        verify_prepared_capability(evidence, REF, APP, snapshot['policy'], identity)
        inspect_identity(self.docker, identity)
        return evidence


def protected_snapshot(snapshot):
    result = copy.deepcopy(snapshot)
    for role in ROLES:
        result['containers'].pop(role)
    return result


def check_candidate(candidate, hold, source):
    require(set(candidate) == {'version', 'purpose', 'nonce', 'not_before', 'expires_at', 'hold_sha256',
                              'snapshot', 'ancestry', 'runtime_capability', 'operations', 'new_names'}, 'strict forward candidate schema')
    require(type(candidate['version']) is int and candidate['version'] == 1 and
            candidate['purpose'] == DOMAIN + '-proof' and candidate['operations'] == OPERATIONS and
            type(candidate['nonce']) is str and re.fullmatch('[0-9a-f]{32}', candidate['nonce']), 'fixed forward domain')
    r.check_window(candidate)
    require(candidate['hold_sha256'] == digest(hold) and candidate['snapshot']['source'] == source and
            candidate['ancestry'] == history_binding(hold) and
            candidate['nonce'] not in hold.get('forward_spent_nonces', []) and
            candidate['new_names'] == predicted_names(candidate['nonce']), 'forward source/hold/session/replay drift')
    verify_prepared_capability(candidate['runtime_capability'], REF, APP, candidate['snapshot']['policy'],
                               candidate['runtime_capability']['identity'])


def predicted_names(nonce):
    return {role: PROJECT + '-forward-' + nonce + '-' + role for role in ROLES}


def phrase(candidate):
    return ('APPROVE FIXED FORWARD API_WORKER_ONLY candidate=' + digest(candidate) +
            ' hold=' + candidate['hold_sha256'] + ' controller=' + candidate['snapshot']['source']['controller_source_sha'] +
            ' old_app=' + OLD_APP + ' old_digest=' + OLD_DIGEST + ' new_app=' + APP + ' new_digest=' + DIGEST +
            ' STOP_REMOVE_RECREATE_PAIR RETAIN_HOLD ANCILLARY_UNCHANGED NO_MIGRATION NO_REENROLLMENT NO_ROLLBACK SINGLE_USE')


def approve(candidate, hold, source, operator, confirmation):
    check_candidate(candidate, hold, source)
    require(type(operator) is str and re.fullmatch('[A-Za-z0-9_.@-]{3,80}', operator) and
            confirmation == phrase(candidate), 'exact human/operator confirmation required')
    now = int(time.time())
    return cold.signed({'version': 1, 'purpose': DOMAIN + '-human-approval', 'candidate_sha256': digest(candidate),
                        'hold_sha256': digest(hold), 'nonce': candidate['nonce'], 'operator': operator,
                        'confirmation': confirmation, 'not_before': now,
                        'expires_at': min(now + 300, candidate['expires_at']), 'approved_at': now})


def check_approval(signed, candidate, hold, source):
    check_candidate(candidate, hold, source)
    human = authenticate(signed)
    require(set(human) == {'version', 'purpose', 'candidate_sha256', 'hold_sha256', 'nonce', 'operator',
                          'confirmation', 'not_before', 'expires_at', 'approved_at'}, 'strict forward human schema')
    r.check_window(human)
    require(type(human['version']) is int and human['version'] == 1 and human['purpose'] == DOMAIN + '-human-approval' and
            human['candidate_sha256'] == digest(candidate) and human['hold_sha256'] == digest(hold) and
            human['nonce'] == candidate['nonce'] and human['confirmation'] == phrase(candidate) and
            type(human['operator']) is str and re.fullmatch('[A-Za-z0-9_.@-]{3,80}', human['operator']) and
            type(human['approved_at']) is int and human['approved_at'] == human['not_before'] and
            candidate['not_before'] <= human['not_before'] < human['expires_at'] <= candidate['expires_at'] and
            human['expires_at'] - human['not_before'] <= 300, 'forward human binding mismatch')
    return human


def new_model(docker, candidate):
    model, policy = cap.rendered_policy(docker, COMPOSE, ENV, REF)
    require(policy == candidate['snapshot']['policy'] and model['name'] == PROJECT, 'new normal PID1 policy drift')
    model['services'] = {role: model['services'][role] for role in ROLES}
    for role, cfg in model['services'].items():
        cfg.pop('build', None)
        cfg.pop('depends_on', None)
        cfg['image'], cfg['restart'], cfg['container_name'] = REF, 'no', candidate['new_names'][role]
        cfg['labels'] = dict(cfg.get('labels') or {}, **{LABEL: candidate['nonce']})
    # Native Compose must neither create nor change any ancillary network/volume.
    for kind in ('networks', 'volumes'):
        for key, cfg in model.get(kind, {}).items():
            name = cfg['name']
            obj = docker.inspect('network' if kind == 'networks' else 'volume', name)
            require((obj.get('Labels') or {}).get('com.docker.compose.project') == PROJECT and
                    (obj.get('Labels') or {}).get('com.docker.compose.' + kind[:-1]) == key,
                    'foreign ancillary resource owner')
            model[kind][key] = {'name': name, 'external': True}
    return model


def compose_input(model):
    # Normalized literal dollars must not be re-interpolated by Compose.
    if type(model) is str:
        return model.replace('$', '$$')
    if type(model) is list:
        return [compose_input(v) for v in model]
    if type(model) is dict:
        return {k: compose_input(v) for k, v in model.items()}
    return model


class Guard:
    """One ephemeral child owns ALL runtime writes, final checks and outcomes."""
    def __init__(self, candidate, approval, hold, backend, lock, parent_fd, deadline):
        self.candidate, self.approval, self.original = candidate, approval, hold
        self.backend, self.docker, self.lock = backend, backend.docker, lock
        self.parent_fd, self.deadline = parent_fd, deadline
        self.nonce = candidate['nonce']
        self.new = {}
        self.removed = []
        self.uncertain = False
        self.armed = False
        self.identity = candidate['runtime_capability']['identity']
        self.old = {role: candidate['snapshot']['containers'][role]['cid'] for role in ROLES}

    def check_owner(self):
        self.lock.check()
        hold = read_success_hold()
        attempt = hold.get('forward_attempt', {})
        require(attempt.get('owner') == self.lock.owner and attempt.get('nonce') == self.nonce and
                attempt.get('candidate_sha256') == digest(self.candidate) and
                attempt.get('approval_sha256') == digest(self.approval) and
                hold.get('forward_original_success') == cold.signed(self.original) and
                attempt.get('old_cids') == self.old and attempt.get('new_cids') == self.new and
                attempt.get('removed_old_cids') == self.removed and
                all(hold.get(k) == self.original.get(k) for k in
                    ('owner', 'project', 'release_sha', 'proof_sha256', 'proof_nonce', 'image_ids')),
                'lost/substituted forward attempt')
        return hold

    def check_live(self):
        self.check_owner()
        r.check_window(self.candidate)
        r.check_window(authenticate(self.approval))
        require(time.monotonic() < self.deadline, 'fixed monotonic cutover deadline')
        ready, _, _ = select.select([self.parent_fd], [], [], 0)
        require(not ready, 'controller EOF or unexpected request')

    def write_attempt(self, new=None, removed=None):
        hold = self.check_owner()
        if new is not None:
            self.new = dict(new)
        if removed is not None:
            self.removed = list(removed)
        hold['forward_attempt']['new_cids'] = dict(self.new)
        hold['forward_attempt']['removed_old_cids'] = list(self.removed)
        cold.write_hold(DIR, hold)

    def mutate(self, operation, args, target):
        self.check_live()
        hold = self.check_owner()
        require(not hold.get('forward_indeterminate_operation'), 'unsettled operation cannot replay')
        marker = {'nonce': self.nonce, 'operation': operation, 'target': target}
        hold['forward_indeterminate_operation'] = marker
        cold.write_hold(DIR, hold)  # fsynced reservation BEFORE Engine request
        self.check_live()
        remaining = self.deadline - time.monotonic()
        require(remaining > 0, 'no nonpositive mutation timeout')
        try:
            result = self.docker.text(args, timeout=min(60, remaining))
        except BaseException:
            self.uncertain = True
            raise
        # An acknowledged operation may still fail observation. Keep reservation
        # until exact resulting identity/status is durably bound by the caller.
        return result

    def settled(self):
        hold = self.check_owner()
        hold.pop('forward_indeterminate_operation', None)
        cold.write_hold(DIR, hold)
        self.check_live()

    def inventory(self):
        found = {}
        for cid in self.docker.text(['ps', '-aq', '--no-trunc']).splitlines():
            obj = self.docker.inspect('container', cid)
            labels = obj['Config'].get('Labels') or {}
            role = labels.get('com.docker.compose.service')
            if labels.get('com.docker.compose.project') == PROJECT or obj['Image'] in (OLD_IMAGE, r.OLD_IMAGE, self.identity['local_image_id']):
                require(labels.get('com.docker.compose.project') == PROJECT and
                        role in self.backend.anchor['services'] and role not in found, 'foreign/extraneous runtime owner')
                expected = self.new.get(role) if role in self.new else self.old.get(role)
                if role not in ROLES:
                    expected = self.backend.anchor['services'][role]['cid']
                require(expected == cid == obj['Id'], 'runtime CID ownership drift')
                found[role] = obj
            require(obj.get('Name', '').lstrip('/') not in self.candidate['new_names'].values() or cid in self.new.values(),
                    'predicted name occupied by foreign owner')
        for role in self.backend.anchor['services']:
            if role not in ROLES:
                before = self.candidate['snapshot']['containers'][role]
                obj = found.get(role)
                require(obj and {'cid': obj['Id'], 'policy_sha256': policy_projection(obj),
                                 'restart_count': obj['RestartCount'], 'epoch': epoch(obj)} == before,
                        'ancillary runtime drift')
        return found

    def perform(self):
        self.docker.deadline = self.deadline
        self.docker.parent_fd = self.parent_fd
        human = check_approval(self.approval, self.candidate, self.original, source_identity())
        require(read_success_hold() == self.original and
                self.backend.snapshot(self.candidate['ancestry']) == self.candidate['snapshot'] and
                read_success_hold() == self.original, 'immediate full hold/snapshot mismatch')
        self.lock.check()
        require(time.monotonic() < self.deadline, 'expired before nonce consumption')
        hold = copy.deepcopy(self.original)
        hold['forward_original_success'] = cold.signed(self.original)
        hold.setdefault('forward_spent_nonces', []).append(self.nonce)
        hold.update(state=STARTING, forward_attempt={'owner': self.lock.owner, 'nonce': self.nonce,
                    'candidate_sha256': digest(self.candidate), 'approval_sha256': digest(self.approval),
                    'operator': human['operator'], 'old_cids': self.old, 'new_cids': {}, 'removed_old_cids': []})
        cold.write_hold(DIR, hold)
        self.armed = True
        self.docker.deadline = self.deadline
        self.docker.parent_fd = self.parent_fd
        self.check_live()
        self.model = new_model(self.docker, self.candidate)
        self.config_path = os.path.join(WORK, 'forward-compose-' + self.nonce + '.json')
        require(not os.path.lexists(self.config_path), 'private config cannot replay')
        private_json(self.config_path, compose_input(self.model))
        hold = self.check_owner()
        hold['forward_attempt']['private_config_sha256'] = cold.file_hash(self.config_path)
        hold['forward_attempt']['old_preimages'] = {
            role: {'cid': obj['Id'], 'name': obj['Name'], 'image': obj['Image'],
                   'policy_sha256': policy_projection(obj), 'epoch': epoch(obj)}
            for role, obj in self.inventory().items() if role in ROLES}
        cold.write_hold(DIR, hold)
        # Complete fresh equality remains immediately adjacent to the first stop.
        require(self.backend.snapshot(self.candidate['ancestry']) == self.candidate['snapshot'], 'pre-cutover drift')
        for role in ROLES:
            self.check_live()
            obj = self.inventory()[role]
            require(policy_projection(obj) == self.candidate['snapshot']['containers'][role]['policy_sha256'], 'old policy substituted')
            self.mutate('stop-old', ['stop', '--time', '20', obj['Id']], obj['Id'])
            stopped = self.docker.inspect('container', obj['Id'])
            require(not stopped['State']['Running'] and stopped['State']['Status'] == 'exited' and
                    not stopped['State']['Paused'] and not stopped['State']['Restarting'] and
                    stopped['State']['ExitCode'] == 0 and policy_projection(stopped) == policy_projection(obj),
                    'old stop not verified clean')
            self.settled()
        for role in ROLES:
            obj = self.inventory()[role]
            require(not obj['State']['Running'] and obj['State']['ExitCode'] == 0, 'old rm requires clean stopped')
            self.mutate('remove-old', ['rm', obj['Id']], obj['Id'])  # no force, no volumes
            self.write_attempt(removed=self.removed + [obj['Id']])
            self.settled()
        for role in ROLES:
            self.inventory()
            require(cold.file_hash(self.config_path) == self.check_owner()['forward_attempt']['private_config_sha256'], 'private config drift')
            args = ['compose', '-p', PROJECT, '-f', self.config_path, '--env-file', ENV,
                    'up', '--no-start', '--no-deps', '--no-recreate', '--no-build', '--pull', 'never', role]
            self.mutate('create-new', args, self.candidate['new_names'][role])
            obj = self.docker.inspect('container', self.candidate['new_names'][role])
            labels = obj['Config'].get('Labels') or {}
            require(re.fullmatch('[0-9a-f]{64}', obj['Id']) and labels.get(LABEL) == self.nonce and
                    labels.get('com.docker.compose.project') == PROJECT and labels.get('com.docker.compose.service') == role and
                    obj['Name'].lstrip('/') == self.candidate['new_names'][role] and
                    obj['State']['Status'] == 'created' and not obj['State']['Running'], 'create ownership/identity drift')
            verify_process(self.docker, obj, self.model['services'][role], self.candidate['snapshot']['policy']['roles'][role], self.identity, self.model)
            new = dict(self.new)
            new[role] = obj['Id']
            self.write_attempt(new=new)  # exact full CID durable BEFORE start
            self.settled()
        for role in ROLES:
            self.inventory()
            obj = self.docker.inspect('container', self.new[role])
            verify_process(self.docker, obj, self.model['services'][role], self.candidate['snapshot']['policy']['roles'][role], self.identity, self.model)
            self.mutate('start-new', ['start', self.new[role]], self.new[role])
            self.settled()
        health_deadline = min(self.deadline, time.monotonic() + 60)
        while True:
            self.check_live()
            require(time.monotonic() < health_deadline, 'new pair health deadline')
            states = [self.docker.inspect('container', cid)['State'] for cid in self.new.values()]
            require(all(s['Running'] and not s['Paused'] and not s['Restarting'] and
                        s.get('Health', {}).get('Status') in ('starting', 'healthy') for s in states),
                    'new pair health failed')
            if all(s['Health']['Status'] == 'healthy' for s in states):
                require(time.monotonic() < health_deadline, 'new pair health deadline')
                break
            time.sleep(min(2, max(0, health_deadline - time.monotonic())))
        replacement = {'identity': self.identity, 'cids': self.new, 'model': self.model}
        post = self.backend.snapshot(self.candidate['ancestry'], completed=True, replacement=replacement)
        require(protected_snapshot(post) == protected_snapshot(self.candidate['snapshot']), 'final protected equality drift')
        self.inventory()
        self.check_live()
        verify_prepared_capability(self.candidate['runtime_capability'], REF, APP,
                                   self.candidate['snapshot']['policy'], self.identity)
        require(source_identity() == self.candidate['snapshot']['source'], 'commit source drift')
        hold = self.check_owner()
        require(not hold.get('forward_indeterminate_operation'), 'cannot commit indeterminate mutation')
        hold.update(state=SUCCESS, forward_verified_cids=dict(self.new), forward_identity=self.identity,
                    forward_no_env_persistence=True, forward_verified_at=int(time.time()), forward_final_snapshot=post)
        cold.write_hold(DIR, hold)
        self.check_live()  # if commit consumed deadline/EOF, sole owner fences

    def fence(self):
        if not self.armed:
            return  # No container mutation was authorized/performed.
        hold = self.check_owner()  # ownership loss REFUSES every cleanup mutation
        unresolved = hold.get('forward_indeterminate_operation')
        self.uncertain = self.uncertain or bool(unresolved)
        self.docker.parent_fd = None  # EOF triggers fencing, not refusal of cleanup
        self.docker.deadline = time.monotonic() + 60  # stop-only cleanup, never start authority
        failed = False
        # Only exact bound CIDs, never global cold.fence/ancillary/foreign owners.
        for role, cid in list(self.new.items()) + list(self.old.items()):
            if cid in self.removed:
                continue
            if unresolved and unresolved.get('target') == cid:
                # Never resend or sample an unsettled target into settlement.
                continue
            try:
                self.check_owner()
                obj = self.docker.inspect('container', cid)
                labels = obj['Config'].get('Labels') or {}
                require(obj['Id'] == cid and labels.get('com.docker.compose.project') == PROJECT and
                        labels.get('com.docker.compose.service') == role and
                        (labels.get(LABEL) == self.nonce and obj['Image'] == self.identity['local_image_id']
                         if cid in self.new.values() else obj['Image'] == OLD_IMAGE and
                         policy_projection(obj) == self.candidate['snapshot']['containers'][role]['policy_sha256']),
                        'foreign fence target')
                if obj['State']['Running'] or obj['State']['Paused'] or obj['State']['Restarting']:
                    require(not obj['State']['Paused'] and not obj['State']['Restarting'], 'unfenceable target activity')
                    hold = self.check_owner()
                    hold['forward_fence_indeterminate'] = {'cid': cid, 'nonce': self.nonce}
                    cold.write_hold(DIR, hold)
                    try:
                        self.docker.text(['stop', '--time', '20', cid], timeout=25)
                    except BaseException:
                        self.uncertain = True
                        raise
                    hold = self.check_owner()
                    hold.pop('forward_fence_indeterminate', None)
                    cold.write_hold(DIR, hold)
                obj = self.docker.inspect('container', cid)
                require(not obj['State']['Running'] and not obj['State']['Paused'] and
                        not obj['State']['Restarting'] and obj['State']['Status'] in ('exited', 'created'), 'scoped fence incomplete')
            except Exception:
                failed = True
        hold = self.check_owner()
        unknown = self.uncertain or failed or bool(hold.get('forward_fence_indeterminate'))
        hold['state'] = 'RECOVERY_REQUIRED_UNKNOWN' if unknown else r.STOPPED
        hold.setdefault('errors', []).append('fixed-forward-outcome-unknown' if unknown else 'fixed-forward-fenced')
        cold.write_hold(DIR, hold)
        if unknown:
            self.lock.retained = True
        else:
            self.lock.retained = False
            self.lock.release()


def execute(candidate, approval, backend, lock):
    require(hasattr(os, 'fork'), 'Linux sole mutation guard required')
    hold = read_success_hold()
    human = check_approval(approval, candidate, hold, source_identity())
    request_r, request_w = os.pipe()
    response_r, response_w = os.pipe()
    # Retain until acknowledged successful child settlement, including arm death.
    lock.retained = True
    deadline = time.monotonic() + min(300, human['expires_at'] - int(time.time()))
    require(time.monotonic() < deadline, 'no stale guard budget')
    pid = os.fork()
    if pid == 0:
        code = 1
        guard = None
        try:
            os.close(request_w)
            os.close(response_r)
            signal.signal(signal.SIGTERM, signal.SIG_IGN)
            signal.signal(signal.SIGINT, signal.SIG_IGN)
            with open(os.path.join(lock.path, 'owner_pid'), 'w') as stream:
                stream.write(str(os.getpid()) + '\n')
                stream.flush()
                os.fsync(stream.fileno())
            guard = Guard(candidate, approval, hold, backend, lock, request_r, deadline)
            guard.perform()
            os.write(response_w, b'O')
            lock.retained = False
            lock.release()
            code = 0
        except BaseException:
            try:
                if guard is not None:
                    guard.fence()
                    if not guard.armed:
                        lock.retained = False
                        lock.release()
            except BaseException:
                pass  # UNKNOWN reservation/live/dead lock is never reclaimed.
        finally:
            os._exit(code)
    os.close(request_r)
    os.close(response_w)
    def terminate(signum, frame):
        os.close(request_w)
        raise Refused('controller terminated')
    previous = {sig: signal.signal(sig, terminate) for sig in (signal.SIGTERM, signal.SIGINT)}
    try:
        ready, _, _ = select.select([response_r], [], [], max(0, deadline + 65 - time.monotonic()))
        acknowledged = ready and os.read(response_r, 1) == b'O'
        settlement = deadline + 66
        status = None
        while time.monotonic() < settlement:
            finished, value = os.waitpid(pid, os.WNOHANG)
            if finished:
                status = value
                break
            time.sleep(min(.1, max(0, settlement - time.monotonic())))
        # Parent never mutates or waits indefinitely; unresolved child retains lock.
        require(acknowledged and status == 0 and read_success_hold()['state'] == SUCCESS, 'guard fenced or indeterminate')
        lock.retained = False
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)
        for fd in (request_w, response_r):
            try:
                os.close(fd)
            except OSError:
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('prepare', 'approve', 'execute'))
    parser.add_argument('--operator')
    parser.add_argument('--confirmation')
    args = parser.parse_args()
    require(sys.platform == 'linux' and os.geteuid() == 0, 'root-owned native Linux entry required')
    os.chdir(ROOT)
    require(not os.environ.get('RELEASE_TRUST_SECRET'), 'existing private file trust key required')
    key = r.protected_path(os.path.join(DIR, 'release-trust.key'), secret=True)
    os.environ['RELEASE_TRUST_KEY_FILE'] = key
    for name in list(os.environ):
        if name.upper().startswith(('DOCKER_', 'COMPOSE_', 'PG', 'XIANZHI_', 'QUARANTINE_')):
            os.environ.pop(name)
    for path in (ROOT, DIR, WORK, os.path.join(WORK, 'state.json'), COMPOSE, ENV, MANIFEST):
        r.protected_path(path)
    source = source_identity()
    hold = read_success_hold()
    ancestry = history_binding(hold)
    lock = r.ReleaseLock()
    try:
        if args.action == 'prepare':
            require(not args.operator and not args.confirmation, 'prepare cannot auto-approve')
            backend = Backend()
            snapshot = backend.snapshot(ancestry)
            evidence = backend.prepare_target(snapshot)
            require(backend.snapshot(ancestry) == snapshot and read_success_hold() == hold, 'prepare hold/snapshot drift')
            now, nonce = int(time.time()), uuid.uuid4().hex
            candidate = {'version': 1, 'purpose': DOMAIN + '-proof', 'nonce': nonce, 'not_before': now,
                         'expires_at': now + 900, 'hold_sha256': digest(hold), 'snapshot': snapshot,
                         'ancestry': ancestry, 'runtime_capability': evidence, 'operations': OPERATIONS,
                         'new_names': predicted_names(nonce)}
            private_json(CANDIDATE, cold.signed(candidate))
            print(phrase(candidate))
        else:
            r.protected_path(CANDIDATE, secret=True)
            candidate = authenticate(r.read(CANDIDATE))
            check_candidate(candidate, hold, source)
            if args.action == 'approve':
                private_json(APPROVAL, approve(candidate, hold, source, args.operator, args.confirmation))
                print('FORWARD_HUMAN_APPROVAL_SIGNED; no cutover performed')
            else:
                require(not args.operator and not args.confirmation, 'execute cannot sign')
                r.protected_path(APPROVAL, secret=True)
                execute(candidate, r.read(APPROVAL), Backend(), lock)
                print(SUCCESS + '; ordinary operations remain blocked')
    finally:
        if os.path.exists(lock.path):
            lock.release()


def entry():
    try:
        main()
    except BaseException:
        sys.stderr.write('FIXED_FORWARD_REFUSED: hold retained; inspect authenticated state/owned lock offline\n')
        sys.exit(1)


if __name__ == '__main__':
    entry()
