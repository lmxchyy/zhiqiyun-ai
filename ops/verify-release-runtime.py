#!/usr/bin/env python3
"""Read-only release gate. No task creation, DB writes or credentials in output.

Scheduler diagnostics use the existing /metrics families, two samples, startup
flags and async runtime state. They are not a new scheduler heartbeat or proof
that an idle scheduler has dispatched a task. Queue consumers are checked
explicitly through RabbitMQ management for the running API's configured vhost.
"""
import json
import math
import os
import re
import shlex
import shutil
import subprocess
import sys
import time


class GateError(Exception):
    pass


def run(args):
    if os.name == 'nt':
        args = [os.environ.get('SH_EXE') or shutil.which('bash') or 'bash',
                '-c', ' '.join(shlex.quote(a) for a in args)]
    try:
        return subprocess.check_output(args, stdin=subprocess.DEVNULL,
                                       stderr=subprocess.PIPE, timeout=20).decode('utf-8').strip()
    except Exception:
        # Never echo argv, subprocess stderr, a rendered config or a broker URL.
        raise GateError('runtime observation command failed or timed out')


FLAGS = """import json,os
print(json.dumps({k:os.environ.get(k,'') for k in
 ['GENERATION_FAIR_SCHEDULER_ENABLED','ASYNC_MESSAGING_ENABLED']}))
"""
BROKER = """import base64,json,os,urllib.parse,urllib.request
u=urllib.parse.urlparse(os.environ['RABBITMQ_URL'])
if u.scheme not in ('amqp','amqps') or not u.hostname or u.username is None or u.password is None:
 raise ValueError('invalid broker configuration')
vhost=urllib.parse.unquote(u.path[1:]) if len(u.path)>1 else '/'
host=u.hostname
if ':' in host: host='['+host+']'
url=('https://' if u.scheme=='amqps' else 'http://')+host+(':'+('15671' if u.scheme=='amqps' else '15672'))+'/api/queues/'+urllib.parse.quote(vhost,safe='')
auth=base64.b64encode((urllib.parse.unquote(u.username)+':'+urllib.parse.unquote(u.password)).encode()).decode()
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args,**kwargs): raise ValueError('broker redirects prohibited')
req=urllib.request.Request(url,headers={'Authorization':'Basic '+auth},method='GET')
with urllib.request.build_opener(NoRedirect()).open(req,timeout=10) as r: data=json.load(r)
print(json.dumps([{k:q[k] for k in ['name','consumers','messages_ready','messages_unacknowledged']} for q in data]))
"""


def scheduler_sample(text):
    values = {}
    required = ('generation_scheduler_db_scrape_success', 'generation_scheduler_errors_total',
                'generation_scheduler_dispatched_total', 'generation_scheduler_recovered_total')
    for line in text.splitlines():
        parts = line.split()
        if parts and parts[0] in required:
            if len(parts) != 2 or parts[0] in values:
                raise GateError('invalid/duplicate scheduler metric')
            value = float(parts[1])
            if not math.isfinite(value) or value < 0:
                raise GateError('invalid scheduler metric value')
            values[parts[0]] = value
    if set(values) != set(required) or values['generation_scheduler_db_scrape_success'] != 1:
        raise GateError('scheduler diagnostics missing or DB scrape unhealthy')
    return values


def queues_ok(queues, phase):
    if not isinstance(queues, list):
        raise GateError('invalid broker queue observation')
    expected = {'x.ai.generation.image.canary', 'x.ai.generation.video.canary',
                'x.ai.generation.ppt.canary'}
    if phase == 'post':
        expected.add('x.ai.generation.image.normal')
    seen = set()
    for q in queues:
        name = q.get('name')
        if not isinstance(name, str) or name in seen:
            raise GateError('missing/duplicate broker queue name')
        seen.add(name)
        for key in ('consumers', 'messages_ready', 'messages_unacknowledged'):
            n = q.get(key)
            if type(n) is not int or n < 0:
                raise GateError('invalid broker queue counters')
        if name in expected or name == 'x.ai.generation.image.normal':
            if q['consumers'] < 1:
                raise GateError('required generation consumer missing')
        if phase == 'pre' and name.startswith('x.ai.') and not name.endswith('.dlq'):
            if q['messages_ready'] or q['messages_unacknowledged']:
                raise GateError('active queue is not drained')
    if not expected.issubset(seen):
        raise GateError('required generation queue missing')


def verify(phase, compose, env, execute=run, pause=time.sleep):
    if phase not in ('pre', 'post'):
        raise GateError('invalid runtime gate phase')
    cmd = ['docker', 'compose', '-f', compose, '--env-file', env]
    flags = json.loads(execute(cmd + ['exec', '-T', 'xianzhi-ai', 'python3', '-c', FLAGS]))
    for key in ('GENERATION_FAIR_SCHEDULER_ENABLED', 'ASYNC_MESSAGING_ENABLED'):
        if str(flags.get(key, '')).lower() != 'true':
            raise GateError('scheduler/async runtime must be explicitly enabled')
    metric_cmd = cmd + ['exec', '-T', 'xianzhi-ai', 'curl', '-fsS', '--max-time', '10',
                        'http://127.0.0.1:3100/metrics']
    first = scheduler_sample(execute(metric_cmd))
    pause(2)
    second = scheduler_sample(execute(metric_cmd))
    if second['generation_scheduler_errors_total'] != first['generation_scheduler_errors_total']:
        raise GateError('scheduler error counter changed during observation')
    for key in ('generation_scheduler_dispatched_total', 'generation_scheduler_recovered_total'):
        if second[key] < first[key]:
            raise GateError('scheduler process reset during observation')
    ready = json.loads(execute(cmd + ['exec', '-T', 'xianzhi-ai', 'curl', '-fsS', '--max-time', '10',
                                    'http://127.0.0.1:3100/api/v1/ready']))
    if str(ready.get('ready', '')).lower() != 'true' or ready.get('asyncMessaging') != 'READY':
        raise GateError('scheduler async runtime is not READY')
    for service in ('xianzhi-ai', 'smartvideo-worker'):
        cid = execute(cmd + ['ps', '-q', service])
        if not re.fullmatch(r'[a-zA-Z0-9_-]+', cid or ''):
            raise GateError('runtime container discovery failed')
        state = execute(['docker', 'inspect', '--format', '{{.State.Running}}', cid])
        healthy = execute(['docker', 'inspect', '--format', '{{.State.Health.Status}}', cid])
        if state != 'true' or healthy != 'healthy':
            raise GateError('API/worker runtime unhealthy')
    queues_ok(json.loads(execute(cmd + ['exec', '-T', 'xianzhi-ai', 'python3', '-c', BROKER])), phase)


def main():
    try:
        if len(sys.argv) != 4:
            raise GateError('usage: verify-release-runtime.py pre|post compose env')
        verify(*sys.argv[1:])
    except Exception as error:
        # GateError messages are fixed strings only; other exceptions may contain secrets.
        detail = str(error) if isinstance(error, GateError) else 'invalid runtime observation'
        print('[release-runtime] FAIL: ' + detail, file=sys.stderr)
        return 1
    print('[release-runtime] scheduler diagnostics, consumers and worker PASS')
    return 0


if __name__ == '__main__':
    sys.exit(main())
