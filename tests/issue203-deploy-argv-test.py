"""Execute the actual deploy.sh callsite bytes in Bash; never execute deployment."""
import hashlib
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
BASH = 'C:/Program Files/Git/bin/bash.exe' if os.name == 'nt' else 'bash'


class DeployArgvTests(unittest.TestCase):
    def test_actual_safe_drain_and_enrollment_argv(self):
        source = (ROOT / 'deploy.sh').read_text(encoding='utf-8')
        drain = source[source.index('check_safe_drain() {'):source.index('verify_health_and_readiness()')]
        start = source.index('    python3 ops/enroll-quarantine.py ')
        end = source.index('\n  fi', start)
        enrollment = source[start:end]
        with tempfile.TemporaryDirectory(prefix='issue203 argv ') as directory:
            manifest = Path(directory) / "approval ' quoted.json"
            manifest.write_bytes(b'{}')
            values = dict(COMPOSE_FILE="compose ' quoted file.json", ENV_FILE='env $ literal file',
                          QUARANTINE_MANIFEST=manifest.as_posix(), PRESTAGED_RELEASE_SHA='b' * 40,
                          EXPECTED_QUARANTINE_MANIFEST_SHA256=hashlib.sha256(b'{}').hexdigest(),
                          DRAIN_TIMEOUT_SECONDS='7',
                          LOCK_DIR='release-lock-dir-mock')
            for proof in ("proof ' with spaces $ literal.json", ''):
                for mode in ('drain', 'drain-no-manifest', 'enroll'):
                    with self.subTest(mode=mode, proof=proof):
                        current = dict(values, PRESTAGE_PROOF_FILE=proof)
                        if mode == 'drain-no-manifest':
                            current['QUARANTINE_MANIFEST'] = ''
                        # Shell function captures real argv after Bash expansion,
                        # NUL-delimited so spaces/newlines cannot fake boundaries.
                        script = 'set -eu\nlog(){ :; }\nfail(){ exit 1; }\npython3(){ printf "%s\\0" "$@"; }\n'
                        script += '\n'.join(k + '=' + shlex.quote(v) for k, v in current.items()) + '\n'
                        script += enrollment if mode == 'enroll' else drain + '\ncheck_safe_drain\n'
                        path = Path(directory) / 'callsite.sh'
                        path.write_bytes(script.encode('utf-8'))
                        proc = subprocess.run([BASH, path.as_posix()], cwd=str(ROOT), stdout=subprocess.PIPE,
                                              stderr=subprocess.PIPE, timeout=10)
                        self.assertEqual(proc.returncode, 0, proc.stderr)
                        actual = proc.stdout.decode('utf-8').split('\0')
                        self.assertEqual(actual.pop(), '')
                        expected = ['ops/' + ('enroll-quarantine.py' if mode == 'enroll' else 'verify-safe-drain.py'),
                                    values['COMPOSE_FILE'], values['ENV_FILE']]
                        if mode == 'enroll':
                            expected += [values['QUARANTINE_MANIFEST'], values['PRESTAGED_RELEASE_SHA'],
                                         '--release-lock-dir', values['LOCK_DIR']]
                        else:
                            expected += ['7']
                            if mode == 'drain':
                                expected += ['--manifest', values['QUARANTINE_MANIFEST'], '--release-sha', values['PRESTAGED_RELEASE_SHA']]
                        if mode != 'drain-no-manifest':
                            expected += ['--expected-manifest-sha256', values['EXPECTED_QUARANTINE_MANIFEST_SHA256']]
                        expected += ['--prestage-proof', proof]
                        self.assertEqual(actual, expected)


if __name__ == '__main__':
    unittest.main(verbosity=2)
