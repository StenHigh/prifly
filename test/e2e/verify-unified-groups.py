#!/usr/bin/env python3
"""Native /5 point groups, inherited /4, private hook and explicit carrier Call."""
import argparse
from contextlib import contextmanager
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


example = module("unified_example", ROOT / "examples/unified-composition/setup.py")
transport = module("unified_transport", ROOT / "test/e2e/verify-extension-points.py")
CASES = ("sequence", "parallel", "choice", "choice-default", "choice-unknown", "audit-fail", "technical-error", "cancel-after-audit")


@contextmanager
def fixture_directory():
    with tempfile.TemporaryDirectory(prefix="prifly-unified-group-") as temporary:
        try:
            yield temporary
        finally:
            authority = (Path(temporary) / "authority").resolve()
            registry = Path.home() / ".prifly/monitor/sources"
            if registry.is_dir():
                for entry in registry.iterdir():
                    if entry.is_file() and Path(entry.read_text().strip()).resolve() == authority:
                        entry.unlink()


def run_case(binary, case):
    with fixture_directory() as temporary:
        root = Path(temporary)
        repository, authority = root / "repository", root / "authority"
        repository.mkdir()
        transport.git("-C", repository, "init", "-q", "-b", "main")
        transport.run(binary, "project", "init", "--repository", repository, "--state-root", authority, "--host", "codex-app")
        operator = case if case in ("sequence", "parallel", "choice") else "choice" if case.startswith("choice-") else "parallel"
        example.build(repository, operator)
        delivery = repository / ".prifly/workflows/delivery"
        extension = delivery / "extend.yaml"
        value = json.loads(extension.read_text())
        if case == "choice-default":
            value["arrange"][0]["choice"]["branches"][0]["predicate"] = False
        if case == "choice-unknown":
            value["arrange"][0]["choice"]["branches"][0]["predicate"] = {"op": "eq", "left": {"kind": "field", "ref": {"from": "stage_output", "stage_id": "produce", "port": "carrier", "pointer": "/missing"}}, "right": {"kind": "literal", "value": True}}
        if case == "technical-error":
            path = delivery / "workflow.yaml"
            workflow = json.loads(path.read_text())
            workflow["package"]["references"]["process"] = "core:adapter/local-process@2.0.0"
            path.write_text(json.dumps(workflow))
            path = delivery / "project/steps/audit.yaml"
            step = json.loads(path.read_text())
            step["authoring"] = "prifly-step/1"
            step.pop("repository_workspace", None)
            step["executor"] = {"adapter_ref": "{{process}}", "operation": "process"}
            path.write_text(json.dumps(step))
            value["execution_bindings"] = {"steps": {"audit": {"executable": "fixture-python", "args": ["-c", "import sys; sys.exit(17)"], "timeout_ms": 30000, "grace_ms": 100, "max_output_bytes": 65536}}}
            transport.run(binary, "project", "local", "set", "--repository", repository, "--allow-executable", "fixture-python=" + str(Path(sys.executable).resolve()))
        extension.write_text(json.dumps(value))
        transport.git("-C", repository, "add", ".")
        transport.git("-C", repository, "commit", "-qm", "Unified group fixture")

        def call(*args, **kwargs):
            return transport.run(binary, "--project", authority, *args, **kwargs)

        args = ["--repository", repository, "--launch", "delivery", "--host", "codex-app", "--workspace", "worktree"]
        if case == "technical-error":
            args += ["--allow-execution"]
        prepared = call("project", "questionnaire", *args, "--prepare")
        assert prepared["schema_version"] == "project-launch-summary/7", prepared
        assert prepared["composition"]["schema_version"] == "workflow-composition/2"
        assert prepared["composition"]["parents"] == ["classic"]
        started = call("project", "start", *args, "--expected-launch-digest", prepared["review_digest"])
        assert started["schema_version"] == "project-start/7"
        run_id = started["run"]["run"]["id"]
        seen, origin, cancelled = [], None, False
        for _ in range(80):
            call("run", "drive", run_id)
            view = call("run", "status", run_id)["run"]
            tasks = call("session", "task", "--run", run_id, "--all")["tasks"]
            if not tasks:
                if view["status"] in ("completed", "failed", "cancelled"):
                    break
                continue
            # Accept audit first for a measured partial-settlement cancellation.
            tasks.sort(key=lambda task: view["steps"][task["step_instance_id"]]["definition_ref"]["id"].endswith("/audit"), reverse=True)
            for task in tasks:
                role = view["steps"][task["step_instance_id"]]["definition_ref"]["id"].rsplit("/", 1)[-1]
                if "take_command" in task:
                    call("session", "take", "--run", run_id, "--attempt", task["attempt_id"])
                seen.append(role)
                values = {}
                if role == "produce":
                    values["carrier"] = {"candidate": "original", "evidence": []}
                if role == "measure":
                    port = task["context"]["inputs"]["carrier"]
                    carrier = json.loads((Path(task["workspace"]) / port["path"]).read_text())
                    assert carrier["candidate"] == "original"
                    origin = port["ref"]
                    values["carrier"] = {"candidate": "measured", "evidence": [origin]}
                if role == "read":
                    carrier = json.loads((Path(task["workspace"]) / task["context"]["inputs"]["carrier"]["path"]).read_text())
                    assert carrier == {"candidate": "measured", "evidence": [origin]}, carrier
                    ref_path, output = root / "nested-ref.json", root / "nested-bytes.json"
                    ref_path.write_text(json.dumps(origin))
                    call("artifact", "export", "--ref", ref_path, "--output", output)
                    assert transport.digest(output.read_bytes()) == origin["digest"]
                    assert json.loads(output.read_text())["candidate"] == "original"
                if role in ("commit", "audit"):
                    carrier = json.loads((Path(task["workspace"]) / task["context"]["inputs"]["carrier"]["path"]).read_text())
                    assert carrier["candidate"] == "original", "output-free group replaced a consumer implicitly"
                submission = call("session", "submit", "--template", "--run", run_id, "--attempt", task["attempt_id"])
                submission["result"]["outputs"] = {name: submission["result"]["outputs"][name] for name in values}
                for name, value in values.items():
                    path = Path(task["workspace"]) / task["context"]["outputs"][name]["path"]
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_text(json.dumps(value))
                    submission["result"]["outputs"][name]["digest"] = transport.digest(path.read_bytes())
                submission["result"].update(verdict="fail" if case == "audit-fail" and role == "audit" else "pass", summary="Synthetic native unified group check")
                submission["answered_questions"] = []
                path = Path(task["workspace"]) / "submission.json"
                path.write_text(json.dumps(submission))
                call("session", "submit", "--file", path)
                if case == "cancel-after-audit" and role == "audit":
                    call("run", "cancel", run_id, "--reason", "Unified group cancellation check")
                    cancelled = True
                    break
            if cancelled:
                for _ in range(6):
                    call("run", "drive", run_id)
                break
        view = call("run", "status", run_id)["run"]
        if cancelled:
            assert view["status"] == "cancelled", view["status"]
        else:
            expected = "rejected" if case in ("audit-fail", "technical-error", "choice-unknown") else "succeeded"
            assert view["status"] == "completed" and view["outcome"] == expected, (case, view["status"], view["outcome"])
            assert ("commit" in seen) == (expected == "succeeded"), seen
        if case == "technical-error":
            failed = [item for item in view["invocations"].values() if item["status"] == "failed"]
            assert failed and all(item.get("outcome") is None for item in failed), failed
        if case in ("parallel", "sequence", "audit-fail"):
            assert seen.count("audit") == 1 and seen.count("measure") == 1 and seen.count("read") == 1, seen
        if case == "sequence":
            assert seen.index("read") < seen.index("audit") < seen.index("commit"), seen
        if case == "choice-default":
            assert seen == ["produce", "audit", "commit"], seen
        if case == "choice-unknown":
            assert seen == ["produce"], seen
        result = {"case": case, "run": run_id, "status": view["status"], "outcome": view["outcome"], "seen": seen, "nested_ref": origin, "summary_schema": prepared["schema_version"], "start_schema": started["schema_version"]}
        claim = started.get("workspace")
        if claim:
            call("claim", "release", "--id", claim["id"], "--generation", claim["generation"])
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--cases", choices=CASES, nargs="+", default=CASES)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    result = {"boundary": __doc__, "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "cases": [], "complete": False}
    for case in args.cases:
        result["cases"].append(run_case(binary, case))
        print(case, "PASS", flush=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
    result["complete"] = True
    args.output.write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
