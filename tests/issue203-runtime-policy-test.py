#!/usr/bin/env python3
"""Bounded real PID1 policy probes, NOT generation-barrier attestation."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import types
import unittest

ROOT = Path(__file__).resolve().parent.parent
path = ROOT / 'ops/verify-image-quarantine-capability.py'
capability = types.ModuleType('policy_probe_capability')
capability.__file__ = str(path)
exec(compile(path.read_bytes(), str(path), 'exec'), capability.__dict__)

class ProcessPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.docker = capability.Docker()
        cls.docker.deadline = time.monotonic() + 120
        cls.docker.evidence_dir = '/work/owned' if os.name != 'nt' else str(ROOT / '.evidence/issue203/priority4-runtime-capability')
        cls.fixture = capability.Fixture(cls.docker)
        cls.image = cls.docker.inspect('image', 'issue203-p4-synthetic:local')
        cls.token = cls.fixture.owner

    @classmethod
    def tearDownClass(cls):
        cls.fixture.cleanup()

    def probe(self, code, entrypoint=None):
        args = ['create', '--pull', 'never', '--label', capability.LABEL + '=' + self.fixture.owner, '--network', 'none', '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--pids-limit', '64', '--memory', '256m', '-e', 'POLICY_PROBE_TOKEN=' + self.token]
        if entrypoint:
            args += ['--entrypoint', entrypoint]
        args += [self.image['Id'], '/usr/bin/python3', '-c', code]
        cid = self.fixture.record('container', self.docker.text(args))
        self.docker.text(['start', cid])
        obj = self.fixture.owned('container', cid)
        expected = dict(x.split('=', 1) for x in obj['Config']['Env'])
        policy = dict(entrypoint=[], command=obj['Config']['Cmd'])
        return cid, obj, policy, expected

    def test_01_actual_pid1_positive(self):
        capability.verify_pid1_policy(self.docker, *self.probe('import time;time.sleep(100)'))

    def test_02_actual_env_substitution_same_config_and_argv(self):
        code = "import os,time; a=open('/proc/self/cmdline','rb').read().rstrip(b'\\0').split(b'\\0'); e=dict(os.environ); e['POLICY_PROBE_TOKEN']='substituted'; os.execve(a[0],a,e) if os.environ['POLICY_PROBE_TOKEN']!='substituted' else time.sleep(100)"
        args = self.probe(code)
        for attempt in range(15):
            if b'POLICY_PROBE_TOKEN=substituted\0' in self.docker.run(['exec', args[0], '/bin/cat', '/proc/1/environ']).stdout:
                break
            time.sleep(.1)
        else:
            self.fail('real env substitution was not installed')
        with self.assertRaisesRegex(capability.Refused, 'PID1 environment policy mismatch'):
            capability.verify_pid1_policy(self.docker, *args)

    def test_03_actual_cmdline_substitution_same_config(self):
        code = "import os;os.execv('/usr/bin/python3',['/usr/bin/python3','-c','import time;time.sleep(100)'])"
        args = self.probe(code)
        for attempt in range(15):
            if self.docker.run(['exec', args[0], '/bin/cat', '/proc/1/cmdline']).stdout != b'\0'.join(x.encode() for x in args[2]['command']) + b'\0':
                break
            time.sleep(.1)
        else:
            self.fail('real cmdline substitution was not installed')
        with self.assertRaisesRegex(capability.Refused, 'PID1 command policy mismatch'):
            capability.verify_pid1_policy(self.docker, *args)

    def test_04_actual_entrypoint_not_self_compared(self):
        args = self.probe('import time;time.sleep(100)', '/usr/bin/env')
        with self.assertRaisesRegex(capability.Refused, 'entrypoint policy mismatch'):
            capability.verify_pid1_policy(self.docker, *args)

    def test_05_actual_proc_permission_denial_fails_closed(self):
        args = self.probe('import time;time.sleep(100)')
        class Unreadable(capability.Docker):
            def run(self, command, **kwargs):
                if command[0] == 'exec' and command[-1] == '/proc/1/environ':
                    command = ['exec', '--user', '65534'] + list(command[1:])
                return super().run(command, **kwargs)
        with self.assertRaisesRegex(capability.Refused, 'Docker operation failed: exec'):
            capability.verify_pid1_policy(Unreadable(), *args)

    def test_06_inherited_image_entrypoint_rejected(self):
        image = json.loads(json.dumps(self.image))
        image['Config']['Entrypoint'] = ['/startup-wrapper']
        class Metadata:
            def inspect(self, *args):
                return image
            def text(self, *args):
                raise AssertionError('must reject before running candidate utilities')
        with self.assertRaisesRegex(capability.Refused, 'explicit empty policy'):
            capability.image_identity(Metadata(), 'issue203-p4-synthetic:local', '607854fd4f5e1313be0a029811b13acf1223bac1', True)

if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(ProcessPolicyTests))
    print('POLICY_RESULT tests=%d failures=%d errors=%d skipped=%d Python=%s' % (result.testsRun, len(result.failures), len(result.errors), len(result.skipped), sys.version.split()[0]), flush=True)
    sys.exit(0 if result.wasSuccessful() and result.testsRun == 6 and not result.skipped else 1)
