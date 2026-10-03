"""Linux isolated real-process tests. No real Docker, network, DB or production paths.
The production monitor and actual release lock functions run unchanged; only
metrics transport and the Docker executable are fixtures.
"""
import importlib.util
import json
import os
import pathlib
import re
import signal
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKER = '''import importlib.util,os,pathlib,sys,time
spec=importlib.util.spec_from_file_location('monitor',sys.argv[1])
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
m.ROOT=pathlib.Path(sys.argv[2])
class Metrics:
 def __enter__(self): return self
 def __exit__(self,*a): pass
 def read(self): return os.environ.get('FIXTURE_METRICS','xianzhi_async_canary_rabbitmq_dlq_depth 8\\n').encode('ascii')
def get(*a,**k):
 (m.ROOT/'metrics_called').touch()
 if (m.ROOT/'block_metrics').exists():
  (m.ROOT/'metrics_entered').touch()
  (m.ROOT/('metrics_entered_'+str(os.getpid()))).touch()
  while not (m.ROOT/'metrics_release').exists(): time.sleep(.01)
 return Metrics()
m.urllib.request.urlopen=get
sys.exit(m.main())
'''
DOCKER = '''#!/usr/bin/env python3
import json,os,pathlib,sys,time
root=pathlib.Path(os.environ['FIXTURE_ROOT'])
with (root/'docker_calls').open('a') as f:f.write(json.dumps(sys.argv[1:])+'\\n')
(root/'compose_started').touch()
while (root/'block_compose').exists() and not (root/'compose_release').exists():time.sleep(.01)
(root/'compose_finished').touch()
sys.exit(int(os.environ.get('FAKE_DOCKER_EXIT','0')))
'''


