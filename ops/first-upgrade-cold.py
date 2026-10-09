#!/usr/bin/env python3
"""First upgrade only: authenticated unsupported baseline, never a start path.

Python 3.6. Recovery has no database, broker, migration or provider transport.
A persistent hold is independent of the transient release lock.
"""
import argparse
import hashlib
import hmac
import json
import os
import shutil
import subprocess
import sys
import time
import types

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BASELINE = 'b45e72613dff3863d9eacc13cc99e4d320cf284f'
BASELINE_TREE = '2f464e146315dbcc04edc87b2afc6815139ff8e5'
SOURCE_PINS = {
    'backend-go/cmd/api/main.go': 'b33ca206c3b5fb4fde96bb6c7d49edb4532c986b3d3976f0fdc4255543d932c3',
    'backend-go/cmd/generation-worker/main.go': '5388b6ceef9761690e2b18b6e29239d05e679348923a4e2099d00a3a3222aaf3',
}
# Stop all other services in the verified project, including maintenance jobs.
INFRASTRUCTURE = ('postgres', 'redis', 'rabbitmq', 'minio')
HOLD = 'cold-recovery-required'


def load_capability():
    path = os.path.join(ROOT, 'ops/verify-image-quarantine-capability.py')
    module = types.ModuleType('cold_capability')
    module.__file__ = path
    with open(path, 'rb') as stream:
        exec(compile(stream.read(), path, 'exec'), module.__dict__)
    return module


capability = load_capability()
Refused = capability.Refused
canonical = capability.canonical
sha = capability.sha


def read_json(path):
    with open(path, 'r', encoding='utf-8') as stream:
        return json.load(stream)


def file_hash(path):
    with open(path, 'rb') as stream:
        return sha(stream.read())


def baseline_identity(receipt, docker):
    if receipt.get('previous_git_sha') != BASELINE:
        raise Refused('cold policy only supports the exact unsupported baseline')
    tree = subprocess.check_output([shutil.which('git') or 'git', 'rev-parse', BASELINE + '^{tree}'], cwd=ROOT).decode().strip()
    if tree != BASELINE_TREE:
        raise Refused('unsupported baseline source tree mismatch')
    for path, digest in SOURCE_PINS.items():
        source = subprocess.check_output([shutil.which('git') or 'git', 'show', BASELINE + ':' + path], cwd=ROOT)
        if sha(source) != digest or b'--quarantine-capability-v1' in source:
            raise Refused('unsupported baseline source identity mismatch')
    provenance = receipt.get('rollback_provenance') or {}
    if (provenance.get('repository') != 'lmxchyy/zhiqiyun-ai' or
            provenance.get('workflow') != '.github/workflows/immutable-image-release.yml' or
            provenance.get('head_sha') != BASELINE or
            not provenance.get('run_id') or not provenance.get('artifact_id')):
        raise Refused('official unsupported baseline provenance required')
    if file_hash(receipt['rollback_manifest_path']) != receipt.get('rollback_manifest_sha256'):
        raise Refused('unsupported baseline manifest changed')
    manifest = read_json(receipt['rollback_manifest_path'])
    ref = manifest.get('image_reference')
    registry = receipt.get('selected_registry')
    if registry:
        ref = (manifest.get('registries', {}).get(registry) or {}).get('image_reference', ref)
    if manifest.get('git_sha') != BASELINE or ref != receipt.get('previous_image_reference'):
        raise Refused('unsupported source/image official manifest mismatch')
    # Hash-only entrypoint on network-none; NEVER invoke the legacy API/worker.
    identity = capability.image_identity(docker, ref, BASELINE)
    if identity['local_image_id'] != receipt.get('previous_image_id'):
        raise Refused('actual rollback image identity mismatch')
    return identity


def cold_model(model):
    data = json.loads(canonical(model))
    if not data.get('name') or not data.get('services'):
        raise Refused('explicit Compose project identity required')
    for service, cfg in data['services'].items():
        if service not in INFRASTRUCTURE:
            cfg['restart'] = 'no'
    return data


def migration_identity(model, docker):
    reference = model['services']['migrate']['image']
    return {'reference': reference, 'local_image_id': docker.inspect('image', reference)['Id']}


