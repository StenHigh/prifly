#!/usr/bin/env python3
"""Exercise the real CLI, local processes, durable hooks and sealed outputs.

Requires an explicitly built binary. Keeps its temporary project and receipts
for inspection. This is separate from the wrapper's isolated protocol tests.
"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    target = args.target.resolve() if args.target else Path(tempfile.mkdtemp(prefix="prifly-cli-", dir="/tmp"))
    example = Path(__file__).resolve().parents[2] / "test/fixtures/foundation/create-demo.py"
    subprocess.run([sys.executable, "-B", str(example), "--binary", str(binary), "--target", str(target)], check=True, timeout=60, stdout=subprocess.PIPE)
    results = target / "verification"
    results.mkdir()

    def write(path, value):
        path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")

    def cli(*arguments):
        command = subprocess.run([str(binary), "--project", str(target), "--json", *arguments], capture_output=True, text=True, timeout=30)
        if command.returncode:
            raise RuntimeError(f"CLI {arguments[:2]} failed ({command.returncode}): {command.stderr}")
        return json.loads(command.stdout)

    cases = [
        ("transform", "transform", ["source=inputs/source.txt"], "succeeded", 1, 2, 0),
        ("checks-pass", "two-checks", ["first=inputs/first.json", "second=inputs/second.json"], "succeeded", 2, 4, 0),
        ("checks-rejected", "two-checks", ["first=inputs/rejected.json", "second=inputs/second.json"], "rejected", 1, 3, 1),
        ("shell", "shell", [], "succeeded", 1, 0, 0),
        ("transform-warning", "transform", ["source=inputs/unchanged.txt"], "succeeded", 1, 3, 1),
    ]
    runs, summary = {}, []
    for name, workflow, inputs, outcome, count, publications, warnings in cases:
        preview = cli("validate", "--workflow", f"workflows/{workflow}.json")
        assert preview["admission"] is False
        command = ["run", "start", "--workflow", f"workflows/{workflow}.json", "--brief", "brief.json", "--command-id", f"command:verify-{name}", "--drive"]
        for item in inputs:
            command += ["--input", item]
        view = cli(*command)
        write(results / f"{name}.json", view)
        run = view["run"]
        assert run["status"] == "completed" and run["outcome"] == outcome, (name, run["status"], run["outcome"])
        assert len(run["steps"]) == count and len(run["attempts"]) == count, name
        assert len(run["publications"]) == publications, name
        assert sum(d["severity"] == "warn" for d in run["diagnostics"]) == warnings, name
        assert all(a.get("accepted") and a["process_outcome"]["exit_code"] == 0 for a in run["attempts"].values()), name
        if name == "checks-pass":
            definitions = [step["definition_ref"] for step in run["steps"].values()]
            assert definitions[0] == definitions[1]
        if name == "checks-rejected":
            assert all(a["stage_id"] != "check_second" for a in run["activations"].values())
            assert set(run["output_artifacts"]) == {"report_first"}
        if name == "transform":
            assert [p["state_version"] for p in run["publications"]] == [1, 2]
        runs[name] = run
        summary.append({"case": name, "run_id": run["id"], "status": run["status"], "outcome": outcome, "attempts": count, "publications": publications, "warnings": warnings})

    for port, ref in runs["transform"]["output_artifacts"].items():
        write(results / f"{port}-ref.json", ref)
        cli("artifact", "export", "--ref", f"verification/{port}-ref.json", "--output", f"verification/exported-{port}")
        content = (results / f"exported-{port}").read_bytes()
        assert "sha256:" + hashlib.sha256(content).hexdigest() == ref["digest"]
    source = (target / "inputs/source.txt").read_bytes()
    assert (results / "exported-text").read_bytes() == source.decode().upper().encode()
    assert json.loads((results / "exported-report").read_bytes()) == {"bytes": len(source), "changed": True}

    # Reuse the no-output shell implementation with a new definition requiring
    # one output. A pass verdict alone must not satisfy that contract.
    registry = json.loads((target / "definitions.json").read_bytes())

    def register(path, kind, definition):
        write(target / path, definition)
        ref = cli("ref", path, "--id", definition["id"], "--version", definition["version"])
        registry["entries"].append({"ref": ref, "kind": kind, "path": path})
        return ref

    report_ref = next(e["ref"] for e in registry["entries"] if e["ref"]["id"] == "demo:schema/report")
    step = json.loads((target / "steps/shell.json").read_bytes())
    step["id"] = "demo:step/missing-output"
    step["outputs"] = {"report": {"format": "json", "schema_ref": report_ref, "required_for": ["pass"]}}
    step_ref = register("steps/missing-output.json", "step", step)
    workflow = json.loads((target / "workflows/shell.json").read_bytes())
    workflow["id"] = "demo:workflow/missing-output"
    workflow["definition"]["stages"]["shell"]["step_ref"] = step_ref
    register("workflows/missing-output.json", "workflow", workflow)
    write(target / "definitions.json", registry)
    config = json.loads((target / "prifly.json").read_bytes())
    config["configuration"]["executors"][step["id"]] = config["configuration"]["executors"]["demo:step/shell"]
    write(target / "prifly.json", config)
    failure = cli("run", "start", "--workflow", "workflows/missing-output.json", "--brief", "brief.json", "--command-id", "command:verify-missing-output", "--drive")
    write(results / "missing-output.json", failure)
    run = failure["run"]
    assert run["status"] == "failed" and run["outcome"] is None and not run["output_artifacts"]
    assert len(run["attempts"]) == 1 and all(a.get("accepted") is None for a in run["attempts"].values())
    assert any(d["code"] == "invalid_output" and d["severity"] == "error" for d in run["diagnostics"])
    summary.append({"case": "missing-output", "run_id": run["id"], "status": "failed", "outcome": None, "attempts": 1, "expected_rejection": "invalid_output"})

    cohort = [case["run_id"] for case in summary]
    for mode, metrics in [
        ("records", ["core.entities_started", "os.cpu_total", "step.processed_total", "step.quality_warnings"]),
        ("aggregate", ["core.failed_run_fraction", "core.first_attempt_pass_fraction", "core.warning_run_fraction"]),
    ]:
        query = {"schema_version": "telemetry-query/1", "mode": mode, "run_ids": cohort, "metrics": metrics, "limit": 1000}
        write(results / f"{mode}-query.json", query)
        telemetry = cli("telemetry", "query", "--file", f"verification/{mode}-query.json")
        write(results / f"telemetry-{mode}.json", telemetry)
        population = telemetry["population"]
        assert population["matched"] == population["terminal"] == 6
        assert population["attempts"] == population["started_attempts"] == population["settled_attempts"] == 7
        ratio = population["ratios"]["core.first_attempt_pass_fraction"]
        assert ratio["numerator"] == 5 and ratio["denominator"] == 7
        assert population["full_warning_coverage"] == 4 and population["unknown_warning_coverage"] == 2
        if mode == "records":
            cpu = [record for record in telemetry["records"] if record["metric"] == "os.cpu_total"]
            assert len(cpu) == 7 and all(record["quality"] == "measured" and record["value"] is not None for record in cpu)
        encoded = json.dumps(telemetry)
        assert all(private not in encoded for private in ("token_hash", "telemetry_cursor_key", "PRIFLY_TOKEN"))

    # The documented control path uses separate CLI processes. Pausing a ready
    # Run must not spawn a worker; release alone must not resume it. Keep this
    # additional Run outside the fixed six-Run analytics cohort above.
    started = cli("run", "start", "--workflow", "workflows/shell.json", "--brief", "brief.json", "--command-id", "command:verify-control-start")
    control_id = started["receipt"]["run_id"]
    saved_receipt = cli("command", "receipt", "--id", "command:verify-control-start")
    assert saved_receipt["receipt"] == started["receipt"]
    paused = cli("run", "pause", control_id, "--reason", "Verify the documented operator path", "--command-id", "command:verify-control-pause")
    duplicate = cli("run", "pause", control_id, "--reason", "Verify the documented operator path", "--command-id", "command:verify-control-pause")
    assert duplicate["duplicate"] is True and duplicate["receipt"] == paused["receipt"]
    view = cli("run", "drive", control_id)
    assert not view["run"]["attempts"]
    stop = next(s for s in view["run"]["stops"] if s["status"] == "active")
    blocked = subprocess.run([str(binary), "--project", str(target), "--json", "run", "resume", control_id, "--expected-version", str(view["run_version"]), "--reason", "Not yet released", "--command-id", "command:verify-control-blocked"], capture_output=True, text=True, timeout=30)
    assert blocked.returncode != 0 and not blocked.stdout
    assert json.loads(blocked.stderr)["code"] == "active_stop"
    cli("run", "release", control_id, "--expected-epoch", str(view["run"]["control_epoch"]), "--stop", f"{stop['id']}:{stop['generation']}", "--reason", "Explicitly release reviewed stop", "--command-id", "command:verify-control-release")
    view = cli("run", "drive", control_id)
    assert view["run"]["resume_required"] and not view["run"]["attempts"]
    cli("run", "resume", control_id, "--expected-version", str(view["run_version"]), "--reason", "Continue after explicit release", "--command-id", "command:verify-control-resume")
    view = cli("run", "drive", control_id)
    assert view["run"]["status"] == "completed" and view["run"]["outcome"] == "succeeded"
    assert len(view["run"]["attempts"]) == 1
    terminal_cut = view["cut"]
    assert cli("run", "drive", control_id)["cut"] == terminal_cut
    write(results / "pause-release-resume.json", view)
    summary.append({"case": "pause-release-resume", "run_id": control_id, "status": "completed", "outcome": "succeeded", "attempts": 1, "receipt_replay_verified": True})
    assert hashlib.sha256(binary.read_bytes()).hexdigest() == binary_digest, "binary changed during verification"
    report = {"binary_sha256": binary_digest, "project": str(target), "cases": summary, "export_bytes_verified": True, "telemetry_population_verified": True}
    write(results / "summary.json", report)
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
