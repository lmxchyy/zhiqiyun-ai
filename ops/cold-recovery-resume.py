#!/usr/bin/env python3
"""Bounded recovery of the release213 stopped hold; Python 3.6.

No ordinary release authority, migration, enrollment, env persistence or
ancillary starts. CLI paths and application identity are deliberately fixed.
"""
import argparse
import datetime
import hashlib
import hmac
import json
import os
import re
import select
import shutil
import signal
import stat
import subprocess
import sys
import time
import types
import uuid

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
APP = 'b749a9ad1e543ac2f0b272aaac1db9b56065c85b'
DIGEST = 'sha256:6ca5046ac2bd53b4ee7e5b96bab5bb04cd8d308d8a292d4fd2004bc75609390b'
IMAGE = 'sha256:3955f86a32d12e68e478c70536607455546885fa07f730a6f5c9089cd0fbf2da'
OLD_PROOF = '5ab75574f57558759241dc13c64401033d2f5caaef02e91d641750f4be13d401'
OLD_IMAGE = 'sha256:5c8de32f7d5af05a05afabae690a903e2443cf1b6ec5a5d5e31d8074316acfc2'
REF = 'crpi-kvtl7houjvj5ftba.cn-hangzhou.personal.cr.aliyuncs.com/zhiqiyun-ai/zhiqiyun-ai@' + DIGEST
ROLES = ('xianzhi-ai', 'smartvideo-worker')
STOPPED = 'STOPPED_RECOVERY_REQUIRED'
STARTING = 'RECOVERY_STARTING'
SUCCESS = 'RECOVERY_API_WORKER_VERIFIED_ANCILLARY_HELD'
DOMAIN = 'cold-recovery-resume'
DIR = os.path.join(ROOT, '.prestage')
PROJECT = 'zhiqiyun-ai-prod'
ANCHOR_PATH = os.path.join(ROOT, 'ops/cold-recovery-anchor.json')
COMPOSE = os.path.join(ROOT, 'compose.prod.yml')
ENV = os.path.join(ROOT, '.env.production')
PROOF_PATH = os.path.join(DIR, APP, 'prestage-proof.json')
WORK = os.path.join(DIR, 'cold-recovery-required')
CANDIDATE = os.path.join(WORK, 'recovery-candidate.json')
APPROVAL = os.path.join(WORK, 'recovery-approval.json')
FROZEN = {'rollback.sh': 'b9fba73deee7e7d5135e9a7d6d951afe0e294eb1947b063f9d735d827b406184',
          'tests/issue203-control-plane-test.py': 'b3a799bf0fde1496f7362158d6fcfe5df1ca6cbaa0b5338dc5a187c9efce3b54'}


def load(name, filename):
    path = os.path.join(ROOT, 'ops', filename)
    module = types.ModuleType(name)
    module.__file__ = path
    with open(path, 'rb') as stream:
        exec(compile(stream.read(), path, 'exec'), module.__dict__)
    return module


cold = load('resume_cold', 'first-upgrade-cold.py')
cap = cold.capability
transport = load('resume_transport', 'quarantine-psql-transport.py')
runtime = load('resume_runtime', 'verify-release-runtime.py')
Refused = cap.Refused
prepare_diagnostics = load('resume_prepare_diagnostics', 'cold-recovery-prepare-diagnostics.py')
_prepare_diagnostic = None
_prepare_active = False


def prepare_observe(stage, **values):
    if _prepare_diagnostic is not None:
        try:
            _prepare_diagnostic.observe(stage, **values)
        except BaseException:
            pass


def prepare_snapshot_equal(post, initial):
    prepare_observe('snapshot_equality', post_snapshot_returned=True)
    equal = post == initial  # Original strict comparison; no normalization.
    prepare_observe('snapshot_equality', snapshot_equal=equal)
    if _prepare_diagnostic is not None:
        try:
            _prepare_diagnostic.snapshots(initial, post)
        except BaseException:
            pass
    if equal:
        prepare_observe('hold_equality')
    return equal


def prepare_hold_equal(equal):
    prepare_observe('hold_equality', hold_equal=equal)
    return equal


def require(condition, reason):
    if not condition:
        raise Refused(reason)


def unique(pairs):
    result = {}
    for name, value in pairs:
        require(name not in result, 'duplicate JSON field')
        result[name] = value
    return result


def read(path):
    with open(path, encoding='utf-8') as stream:
        return json.load(stream, object_pairs_hook=unique,
                         parse_constant=lambda _: require(False, 'nonfinite JSON'))


def digest(value):
    return cap.sha(cap.canonical(value).encode('utf-8'))


def authenticate(value):
    require(type(value) is dict and type(value.get('signature')) is str, 'signature required')
    payload = dict(value)
    signature = payload.pop('signature')
    require(hmac.compare_digest(signature, cold.signed(payload)['signature']), 'authentication failed')
    return payload


