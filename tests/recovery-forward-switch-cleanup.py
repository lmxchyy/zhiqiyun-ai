#!/usr/bin/env python3
"""Forward CI fixture cleanup only: recorded exact IDs, rechecked UUID owners.

No production paths, down/prune/project-wide deletion or stale-lock reclamation.
Private-file sanitization is independent of Docker availability.
"""
import json
import os
from pathlib import Path
import re
import sys
import types

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / '.evidence/forward-switch'
path = ROOT / 'ops/verify-image-quarantine-capability.py'
cap = types.ModuleType('forward_fixture_cleanup'); cap.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), cap.__dict__)
BACKEND_LABEL = 'org.xianzhi.cold-resume.backend-owner'
HASH_LABEL = 'org.xianzhi.release213.forward-nonce'


def read(path):
    if path.is_symlink() or not path.is_file(): raise RuntimeError('unsafe ownership manifest')
    return json.loads(path.read_text(encoding='utf-8'))


def backend_manifest(directory):
    if directory.is_symlink() or not re.fullmatch('[0-9a-f]{32}', directory.name):
        raise RuntimeError('unowned fixture directory')
    data = read(directory / 'resources.json')
    if data.get('owner') != directory.name or data.get('project') != 'resume-real-' + directory.name or type(data.get('resources')) is not list:
        raise RuntimeError('invalid backend manifest')
    return data


def remove(docker, kind, identity, label, owner, project=None):
    if kind not in ('container', 'network', 'volume') or not re.fullmatch('[0-9a-f-]{32,36}', owner):
        raise RuntimeError('invalid owned resource')
    if kind == 'volume':
        if not project or not identity.startswith(project + '_'): raise RuntimeError('foreign volume name')
    elif not re.fullmatch('[0-9a-f]{64}', identity): raise RuntimeError('exact full ID required')
    result = docker.run([kind, 'inspect', identity], check=False)
    if result.returncode:
        if b'No such' in result.stderr: return 'already-absent'
        raise RuntimeError('resource observation failed')
    rows = json.loads(result.stdout.decode())
    if len(rows) != 1: raise RuntimeError('ambiguous resource')
    obj = rows[0]
    labels = obj.get('Config', {}).get('Labels', {}) if kind == 'container' else obj.get('Labels', {})
    if (obj.get('Name') if kind == 'volume' else obj.get('Id')) != identity or labels.get(label) != owner:
        raise RuntimeError('foreign cleanup refused')
    if project and labels.get('com.docker.compose.project') != project: raise RuntimeError('foreign project')
    docker.text(['rm', '-f', '-v', identity] if kind == 'container' else [kind, 'rm', identity])
    return 'removed'


def sanitize_backend(directory):
    backend_manifest(directory)
    private_paths = [directory / name for name in ('fixture.env', 'compose.json', 'fixture-release-trust.key', 'rabbitmq-inspect-private.json')]
    prestage = directory / 'prestage'
    if prestage.is_symlink(): raise RuntimeError('unsafe fixture prestage')
    if prestage.exists():
        private_paths += list(prestage.rglob('forward-compose-*.json'))
    for private in private_paths:
        if private.is_symlink() or private.exists() and not private.is_file(): raise RuntimeError('unsafe private path')
        if private.exists(): private.unlink()
        if private.exists() or private.is_symlink(): raise RuntimeError('private removal unproved')


def cleanup_backend(directory):
    data = backend_manifest(directory)
    docker = cap.Docker()
    results = []
    try:
        for kind, identity in reversed(data['resources']):
            results.append({'kind': kind, 'identity': identity,
                            'result': remove(docker, kind, identity, BACKEND_LABEL, data['owner'], data['project'])})
        # Detect delayed/unrecorded fixture acceptance; NEVER delete those by label.
        if docker.text(['ps', '-aq', '--no-trunc', '--filter', 'label='+BACKEND_LABEL+'='+data['owner']]):
            raise RuntimeError('unrecorded fixture container remains; no broad deletion')
        if docker.text(['network', 'ls', '-q', '--no-trunc', '--filter', 'label='+BACKEND_LABEL+'='+data['owner']]):
            raise RuntimeError('unrecorded fixture network remains')
        if docker.text(['volume', 'ls', '-q', '--filter', 'label='+BACKEND_LABEL+'='+data['owner']]):
            raise RuntimeError('unrecorded fixture volume remains')
        (directory / 'cleanup.json').write_text(json.dumps({'status': 'passed', 'results': results})+'\n', encoding='utf-8')
    finally:
        sanitize_backend(directory)


def directories():
    backend = OUT / 'backend'
    if OUT.is_symlink() or backend.is_symlink(): raise RuntimeError('unsafe evidence root')
    return list(sorted(backend.iterdir())) if backend.exists() else []


