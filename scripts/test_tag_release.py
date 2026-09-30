"""Checks for scripts/tag-release.py: which runs qualify a commit."""

import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("tag_release", pathlib.Path(__file__).with_name("tag-release.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def ok(name):
    return {"name": name, "status": "completed", "conclusion": "success"}


class Qualification(unittest.TestCase):
    def test_both_green_qualifies(self):
        self.assertEqual(module.missing_checks([ok("verify"), ok("qualify")]), [])

    def test_a_missing_qualification_is_named(self):
        self.assertEqual(module.missing_checks([ok("verify")]), ["qualify: never ran on this commit"])

    def test_a_running_or_failed_check_is_named(self):
        runs = [ok("verify"), {"name": "qualify", "status": "in_progress", "conclusion": None}]
        self.assertEqual(module.missing_checks(runs), ["qualify: still running"])
        runs = [{"name": "verify", "status": "completed", "conclusion": "failure"}, ok("qualify")]
        self.assertEqual(module.missing_checks(runs), ["verify: failure"])

    def test_a_later_success_counts(self):
        runs = [{"name": "qualify", "status": "completed", "conclusion": "failure"}, ok("qualify"), ok("verify")]
        self.assertEqual(module.missing_checks(runs), [])


if __name__ == "__main__":
    unittest.main()