def protected_path(path, secret=False):
    """No symlink ancestors or writable non-root files in the signing ceremony."""
    absolute = os.path.abspath(path)
    current = absolute
    while True:
        info = os.lstat(current)
        require(not stat.S_ISLNK(info.st_mode), 'symlink path refused')
        require(info.st_uid == 0 and not info.st_mode & 0o022, 'root-owned protected path required')
        if current == absolute and secret:
            require(stat.S_ISREG(info.st_mode) and not info.st_mode & 0o077, 'private key mode required')
        parent = os.path.dirname(current)
        if current == parent:
            break
        current = parent
    return absolute


def source_identity():
    def git(*args):
        return subprocess.check_output(['git'] + list(args), cwd=ROOT).decode().strip()
    require(not git('status', '--porcelain', '--untracked-files=all'), 'clean controller required')
    revision = git('rev-parse', 'HEAD')
    require(re.fullmatch('[0-9a-f]{40}', revision) and revision != APP, 'distinct committed controller required')
    paths = git('ls-files').splitlines()
    selected = [p for p in paths if p.startswith(('ops/', 'database/', 'backend-go/')) or p in FROZEN or p in ('compose.prod.yml', 'deploy.sh', '.github/workflows/cold-recovery-resume.yml')]
    hashes = {p: cold.file_hash(os.path.join(ROOT, p)) for p in selected}
    require(all(hashes.get(p) == expected for p, expected in FROZEN.items()), 'frozen source changed')
    # Fixture schema/migrations must describe fixed application, not controller HEAD.
    for path in paths:
        if path.startswith('database/') and path.endswith('.sql'):
            old = subprocess.check_output(['git', 'show', APP + ':' + path], cwd=ROOT)
            require(cap.sha(old) == cold.file_hash(os.path.join(ROOT, path)), 'app SQL drift')
    return {'controller_source_sha': revision, 'helper_sha256': hashes, 'app_source_sha': APP}


class ReleaseLock:
    """Same lock namespace; never reclaim an indeterminate/dead attempt lock."""
    def __init__(self, directory=DIR):
        self.path = os.path.join(directory, 'release.lock')
        require(not os.path.lexists(os.path.join(directory, 'release.lock.recovering')), 'lock recovery active')
        os.mkdir(self.path, 0o700)
        self.owner = 'cold-resume-' + uuid.uuid4().hex
        for name, value in (('owner_token', self.owner), ('owner_pid', str(os.getpid())), ('created_at', str(int(time.time())))):
            with open(os.path.join(self.path, name), 'x') as stream:
                stream.write(value + '\n')
        self.retained = False

    def check(self):
        with open(os.path.join(self.path, 'owner_token')) as stream:
            require(stream.read().strip() == self.owner, 'foreign lock owner')

    def release(self):
        self.check()
        if not self.retained:
            shutil.rmtree(self.path)


# Recovery-only descriptors. The ordinary 103 SQL descriptors remain unchanged.
QUARANTINE = ('SELECT execution_id,task_id,attempt,generation,snapshot_sha256,evidence_sha256,'
              'approval_id,release_sha,not_before,expires_at,created_at '
              'FROM public.provider_execution_quarantine ORDER BY execution_id')
MIGRATIONS = 'SELECT filename,applied_at FROM public.schema_migrations ORDER BY filename'
CTE = "WITH scope AS (SELECT task_id FROM public.provider_executions WHERE id=ANY(ARRAY[1,10,12,14,22,25,31,100,102,103]::bigint[]) UNION SELECT id FROM public.xz_generation_tasks WHERE id IN ('task_000313','task_000322','task_000326','task_000331','task_000332','task_000333')) "
PREDICATES = [('provider_executions', 'task_id IN (SELECT task_id FROM scope)'),
              ('xz_generation_tasks', 'id IN (SELECT task_id FROM scope)'),
              ('xz_personal_point_reservations', 'business_id IN (SELECT task_id FROM scope)'),
              ('xz_wallet_ledger', 'task_id IN (SELECT task_id FROM scope) OR reference_id IN (SELECT task_id FROM scope)'),
              ('xz_billing_lifecycle_events', 'task_id IN (SELECT task_id FROM scope)'),
              ('xz_billing_events', 'task_id IN (SELECT task_id FROM scope)'),
              ('xz_assets', 'task_id IN (SELECT task_id FROM scope)')]
HISTORY = [CTE + "SELECT '" + table + "|'||count(*)||'|'||md5(coalesce(string_agg(row_to_json(t)::text,'|' ORDER BY id::text),'')) FROM public." + table + ' t WHERE ' + predicate for table, predicate in PREDICATES]
DESCRIPTORS = {QUARANTINE: (20, 25, 23, 20, 1042, 1042, 25, 1042, 1184, 1184, 1184),
               MIGRATIONS: (25, 1184)}
