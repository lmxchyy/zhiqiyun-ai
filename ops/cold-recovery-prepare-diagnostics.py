"""Prepare-only metadata observer. Never serialize exceptions or observations."""
import builtins
import json
import os
import queue
import stat
import subprocess
import uuid


class Diagnostic:
    def __init__(self, root, sources, refused, transport_error, gate_error):
        self.directory = os.path.join(root, '.prestage')
        self.sources = sources
        self.types = {getattr(builtins, name): name for name in (
            'BaseException', 'Exception', 'OSError', 'FileNotFoundError', 'FileExistsError',
            'PermissionError', 'TimeoutError', 'BrokenPipeError', 'ValueError', 'TypeError',
            'KeyError', 'IndexError', 'AttributeError', 'RuntimeError', 'MemoryError',
            'UnicodeDecodeError', 'UnicodeEncodeError', 'KeyboardInterrupt', 'SystemExit')}
        self.types.update({refused: 'Refused', transport_error: 'TransportError',
                           gate_error: 'GateError',
                           json.JSONDecodeError: 'JSONDecodeError', queue.Empty: 'Empty',
                           subprocess.CalledProcessError: 'CalledProcessError',
                           subprocess.TimeoutExpired: 'TimeoutExpired'})
        self.stage = 'preflight'
        self.interrupted_stage = None
        self.summaries = {}

    def observe(self, stage, **values):
        # Callers supply literals and booleans only; no arbitrary fields pass through.
        if stage == 'lock_release':
            self.interrupted_stage = self.stage
        if stage == 'lock_released':
            self.stage = self.interrupted_stage
            return
        self.stage = stage
        for name in ('initial_snapshot_returned', 'attest_returned', 'verify_returned',
                     'post_snapshot_returned', 'snapshot_equal', 'hold_equal'):
            if type(values.get(name)) is bool:
                self.summaries[name] = values[name]

    def snapshots(self, initial, post):
        # These are already-returned snapshots, never a new observer/RPC.
        for name in ('source', 'containers', 'postgres', 'database', 'compose_sha256',
                     'env_sha256', 'model_sha256', 'policy', 'image_identity'):
            self.summaries[name + '_equal'] = initial.get(name) == post.get(name)
        for role in ('xianzhi-ai', 'smartvideo-worker', 'postgres', 'redis', 'rabbitmq',
                     'minio', 'disk-monitor', 'postgres-backup', 'migrate'):
            before = initial.get('containers', {}).get(role, {})
            after = post.get('containers', {}).get(role, {})
            self.summaries[role + '_policy_equal'] = before.get('policy_sha256') == after.get('policy_sha256')
        for name in ('quarantine_records', 'history', 'migrations'):
            before = initial.get('database', {}).get(name)
            after = post.get('database', {}).get(name)
            self.summaries[name + '_equal'] = before == after
            if type(before) is list and type(after) is list:
                self.summaries[name + '_initial_count'] = len(before)
                self.summaries[name + '_post_count'] = len(after)

    def metadata(self, error):
        frames = []
        tb = BaseException.__getattribute__(error, '__traceback__')
        helper = False
        while tb is not None:
            frame = tb.tb_frame
            for namespace, filename, public in self.sources:
                if frame.f_globals is namespace and frame.f_code.co_filename == filename:
                    helper = frame.f_code.co_name in ('require', 'block')
                    if not helper:
                        frames.append({'source_file': public, 'function': frame.f_code.co_name,
                                       'line': tb.tb_lineno})
                    break
            tb = tb.tb_next
        location = frames[-1] if frames else {'source_file': 'UNKNOWN', 'function': 'UNKNOWN', 'line': 0}
        kind = self.types.get(type(error), 'UNKNOWN')
        check = '{source_file}:{function}:L{line}'.format(**location) if frames else 'UNKNOWN'
        reason = ('CHECK_AT_SOURCE' if helper else
                  'REFUSAL_AT_SOURCE' if kind in ('Refused', 'TransportError', 'GateError') else 'OPERATION_AT_SOURCE') if frames else 'UNKNOWN'
        if location['function'] == 'main' and reason == 'CHECK_AT_SOURCE':
            for name, mismatch in (('snapshot_equal', 'POST_SNAPSHOT_MISMATCH'),
                                   ('hold_equal', 'STOPPED_HOLD_MISMATCH')):
                if self.summaries.get(name) is False:
                    reason = mismatch
                    check += ':' + name
                    break
        return dict(location, error_type=kind, check_name=check, reason_code=reason,
                    source_frames=frames[-12:])

    def report(self, error):
        result = self.metadata(error)
        result.update(stage=self.stage, summaries=dict(self.summaries), exception_chain=[])
        pending = [(error, 'raised')]
        seen = set()
        while pending and len(seen) < 8:
            current, relation = pending.pop(0)
            if id(current) in seen:
                continue
            seen.add(id(current))
            item = self.metadata(current)
            item['relation'] = relation
            item['stage'] = (self.interrupted_stage if relation != 'raised' and
                             self.stage == 'lock_release' else self.stage)
            result['exception_chain'].append(item)
            for attribute, link in (('__cause__', 'cause'), ('__context__', 'context')):
                previous = BaseException.__getattribute__(current, attribute)
                if previous is not None:
                    pending.append((previous, link))
        return result

    def persist(self, text):
        # Fixed namespace only. No directory creation/repair, hold, lock or key calls.
        current = self.directory
        while True:
            info = os.lstat(current)
            if stat.S_ISLNK(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
                raise PermissionError()
            parent = os.path.dirname(current)
            if current == parent:
                break
            current = parent
        name = os.path.join(self.directory, 'prepare-diagnostic-' + uuid.uuid4().hex + '.json')
        fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w', encoding='utf-8') as stream:
            stream.write(text + '\n')
            stream.flush()
            os.fsync(stream.fileno())

    def emit(self, error, stderr):
        try:
            text = json.dumps(self.report(error), sort_keys=True, separators=(',', ':'))
        except BaseException:
            text = '{"reason_code":"UNKNOWN","check_name":"UNKNOWN","stage":"diagnostic_unavailable","error_type":"UNKNOWN"}'
        # Public JSON always reaches the native audit even if private persistence fails.
        try:
            stderr.write('COLD_RECOVERY_PREPARE_DIAGNOSTIC: ' + text + '\n')
        except BaseException:
            pass
        try:
            self.persist(text)
        except BaseException:
            pass
