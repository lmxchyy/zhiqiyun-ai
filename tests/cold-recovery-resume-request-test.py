#!/usr/bin/env python3
"""Original owned Engine requests through a test-only Unix HTTP fault relay.

Captured Engine 204 proves completion. A write/close before a response does NOT
prove handler acceptance or pre-lock phase. Nothing retries an unknown POST.
"""
import http.client
import http.server
import json
import multiprocessing
import os
from pathlib import Path
import re
import signal
import socket
import socketserver
import subprocess
import sys
import tempfile
import threading
import time
import types
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[1]
r = types.ModuleType('resume_request')
r.__file__ = str(ROOT / 'ops/cold-recovery-resume.py')
exec(compile(Path(r.__file__).read_bytes(), r.__file__, 'exec'), r.__dict__)
LABEL = 'org.xianzhi.cold-resume.request-owner'
OUT = ROOT / '.evidence/cold-recovery-resume/request'


def event(log_path, kind, **fields):
    fields.update(event=kind, monotonic=time.monotonic(), wall=time.time())
    fd = os.open(str(log_path), os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(fd, (json.dumps(fields, sort_keys=True) + '\n').encode())
    finally:
        os.close(fd)


class Engine(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect('/var/run/docker.sock')


def serve(path, log, cids, fault, mode, ready):
    """Separate process: the controller may fork without inheriting relay threads."""
    counts, mutex = {}, threading.Lock()
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = 'HTTP/1.1'
        def log_message(self, *args):
            pass
        def relay(self):
            event(log, 'handler-enter', method=self.command, path=self.path)
            upstream = None
            try:
                clean = re.sub(r'^/v[0-9.]+', '', self.path).split('?')[0]
                mutation = self.command == 'POST'
                match = re.fullmatch(r'/containers/([0-9a-f]{64})/(start|stop|update)', clean)
                allowed = (self.command in ('GET', 'HEAD') and clean in ('/_ping', '/version')) or (match and match.group(1) in cids)
                if not allowed:
                    event(log, 'refused-before-forward', method=self.command, path=self.path)
                    self.send_error(403)
                    return
                if mutation:
                    with mutex:
                        counts[clean] = counts.get(clean, 0) + 1
                        # Only the faulted original request is single-use. Fencing
                        # may issue a distinct stop/update, never resend that POST.
                        if clean == fault and counts[clean] > 1:
                            event(log, 'duplicate-refused', path=clean)
                            self.send_error(409)
                            return
                length = int(self.headers.get('Content-Length', '0'))
                if length > 65536:
                    raise ValueError('oversized fixture request')
                body = self.rfile.read(length) if length else None
                event(log, 'received', method=self.command, path=self.path)
                upstream = Engine('localhost', timeout=20)
                upstream.request(self.command, self.path, body=body,
                                 headers={'Content-Type': self.headers.get('Content-Type', 'application/json')})
                event(log, 'upstream-send-complete', method=self.command, path=self.path,
                      acceptance='socket-write-only-not-handler-acceptance')
                affected = mutation and clean == fault
                if affected and mode == 'upstream-close':
                    phase = 'unproved'
                    if clean.endswith('/stop'):
                        end = time.monotonic() + 5
                        while time.monotonic() < end:
                            probe = Engine('localhost', timeout=2)
                            try:
                                probe.request('GET', '/containers/' + match.group(1) + '/archive?path=/tmp/term-accepted')
                                observed = probe.getresponse()
                                body = observed.read()
                                if observed.status == 200 and b'TERM' in body:
                                    phase = 'actual-pid1-term-marker-before-completion'
                                    event(log, 'upstream-fault-term-marker', cid=match.group(1))
                                    break
                            finally:
                                probe.close()
                            time.sleep(.05)
                        if phase == 'unproved':
                            raise RuntimeError('stop fault acceptance marker unavailable')
                    upstream.close()
                    event(log, 'upstream-closed-before-ack', phase=phase)
                    self.close_connection = True
                    return
                response = upstream.getresponse()
                payload = response.read()
                event(log, 'engine-response', method=self.command, path=self.path, status=response.status)
                if affected:
                    if mode == 'disconnect':
                        event(log, 'downstream-disconnected-after-ack')
                        self.close_connection = True
                        return
                    event(log, 'downstream-response-suppressed')
                    self.connection.settimeout(5)
                    try:
                        self.connection.recv(1)
                    except socket.timeout:
                        pass
                    event(log, 'downstream-closed-or-bounded-timeout')
                    self.close_connection = True
                    return
                self.send_response(response.status)
                for header in ('API-Version', 'Docker-Experimental', 'OSType'):
                    if response.getheader(header):
                        self.send_header(header, response.getheader(header))
                self.send_header('Content-Length', str(len(payload)))
                self.end_headers()
                if self.command != 'HEAD':
                    self.wfile.write(payload)
            except (OSError, http.client.HTTPException):
                event(log, 'relay-connection-ended')
                self.close_connection = True
            finally:
                if upstream is not None:
                    upstream.close()
        do_GET = do_HEAD = do_POST = relay
    class Server(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
        daemon_threads = True
    with Server(str(path), Handler) as server:
        os.chmod(str(path), 0o600)
        event(log, 'relay-armed', socket=str(path))
        ready.set()
        server.serve_forever()


class FaultRelay:
    def __init__(self, directory, log, cids, operation, mode):
        self.path = Path(directory) / 'engine.sock'
        self.ready = multiprocessing.Event()
        self.process = multiprocessing.Process(target=serve, args=(self.path, log, cids, operation, mode, self.ready))
        self.process.start()
        if not self.ready.wait(5):
            self.close()
            raise RuntimeError('relay did not arm')
    def close(self):
        if self.process.is_alive():
            self.process.terminate()
        self.process.join(5)
        if self.process.is_alive():
            os.kill(self.process.pid, signal.SIGKILL)
            self.process.join()
        if self.path.exists():
            self.path.unlink()


def facts(docker, cid):
    obj = docker.inspect('container', cid)
    image = docker.inspect('image', obj['Image'])
    return dict(cid=obj['Id'], image=obj['Image'], image_reference=obj['Config']['Image'],
                repo_digests=image.get('RepoDigests', []), platform=[image['Os'], image['Architecture']],
                labels=obj['Config'].get('Labels'), restart_policy=obj['HostConfig']['RestartPolicy'],
                restart_count=obj['RestartCount'],
                state={k: obj['State'].get(k) for k in ('Running', 'Status', 'Paused', 'Restarting', 'StartedAt', 'FinishedAt')})


class OriginalRequestTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if sys.platform != 'linux' or sys.version_info[:3] != (3, 6, 8):
            raise RuntimeError('native Linux Python3.6.8 mandatory; zero skips')
        OUT.mkdir(parents=True, exist_ok=True)
        cls.raw = r.cap.Docker()
        cls.image = cls.raw.inspect('image', 'python:3.6.8-alpine')['Id']
        (OUT / 'engine-local-only.json').write_text(cls.raw.text(['version', '--format', '{{json .}}']) + '\n')

    def setUp(self):
        self.owner = uuid.uuid4().hex
        self.directory = tempfile.TemporaryDirectory(prefix='original-request-')
        self.addCleanup(self.directory.cleanup)
        self.log = OUT / (self.id().split('.')[-1] + '-' + self.owner + '.jsonl')
        self.cids = []
        self.addCleanup(self.cleanup)
        script = ("import os,signal,time\n"
                  "def term(*args):\n open('/tmp/term-accepted','w').write('TERM')\n"
                  " end=time.monotonic()+12\n"
                  " while not os.path.exists('/tmp/release-gate') and time.monotonic()<end: time.sleep(.05)\n"
                  " raise SystemExit(0)\n"
                  "signal.signal(signal.SIGTERM,term)\nopen('/tmp/pid1-ready','w').write('ready')\n"
                  "while True: time.sleep(.05)\n")
        for role in r.ROLES:
            cid = self.raw.text(['create', '--pull', 'never', '--network', 'none', '--restart', 'no',
                                 '--label', LABEL + '=' + self.owner,
                                 '--label', 'com.docker.compose.project=request-' + self.owner,
                                 '--label', 'com.docker.compose.service=' + role,
                                 self.image, 'python', '-u', '-c', script])
            self.cids.append(cid)
            event(self.log, 'owned-resource-recorded', cid=cid, owner=self.owner, project='request-' + self.owner)
        self.foreign = self.raw.text(['create', '--pull', 'never', '--network', 'none', '--restart', 'no',
            '--label', LABEL + '=' + self.owner, '--label', 'com.docker.compose.project=foreign-' + self.owner,
            self.image, 'python', '-c', 'import time; time.sleep(30)'])
        event(self.log, 'owned-resource-recorded', cid=self.foreign, owner=self.owner, project='foreign-' + self.owner)
        self.foreign_before = facts(self.raw, self.foreign)
        self.before = [facts(self.raw, cid) for cid in self.cids]
        self.assertNotEqual(self.image, r.OLD_IMAGE)
        self.receipt = dict(scope='LOCAL_ONLY tooling original request; NOT production Engine applicability',
                            before=self.before, after=None, completion='unproved', pre_lock_phase='unproved')
        self.addCleanup(self.save)

    def owned(self, cid):
        obj = self.raw.inspect('container', cid)
        self.assertEqual(obj['Id'], cid)
        self.assertEqual(obj['Config']['Labels'][LABEL], self.owner)
        return obj

    def cleanup(self):
        resources = self.cids + ([self.foreign] if hasattr(self, 'foreign') else [])
        for cid in reversed(resources):
            self.owned(cid)
            self.raw.text(['rm', '-f', '-v', cid])
        event(self.log, 'owned-cleanup-complete', exact_cids=resources)

    def save(self):
        self.receipt['after'] = [facts(self.raw, cid) for cid in self.cids]
        self.receipt['foreign_before'] = self.foreign_before
        self.receipt['foreign_after'] = facts(self.raw, self.foreign)
        self.assertEqual(self.receipt['foreign_after'], self.foreign_before)
        self.assertEqual(self.receipt['after'][1], self.before[1])
        self.log.with_suffix('.json').write_text(json.dumps(self.receipt, indent=2, sort_keys=True) + '\n')

    def release_gate(self, cid):
        self.owned(cid)
        self.raw.text(['exec', cid, 'touch', '/tmp/release-gate'])

    def wait_marker(self, cid, marker):
        end = time.monotonic() + 5
        while time.monotonic() < end:
            if self.raw.run(['exec', cid, 'test', '-f', marker], check=False).returncode == 0:
                return
            time.sleep(.05)
        self.fail('actual PID1 marker unavailable')

    def relay_docker(self, op, mode):
        relay = FaultRelay(self.directory.name, self.log, self.cids, '/containers/' + self.cids[0] + '/' + op, mode)
        self.addCleanup(relay.close)
        docker = r.cap.Docker()
        docker.cli = [docker.cli[0], '--host', 'unix://' + str(relay.path)]
        return docker

    def events(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def assert_original_once(self, op, acknowledged):
        path = '/containers/' + self.cids[0] + '/' + op
        rows = [v for v in self.events() if v['event'] == 'upstream-send-complete' and v['path'].split('?')[0].endswith(path)]
        self.assertEqual(len(rows), 1)
        self.assertFalse(any(v['event'] == 'duplicate-refused' for v in self.events()))
        acks = [v for v in self.events() if v['event'] == 'engine-response' and v['path'].split('?')[0].endswith(path)]
        if acknowledged:
            self.assertEqual([v['status'] for v in acks], [204])
            self.receipt['completion'] = 'original Engine 204 captured before downstream loss'
        else:
            self.assertEqual(acks, [])
            self.receipt['completion'] = 'no terminal ack; factual samples do not settle original operation'

    def lost_response(self, op, mode):
        cid = self.cids[0]
        if op == 'stop':
            self.raw.text(['start', cid])
            self.wait_marker(cid, '/tmp/pid1-ready')
            self.release_gate(cid)
        self.receipt['pre_request'] = facts(self.raw, cid)
        docker = self.relay_docker(op, mode)
        timeout = 2 if mode == 'timeout' else 10
        with self.assertRaises((subprocess.TimeoutExpired, r.Refused)) as failure:
            docker.text([op, cid], timeout=timeout)
        self.receipt['client_fault'] = str(failure.exception)
        if not self.log.exists():
            self.fail('original request never reached relay: ' + str(failure.exception))
        self.receipt['fault_observed'] = facts(self.raw, cid)
        self.assert_original_once(op, True)
        self.assertEqual(self.receipt['fault_observed']['state']['Running'], op == 'start')
        self.assertEqual(self.receipt['fault_observed']['cid'], self.before[0]['cid'])
        self.assertEqual(self.receipt['fault_observed']['image'], self.before[0]['image'])
        self.assertEqual(self.receipt['fault_observed']['labels'], self.before[0]['labels'])
        self.assertFalse(self.receipt['fault_observed']['state']['Restarting'])
        self.assertEqual(self.receipt['fault_observed']['restart_policy']['Name'], 'no')
        epoch = 'StartedAt' if op == 'start' else 'FinishedAt'
        self.assertNotEqual(self.receipt['pre_request']['state'][epoch], self.receipt['fault_observed']['state'][epoch])

    def test_original_start_completed_response_timeout(self): self.lost_response('start', 'timeout')
    def test_original_start_completed_response_disconnect(self): self.lost_response('start', 'disconnect')
    def test_original_stop_completed_response_timeout(self): self.lost_response('stop', 'timeout')
    def test_original_stop_completed_response_disconnect(self): self.lost_response('stop', 'disconnect')

    def test_original_start_upstream_disconnect_phase_unproved(self):
        docker = self.relay_docker('start', 'upstream-close')
        with self.assertRaises(r.Refused): docker.text(['start', self.cids[0]])
        self.receipt['fault_observed'] = facts(self.raw, self.cids[0])
        self.assert_original_once('start', False)

    def inflight_stop(self, upstream_close):
        cid = self.cids[0]
        self.raw.text(['start', cid])
        self.wait_marker(cid, '/tmp/pid1-ready')
        docker = self.relay_docker('stop', 'upstream-close' if upstream_close else 'timeout')
        process = subprocess.Popen(docker.cli + ['stop', '-t', '20', cid], env=docker.env,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            self.wait_marker(cid, '/tmp/term-accepted')
            event(self.log, 'actual-pid1-term-marker', cid=cid)
            self.receipt['inflight'] = facts(self.raw, cid)
            self.assertTrue(self.receipt['inflight']['state']['Running'])
            if process.poll() is None:
                process.kill()  # real downstream disconnect, original stop already delivered
            process.communicate(timeout=5)
            event(self.log, 'actual-client-disconnected', upstream_closed=upstream_close)
            self.release_gate(cid)
            end = time.monotonic() + 5
            while time.monotonic() < end:
                if not self.raw.inspect('container', cid)['State']['Running']: break
                time.sleep(.05)
            self.assertFalse(self.raw.inspect('container', cid)['State']['Running'])
            if not upstream_close:
                end = time.monotonic() + 5
                while time.monotonic() < end:
                    if any(v['event'] == 'engine-response' and v.get('method') == 'POST' and
                           v.get('path', '').split('?')[0].endswith('/containers/' + cid + '/stop')
                           for v in self.events()): break
                    time.sleep(.05)
            self.assert_original_once('stop', not upstream_close)
            if not upstream_close:
                self.receipt['completion'] = 'original stop Engine 204 captured AFTER in-flight downstream loss'
            else:
                self.assertTrue(any(v['event'] == 'upstream-fault-term-marker' for v in self.events()))
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate()
            process.wait()

    def test_original_stop_inflight_downstream_disconnect(self): self.inflight_stop(False)
    def test_original_stop_inflight_upstream_disconnect(self): self.inflight_stop(True)

    def test_original_start_loss_guard_retains_authentic_unknown(self):
        docker = self.relay_docker('start', 'disconnect')
        raw, owner, cids = self.raw, self.owner, self.cids
        class Owned:
            def inspect(self, kind, identity):
                obj = raw.inspect(kind, identity)
                if kind == 'container' and obj['Config']['Labels'].get(LABEL) != owner:
                    raise AssertionError('unowned identity')
                return obj
            def text(self, args, **kwargs):
                if args[:3] == ['ps', '-aq', '--no-trunc']:
                    return raw.text(args + ['--filter', 'label=' + LABEL + '=' + owner])
                if args[0] in ('start', 'stop', 'update'):
                    if args[-1] not in cids:
                        raise AssertionError('foreign project mutation')
                    self.inspect('container', args[-1])
                return docker.text(args, **kwargs) if args[0] == 'start' else raw.text(args, **kwargs)
        key = Path(self.directory.name) / 'fixture.key'
        key.write_text(uuid.uuid4().hex + uuid.uuid4().hex + '\n')
        key.chmod(0o600)
        old_key, old_secret = os.environ.get('RELEASE_TRUST_KEY_FILE'), os.environ.pop('RELEASE_TRUST_SECRET', None)
        os.environ['RELEASE_TRUST_KEY_FILE'] = str(key)
        try:
            Path(self.directory.name, r.cold.HOLD).mkdir()
            lock = r.ReleaseLock(self.directory.name)
            hold = dict(version=1, state=r.STARTING, owner='fixture-history', project='request-' + owner,
                        image_ids=['sha256:' + 'a' * 64, 'sha256:' + 'b' * 64], release_sha=r.APP,
                        proof_sha256=r.OLD_PROOF, proof_nonce=str(uuid.uuid4()), recovery_spent_nonces=[owner],
                        recovery_authorized_cids=cids, recovery_attempt={'nonce': owner, 'owner': lock.owner})
            r.cold.write_hold(self.directory.name, hold)
            supervisor = r.AttemptSupervisor(self.directory.name, lock, Owned(), owner, cids)
            with self.assertRaises(r.Refused): supervisor.command(b'S')
            with self.assertRaises(r.Refused): supervisor.close()
            after = r.cold.read_hold(self.directory.name)
            self.assertEqual(after['state'], 'RECOVERY_REQUIRED_UNKNOWN')
            self.assertEqual(after['recovery_spent_nonces'], [owner])
            self.assertIn('recovery_indeterminate_operation', after)
            self.assertTrue(Path(lock.path).is_dir())
            self.assert_original_once('start', True)
            # Recreate/create and foreign mutation are refused at the relay,
            # before forwarding anything to Engine; this grants no capability.
            connection = http.client.HTTPConnection('localhost', timeout=5)
            connection.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            connection.sock.connect(str(Path(self.directory.name) / 'engine.sock'))
            connection.request('POST', '/containers/create', body=b'{}')
            self.assertEqual(connection.getresponse().status, 403)
            connection.close()
            self.receipt['authenticated_hold_after'] = after
            self.receipt['fence_postimage'] = facts(raw, cids[0])
            self.assertFalse(self.receipt['fence_postimage']['state']['Running'])
            for cid in cids:
                self.assertEqual(facts(raw, cid)['image'], self.image)
        finally:
            if old_key is None: os.environ.pop('RELEASE_TRUST_KEY_FILE', None)
            else: os.environ['RELEASE_TRUST_KEY_FILE'] = old_key
            if old_secret is not None: os.environ['RELEASE_TRUST_SECRET'] = old_secret


if __name__ == '__main__':
    unittest.main(verbosity=2)
