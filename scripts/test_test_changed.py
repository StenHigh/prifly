"""Checks for scripts/test-changed.py: the selection it makes from changed files."""

import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("test_changed", pathlib.Path(__file__).with_name("test-changed.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
M = module.MODULE
ROOT = "/repo"
GRAPH = {
    M + "/internal/local": {"dir": ROOT + "/internal/local", "imports": set()},
    M + "/internal/flow": {"dir": ROOT + "/internal/flow", "imports": set()},
    M + "/internal/runtime": {"dir": ROOT + "/internal/runtime", "imports": {M + "/internal/local", M + "/internal/flow"}},
    M + "/cmd/prifly": {"dir": ROOT + "/cmd/prifly", "imports": {M + "/internal/runtime"}},
    M + "/internal/testguard": {"dir": ROOT + "/internal/testguard", "imports": set()},
}


def packages(command):
    return sorted(command[2:])


class Selection(unittest.TestCase):
    def test_a_package_runs_with_everything_that_imports_it(self):
        command, _ = module.choose(["internal/local/store.go"], GRAPH, ROOT)
        self.assertEqual(packages(command), ["./cmd/prifly", "./internal/local", "./internal/runtime"])

    def test_a_file_read_by_path_runs_the_tests_that_read_it(self):
        # The author index test opens examples/README.md; it names no Go file.
        command, _ = module.choose(["examples/README.md"], GRAPH, ROOT)
        self.assertIn("./internal/runtime", packages(command))

    def test_a_changed_test_runs_the_parallel_guard(self):
        command, _ = module.choose(["internal/flow/flow_test.go"], GRAPH, ROOT)
        self.assertIn("./internal/testguard", packages(command))

    def test_a_schema_runs_the_schema_check(self):
        _, extra = module.choose(["schemas/core/questions.schema.json"], GRAPH, ROOT)
        self.assertIn(["make", "schemas-check"], extra)

    def test_the_monitor_page_runs_its_node_test(self):
        _, extra = module.choose(["cmd/prifly/monitor.js"], GRAPH, ROOT)
        self.assertIn(["node", "cmd/prifly/monitor_ui_test.cjs"], extra)

    def test_a_build_file_runs_everything(self):
        command, _ = module.choose(["Makefile"], GRAPH, ROOT)
        self.assertEqual(command, ["go", "test", "./..."])

    def test_an_unknown_place_runs_everything_rather_than_nothing(self):
        command, _ = module.choose(["somewhere/new.txt"], GRAPH, ROOT)
        self.assertEqual(command, ["go", "test", "./..."])

    def test_a_change_no_test_reads_runs_nothing(self):
        command, extra = module.choose(["openspec/changes/x/tasks.md"], GRAPH, ROOT)
        self.assertEqual((command, extra), (None, []))


if __name__ == "__main__":
    unittest.main()