DESCRIPTORS.update({query: (25,) for query in HISTORY})
SETTINGS = ("SET application_name = 'issue203-quarantine-transport'",
            'BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY', 'ROLLBACK',
            "SET LOCAL search_path = pg_catalog, public", "SET LOCAL TIME ZONE 'UTC'",
            "SET LOCAL DateStyle = 'ISO, YMD'", 'SET LOCAL extra_float_digits = 3')


class ReadOnlyCursor(transport.Cursor):
    def execute(self, sql, parameters=()):
        conn = self.connection
        try:
            require(not parameters and sql in tuple(DESCRIPTORS) + SETTINGS, 'non-recovery SQL refused')
            require(not conn.closed and conn.process.poll() is None, 'read-only backend unavailable')
            token = uuid.uuid4().hex
            expected = DESCRIPTORS.get(sql, ())
            if expected:
                require(conn.status == 2, 'read-only transaction required')
                statement = transport.select_statement(sql, token, expected)
            else:
                statement = sql + ";\nSELECT json_build_object('token','" + token + "','count',0,'columns',0,'types','[]'::json,'rows','[]'::json);\n"
            conn.process.stdin.write((statement + '\\echo END_' + token + '\n').encode())
            conn.process.stdin.flush()
            raw = conn.messages.get(timeout=transport.TIMEOUT)
            self.rows = transport.decode_frame(raw, token, expected)
            require(conn.messages.get(timeout=transport.TIMEOUT).strip() == ('END_' + token).encode(), 'native frame incomplete')
            require(conn.process.poll() is None, 'read-only backend exited')
            if sql.startswith('BEGIN '):
                conn.status = 2
            elif sql == 'ROLLBACK':
                conn.status = 0
        except Exception:
            conn.close()
            raise Refused('recovery read-only transport refused')


class ReadOnlyConnection(transport.Connection):
    def cursor(self):
        require(not self.closed, 'connection closed')
        return ReadOnlyCursor(self)


class ReadOnlyTarget:
    def __init__(self, expected):
        self.expected = expected
        self.check()

    def check(self):
        require(transport.binding(COMPOSE, ENV, REF) == self.expected, 'PostgreSQL identity drift')


def database_snapshot(expected, anchor):
    target = ReadOnlyTarget(expected)
    conn = ReadOnlyConnection(target)
    try:
        cursor = conn.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
        for setting in SETTINGS[3:]:
            cursor.execute(setting)
        cursor.execute(QUARANTINE)
        rows = cursor.fetchall()
        fields = QUARANTINE[len('SELECT '):].split(' FROM ')[0].split(',')
        enrolled = []
        for row in rows:
            enrolled.append({k: v.isoformat() if type(v) is datetime.datetime else v for k, v in zip(fields, row)})
        require(enrolled == anchor['quarantine_records'], 'exact9 full records drift')
        cursor.execute(MIGRATIONS)
        migration = cursor.fetchall()
        names = [r[0] for r in migration]
        required = sorted(p for p in os.listdir(os.path.join(ROOT, 'database/migrations'))
                          if re.match(r'^\d{3}-.*\.sql$', p) and not p.endswith('.down.sql') and 108 <= int(p[:3]) <= 121)
        require(names == required and required[-1] == '121-provider-execution-quarantine.sql', 'migration baseline/completion drift')
        history = []
        for query in HISTORY:
            cursor.execute(query)
            result = cursor.fetchall()
            require(len(result) == 1, 'history singleton required')
            history.append(result[0][0])
        require(history == anchor['history'], 'protected business/financial drift')
        cursor.execute('ROLLBACK')
        target.check()
        return {'quarantine_records': enrolled, 'history': history,
                'migrations': [[n, t.isoformat()] for n, t in migration]}
    finally:
        conn.close()


def read_stopped_hold(directory=DIR):
    path = os.path.join(directory, cold.HOLD, 'state.json')
    hold = authenticate(read(path))
    require(type(hold.get('version')) is int and hold['version'] == 1 and hold.get('state') == STOPPED,
            'only legal authenticated stopped hold')
    require(hold.get('release_sha') == APP and hold.get('proof_sha256') == OLD_PROOF and
            hold.get('project') == PROJECT and type(hold.get('owner')) is str and hold['owner'] and
            type(hold.get('image_ids')) is list and len(hold['image_ids']) == 2 and
            hold['image_ids'] == [IMAGE, OLD_IMAGE] and
            type(hold.get('proof_nonce')) is str and re.fullmatch('[0-9a-f-]{36}', hold['proof_nonce']),
            'fixed hold identity required')
    return hold


