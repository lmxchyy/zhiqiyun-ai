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


class PostStartDiagnostic(Diagnostic):
    """First-error observer only; shares one exclusive artifact across fork.

    No error text/args or locals dictionaries are serialized. Code-matched
    frames may contribute only explicitly bounded enums from returned data.
    """
    def __init__(self, root, sources, refused, transport_error, gate_error):
        super().__init__(root, sources, refused, transport_error, gate_error)
        self.filename = 'post-start-diagnostic-' + uuid.uuid4().hex + '.json'
        self.first = False
        self.captured = None
        self.dispatched = False
        self.codes = {}
        import types
        def add(code, source):
            if code.co_filename != source[1]:
                return
            self.codes[code] = source
            for item in code.co_consts:
                if type(item) is types.CodeType:
                    add(item, source)
        for source in sources:
            for value in source[0].values():
                members = [value]
                if isinstance(value, type):
                    members += list(value.__dict__.values())
                for member in members:
                    if type(member) in (staticmethod, classmethod):
                        member = member.__func__
                    if type(member) is types.FunctionType:
                        add(member.__code__, source)

    def stage_at(self, stage):
        if stage in ('post_start_ack', 'post_start_health', 'post_start_verify',
                     'post_start_snapshot', 'post_start_window_lock', 'post_start_commit'):
            self.stage = stage

    def details(self, error):
        import ast
        # verify_running loads a second official runtime module. Accept its
        # frame only when the complete code object equals the trusted one.
        sources = list(self.sources)
        frames = []
        tb = BaseException.__getattribute__(error, '__traceback__')
        while tb is not None:
            source = self.codes.get(tb.tb_frame.f_code)
            if source is not None:
                sources.append((tb.tb_frame.f_globals, source[1], source[2]))
                if tb.tb_frame.f_code.co_name not in ('require', 'block'):
                    frames.append((tb.tb_frame, tb.tb_lineno, source))
            tb = tb.tb_next
        original = self.sources
        try:
            self.sources = sources
            info = self.metadata(error)
        finally:
            self.sources = original
        actual = {'check_passes': False, 'error_type': info['error_type']}
        expected = {'check_passes': True}
        label = None
        if frames:
            frame, line, source = frames[-1]
            with open(source[1], 'rb') as stream:
                tree = ast.parse(stream.read(), filename=source[1])
            candidates = []
            for node in ast.walk(tree):
                call = node if isinstance(node, ast.Call) else (
                    node.exc if isinstance(node, ast.Raise) and isinstance(node.exc, ast.Call) else None)
                if call is None or not isinstance(call.func, ast.Name):
                    continue
                if call.func.id not in ('require', 'block', 'Refused', 'GateError') or not call.args:
                    continue
                text = call.args[-1]
                if not isinstance(text, ast.Str):
                    continue
                end = max(getattr(n, 'lineno', call.lineno) for n in ast.walk(call))
                if call.lineno <= line <= end:
                    candidates.append((end - call.lineno, text.s))
            if candidates:
                label = min(candidates)[1]
            # These are fixed scalar summaries, never arbitrary local data.
            values = frame.f_locals
            for key, allowed in (
                ('service', ('xianzhi-ai', 'smartvideo-worker', 'postgres')),
                ('state', ('true', 'false')),
                ('healthy', ('healthy', 'unhealthy', 'starting'))):
                value = values.get(key)
                if type(value) is str:
                    actual[key] = value if value in allowed else 'UNRECOGNIZED'
            ready = values.get('ready')
            if type(ready) is dict:
                value = ready.get('ready')
                if type(value) is bool:
                    actual['ready'] = value
                elif type(value) is str:
                    actual['ready'] = value if value in ('true', 'false') else 'UNRECOGNIZED'
                value = ready.get('asyncMessaging')
                if type(value) is str:
                    actual['asyncMessaging'] = value if value in ('READY', 'STARTING', 'DEGRADED', 'DISABLED') else 'UNRECOGNIZED'
        if label == 'API/worker runtime unhealthy':
            expected.update(state='true', healthy='healthy')
        elif label == 'scheduler async runtime is not READY':
            expected.update(ready=True, asyncMessaging='READY')
        elif label == 'post-start protected drift':
            expected['snapshot_equal'] = True
            actual['snapshot_equal'] = self.summaries.get('snapshot_equal', 'UNKNOWN')
        result = {k: info[k] for k in ('reason_code', 'check_name', 'source_file', 'function', 'line')}
        if label is not None:
            result['reason_code'] = {
                'API/worker runtime unhealthy': 'POST_START_RUNTIME_HEALTH_FAILED',
                'scheduler async runtime is not READY': 'POST_START_READY_FAILED',
                'post-start protected drift': 'POST_START_PROTECTED_SNAPSHOT_MISMATCH',
                'actual PID1 environment policy mismatch': 'POST_START_PID1_ENV_POLICY_MISMATCH',
                'fresh bounded window required': 'POST_START_WINDOW_INVALID',
            }.get(label, 'POST_START_CHECK_FAILED')
        result.update(stage=self.stage, expected=expected, actual=actual,
                      guard_label=label if label is not None else 'UNKNOWN',
                      summaries=dict(self.summaries), scope='POST_START_FIRST_FAILURE_ONLY')
        return result

    def persist(self, text):
        # Existing private-path rules; O_EXCL is the diagnostic-only first
        # writer latch. It is not recovery authority and never changes a gate.
        current = self.directory
        while True:
            info = os.lstat(current)
            if stat.S_ISLNK(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
                raise PermissionError()
            parent = os.path.dirname(current)
            if current == parent:
                break
            current = parent
        fd = os.open(os.path.join(self.directory, self.filename),
                     os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w', encoding='utf-8') as stream:
            stream.write(text + '\n')
            stream.flush()
            os.fsync(stream.fileno())

    def capture_first(self, error):
        # Pre-cleanup path: only retain the exception reference and scalar state.
        # No source reads, filesystem operations, stderr, threads or waits here.
        if self.captured is None:
            self.captured = (error, self.stage, dict(self.summaries))

    def emit_after_cleanup(self, stderr):
        if self.captured is None or self.dispatched:
            return
        self.dispatched = True
        error, stage, summaries = self.captured
        self.stage, self.summaries = stage, summaries
        # All potentially blocking observer work starts AFTER native cleanup.
        # An undrained pipe/stalled filesystem may lose diagnostics, never fence.
        # Daemon work cannot keep the guard alive on os._exit or shutdown.
        import threading
        worker = threading.Thread(target=self.emit_first, args=(error, stderr))
        worker.daemon = True
        worker.start()
        worker.join(0.05)  # Diagnostic-only, bounded post-cleanup grace; no retry.

    def emit_first(self, error, stderr):
        if self.first:
            return
        self.first = True
        try:
            text = json.dumps(self.details(error), sort_keys=True, separators=(',', ':'))
        except BaseException:
            text = json.dumps(dict(reason_code='UNKNOWN', check_name='UNKNOWN', stage=self.stage,
                                   expected={'check_passes': True}, actual={'check_passes': False},
                                   source_file='UNKNOWN', function='UNKNOWN', line=0))
        try:
            self.persist(text)
        except FileExistsError:
            return  # The guard's first failure must survive the parent's ACK error.
        except BaseException:
            pass
        try:
            stderr.write('COLD_RECOVERY_POST_START_DIAGNOSTIC: ' + text + '\n')
            stderr.flush()  # Best effort in the post-cleanup daemon observer.
        except BaseException:
            pass
