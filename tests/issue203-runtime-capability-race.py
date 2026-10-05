#!/usr/bin/env python3
"""Owned internal-DB CGO race regression; NOT packaged runtime attestation."""
import json
import os
import subprocess
import sys
import types
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EVIDENCE = ROOT / '.evidence/issue203/priority4-runtime-capability'
sys.dont_write_bytecode = True
path = ROOT / 'ops/verify-image-quarantine-capability.py'
capability = types.ModuleType('capability_regression_fixture')
capability.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), capability.__dict__)


def main():
    docker = capability.Docker()
    fixture = capability.Fixture(docker)
    with (EVIDENCE / 'synthetic-evidence-final.json').open(encoding='utf-8') as stream:
        identity = json.load(stream)['identity']
    try:
        fixture.provision(identity['local_image_id'])
        image_id = docker.inspect('image', 'golang:1.25-bookworm')['Id']
        cache = subprocess.check_output(['go', 'env', 'GOMODCACHE'], cwd=str(ROOT / 'backend-go')).decode().strip()
        dsn = 'postgres://fixture_admin:' + fixture.password + '@fixture-db:5432/' + fixture.name + '?sslmode=disable'
        args = ['create', '--pull', 'never', '--label', capability.LABEL + '=' + fixture.owner, '--network', fixture.network,
                '--mount', 'type=bind,source=' + str(ROOT) + ',target=/workspace,readonly',
                '--mount', 'type=bind,source=' + cache + ',target=/go/pkg/mod,readonly',
                '-w', '/workspace/backend-go', '-e', 'CGO_ENABLED=1', '-e', 'GOTOOLCHAIN=local',
                '-e', 'GOPROXY=off', '-e', 'GOSUMDB=off']
        for key in ('XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL', 'XIANZHI_TEST_DATABASE_URL', 'PERSONAL_POINTS_POSTGRES_TEST_DSN', 'XIANZHI_PERSONAL_POINT_TEST_DATABASE_URL'):
            args += ['-e', key + '=' + dsn]
        args += [image_id, 'go', 'test', '-json', '-race', '-count=1', '-mod=readonly', './internal/providerexecution', './internal/httpserver', '-run', 'Quarantine|Timing']
        cid = fixture.record('container', docker.text(args))
        result = docker.run(['start', '-a', cid], timeout=540, check=False)
        text = result.stdout.decode('utf-8', errors='replace').replace(fixture.password, '[redacted]')
        errors = result.stderr.decode('utf-8', errors='replace').replace(fixture.password, '[redacted]')
        (EVIDENCE / 'cgo-race-go-json.log').write_text(text + errors, encoding='utf-8')
        state = fixture.owned('container', cid)['State']
        counts = {'pass': 0, 'fail': 0, 'skip': 0}
        for line in text.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if event.get('Test') and event.get('Action') in counts:
                counts[event['Action']] += 1
        evidence = {'owner': fixture.owner, 'db_id': fixture.db, 'go_test_id': cid, 'image_id': image_id, 'counts': counts, 'exit_code': state['ExitCode'], 'packaged_proof': False}
        (EVIDENCE / 'cgo-race-result.json').write_text(json.dumps(evidence, indent=2, sort_keys=True), encoding='utf-8')
        print(json.dumps(evidence, sort_keys=True))
        if state['ExitCode'] or counts['skip'] or counts['fail'] or not counts['pass']:
            raise RuntimeError('CGO race regression failed/no selected tests; no SKIP acceptance')
    finally:
        fixture.cleanup()

if __name__ == '__main__':
    main()