def verify_container_config(docker, model, containers, policy):
    image = docker.inspect('image', IMAGE)
    for service in ROLES:
        obj = docker.inspect('container', containers[service]['cid'])
        cfg, expected = model['services'][service], policy['roles'][service]
        env = dict(x.split('=', 1) for x in image['Config'].get('Env', []))
        env.update({k: str(v) for k, v in (cfg.get('environment') or {}).items()})
        require(obj['Config']['Cmd'] == expected['command'] and
                (obj['Config'].get('Entrypoint') or []) == expected['entrypoint'] and
                dict(x.split('=', 1) for x in obj['Config'].get('Env', [])) == env and
                obj['Config'].get('User', '') == (expected['user'] or image['Config'].get('User', '')),
                'target effective process/env mismatch')
        mounts = {}
        for mount in cfg.get('volumes') or []:
            require(type(mount) is dict, 'normalized volume required')
            source = mount['source']
            if mount['type'] == 'volume':
                source = model.get('volumes', {}).get(source, {}).get('name', source)
            mounts[mount['target']] = (mount['type'], source, not mount.get('read_only', False))
        actual = {m['Destination']: (m['Type'], m.get('Name') if m['Type'] == 'volume' else m['Source'], m['RW']) for m in obj.get('Mounts', [])}
        require(actual == mounts, 'target effective mounts mismatch')


def observe(docker, anchor, running=()):
    hold = authenticate(read(os.path.join(WORK, 'state.json')))
    require(hold['release_sha'] == APP and hold['proof_sha256'] == OLD_PROOF, 'hold identity drift')
    found = {}
    for cid in docker.text(['ps', '-aq', '--no-trunc']).splitlines():
        obj = docker.inspect('container', cid)
        labels = obj['Config'].get('Labels') or {}
        service = labels.get('com.docker.compose.service')
        project = labels.get('com.docker.compose.project')
        if obj['Image'] in hold['image_ids'] or project == hold['project']:
            require(project == hold['project'] and service in anchor['services'] and service not in found,
                    'old/external/unclassified business owner')
            require(cid == obj['Id'] == anchor['services'][service]['cid'], 'fixed container identity changed')
            state = obj['State']
            want_running = service in cold.INFRASTRUCTURE or service in running
            require(state.get('Running') is want_running and not state.get('Paused') and not state.get('Restarting'), 'container activity mismatch')
            if service in cold.INFRASTRUCTURE:
                require(state.get('Health', {}).get('Status') == 'healthy' and state['StartedAt'] == anchor['services'][service]['started_at'], 'infrastructure health/epoch drift')
            else:
                require(obj['HostConfig']['RestartPolicy']['Name'] == 'no', 'business restart policy')
                require(state.get('Status') in (('running',) if want_running else ('exited', 'created')), 'cold state invalid')
            require(obj['Image'] == anchor['services'][service]['image_id'] and obj['Config']['Image'] == anchor['services'][service]['image_reference'], 'container image substituted')
            if service == 'migrate':
                require(type(state.get('ExitCode')) is int and state['ExitCode'] == 0 and state['Status'] == 'exited', 'migration unsuccessful')
            # Network endpoints change on start; bind static networks, not ephemeral IPs.
            policy = {k: obj.get(k) for k in ('Id', 'Image', 'Config', 'HostConfig', 'Mounts', 'Path', 'Args')}
            policy['networks'] = sorted(obj.get('NetworkSettings', {}).get('Networks', {}))
            found[service] = {'cid': cid, 'policy_sha256': digest(policy), 'restart_count': obj.get('RestartCount')}
    require(set(found) == set(anchor['services']), 'service coverage changed')
    return found


class Backend:
    def __init__(self):
        self.docker = cap.Docker()
        self.anchor = read(ANCHOR_PATH)

    def snapshot(self, running=()):
        model, policy = cap.rendered_policy(self.docker, COMPOSE, ENV, REF)
        require(set(policy['roles']) == set(ROLES), 'unexpected generation roles')
        require(model['name'] == PROJECT, 'project mismatch')
        containers = observe(self.docker, self.anchor, running)
        verify_container_config(self.docker, model, containers, policy)
        runtime.verify_quarantine_barrier_read_only(COMPOSE, ENV, execute=lambda args: self.docker.text(args[1:]))
        pg = transport.binding(COMPOSE, ENV, REF)
        db = database_snapshot(pg, self.anchor)
        identity = cap.image_identity(self.docker, REF, APP)
        require(identity['local_image_id'] == IMAGE, 'fixed packaged bytes required')
        return {'source': source_identity(), 'containers': containers, 'postgres': pg, 'database': db,
                'compose_sha256': cold.file_hash(COMPOSE), 'env_sha256': cold.file_hash(ENV),
                'model_sha256': digest(model), 'policy': policy, 'image_identity': identity}

    def certify(self, snapshot):
        prepare_observe('attest')
        evidence = cap.attest(REF, APP, snapshot['policy'], synthetic=False,
                              evidence_directory=os.path.join(ROOT, '.evidence/cold-recovery-resume'), include_history=True)
        prepare_observe('capability_verify', attest_returned=True)
        self.verify(evidence, snapshot)
        prepare_observe('capability_verify', verify_returned=True)
        return evidence

    def verify(self, evidence, snapshot):
        require(cap.verify(evidence, REF, APP, snapshot['policy']) == IMAGE, 'target certification mismatch')
        require(cap.verify_history(evidence, APP), 'full history certification required')

    def healthy(self, evidence):
        model, policy = cap.rendered_policy(self.docker, COMPOSE, ENV, REF)
        model['_compose_file'], model['_env_file'] = COMPOSE, ENV
        cap.verify_running(self.docker, model, evidence, REF)
        runtime.verify('post', COMPOSE, ENV, execute=lambda args: self.docker.text(args[1:]))
        base = ['compose', '-f', COMPOSE, '--env-file', ENV]
        require(cold.read_health(self.docker, base, 'health').get('status') == 'ok', 'API health failed')