def attestation_kind(path):
    data = read(path)
    owner_pattern = r'(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})'
    journal = re.fullmatch(r'(owned|hash)-(' + owner_pattern + r')\.json', path.name)
    if journal:
        if data.get('owner') != journal.group(2) or type(data.get('resources')) is not list:
            raise RuntimeError('unowned attestation journal')
        return journal.group(1), data
    if path.name == 'certification.json' and type(data) is dict:
        return 'certificate', data
    observation = re.fullmatch(r'observation-(' + owner_pattern + r')(?:-(history|history-control))?\.json', path.name)
    if observation and type(data) is dict and {'role', 'phase', 'image_identity', 'before', 'after', 'effects', 'exit_code', 'stdout'} <= set(data):
        if observation.group(2) and (data.get('owner') != observation.group(1) or data.get('phase') != observation.group(2)):
            raise RuntimeError('invalid history observation')
        return 'observation', data
    raise RuntimeError('unknown or malformed attestation evidence')


def attest_cleanup():
    directory = OUT / 'attest'
    if directory.is_symlink(): raise RuntimeError('unsafe attestation directory')
    docker = cap.Docker()
    for path in sorted(directory.glob('*.json')):
        family, data = attestation_kind(path)
        if family in ('certificate', 'observation'): continue
        label = HASH_LABEL if family == 'hash' else cap.LABEL
        for kind, identity in reversed(data['resources']):
            remove(docker, kind, identity, label, data['owner'])
        if docker.text(['ps', '-aq', '--no-trunc', '--filter', 'label='+label+'='+data['owner']]):
            raise RuntimeError('unrecorded attester resource remains')
        if docker.text(['network', 'ls', '-q', '--no-trunc', '--filter', 'label='+label+'='+data['owner']]):
            raise RuntimeError('unrecorded attester network remains')


def main(sanitize_only=False):
    status = 'failed'
    try:
        if not sanitize_only:
            for directory in directories(): cleanup_backend(directory)
            attest_cleanup()
        status = 'passed'
    finally:
        for directory in directories(): sanitize_backend(directory)
        OUT.mkdir(parents=True, exist_ok=True)
        receipt = OUT / ('fixture-sanitization.json' if sanitize_only else 'owned-cleanup.json')
        if receipt.is_symlink(): raise RuntimeError('unsafe receipt')
        receipt.write_text(json.dumps({'status': status})+'\n', encoding='utf-8')


def export_sanitized(destination):
    """Copy only sanitized receipt families; never expose the private root."""
    if read(OUT / 'fixture-sanitization.json').get('status') != 'passed':
        raise RuntimeError('sanitization gate not passed')
    destination = Path(destination)
    if not destination.is_dir() or any(p.is_symlink() for p in [destination] + list(destination.parents)):
        raise RuntimeError('unsafe artifact staging directory')
    paths = [OUT / name for name in ('native.log', 'owned-cleanup.json', 'fixture-sanitization.json')]
    attest = OUT / 'attest'
    if attest.is_symlink(): raise RuntimeError('unsafe attestation directory')
    for path in sorted(attest.glob('*.json')):
        family, _ = attestation_kind(path)
        if family in ('certificate', 'observation'): paths.append(path)
    for directory in directories():
        backend_manifest(directory)
        paths += [directory / name for name in ('certification.json', 'switch-receipt.json',
                  'provenance-boundary.json', 'cleanup.json', 'commands.jsonl', 'postgres-statements.log')]
    total = 0
    for path in paths:
        if any(p.is_symlink() for p in [path] + list(path.parents)):
            raise RuntimeError('unsafe receipt source')
        if not path.exists(): continue
        if not path.is_file() or path.stat().st_size > 32 * 1024 * 1024:
            raise RuntimeError('unbounded receipt')
        payload = path.read_bytes(); total += len(payload)
        if total > 256 * 1024 * 1024: raise RuntimeError('unbounded artifact total')
        target = destination / path.relative_to(OUT)
        target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
        if any(p.is_symlink() for p in [target] + list(target.parents)):
            raise RuntimeError('unsafe receipt destination')
        fd = os.open(str(target), os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0), 0o644)
        with os.fdopen(fd, 'wb') as stream: stream.write(payload)
        os.chmod(str(target), 0o644)


if __name__ == '__main__':
    if len(sys.argv) == 3 and sys.argv[1] == '--export-sanitized':
        export_sanitized(sys.argv[2])
    else:
        if sys.argv[1:] not in ([], ['--sanitize-only']): raise RuntimeError('unexpected cleanup option')
        main(sys.argv[1:] == ['--sanitize-only'])
