#!/usr/bin/env python3
"""Owned test-fixture EXIT cleanup and independent fail-closed sanitization.

Exact IDs/owner/project are rechecked before Docker mutation. Sanitization never
needs a working daemon. This helper is not a production recovery entry.
"""
import json
from pathlib import Path
import re
import sys
import types

ROOT = Path(__file__).resolve().parents[1]
path = ROOT / 'ops/verify-image-quarantine-capability.py'
cap = types.ModuleType('cleanup_capability')
cap.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), cap.__dict__)
OUT = ROOT / '.evidence/cold-recovery-resume'
PRIVATE = ('fixture.env', 'compose.json', 'fixture-release-trust.key')


def remove(docker, kind, identity, label, owner, project):
    if kind not in ('container', 'network', 'volume'):
        raise RuntimeError('unexpected fixture kind')
    if not re.fullmatch('[0-9a-f]{32}', owner):
        raise RuntimeError('invalid fixture owner')
    if kind == 'volume':
        if not identity.startswith('resume-real-' + owner + '_'):
            raise RuntimeError('unexpected fixture volume')
    elif not re.fullmatch('[0-9a-f]{64}', identity):
        raise RuntimeError('full fixture ID required')
    result = docker.run([kind, 'inspect', identity], check=False)
    if result.returncode:
        if b'No such' in result.stderr:
            return 'already-absent'
        raise RuntimeError('fixture identity observation failed')
    objects = json.loads(result.stdout.decode('utf-8'))
    if len(objects) != 1:
        raise RuntimeError('ambiguous fixture')
    obj = objects[0]
    labels = obj.get('Config', {}).get('Labels', {}) if kind == 'container' else obj.get('Labels', {})
    actual = obj.get('Name') if kind == 'volume' else obj.get('Id')
    if actual != identity or labels.get(label) != owner:
        raise RuntimeError('foreign cleanup refused')
    if labels.get('com.docker.compose.project') != project:
        raise RuntimeError('foreign project cleanup refused')
    docker.text(['rm', '-f', '-v', identity] if kind == 'container' else [kind, 'rm', identity])
    return 'removed'


def backend_manifests():
    backend = OUT / 'backend'
    if backend.is_symlink() or OUT.is_symlink():
        raise RuntimeError('fixture evidence symlink refused')
    if not backend.exists():
        return []
    records = []
    for directory in sorted(backend.iterdir()):
        if directory.is_symlink() or not directory.is_dir() or not re.fullmatch('[0-9a-f]{32}', directory.name):
            raise RuntimeError('unowned backend evidence refused')
        record = directory / 'resources.json'
        if record.is_symlink() or not record.is_file():
            raise RuntimeError('backend ownership manifest missing')
        data = json.loads(record.read_text(encoding='utf-8'))
        if data.get('owner') != directory.name or data.get('project') != 'resume-real-' + directory.name or type(data.get('resources')) is not list:
            raise RuntimeError('invalid backend manifest')
        records.append((directory, data))
    return records


def sanitize_private():
    """Remove only named private files in proven fixture dirs; fail upload closed."""
    status, removed = 'failed', []
    try:
        for directory, data in backend_manifests():
            for name in PRIVATE:
                private = directory / name
                if private.is_symlink() or (private.exists() and not private.is_file()):
                    raise RuntimeError('fixture private path refused')
                if private.exists():
                    private.unlink()
                    removed.append(directory.name + '/' + name)
                if private.exists() or private.is_symlink():
                    raise RuntimeError('fixture private deletion unproved')
        status = 'passed'
    finally:
        if OUT.is_symlink() or (OUT / 'fixture-sanitization.json').is_symlink():
            raise RuntimeError('sanitization receipt path refused')
        OUT.mkdir(parents=True, exist_ok=True)
        (OUT / 'fixture-sanitization.json').write_text(json.dumps(dict(status=status, removed=removed), indent=2, sort_keys=True) + '\n', encoding='utf-8')


def main():
    results, status = [], 'failed-ownership-or-observation; no broad cleanup attempted'
    try:
        docker = cap.Docker()
        for directory, data in backend_manifests():
            owner, project = data['owner'], data['project']
            for kind, identity in reversed(data['resources']):
                result = remove(docker, kind, identity, 'org.xianzhi.cold-resume.backend-owner', owner, project)
                results.append(dict(kind=kind, identity=identity, result=result))
        for record in sorted((OUT / 'request').glob('*.jsonl')):
            if record.is_symlink():
                raise RuntimeError('request manifest symlink refused')
            for line in record.read_text(encoding='utf-8').splitlines():
                data = json.loads(line)
                if data['event'] != 'owned-resource-recorded': continue
                owner, project, cid = data['owner'], data['project'], data['cid']
                if project not in ('request-' + owner, 'foreign-' + owner) or owner not in record.name:
                    raise RuntimeError('invalid request manifest')
                result = remove(docker, 'container', cid, 'org.xianzhi.cold-resume.request-owner', owner, project)
                results.append(dict(kind='container', identity=cid, result=result))
        status = 'passed'
    finally:
        # Docker observation/removal failures cannot bypass private-file cleanup.
        try:
            sanitize_private()
        finally:
            if OUT.is_symlink() or (OUT / 'owned-cleanup.json').is_symlink():
                raise RuntimeError('cleanup receipt path refused')
            OUT.mkdir(parents=True, exist_ok=True)
            (OUT / 'owned-cleanup.json').write_text(json.dumps(dict(status=status, resources=results), indent=2, sort_keys=True) + '\n', encoding='utf-8')


if __name__ == '__main__':
    if sys.argv[1:] == ['--sanitize-only']:
        sanitize_private()
    elif sys.argv[1:]:
        raise RuntimeError('unknown fixture cleanup option')
    else:
        main()
