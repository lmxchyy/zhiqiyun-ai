#!/usr/bin/env python3
"""Mandatory Ubuntu local packaged/transport job, never an official Proof.

Only the exact local Linux Docker socket, a UUID-labelled local registry and
new source-built ALL4 inventory qualify. Identity-only input is not behavior.
The transport harness generates fresh actual strict300s behavior; runner900s
is a hard host limit. No shared DB/broker, template pooling or release key.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / '.evidence/issue203/priority4-runtime-capability'
LABEL = 'issue203.safe-drain-ci.owner'
BINARIES = ('xianzhi-api', 'generation-worker', 'smartvideo-worker', 'video-backfill')


def source_hashes():
    paths = set()
    for directory, suffixes in (('backend-go', ('.go', '.mod', '.sum')), ('database', ('.sql',)), ('ops', ('.py', '.sh'))):
        paths.update(p for p in (ROOT / directory).rglob('*') if p.is_file() and p.suffix in suffixes)
    paths.update(ROOT / name for name in ('rollback.sh', 'tests/issue203-control-plane-test.py',
                 'tests/issue203-historical-drain-test.py', 'tests/issue203-safe-drain-transport-test.py',
                 'tests/issue203-safe-drain-transport.Dockerfile', 'tests/issue203-packaged-ci.Dockerfile',
                 'tests/issue203-safe-drain-ci.py', '.github/workflows/user-core.yml'))
    return {p.relative_to(ROOT).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(paths)}


def main():
    if sys.platform != 'linux' or not Path('/var/run/docker.sock').is_socket():
        raise RuntimeError('mandatory native Linux local Docker socket absent; no SKIP')
    release = os.environ.get('GITHUB_SHA', '')
    if not re.fullmatch('[0-9a-f]{40}', release):
        raise RuntimeError('CI exact source SHA required')
    env = {k: v for k, v in os.environ.items() if not k.upper().startswith(('DOCKER_', 'COMPOSE_', 'PG', 'POSTGRES', 'DATABASE', 'XIANZHI_', 'RELEASE_TRUST', 'QUARANTINE_', 'PROVIDER', 'RABBITMQ', 'REDIS'))}
    env['DOCKER_HOST'] = 'unix:///var/run/docker.sock'
    owner = uuid.uuid4().hex
    registry, network = 'issue203-registry-' + owner, 'issue203-registry-net-' + owner
    tag = None
    resources = []
    OUT.mkdir(parents=True, exist_ok=True)
    hashes = source_hashes()
    log_path = OUT / (owner + '-ci.log')
    deadline = time.monotonic() + 300
    def run(args, timeout=60, check=True):
        with log_path.open('ab') as log:
            result = subprocess.run(args, cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=log, timeout=timeout)
            log.write(result.stdout)
        if check and result.returncode:
            raise RuntimeError('CI prerequisite failed: ' + args[0] + '; see ' + str(log_path))
        return result.stdout.decode().strip()
    def docker(args, timeout=60, check=True):
        return run(['docker', '--host', 'unix:///var/run/docker.sock'] + args, timeout, check)
    def owned(kind, identity):
        obj = json.loads(docker([kind, 'inspect', identity]))[0]
        labels = obj.get('Labels') if kind == 'network' else obj.get('Config', {}).get('Labels')
        if (labels or {}).get(LABEL) != owner:
            raise RuntimeError('CI resource ownership drift')
        return obj
    try:
        with tempfile.TemporaryDirectory(prefix='issue203-all4-') as work:
            context = Path(work)
            for binary, package in zip(BINARIES, ('api', 'generation-worker', 'smartvideo-worker', 'video-backfill')):
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise RuntimeError('ALL4 package build300 deadline exceeded')
                build_env = dict(env, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
                with log_path.open('ab') as log:
                    result = subprocess.run(['go', 'build', '-trimpath', '-ldflags', '-X xianzhi-ai/backend-go/internal/httpserver.CapabilityReleaseSHA=' + release,
                                             '-o', str(context / binary), './cmd/' + package], cwd=ROOT / 'backend-go', env=build_env,
                                            stdout=log, stderr=log, timeout=remaining)
                if result.returncode:
                    raise RuntimeError('ALL4 source build failed: ' + binary)
            shutil.copyfile(str(ROOT / 'tests/issue203-packaged-ci.Dockerfile'), str(context / 'Dockerfile'))
            resources.append(('network', docker(['network', 'create', '--label', LABEL + '=' + owner, network])))
            resources.append(('container', docker(['run', '-d', '--pull', 'never', '--name', registry, '--network', network,
                '--label', LABEL + '=' + owner, '-p', '127.0.0.1::5000', 'registry:2'])))
            obj = owned('container', resources[-1][1])
            ports = obj['NetworkSettings']['Ports']['5000/tcp']
            if len(ports) != 1 or ports[0]['HostIp'] != '127.0.0.1':
                raise RuntimeError('registry must be loopback-only')
            tag = '127.0.0.1:' + ports[0]['HostPort'] + '/issue203-all4-' + owner + ':fixture'
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RuntimeError('ALL4 package build300 deadline exceeded')
            docker(['build', '--platform', 'linux/amd64', '--label', LABEL + '=' + owner, '-t', tag, str(context)], timeout=remaining)
            owned('image', tag)
            for unused in range(20):
                if docker(['exec', registry, 'wget', '-q', '-O', '-', 'http://127.0.0.1:5000/v2/'], check=False) == '{}':
                    break
                time.sleep(.25)
            else:
                raise RuntimeError('owned registry not ready')
            # Push ONLY to this owned loopback fixture registry, no publication.
            docker(['push', tag], timeout=60)
            obj = owned('image', tag)
            digests = obj.get('RepoDigests', [])
            if len(digests) != 1 or not digests[0].startswith(tag.split(':fixture')[0] + '@sha256:'):
                raise RuntimeError('immutable local registry digest absent')
            import types
            path = ROOT / 'ops/verify-image-quarantine-capability.py'
            capability = types.ModuleType('ci_identity')
            capability.__file__ = str(path)
            exec(compile(path.read_bytes(), str(path), 'exec'), capability.__dict__)
            identity = capability.image_identity(capability.Docker(), digests[0], release)
            if source_hashes() != hashes:
                raise RuntimeError('source changed while compiling packaged inventory')
            (OUT / 'synthetic-evidence-final.json').write_text(json.dumps(dict(synthetic_nonofficial=True,
                identity_only_nonbehavior=True, identity=identity, source_sha256=hashes), indent=2, sort_keys=True))
        # Full package and transport phases run together under existing host900.
        result = subprocess.run([sys.executable, str(ROOT / 'tests/issue203-safe-drain-transport-test.py')], cwd=ROOT, env=env)
        if result.returncode or source_hashes() != hashes:
            raise RuntimeError('mandatory transport/package failed or source drift; no accepted proof')
        print('CI_REQUIRED_RESULT ALL4=4 roles=2 transport=15 skips=0 NONOFFICIAL')
    finally:
        errors = []
        if tag:
            try:
                owned('image', tag)
                docker(['image', 'rm', tag])  # own tag only, never shared cache IDs
            except Exception as error:
                errors.append(str(error))
        for kind, identity in reversed(resources):
            try:
                owned(kind, identity)
                docker([kind, 'rm'] + (['-f', '-v'] if kind == 'container' else []) + [identity])
            except Exception as error:
                errors.append(str(error))
        (OUT / (owner + '-cleanup.json')).write_text(json.dumps(dict(owner=owner, resources=resources, cleanup='failed' if errors else 'complete', errors=errors)))
        if errors:
            raise RuntimeError('CI owned cleanup incomplete')


if __name__ == '__main__':
    main()
