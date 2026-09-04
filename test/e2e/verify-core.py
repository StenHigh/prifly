#!/usr/bin/env python3
"""Create and exercise an independent Core workflow project with the real CLI.

No dependencies, AI, credentials or external services are needed. One shell
worker deliberately exits with code 9; other workers produce or consume JSON.
The project, exported artifacts and raw CLI responses remain inspectable.
This checks representative Core semantics, not full F2.
"""

import argparse
import copy
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import time


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False) + "\n").encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", type=Path, help="absent or empty directory; otherwise a new temporary project")
    parser.add_argument("--evidence", type=Path, help="new evidence file; defaults to PROJECT/verification/summary.json")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    target = (args.target or Path(tempfile.mkdtemp(prefix="prifly-core-", dir="/tmp"))).resolve()
    if target.exists() and (not target.is_dir() or any(target.iterdir())):
        parser.error("target must be absent or empty; no existing project files were changed")
    evidence = args.evidence.absolute() if args.evidence else target / "verification/summary.json"
    if evidence.exists() or evidence.is_symlink():
        parser.error("evidence already exists; previous results are never overwritten")
    results = target / "verification"
    transcript = results / "transcript"
    transcript.mkdir(parents=True)
    evidence.parent.mkdir(parents=True, exist_ok=True)
    report = {
        "schema_version": "core-example-evidence/1", "outcome": "failed", "project": str(target),
        "binary": {"path": str(binary), "sha256": digest(binary.read_bytes())},
        "script_sha256": digest(Path(__file__).read_bytes()),
        "host": {"platform": platform.platform(), "machine": platform.machine(), "python": platform.python_version()},
        "commands": [], "cases": [],
        "limitations": [
            "Local cooperative processes only; no sandbox or external-effect guarantee.",
            "Parallel and map use control-only single-child cases; concurrency, cancellation and recovery are qualified elsewhere.",
            "Wait proves durable registration and timeout on a later Drive; event delivery, autonomous wakeup and source authentication are outside this example.",
            "Call cases cover local aliases, scoped inputs/exports and shared counters; they do not qualify scoped-stop or unknown-effect recovery.",
            "Repeat cases cover native bodies, exact exports, until/limit/outcome/unknown routing and durable decisions; process-crash and concurrency are checked separately.",
            "This example does not replace crash recovery, concurrent control or release qualification tests.",
        ],
    }

    def write(name, value):
        path = target / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(encoded(value))
        return path

    def cli(label, *arguments, rejection=None):
        argv = [str(binary), "--project", str(target), "--json", *map(str, arguments)]
        result = subprocess.run(argv, capture_output=True, timeout=60)
        entry = {"label": label, "argv": argv, "exit_code": result.returncode}
        for stream in ("stdout", "stderr"):
            data = getattr(result, stream)
            name = f"{len(report['commands']):03d}-{label}.{stream}"
            (transcript / name).write_bytes(data)
            entry[stream] = {"file": name, "sha256": digest(data), "size_bytes": len(data)}
        report["commands"].append(entry)
        if rejection:
            assert result.returncode > 0 and not result.stdout, (label, "expected clean refusal")
            problem = json.loads(result.stderr)
            assert problem["code"] == rejection, (label, problem)
            entry["expected_rejection"] = problem
            return problem
        if result.returncode:
            raise RuntimeError(f"{label} failed ({result.returncode}); inspect {transcript / entry['stderr']['file']}")
        return json.loads(result.stdout)

    entries, aliases = [], {}

    def registry():
        value = {"schema_version": "2" if aliases else "1", "entries": entries}
        if aliases:
            value["aliases"] = aliases
        write("definitions.json", value)

    def define(kind, name, identifier, value):
        write(name, value)
        ref = cli("ref-" + Path(name).stem, "ref", name, "--id", identifier, "--version", "1.0.0")
        assert set(ref) == {"id", "version", "digest"}, ref
        entries.append({"ref": ref, "kind": kind, "path": name})
        registry()
        return ref

    def export(name, ref):
        write(f"verification/{name}-ref.json", ref)
        metadata = cli("inspect-" + name, "artifact", "inspect", "--ref", f"verification/{name}-ref.json")
        assert metadata["ref"] == ref
        cli("export-" + name, "artifact", "export", "--ref", f"verification/{name}-ref.json", "--output", f"verification/{name}.json")
        data = (results / (name + ".json")).read_bytes()
        assert "sha256:" + digest(data) == ref["digest"], name
        return metadata["artifact"], data

    with evidence.open("xb") as evidence_file:
        try:
            report["binary"]["version"] = cli("version", "version")
            cli("init-core", "init", "--profile", "core-workflow/1", target)
            inventory = cli("inventory", "inventory")["definitions"]
            builtins = {(entry["ref"]["id"], entry["ref"]["version"]): entry["ref"] for entry in inventory}
            config = json.loads((target / "prifly.json").read_bytes())
            assert config["configuration"]["semantics_profile"] == "core-workflow/1"
            assert config["configuration_schema_ref"] == builtins["core:schema/core-configuration", "1.0.0"]
            write("brief.json", {
                "schema_version": "1", "id": "demo:brief/core", "subject": "Deterministic Core examples",
                "desired_outcome": "Verify declared values, artifacts and error transitions",
                "in_scope": ["Local scratch files"], "out_of_scope": ["Network, AI, external writes"],
                "completion_criteria": ["Declared checks pass"], "source_refs": [], "assumptions": [], "confirmation": "explicit",
            })
            string_schema = {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "string", "maxLength": 128}
            string_ref = define("schema", "schemas/string.json", "demo:schema/core-string", string_schema)
            nullable_ref = define("schema", "schemas/nullable.json", "demo:schema/core-nullable", {**string_schema, "type": ["string", "null"]})
            settings_ref = define("schema", "schemas/settings.json", "demo:schema/core-settings", {
                "$schema": string_schema["$schema"], "type": "object",
                "properties": {"message": {"type": "string", "maxLength": 128}, "nullable": {"type": ["string", "null"], "maxLength": 128}, "optional": {"type": "string", "maxLength": 128}},
                "required": ["message", "nullable"], "additionalProperties": False,
            })

            def workflow(identifier, inputs, outputs, stages, entry="done", outcomes=None):
                return {"schema_version": "2", "id": identifier, "version": "1.0.0", "title": identifier,
                        "inputs": inputs, "outputs": outputs, "allowed_outcomes": outcomes or ["succeeded"],
                        "definition": {"entry": entry, "stages": stages},
                        "limits": {"max_step_instances": 1, "max_control_transitions": 4, "max_parallelism": 1, "max_child_depth": 0},
                        "policy_ref": config["default_policy_ref"]}

            def finish(outcome="succeeded", bindings=None):
                return {"kind": "finish", "outcome": outcome, "output_bindings": bindings or {}}

            defaults = {"message": "package", "nullable": None}
            output_schemas = {"message": string_ref, "nullable": nullable_ref, "optional": string_ref}
            values_workflow = workflow("demo:workflow/core-values", {
                "settings": {"format": "json", "schema_ref": settings_ref, "required": True, "configuration": {"scope": "run", "default": defaults}},
            }, {name: {"format": "json", "schema_ref": ref, "required_for": [] if name == "optional" else ["succeeded"]} for name, ref in output_schemas.items()}, {
                "done": finish(bindings={name: {"from": "workflow_input", "port": "settings", "pointer": "/" + name, "projected_schema_ref": ref} for name, ref in output_schemas.items()}),
            })
            values_ref = define("workflow", "workflows/values.json", values_workflow["id"], values_workflow)
            saved_runs = []
            for name, source, value in (
                ("default", "package_default", defaults),
                ("project", "project", {"message": "project", "nullable": "project value", "optional": "present"}),
                ("run", "run", {"message": "run", "nullable": None}),
            ):
                if name == "project":
                    config["configuration"]["input_values"] = {values_workflow["id"]: {"settings": value}}
                    write("prifly.json", config)
                inputs = []
                if name == "run":
                    write("inputs/settings.json", value)
                    inputs = ["--input", "settings=inputs/settings.json"]
                preview = cli("preview-" + name, "validate", "--workflow", "workflows/values.json")
                assert preview["admission"] is False
                view = cli("start-" + name, "run", "start", "--workflow", "workflows/values.json", "--brief", "brief.json", "--command-id", "command:core-" + name, "--drive", *inputs)
                run = view["run"]
                assert run["semantics_profile"] == "core-workflow/1"
                assert run["status"] == "completed" and run["outcome"] == "succeeded"
                assert not run["attempts"] and not run["steps"], "JSON projection must not create a worker"
                effective = run["effective_configuration"]
                assert effective["schema_version"] == "effective-configuration/1" and effective["workflow_ref"] == values_ref
                assert encoded(effective["inputs"]["settings"]) == encoded({"source": source, "value": value})
                assert set(run["output_artifacts"]) == set(value), (name, "absent optional output became present")
                source_ref = run["input_artifacts"]["settings"]
                for port, ref in run["output_artifacts"].items():
                    artifact, data = export(name + "-" + port, ref)
                    assert encoded(json.loads(data)) == encoded(value[port])
                    if value[port] is None:
                        assert data == b"null", "explicit null must be a sealed value, not absence"
                    assert artifact["format"] == "json" and artifact["schema_ref"] == output_schemas[port]
                    provenance = artifact["provenance"]
                    assert len(provenance) == 2 and provenance[0] == source_ref and ref != source_ref
                    manifest, manifest_data = export(name + "-" + port + "-manifest", provenance[1])
                    assert manifest["schema_ref"] == builtins["core:schema/json-projection", "1.0.0"]
                    assert manifest["provenance"] == [source_ref]
                    assert encoded(json.loads(manifest_data)) == encoded({
                        "schema_version": "json-projection/1", "source_ref": source_ref, "pointer": "/" + port,
                        "projected_schema_ref": output_schemas[port], "workflow_ref": values_ref,
                    })
                saved_runs.append(run)
                report["cases"].append({"case": name, "run_id": run["id"], "source": source, "output_ports": sorted(value), "attempts": 0, "projection_provenance_verified": True})

            # Current configuration can change; earlier Runs retain their values.
            for run in saved_runs:
                reopened = cli("reopen-" + run["effective_configuration"]["inputs"]["settings"]["source"], "run", "status", run["id"])["run"]
                assert encoded(reopened) == encoded(run), "configuration changes rewrote a saved Run"

            scoped = copy.deepcopy(values_workflow)
            scoped["id"] = "demo:workflow/core-project-scope"
            scoped["inputs"]["settings"]["configuration"]["scope"] = "project"
            define("workflow", "workflows/project-scope.json", scoped["id"], scoped)
            before = cli("before-scope-refusal", "telemetry", "catalog")
            cli("scope-refusal", "run", "start", "--workflow", "workflows/project-scope.json", "--brief", "brief.json", "--input", "settings=inputs/settings.json", "--command-id", "command:core-scope-refused", "--drive", rejection="configuration_scope")
            after = cli("after-scope-refusal", "telemetry", "catalog")
            assert before["cut"] == after["cut"] and before["population"] == after["population"]
            report["cases"].append({"case": "project-scope", "expected_rejection": "configuration_scope", "no_admission": True})

            # A valid candidate is insufficient when the owned process fails.
            # Reuse the normal shell protocol example and change only its exit.
            worker = (Path(__file__).resolve().parents[2] / "test/fixtures/foundation/empty-step.sh").read_bytes() + b"\nexit 9\n"
            (target / "scripts").mkdir()
            (target / "scripts/failed-step.sh").write_bytes(worker)
            report["worker_sha256"] = digest(worker)
            step_id = "demo:step/core-failure"
            step_ref = define("step", "steps/failure.json", step_id, {
                "schema_version": "1", "id": step_id, "version": "1.0.0", "title": "Known process failure", "kind": "command", "inputs": {}, "outputs": {},
                "executor": {"adapter_ref": builtins["core:adapter/local-process", "1.0.0"], "operation": "process"}, "context_refs": [], "required_capabilities": [],
                "effects": {"class": "none", "retry_class": "pure"}, "result_check_refs": [], "result_schema_ref": builtins["core:schema/step-result", "1.0.0"],
            })
            config["configuration"]["executors"][step_id] = {
                "executable": "/bin/sh", "args": ["failed-step.sh"], "files": {"failed-step.sh": "scripts/failed-step.sh"}, "environment": {},
                "timeout_ms": 10000, "grace_ms": 100, "max_output_bytes": 1048576,
            }
            write("prifly.json", config)
            failed_workflow = workflow("demo:workflow/core-error", {}, {}, {
                "work": {"kind": "step", "step_ref": step_ref, "input_bindings": {}, "on": {"pass": "done"}, "on_error": "rejected"},
                "done": finish(), "rejected": finish("rejected"),
            }, entry="work", outcomes=["succeeded", "rejected"])
            define("workflow", "workflows/error.json", failed_workflow["id"], failed_workflow)
            run = cli("start-known-failure", "run", "start", "--workflow", "workflows/error.json", "--brief", "brief.json", "--command-id", "command:core-error", "--drive")["run"]
            assert run["status"] == "completed" and run["outcome"] == "rejected"
            assert len(run["attempts"]) == len(run["steps"]) == 1
            attempt = next(iter(run["attempts"].values()))
            assert attempt["status"] == "failed" and not attempt.get("accepted")
            assert next(iter(run["steps"].values()))["status"] == "failed"
            process = attempt["process_outcome"]
            assert process["started"] and process["wait_returned"] and process["group_empty"] and not process["uncertain"]
            assert process["exit_code"] == 9
            history = cli("known-failure-events", "run", "events", run["id"], "--limit", "1000")["view"]
            assert not history["more"]
            handled = [event for event in history["events"] if event["type"] == "stage.error_handled"]
            assert len(handled) == 1
            assert handled[0]["data"]["failure"] == "nonzero_exit" and handled[0]["data"]["attempt_id"] == attempt["id"]
            assert handled[0]["data"]["next_stage_id"] == "rejected"
            report["cases"].append({"case": "known-failure", "run_id": run["id"], "outcome": "rejected", "attempts": 1, "exit_code": 9, "error_transition_verified": True})

            # The source is valid, but its projected field violates the output
            # enum. A finish failure must settle durably without inventing a Step
            # or Attempt, and a later drive must not retry the failed projection.
            restricted_ref = define("schema", "schemas/restricted-string.json", "demo:schema/core-restricted-string", {**string_schema, "enum": ["allowed"]})
            invalid_projection = workflow("demo:workflow/core-invalid-projection", {
                "settings": {"format": "json", "schema_ref": settings_ref, "required": True, "configuration": {"scope": "run", "default": {"message": "outside-enum", "nullable": None}}},
            }, {"message": {"format": "json", "schema_ref": restricted_ref, "required_for": ["succeeded"]}}, {
                "done": finish(bindings={"message": {"from": "workflow_input", "port": "settings", "pointer": "/message", "projected_schema_ref": restricted_ref}}),
            })
            define("workflow", "workflows/invalid-projection.json", invalid_projection["id"], invalid_projection)
            assert cli("preview-invalid-projection", "validate", "--workflow", "workflows/invalid-projection.json")["admission"] is False
            admitted = cli("start-invalid-projection", "run", "start", "--workflow", "workflows/invalid-projection.json", "--brief", "brief.json", "--command-id", "command:core-invalid-projection")
            failed = cli("drive-invalid-projection", "run", "drive", admitted["receipt"]["run_id"])
            run = failed["run"]
            assert run["status"] == "failed" and run["outcome"] is None and not run["ready_stages"]
            assert not run["steps"] and not run["attempts"] and not run["output_artifacts"]
            assert len(run["activations"]) == 1
            activation = next(iter(run["activations"].values()))
            assert activation["kind"] == "finish" and activation["status"] == "failed" and activation.get("settled")
            # The diagnostic names the refusal preparation found, not the phase:
            # a projection that fails its schema says so.
            diagnostics = [d for d in run["diagnostics"] if d["code"] == "projection_schema_invalid"]
            assert len(diagnostics) == 1
            diagnostic = diagnostics[0]
            assert diagnostic["severity"] == "error" and diagnostic["category"] == "workflow" and diagnostic["phase"] == "preparation"
            assert diagnostic["stage_activation_id"] == activation["id"] and not diagnostic.get("attempt_id")
            history = cli("invalid-projection-events", "run", "events", run["id"], "--limit", "1000")["view"]
            assert not history["more"]
            failures = [e for e in history["events"] if e["type"] == "stage.failed"]
            assert len(failures) == 1 and failures[0]["data"]["execution"] == "not_admitted"
            assert failures[0]["data"]["stage_activation_id"] == activation["id"]
            assert any(e["type"] == "diagnostic.recorded" and e["data"]["diagnostic_id"] == diagnostic["id"] for e in history["events"])
            assert not any(e["type"].startswith("attempt.") for e in history["events"])
            artifact_records = sorted(p.name for p in (target / ".prifly/artifact-refs").iterdir())
            for operation in ("status", "drive"):
                repeated = cli("invalid-projection-repeat-" + operation, "run", operation, run["id"])
                assert encoded(repeated["run"]) == encoded(run)
                assert all(repeated[key] == failed[key] for key in ("run_version", "event_sequence", "cut"))
            repeated_history = cli("invalid-projection-events-after-reopen", "run", "events", run["id"], "--limit", "1000")["view"]
            assert not repeated_history["more"] and encoded(repeated_history["events"]) == encoded(history["events"])
            assert artifact_records == sorted(p.name for p in (target / ".prifly/artifact-refs").iterdir())
            report["cases"].append({"case": "invalid-finish-projection", "run_id": run["id"], "status": "failed", "diagnostic_code": "projection_schema_invalid", "steps": 0, "attempts": 0, "repeat_drive_inert": True})

            # Choices are core control stages. A selected producer runs as an
            # ordinary process; an unselected branch has no Step or Attempt.
            wrapper = (Path(__file__).resolve().parents[2] / "test/fixtures/foundation/prifly_step.py").read_bytes()
            (target / "scripts/prifly_step.py").write_bytes(wrapper)
            choice_worker = b'''import json
from pathlib import Path
import sys
from prifly_step import Step

step = Step()
with Path("choice-worker-starts").open("a") as marker:
    marker.write("start\\n")
if sys.argv[1] == "produce":
    step.output_json("report", {"message": "produced", "nullable": None})
else:
    data = step.input_bytes("report")
    if data is not None:
        assert json.loads(data) == {"message": "produced", "nullable": None}
    Path("consumer-input-state").write_text("absent" if data is None else "present")
step.complete("pass", "Declared JSON output or optional input verified.")
'''
            (target / "scripts/choice-step.py").write_bytes(choice_worker)
            report["choice_worker_sha256"] = digest(choice_worker)
            report["wrapper_sha256"] = digest(wrapper)

            def choice_step(name, required=None):
                definition = json.loads((target / "steps/failure.json").read_bytes())
                definition["id"], definition["title"] = "demo:step/core-" + name, name
                definition["effects"] = {"class": "workspace_write", "retry_class": "pure"}
                if required is None:
                    definition["outputs"] = {"report": {"format": "json", "schema_ref": settings_ref, "required_for": ["pass"]}}
                else:
                    definition["inputs"] = {"report": {"format": "json", "schema_ref": settings_ref, "required": required}}
                ref = define("step", "steps/" + name + ".json", definition["id"], definition)
                config["configuration"]["executors"][definition["id"]] = {
                    "executable": str(Path(sys.executable).resolve()), "args": ["-B", "choice-step.py", "produce" if required is None else "consume"],
                    "files": {"choice-step.py": "scripts/choice-step.py", "prifly_step.py": "scripts/prifly_step.py"}, "environment": {},
                    "timeout_ms": 10000, "grace_ms": 100, "max_output_bytes": 1048576,
                }
                write("prifly.json", config)
                return ref

            producer_ref = choice_step("choice-producer")

            def field(pointer, stage=None):
                ref = {"from": "stage_output" if stage else "workflow_input", "port": "report" if stage else "settings", "pointer": pointer}
                if stage:
                    ref["stage_id"] = stage
                return {"kind": "field", "ref": ref}

            def equals(pointer, value, stage=None, op="eq"):
                return {"op": op, "left": field(pointer, stage), "right": {"kind": "literal", "value": value}}

            def branch(identifier, predicate, next_stage="done"):
                return {"id": identifier, "predicate": predicate, "next": next_stage}

            def control_inputs(value=defaults):
                return {"settings": {"format": "json", "schema_ref": settings_ref, "required": True, "configuration": {"scope": "run", "default": value}}}

            def decisions(label, run):
                history = cli(label, "run", "events", run["id"], "--limit", "1000")["view"]
                assert not history["more"]
                values = [event["data"] for event in history["events"] if event["type"] == "stage.choice_decided"]
                for value in values:
                    assert value["schema_version"] == "choice-decision/1" and value["run_id"] == run["id"]
                    assert value["workflow_ref"] == run["workflow_ref"] and value["workflow_invocation_id"] == run["root_workflow_invocation_id"]
                    assert value["observation"]["utc"]
                    activation = run["activations"][value["stage_activation_id"]]
                    assert activation["stage_id"] == value["stage_id"] and activation["kind"] == "choice" and not activation.get("step_instance_id")
                return values

            selected_workflow = workflow("demo:workflow/core-choice-worker", control_inputs(), {}, {
                "pick": {"kind": "choice", "selection": "exclusive", "branches": [branch("package", equals("/message", "package"), "producer"), branch("other", equals("/message", "package", op="ne"), "unselected")]},
                "producer": {"kind": "step", "step_ref": producer_ref, "input_bindings": {}, "on": {"pass": "inspect"}},
                "unselected": {"kind": "step", "step_ref": producer_ref, "input_bindings": {}, "on": {"pass": "rejected"}},
                "inspect": {"kind": "choice", "selection": "exclusive", "branches": [branch("explicit-null", equals("/nullable", None, "producer"))], "default": "rejected"},
                "done": finish(), "rejected": finish("rejected"),
            }, entry="pick", outcomes=["succeeded", "rejected"])
            selected_workflow["limits"]["max_control_transitions"] = 8
            define("workflow", "workflows/choice-worker.json", selected_workflow["id"], selected_workflow)
            write("inputs/choice.json", defaults)
            admitted = cli("start-choice-worker", "run", "start", "--workflow", "workflows/choice-worker.json", "--brief", "brief.json", "--input", "settings=inputs/choice.json", "--command-id", "command:core-choice-worker")
            # Original caller files are not live routing inputs after Start.
            write("inputs/choice.json", {"message": "changed-after-admission", "nullable": None})
            selected = cli("drive-choice-worker", "run", "drive", admitted["receipt"]["run_id"])
            run = selected["run"]
            assert run["status"] == "completed" and run["outcome"] == "succeeded"
            assert len(run["steps"]) == len(run["attempts"]) == 1
            assert not any(a["stage_id"] == "unselected" for a in run["activations"].values())
            attempt = next(iter(run["attempts"].values()))
            process = attempt["process_outcome"]
            assert attempt["accepted"] and process["started"] and process["wait_returned"] and process["group_empty"] and process["exit_code"] == 0
            marker = Path(attempt["workspace"]) / "choice-worker-starts"
            assert marker.read_text() == "start\n"
            values = decisions("choice-worker-events", run)
            assert len(values) == 2 and [v["stage_id"] for v in values] == ["pick", "inspect"]
            assert values[0]["branch_id"] == "package" and values[0]["next_stage_id"] == "producer"
            assert values[0]["evaluations"] == [{"branch_id": "package", "result": "true"}, {"branch_id": "other", "result": "false"}]
            assert len(values[0]["inputs"]) == 1 and values[0]["inputs"][0]["source_ref"] == run["input_artifacts"]["settings"]
            producer = next(a for a in run["activations"].values() if a["stage_id"] == "producer")
            accepted_ref = next(iter(run["steps"].values()))["outputs"]["report"]
            assert values[1]["branch_id"] == "explicit-null" and values[1]["inputs"] == [{
                "field_ref": field("/nullable", "producer")["ref"], "source_ref": accepted_ref,
                "producer_activation_id": producer["id"], "availability": "present",
            }]
            artifact, data = export("choice-producer-report", accepted_ref)
            assert json.loads(data) == {"message": "produced", "nullable": None} and artifact["schema_ref"] == settings_ref
            for operation in ("status", "drive"):
                repeated = cli("choice-worker-repeat-" + operation, "run", operation, run["id"])
                assert encoded(repeated["run"]) == encoded(run) and all(repeated[key] == selected[key] for key in ("run_version", "event_sequence", "cut"))
            assert decisions("choice-worker-events-reopened", run) == values and marker.read_text() == "start\n"
            report["cases"].append({"case": "choice-selected-worker", "run_id": run["id"], "decisions": 2, "attempts": 1, "sealed_input_used": True, "accepted_producer_ref_verified": True, "repeat_drive_inert": True})

            # first_match cannot skip an unknown before a later true branch,
            # and does not evaluate branches after the first true result.
            for name, branches, route, outcome, trace, pointer, availability in (
                ("unknown-before-true", [branch("optional", equals("/optional", "yes")), branch("known", equals("/message", "package"))], "on_unknown", "rejected", ["unknown", "not_evaluated"], "/optional", "absent"),
                ("unknown-after-true", [branch("known", equals("/message", "package")), branch("optional", equals("/optional", "yes"))], "branch", "succeeded", ["true", "not_evaluated"], "/message", "present"),
            ):
                name = "choice-" + name
                document = workflow("demo:workflow/" + name, control_inputs(), {}, {
                    "pick": {"kind": "choice", "selection": "first_match", "branches": branches, "on_unknown": "rejected", "default": "no_work"},
                    "done": finish(), "rejected": finish("rejected"), "no_work": finish("no_work"),
                }, entry="pick", outcomes=["succeeded", "rejected", "no_work"])
                define("workflow", "workflows/" + name + ".json", document["id"], document)
                run = cli("start-" + name, "run", "start", "--workflow", "workflows/" + name + ".json", "--brief", "brief.json", "--command-id", "command:" + name, "--drive")["run"]
                assert run["status"] == "completed" and run["outcome"] == outcome and not run["steps"] and not run["attempts"]
                values = decisions(name + "-events", run)
                assert len(values) == 1 and values[0]["route"] == route
                assert values[0]["evaluations"] == [{"branch_id": b["id"], "result": r} for b, r in zip(branches, trace)]
                assert values[0]["inputs"] == [{"field_ref": field(pointer)["ref"], "source_ref": run["input_artifacts"]["settings"], "availability": availability}]
                report["cases"].append({"case": name, "run_id": run["id"], "route": route, "trace": trace, "attempts": 0})

            ambiguous = workflow("demo:workflow/choice-ambiguous", control_inputs(), {}, {
                "pick": {"kind": "choice", "selection": "exclusive", "branches": [branch("one", equals("/message", "package")), branch("two", equals("/message", "package"))], "on_error": "rejected"},
                "done": finish(), "rejected": finish("rejected"),
            }, entry="pick", outcomes=["succeeded", "rejected"])
            define("workflow", "workflows/choice-ambiguous.json", ambiguous["id"], ambiguous)
            run = cli("start-choice-ambiguous", "run", "start", "--workflow", "workflows/choice-ambiguous.json", "--brief", "brief.json", "--command-id", "command:choice-ambiguous", "--drive")["run"]
            assert run["status"] == "completed" and run["outcome"] == "rejected" and not run["steps"] and not run["attempts"]
            values = decisions("choice-ambiguous-events", run)
            assert len(values) == 1 and values[0]["route"] == "on_error" and values[0]["failure"] == "ambiguous_branch"
            assert [v["result"] for v in values[0]["evaluations"]] == ["true", "true"]
            assert run["activations"][values[0]["stage_activation_id"]]["status"] == "failed"
            diagnostic = next(d for d in run["diagnostics"] if d["code"] == "ambiguous_branch")
            assert diagnostic["stage_activation_id"] == values[0]["stage_activation_id"] and not diagnostic.get("attempt_id")
            report["cases"].append({"case": "choice-ambiguous", "run_id": run["id"], "route": "on_error", "failure": "ambiguous_branch", "attempts": 0})

            # A common required input must be guaranteed on every incoming
            # path, even when today's configuration chooses the producer path.
            required_consumer = choice_step("required-consumer", required=True)
            optional_consumer = choice_step("optional-consumer", required=False)
            common = workflow("demo:workflow/choice-required-consumer", control_inputs(), {}, {
                "pick": {"kind": "choice", "selection": "exclusive", "branches": [branch("produce", equals("/message", "package"), "producer")], "default": "consume"},
                "producer": {"kind": "step", "step_ref": producer_ref, "input_bindings": {}, "on": {"pass": "consume"}},
                "consume": {"kind": "step", "step_ref": required_consumer, "input_bindings": {"report": {"from": "stage_output", "stage_id": "producer", "port": "report"}}, "on": {"pass": "done"}},
                "done": finish(),
            }, entry="pick")
            common["limits"].update({"max_step_instances": 2, "max_control_transitions": 6})
            define("workflow", "workflows/choice-required-consumer.json", common["id"], common)
            write("inputs/choice-required.json", defaults)
            before = cli("before-choice-required-refusal", "telemetry", "catalog")
            cli("preview-choice-required-refusal", "validate", "--workflow", "workflows/choice-required-consumer.json", rejection="unavailable_output")
            cli("start-choice-required-refusal", "run", "start", "--workflow", "workflows/choice-required-consumer.json", "--brief", "brief.json", "--input", "settings=inputs/choice-required.json", "--command-id", "command:choice-required-refusal", "--drive", rejection="unavailable_output")
            after = cli("after-choice-required-refusal", "telemetry", "catalog")
            assert before["cut"] == after["cut"] and before["population"] == after["population"]
            common["id"] = "demo:workflow/choice-optional-consumer"
            common["definition"]["stages"]["consume"]["step_ref"] = optional_consumer
            common["inputs"] = control_inputs({"message": "skip-producer", "nullable": None})
            define("workflow", "workflows/choice-optional-consumer.json", common["id"], common)
            run = cli("start-choice-optional-consumer", "run", "start", "--workflow", "workflows/choice-optional-consumer.json", "--brief", "brief.json", "--command-id", "command:choice-optional-consumer", "--drive")["run"]
            assert run["status"] == "completed" and run["outcome"] == "succeeded" and len(run["steps"]) == len(run["attempts"]) == 1
            assert not any(a["stage_id"] == "producer" for a in run["activations"].values())
            attempt = next(iter(run["attempts"].values()))
            assert attempt["accepted"] and (Path(attempt["workspace"]) / "consumer-input-state").read_text() == "absent"
            values = decisions("choice-optional-consumer-events", run)
            assert len(values) == 1 and values[0]["route"] == "default" and values[0]["next_stage_id"] == "consume"
            report["cases"].append({"case": "choice-common-consumer", "required_rejection": "unavailable_output", "no_required_admission": True, "optional_run_id": run["id"], "optional_input": "absent", "attempts": 1})

            # Predicate AST is data, never shell, SQL or a template language.
            # These are literal refusal payloads; this script never executes them.
            sentinel = results / "predicate-side-effect"
            unsafe = {
                "shell": {"op": "shell", "command": ["/usr/bin/touch", str(sentinel)]},
                "sql": {"op": "sql", "query": "VACUUM INTO '" + str(sentinel) + "'"},
                "template": {"op": "template", "expression": "{{ __import__('pathlib').Path(" + repr(str(sentinel)) + ").touch() }}"},
            }
            before = cli("before-unsafe-predicates", "telemetry", "catalog")
            workspace = target / config["configuration"]["workspace_root"]
            workspace_names = sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*"))
            artifact_records = sorted(p.name for p in (target / ".prifly/artifact-refs").iterdir())
            for kind, predicate in unsafe.items():
                name = "workflows/refused-predicate-" + kind + ".json"
                write(name, workflow("demo:workflow/refused-predicate-" + kind, {}, {}, {
                    "pick": {"kind": "choice", "selection": "exclusive", "branches": [branch("unsafe", predicate, "producer")]},
                    "producer": {"kind": "step", "step_ref": producer_ref, "input_bindings": {}, "on": {"pass": "done"}}, "done": finish(),
                }, entry="pick"))
                cli("preview-refused-predicate-" + kind, "validate", "--workflow", name, rejection="schema_invalid")
                cli("start-refused-predicate-" + kind, "run", "start", "--workflow", name, "--brief", "brief.json", "--command-id", "command:unsafe-predicate-" + kind, "--drive", rejection="schema_invalid")
                assert not sentinel.exists()
            after = cli("after-unsafe-predicates", "telemetry", "catalog")
            assert before["cut"] == after["cut"] and before["population"] == after["population"]
            assert workspace_names == sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*"))
            assert artifact_records == sorted(p.name for p in (target / ".prifly/artifact-refs").iterdir())
            report["cases"].append({"case": "unsafe-predicates", "operators": sorted(unsafe), "expected_rejection": "schema_invalid", "no_admission_artifact_or_side_effect": True})

            # Each call has its own invocation, even when both executions use
            # the same exact definition and the same local stage IDs. Different
            # inputs make accidental cross-invocation output reuse observable.
            call_worker = b'''import json
from pathlib import Path
from prifly_step import Step

step = Step()
value = json.loads(step.input_bytes("settings"))
Path("call-scope.json").write_text(json.dumps({key: step.envelope[key] for key in (
    "run_id", "workflow_ref", "workflow_invocation_id", "stage_activation_id", "step_instance_id", "attempt_id"
)}))
with Path("call-worker-starts").open("a") as marker:
    marker.write(value["message"] + "\\n")
step.output_json("report", value)
step.complete("pass", "Scoped call input copied to the declared output.")
'''
            (target / "scripts/call-step.py").write_bytes(call_worker)
            report["call_worker_sha256"] = digest(call_worker)
            call_step = json.loads((target / "steps/choice-producer.json").read_bytes())
            call_step["id"], call_step["title"] = "demo:step/call-echo", "Call echo"
            call_step["inputs"] = {"settings": {"format": "json", "schema_ref": settings_ref, "required": True}}
            call_step_ref = define("step", "steps/call-echo.json", call_step["id"], call_step)
            config["configuration"]["executors"][call_step["id"]] = {
                "executable": str(Path(sys.executable).resolve()), "args": ["-B", "call-step.py"],
                "files": {"call-step.py": "scripts/call-step.py", "prifly_step.py": "scripts/prifly_step.py"}, "environment": {},
                "timeout_ms": 10000, "grace_ms": 100, "max_output_bytes": 1048576,
            }
            write("prifly.json", config)

            def call(ref, outcome="succeeded", next_stage="done", bindings=None):
                return {"kind": "call", "workflow_ref": ref, "input_bindings": bindings or {}, "on": {outcome: next_stage}}

            def call_output(outcome):
                return {"format": "json", "schema_ref": settings_ref, "required_for": [outcome]}

            def stage_report(stage="work"):
                return {"from": "stage_output", "stage_id": stage, "port": "report"}

            def call_history(label, view, outcome, invocation_count, steps, controls):
                run = view["run"]
                assert view["schema_version"] == "core-read/2" and run["schema_version"] == "core-state/2"
                assert run["status"] == "completed" and run["outcome"] == outcome
                assert "ready_stages" not in run and not run["active_attempt_ids"]
                assert len(run["invocations"]) == invocation_count and len(run["steps"]) == len(run["attempts"]) == steps
                root = run["invocations"][run["root_workflow_invocation_id"]]
                assert root["workflow_ref"] == run["workflow_ref"] and not root.get("parent_invocation_id") and not root.get("caller_stage_activation_id")
                assert root["input_refs"] == run["input_artifacts"] and root["output_refs"] == run["output_artifacts"]
                assert root["control_transitions"] == run["control_transitions"] == controls and root["step_instances"] == steps
                history = cli(label, "run", "events", run["id"], "--limit", "1000")["view"]
                assert not history["more"]
                events = history["events"]
                assert sum(e["type"] == "run.created" for e in events) == 1
                assert sum(e["type"] == "run.finished" for e in events) == 1
                assert sum(e["type"] == "invocation.created" for e in events) == invocation_count - 1
                assert sum(e["type"] == "invocation.finished" for e in events) == invocation_count - 1
                assert sum(e["type"] == "stage.call_returned" for e in events) == invocation_count - 1
                root_finish = next(i for i, e in enumerate(events) if e["type"] == "run.finished")
                assert events[root_finish]["data"]["workflow_invocation_id"] == root["id"]
                assert events[root_finish]["data"]["output_refs"] == root["output_refs"]
                for invocation in run["invocations"].values():
                    assert invocation["run_id"] == run["id"] and invocation["status"] == "completed" and invocation["outcome"] == outcome
                    assert not invocation["ready_stages"] and invocation["settled"]
                    if invocation == root:
                        continue
                    caller = run["activations"][invocation["caller_stage_activation_id"]]
                    assert caller["workflow_invocation_id"] == invocation["parent_invocation_id"] and caller["kind"] == "call"
                    assert caller["status"] == "completed" and caller["settled"] and not caller.get("step_instance_id")
                    created = next(e["data"] for e in events if e["type"] == "invocation.created" and e["data"]["workflow_invocation_id"] == invocation["id"])
                    assert created["workflow_ref"] == invocation["workflow_ref"] and created["input_refs"] == invocation["input_refs"]
                    finished = next(i for i, e in enumerate(events) if e["type"] == "invocation.finished" and e["data"]["workflow_invocation_id"] == invocation["id"])
                    returned = next(i for i, e in enumerate(events) if e["type"] == "stage.call_returned" and e["data"]["workflow_invocation_id"] == invocation["id"])
                    assert finished < returned < root_finish, "a child finish must not complete its parent"
                    settled = events[finished]["data"]
                    assert settled["workflow_ref"] == invocation["workflow_ref"] and settled["output_refs"] == invocation["output_refs"]
                    assert settled["status"] == "completed" and settled["outcome"] == outcome and settled["caller_stage_activation_id"] == caller["id"]
                    assert events[returned]["data"]["output_refs"] == invocation["output_refs"] and events[returned]["data"]["outcome"] == outcome

            child = workflow("demo:workflow/call-child", copy.deepcopy(call_step["inputs"]), {"report": call_output("succeeded")}, {
                "work": {"kind": "step", "step_ref": call_step_ref, "input_bindings": {"settings": {"from": "workflow_input", "port": "settings"}}, "on": {"pass": "done"}},
                "done": finish(bindings={"report": stage_report()}),
            }, entry="work")
            write("workflows/call-child.json", child)
            aliases["child"] = "workflows/call-child.json"
            registry()
            call_values = {name: {"message": name + "-call", "nullable": None} for name in ("first", "second")}
            repeated = workflow("demo:workflow/call-repeated", {
                name: {"format": "json", "schema_ref": settings_ref, "required": True, "configuration": {"scope": "run", "default": value}}
                for name, value in call_values.items()
            }, {name: call_output("succeeded") for name in call_values}, {
                "work": call({"alias": "child"}, next_stage="work_again", bindings={"settings": {"from": "workflow_input", "port": "first"}}),
                "work_again": call({"alias": "child"}, bindings={"settings": {"from": "workflow_input", "port": "second"}}),
                "done": finish(bindings={"first": stage_report(), "second": stage_report("work_again")}),
            }, entry="work")
            repeated["limits"].update({"max_step_instances": 2, "max_control_transitions": 9, "max_child_depth": 1})
            write("workflows/call-repeated.json", repeated)
            preview = cli("preview-call-repeated", "validate", "--workflow", "workflows/call-repeated.json")
            assert preview["schema_version"] == "core-preview/2" and preview["admission"] is False and len(preview["workflows"]) == 2
            child_ref = next(item["workflow_ref"] for item in preview["workflows"].values() if item["workflow_ref"]["id"] == child["id"])
            assert set(preview["workflows"]) == {child_ref["digest"], preview["workflow_ref"]["digest"]}
            admitted = cli("start-call-repeated", "run", "start", "--workflow", "workflows/call-repeated.json", "--brief", "brief.json", "--command-id", "command:call-repeated")
            # Alias source is mutable authoring input, not a live dispatch ref.
            changed_child = copy.deepcopy(child)
            changed_child["title"] = "Changed after the Run pinned its call target"
            write("workflows/call-child.json", changed_child)
            completed = cli("drive-call-repeated", "run", "drive", admitted["receipt"]["run_id"])
            run = completed["run"]
            call_history("call-repeated-events", completed, "succeeded", 3, 2, 9)
            assert run["workflow_ref"] == preview["workflow_ref"]
            # Public read views deliberately redact definitions and envelopes.
            # The worker records only its own nonsecret execution identity.
            assert run["workflow"] is None and run["definitions"] is None
            markers = []
            for name, caller_stage in (("first", "work"), ("second", "work_again")):
                caller = next(a for a in run["activations"].values() if a["workflow_invocation_id"] == run["root_workflow_invocation_id"] and a["stage_id"] == caller_stage)
                invocation = next(i for i in run["invocations"].values() if i.get("caller_stage_activation_id") == caller["id"])
                assert invocation["workflow_ref"] == child_ref and invocation["input_refs"] == {"settings": run["input_artifacts"][name]}
                assert invocation["control_transitions"] == 2 and invocation["step_instances"] == 1
                worker = next(a for a in run["activations"].values() if a["workflow_invocation_id"] == invocation["id"] and a["stage_id"] == "work")
                step = run["steps"][worker["step_instance_id"]]
                assert len(step["attempt_ids"]) == 1
                attempt = run["attempts"][step["attempt_ids"][0]]
                assert attempt["accepted"]["outputs"] == step["outputs"] == invocation["output_refs"]
                scope = json.loads((Path(attempt["workspace"]) / "call-scope.json").read_bytes())
                assert scope == {"run_id": run["id"], "workflow_ref": child_ref, "workflow_invocation_id": invocation["id"], "stage_activation_id": worker["id"], "step_instance_id": step["id"], "attempt_id": attempt["id"]}
                process = attempt["process_outcome"]
                assert process["started"] and process["wait_returned"] and process["group_empty"] and process["exit_code"] == 0
                ref = run["output_artifacts"][name]
                assert ref == invocation["output_refs"]["report"]
                artifact, data = export("call-repeated-" + name, ref)
                assert json.loads(data) == call_values[name] and artifact["schema_ref"] == settings_ref
                assert artifact["producer"] == {"kind": "step", "run_id": run["id"], "workflow_invocation_id": invocation["id"], "stage_activation_id": worker["id"], "step_instance_id": step["id"], "attempt_id": attempt["id"], "port": "report"}
                marker = Path(attempt["workspace"]) / "call-worker-starts"
                assert marker.read_text() == call_values[name]["message"] + "\n"
                markers.append(marker)
            assert markers[0].parent != markers[1].parent and run["output_artifacts"]["first"] != run["output_artifacts"]["second"]
            for operation in ("status", "drive"):
                again = cli("call-repeated-repeat-" + operation, "run", operation, run["id"])
                assert again["run"] == run and all(again[key] == completed[key] for key in ("run_version", "event_sequence", "cut"))
            assert [marker.read_text() for marker in markers] == [value["message"] + "\n" for value in call_values.values()]
            write("workflows/call-child.json", child)
            report["cases"].append({"case": "call-repeated-alias-worker", "run_id": run["id"], "invocations": 3, "attempts": 2, "control_transitions": 9, "distinct_scoped_outputs": True, "sealed_alias_used": True, "repeat_drive_inert": True})

            # Nested exact refs forward the leaf's public export unchanged.
            # Neither partial nor rejected is a technical process failure.
            for outcome, source in (("partial", "package_default"), ("rejected", "project")):
                value = {"message": "nested-" + outcome, "nullable": None}
                leaf = copy.deepcopy(child)
                leaf["id"] = "demo:workflow/call-leaf-" + outcome
                leaf["allowed_outcomes"] = [outcome]
                leaf["inputs"] = control_inputs(value)
                leaf["outputs"]["report"] = call_output(outcome)
                leaf["definition"]["stages"]["done"] = finish(outcome, {"report": stage_report()})
                leaf_ref = define("workflow", "workflows/call-leaf-" + outcome + ".json", leaf["id"], leaf)
                middle = workflow("demo:workflow/call-middle-" + outcome, {}, {"report": call_output(outcome)}, {
                    "work": call(leaf_ref, outcome), "done": finish(outcome, {"report": stage_report()}),
                }, entry="work", outcomes=[outcome])
                middle["limits"].update({"max_control_transitions": 5, "max_child_depth": 1})
                middle_ref = define("workflow", "workflows/call-middle-" + outcome + ".json", middle["id"], middle)
                parent = workflow("demo:workflow/call-nested-" + outcome, {}, {"report": call_output(outcome)}, {
                    "work": call(middle_ref, outcome), "done": finish(outcome, {"report": stage_report()}),
                }, entry="work", outcomes=[outcome])
                parent["limits"].update({"max_control_transitions": 8, "max_child_depth": 2})
                path = "workflows/call-nested-" + outcome + ".json"
                parent_ref = define("workflow", path, parent["id"], parent)
                if source == "project":
                    value = {"message": "nested-project-" + outcome, "nullable": None}
                    config["configuration"]["input_values"][leaf["id"]] = {"settings": value}
                    write("prifly.json", config)
                preview = cli("preview-call-nested-" + outcome, "validate", "--workflow", path)
                assert preview["schema_version"] == "core-preview/2" and preview["admission"] is False
                assert set(preview["workflows"]) == {ref["digest"] for ref in (parent_ref, middle_ref, leaf_ref)}
                admitted = cli("start-call-nested-" + outcome, "run", "start", "--workflow", path, "--brief", "brief.json", "--command-id", "command:call-nested-" + outcome)
                config["configuration"]["input_values"][leaf["id"]] = {"settings": {"message": "changed-after-start", "nullable": None}}
                write("prifly.json", config)
                completed = cli("drive-call-nested-" + outcome, "run", "drive", admitted["receipt"]["run_id"])
                run = completed["run"]
                call_history("call-nested-" + outcome + "-events", completed, outcome, 3, 1, 8)
                assert run["workflow_configurations"][leaf_ref["digest"]]["inputs"]["settings"] == {"source": source, "value": value}
                invocations = {i["workflow_ref"]["digest"]: i for i in run["invocations"].values()}
                leaf_inv, middle_inv = invocations[leaf_ref["digest"]], invocations[middle_ref["digest"]]
                assert leaf_inv["parent_invocation_id"] == middle_inv["id"] and middle_inv["parent_invocation_id"] == run["root_workflow_invocation_id"]
                assert leaf_inv["control_transitions"] == 2 and middle_inv["control_transitions"] == 5
                attempt = next(iter(run["attempts"].values()))
                assert attempt["accepted"]["outputs"] == leaf_inv["output_refs"] == middle_inv["output_refs"] == run["output_artifacts"]
                artifact, data = export("call-nested-" + outcome, run["output_artifacts"]["report"])
                assert json.loads(data) == value and artifact["producer"]["workflow_invocation_id"] == leaf_inv["id"]
                assert (Path(attempt["workspace"]) / "call-worker-starts").read_text() == value["message"] + "\n"
                report["cases"].append({"case": "call-nested-" + outcome, "run_id": run["id"], "outcome": outcome, "invocations": 3, "attempts": 1, "control_transitions": 8, "unchanged_leaf_export_ref": True, "pinned_configuration_source": source})

            # Resolve the whole alias graph before creating a Run or worker.
            # Both aliases have valid local contracts; only recursion is invalid.
            for name, other in (("cycle-a", "cycle-b"), ("cycle-b", "cycle-a")):
                document = copy.deepcopy(child)
                document["id"] = "demo:workflow/" + name
                document["definition"]["stages"]["work"] = call({"alias": other}, bindings={"settings": {"from": "workflow_input", "port": "settings"}})
                document["limits"].update({"max_control_transitions": 32, "max_child_depth": 4})
                name_path = "workflows/" + name + ".json"
                write(name_path, document)
                aliases[name] = name_path
            registry()
            cyclic = copy.deepcopy(repeated)
            cyclic["id"] = "demo:workflow/call-alias-cycle"
            cyclic["definition"]["stages"]["work"]["workflow_ref"] = {"alias": "cycle-a"}
            cyclic["limits"].update({"max_control_transitions": 64, "max_child_depth": 4})
            write("workflows/call-alias-cycle.json", cyclic)
            before = cli("before-call-alias-cycle", "telemetry", "catalog")
            workspace_names = sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*"))
            artifact_records = sorted(p.name for p in (target / ".prifly/artifact-refs").iterdir())
            cli("preview-call-alias-cycle", "validate", "--workflow", "workflows/call-alias-cycle.json", rejection="alias_cycle")
            cli("start-call-alias-cycle", "run", "start", "--workflow", "workflows/call-alias-cycle.json", "--brief", "brief.json", "--command-id", "command:call-alias-cycle", "--drive", rejection="alias_cycle")
            after = cli("after-call-alias-cycle", "telemetry", "catalog")
            assert before["cut"] == after["cut"] and before["population"] == after["population"]
            assert workspace_names == sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*"))
            assert artifact_records == sorted(p.name for p in (target / ".prifly/artifact-refs").iterdir())
            report["cases"].append({"case": "call-alias-cycle", "expected_rejection": "alias_cycle", "no_admission_artifact_or_workspace": True})
            del aliases["cycle-a"], aliases["cycle-b"]
            registry()

            # Repeat uses the same ordinary echo worker. Every body is a new
            # invocation; the next input is explicit, never an implicit carry.
            def repeat_stage(body_ref, maximum, until, initial=None, next_inputs=None, continue_on=None):
                return {"kind": "repeat", "body_workflow_ref": body_ref,
                        "initial_bindings": initial or {}, "next_bindings": next_inputs or {},
                        "continue_on": continue_on or ["succeeded"], "until": until, "max_iterations": maximum,
                        "on_complete": {"succeeded": "done"}, "on_limit": "done"}

            def repeat_history(label, view, routes, attempts):
                run = view["run"]
                assert view["schema_version"] == "core-read/3" and run["schema_version"] == "core-state/3"
                assert run["status"] == "completed" and run["outcome"] == "succeeded"
                assert "ready_stages" not in run and not run["active_attempt_ids"]
                root_id = run["root_workflow_invocation_id"]
                controller = next(a for a in run["activations"].values() if a["workflow_invocation_id"] == root_id and a["kind"] == "repeat")
                bodies = sorted((i for i in run["invocations"].values() if i.get("caller_stage_activation_id") == controller["id"]), key=lambda i: i["iteration"])
                assert len(bodies) == len(routes) and len(run["invocations"]) == len(bodies) + 1
                assert len(run["steps"]) == len(run["attempts"]) == attempts
                assert run["control_transitions"] == 2 * len(bodies) + attempts + 2
                assert run["invocations"][root_id]["control_transitions"] == run["control_transitions"]
                assert controller["repeat"]["iteration_count"] == len(bodies)
                assert controller["repeat"]["current_body_workflow_invocation_id"] == bodies[-1]["id"]
                history = cli(label, "run", "events", run["id"], "--limit", "1000")["view"]
                assert not history["more"]
                events = history["events"]
                decisions = [e["data"] for e in events if e["type"] == "stage.repeat_decided"]
                assert [d["route"] for d in decisions] == routes
                assert controller["repeat"]["last_decision"] == decisions[-1]
                assert sum(e["type"] == "stage.repeat_entered" for e in events) == 1
                assert sum(e["type"] == "invocation.created" for e in events) == len(bodies)
                assert sum(e["type"] == "invocation.finished" for e in events) == len(bodies)
                for index, (body, decision) in enumerate(zip(bodies, decisions), 1):
                    assert body["iteration"] == decision["iteration"] == index
                    assert body["parent_invocation_id"] == root_id and body["status"] == "completed" and body["outcome"] == "succeeded"
                    assert body["settled"] and not body["ready_stages"]
                    assert decision["body_workflow_invocation_id"] == body["id"] and decision["body_outcome"] == body["outcome"]
                    assert decision["workflow_invocation_id"] == root_id and decision["workflow_ref"] == run["workflow_ref"]
                    assert decision["stage_activation_id"] == controller["id"] and decision["schema_version"] == "repeat-decision/1"
                    if index < len(bodies):
                        assert decision["next_body_workflow_invocation_id"] == bodies[index]["id"]
                        assert decision["observation"] == bodies[index]["created"]
                    else:
                        assert "next_body_workflow_invocation_id" not in decision and decision["next_stage_id"] == "done"
                for operation in ("status", "drive"):
                    again = cli(label + "-" + operation, "run", operation, run["id"])
                    assert again["run"] == run and all(again[key] == view[key] for key in ("run_version", "event_sequence", "cut"))
                return run, controller, bodies, decisions

            repeat_values = [{"message": "first-repeat", "nullable": None}, {"message": "second-repeat", "nullable": None}]
            until_second = {"op": "eq", "left": {"kind": "field", "ref": {"from": "iteration_output", "port": "report", "pointer": "/message"}}, "right": {"kind": "literal", "value": "second-repeat"}}
            repeat_worker = workflow("demo:workflow/repeat-worker", control_inputs(repeat_values[0]), {"report": call_output("succeeded")}, {
                "work": repeat_stage({"alias": "child"}, 3, until_second,
                                     {"settings": {"from": "workflow_input", "port": "settings"}},
                                     {"settings": {"from": "literal", "value": repeat_values[1], "schema_ref": settings_ref}}),
                "done": finish(bindings={"report": stage_report()}),
            }, entry="work")
            repeat_worker["limits"].update({"max_step_instances": 3, "max_control_transitions": 11, "max_child_depth": 1})
            write("workflows/repeat-worker.json", repeat_worker)
            preview = cli("preview-repeat-worker", "validate", "--workflow", "workflows/repeat-worker.json")
            assert preview["schema_version"] == "core-preview/3" and len(preview["workflows"]) == 2 and not preview["admission"]
            admitted = cli("start-repeat-worker", "run", "start", "--workflow", "workflows/repeat-worker.json", "--brief", "brief.json", "--command-id", "command:repeat-worker")
            run_id = admitted["receipt"]["run_id"]
            next_view = cli("next-repeat-worker", "run", "next", run_id)
            assert next_view["schema_version"] == "core-next/3" and next_view["action"] == "stage" and next_view["stage_id"] == "work"
            write("workflows/call-child.json", changed_child)
            completed = cli("drive-repeat-worker", "run", "drive", run_id)
            run, controller, bodies, repeat_decisions = repeat_history("repeat-worker-events", completed, ["continue", "on_complete"], 2)
            assert [d["until_result"] for d in repeat_decisions] == ["false", "true"]
            assert run["output_artifacts"] == bodies[-1]["output_refs"]
            repeat_markers = []
            for body, value, decision in zip(bodies, repeat_values, repeat_decisions):
                assert body["workflow_ref"] == child_ref and body["control_transitions"] == 2 and body["step_instances"] == 1
                worker = next(a for a in run["activations"].values() if a["workflow_invocation_id"] == body["id"] and a["kind"] == "step")
                step = run["steps"][worker["step_instance_id"]]
                attempt = run["attempts"][step["attempt_ids"][0]]
                scope = json.loads((Path(attempt["workspace"]) / "call-scope.json").read_bytes())
                assert scope["workflow_invocation_id"] == body["id"] and scope["stage_activation_id"] == worker["id"]
                assert step["outputs"] == attempt["accepted"]["outputs"] == body["output_refs"]
                artifact, data = export("repeat-body-" + str(body["iteration"]), body["output_refs"]["report"])
                assert json.loads(data) == value and artifact["producer"]["workflow_invocation_id"] == body["id"]
                assert len(decision["inputs"]) == 1 and decision["inputs"][0]["source_ref"] == body["output_refs"]["report"]
                assert decision["inputs"][0]["availability"] == "present"
                marker = Path(attempt["workspace"]) / "call-worker-starts"
                assert marker.read_text() == value["message"] + "\n"
                repeat_markers.append(marker)
            assert repeat_markers[0].parent != repeat_markers[1].parent
            write("workflows/call-child.json", child)
            report["cases"].append({"case": "repeat-native-until", "run_id": run_id, "invocations": 3, "attempts": 2, "control_transitions": 8, "routes": ["continue", "on_complete"], "exact_current_body_refs": True, "explicit_next_inputs": True, "sealed_body_alias": True, "repeat_drive_inert": True})

            # No process is needed to enforce a count, select a non-continuing
            # outcome, or record unknown before considering the iteration cap.
            control_body = workflow("demo:workflow/repeat-control-body", {}, {"optional": {"format": "json", "schema_ref": string_ref, "required_for": []}}, {"done": finish()}, outcomes=["succeeded", "rejected"])
            control_body_ref = define("workflow", "workflows/repeat-control-body.json", control_body["id"], control_body)
            literal_false = {"op": "eq", "left": {"kind": "literal", "value": False}, "right": {"kind": "literal", "value": True}}
            unknown_until = {"op": "eq", "left": {"kind": "field", "ref": {"from": "iteration_output", "port": "optional", "pointer": ""}}, "right": {"kind": "literal", "value": "present"}}
            for name, maximum, outcomes, until, routes, truth in (
                ("limit", 3, ["succeeded"], literal_false, ["continue", "continue", "on_limit"], "false"),
                ("noncontinuing", 3, ["rejected"], unknown_until, ["on_complete"], "not_evaluated"),
                ("unknown-at-limit", 1, ["succeeded"], unknown_until, ["on_unknown"], "unknown"),
            ):
                stage = repeat_stage(control_body_ref, maximum, until, continue_on=outcomes)
                if name == "unknown-at-limit":
                    stage["on_unknown"] = "done"
                document = workflow("demo:workflow/repeat-" + name, {}, {}, {"work": stage, "done": finish()}, entry="work")
                document["limits"].update({"max_control_transitions": 2 * maximum + 2, "max_child_depth": 1})
                path = "workflows/repeat-" + name + ".json"
                write(path, document)
                completed = cli("start-repeat-" + name, "run", "start", "--workflow", path, "--brief", "brief.json", "--command-id", "command:repeat-" + name, "--drive")
                run, _, bodies, repeat_decisions = repeat_history("repeat-" + name + "-events", completed, routes, 0)
                assert all(d["until_result"] == truth for d in repeat_decisions)
                if name == "noncontinuing":
                    assert repeat_decisions[0]["inputs"] == [], "non-continuing outcome evaluated until"
                if name == "unknown-at-limit":
                    assert repeat_decisions[0]["inputs"][0]["availability"] == "absent"
                    assert "source_ref" not in repeat_decisions[0]["inputs"][0]
                report["cases"].append({"case": "repeat-" + name, "run_id": run["id"], "invocations": len(bodies) + 1, "attempts": 0, "control_transitions": run["control_transitions"], "routes": routes, "until_result": truth})

            unqualified = copy.deepcopy(document)
            unqualified["id"] = "demo:workflow/repeat-unqualified"
            unqualified["limits"].update({"max_control_transitions": 1024, "max_step_instances": 256})
            unqualified["definition"]["stages"]["work"]["max_iterations"] = 101
            write("workflows/repeat-unqualified.json", unqualified)
            before = cli("before-repeat-capacity", "telemetry", "catalog")
            cli("preview-repeat-capacity", "validate", "--workflow", "workflows/repeat-unqualified.json", rejection="resource_limit")
            cli("start-repeat-capacity", "run", "start", "--workflow", "workflows/repeat-unqualified.json", "--brief", "brief.json", "--command-id", "command:repeat-unqualified", "--drive", rejection="resource_limit")
            after = cli("after-repeat-capacity", "telemetry", "catalog")
            assert before["cut"] == after["cut"] and before["population"] == after["population"]
            report["cases"].append({"case": "repeat-capacity", "maximum_iterations": 101, "expected_rejection": "resource_limit", "no_admission": True})

            # These operators are supported now. Keep one small positive CLI
            # case for each so this example cannot drift back to claiming the
            # opposite of what the runtime actually does.
            join = {"mode": "all", "accept_outcomes": ["succeeded"], "selection": "all", "remainder": "wait"}

            parallel = workflow("demo:workflow/parallel-supported", {}, {}, {
                "fan": {"kind": "parallel", "branches": [{"id": "one", "workflow_ref": control_body_ref, "input_bindings": {}}],
                        "join": join, "max_parallelism": 1, "on": {"satisfied": "done", "unsatisfied": "done"}},
                "done": finish(),
            }, entry="fan")
            parallel["limits"].update({"max_control_transitions": 16, "max_child_depth": 1})
            write("workflows/parallel-supported.json", parallel)
            run = cli("start-parallel-supported", "run", "start", "--workflow", "workflows/parallel-supported.json",
                      "--brief", "brief.json", "--command-id", "command:parallel-supported", "--drive")["run"]
            fan = next(a for a in run["activations"].values() if a["kind"] == "parallel")
            assert run["status"] == "completed" and run["outcome"] == "succeeded" and not run["attempts"]
            assert fan["parallel"]["branch_ids"] == ["one"] and fan["parallel"]["entered_count"] == 1
            _, summary = export("parallel-summary", fan["parallel"]["results_ref"])
            summary = json.loads(summary)
            assert summary["join_result"] == "satisfied" and summary["selected_branch_ids"] == ["one"]
            assert [(branch["id"], branch["outcome"]) for branch in summary["branches"]] == [("one", "succeeded")]
            report["cases"].append({"case": "parallel-supported", "run_id": run["id"], "branches": 1,
                                    "attempts": 0, "aggregate_verified": True})

            item_schema = {"$schema": string_schema["$schema"], "type": "object", "required": ["id"],
                           "properties": {"id": {"type": "string", "minLength": 1}}, "additionalProperties": False}
            item_ref = define("schema", "schemas/item.json", "demo:schema/item", item_schema)
            array_ref = define("schema", "schemas/items.json", "demo:schema/items", {
                "$schema": string_schema["$schema"], "type": "array", "maxItems": 1, "items": item_schema})
            item_body = workflow("demo:workflow/map-item", {
                "item": {"format": "json", "schema_ref": item_ref, "required": True},
            }, {}, {"done": finish()})
            item_body_ref = define("workflow", "workflows/map-item.json", item_body["id"], item_body)
            mapped = workflow("demo:workflow/map-supported", {}, {}, {
                "over": {"kind": "map", "items": {"from": "literal", "value": [{"id": "one"}], "schema_ref": array_ref},
                         "body_workflow_ref": item_body_ref, "item_input": "item", "item_key_pointer": "/id",
                         "input_bindings": {}, "max_items": 1, "max_parallelism": 1, "join": join,
                         "on": {"satisfied": "done", "unsatisfied": "done", "empty": "done"}},
                "done": finish(),
            }, entry="over")
            mapped["limits"].update({"max_control_transitions": 16, "max_child_depth": 1})
            write("workflows/map-supported.json", mapped)
            run = cli("start-map-supported", "run", "start", "--workflow", "workflows/map-supported.json",
                      "--brief", "brief.json", "--command-id", "command:map-supported", "--drive")["run"]
            over = next(a for a in run["activations"].values() if a["kind"] == "map")
            assert run["status"] == "completed" and run["outcome"] == "succeeded" and not run["attempts"]
            assert over["parallel"]["branch_ids"] == ["string:one"] and over["parallel"]["sealed"][0]["key"] == "string:one"
            assert "results_ref" in over["parallel"]
            report["cases"].append({"case": "map-supported", "run_id": run["id"], "items": 1,
                                    "attempts": 0, "sealed_item_verified": True})

            source_ref = define("resource", "resources/manual-source.json", "demo:source/manual",
                                {"description": "Manual event source for the durable wait example"})
            waiting = workflow("demo:workflow/wait-supported", {}, {}, {
                "hold": {"kind": "wait", "source_ref": source_ref, "event_type": "manual", "event_schema_ref": string_ref,
                         "correlation_input": {"from": "literal", "value": "example", "schema_ref": string_ref},
                         "timeout_seconds": 1, "on_event": "done", "on_timeout": "done"},
                "done": finish(),
            }, entry="hold")
            waiting["limits"]["max_control_transitions"] = 8
            write("workflows/wait-supported.json", waiting)
            run = cli("start-wait-supported", "run", "start", "--workflow", "workflows/wait-supported.json",
                      "--brief", "brief.json", "--command-id", "command:wait-supported", "--drive")["run"]
            hold = next(a for a in run["activations"].values() if a["kind"] == "wait")
            registration = run["wait_registrations"][hold["wait"]["wait_registration_id"]]
            assert run["status"] == "waiting" and not run["attempts"] and not run["active_attempt_ids"]
            assert hold["status"] == "waiting" and registration["status"] == "active" and registration["expires_at"]
            time.sleep(1.1)
            run = cli("drive-wait-timeout", "run", "drive", run["id"])["run"]
            hold = next(a for a in run["activations"].values() if a["kind"] == "wait")
            registration = run["wait_registrations"][hold["wait"]["wait_registration_id"]]
            assert run["status"] == "completed" and run["outcome"] == "succeeded" and not run["attempts"]
            assert hold["wait"]["resolution"] == "timeout" and registration["status"] == "expired"
            report["cases"].append({"case": "wait-supported", "run_id": run["id"], "outcome": "succeeded",
                                    "attempts": 0, "active_registration_verified": True, "timeout_on_drive_verified": True})

            report["outcome"] = "passed"
        except Exception as error:
            report["failure"] = {"type": type(error).__name__, "message": str(error)[:4000]}
        finally:
            report["binary"]["unchanged_after_verification"] = binary.exists() and digest(binary.read_bytes()) == report["binary"]["sha256"]
            if not report["binary"]["unchanged_after_verification"]:
                report["outcome"] = "failed"
                report["failure"] = {"type": "BinaryChanged", "message": "executable changed during verification"}
            evidence_file.write(encoded(report))
    print(json.dumps({"outcome": report["outcome"], "project": str(target), "evidence": str(evidence), "commands": len(report["commands"]), "failure": report.get("failure")}, indent=2))
    return 0 if report["outcome"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