def check_window(document, now=None):
    now = int(time.time()) if now is None else now
    start, end = document.get('not_before'), document.get('expires_at')
    require(type(start) is int and type(end) is int and start <= now < end and 0 < end - start <= 900,
            'fresh bounded window required')


def validate_candidate(value, hold, source):
    require(set(value) == {'version', 'purpose', 'nonce', 'not_before', 'expires_at', 'hold_sha256', 'snapshot', 'runtime_capability', 'operations'}, 'strict recovery Proof schema')
    require(type(value['version']) is int and value['version'] == 1 and value['purpose'] == DOMAIN + '-proof' and
            value['operations'] == ['start-existing-api', 'start-existing-worker'] and
            re.fullmatch('[0-9a-f]{32}', value['nonce']), 'recovery domain/nonce required')
    check_window(value)
    require(value['hold_sha256'] == digest(hold) and value['snapshot']['source'] == source and
            source['app_source_sha'] == APP and source['controller_source_sha'] != APP, 'hold/controller/app binding mismatch')
    require(value['nonce'] not in hold.get('recovery_spent_nonces', []), 'candidate replay')


def phrase(candidate):
    return ('APPROVE COLD RECOVERY API_WORKER_ONLY candidate=' + digest(candidate) +
            ' hold=' + candidate['hold_sha256'] + ' controller=' + candidate['snapshot']['source']['controller_source_sha'] +
            ' app=' + APP + ' digest=' + DIGEST +
            ' ANCILLARY_STAYS_STOPPED NO_MIGRATION NO_REENROLLMENT SINGLE_USE')


def approve(candidate, hold, source, operator, confirmation, now=None):
    validate_candidate(candidate, hold, source)
    require(type(operator) is str and re.fullmatch('[A-Za-z0-9_.@-]{3,80}', operator), 'explicit operator required')
    require(confirmation == phrase(candidate), 'explicit exact human confirmation required')
    now = int(time.time()) if now is None else now
    return cold.signed({'version': 1, 'purpose': DOMAIN + '-human-approval', 'candidate_sha256': digest(candidate),
                        'hold_sha256': digest(hold), 'nonce': candidate['nonce'], 'operator': operator,
                        'confirmation': confirmation, 'not_before': now,
                        'expires_at': min(now + 300, candidate['expires_at']), 'approved_at': now})


def validate_approval(approval, candidate, hold, source):
    value = authenticate(approval)
    require(set(value) == {'version', 'purpose', 'candidate_sha256', 'hold_sha256', 'nonce', 'operator', 'confirmation', 'not_before', 'expires_at', 'approved_at'}, 'strict human approval schema')
    validate_candidate(candidate, hold, source)
    check_window(value)
    require(type(value['version']) is int and value['version'] == 1 and value['purpose'] == DOMAIN + '-human-approval' and
            value['candidate_sha256'] == digest(candidate) and value['hold_sha256'] == digest(hold) and
            value['nonce'] == candidate['nonce'] and value['confirmation'] == phrase(candidate) and
            type(value['operator']) is str and re.fullmatch('[A-Za-z0-9_.@-]{3,80}', value['operator']) and
            type(value['approved_at']) is int and value['approved_at'] == value['not_before'] and
            candidate['not_before'] <= value['not_before'] and
            0 < value['expires_at'] - value['not_before'] <= 300 and
            value['expires_at'] <= candidate['expires_at'], 'human approval mismatch')
    return value


