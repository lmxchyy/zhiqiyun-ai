#!/usr/bin/env python3
"""Zero-skip Linux3.6.8 actual NEW Carrier/Backend/native-switch integration.

Reuses owned disposable PG/Rabbit/Redis/MinIO setup ONLY. Historical OLD
certificate is an explicitly synthetic, structurally complete fixture precondition;
NEW attestation, SQL transport, Backend, PID1 and Guard are real and unmodified.
Each invocation owns one fixture/attempt. No baseline restart or acceptance retry.
"""
import copy
import json
import os
from pathlib import Path
import signal
import sys
import time
import types
import unittest

ROOT = Path(__file__).resolve().parents[1]


def load(name, relative):
    m = types.ModuleType(name); m.__file__ = str(ROOT / relative)
    exec(compile(Path(m.__file__).read_bytes(), m.__file__, 'exec'), m.__dict__)
    return m


unit = load('forward_integration_preconditions', 'tests/recovery-forward-switch-test.py')
f = unit.f
baseline = load('forward_baseline_setup', 'tests/cold-recovery-resume-backend-test.py')
b = baseline.r
OUT = ROOT / '.evidence/forward-switch/backend'
MINIO_REF = 'docker.io/minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e'
CASE = sys.argv[1] if len(sys.argv) == 2 else ''
CASES = ('success', 'parent-kill', 'parent-term', 'partial-create', 'response-loss', 'broker')
if CASE not in CASES:
    raise RuntimeError('one explicit owned integration case required')
sys.argv[:] = sys.argv[:1]


class ForwardDocker(f.NativeDocker):
    def __init__(self, fixture):
        super().__init__()
        self.trace, self.owner = fixture.trace, fixture.owner
        self.phase = 'provisioning'
        self.fixture = fixture
        self.new_starts = 0
        self.armed_fault = False

    def run(self, args, **kwargs):
        baseline.emit(self.trace, 'command', phase=self.phase, argv=[self.redact(str(a)) for a in args],
                      stdin_sha256=f.cap.sha(kwargs['data']) if kwargs.get('data') else None)
        if self.phase != 'forward-entry':
            return super().run(args, **kwargs)
        mutation = args[0] in ('start', 'stop', 'rm') or args[0] == 'compose' and 'up' in args
        if mutation:
            hold = f.read_success_hold()
            attempt = hold['forward_attempt']
            if args[0] in ('start', 'stop', 'rm'):
                cid = args[-1]
                allowed = list(attempt['new_cids'].values()) + list(attempt['old_cids'].values())
                if cid not in allowed: raise AssertionError('unbound native mutation')
                obj = self.inspect('container', cid)
                if obj['Config']['Labels'].get(baseline.LABEL) != self.owner:
                    raise AssertionError('foreign native mutation')
                if args[0] == 'start' and (obj['Image'] == f.OLD_IMAGE or cid not in attempt['new_cids'].values()):
                    raise AssertionError('old/unbound start forbidden')
        result = super().run(args, **kwargs)  # actual original Engine request/result
        if args[0] == 'compose' and 'up' in args:
            role = args[-1]
            name = f.predicted_names(f.read_success_hold()['forward_attempt']['nonce'])[role]
            obj = self.inspect('container', name)
            self.fixture.record('container', obj['Id'])  # exact resource ID for EXIT cleanup
            baseline.emit(self.trace, 'real-created', cid=obj['Id'], role=role, owner=self.owner)
            if CASE == 'partial-create' and role == f.ROLES[0]:
                raise RuntimeError('test-only completed-create response loss; native create actually acknowledged')
        if args[0] == 'start':
            self.new_starts += 1
            baseline.emit(self.trace, 'real-new-start-ack', cid=args[-1], ordinal=self.new_starts,
                          lifecycle=baseline.lifecycle(self.inspect('container', args[-1])))
            if self.new_starts == 1 and CASE in ('parent-kill', 'parent-term'):
                os.kill(self.controller_pid, signal.SIGKILL if CASE == 'parent-kill' else signal.SIGTERM)
            if self.new_starts == 1 and CASE == 'response-loss':
                raise RuntimeError('test-only completed-start response loss, not in-flight acceptance proof')
            if self.new_starts == 2 and CASE == 'broker':
                super().run(['exec', self.rabbit, 'rabbitmqctl', 'delete_user', 'fixture_admin'])
        return result


