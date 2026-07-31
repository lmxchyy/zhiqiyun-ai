import importlib.util
import unittest
from pathlib import Path


BRIDGE_PATH = Path(__file__).with_name("seedance_bridge.py")
SPEC = importlib.util.spec_from_file_location("seedance_bridge", BRIDGE_PATH)
BRIDGE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(BRIDGE)


class SeedanceBridgeAspectRatioTest(unittest.TestCase):
    def test_prefers_canonical_aspect_ratio_and_supports_legacy_tasks(self):
        resolver = getattr(BRIDGE, "_video_aspect_ratio", None)
        self.assertTrue(callable(resolver), "Seedance bridge must expose its ratio normalization behavior")
        self.assertEqual(resolver({"aspect_ratio": "9:16"}), "9:16")
        self.assertEqual(resolver({"ratio": "1:1"}), "1:1")
        self.assertEqual(
            resolver({"aspect_ratio": "16:9", "ratio": "9:16"}),
            "16:9",
        )


if __name__ == "__main__":
    unittest.main()
