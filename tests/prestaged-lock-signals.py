"""Linux-only real-process signal/lock tests; no services, network, or DB.
Run in the existing isolated production-contract image with python3, repo RO.
Extracts the actual shell functions from each release entrypoint.
"""
import os
import pathlib
import re
import select
import signal
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class LockSignals(unittest.TestCase):
    def functions(self, name):
        text = (ROOT / name).read_text()
        out = []
        for fn in ('cleanup', 'handle_signal', 'is_pid_alive', 'acquire_release_lock'):
            match = re.search(r'^' + fn + r'\(\) \{\n.*?^\}', text, re.M | re.S)
            self.assertIsNotNone(match, fn)
            out.append(match.group())
        return '\n'.join(out)

    def spawn(self, source, directory, mode='owner'):
        code = '''set -Eeuo pipefail
PRESTAGE_DIR="$1"
LOCK_DIR="$PRESTAGE_DIR/release.lock"
RECOVERY_LOCK="$PRESTAGE_DIR/release.lock.recovering"
OWNER_TOKEN="${BASHPID:-$$}-$RANDOM"
IS_LOCK_OWNER=0
TMP_PROOF=""
log() { :; }
fail() { echo "$*" >&2; exit 1; }
''' + self.functions(source) + '''
trap 'handle_signal INT' INT
trap 'handle_signal TERM' TERM
trap cleanup EXIT
if [ "$2" = nonowner ]; then
 echo READY
else
 acquire_release_lock
 echo ACQUIRED
fi
while :; do sleep 0.05; done
'''
        proc = subprocess.Popen(['bash', '-c', code, 'lock-test', directory, mode],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                                start_new_session=True)
        self.children.append(proc)
        return proc

    def line(self, proc):
        ready, _, _ = select.select([proc.stdout], [], [], 5)
        self.assertTrue(ready, 'child did not report a lock outcome')
        return proc.stdout.readline().strip()

    def setUp(self):
        self.children = []

    def tearDown(self):
        for p in self.children:
            if p.poll() is None:
                os.killpg(p.pid, signal.SIGKILL)
            p.wait(timeout=5)
            p.stdout.close()
            p.stderr.close()

    def test_real_owner_nonowner_signals_and_crash_recovery(self):
        for source in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh'):
            for sig, expected in ((signal.SIGINT, 130), (signal.SIGTERM, 143)):
                with self.subTest(script=source, signal=sig), tempfile.TemporaryDirectory() as d:
                    owner = self.spawn(source, d)
                    self.assertEqual(self.line(owner), 'ACQUIRED')
                    lock = pathlib.Path(d) / 'release.lock'
                    token = (lock / 'owner_token').read_text()
                    # A real competing process fails, without deleting A's lock.
                    rival = self.spawn(source, d)
                    self.assertEqual(rival.wait(timeout=5), 1)
                    self.assertEqual((lock / 'owner_token').read_text(), token)
                    # Signal an actual non-owner process with the same trap installed.
                    nonowner = self.spawn(source, d, 'nonowner')
                    self.assertEqual(self.line(nonowner), 'READY')
                    os.kill(nonowner.pid, sig)
                    self.assertEqual(nonowner.wait(timeout=5), expected)
                    self.assertEqual((lock / 'owner_token').read_text(), token)
                    # Abnormal kill of an actual non-owner process must not touch owner's lock.
                    nonowner_kill = self.spawn(source, d, 'nonowner')
                    self.assertEqual(self.line(nonowner_kill), 'READY')
                    os.kill(nonowner_kill.pid, signal.SIGKILL)
                    self.assertEqual(nonowner_kill.wait(timeout=5), -signal.SIGKILL)
                    self.assertEqual((lock / 'owner_token').read_text(), token)
                    os.kill(owner.pid, sig)
                    self.assertEqual(owner.wait(timeout=5), expected)
                    self.assertFalse(lock.exists(), 'owner signal must clean its own lock')
            with self.subTest(script=source, abnormal='SIGKILL'), tempfile.TemporaryDirectory() as d:
                owner = self.spawn(source, d)
                self.assertEqual(self.line(owner), 'ACQUIRED')
                lock = pathlib.Path(d) / 'release.lock'
                old = (lock / 'owner_token').read_text()
                os.kill(owner.pid, signal.SIGKILL)
                self.assertEqual(owner.wait(timeout=5), -signal.SIGKILL)
                self.assertTrue(lock.exists(), 'SIGKILL cannot run cleanup')
                # Two actual contenders attempt recovery of the same dead owner.
                contenders = [self.spawn(source, d), self.spawn(source, d)]
                outcomes = [self.line(p) for p in contenders]
                self.assertEqual(outcomes.count('ACQUIRED'), 1, outcomes)
                winner = contenders[outcomes.index('ACQUIRED')]
                loser = contenders[1 - outcomes.index('ACQUIRED')]
                self.assertEqual(loser.wait(timeout=5), 1)
                self.assertNotEqual((lock / 'owner_token').read_text(), old)
                os.kill(winner.pid, signal.SIGTERM)
                self.assertEqual(winner.wait(timeout=5), 143)
                self.assertFalse(lock.exists())
                self.assertFalse((pathlib.Path(d) / 'release.lock.recovering').exists())

    def test_indeterminate_lock_owner_is_not_recovered(self):
        for source in ('deploy.sh', 'rollback.sh', 'ops/prestage-release.sh'):
            for pid in ('not-a-pid', '-1'):
                with self.subTest(script=source, pid=pid), tempfile.TemporaryDirectory() as d:
                    lock = pathlib.Path(d) / 'release.lock'
                    lock.mkdir()
                    (lock / 'owner_pid').write_text(pid)
                    (lock / 'owner_token').write_text('unknown-owner')
                    p = self.spawn(source, d)
                    self.assertEqual(p.wait(timeout=5), 1)
                    self.assertEqual((lock / 'owner_token').read_text(), 'unknown-owner')


if __name__ == '__main__':
    unittest.main(verbosity=2)
