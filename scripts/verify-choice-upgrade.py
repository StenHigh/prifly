#!/usr/bin/env python3
"""Exercise an actual Core upgrade on fresh authorities.

No database edits, authority copies or fabricated replay records are used.
The default choice check uses the frozen P2-01 executable. --extension call
uses the frozen P2-02 executable; --extension repeat uses frozen P2-03 call
trees. Both check scoped state plus the new journal boundary. Raw CLI
responses, fixtures and both projects remain inspectable.
"""

import argparse
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parents[1]
P201_SHA256 = "c0b8ef766414689fa6acc3dea39ac350554de1fd407822bcd1ebb89a913bebed"
P202_SHA256 = "fa421bd4bfa31e7ad4eb46e07c72bb857540cb7d53b3aa30a58b75984dd10e99"
P203_SHA256 = "41e5a84681aff5821ea213d5bb7d33fd5396df159b5bc9e08533fb46dc92dc17"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False) + "\n").encode()


def persisted(view):
    return {key: view[key] for key in ("schema_version", "run_version", "event_sequence", "run")}


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--old-binary", type=Path, required=True)
    parser.add_argument("--new-binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--extension", choices=("choice", "call", "repeat"), default="choice")
    args = parser.parse_args()
    extension = args.extension
    old_label, expected_old_sha256 = {"choice": ("P201", P201_SHA256), "call": ("P202", P202_SHA256), "repeat": ("P203", P203_SHA256)}[extension]
    scoped_upgrade = extension in ("call", "repeat")
    old, new = args.old_binary.resolve(strict=True), args.new_binary.resolve(strict=True)
    identities = {name: {"path": str(path), "sha256": digest(path.read_bytes())} for name, path in (("old", old), ("new", new))}
    if identities["old"]["sha256"] != expected_old_sha256:
        parser.error(f"old-binary must be the exact accepted {old_label} executable: " + expected_old_sha256)
    if identities["new"]["sha256"] == expected_old_sha256:
        parser.error(f"new-binary must differ from the frozen {old_label} executable")
    evidence = args.evidence.absolute()
    evidence.parent.mkdir(parents=True, exist_ok=True)
    with evidence.open("xb") as evidence_file:
        temporary = Path(tempfile.mkdtemp(prefix=f"prifly-{extension}-upgrade-", dir="/tmp")).resolve()
        previous, extensions = temporary / "previous", temporary / (extension + "s")
        transcript = temporary / "transcript"
        transcript.mkdir()
        report = {
            "schema_version": f"{extension}-upgrade-evidence/1", "outcome": "failed", "binaries": identities,
            "reference_" + old_label.lower() + "_sha256": expected_old_sha256, "harness_sha256": digest(Path(__file__).read_bytes()),
            "host": {"platform": platform.platform(), "machine": platform.machine(), "python": platform.python_version()},
            "temporary_directory": str(temporary), "previous_project": str(previous), extension + "_project": str(extensions),
            "commands": [], "comparisons": [],
            "limitations": [
                "Local cooperative processes only; no sandbox or external-effect guarantee.",
                "Both authorities are freshly initialized at their final paths; no SQL edits, authority copy, migration, upcast or restore.",
                "Old and new commands are sequential, not mixed-version concurrent writers.",
                {
                    "choice": "Before a choice event, P2-01 can inspect the ready Core Run but cannot compile or dispatch its unsupported workflow.",
                    "call": "Before a call event, P2-02 can inspect a compatible peer but refuses core-state/2 reads/dispatch and call compilation.",
                    "repeat": "Before a repeat event, P2-03 can inspect its compatible call tree but refuses core-state/3 reads/dispatch and repeat compilation.",
                }[extension],
                f"After a {extension} event, P2-{old_label[-2:]} refuses the entire authority, including an otherwise compatible Run in that authority.",
                "Public persisted Run fields/counters and complete events are compared; volatile as_of/live timing and hidden replay payloads are not.",
                "An exact Start retry may append diagnostic samples; its original receipt cut and Run/event sequence must remain unchanged.",
                "This check does not replace crash, concurrent control, F1 compatibility or full F2 acceptance tests.",
            ],
        }
        if scoped_upgrade:
            report["extension"], report["reference_old_sha256"] = extension, expected_old_sha256
            report["limitations"].append("Receipt comparisons cover every event-referenced receipt for the CLI owner; runner-scoped receipts are not exposed by command receipt.")

        def record_cli(label, argv, result):
            entry = {"label": label, "argv": argv, "exit_code": result.returncode}
            for stream in ("stdout", "stderr"):
                data = getattr(result, stream)
                name = f"{len(report['commands']):03d}-{label}.{stream}"
                (transcript / name).write_bytes(data)
                entry[stream] = {"file": name, "sha256": digest(data), "size_bytes": len(data)}
            report["commands"].append(entry)
            return entry

        def cli(binary, project, label, *arguments, rejection=None):
            argv = [str(binary), "--project", str(project), "--json", *map(str, arguments)]
            result = subprocess.run(argv, capture_output=True, timeout=90)
            entry = record_cli(label, argv, result)
            if rejection:
                assert result.returncode > 0 and not result.stdout, (label, "expected clean refusal")
                problem = json.loads(result.stderr)
                assert problem["code"] == rejection, (label, problem)
                entry["expected_rejection"] = problem
                return problem
            if result.returncode:
                raise RuntimeError(f"{label} failed ({result.returncode}); inspect {transcript / entry['stderr']['file']}")
            return json.loads(result.stdout)

        def write(project, name, value):
            path = project / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(encoded(value))

        def same(label, before, after):
            data = encoded(before)
            assert data == encoded(after), (label, "values differ")
            report["comparisons"].append({"check": label, "sorted_json_sha256": digest(data)})

        registries = {}

        def define(binary, project, kind, name, identifier, value):
            write(project, name, value)
            ref = cli(binary, project, project.name + "-ref-" + Path(name).stem, "ref", name, "--id", identifier, "--version", "1.0.0")
            assert set(ref) == {"id", "version", "digest"}
            registries.setdefault(project, []).append({"ref": ref, "kind": kind, "path": name})
            write(project, "definitions.json", {"schema_version": "1", "entries": registries[project]})
            return ref

        def initialize(binary, project):
            cli(binary, project, "init-" + project.name, "init", "--profile", "core-workflow/1", project)
            config = json.loads((project / "prifly.json").read_bytes())
            definitions = cli(binary, project, "inventory-" + project.name, "inventory")["definitions"]
            refs = {item["ref"]["id"]: item["ref"] for item in definitions}
            # Several immutable policy revisions can share an ID. Historical
            # fixtures use the actual project default, not inventory order.
            assert any(item["ref"] == config["default_policy_ref"] for item in definitions)
            refs["core:policy/local"] = config["default_policy_ref"]
            write(project, "brief.json", {"schema_version": "1", "id": "upgrade:brief/core", "subject": "Core compatibility",
                "desired_outcome": "Retain old facts and refuse unsupported execution", "in_scope": ["New local scratch authority"],
                "out_of_scope": ["Network and external effects"], "completion_criteria": ["Actual binary checks pass"], "source_refs": [], "assumptions": [], "confirmation": "explicit"})
            worker = (ROOT / "test/fixtures/foundation/empty-step.sh").read_bytes() + b"\nprintf 'start\\n' >>worker-starts\n"
            (project / "scripts").mkdir(exist_ok=True)
            (project / "scripts/worker.sh").write_bytes(worker)
            report["worker_sha256"] = digest(worker)
            step = define(binary, project, "step", "steps/worker.json", "upgrade:step/core-worker", {
                "schema_version": "1", "id": "upgrade:step/core-worker", "version": "1.0.0", "title": "Owned shell process", "kind": "command", "inputs": {}, "outputs": {},
                "executor": {"adapter_ref": refs["core:adapter/local-process"], "operation": "process"}, "context_refs": [], "required_capabilities": [],
                "effects": {"class": "workspace_write", "retry_class": "pure"}, "result_check_refs": [], "result_schema_ref": refs["core:schema/step-result"],
            })
            config["configuration"]["executors"][step["id"]] = {"executable": "/bin/sh", "args": ["worker.sh"], "files": {"worker.sh": "scripts/worker.sh"}, "environment": {}, "timeout_ms": 10000, "grace_ms": 100, "max_output_bytes": 1048576}
            write(project, "prifly.json", config)
            return refs, step, project / config["configuration"]["workspace_root"]

        def workflow(identifier, refs, stages, entry="done", inputs=None, outputs=None):
            return {"schema_version": "2", "id": identifier, "version": "1.0.0", "title": identifier,
                "inputs": inputs or {}, "outputs": outputs or {}, "allowed_outcomes": ["succeeded"],
                "definition": {"entry": entry, "stages": stages}, "limits": {"max_step_instances": 1, "max_control_transitions": 6, "max_parallelism": 1, "max_child_depth": 0}, "policy_ref": refs["core:policy/local"]}

        def finish(bindings=None):
            return {"kind": "finish", "outcome": "succeeded", "output_bindings": bindings or {}}

        def literal(value):
            return {"kind": "literal", "value": value}

        def events(binary, project, label, run_id):
            view = cli(binary, project, label, "run", "events", run_id, "--limit", "1000")["view"]
            assert not view["more"]
            return view

        def workers_completed(view, count=1):
            run = view["run"]
            assert run["status"] == "completed" and run["outcome"] == "succeeded" and len(run["attempts"]) == len(run["steps"]) == count
            for attempt in run["attempts"].values():
                process = attempt["process_outcome"]
                assert attempt["accepted"] and process["started"] and process["wait_returned"] and process["group_empty"] and process["exit_code"] == 0
                assert (Path(attempt["workspace"]) / "worker-starts").read_text() == "start\n"

        repeat_driver, repeat_streams, repeat_driver_recorded = None, None, False
        try:
            for name, binary in (("old", old), ("new", new)):
                identities[name]["version"] = cli(binary, previous, name + "-version", "version")
            refs, step, _ = initialize(old, previous)
            bool_ref = define(old, previous, "schema", "schemas/boolean.json", "upgrade:schema/core-boolean", {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "boolean"})
            settings_ref = define(old, previous, "schema", "schemas/settings.json", "upgrade:schema/core-settings", {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": {"enabled": {"type": "boolean"}}, "required": ["enabled"], "additionalProperties": False})
            projected = workflow("upgrade:workflow/core-projection", refs, {"done": finish({"enabled": {"from": "workflow_input", "port": "settings", "pointer": "/enabled", "projected_schema_ref": bool_ref}})},
                inputs={"settings": {"format": "json", "schema_ref": settings_ref, "required": True, "configuration": {"scope": "run", "default": {"enabled": True}}}},
                outputs={"enabled": {"format": "json", "schema_ref": bool_ref, "required_for": ["succeeded"]}})
            define(old, previous, "workflow", "workflows/projection.json", projected["id"], projected)
            worker_stage = {"kind": "step", "step_ref": step, "input_bindings": {}, "on": {"pass": "done"}}
            document = workflow("upgrade:workflow/core-worker", refs, {"work": worker_stage, "done": finish()}, entry="work")
            define(old, previous, "workflow", "workflows/worker.json", document["id"], document)
            start_arguments = ["run", "start", "--workflow", "workflows/projection.json", "--brief", "brief.json", "--command-id", "command:core-upgrade-projection"]
            original = cli(old, previous, "old-start-projection", *start_arguments)
            projection_id = original["receipt"]["run_id"]
            projection = cli(old, previous, "old-drive-projection", "run", "drive", projection_id)
            assert projection["run"]["status"] == "completed" and not projection["run"]["attempts"]
            terminal = cli(old, previous, "old-completed-worker", "run", "start", "--workflow", "workflows/worker.json", "--brief", "brief.json", "--command-id", "command:core-upgrade-terminal", "--drive")
            workers_completed(terminal)
            terminal_id = terminal["run"]["id"]
            ready = cli(old, previous, "old-ready-worker", "run", "start", "--workflow", "workflows/worker.json", "--brief", "brief.json", "--command-id", "command:core-upgrade-ready")
            ready_id = ready["receipt"]["run_id"]
            cli(old, previous, "old-pause-ready", "run", "pause", ready_id, "--reason", "Pause for actual upgrade", "--command-id", "command:core-upgrade-pause")
            previous_cases = [("projection", projection_id), ("terminal", terminal_id), ("ready", ready_id)]
            if scoped_upgrade:
                old_choice = workflow("upgrade:workflow/old-choice", refs, {
                    "pick": {"kind": "choice", "selection": "exclusive", "branches": [
                        {"id": "selected", "predicate": {"op": "eq", "left": literal(True), "right": literal(True)}, "next": "done"},
                    ]}, "done": finish(),
                }, entry="pick")
                define(old, previous, "workflow", "workflows/old-choice.json", old_choice["id"], old_choice)
                old_choice_view = cli(old, previous, "old-completed-choice", "run", "start", "--workflow", "workflows/old-choice.json", "--brief", "brief.json", "--command-id", "command:core-upgrade-old-choice", "--drive")
                assert old_choice_view["run"]["schema_version"] == "core-state/1" and old_choice_view["run"]["status"] == "completed" and not old_choice_view["run"]["attempts"]
                old_choice_id = old_choice_view["run"]["id"]
                previous_cases.append(("choice", old_choice_id))
            old_call_starts, old_call_ids = {}, []
            if extension == "repeat":
                old_body = workflow("upgrade:workflow/old-call-body", refs, {"work": worker_stage, "done": projected["definition"]["stages"]["done"]},
                    entry="work", inputs=projected["inputs"], outputs=projected["outputs"])
                old_body_ref = define(old, previous, "workflow", "workflows/old-call-body.json", old_body["id"], old_body)
                old_call = workflow("upgrade:workflow/old-call", refs, {
                    "invoke": {"kind": "call", "workflow_ref": old_body_ref, "input_bindings": {"settings": {"from": "workflow_input", "port": "settings"}}, "on": {"succeeded": "done"}},
                    "done": finish({"enabled": {"from": "stage_output", "stage_id": "invoke", "port": "enabled"}}),
                }, entry="invoke", inputs=projected["inputs"], outputs=projected["outputs"])
                old_call["limits"]["max_child_depth"] = 1
                define(old, previous, "workflow", "workflows/old-call.json", old_call["id"], old_call)
                for label in ("call_terminal", "call_ready"):
                    arguments = ["run", "start", "--workflow", "workflows/old-call.json", "--brief", "brief.json", "--command-id", "command:core-upgrade-" + label]
                    accepted = cli(old, previous, "old-start-" + label, *arguments)
                    identifier = accepted["receipt"]["run_id"]
                    old_call_ids.append(identifier)
                    old_call_starts[identifier] = {"arguments": arguments, "receipt": accepted["receipt"]}
                    if label == "call_terminal":
                        completed_call = cli(old, previous, "old-drive-call-terminal", "run", "drive", identifier)
                        workers_completed(completed_call)
                        assert completed_call["schema_version"] == "core-read/2" and completed_call["run"]["schema_version"] == "core-state/2" and len(completed_call["run"]["invocations"]) == 2
                    else:
                        cli(old, previous, "old-pause-call-ready", "run", "pause", identifier, "--reason", "Pause a state2 call for upgrade", "--command-id", "command:core-upgrade-pause-call")
                    previous_cases.append((label, identifier))
            baseline, history = {}, {}
            for label, identifier in previous_cases:
                baseline[identifier] = cli(old, previous, "old-status-" + label, "run", "status", identifier)
                history[identifier] = events(old, previous, "old-events-" + label, identifier)
            receipts = {}
            if scoped_upgrade:
                facts = [event for event in history[old_choice_id]["events"] if event["type"] == "stage.choice_decided"]
                assert len(facts) == 1 and facts[0]["data"]["schema_version"] == "choice-decision/1" and facts[0]["data"]["branch_id"] == "selected"
                receipt_actor = original["receipt"]["actor"]
                command_ids = sorted({event["command_id"] for view in history.values() for event in view["events"] if event["actor"] == receipt_actor})
                private_receipts = {(event["actor"], event["command_id"]) for view in history.values() for event in view["events"] if event["actor"] != receipt_actor}
                assert command_ids and private_receipts
                for index, command_id in enumerate(command_ids):
                    receipts[command_id] = cli(old, previous, f"old-journal-receipt-{index:03d}", "command", "receipt", "--id", command_id)
            cut = baseline[projection_id]["cut"]
            assert all(view["cut"] == cut for view in baseline.values())
            output_ref = projection["run"]["output_artifacts"]["enabled"]
            write(previous, "verification/output-ref.json", output_ref)
            metadata = cli(old, previous, "old-projection-artifact", "artifact", "inspect", "--ref", "verification/output-ref.json")
            if scoped_upgrade:
                cli(old, previous, "old-export-projection", "artifact", "export", "--ref", "verification/output-ref.json", "--output", "verification/old-enabled.json")
                old_output_bytes = (previous / "verification/old-enabled.json").read_bytes()
                assert old_output_bytes == b"true" and "sha256:" + digest(old_output_bytes) == output_ref["digest"]
            call_artifacts = []
            if extension == "repeat":
                pending, seen_refs = [], set()
                for identifier in old_call_ids:
                    saved = baseline[identifier]["run"]
                    assert saved["schema_version"] == "core-state/2" and "ready_stages" not in saved
                    pending.append(saved["brief_ref"])
                    for invocation in saved["invocations"].values():
                        assert invocation["run_id"] == identifier and "iteration" not in invocation
                        pending.extend(invocation["input_refs"].values())
                        pending.extend(invocation["output_refs"].values())
                    for instance in saved["steps"].values():
                        pending.extend(instance["outputs"].values())
                    assert all("repeat" not in activation for activation in saved["activations"].values())
                # Inspect the actual public refs and their provenance closure,
                # including each projection manifest; never read a DB table.
                for ref in pending:
                    key = (ref["artifact_id"], ref["revision"], ref["digest"])
                    if key in seen_refs:
                        continue
                    seen_refs.add(key)
                    index = len(call_artifacts)
                    ref_file, output_file = f"verification/call-artifact-{index:03d}-ref.json", f"verification/old-call-artifact-{index:03d}.bin"
                    write(previous, ref_file, ref)
                    artifact = cli(old, previous, f"old-call-artifact-{index:03d}", "artifact", "inspect", "--ref", ref_file)
                    cli(old, previous, f"old-call-export-{index:03d}", "artifact", "export", "--ref", ref_file, "--output", output_file)
                    data = (previous / output_file).read_bytes()
                    assert artifact["ref"] == ref and "sha256:" + digest(data) == ref["digest"]
                    call_artifacts.append({"ref": ref, "ref_file": ref_file, "old_output": output_file, "metadata": artifact, "sha256": digest(data)})
                    pending.extend(artifact["artifact"]["provenance"])

            def compare_call_artifacts(phase):
                for index, sample in enumerate(call_artifacts):
                    label = f"call-artifact-{phase}-{index:03d}"
                    same(label + " metadata", sample["metadata"], cli(new, previous, label, "artifact", "inspect", "--ref", sample["ref_file"]))
                    output = f"verification/{label}.bin"
                    cli(new, previous, label + "-export", "artifact", "export", "--ref", sample["ref_file"], "--output", output)
                    same(label + " bytes", (previous / sample["old_output"]).read_bytes().hex(), (previous / output).read_bytes().hex())

            telemetry = {}
            for mode, metrics in (("records", ["core.command_duration", "os.cpu_total", "timing.elapsed"]), ("aggregate", ["core.succeeded_run_fraction", "core.failed_run_fraction"])):
                write(previous, f"verification/{mode}.json", {"schema_version": "telemetry-query/1", "mode": mode, "run_ids": list(baseline), "metrics": metrics, "cut": cut, "limit": 1000})
                telemetry[mode] = cli(old, previous, "old-telemetry-" + mode, "telemetry", "query", "--file", f"verification/{mode}.json")
                expected_revisions = ("core-telemetry/2", "core-timing/1") if extension == "repeat" else ("core-telemetry/1", "foundation-timing/1")
                assert (telemetry[mode]["calculator_revision"], telemetry[mode]["timing_revision"]) == expected_revisions and not telemetry[mode].get("next_cursor")
            assert any(r["metric"] == "os.cpu_total" and r["quality"] == "measured" for r in telemetry["records"]["records"])
            for label, identifier in previous_cases:
                current = cli(new, previous, "new-status-" + label, "run", "status", identifier)
                same("new reader preserves " + label, persisted(baseline[identifier]), persisted(current))
                same("new reader preserves events " + label, history[identifier], events(new, previous, "new-events-" + label, identifier))
                assert current["cut"] == cut
            same("old projection metadata preserved", metadata, cli(new, previous, "new-projection-artifact", "artifact", "inspect", "--ref", "verification/output-ref.json"))
            cli(new, previous, "new-export-projection", "artifact", "export", "--ref", "verification/output-ref.json", "--output", "verification/enabled.json")
            data = (previous / "verification/enabled.json").read_bytes()
            assert data == b"true" and "sha256:" + digest(data) == output_ref["digest"]
            if scoped_upgrade:
                same("old exported projection bytes preserved", old_output_bytes.decode(), data.decode())
                for index, (command_id, receipt) in enumerate(receipts.items()):
                    same(f"old journal receipt preserved before writes {index}", receipt, cli(new, previous, f"new-journal-receipt-{index:03d}", "command", "receipt", "--id", command_id))
            compare_call_artifacts("before-writes")
            for mode in telemetry:
                same("old-cut " + mode + " before writes", telemetry[mode], cli(new, previous, "new-telemetry-" + mode, "telemetry", "query", "--file", f"verification/{mode}.json"))
            retried = cli(new, previous, "new-exact-start-retry", *start_arguments)
            assert retried["duplicate"] is True
            same(f"exact {old_label} Start receipt and original cut", original["receipt"], retried["receipt"])
            same("exact retry leaves projection Run unchanged", persisted(baseline[projection_id]), persisted(cli(new, previous, "new-status-after-retry", "run", "status", projection_id)))
            same("exact retry leaves projection events unchanged", history[projection_id]["events"], events(new, previous, "new-events-after-retry", projection_id)["events"])
            paused = cli(new, previous, "new-drive-paused", "run", "drive", ready_id)
            same("paused old Run does not dispatch", persisted(baseline[ready_id]), persisted(paused))
            stop = next(s for s in paused["run"]["stops"] if s["status"] == "active")
            cli(new, previous, "new-release", "run", "release", ready_id, "--expected-epoch", paused["run"]["control_epoch"], "--stop", f"{stop['id']}:{stop['generation']}", "--reason", "Upgrade verified", "--command-id", "command:core-upgrade-release")
            released = cli(new, previous, "new-drive-released", "run", "drive", ready_id)
            assert released["run"]["resume_required"] and not released["run"]["attempts"]
            cli(new, previous, "new-resume", "run", "resume", ready_id, "--expected-version", released["run_version"], "--reason", "Explicit continuation", "--command-id", "command:core-upgrade-resume")
            resumed = cli(new, previous, "new-drive-resumed", "run", "drive", ready_id)
            workers_completed(resumed)
            same(old_label + " can still inspect a compatible Run after new dispatch", persisted(resumed), persisted(cli(old, previous, "old-reads-resumed", "run", "status", ready_id)))
            if extension == "repeat":
                for index, (identifier, saved) in enumerate(old_call_starts.items()):
                    retried_call = cli(new, previous, f"new-exact-call-retry-{index}", *saved["arguments"])
                    assert retried_call["duplicate"] is True
                    same(f"exact P203 call Start receipt {index}", saved["receipt"], retried_call["receipt"])
                    same(f"exact retry leaves state2 call {index} unchanged", persisted(baseline[identifier]), persisted(cli(new, previous, f"new-call-status-after-retry-{index}", "run", "status", identifier)))
                old_call_ready_id = old_call_ids[1]
                paused_call = cli(new, previous, "new-drive-paused-call", "run", "drive", old_call_ready_id)
                same("paused state2 call does not dispatch", persisted(baseline[old_call_ready_id]), persisted(paused_call))
                stop = next(s for s in paused_call["run"]["stops"] if s["status"] == "active")
                cli(new, previous, "new-release-call", "run", "release", old_call_ready_id, "--expected-epoch", paused_call["run"]["control_epoch"], "--stop", f"{stop['id']}:{stop['generation']}", "--reason", "Upgrade verified for call state2", "--command-id", "command:core-upgrade-call-release")
                released_call = cli(new, previous, "new-drive-released-call", "run", "drive", old_call_ready_id)
                assert released_call["run"]["resume_required"] and not released_call["run"]["attempts"]
                cli(new, previous, "new-resume-call", "run", "resume", old_call_ready_id, "--expected-version", released_call["run_version"], "--reason", "Explicit call continuation", "--command-id", "command:core-upgrade-call-resume")
                resumed_call = cli(new, previous, "new-drive-resumed-call", "run", "drive", old_call_ready_id)
                workers_completed(resumed_call)
                assert resumed_call["schema_version"] == "core-read/2" and resumed_call["run"]["schema_version"] == "core-state/2" and len(resumed_call["run"]["invocations"]) == 2
                same("P203 reads resumed state2 call after new dispatch", persisted(resumed_call), persisted(cli(old, previous, "old-reads-resumed-call", "run", "status", old_call_ready_id)))
                same("finished state2 call remains unchanged", persisted(baseline[old_call_ids[0]]), persisted(cli(new, previous, "new-final-call-terminal", "run", "status", old_call_ids[0])))
                compare_call_artifacts("after-writes")
            same("old accepted worker remains unchanged", persisted(baseline[terminal_id]), persisted(cli(new, previous, "new-original-terminal", "run", "status", terminal_id)))
            for mode in telemetry:
                same("old-cut " + mode + " after writes", telemetry[mode], cli(new, previous, "new-historical-" + mode, "telemetry", "query", "--file", f"verification/{mode}.json"))
            population = cli(new, previous, "new-previous-population", "telemetry", "catalog")["population"]
            assert population["matched"] == len(previous_cases) and population["attempts"] == (4 if extension == "repeat" else 2)
            report["previous"] = {"projection_run_id": projection_id, "terminal_run_id": terminal_id, "continued_run_id": ready_id, "historical_cut": cut, "exact_retry_receipt_cut": retried["receipt"]["cut"], "population": population}
            if scoped_upgrade:
                report["previous"].update({"choice_run_id": old_choice_id, "old_choice_decisions": 1, "retained_receipts": len(receipts),
                    "receipt_actor": receipt_actor, "runner_receipts_not_exposed": len(private_receipts), "output_bytes_sha256": digest(old_output_bytes)})
                same("old choice Run remains unchanged after new writes", persisted(baseline[old_choice_id]), persisted(cli(new, previous, "new-final-old-choice", "run", "status", old_choice_id)))
                for index, (command_id, receipt) in enumerate(receipts.items()):
                    same(f"old journal receipt preserved after writes {index}", receipt, cli(new, previous, f"new-final-journal-receipt-{index:03d}", "command", "receipt", "--id", command_id))
            if extension == "repeat":
                report["previous"].update({"call_terminal_run_id": old_call_ids[0], "call_continued_run_id": old_call_ids[1], "call_state_version": "core-state/2",
                    "call_artifacts": [{"ref": sample["ref"], "bytes_sha256": sample["sha256"]} for sample in call_artifacts],
                    "historical_calculator_revision": telemetry["records"]["calculator_revision"], "historical_timing_revision": telemetry["records"]["timing_revision"]})
            # Compare event facts after all new writes, not the read view's
            # newer global cut. Continuation may only append to the old prefix.
            report["previous"]["event_counts"] = {}
            for label, identifier in previous_cases:
                label = "continued" if label == "ready" else label
                original_events = history[identifier]["events"]
                current_events = events(new, previous, "new-final-events-" + label, identifier)["events"]
                report["previous"]["event_counts"][label] = {"before": len(original_events), "after": len(current_events)}
                if identifier == ready_id or extension == "repeat" and identifier == old_call_ids[1]:
                    assert len(current_events) > len(original_events), "continued Run did not append events"
                    same(label + " Run preserves complete old event prefix", original_events, current_events[:len(original_events)])
                else:
                    same("later writes preserve full old " + label + " history", original_events, current_events)

            # New authority: a compatible old Run and the new operator coexist.
            # Check the boundary both before and after the first new event.
            # Keep the old configuration valid so a refusal proves the state
            # or journal boundary rather than a newer Init default policy.
            refs, step, workspace = initialize(old, extensions)
            empty = workflow("upgrade:workflow/empty-core", refs, {"done": finish()})
            empty_ref = define(new, extensions, "workflow", "workflows/empty.json", empty["id"], empty)
            peer_file = "workflows/empty.json"
            if extension == "repeat":
                peer = workflow("upgrade:workflow/call-peer", refs, {
                    "invoke": {"kind": "call", "workflow_ref": empty_ref, "input_bindings": {}, "on": {"succeeded": "done"}}, "done": finish(),
                }, entry="invoke")
                peer["limits"]["max_child_depth"] = 1
                peer_file = "workflows/call-peer.json"
                define(old, extensions, "workflow", peer_file, peer["id"], peer)
            legacy = cli(old, extensions, "old-creates-compatible-peer", "run", "start", "--workflow", peer_file, "--brief", "brief.json", "--command-id", f"command:{extension}-legacy", "--drive")
            legacy_id = legacy["run"]["id"]
            assert legacy["run"]["status"] == "completed" and not legacy["run"]["attempts"]
            if extension == "call":
                new_definitions = cli(new, extensions, "new-call-policy-inventory", "inventory")["definitions"]
                policies = [item["ref"] for item in new_definitions if item["ref"]["id"] == "core:policy/local" and item["ref"]["version"] == "2.0.0"]
                assert len(policies) == 1 and refs["core:policy/local"]["version"] == "1.0.0"
                call_refs = {**refs, "core:policy/local": policies[0]}
                child = workflow("upgrade:workflow/call-child", call_refs, {
                    "work": {"kind": "step", "step_ref": step, "input_bindings": {}, "on": {"pass": "done"}}, "done": finish(),
                }, entry="work")
                child_ref = define(new, extensions, "workflow", "workflows/child.json", child["id"], child)
                document = workflow("upgrade:workflow/call", call_refs, {
                    "invoke": {"kind": "call", "workflow_ref": child_ref, "input_bindings": {}, "on": {"succeeded": "done"}}, "done": finish(),
                }, entry="invoke")
                document["limits"]["max_child_depth"] = 1
                new_event = "invocation.created"
            elif extension == "repeat":
                assert refs["core:policy/local"]["version"] == "2.0.0"
                assert legacy["run"]["schema_version"] == "core-state/2" and len(legacy["run"]["invocations"]) == 2
                child = workflow("upgrade:workflow/repeat-body", refs, {
                    "work": {"kind": "step", "step_ref": step, "input_bindings": {}, "on": {"pass": "done"}}, "done": finish(),
                }, entry="work")
                child_ref = define(new, extensions, "workflow", "workflows/body.json", child["id"], child)
                document = workflow("upgrade:workflow/repeat", refs, {
                    "repeat": {"kind": "repeat", "body_workflow_ref": child_ref, "initial_bindings": {}, "next_bindings": {},
                        "continue_on": ["succeeded"], "until": {"op": "eq", "left": literal(False), "right": literal(True)},
                        "max_iterations": 2, "on_complete": {"succeeded": "done"}, "on_limit": "done"}, "done": finish(),
                }, entry="repeat")
                document["limits"].update(max_step_instances=2, max_control_transitions=8, max_child_depth=1)
                # This cooperative worker waits only in its own workspace.
                # It allows inspection after entry, before any repeat decision,
                # without production pause hooks or manufactured journal facts.
                worker = b"#!/bin/sh\nset -eu\nprintf 'start\\n' >>worker-starts\n: >worker-ready\nremaining=600\nwhile [ ! -f worker-release ]; do\n  remaining=$((remaining - 1))\n  [ \"$remaining\" -gt 0 ] || exit 71\n  sleep 0.05\ndone\n" + (ROOT / "test/fixtures/foundation/empty-step.sh").read_bytes()
                (extensions / "scripts/worker.sh").write_bytes(worker)
                config = json.loads((extensions / "prifly.json").read_bytes())
                config["configuration"]["executors"][step["id"]]["timeout_ms"] = 60000
                write(extensions, "prifly.json", config)
                report["repeat_worker_sha256"] = digest(worker)
                report["limitations"][2] = "No mixed-version writers are admitted; old refusal probes run while one owned new driver waits in a bounded cooperative worker."
                report["limitations"].append("Only the new CLI process created by this harness may be killed on failure; a worker's own bounded wait also expires without external PID signalling.")
                new_event = "stage.repeat_entered"
            else:
                document = workflow("upgrade:workflow/choice", refs, {
                    "pick": {"kind": "choice", "selection": "exclusive", "branches": [
                        {"id": "selected", "predicate": {"op": "eq", "left": literal(True), "right": literal(True)}, "next": "work"},
                        {"id": "other", "predicate": {"op": "eq", "left": literal(True), "right": literal(False)}, "next": "unselected"},
                    ]},
                    "work": {"kind": "step", "step_ref": step, "input_bindings": {}, "on": {"pass": "done"}},
                    "unselected": {"kind": "step", "step_ref": step, "input_bindings": {}, "on": {"pass": "done"}}, "done": finish(),
                }, entry="pick")
                new_event = "stage.choice_decided"
            workflow_file = f"workflows/{extension}.json"
            define(new, extensions, "workflow", workflow_file, document["id"], document)
            started = cli(new, extensions, "new-start-" + extension, "run", "start", "--workflow", workflow_file, "--brief", "brief.json", "--command-id", "command:new-" + extension)
            extension_id = started["receipt"]["run_id"]
            before = cli(new, extensions, "new-ready-" + extension, "run", "status", extension_id)
            before_events = events(new, extensions, "new-ready-" + extension + "-events", extension_id)
            assert not list(workspace.iterdir()) and not before["run"]["steps"] and not before["run"]["attempts"]
            assert not any(e["type"] == new_event for e in before_events["events"])
            if scoped_upgrade:
                state_revision = "3" if extension == "repeat" else "2"
                assert before["schema_version"] == "core-read/" + state_revision and before["run"]["schema_version"] == "core-state/" + state_revision
                assert "ready_stages" not in before["run"] and len(before["run"]["invocations"]) == 1
                same(old_label + " reads compatible peer before a " + extension + " event", persisted(legacy), persisted(cli(old, extensions, "old-peer-before-" + extension + "-event", "run", "status", legacy_id)))
                # The frozen binary retains the strict decoder's json error
                # prefix as its problem code; do not rename that wire fact.
                cli(old, extensions, "old-refuses-" + extension + "-state", "run", "status", extension_id, rejection="json")
            else:
                same("old reader can inspect before new choice event", persisted(before), persisted(cli(old, extensions, "old-reads-ready-choice", "run", "status", extension_id)))
            cli(old, extensions, "old-refuses-" + extension + "-drive", "run", "drive", extension_id, rejection="json" if scoped_upgrade else "unsupported")
            cli(old, extensions, "old-refuses-" + extension + "-start", "run", "start", "--workflow", workflow_file, "--brief", "brief.json", "--command-id", f"command:old-{extension}-refused", "--drive", rejection="unsupported")
            after = cli(new, extensions, "new-" + extension + "-after-old-refusals", "run", "status", extension_id)
            same("old unsupported operations preserve ready Run", persisted(before), persisted(after))
            same("old unsupported operations preserve journal", before_events, events(new, extensions, "new-" + extension + "-events-after-refusals", extension_id))
            assert after["cut"] == before["cut"] and not list(workspace.iterdir())
            if extension == "repeat":
                argv = [str(new), "--project", str(extensions), "--json", "run", "drive", extension_id]
                # File streams avoid a full stdout pipe blocking CLI exit
                # while this parent is releasing the worker workspaces.
                repeat_streams = (tempfile.TemporaryFile(), tempfile.TemporaryFile())
                repeat_driver = subprocess.Popen(argv, stdout=repeat_streams[0], stderr=repeat_streams[1])
                deadline, probe = time.monotonic() + 25, 0
                while True:
                    assert repeat_driver.poll() is None, "repeat driver exited before the controlled entry boundary"
                    if list(workspace.glob("*/worker-ready")):
                        entered = cli(new, extensions, f"new-repeat-entry-status-{probe}", "run", "status", extension_id)
                        probe += 1
                        attempts = list(entered["run"]["attempts"].values())
                        if len(attempts) == 1 and attempts[0].get("started"):
                            break
                    assert time.monotonic() < deadline, "owned worker did not reach the bounded entry probe"
                    time.sleep(0.025)
                entry_events = events(new, extensions, "new-repeat-entry-events", extension_id)
                entry_facts = [event for event in entry_events["events"] if event["type"] == new_event]
                assert len(entry_facts) == 1 and not any(event["type"] == "stage.repeat_decided" for event in entry_events["events"])
                assert len(entered["run"]["invocations"]) == 2 and entered["run"]["status"] == "running"
                for label, operation, identifier in (("repeat-status", "status", extension_id), ("repeat-drive", "drive", extension_id), ("peer-status", "status", legacy_id), ("peer-drive", "drive", legacy_id)):
                    cli(old, extensions, "old-entry-refuses-" + label, "run", operation, identifier, rejection="unsupported_storage_version")
                same("repeat entry refusal preserves exact event view", entry_events, events(new, extensions, "new-repeat-entry-events-after-refusals", extension_id))
                entered_after = cli(new, extensions, "new-repeat-entry-after-refusals", "run", "status", extension_id)
                same("repeat entry refusal preserves running state", persisted(entered), persisted(entered_after))
                assert entered_after["cut"] == entered["cut"]
                same("compatible state2 peer survives entry refusal", persisted(legacy), persisted(cli(new, extensions, "new-peer-after-entry-refusal", "run", "status", legacy_id)))
                report["entry_boundary"] = {"cut": entered["cut"], "event_sequence": entered["event_sequence"], "repeat_entries": 1, "repeat_decisions": 0,
                    "body_invocation_id": entry_facts[0]["data"]["body_workflow_invocation_id"], "attempt_id": attempts[0]["id"], "refused_old_commands": 4}
                deadline = time.monotonic() + 30
                while repeat_driver.poll() is None:
                    for marker in workspace.glob("*/worker-ready"):
                        (marker.parent / "worker-release").touch()
                    assert time.monotonic() < deadline, "owned driver did not finish after releasing both workers"
                    time.sleep(0.025)
                repeat_driver.wait(timeout=5)
                for stream in repeat_streams:
                    stream.seek(0)
                stdout, stderr = (stream.read() for stream in repeat_streams)
                result = subprocess.CompletedProcess(argv, repeat_driver.returncode, stdout, stderr)
                entry = record_cli("new-drive-repeat", argv, result)
                repeat_driver_recorded = True
                if result.returncode:
                    raise RuntimeError(f"new-drive-repeat failed ({result.returncode}); inspect {transcript / entry['stderr']['file']}")
                completed = json.loads(stdout)
            else:
                completed = cli(new, extensions, "new-drive-" + extension, "run", "drive", extension_id)
            workers_completed(completed, 2 if extension == "repeat" else 1)
            assert not any(a["stage_id"] == "unselected" for a in completed["run"]["activations"].values())
            committed = events(new, extensions, "new-committed-" + extension + "-events", extension_id)
            facts = [e for e in committed["events"] if e["type"] == new_event]
            assert len(facts) == 1
            if extension == "call":
                run = completed["run"]
                child_id = facts[0]["data"]["workflow_invocation_id"]
                child = run["invocations"][child_id]
                assert len(run["invocations"]) == 2 and child["run_id"] == extension_id and child["workflow_ref"] == child_ref
                assert child["parent_invocation_id"] == run["root_workflow_invocation_id"] and child["status"] == "completed" and child["outcome"] == "succeeded"
                caller = run["activations"][child["caller_stage_activation_id"]]
                assert caller["kind"] == "call" and caller["status"] == "completed" and "step_instance_id" not in caller
                attempt = next(iter(run["attempts"].values()))
                assert run["activations"][attempt["stage_activation_id"]]["workflow_invocation_id"] == child_id
                returned = [e for e in committed["events"] if e["type"] == "stage.call_returned"]
                assert len(returned) == 1 and returned[0]["data"]["workflow_invocation_id"] == child_id and returned[0]["data"]["outcome"] == "succeeded"
            elif extension == "repeat":
                run = completed["run"]
                bodies = sorted((inv for inv in run["invocations"].values() if "iteration" in inv), key=lambda inv: inv["iteration"])
                assert len(run["invocations"]) == 3 and [inv["iteration"] for inv in bodies] == [1, 2]
                caller = run["activations"][bodies[0]["caller_stage_activation_id"]]
                assert caller["kind"] == "repeat" and caller["status"] == "completed" and "step_instance_id" not in caller
                assert caller["repeat"]["iteration_count"] == 2 and caller["repeat"]["current_body_workflow_invocation_id"] == bodies[-1]["id"] and run["control_transitions"] == 8
                decisions = [event["data"] for event in committed["events"] if event["type"] == "stage.repeat_decided"]
                assert len(decisions) == 2 and [decision["route"] for decision in decisions] == ["continue", "on_limit"]
                for index, body in enumerate(bodies):
                    assert body["run_id"] == extension_id and body["workflow_ref"] == child_ref and body["parent_invocation_id"] == run["root_workflow_invocation_id"]
                    assert body["status"] == "completed" and body["outcome"] == "succeeded" and body["control_transitions"] == 2 and body["step_instances"] == 1
                    assert decisions[index]["body_workflow_invocation_id"] == body["id"] and decisions[index]["iteration"] == index + 1
                assert decisions[0]["next_body_workflow_invocation_id"] == bodies[1]["id"] and decisions[0]["observation"] == bodies[1]["created"]
                assert caller["repeat"]["last_decision"] == decisions[1]
                assert {run["activations"][attempt["stage_activation_id"]]["workflow_invocation_id"] for attempt in run["attempts"].values()} == {body["id"] for body in bodies}
            else:
                assert facts[0]["data"]["schema_version"] == "choice-decision/1" and facts[0]["data"]["branch_id"] == "selected"
            names = sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*"))
            for label, operation, identifier in ((extension + "-status", "status", extension_id), (extension + "-drive", "drive", extension_id), ("legacy-status", "status", legacy_id), ("legacy-drive", "drive", legacy_id)):
                cli(old, extensions, "old-refuses-" + label, "run", operation, identifier, rejection="unsupported_storage_version")
            after = cli(new, extensions, "new-" + extension + "-after-downgrade", "run", "status", extension_id)
            same("old unknown-event refusals preserve Run", persisted(completed), persisted(after))
            same("old unknown-event refusals preserve event history", committed, events(new, extensions, "new-events-after-downgrade", extension_id))
            assert after["cut"] == completed["cut"] and names == sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*"))
            same("compatible peer remains unchanged", persisted(legacy), persisted(cli(new, extensions, "new-peer-after-downgrade", "run", "status", legacy_id)))
            repeated = cli(new, extensions, "new-" + extension + "-repeat-drive", "run", "drive", extension_id)
            same("new terminal drive is inert", persisted(completed), persisted(repeated))
            assert repeated["cut"] == completed["cut"]
            workers_completed(repeated, 2 if extension == "repeat" else 1)
            population = cli(new, extensions, "new-" + extension + "-population", "telemetry", "catalog")["population"]
            assert population["matched"] == 2 and population["attempts"] == (2 if extension == "repeat" else 1)
            report[extension] = {"run_id": extension_id, "compatible_peer_run_id": legacy_id, "before_cut": before["cut"], "final_cut": completed["cut"], "population": population}
            if extension == "call":
                assert population["invocations"] == 3
                report[extension].update({"child_invocation_id": child_id, "created_invocations": 1, "call_returns": 1,
                    "project_default_policy_ref": refs["core:policy/local"], "call_policy_ref": call_refs["core:policy/local"]})
            elif extension == "repeat":
                assert population["invocations"] == 5
                report[extension].update({"body_invocation_ids": [body["id"] for body in bodies], "created_invocations": 2, "decisions": 2,
                    "compatible_peer_state_version": "core-state/2", "project_default_policy_ref": refs["core:policy/local"]})
            else:
                report[extension]["decisions"] = 1
            report["outcome"] = "passed"
        except Exception as error:
            report["failure"] = {"type": type(error).__name__, "message": str(error)[:4000]}
        finally:
            if repeat_driver is not None and not repeat_driver_recorded:
                if repeat_driver.poll() is None:
                    repeat_driver.kill()  # Only the process returned by this Popen.
                repeat_driver.wait(timeout=10)
                for stream in repeat_streams:
                    stream.seek(0)
                stdout, stderr = (stream.read() for stream in repeat_streams)
                record_cli("new-drive-repeat-cleanup", repeat_driver.args, subprocess.CompletedProcess(repeat_driver.args, repeat_driver.returncode, stdout, stderr))
            if repeat_streams is not None:
                for stream in repeat_streams:
                    stream.close()
            for name, binary in (("old", old), ("new", new)):
                identities[name]["unchanged_after_verification"] = binary.exists() and digest(binary.read_bytes()) == identities[name]["sha256"]
                if not identities[name]["unchanged_after_verification"]:
                    report["outcome"] = "failed"
                    report["failure"] = {"type": "BinaryChanged", "message": name + " executable changed during verification"}
            evidence_file.write(encoded(report))
        print(json.dumps({"outcome": report["outcome"], "evidence": str(evidence), "temporary_directory": str(temporary), "commands": len(report["commands"]), "comparisons": len(report["comparisons"]), "failure": report.get("failure")}, indent=2))
        return 0 if report["outcome"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