def check_owned_attempt(directory, lock, nonce, cids, authority=None, identity=None):
    lock.check()
    current = cold.read_hold(directory)
    attempt = current.get('recovery_attempt', {})
    require(attempt.get('nonce') == nonce and attempt.get('owner') == lock.owner and
            current.get('recovery_authorized_cids') == list(cids), 'foreign attempt or CID binding')
    fields = ('owner', 'project', 'image_ids', 'release_sha', 'proof_sha256', 'proof_nonce')
    expected = authority['hold'] if authority is not None else identity
    require(expected is not None and all(current.get(name) == expected.get(name) for name in fields),
            'attempt hold identity substituted')
    if authority is not None:
        candidate = authority['candidate']
        require(attempt.get('candidate_sha256') == digest(candidate) and
                attempt.get('approval_sha256') == digest(authority['approval']) and
                list(cids) == [candidate['snapshot']['containers'][role]['cid'] for role in ROLES],
                'attempt authorization substituted')
    return current


class AttemptSupervisor:
    """Ephemeral Linux child: pipe EOF/controller death fences before exit.

    Child owns the shared lock PID and serializes starts, so SIGKILL of parent
    cannot race cleanup with an unfinished start CLI. No daemon is installed.
    A timed-out Docker CLI is indeterminate: keep lock/sentinel and UNKNOWN.
    """
    def __init__(self, directory, lock, docker, nonce, cids, lifetime=300, authority=None):
        require(hasattr(os, 'fork'), 'native Linux cleanup supervisor required')
        request_r, request_w = os.pipe()
        response_r, response_w = os.pipe()
        pid = os.fork()
        if pid == 0:
            code = 1
            try:
                os.close(request_w)
                os.close(response_r)
                code = self.watch(directory, lock, docker, nonce, cids, request_r, response_w, lifetime, authority)
            finally:
                # Never unwind into the controller (or an importing test runner).
                os._exit(code)
        os.close(request_r)
        os.close(response_w)
        self.pid, self.request, self.response = pid, request_w, response_r
        self.directory, self.lock = directory, lock
        self.finished = False
        # Fail closed while child owns cleanup. Ordinary lock recovery sees live PID.
        with open(os.path.join(lock.path, 'owner_pid'), 'w') as stream:
            stream.write(str(pid) + '\n')
        require(os.read(response_r, 1) == b'R', 'supervisor not armed')

    @staticmethod
    def watch(directory, lock, docker, nonce, cids, request, response, lifetime, authority):
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        deadline = time.monotonic() + lifetime
        uncertain = False
        initial = cold.read_hold(directory)
        def check_attempt():
            return check_owned_attempt(directory, lock, nonce, cids, authority, initial)

        try:
            check_attempt()
            os.write(response, b'R')
            started = 0
            while time.monotonic() < deadline:
                ready, _, _ = select.select([request], [], [], min(1, max(0, deadline - time.monotonic())))
                if not ready:
                    continue
                message = os.read(request, 1)
                if message == b'S' and started < len(cids):
                    require(time.monotonic() < deadline, 'attempt deadline before start')
                    if authority is not None:
                        check_window(authority['candidate'])
                        check_window(authenticate(authority['approval']))
                    hold = check_attempt()
                    require(hold.get('state') == STARTING, 'attempt no longer starting')
                    hold['recovery_indeterminate_operation'] = {'nonce': nonce, 'cid': cids[started]}
                    cold.write_hold(directory, hold)
                    # Durable sentinel fsync/hold inspection can consume the last
                    # slice. Never turn a nonpositive budget into a start timeout.
                    check_attempt()
                    if authority is not None:
                        check_window(authority['candidate'])
                        check_window(authenticate(authority['approval']))
                    remaining = deadline - time.monotonic()
                    require(remaining > 0, 'attempt deadline after start sentinel')
                    try:
                        docker.text(['start', cids[started]], timeout=min(60, remaining))
                    except BaseException:
                        # CLI disconnect/nonzero is also indeterminate: the daemon
                        # may still finish the accepted start after a stop sample.
                        uncertain = True
                        raise
                    started += 1
                    hold = check_attempt()
                    hold.pop('recovery_indeterminate_operation', None)
                    cold.write_hold(directory, hold)
                    os.write(response, b'O')
                elif message == b'C':
                    # Single writer for terminal success AND failure: the parent
                    # cannot race this guard's deadline/EOF fence with a write.
                    require(authority is not None and started == len(cids), 'commit authority missing')
                    candidate, approval = authority['candidate'], authority['approval']
                    human = validate_approval(approval, candidate, authority['hold'], source_identity())
                    evidence = candidate['runtime_capability']
                    require(cap.verify(evidence, REF, APP, candidate['snapshot']['policy']) == IMAGE and
                            cap.verify_history(evidence, APP), 'commit capability unavailable')
                    check_window(candidate)
                    check_window(human)
                    require(time.monotonic() < deadline, 'attempt deadline before commit')
                    hold = check_attempt()
                    attempt = hold.get('recovery_attempt', {})
                    require(hold.get('state') == STARTING and attempt.get('nonce') == nonce and
                            attempt.get('owner') == lock.owner and attempt.get('candidate_sha256') == digest(candidate) and
                            attempt.get('approval_sha256') == digest(approval) and
                            not hold.get('recovery_indeterminate_operation') and not hold.get('errors'),
                            'commit attempt changed or fenced')
                    hold.update(state=SUCCESS, recovery_verified_cids=list(cids),
                                recovery_intentionally_stopped=['disk-monitor', 'postgres-backup', 'migrate'],
                                recovery_no_env_persistence=True, recovery_verified_at=int(time.time()))
                    cold.write_hold(directory, hold)
                    # An audit-write/ack timeout is failure, never inferred success.
                    require(time.monotonic() < deadline, 'attempt deadline during commit')
                    check_window(candidate)
                    check_window(human)
                    os.write(response, b'O')
                    return 0
                else:
                    break
        except Exception:
            pass
        try:
            # A lost owner or substituted authenticated attempt is NOT authority
            # to mutate its hold, containers or lock, even in failure cleanup.
            check_attempt()
            cold.fence(directory, 'recovery-attempt', 'controller-lost-or-failed', docker)
            if uncertain:
                hold = cold.read_hold(directory)
                hold['state'] = 'RECOVERY_REQUIRED_UNKNOWN'
                hold['errors'].append('docker-start-outcome-indeterminate')
                cold.write_hold(directory, hold)
                lock.retained = True
            else:
                lock.release()
        except Exception:
            lock.retained = True
        return 1

    def command(self, message):
        os.write(self.request, message)
        ready, _, _ = select.select([self.response], [], [], 310)
        require(ready and os.read(self.response, 1) == b'O', 'supervised operation failed')

    def close(self, success=False):
        if self.finished:
            return
        self.finished = True
        try:
            if success:
                self.command(b'C')
        finally:
            os.close(self.request)
            os.close(self.response)
            _, status = os.waitpid(self.pid, 0)
            if status != 0:
                self.lock.retained = True
            require(success and status == 0, 'attempt fenced or outcome unknown')


