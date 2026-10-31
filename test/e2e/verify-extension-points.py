#!/usr/bin/env python3
"""Bounded native extension-point checks. Synthetic replies; no LLM or provision."""

import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

FIXTURE = Path(__file__).resolve().parents[2] / "examples/nested-extension-points"
CASES = ("success", "repair", "failed", "continue", "blocked", "fail", "no_work", "call-partial", "missing-output", "absent", "disabled")


def digest(raw):
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def run(binary, *args, expect_ok=True):
    process = subprocess.run([str(binary), "--json", *map(str, args)], capture_output=True, text=True, timeout=120)
    if not expect_ok:
        assert process.returncode != 0, (args, process.stdout)
        return json.loads(process.stderr.strip().splitlines()[-1])
    assert process.returncode == 0, (args, process.stderr)
    value = json.loads(process.stdout)
    assert not value.get("receipt", {}).get("rejection"), value
    return value


def git(*args):
    subprocess.run(["git", "-c", "user.name=Pri-Fly fixture", "-c", "user.email=fixture@example.invalid", *map(str, args)], check=True, capture_output=True, timeout=30)


def run_case(binary, case):
    with tempfile.TemporaryDirectory(prefix="prifly-points-") as temporary:
        root = Path(temporary)
        repository, authority = root / "repository", root / "authority"
        repository.mkdir()
        git("-C", repository, "init", "-q", "-b", "main")
        run(binary, "project", "init", "--repository", repository, "--state-root", authority, "--host", "codex-app")
        folder = repository / ".prifly/workflows/transport"
        shutil.copytree(FIXTURE, folder)
        if case == "absent":
            (folder / "extend.yaml").write_text("authoring: prifly-extension/4\nextensions: []\n")
        if case == "disabled":
            p = folder / "extend.yaml"
            p.write_text(p.read_text().replace("    point:", "    enabled: false\n    point:"))
        profile = repository / ".prifly/project.yaml"
        text = profile.read_text().replace("packages: {}", "packages:\n  transport: {source: .prifly/workflows/transport}")
        text = text.replace("launches: {}", "launches:\n  transport:\n    title: Native extension points\n    description: Synthetic replies, no provision.\n    kind: workflow\n    workflow: .prifly/workflows/transport/workflow.yaml")
        profile.write_text(text)
        (repository / "metadata.txt").write_text("original\n")
        git("-C", repository, "add", "-A")
        git("-C", repository, "commit", "-qm", "Generic fixture")
        runs, seen, reports, logs, expected, producer_paths = [], [], {}, {}, {}, []
        missing_refusal = None

        def call(*args, **kwargs):
            return run(binary, "--project", authority, *args, **kwargs)

        def export(ref):
            reference, output = root / "ref.json", root / "export"
            reference.write_text(json.dumps(ref))
            output.unlink(missing_ok=True)
            call("artifact", "export", "--ref", reference, "--output", output)
            raw = output.read_bytes()
            assert digest(raw) == ref["digest"]
            return raw

        def submission(task, verdict, values, expect_ok=True):
            template = call("session", "submit", "--template", "--run", runs[-1], "--attempt", task["attempt_id"])
            template["result"]["outputs"] = {port: template["result"]["outputs"][port] for port in values}
            for port, raw in values.items():
                path = Path(task["workspace"]) / task["context"]["outputs"][port]["path"]
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(raw)
                template["result"]["outputs"][port]["digest"] = digest(raw)
            template["result"].update(verdict=verdict, summary="Synthetic extension point check")
            template["answered_questions"] = []
            path = Path(task["workspace"]) / "submission.json"
            path.write_text(json.dumps(template))
            return call("session", "submit", "--file", path, expect_ok=expect_ok)

        def check_carrier(carrier):
            assert carrier["reports"][0] == expected["original_ref"]
            for ref in carrier["reports"]:
                raw = export(ref)
                assert raw == reports[ref["digest"]]
                for log in json.loads(raw)["logs"]:
                    assert export(log) == logs[log["digest"]]

        try:
            compiled = call("project", "compile", "--repository", repository, "--package", "transport", "--output", root / "compiled")
            assert any(path.endswith("/extension-points.yaml") for path in compiled["sources"])
            arguments = ("--repository", repository, "--launch", "transport", "--host", "codex-app", "--workspace", "worktree")
            prepared = call("project", "questionnaire", *arguments, "--prepare")
            stale_prepare = None
            if case == "success":
                manifest = folder / "extension-points.yaml"
                original_manifest = manifest.read_text()
                manifest.write_text(original_manifest.replace("needs_revision: stopped", "needs_revision: blocked", 1))
                stale_prepare = call("project", "start", *arguments, "--expected-launch-digest", prepared["review_digest"], expect_ok=False)
                assert stale_prepare["code"] == "project_start_stale_launch", stale_prepare
                manifest.write_text(original_manifest)
            started = call("project", "start", *arguments, "--expected-launch-digest", prepared["review_digest"])
            runs.append(started["run"]["run"]["id"])
            continuation = None
            for _ in range(32):
                call("run", "drive", runs[-1])
                tasks = call("session", "task", "--run", runs[-1], "--all")["tasks"]
                if not tasks:
                    if case == "continue" and len(runs) == 1:
                        source = call("run", "status", runs[0])["run"]
                        assert source["outcome"] == "partial", source
                        continuation_args = ("project", "continue", "--repository", repository, "--launch", "transport", "--host", "codex-app", "--source-run", runs[0])
                        nested_resume = call(*continuation_args, "--from-stage", "measure-before-read", "--prepare", expect_ok=False)
                        assert nested_resume["code"] == "resume_from_stage_invalid", nested_resume
                        producer = folder / "steps/produce.yaml"
                        original_producer = producer.read_text()
                        producer.write_text(original_producer.replace("Publish one report", "Changed producer contract", 1))
                        changed_prefix = call(*continuation_args, "--prepare", expect_ok=False)
                        assert changed_prefix["code"] == "recover_prefix_changed", changed_prefix
                        producer.write_text(original_producer)
                        reviewed = call(*continuation_args, "--prepare")
                        assert reviewed["recovery"]["frontier_stage_id"] == "cycle"
                        assert call("run", "status", runs[0])["run"] == source
                        reused = [item["stage_id"] for item in reviewed["recovery"]["reused"]]
                        assert reused == ["produce"], reused
                        resumed = call(*continuation_args, "--expected-launch-digest", reviewed["review_digest"])
                        runs.append(resumed["run"]["run"]["id"])
                        assert call("run", "status", runs[0])["run"] == source
                        continuation = {"frontier": "cycle", "reused": reused, "source_unchanged": True, "nested_resume_refusal": nested_resume["code"], "changed_prefix_refusal": changed_prefix["code"]}
                        continue
                    break
                for task in tasks:
                    role = task["skill_refs"][0]["id"].rsplit("/", 1)[-1]
                    seen.append(role)
                    if "take_command" in task:
                        call("session", "take", "--run", runs[-1], "--attempt", task["attempt_id"])
                    for directory in producer_paths:
                        if directory.exists():
                            assert directory.is_relative_to(authority)
                            shutil.rmtree(directory)
                    values, verdict = {}, "pass"
                    if role == "produce":
                        carrier = {"candidate": "original", "reports": []}
                    else:
                        carrier = json.loads((Path(task["workspace"]) / task["context"]["inputs"]["carrier"]["path"]).read_text())
                        check_carrier(carrier)
                    if role in ("produce", "measure"):
                        if role == "measure":
                            assert task["permitted_effects"] == ["write_inside_declared_output_slot", "change_declared_external_target"], task["permitted_effects"]
                            if task["context"]["inputs"]["carrier"]["ref"] == expected.get("repair_ref"):
                                assert carrier["candidate"] == "metadata-repaired"
                            if case in ("blocked", "fail", "no_work"):
                                verdict = case
                            if case == "call-partial" and seen.count("read"):
                                verdict = "needs_revision"
                            if case == "missing-output":
                                missing_refusal = submission(task, "pass", {}, expect_ok=False)
                                assert missing_refusal["code"] == "output_required_missing", missing_refusal
                                verdict = "fail"
                        if verdict == "pass":
                            slots = task["context"]["outputs"]
                            if role == "measure" and case == "continue" and len(runs) == 2:
                                assert carrier["reports"] == [expected["original_ref"]], "stale measurement carried into continued root frontier"
                            number = seen.count("measure") if role == "measure" else 0
                            log = f"synthetic {role} {number}\n".encode()
                            log_ref = {key: slots["log"][key] for key in ("artifact_id", "revision")}
                            log_ref["digest"] = digest(log)
                            report = {"subject": f"dataset:{carrier['candidate']}", "basis": f"fixture-contract:{number}", "method": "synthetic check", "exit": 1 if case == "failed" and role == "produce" else 0, "status": "failed" if case == "failed" and role == "produce" else "passed", "logs": [log_ref]}
                            raw = json.dumps(report).encode()
                            ref = {key: slots["report"][key] for key in ("artifact_id", "revision")}
                            ref["digest"] = digest(raw)
                            carrier["reports"].append(ref)
                            reports[ref["digest"]], logs[log_ref["digest"]] = raw, log
                            if role == "produce":
                                expected["original_ref"] = ref
                            else:
                                expected["latest_carrier"] = json.loads(json.dumps(carrier))
                                expected["measurement_ref"] = {key: slots["carrier"][key] for key in ("artifact_id", "revision")}
                                expected["measurement_ref"]["digest"] = digest(json.dumps(carrier).encode())
                            values = {"carrier": json.dumps(carrier).encode(), "report": raw, "log": log}
                            producer_paths.append(Path(task["workspace"]))
                    elif role == "read":
                        assert task["permitted_effects"] == ["write_inside_declared_output_slot"], task["permitted_effects"]
                        if case not in ("absent", "disabled"):
                            assert carrier == expected["latest_carrier"]
                            assert task["context"]["inputs"]["carrier"]["ref"] == expected["measurement_ref"]
                        if case in ("repair", "call-partial") and seen.count("read") == 1:
                            verdict = "needs_revision"
                        elif case == "failed" or (case == "continue" and len(runs) == 1):
                            verdict = "blocked"
                    elif role == "repair":
                        assert carrier == expected["latest_carrier"]
                        tree = Path(task["repository_workspace"])
                        (tree / "metadata.txt").write_text("metadata repaired\n")
                        git("-C", tree, "add", "metadata.txt")
                        git("-C", tree, "commit", "-qm", "Synthetic repair")
                        carrier["candidate"] = "metadata-repaired"
                        raw = json.dumps(carrier).encode()
                        values = {"carrier": raw}
                        expected["repair_ref"] = {key: task["context"]["outputs"]["carrier"][key] for key in ("artifact_id", "revision")}
                        expected["repair_ref"]["digest"] = digest(raw)
                    else:
                        raise AssertionError(role)
                    submission(task, verdict, values)
            else:
                raise AssertionError("Bounded fixture exceeded 32 drive rounds")
            state = call("run", "status", runs[-1])["run"]
            wanted = "partial" if case == "failed" else "rejected" if case in ("blocked", "fail", "no_work", "call-partial", "missing-output") else "succeeded"
            assert state["status"] == "completed" and state["outcome"] == wanted, state
            if wanted != "rejected":
                final = json.loads(export(state["output_artifacts"]["carrier"]))
                check_carrier(final)
                if case not in ("failed", "absent", "disabled"):
                    assert final == expected["latest_carrier"]
            assert seen.count("produce") == 1
            assert seen.count("measure") == (0 if case in ("absent", "disabled") else 3 if case == "repair" else 2 if case in ("continue", "call-partial") else 1), seen
            if case == "repair":
                assert seen == ["produce", "measure", "read", "repair", "measure", "measure", "read"], seen
            invalid_refs = {}
            for name, change, code in (("unknown", {"artifact_id": "artifact:" + "0" * 64}, "not_found"), ("missing_revision", {"revision": 2}, "integrity_failure"), ("substituted_digest", {"digest": "sha256:" + "0" * 64}, "integrity_failure")):
                reference = root / "ref.json"
                reference.write_text(json.dumps({**expected["original_ref"], **change}))
                output = root / "bad-export"
                refused = call("artifact", "export", "--ref", reference, "--output", output, expect_ok=False)
                assert refused["code"] == code, refused
                invalid_refs[name] = code
            return {"case": case, "outcome": wanted, "handoffs": seen, "runs": len(runs), "fresh_carrier_checked": case not in ("absent", "disabled"), "report_log_refs_checked": True, "producer_directories_removed": True, "invalid_refs": invalid_refs, "continue": continuation, "missing_output_refusal": missing_refusal, "stale_prepare_refusal": stale_prepare["code"] if stale_prepare else None}
        finally:
            try:
                for run_id in runs:
                    state = call("run", "status", run_id)["run"]
                    if state["status"] not in ("completed", "failed", "cancelled"):
                        call("run", "cancel", run_id, "--reason", "Synthetic fixture cleanup")
                        driven = subprocess.run([str(binary), "--json", "--project", str(authority), "run", "drive", run_id], capture_output=True, text=True, timeout=120)
                        if driven.returncode:
                            assert json.loads(driven.stderr)["code"] == "recovery_required", driven.stderr
                            view = call("run", "status", run_id)["run"]
                            for attempt in view["attempts"].values():
                                if attempt["status"] == "uncertain":
                                    call("run", "resolve", run_id, "--attempt", attempt["id"], "--outcome", "not_applied", "--reason", "Synthetic fixture performed no external action")
                            call("run", "cancel", run_id, "--reason", "Settle synthetic fixture")
                            call("run", "drive", run_id)
                for claim in call("claim", "list")["claims"]:
                    if claim["status"] == "active":
                        call("claim", "release", "--id", claim["id"], "--generation", claim["generation"])
                assert not any(c["status"] == "active" for c in call("claim", "list")["claims"])
            finally:
                registry = Path.home() / ".prifly/monitor/sources"
                if registry.is_dir():
                    for entry in registry.iterdir():
                        if entry.is_file() and Path(entry.read_text().strip()).resolve() == authority.resolve():
                            entry.unlink()



def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--cases", nargs="+", choices=CASES, default=CASES)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    result = {"binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "boundary": __doc__, "cases": [], "complete": False}
    for case in args.cases:
        result["cases"].append(run_case(binary, case))
        print(case, "PASS", flush=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
    result["complete"] = True
    args.output.write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