def create_policy(receipt_path, receipt_hash, model, config_path, docker):
    if file_hash(receipt_path) != receipt_hash:
        raise Refused('actual rollback receipt changed')
    identity = baseline_identity(read_json(receipt_path), docker)
    data = cold_model(model)
    atomic_json(config_path, data)
    return {'version': 1, 'mode': 'first-upgrade-cold', 'unsupported_source_tree': BASELINE_TREE,
            'rollback_receipt_sha256': receipt_hash, 'baseline_identity': identity,
            'migration_runner': migration_identity(model, docker),
            'config_path': config_path, 'config_sha256': file_hash(config_path),
            'project': data['name'], 'services': sorted(set(data['services']) - set(INFRASTRUCTURE))}


def verify_policy(proof, model, docker, receipt_path=None):
    policy = proof.get('cold_recovery_policy')
    if not isinstance(policy, dict) or policy.get('version') != 1 or policy.get('mode') != 'first-upgrade-cold':
        raise Refused('explicit signed first-upgrade cold policy missing')
    if proof.get('rollback_runtime_capability') is not None:
        raise Refused('capable rollback cannot be classified cold')
    path = receipt_path or proof.get('rollback_receipt_path')
    if file_hash(path) != proof.get('rollback_receipt_hash') or policy.get('rollback_receipt_sha256') != proof.get('rollback_receipt_hash'):
        raise Refused('cold policy actual rollback receipt mismatch')
    if policy.get('baseline_identity') != baseline_identity(read_json(path), docker) or policy.get('unsupported_source_tree') != BASELINE_TREE:
        raise Refused('cold baseline evidence substituted')
    if policy.get('migration_runner') != migration_identity(model, docker):
        raise Refused('cold migration runner identity changed')
    data = cold_model(model)
    if (policy.get('project') != data['name'] or
            policy.get('services') != sorted(set(data['services']) - set(INFRASTRUCTURE)) or
            file_hash(policy['config_path']) != policy.get('config_sha256') or
            read_json(policy['config_path']) != data):
        raise Refused('cold no-restart configuration changed')
    return policy