class FixedForwardIntegration(baseline.RealBackendTests):
    @classmethod
    def setUpClass(cls):
        if sys.platform != 'linux' or sys.version_info[:3] != (3, 6, 8):
            raise RuntimeError('actual Linux Python3.6.8 mandatory; no skip')
        raw = f.cap.Docker()
        for ref, image in ((baseline.FIXED_REF, f.OLD_IMAGE), ('ghcr.io/lmxchyy/zhiqiyun-ai@' + f.DIGEST, None)):
            obj = raw.inspect('image', ref)
            if ref not in obj.get('RepoDigests', []) or (obj['Os'], obj['Architecture']) != ('linux', 'amd64') or image and obj['Id'] != image:
                raise RuntimeError('exact official old/new Carrier absent')
        f.source_identity()
        minio = raw.inspect('image', MINIO_REF)
        if not {MINIO_REF, MINIO_REF[len('docker.io/'):]} & set(minio.get('RepoDigests', [])) or (minio['Os'], minio['Architecture']) != ('linux', 'amd64'):
            raise RuntimeError('same-digest official MinIO tooling absent; no fallback')
        OUT.mkdir(mode=0o700, parents=True, exist_ok=True)
        f.r.protected_path(str(OUT))
        baseline.OUT = OUT

    def provision(self):
        # Fixture-only override of the inherited denied mirror. Same bytes only.
        self.docker.env['MINIO_IMAGE'] = MINIO_REF
        super().provision()
        env_path = Path(b.ENV)
        text = env_path.read_text(encoding='utf-8')
        denied = 'docker.m.daocloud.io/minio/minio@' + MINIO_REF.split('@')[1]
        self.assertIn('MINIO_IMAGE=' + denied, text)
        env_path.write_text(text.replace('MINIO_IMAGE=' + denied, 'MINIO_IMAGE=' + MINIO_REF), encoding='utf-8')
        env_path.chmod(0o600)
        obj = self.docker.inspect('container', self.services['minio'])
        self.assertEqual(obj['Config']['Image'], MINIO_REF)
        self.assertEqual(obj['Image'], self.docker.inspect('image', MINIO_REF)['Id'])

    def setUp(self):
        # Original fixture's UUID manifest precedes its native provisioning.
        super().setUp()
        redactions = list(self.docker.redactions)
        self.docker = ForwardDocker(self)
        self.docker.redactions = redactions
        self.backend.docker = self.docker
        self.docker.rabbit = self.services['rabbitmq']
        self.docker.controller_pid = os.getpid()
        # Only setup may launch OLD normal API/Worker, exactly once per role.
        self.docker.phase = 'baseline-setup-old-once'
        for role in f.ROLES: self.docker.text(['start', self.services[role]])
        self.wait_healthy(f.ROLES)
        # Fixture-only durable334 analogue: no external provider configuration,
        # no mutable heartbeat/event snapshots. Future lease avoids setup races.
        raw = json.dumps({'id': 'task_000334', 'userId': 'fixture-user', 'type': 'TEXT_TO_IMAGE',
                          'model': 'gpt-image-2', 'status': 'PROCESSING', 'params': {}})
        self.sql("INSERT INTO xz_generation_tasks(id,user_id,type,model,status,task_status,execution_generation,worker_id,lease_until,params,raw) "
                 "VALUES('task_000334','fixture-user','TEXT_TO_IMAGE','gpt-image-2','PROCESSING','RUNNING',3,'fixture-preexisting',now()+interval '5 minutes','{}'," + b.transport.parameter(raw) + "::jsonb); "
                 "INSERT INTO provider_executions(id,task_id,provider,provider_model,capability,attempt,status,request_fingerprint,task_execution_generation,succeeded_at,result_metadata) "
                 "VALUES(113,'task_000334','fixture','gpt-image-2','image',1,'succeeded',repeat('a',64),3,'2026-10-13T03:51:10.958975Z','[{\"URL\":\"data:image/png;base64,aGVsbG8=\"}]'::jsonb);")
        anchor = b.read(b.ANCHOR_PATH)
        for role, cid in self.services.items():
            obj = self.docker.inspect('container', cid)
            anchor['services'][role].update(started_at=obj['State']['StartedAt'], finished_at=obj['State']['FinishedAt'])
        self.write('anchor.json', anchor)
        # Bind only fixed test paths/project/reference. No production override CLI.
        for name in ('PROJECT', 'COMPOSE', 'ENV', 'DIR', 'WORK', 'ANCHOR_PATH'):
            setattr(f, name, getattr(b, name))
            setattr(f.r, name, getattr(b, name))
        f.OLD_REF = baseline.FIXED_REF
        f.REF = 'ghcr.io/lmxchyy/zhiqiyun-ai@' + f.DIGEST
        f.r.REF = f.OLD_REF
        f.r.OLD_PROOF = b.OLD_PROOF
        f.r.PROOF_PATH = str(self.path / 'fixture-historical-proof.json')
        f.r.CANDIDATE, f.r.APPROVAL = str(self.path / 'prior-candidate.json'), str(self.path / 'prior-approval.json')
        proof = f.authenticate(b.read(f.r.PROOF_PATH))
        proof.update(compose_hash=b.cold.file_hash(b.COMPOSE), env_hash=b.cold.file_hash(b.ENV),
                     cold_recovery_policy={'baseline_identity': {'local_image_id': f.r.OLD_IMAGE}, 'project': f.PROJECT})
        self.write('fixture-historical-proof.json', f.cold.signed(proof))
        f.r.OLD_PROOF = b.cold.file_hash(f.r.PROOF_PATH)
        original = b.cold.read_hold(b.DIR)
        original['proof_sha256'] = f.r.OLD_PROOF
        source = f.source_identity()
        model, policy = f.cap.rendered_policy(self.docker, f.COMPOSE, f.ENV, f.OLD_REF)
        old_identity = self.old_hash_identity()  # SETUP ONLY, owned network-none hash
        ancestry = {'old_identity': old_identity, 'old_policy': policy}
        self.forward_backend = f.Backend()
        self.forward_backend.docker = self.docker
        self.forward_backend.anchor = anchor
        snapshot = self.forward_backend.snapshot(ancestry)
        prior_snapshot = copy.deepcopy(snapshot)
        prior_snapshot['image_identity'] = old_identity
        prior_snapshot['source'] = b.source_identity()
        now = int(time.time())
        latest_stopped = copy.deepcopy(original)
        failed_nonce = os.urandom(16).hex()
        latest_stopped.update(recovery_original_hold=f.cold.signed(original), recovery_spent_nonces=[failed_nonce],
                              errors=['prior-attempt-completely-fenced'])
        prior = dict(version=1, purpose=b.DOMAIN + '-proof', nonce=os.urandom(16).hex(), not_before=now-100,
                     expires_at=now+800, hold_sha256=f.digest(latest_stopped), snapshot=prior_snapshot,
                     runtime_capability=unit.synthetic_certificate(old_identity, policy),
                     operations=['start-existing-api', 'start-existing-worker'])
        approval = b.approve(prior, latest_stopped, prior_snapshot['source'], 'fixture.operator', b.phrase(prior))
        self.write('prior-candidate.json', f.cold.signed(prior)); self.write('prior-approval.json', approval)
        hold = copy.deepcopy(original)
        cids = [self.services[role] for role in f.ROLES]
        hold.update(state=b.SUCCESS, recovery_original_hold=f.cold.signed(original), recovery_spent_nonces=[failed_nonce, prior['nonce']],
                    recovery_authorized_cids=cids, recovery_verified_cids=cids,
                    recovery_intentionally_stopped=['disk-monitor', 'postgres-backup', 'migrate'],
                    recovery_no_env_persistence=True, recovery_verified_at=now,
                    recovery_attempt={'nonce': prior['nonce'], 'owner': 'fixture-original-session', 'operator': 'fixture.operator',
                                      'candidate_sha256': f.digest(prior), 'approval_sha256': f.digest(approval)})
        f.cold.write_hold(f.DIR, hold)
        self.hold = hold
        self.ancestry = f.history_binding(hold)  # production historical validators unchanged
        self.write('provenance-boundary.json', {'historical_old_certificate': 'SYNTHETIC structural precondition ONLY',
                    'new_attestation': 'actual exact official Carrier mandatory', 'old_normal_starts_per_role': 1,
                    'case': CASE, 'production_authority': False})
        self.before = {role: baseline.lifecycle(self.docker.inspect('container', cid)) for role, cid in self.services.items()}

    def old_hash_identity(self):
        obj = self.docker.inspect('image', f.OLD_REF)
        name = 'forward-old-hash-' + self.owner
        self.write('old-hash-owner.json', {'owner': self.owner, 'name': name, 'resources': []})
        cid = self.record('container', self.docker.text(['create', '--name', name, '--pull', 'never',
                '--label', baseline.LABEL+'='+self.owner, '--label', 'com.docker.compose.project='+self.project,
                '--label', 'com.docker.compose.service=fixture-old-hash', '--network', 'none', '--read-only',
                '--entrypoint', '/usr/bin/sha256sum', obj['Id']] + list(f.cap.BINARIES)))
        self.write('old-hash-owner.json', {'owner': self.owner, 'name': name, 'resources': [['container', cid]]})
        output = self.docker.text(['start', '-a', cid])
        hashes = {line.split()[1]: line.split()[0] for line in output.splitlines()}
        self.docker.text(['rm', cid])
        return {'reference': f.OLD_REF, 'release_sha': f.OLD_APP, 'local_image_id': obj['Id'],
                'repo_digests': sorted(obj['RepoDigests']), 'binary_sha256': hashes,
                'platform': {'os': obj['Os'], 'architecture': obj['Architecture'], 'variant': obj.get('Variant', '')}}

    def capture_final(self):
        # Old pair can have been removed. Never turn missing original CID into a
        # cleanup failure or attempt to recreate it for a diagnostic receipt.
        if not hasattr(self, 'services'): return
        current = self.docker.text(['ps', '-aq', '--no-trunc']).splitlines()
        self.write('final.json', {'hold': f.read_success_hold() if hasattr(self, 'hold') else None,
            'lifecycle': {role: baseline.lifecycle(self.docker.inspect('container', cid))
                          for role, cid in self.services.items() if cid in current}})
        postgres = self.services['postgres']
        result = self.docker.run(['logs', postgres], check=False)
        (self.path / 'postgres-statements.log').write_text(self.docker.redact(result.stderr.decode('utf-8', errors='replace')), encoding='utf-8')

    def cleanup(self):
        cleanup = load('forward_exact_fixture_cleanup', 'tests/recovery-forward-switch-cleanup.py')
        cleanup.cleanup_backend(self.path)
        cleanup.sanitize_backend(self.path)

    # Override inherited business integration tests: this job executes ONLY one
    # new fixed forward test, not an accidental second old recovery attempt.
    def runTest(self):
        snapshot = self.forward_backend.snapshot(self.ancestry)
        # No mock of actual certify/native target identity/history.
        evidence = f.attest_owned(f.REF, f.APP, snapshot['policy'])
        f.verify_prepared_capability(evidence, f.REF, f.APP, snapshot['policy'], evidence['identity'])
        now, nonce = int(time.time()), os.urandom(16).hex()
        candidate = dict(version=1, purpose=f.DOMAIN+'-proof', nonce=nonce, not_before=now, expires_at=now+900,
                         hold_sha256=f.digest(self.hold), snapshot=snapshot, ancestry=self.ancestry,
                         runtime_capability=evidence, operations=f.OPERATIONS, new_names=f.predicted_names(nonce))
        approval = f.approve(candidate, self.hold, snapshot['source'], 'fixture.operator', f.phrase(candidate))
        self.write('certification.json', evidence)
        self.docker.phase = 'forward-entry'
        started = time.monotonic()
        if CASE in ('parent-kill', 'parent-term'):
            pid = os.fork()
            if pid == 0:
                self.docker.controller_pid = os.getpid()
                try:
                    lock = f.r.ReleaseLock(f.DIR)
                    self.assertEqual(Path(lock.path).parent, Path(f.DIR))
                    f.execute(candidate, approval, self.forward_backend, lock)
                finally:
                    os._exit(0)
            os.waitpid(pid, 0)
            deadline = time.monotonic()+65
            while time.monotonic() < deadline and f.read_success_hold()['state'] == f.STARTING:
                time.sleep(.2)
            self.assertNotEqual(f.read_success_hold()['state'], f.STARTING)
        else:
            lock = f.r.ReleaseLock(f.DIR)
            self.assertEqual(Path(lock.path).parent, Path(f.DIR))
            if CASE == 'success': f.execute(candidate, approval, self.forward_backend, lock)
            else:
                with self.assertRaises(f.Refused): f.execute(candidate, approval, self.forward_backend, lock)
        post = f.read_success_hold()
        self.assertEqual(post['forward_spent_nonces'].count(nonce), 1)
        self.assertEqual(post['forward_original_success'], f.cold.signed(self.hold))
        self.assertTrue(Path(f.WORK).is_dir())
        self.assertEqual(f.cold.file_hash(f.COMPOSE), snapshot['compose_sha256'])
        self.assertEqual(f.cold.file_hash(f.ENV), snapshot['env_sha256'])
        for role in f.cold.INFRASTRUCTURE + ('migrate', 'postgres-backup', 'disk-monitor'):
            self.assertEqual(baseline.lifecycle(self.docker.inspect('container', self.services[role])), self.before[role])
        if CASE == 'success':
            self.assertEqual(post['state'], f.SUCCESS)
            final = self.forward_backend.snapshot(self.ancestry, completed=True,
                    replacement={'identity': evidence['identity'], 'cids': post['forward_verified_cids'],
                                 'model': f.new_model(self.docker, candidate)})
            self.assertEqual(f.protected_snapshot(final), f.protected_snapshot(snapshot))
            for role, cid in post['forward_verified_cids'].items():
                self.assertNotEqual(cid, self.services[role])
                self.assertTrue(self.docker.inspect('container', cid)['State']['Running'])
        else:
            self.assertIn(post['state'], (f.r.STOPPED, 'RECOVERY_REQUIRED_UNKNOWN'))
            if CASE in ('response-loss', 'partial-create'):
                self.assertEqual(post['state'], 'RECOVERY_REQUIRED_UNKNOWN')
                self.assertIn('forward_indeterminate_operation', post)
                self.assertTrue(Path(f.DIR, 'release.lock').is_dir())
        self.write('switch-receipt.json', {'case': CASE, 'state': post['state'], 'nonce': nonce,
                    'elapsed_seconds': time.monotonic()-started, 'old_cids': post['forward_attempt']['old_cids'],
                    'new_cids': post['forward_attempt']['new_cids'], 'ancillary_equal': True,
                    'original_history': f.database_snapshot(snapshot['postgres'], self.forward_backend.anchor, self.docker),
                    'engine_acceptance_boundary': 'response-loss cases inject only after completed real native response; in-flight phase unproved'})
        self.assertLess(time.monotonic()-started, 300)


if __name__ == '__main__':
    # Explicit single TestCase avoids discovering inherited old-recovery cases.
    result = unittest.TextTestRunner(verbosity=2).run(unittest.TestSuite([FixedForwardIntegration('runTest')]))
    sys.exit(0 if result.wasSuccessful() else 1)
