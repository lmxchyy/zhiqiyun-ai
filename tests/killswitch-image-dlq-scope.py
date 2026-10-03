"""Image-DLQ compatibility tests using the existing isolated process fixture.
No production, network, provider, DB or real Docker operation is involved.
"""
import importlib.util
import pathlib
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('interlock_fixture', ROOT / 'tests/killswitch-release-interlock.py')
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)

IMAGE_DLQ = 'xianzhi_async_canary_rabbitmq_dlq_depth'
NON_IMAGE_ALERTS = (
    'xianzhi_async_canary_video_rabbitmq_dlq_depth',
    'xianzhi_async_canary_video_generation_stuck',
    'xianzhi_async_canary_ppt_rabbitmq_dlq_depth',
    'xianzhi_async_canary_ppt_generation_stuck',
    'xianzhi_async_canary_outbox_failed',
    'xianzhi_async_canary_points_settlement_conflicts_total',
    'xianzhi_async_canary_artifact_recovery_failures_total',
    'xianzhi_async_canary_generation_stuck',
)


class ImageDLQScope(unittest.TestCase):
    def setUp(self):
        self.f = fixture.Interlock()
        self.f.setUp()
        # Keep real configuration field names, but use only synthetic values.
        self.original = (self.f.original +
                         'VIDEO_ASYNC_CANARY_USERS=fixture-user\n'
                         'VIDEO_ASYNC_CANARY_PROVIDER_ALLOWLIST=fixture-provider\n'
                         'VIDEO_ASYNC_CANARY_MODEL_ALLOWLIST=fixture-model\n'
                         'GENERATION_ASYNC_CANARY_USERS=fixture-image-user\n'
                         'GENERATION_ASYNC_CANARY_PROVIDER_ALLOWLIST=fixture-image-provider\n'
                         'GENERATION_ASYNC_CANARY_MODEL_ALLOWLIST=fixture-image-model\n'
                         '# unrelated fixture comment\n')
        self.set_env(self.original)

    def tearDown(self):
        self.f.tearDown()

    def set_env(self, content):
        self.f.original = content
        self.f.envfile.write_text(content)
        self.f.before = self.f.envfile.stat().st_mtime_ns

    def run_alarm(self, name=IMAGE_DLQ, value='8'):
        return self.f.monitor(FIXTURE_METRICS=name + ' ' + value + '\n').wait(timeout=5)

    def assert_api_only(self):
        argv = self.f.calls()[-1]
        self.assertEqual(argv[-1], 'xianzhi-ai')
        self.assertIn('--no-deps', argv)
        self.assertIn('--no-build', argv)
        self.assertEqual(argv[argv.index('--pull') + 1], 'never')
        # No worker, queue, provider or route operation is present.
        self.assertNotIn('migrate', argv)
        self.assertNotIn('smartvideo-worker', argv)
        self.assertNotIn('generation-worker', argv)
        self.assertFalse(self.f.lock.exists())

    def test_image_dlq_turns_off_only_image_preserving_video_true_and_allowlists(self):
        self.assertEqual(self.run_alarm(), 1)
        self.assertEqual(self.f.envfile.read_bytes(),
                         self.original.replace('GENERATION_ASYNC_CANARY_ENABLED=true',
                                               'GENERATION_ASYNC_CANARY_ENABLED=false').encode())
        self.assertEqual(self.f.envfile.stat().st_mode & 0o777, 0o600)
        self.assertEqual(len(self.f.calls()), 1)
        self.assert_api_only()

    def test_image_already_off_video_true_does_not_rewrite_env(self):
        original = self.original.replace('GENERATION_ASYNC_CANARY_ENABLED=true',
                                         'GENERATION_ASYNC_CANARY_ENABLED=false')
        self.set_env(original)
        self.assertEqual(self.run_alarm(), 1)
        self.assertEqual(self.f.envfile.read_bytes(), original.encode())
        self.assertEqual(self.f.envfile.stat().st_mtime_ns, self.f.before)
        self.assertEqual(len(self.f.calls()), 1)
        self.assert_api_only()

    def test_video_false_remains_false_on_image_dlq(self):
        original = self.original.replace('VIDEO_ASYNC_CANARY_ENABLED=true',
                                         'VIDEO_ASYNC_CANARY_ENABLED=false')
        self.set_env(original)
        self.assertEqual(self.run_alarm(), 1)
        self.assertEqual(self.f.envfile.read_bytes(),
                         original.replace('GENERATION_ASYNC_CANARY_ENABLED=true',
                                          'GENERATION_ASYNC_CANARY_ENABLED=false').encode())
        self.assert_api_only()

    def test_release_lock_keeps_both_canaries_and_all_env_bytes_unchanged(self):
        self.f.lock.mkdir(parents=True)
        (self.f.lock / 'owner_token').write_text('release-owner')
        (self.f.lock / 'owner_pid').write_text('unknown-release-owner')
        self.assertEqual(self.run_alarm(), 0)
        self.f.assert_untouched()
        self.assertEqual((self.f.lock / 'owner_token').read_text(), 'release-owner')
        self.assertFalse((self.f.root / 'metrics_called').exists())

    def test_zero_image_dlq_keeps_threshold_and_no_side_effects(self):
        self.assertEqual(self.run_alarm(value='0'), 0)
        self.f.assert_untouched()
        self.assertFalse(self.f.lock.exists())

    def test_video_ppt_and_other_alert_branches_retain_prior_default_behavior(self):
        # Deliberately do NOT redesign legacy default behavior here: every
        # non-image-DLQ branch disabled all three flags before this patch.
        expected = self.original.replace('_ASYNC_CANARY_ENABLED=true',
                                         '_ASYNC_CANARY_ENABLED=false').encode()
        for metric in NON_IMAGE_ALERTS:
            with self.subTest(metric=metric):
                self.set_env(self.original)
                self.assertEqual(self.run_alarm(metric), 1)
                self.assertEqual(self.f.envfile.read_bytes(), expected)
                self.assert_api_only()
        self.assertEqual(len(self.f.calls()), len(NON_IMAGE_ALERTS))


if __name__ == '__main__':
    if sys.platform != 'linux':
        raise SystemExit('Linux fixture required; use the isolated production-contract image.')
    unittest.main(verbosity=2)
