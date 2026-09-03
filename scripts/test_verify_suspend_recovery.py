import importlib.util
from pathlib import Path
import unittest


SPEC = importlib.util.spec_from_file_location("suspend_probe", Path(__file__).with_name("verify-suspend-recovery.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class SleepBoundaryTest(unittest.TestCase):
    def test_requires_the_full_requested_sleep_interval(self):
        self.assertFalse(MODULE.sleep_boundary_observed(63_839, 52_070, 20_000))

    def test_accepts_a_full_observed_sleep_interval(self):
        self.assertTrue(MODULE.sleep_boundary_observed(72_000, 48_000, 20_000))


if __name__ == "__main__":
    unittest.main()