def execute(candidate, approval, backend, lock, directory=DIR, supervisor_factory=AttemptSupervisor):
    hold = read_stopped_hold(directory)
    authority = {'hold': json.loads(cap.canonical(hold)), 'candidate': candidate, 'approval': approval}
    human = validate_approval(approval, candidate, hold, source_identity())
    backend.verify(candidate['runtime_capability'], candidate['snapshot'])
    require(backend.snapshot() == candidate['snapshot'], 'pre-start snapshot drift')
    check_window(human)
    check_window(candidate)
    lock.check()
    # No business mutation until this atomic single-use attempt is durable.
    cids = [candidate['snapshot']['containers'][role]['cid'] for role in ROLES]
    hold['recovery_authorized_cids'] = cids
    hold.setdefault('recovery_original_hold', cold.signed(dict(authority['hold'])))
    hold.setdefault('recovery_spent_nonces', []).append(candidate['nonce'])
    hold.update(state=STARTING, recovery_attempt={'nonce': candidate['nonce'], 'approval_sha256': digest(approval),
                'candidate_sha256': digest(candidate), 'operator': human['operator'], 'owner': lock.owner})
    cold.write_hold(directory, hold)
    supervisor = None
    success = False
    try:
        supervisor = supervisor_factory(directory, lock, backend.docker, candidate['nonce'], cids,
                                         lifetime=min(300, human['expires_at'] - int(time.time())), authority=authority)
        for index, cid in enumerate(cids):
            check_window(human)
            check_window(candidate)
            lock.check()
            backend.verify(candidate['runtime_capability'], candidate['snapshot'])
            require(backend.snapshot(ROLES[:index]) == candidate['snapshot'], 'immediate pre-start drift')
            check_window(human)
            check_window(candidate)
            supervisor.command(b'S')
        backend.healthy(candidate['runtime_capability'])
        check_window(human)
        check_window(candidate)
        backend.verify(candidate['runtime_capability'], candidate['snapshot'])
        require(backend.snapshot(ROLES) == candidate['snapshot'], 'post-start protected drift')
        check_window(human)
        check_window(candidate)
        lock.check()
        # Guard alone commits durable terminal state, serialized with its fence.
        supervisor.close(success=True)
        success = True
    finally:
        if not success:
            if supervisor is not None:
                supervisor.close()
            else:
                lock.retained = True
                check_owned_attempt(directory, lock, candidate['nonce'], cids, authority)
                cold.fence(directory, 'recovery-supervisor-arm', 'supervisor-unavailable', backend.docker)