def atomic_json(path, value):
    temporary = path + '.tmp-' + str(os.getpid())
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    descriptor = os.open(temporary, flags, 0o600)
    try:
        with os.fdopen(descriptor, 'w', encoding='utf-8') as stream:
            json.dump(value, stream, sort_keys=True, indent=2)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        if os.name != 'nt':
            directory = os.open(os.path.dirname(path) or '.', os.O_RDONLY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def key():
    secret = os.environ.get('RELEASE_TRUST_SECRET', '').strip()
    if not secret:
        for path in (os.environ.get('RELEASE_TRUST_KEY_FILE'), '/etc/zhiqiyun/release-trust.key', '.prestage/release-trust.key'):
            if path and os.path.isfile(path):
                with open(path, 'r') as stream:
                    secret = stream.read().strip()
                break
    if not secret:
        raise Refused('cold hold authentication key missing')
    return secret.encode('utf-8')


def signed(value):
    payload = dict(value)
    payload['signature'] = hmac.new(key(), canonical(value).encode('utf-8'), hashlib.sha256).hexdigest()
    return payload


def read_hold(directory):
    path = os.path.join(directory, HOLD, 'state.json')
    value = read_json(path)
    signature = value.pop('signature', '')
    if not hmac.compare_digest(signature, signed(value)['signature']):
        raise Refused('cold hold authentication failed; recovery authorization required')
    return value


def write_hold(directory, value):
    atomic_json(os.path.join(directory, HOLD, 'state.json'), signed(value))


def inventory(docker, hold):
    ids = docker.text(['ps', '-aq', '--no-trunc']).splitlines()
    result = []
    for cid in ids:
        obj = docker.inspect('container', cid)
        labels = obj.get('Config', {}).get('Labels') or {}
        project = labels.get('com.docker.compose.project')
        service = labels.get('com.docker.compose.service')
        image = obj.get('Image')
        # Infrastructure labels do not exempt a container running business bytes.
        exact_business = image in hold['image_ids']
        if exact_business or (project == hold['project'] and service not in INFRASTRUCTURE):
            if not service and project == hold['project']:
                service = 'unclassified-project-process'
            result.append((cid, obj, service or 'exact-business-image'))
    return result


def fence(directory, stage, reason, docker, preparing=False, owner=None, proof_path=None):
    hold = read_hold(directory)
    if owner is not None and hold.get('owner') != owner:
        raise Refused('cleanup cannot fence a foreign hold')
    if proof_path is not None:
        if (hold.get('proof_sha256') != file_hash(proof_path) or
                hold.get('release_sha') != read_json(proof_path).get('git_sha')):
            raise Refused('cleanup hold proof/release identity mismatch')
    if preparing and (hold.get('state') != 'ARMED_RECOVERY_REQUIRED' or not owner):
        raise Refused('prepare requires same in-flight hold owner')
    hold.update(state='RECOVERY_REQUIRED_UNKNOWN', failure_stage=stage, trigger_reason=reason,
                observed_at_unix=int(time.time()), stopped_services=[], observed_state=[], errors=[])
    # Persist recovery-required BEFORE any command; failure must never imply PASS.
    write_hold(directory, hold)
    try:
        entries = inventory(docker, hold)
    except Exception:
        hold['errors'].append('inventory-inspection-failed')
        write_hold(directory, hold)
        raise Refused('cold inventory unknown; hold retained')
    for cid, obj, service in entries:
        try:
            # Revalidate identity before each mutation. No arbitrary names/tags.
            current = docker.inspect('container', cid)
            if (current.get('Id') != obj.get('Id') or current.get('Image') != obj.get('Image') or
                    current.get('Config') != obj.get('Config')):
                raise Refused('container identity changed before fencing')
            docker.text(['update', '--restart=no', cid])
            docker.text(['stop', '-t', '120', cid], timeout=150)
            observed = docker.inspect('container', cid)
            state = observed['State']
            restart = observed['HostConfig']['RestartPolicy']['Name']
            good = (observed['Image'] == obj['Image'] and state.get('Running') is False and
                    state.get('Status') in ('exited', 'created') and not state.get('Restarting') and restart == 'no')
            hold['observed_state'].append({'container_id': cid, 'service': service,
                                           'running': state.get('Running'), 'status': state.get('Status'), 'restart_policy': restart})
            if not good:
                raise Refused('container not cold stopped')
            hold['stopped_services'].append(service)
        except Exception:
            hold['errors'].append('fencing-failed:' + cid)
    try:
        after = inventory(docker, hold)
        for cid, obj, service in after:
            if (obj['State'].get('Running') is not False or obj['State'].get('Restarting') or
                    obj['State'].get('Status') not in ('exited', 'created') or
                    obj['HostConfig']['RestartPolicy']['Name'] != 'no' or cid not in {e[0] for e in entries}):
                hold['errors'].append('post-fence-drift:' + cid)
    except Exception:
        hold['errors'].append('post-fence-inspection-failed')
    hold['state'] = ('RECOVERY_REQUIRED_UNKNOWN' if hold['errors'] else
                     ('ARMED_RECOVERY_REQUIRED' if preparing else 'STOPPED_RECOVERY_REQUIRED'))
    # This is the audit receipt. No env, credentials, SQL or raw task payloads.
    write_hold(directory, hold)
    if hold['errors']:
        raise Refused('cold stop incomplete; recovery-required hold retained')
    print('STOPPED_RECOVERY_REQUIRED')


def arm(args, docker):
    proof = capability.authenticated_proof(args.proof)
    model, policy = capability.rendered_policy(docker, args.compose_file, args.env_file, proof['image_reference'])
    for path, field in ((args.compose_file, 'compose_hash'), (args.env_file, 'env_hash')):
        if file_hash(path) != proof.get(field):
            raise Refused('cold protected configuration changed')
    if sha(canonical(model).encode('utf-8')) != proof.get('bound_config_hash'):
        raise Refused('cold bound configuration changed')
    target_id = capability.verify(proof.get('runtime_capability'), proof['image_reference'], proof['git_sha'], policy)
    cold = verify_policy(proof, model, docker, args.receipt)
    lock = os.path.join(args.prestage_dir, 'release.lock')
    with open(os.path.join(lock, 'owner_token')) as stream:
        if not args.owner or stream.read().strip() != args.owner:
            raise Refused('cold arm requires owned release lock')
    os.mkdir(os.path.join(args.prestage_dir, HOLD), 0o700)
    hold = {'version': 1, 'state': 'ARMED_RECOVERY_REQUIRED', 'release_sha': proof['git_sha'],
            'proof_nonce': proof['proof_nonce'], 'proof_sha256': file_hash(args.proof), 'owner': args.owner,
            'project': cold['project'], 'image_ids': [target_id, cold['baseline_identity']['local_image_id']],
            'failure_stage': args.stage, 'trigger_reason': 'cold-policy-armed', 'stopped_services': [], 'observed_state': []}
    initial = inventory(docker, hold)
    hold['initial_running_services'] = sorted({service for cid, obj, service in initial
                                               if obj['State'].get('Running') and
                                               (obj.get('Config', {}).get('Labels') or {}).get('com.docker.compose.project') == cold['project']})
    write_hold(args.prestage_dir, hold)
    print(cold['config_path'])


def complete(args, docker):
    hold = read_hold(args.prestage_dir)
    if (hold['owner'] != args.owner or hold['proof_sha256'] != file_hash(args.proof) or
            hold['state'] != 'ARMED_RECOVERY_REQUIRED'):
        raise Refused('only the same successful in-flight release may finish its hold')
    proof = capability.authenticated_proof(args.proof)
    model, policy = capability.rendered_policy(docker, args.compose_file, args.env_file, proof['image_reference'])
    lock = os.path.join(args.prestage_dir, 'release.lock')
    with open(os.path.join(lock, 'owner_token')) as stream:
        if stream.read().strip() != args.owner:
            raise Refused('completion requires current release lock owner')
    # Deploy intentionally persists the verified image reference in the env
    # file before completion, so bind effective config rather than old env bytes.
    if file_hash(args.compose_file) != proof.get('compose_hash'):
        raise Refused('completion protected Compose bytes changed')
    if sha(canonical(model).encode('utf-8')) != proof.get('bound_config_hash'):
        raise Refused('completion effective configuration drift')
    ledger = read_json(args.ledger)
    consumed = [entry for entry in ledger.get('entries', []) if entry.get('nonce') == proof['proof_nonce']]
    if (len(consumed) != 1 or consumed[0].get('status') != 'CONSUMED' or
            consumed[0].get('git_sha') != proof['git_sha'] or
            consumed[0].get('image_reference') != proof['image_reference']):
        raise Refused('completion requires exact consumed forward release ledger entry')
    with open(args.env_file, encoding='utf-8') as stream:
        desired = [line.split('=', 1)[1].strip().strip('\"\'') for line in stream
                   if line.startswith('XIANZHI_IMAGE_REFERENCE=')]
    if desired != [proof['image_reference']]:
        raise Refused('completion desired image was not persisted')
    capability.verify(proof.get('runtime_capability'), proof['image_reference'], proof['git_sha'], policy)
    model['_compose_file'], model['_env_file'] = args.compose_file, args.env_file
    capability.verify_running_processes(docker, model, proof['runtime_capability'], proof['image_reference'])
    # Observe final forward/health state independently; no direct SQL, broker
    # or provider commands. Existing ordinary deploy catalog gate is mandatory.
    base = ['compose', '-f', args.compose_file, '--env-file', args.env_file]
    migrate = docker.text(base + ['ps', '-a', '-q', 'migrate'])
    if not migrate or '\n' in migrate:
        raise Refused('completion migration singleton unavailable')
    migration = docker.inspect('container', migrate)
    state = migration['State']
    runner = migration_identity(model, docker)
    if runner != proof['cold_recovery_policy'].get('migration_runner'):
        raise Refused('completion migration runner changed after prestage')
    labels = migration.get('Config', {}).get('Labels') or {}
    if (migration['Image'] != runner['local_image_id'] or migration['Config']['Image'] != runner['reference'] or
            labels.get('com.docker.compose.project') != hold['project'] or
            labels.get('com.docker.compose.service') != 'migrate' or
            state.get('Running') is not False or state.get('Status') != 'exited' or
            type(state.get('ExitCode')) is not int or state['ExitCode'] != 0):
        raise Refused('completion migration not successfully exited on target image')
    health = read_health(docker, base, 'health')
    ready = read_health(docker, base, 'ready')
    async_enabled = str(model['services']['xianzhi-ai'].get('environment', {}).get('ASYNC_MESSAGING_ENABLED', '')).lower() == 'true'
    if (health.get('status') != 'ok' or ready.get('ready') is not True or
            (async_enabled and ready.get('asyncMessaging') != 'READY')):
        raise Refused('completion API health/readiness unavailable')
    worker = docker.text(base + ['ps', '-q', 'smartvideo-worker'])
    if not worker or '\n' in worker or docker.inspect('container', worker)['State'].get('Health', {}).get('Status') != 'healthy':
        raise Refused('completion smartvideo worker unhealthy')
    # Ancillary availability must also be restored.
    found = set()
    observed = []
    for cid, obj, service in inventory(docker, hold):
        state = obj['State']
        if (obj['HostConfig']['RestartPolicy']['Name'] != 'no' or state.get('Restarting') or
                state.get('Running') not in (True, False) or
                state.get('Status') not in ('running', 'exited', 'created')):
            raise Refused('cold successful release restart policy/state mismatch')
        observed.append({'container_id': cid, 'service': service, 'running': obj['State'].get('Running'),
                         'status': obj['State'].get('Status'), 'restart_policy': 'no'})
        if obj['State'].get('Running'):
            cfg = model['services'].get(service)
            labels = obj.get('Config', {}).get('Labels') or {}
            if (not cfg or obj['Image'] == hold['image_ids'][1] or
                    labels.get('com.docker.compose.project') != hold['project']):
                raise Refused('old or unknown business image still running')
            expected_id = docker.inspect('image', cfg['image'])['Id']
            if obj['Image'] != expected_id or obj['Config']['Image'] != cfg['image']:
                raise Refused('ancillary service image identity mismatch')
            found.add(service)
    if not set(policy['roles']).issubset(found):
        raise Refused('cold successful release role coverage incomplete')
    required = set(hold.get('initial_running_services', [])) - {'migrate'}
    if not required.issubset(found):
        raise Refused('ancillary availability not restored; recovery remains required')
    with open(os.path.join(lock, 'owner_token')) as stream:
        if stream.read().strip() != args.owner:
            raise Refused('completion release lock ownership changed during observation')
    current = read_hold(args.prestage_dir)
    if current != hold:
        raise Refused('completion hold changed during observation')
    hold['observed_state'] = observed
    hold['restored_services'] = sorted(found)
    hold['intentionally_stopped_services'] = sorted(set(hold.get('stopped_services', [])) - found)
    # Preserve successful audit before removing only our own hold. Never resume.
    hold.update(state='FORWARD_VERIFIED', failure_stage='complete', trigger_reason='same-inflight-success')
    audit = os.path.join(args.prestage_dir, 'cold-forward-' + proof['proof_nonce'] + '.json')
    atomic_json(audit, signed(hold))
    os.unlink(os.path.join(args.prestage_dir, HOLD, 'state.json'))
    os.rmdir(os.path.join(args.prestage_dir, HOLD))


def read_health(docker, base, endpoint):
    return json.loads(docker.text(base + ['exec', '-T', 'xianzhi-ai', 'curl', '-fsS', '--max-time', '10',
                                          'http://127.0.0.1:3100/api/v1/' + endpoint]))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=('arm', 'prepare', 'fence', 'complete'))
    parser.add_argument('--proof')
    parser.add_argument('--receipt')
    parser.add_argument('--compose-file', default='compose.prod.yml')
    parser.add_argument('--env-file', default='.env.production')
    parser.add_argument('--prestage-dir', default='.prestage')
    parser.add_argument('--owner')
    parser.add_argument('--ledger', default='backups/release-ledger.json')
    parser.add_argument('--stage', default='unknown')
    parser.add_argument('--reason', choices=('exit-failure', 'signal-INT', 'signal-TERM', 'explicit-cold-rollback', 'reentry'), default='exit-failure')
    args = parser.parse_args()
    docker = capability.Docker()
    if args.action == 'arm':
        arm(args, docker)
    elif args.action in ('fence', 'prepare'):
        if args.action == 'fence' and args.owner and not args.proof:
            raise Refused('owned cleanup requires its proof identity')
        fence(args.prestage_dir, args.stage, args.reason, docker, args.action == 'prepare', args.owner, args.proof)
    else:
        complete(args, docker)


if __name__ == '__main__':
    try:
        main()
    except Exception:
        # Docker/helper errors may contain env data. Do not echo raw exceptions.
        sys.stderr.write('COLD_RECOVERY_REFUSED: state unknown or authorization invalid; hold retained if armed\n')
        sys.exit(1)