class Interlock(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name)
        self.envfile = self.root / '.env.production'
        self.original = ('GENERATION_ASYNC_CANARY_ENABLED=true\n'
                         'VIDEO_ASYNC_CANARY_ENABLED=true\nPPT_ASYNC_CANARY_ENABLED=true\n'
                         'VIDEO_STORAGE_PERSISTENCE_ENABLED=true\nPRIVATE_VALUE=fixture-only\n')
        self.envfile.write_text(self.original)
        self.envfile.chmod(0o600)
        self.before = self.envfile.stat().st_mtime_ns
        self.directory = self.root / '.prestage'
        self.lock = self.directory / 'release.lock'
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        docker = self.bin / 'docker'
        docker.write_text(DOCKER)
        docker.chmod(0o755)
        self.children = []

    def tearDown(self):
        for p in self.children:
            try:
                os.killpg(p.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            p.wait(timeout=5)
            p.stdout.close()
            p.stderr.close()
        self.temp.cleanup()

    def wait_file(self, name):
        limit = time.monotonic() + 5
        while not (self.root / name).exists() and time.monotonic() < limit:
            time.sleep(.01)
        self.assertTrue((self.root / name).exists(), name)

    def monitor(self, **extra):
        env = dict(os.environ, PRESTAGE_DIR='.prestage', FIXTURE_ROOT=str(self.root),
                   PATH=str(self.bin) + os.pathsep + os.environ['PATH'])
        env.update(extra)
        p = subprocess.Popen([sys.executable, '-c', WORKER,
                              str(ROOT / 'ops/auto_monitor_killswitch.py'), str(self.root)],
                             env=env, cwd=str(self.root), stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, universal_newlines=True, start_new_session=True)
        self.children.append(p)
        return p

    def release(self, source, hold=False):
        text = (ROOT / source).read_text()
        functions = []
        for name in ('cleanup', 'handle_signal', 'is_pid_alive', 'acquire_release_lock'):
            match = re.search(r'^' + name + r'\(\) \{\n.*?^\}', text, re.M | re.S)
            self.assertIsNotNone(match, name)
            functions.append(match.group())
        code = '''set -Eeuo pipefail
PRESTAGE_DIR="$1"
LOCK_DIR="$PRESTAGE_DIR/release.lock"
RECOVERY_LOCK="$PRESTAGE_DIR/release.lock.recovering"
OWNER_TOKEN="${BASHPID:-$$}-$RANDOM"
IS_LOCK_OWNER=0
TMP_PROOF=""
log() { :; }
fail() { echo "$*" >&2; exit 1; }
''' + '\n'.join(functions) + '''
trap cleanup EXIT
acquire_release_lock
echo ACQUIRED
if [ "$2" = hold ]; then
 touch "$3/release_acquired"
 while [ ! -f "$3/release_finished" ]; do sleep .01; done
fi
'''
        p = subprocess.Popen(['bash', '-c', code, 'release-fixture', str(self.directory),
                              'hold' if hold else 'once', str(self.root)], stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, universal_newlines=True, start_new_session=True)
        self.children.append(p)
        return p

    def assert_untouched(self):
        self.assertEqual(self.envfile.read_text(), self.original)
        self.assertEqual(self.envfile.stat().st_mtime_ns, self.before)
        self.assertFalse((self.root / 'docker_calls').exists())

    def calls(self):
        return [json.loads(s) for s in (self.root / 'docker_calls').read_text().splitlines()]

    def test_existing_release_blocks_monitor_for_all_three_entrypoints(self):
        for source in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh'):
            with self.subTest(source=source):
                (self.root / 'release_acquired').unlink(missing_ok=True)
                (self.root / 'release_finished').unlink(missing_ok=True)
                owner = self.release(source, hold=True)
                self.wait_file('release_acquired')
                token = (self.lock / 'owner_token').read_text()
                self.assertEqual(self.monitor().wait(timeout=5), 0)
                self.assert_untouched()
                self.assertFalse((self.root / 'metrics_called').exists())
                self.assertEqual((self.lock / 'owner_token').read_text(), token)
                (self.root / 'release_finished').touch()
                self.assertEqual(owner.wait(timeout=5), 0)
                self.assertFalse(self.lock.exists())

    def test_release_started_during_metrics_is_rechecked_atomically(self):
        (self.root / 'block_metrics').touch()
        monitor = self.monitor()
        self.wait_file('metrics_entered')
        owner = self.release('deploy.sh', hold=True)
        self.wait_file('release_acquired')
        token = (self.lock / 'owner_token').read_text()
        (self.root / 'metrics_release').touch()
        self.assertEqual(monitor.wait(timeout=5), 0)
        self.assert_untouched()
        self.assertEqual((self.lock / 'owner_token').read_text(), token)
        (self.root / 'release_finished').touch()
        self.assertEqual(owner.wait(timeout=5), 0)

    def test_monitor_holds_lock_for_whole_compose_and_blocks_releases(self):
        (self.root / 'block_compose').touch()
        owner = self.monitor()
        self.wait_file('compose_started')
        token = (self.lock / 'owner_token').read_text()
        for source in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh'):
            self.assertEqual(self.release(source).wait(timeout=5), 1)
            self.assertEqual((self.lock / 'owner_token').read_text(), token)
        rival = self.monitor()
        self.assertEqual(rival.wait(timeout=5), 0)
        self.assertEqual(len(self.calls()), 1)
        self.assertEqual((self.lock / 'owner_token').read_text(), token)
        (self.root / 'compose_release').touch()
        self.assertEqual(owner.wait(timeout=5), 1)
        self.assertFalse(self.lock.exists())
        self.assertEqual(self.release('deploy.sh').wait(timeout=5), 0)

    def test_simultaneous_monitors_one_winner(self):
        (self.root / 'block_metrics').touch()
        (self.root / 'block_compose').touch()
        contenders = [self.monitor() for _ in range(5)]
        limit = time.monotonic() + 5
        while len(list(self.root.glob('metrics_entered_*'))) != 5 and time.monotonic() < limit:
            time.sleep(.01)
        self.assertEqual(len(list(self.root.glob('metrics_entered_*'))), 5)
        (self.root / 'metrics_release').touch()
        self.wait_file('compose_started')
        while sum(p.poll() is not None for p in contenders) != 4 and time.monotonic() < limit:
            time.sleep(.01)
        self.assertEqual(sum(p.poll() is not None for p in contenders), 4)
        self.assertEqual(len(self.calls()), 1)
        (self.root / 'compose_release').touch()
        outcomes = [p.wait(timeout=5) for p in contenders]
        self.assertEqual(outcomes.count(1), 1, outcomes)
        self.assertFalse(self.lock.exists())

    def test_api_only_no_build_no_pull_and_all_switches_preserved(self):
        self.assertEqual(self.monitor().wait(timeout=5), 1)
        content = self.envfile.read_text()
        # The authorized image-DLQ exception changes only this expectation;
        # all lock, signal, ownership and API-only assertions remain intact.
        self.assertEqual(content, self.original.replace('GENERATION_ASYNC_CANARY_ENABLED=true',
                                                       'GENERATION_ASYNC_CANARY_ENABLED=false'))
        self.assertIn('VIDEO_STORAGE_PERSISTENCE_ENABLED=true', content)
        self.assertIn('PRIVATE_VALUE=fixture-only', content)
        self.assertEqual(self.envfile.stat().st_mode & 0o777, 0o600)
        argv = self.calls()[0]
        self.assertEqual(argv[-1], 'xianzhi-ai')
        for flag in ('--no-deps', '--no-build'):
            self.assertIn(flag, argv)
        self.assertEqual(argv[argv.index('--pull') + 1], 'never')
        self.assertNotIn('migrate', argv)
        self.assertFalse(self.lock.exists())

    def test_already_off_does_not_rewrite_but_reconciles_runtime(self):
        self.envfile.write_text(self.original.replace('_ENABLED=true', '_ENABLED=false'))
        before = self.envfile.stat().st_mtime_ns
        self.assertEqual(self.monitor().wait(timeout=5), 1)
        self.assertEqual(self.envfile.stat().st_mtime_ns, before)
        self.assertEqual(len(self.calls()), 1)
        self.assertFalse(self.lock.exists())

    def test_failed_compose_does_not_claim_completion_or_unlock(self):
        p = self.monitor(FAKE_DOCKER_EXIT='7')
        self.assertEqual(p.wait(timeout=5), 1)
        self.assertNotIn('COMPLETED', p.stdout.read())
        self.assertEqual((self.lock / 'owner_pid').read_text().strip(), 'indeterminate-compose')
        for source in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh'):
            self.assertEqual(self.release(source).wait(timeout=5), 1)
        before = self.envfile.stat().st_mtime_ns
        self.assertEqual(self.monitor().wait(timeout=5), 0)
        self.assertEqual(self.envfile.stat().st_mtime_ns, before)
        self.assertEqual(len(self.calls()), 1)
        self.assertTrue(self.lock.exists())

    def test_unknown_dead_and_recovering_locks_defer_without_cleanup(self):
        self.directory.mkdir()
        for pid in ('not-a-pid', '99999999', ''):
            with self.subTest(pid=pid):
                self.lock.mkdir()
                (self.lock / 'owner_pid').write_text(pid)
                (self.lock / 'owner_token').write_text('foreign-owner')
                self.assertEqual(self.monitor().wait(timeout=5), 0)
                self.assertEqual((self.lock / 'owner_token').read_text(), 'foreign-owner')
                self.assert_untouched()
                for p in self.lock.iterdir():p.unlink()
                self.lock.rmdir()
        recovery = self.directory / 'release.lock.recovering'
        recovery.mkdir()
        self.assertEqual(self.monitor().wait(timeout=5), 0)
        self.assertTrue(recovery.exists())
        self.assert_untouched()

    def test_dangling_lock_symlink_is_also_locked(self):
        self.directory.mkdir()
        self.lock.symlink_to(self.root / 'absent-target')
        self.assertEqual(self.monitor().wait(timeout=5), 0)
        self.assertTrue(self.lock.is_symlink())
        self.assert_untouched()

    def test_token_mismatch_does_not_remove_foreign_lock(self):
        (self.root / 'block_compose').touch()
        owner = self.monitor()
        self.wait_file('compose_started')
        (self.lock / 'owner_token').write_text('replacement-owner')
        (self.root / 'compose_release').touch()
        self.assertEqual(owner.wait(timeout=5), 1)
        self.assertEqual((self.lock / 'owner_token').read_text(), 'replacement-owner')

    def test_real_int_term_wait_for_compose_before_unlocking(self):
        for sig in (signal.SIGINT, signal.SIGTERM):
            with self.subTest(signal=sig):
                for name in ('compose_started', 'compose_finished', 'compose_release'):
                    (self.root / name).unlink(missing_ok=True)
                (self.root / 'block_compose').touch()
                owner = self.monitor()
                self.wait_file('compose_started')
                os.kill(owner.pid, sig)
                time.sleep(.1)
                self.assertIsNone(owner.poll())
                self.assertEqual(self.release('deploy.sh').wait(timeout=5), 1)
                (self.root / 'compose_release').touch()
                self.assertEqual(owner.wait(timeout=5), 128 + sig)
                self.assertTrue((self.root / 'compose_finished').exists())
                self.assertFalse(self.lock.exists())

    def test_sigkill_during_compose_cannot_recover_just_dead_monitor_pid(self):
        (self.root / 'block_compose').touch()
        owner = self.monitor()
        self.wait_file('compose_started')
        os.kill(owner.pid, signal.SIGKILL)
        self.assertEqual(owner.wait(timeout=5), -signal.SIGKILL)
        self.assertEqual((self.lock / 'owner_pid').read_text().strip(), 'indeterminate-compose')
        for source in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh'):
            self.assertEqual(self.release(source).wait(timeout=5), 1)
        (self.root / 'compose_release').touch()
        self.wait_file('compose_finished')
        # Still requires explicit operator evidence; never auto-claim daemon safety.
        self.assertEqual(self.release('deploy.sh').wait(timeout=5), 1)
        self.assertTrue(self.lock.exists())

    def test_custom_prestage_directory_matches_release_configuration(self):
        custom = self.root / 'custom-stage'
        custom.mkdir()
        (custom / 'release.lock').mkdir()
        self.assertEqual(self.monitor(PRESTAGE_DIR=str(custom)).wait(timeout=5), 0)
        self.assert_untouched()


if __name__ == '__main__':
    if sys.platform != 'linux':
        raise SystemExit('Linux process tests required; use the isolated production-contract image.')
    unittest.main(verbosity=2)