def main():
    global _prepare_diagnostic, _prepare_active
    _prepare_diagnostic = None
    _prepare_active = False
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('prepare', 'approve', 'execute'))
    parser.add_argument('--operator')
    parser.add_argument('--confirmation')
    args = parser.parse_args()
    if args.action == 'prepare':
        _prepare_active = True
        try:
            sources = [(globals(), __file__, 'ops/cold-recovery-resume.py')]
            sources += [(module.__dict__, module.__file__, 'ops/' + os.path.basename(module.__file__))
                        for module in (cold, cap, transport, runtime)]
            _prepare_diagnostic = prepare_diagnostics.Diagnostic(ROOT, sources, Refused,
                                                               transport.TransportError, runtime.GateError)
        except BaseException:
            pass
    require(sys.platform == 'linux' and os.geteuid() == 0, 'root-owned native Linux entry required')
    os.chdir(ROOT)
    # Trust root remains existing release-trust key; no secret env/test override.
    require(not os.environ.get('RELEASE_TRUST_SECRET'), 'root-owned key required, not env secret')
    key_path = protected_path(os.path.join(DIR, 'release-trust.key'), secret=True)
    os.environ['RELEASE_TRUST_KEY_FILE'] = key_path
    for name in list(os.environ):
        if name.upper().startswith(('DOCKER_', 'COMPOSE_', 'PG', 'XIANZHI_', 'QUARANTINE_')):
            os.environ.pop(name)
    for path in (ROOT, DIR, WORK, os.path.join(WORK, 'state.json'), COMPOSE, ENV, PROOF_PATH):
        protected_path(path)
    prepare_observe('source_identity')
    source = source_identity()
    prepare_observe('initial_hold')
    hold = read_stopped_hold()
    prepare_observe('historical_binding')
    historical = authenticate(read(PROOF_PATH))  # historical identity ONLY, no TTL authority
    require(cold.file_hash(PROOF_PATH) == OLD_PROOF and historical.get('git_sha') == APP and
            historical.get('image_reference') == REF and historical.get('proof_nonce') == hold['proof_nonce'] and
            historical.get('cold_recovery_policy', {}).get('baseline_identity', {}).get('local_image_id') == OLD_IMAGE and
            historical.get('cold_recovery_policy', {}).get('project') == hold['project'] and
            historical.get('compose_hash') == cold.file_hash(COMPOSE) and
            historical.get('env_hash') == cold.file_hash(ENV), 'historical signed identity/config mismatch')
    prepare_observe('lock_acquire')
    lock = ReleaseLock()
    try:
        if args.action == 'prepare':
            require(not args.operator and not args.confirmation, 'prepare cannot approve')
            prepare_observe('initial_snapshot')
            backend = Backend()
            snapshot = backend.snapshot()
            prepare_observe('attest', initial_snapshot_returned=True)
            evidence = backend.certify(snapshot)
            prepare_observe('post_snapshot')
            require(prepare_snapshot_equal(backend.snapshot(), snapshot) and
                    prepare_hold_equal(read_stopped_hold() == hold), 'post-certification drift')
            prepare_observe('candidate_signing')
            now = int(time.time())
            candidate = {'version': 1, 'purpose': DOMAIN + '-proof', 'nonce': uuid.uuid4().hex,
                         'not_before': now, 'expires_at': now + 900, 'hold_sha256': digest(hold),
                         'snapshot': snapshot, 'runtime_capability': evidence,
                         'operations': ['start-existing-api', 'start-existing-worker']}
            signed = cold.signed(candidate)
            prepare_observe('candidate_publication')
            cold.atomic_json(CANDIDATE, signed)
            prepare_observe('candidate_phrase')
            print(phrase(candidate))
        else:
            protected_path(CANDIDATE)
            candidate = authenticate(read(CANDIDATE))
            validate_candidate(candidate, hold, source)
            if args.action == 'approve':
                # Root key holder must explicitly supply displayed phrase/operator.
                cold.atomic_json(APPROVAL, approve(candidate, hold, source, args.operator, args.confirmation))
                print('RECOVERY_HUMAN_APPROVAL_SIGNED; no starts performed')
            else:
                require(not args.operator and not args.confirmation, 'execute cannot sign')
                protected_path(APPROVAL)
                execute(candidate, read(APPROVAL), Backend(), lock)
                print(SUCCESS + '; ordinary releases remain blocked')
    finally:
        prepare_observe('lock_release')
        if os.path.exists(lock.path):
            lock.release()
        prepare_observe('lock_released')


def entry():
    try:
        main()
    except BaseException as error:
        # Never print Docker/config/database exceptions or secret values.
        sys.stderr.write('COLD_RECOVERY_RESUME_REFUSED: hold retained; review authenticated state and lock\n')
        if _prepare_active:
            try:
                if _prepare_diagnostic is not None:
                    _prepare_diagnostic.emit(error, sys.stderr)
                else:
                    sys.stderr.write('COLD_RECOVERY_PREPARE_DIAGNOSTIC: {"reason_code":"UNKNOWN","check_name":"UNKNOWN","stage":"diagnostic_unavailable","error_type":"UNKNOWN"}\n')
            except BaseException:
                pass
        sys.exit(1)


if __name__ == '__main__':
    entry()
