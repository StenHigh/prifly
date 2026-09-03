#!/usr/bin/env python3
"""Verify an actual P2-04 -> P2-05 upgrade in one fresh local authority.

Both binaries execute real CLI commands. No database copies, SQL edits, replay
fixtures or mock workers are used. Evidence and raw responses are retained.

Checkpoint status: syntax checked only; native qualification is still pending.
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
P204_SHA256 = "6c138be164819bf53d12516ab216675ecf5e7a2823fe0602ee983eadb977285b"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False) + "\n").encode()


def persisted(view):
    return {key: view[key] for key in ("schema_version", "run_version", "event_sequence", "run")}


def artifact_refs(value):
    if isinstance(value, dict):
        if {"artifact_id", "revision", "digest"} <= value.keys():
            yield {key: value[key] for key in ("artifact_id", "revision", "digest")}
        else:
            for child in value.values():
                yield from artifact_refs(child)
    elif isinstance(value, list):
        for child in value:
            yield from artifact_refs(child)


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--old-binary", type=Path, required=True)
    parser.add_argument("--new-binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    old, new = args.old_binary.resolve(strict=True), args.new_binary.resolve(strict=True)
    identities = {name: {"path": str(path), "sha256": digest(path.read_bytes())} for name, path in (("old", old), ("new", new))}
    if identities["old"]["sha256"] != P204_SHA256:
        parser.error("old-binary must be the exact accepted P204 executable: " + P204_SHA256)
    if identities["new"]["sha256"] == P204_SHA256:
        parser.error("new-binary must differ from the frozen P204 executable")
    evidence = args.evidence.absolute()
    evidence.parent.mkdir(parents=True, exist_ok=True)
    with evidence.open("xb") as evidence_file:
        temporary = Path(tempfile.mkdtemp(prefix="prifly-context-upgrade-", dir="/tmp")).resolve()
        project, transcript = temporary / "project", temporary / "transcript"
        transcript.mkdir()
        report = {
            "schema_version": "context-upgrade-evidence/1", "outcome": "failed", "binaries": identities,
            "reference_p204_sha256": P204_SHA256, "harness_sha256": digest(Path(__file__).read_bytes()),
            "host": {"platform": platform.platform(), "machine": platform.machine(), "python": platform.python_version()},
            "temporary_directory": str(temporary), "project": str(project), "transcript": str(transcript),
            "commands": [], "comparisons": [], "artifacts": [],
            "limitations": [
                "One fresh authority at its final path; no SQL edits, database copy, migration, upcast or restore.",
                "Local cooperative shell workers only; no AI, network, external-effect or sandbox qualification.",
                "One owned driver and an owner pause overlap; old/new writers are never concurrent.",
                "Workers wait only for their own workspace marker, with a finite timeout; cleanup may kill only the Popen-owned driver.",
                "Persisted public Run fields and full public events are compared; volatile live timing and hidden replay bytes are not exposed.",
                "Receipts cover event-referenced CLI-owner commands; private runner receipts are not exposed by command receipt.",
                "Exact Start retries may append diagnostic samples, but must preserve original receipts, Run state and event sequences.",
                "This compatibility check does not close P2-05, hardware clock, crash-recovery or remaining F2 acceptance gates.",
            ],
        }

        def record(label, argv, code, stdout, stderr):
            entry = {"label": label, "argv": argv, "exit_code": code}
            for stream, data in (("stdout", stdout), ("stderr", stderr)):
                name = f"{len(report['commands']):03d}-{label}.{stream}"
                (transcript / name).write_bytes(data)
                entry[stream] = {"file": name, "sha256": digest(data), "size_bytes": len(data)}
            report["commands"].append(entry)
            return entry

        def response(label, entry, stdout, stderr, rejection=None):
            if rejection:
                assert entry["exit_code"] > 0 and not stdout, (label, "expected clean refusal")
                problem = json.loads(stderr)
                assert problem["code"] == rejection, (label, problem)
                entry["expected_rejection"] = problem
                return problem
            assert entry["exit_code"] == 0, (label, entry["exit_code"], str(transcript / entry["stderr"]["file"]))
            return json.loads(stdout)

        def cli(binary, label, *arguments, rejection=None):
            argv = [str(binary), "--project", str(project), "--json", *map(str, arguments)]
            try:
                result = subprocess.run(argv, capture_output=True, timeout=120)
            except subprocess.TimeoutExpired as error:
                record(label, argv, None, error.stdout or b"", error.stderr or b"")["timeout_seconds"] = 120
                raise
            entry = record(label, argv, result.returncode, result.stdout, result.stderr)
            return response(label, entry, result.stdout, result.stderr, rejection)

        def write(name, value):
            path = project / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(value if isinstance(value, bytes) else encoded(value))

        def same(label, before, after):
            data = encoded(before)
            assert data == encoded(after), (label, "values differ")
            report["comparisons"].append({"check": label, "sorted_json_sha256": digest(data)})

        entries, registry_version = [], "1"

        def define(binary, kind, name, identifier, value, raw_text=False):
            write(name, value)
            arguments = ["ref", name, "--id", identifier, "--version", "1.0.0"]
            if raw_text:
                arguments.append("--raw-text")
            ref = cli(binary, "ref-" + Path(name).stem, *arguments)
            assert set(ref) == {"id", "version", "digest"}
            entry = {"ref": ref, "kind": kind, "path": name}
            if raw_text:
                entry.update(byte_encoding="utf8_text", media_type="text/plain; charset=utf-8")
            entries.append(entry)
            write("definitions.json", {"schema_version": registry_version, "entries": entries})
            return ref

        def history(binary, label, run_id):
            value = cli(binary, label, "run", "events", run_id, "--limit", "1000")["view"]
            assert not value["more"]
            return value

        def export(binary, label, ref):
            ref_file, output_file = f"verification/{label}-ref.json", f"verification/{label}.bin"
            write(ref_file, ref)
            metadata = cli(binary, label + "-inspect", "artifact", "inspect", "--ref", ref_file)
            cli(binary, label + "-export", "artifact", "export", "--ref", ref_file, "--output", output_file)
            data = (project / output_file).read_bytes()
            assert metadata["ref"] == ref and "sha256:" + digest(data) == ref["digest"]
            assert metadata["artifact"]["size_bytes"] == len(data)
            return metadata, data, output_file

        def drive(binary, label, run_id, pause=False):
            # Every wait/release is confined to newly created fixture workspaces.
            before = set(workspace.glob("*/worker-ready"))
            argv = [str(binary), "--project", str(project), "--json", "run", "drive", run_id]
            with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
                process = subprocess.Popen(argv, stdout=stdout, stderr=stderr)
                try:
                    deadline, paused = time.monotonic() + 120, False
                    while process.poll() is None:
                        markers = set(workspace.glob("*/worker-ready")) - before
                        if pause and not paused and markers:
                            current = cli(binary, label + "-running", "run", "status", run_id)
                            attempts = list(current["run"]["attempts"].values())
                            assert len(attempts) == 1 and len(markers) == 1
                            assert Path(attempts[0]["workspace"]) / "worker-ready" in markers
                            if attempts[0].get("started"):
                                cli(binary, label + "-pause", "run", "pause", run_id, "--reason", "Pause an actual repeat before upgrade", "--command-id", "command:context-upgrade-pause")
                                paused = True
                        if not pause or paused:
                            for marker in markers:
                                (marker.parent / "worker-release").touch(exist_ok=True)
                        assert time.monotonic() < deadline, (label, "owned driver exceeded fixture deadline")
                        time.sleep(0.025)
                    process.wait(timeout=5)
                    assert not pause or paused, "driver finished before the intended pause boundary"
                finally:
                    if process.poll() is None:
                        process.kill()  # Only the process created by the Popen above.
                    process.wait(timeout=10)
                    stdout.seek(0)
                    stderr.seek(0)
                    out, err = stdout.read(), stderr.read()
                    entry = record(label, argv, process.returncode, out, err)
            return response(label, entry, out, err)

        def settled_attempts(view, count):
            run = view["run"]
            assert len(run["attempts"]) == len(run["steps"]) == count and not run["active_attempt_ids"]
            for attempt in run["attempts"].values():
                process = attempt["process_outcome"]
                assert attempt["status"] == "completed" and attempt["settled"] and attempt["accepted"]["verdict"] == "pass"
                assert process["started"] and process["wait_returned"] and process["group_empty"] and not process["uncertain"] and process["exit_code"] == 0
                assert (Path(attempt["workspace"]) / "worker-starts").read_bytes() == b"start\n"

        def completed_repeat(view):
            run = view["run"]
            assert view["schema_version"] == "core-read/3" and run["schema_version"] == "core-state/3"
            assert run["status"] == "completed" and run["outcome"] == "succeeded" and run["control_transitions"] == 8
            settled_attempts(view, 2)
            bodies = sorted((inv for inv in run["invocations"].values() if "iteration" in inv), key=lambda inv: inv["iteration"])
            assert len(run["invocations"]) == 3 and [body["iteration"] for body in bodies] == [1, 2]
            assert all(body["status"] == "completed" and body["parent_invocation_id"] == run["root_workflow_invocation_id"] for body in bodies)
            repeat = run["activations"][bodies[0]["caller_stage_activation_id"]]["repeat"]
            assert repeat["iteration_count"] == 2 and repeat["current_body_workflow_invocation_id"] == bodies[1]["id"]
            assert repeat["last_decision"]["route"] == "on_limit"

        def workspace_files():
            return {str(path.relative_to(workspace)): digest(path.read_bytes()) if path.is_file() else "directory" for path in sorted(workspace.rglob("*"))}

        try:
            for name, binary in (("old", old), ("new", new)):
                identities[name]["version"] = cli(binary, name + "-version", "version")
            cli(old, "old-init", "init", "--profile", "core-workflow/1", project)
            builtins = {(entry["ref"]["id"], entry["ref"]["version"]): entry["ref"] for entry in cli(old, "old-inventory", "inventory")["definitions"]}
            config = json.loads((project / "prifly.json").read_bytes())
            assert config["default_policy_ref"] == builtins["core:policy/local", "2.0.0"]
            workspace = project / config["configuration"]["workspace_root"]
            worker = (b"#!/bin/sh\nset -eu\nprintf 'start\\n' >>worker-starts\n: >worker-ready\nremaining=1000\n"
                      b"while [ ! -f worker-release ]; do\n  remaining=$((remaining - 1))\n  [ \"$remaining\" -gt 0 ] || exit 71\n  sleep 0.05\ndone\n"
                      + (ROOT / "test/fixtures/foundation/empty-step.sh").read_bytes())
            write("scripts/worker.sh", worker)
            report["worker_sha256"] = digest(worker)
            write("brief.json", {"schema_version": "1", "id": "upgrade:brief/context", "subject": "Context upgrade compatibility", "desired_outcome": "Preserve old repeat work and refuse an unsupported authority", "in_scope": ["One owned scratch authority"], "out_of_scope": ["AI", "Network", "External writes"], "completion_criteria": ["Native compatibility assertions pass"], "source_refs": [], "assumptions": [], "confirmation": "explicit"})
            boolean = define(old, "schema", "schemas/boolean.json", "upgrade:schema/boolean", {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "boolean"})
            settings = define(old, "schema", "schemas/settings.json", "upgrade:schema/settings", {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": {"enabled": {"type": "boolean"}}, "required": ["enabled"], "additionalProperties": False})
            step = {"schema_version": "1", "id": "upgrade:step/repeat-worker", "version": "1.0.0", "title": "Owned shell worker", "kind": "command", "inputs": {}, "outputs": {}, "executor": {"adapter_ref": builtins["core:adapter/local-process", "1.0.0"], "operation": "process"}, "context_refs": [], "required_capabilities": [], "effects": {"class": "workspace_write", "retry_class": "pure"}, "result_check_refs": [], "result_schema_ref": builtins["core:schema/step-result", "1.0.0"]}
            step_ref = define(old, "step", "steps/repeat-worker.json", step["id"], step)
            executor = {"executable": "/bin/sh", "args": ["worker.sh"], "files": {"worker.sh": "scripts/worker.sh"}, "environment": {}, "timeout_ms": 60000, "grace_ms": 100, "max_output_bytes": 1048576}
            config["configuration"]["executors"][step["id"]] = executor
            write("prifly.json", config)

            def workflow(identifier, stages, entry, *, repeat=False, configured=False):
                port = {"format": "json", "schema_ref": settings, "required": True}
                if configured:
                    port["configuration"] = {"scope": "run", "default": {"enabled": True}}
                return {"schema_version": "2", "id": identifier, "version": "1.0.0", "title": identifier, "inputs": {"settings": port}, "outputs": {"enabled": {"format": "json", "schema_ref": boolean, "required_for": ["succeeded"]}}, "allowed_outcomes": ["succeeded"], "definition": {"entry": entry, "stages": stages}, "limits": {"max_step_instances": 2 if repeat else 1, "max_control_transitions": 8 if repeat else 2, "max_parallelism": 1, "max_child_depth": 1 if repeat else 0}, "policy_ref": config["default_policy_ref"]}

            projected = {"kind": "finish", "outcome": "succeeded", "output_bindings": {"enabled": {"from": "workflow_input", "port": "settings", "pointer": "/enabled", "projected_schema_ref": boolean}}}
            body = workflow("upgrade:workflow/repeat-body", {"work": {"kind": "step", "step_ref": step_ref, "input_bindings": {}, "on": {"pass": "done"}}, "done": projected}, "work")
            body_ref = define(old, "workflow", "workflows/body.json", body["id"], body)
            binding = {"settings": {"from": "workflow_input", "port": "settings"}}
            document = workflow("upgrade:workflow/repeat", {
                "loop": {"kind": "repeat", "body_workflow_ref": body_ref, "initial_bindings": binding, "next_bindings": binding, "continue_on": ["succeeded"],
                         "until": {"op": "eq", "left": {"kind": "field", "ref": {"from": "iteration_output", "port": "enabled", "pointer": ""}}, "right": {"kind": "literal", "value": False}},
                         "max_iterations": 2, "on_complete": {"succeeded": "done"}, "on_limit": "done"},
                "done": {"kind": "finish", "outcome": "succeeded", "output_bindings": {"enabled": {"from": "stage_output", "stage_id": "loop", "port": "enabled"}}},
            }, "loop", repeat=True, configured=True)
            define(old, "workflow", "workflows/repeat.json", document["id"], document)
            starts, run_ids = {}, {}
            for label in ("completed", "paused"):
                arguments = ["run", "start", "--workflow", "workflows/repeat.json", "--brief", "brief.json", "--command-id", "command:context-upgrade-" + label]
                accepted = cli(old, "old-start-" + label, *arguments)
                identifier = accepted["receipt"]["run_id"]
                starts[identifier], run_ids[label] = (arguments, accepted["receipt"]), identifier
                view = drive(old, "old-drive-" + label, identifier, pause=label == "paused")
                if label == "completed":
                    completed_repeat(view)
                else:
                    settled_attempts(view, 1)
                    run = view["run"]
                    bodies = [inv for inv in run["invocations"].values() if "iteration" in inv]
                    assert run["schema_version"] == "core-state/3" and run["resume_required"] and run["control_transitions"] == 2
                    assert len(bodies) == 1 and bodies[0]["iteration"] == 1 and bodies[0]["ready_stages"] == ["done"]
                    assert cli(old, "old-paused-next", "run", "next", identifier)["action"] == "restricted"
            baseline = {identifier: cli(old, "old-baseline-" + label, "run", "status", identifier) for label, identifier in run_ids.items()}
            histories = {identifier: history(old, "old-events-" + label, identifier) for label, identifier in run_ids.items()}
            cut = baseline[run_ids["completed"]]["cut"]
            assert all(view["cut"] == cut for view in baseline.values())
            assert not any(event["type"] == "stage.repeat_decided" for event in histories[run_ids["paused"]]["events"])
            owner = starts[run_ids["completed"]][1]["actor"]
            command_ids = sorted({event["command_id"] for view in histories.values() for event in view["events"] if event["actor"] == owner})
            receipts = {identifier: cli(old, f"old-receipt-{index:03d}", "command", "receipt", "--id", identifier) for index, identifier in enumerate(command_ids)}
            assert receipts
            samples, seen = [], set()
            pending = list(artifact_refs([baseline, histories]))
            for ref in pending:
                key = (ref["artifact_id"], ref["revision"], ref["digest"])
                if key in seen:
                    continue
                seen.add(key)
                assert len(seen) <= 128, "fixture artifact closure exceeded its bound"
                metadata, data, filename = export(old, f"old-artifact-{len(samples):03d}", ref)
                samples.append((ref, metadata, data))
                report["artifacts"].append({"ref": ref, "baseline_file": filename, "sha256": digest(data), "size_bytes": len(data)})
                pending.extend(metadata["artifact"]["provenance"])
            assert samples and any(data == b"true" for _, _, data in samples)
            queries, telemetry = {}, {}
            for mode, metrics in (("records", ["core.command_duration", "os.cpu_total", "timing.elapsed"]), ("aggregate", ["core.succeeded_run_fraction", "core.failed_run_fraction"])):
                queries[mode] = {"schema_version": "telemetry-query/1", "mode": mode, "run_ids": list(baseline), "metrics": metrics, "cut": cut, "limit": 1000}
                write("verification/" + mode + ".json", queries[mode])
                telemetry[mode] = cli(old, "old-fixed-" + mode, "telemetry", "query", "--file", "verification/" + mode + ".json")
                assert (telemetry[mode]["calculator_revision"], telemetry[mode]["timing_revision"]) == ("core-telemetry/2", "core-timing/1") and not telemetry[mode].get("next_cursor")
            assert any(record["metric"] == "os.cpu_total" and record["quality"] == "measured" for record in telemetry["records"]["records"])

            def fixed_telemetry(phase):
                for mode, saved in telemetry.items():
                    same("fixed-cut " + mode + " " + phase, saved, cli(new, phase + "-fixed-" + mode, "telemetry", "query", "--file", "verification/" + mode + ".json"))

            for label, identifier in run_ids.items():
                current = cli(new, "new-baseline-" + label, "run", "status", identifier)
                same("new reader retains state3 " + label, persisted(baseline[identifier]), persisted(current))
                # `view.cut` is authority-wide and can advance when the earlier
                # duplicate command receipt is recorded. Run facts must remain
                # byte-for-byte stable even when that cursor changes.
                same("new reader retains events " + label, histories[identifier]["events"], history(new, "new-events-" + label, identifier)["events"])
                assert current["cut"] >= cut
                arguments, receipt = starts[identifier]
                retried = cli(new, "new-exact-retry-" + label, *arguments)
                assert retried["duplicate"] is True
                same("exact original receipt " + label, receipt, retried["receipt"])
                same("exact retry retains state " + label, persisted(baseline[identifier]), persisted(cli(new, "new-after-retry-" + label, "run", "status", identifier)))
                same("exact retry retains events " + label, histories[identifier]["events"], history(new, "new-after-retry-events-" + label, identifier)["events"])
            fixed_telemetry("before-continuation")
            ready_id, completed_id = run_ids["paused"], run_ids["completed"]
            paused = cli(new, "new-drive-paused", "run", "drive", ready_id)
            same("pause blocks resumed work", persisted(baseline[ready_id]), persisted(paused))
            stop = next(stop for stop in paused["run"]["stops"] if stop["status"] == "active")
            cli(new, "new-release", "run", "release", ready_id, "--expected-epoch", paused["run"]["control_epoch"], "--stop", f"{stop['id']}:{stop['generation']}", "--reason", "Upgrade comparisons passed", "--command-id", "command:context-upgrade-release")
            released = cli(new, "new-drive-released", "run", "drive", ready_id)
            assert released["run"]["resume_required"] and released["run"]["control_transitions"] == 2
            same("release does not repeat accepted work", baseline[ready_id]["run"]["attempts"], released["run"]["attempts"])
            cli(new, "new-resume", "run", "resume", ready_id, "--expected-version", released["run_version"], "--reason", "Explicit continuation of pinned state3", "--command-id", "command:context-upgrade-resume")
            continued = drive(new, "new-drive-continued", ready_id)
            completed_repeat(continued)
            for identifier, attempt in baseline[ready_id]["run"]["attempts"].items():
                same("accepted old Attempt is not repeated or rewritten " + identifier, attempt, continued["run"]["attempts"][identifier])
            same("old P204 reads continued state3", persisted(continued), persisted(cli(old, "old-reads-continued", "run", "status", ready_id)))
            fixed_telemetry("after-continuation")
            continued_events = history(new, "new-continued-events", ready_id)["events"]
            prefix = histories[ready_id]["events"]
            same("continued Run preserves old event prefix", prefix, continued_events[:len(prefix)])
            assert [event["data"]["route"] for event in continued_events if event["type"] == "stage.repeat_decided"] == ["continue", "on_limit"]
            old_config, old_registry = (project / "prifly.json").read_bytes(), (project / "definitions.json").read_bytes()
            report["previous"] = {"run_ids": run_ids, "historical_cut": cut, "retained_owner_receipts": len(receipts), "accepted_old_attempt_ids": list(baseline[ready_id]["run"]["attempts"]), "paused_control_transitions": 2, "continued_control_transitions": 8, "continued_event_count": len(continued_events), "configuration_sha256": digest(old_config), "registry_sha256": digest(old_registry)}

            # The same authority now receives its first state4 Run. Old author
            # config/registry are restored before probing the journal boundary.
            builtins = {(entry["ref"]["id"], entry["ref"]["version"]): entry["ref"] for entry in cli(new, "new-context-inventory", "inventory")["definitions"]}
            registry_version = "3"
            instructions = b"Use only the explicitly supplied immutable context.\n"
            instruction_ref = define(new, "resource", "resources/instructions.txt", "upgrade:context/instructions", instructions, raw_text=True)
            adapter = builtins["core:adapter/local-process", "2.0.0"]
            context_step = {**step, "id": "upgrade:step/context-worker", "executor": {"adapter_ref": adapter, "operation": "process"}, "instructions_ref": instruction_ref}
            context_step_ref = define(new, "step", "steps/context-worker.json", context_step["id"], context_step)
            config["configuration_schema_ref"] = builtins["core:schema/core-configuration", "2.0.0"]
            config["configuration"]["schema_version"] = "core-configuration/2"
            config["adapter_bindings"]["local_process"] = adapter
            config["configuration"]["executors"][context_step["id"]] = executor
            write("prifly.json", config)
            context_workflow = workflow("upgrade:workflow/context", {"work": {"kind": "step", "step_ref": context_step_ref, "input_bindings": {}, "on": {"pass": "done"}}, "done": projected}, "work", configured=True)
            define(new, "workflow", "workflows/context.json", context_workflow["id"], context_workflow)
            preview = cli(new, "new-context-preview", "validate", "--workflow", "workflows/context.json")
            assert preview["admission"] is False
            accepted = cli(new, "new-context-start", "run", "start", "--workflow", "workflows/context.json", "--brief", "brief.json", "--command-id", "command:context-upgrade-state4")
            context_id = accepted["receipt"]["run_id"]
            context_before = cli(new, "new-context-ready", "run", "status", context_id)
            context_history = history(new, "new-context-ready-events", context_id)
            facts = [event for event in context_history["events"] if event["type"] == "run.context_pinned"]
            assert len(facts) == 1 and context_before["run"]["schema_version"] == "core-state/4" and context_before["schema_version"] == "core-read/4"
            assert not context_before["run"]["attempts"]
            assert len(context_before["run"]["steps"]) == 1
            assert next(iter(context_before["run"]["steps"].values()))["status"] == "ready"
            write("verification/context-project-config.json", (project / "prifly.json").read_bytes())
            write("verification/context-registry.json", (project / "definitions.json").read_bytes())
            write("prifly.json", old_config)
            write("definitions.json", old_registry)
            before_files = workspace_files()
            for label, operation, identifier in (("context-status", "status", context_id), ("context-drive", "drive", context_id), ("peer-status", "status", completed_id), ("peer-drive", "drive", completed_id)):
                cli(old, "old-refuses-" + label, "run", operation, identifier, rejection="unsupported_storage_version")
            after = cli(new, "new-after-old-refusals", "run", "status", context_id)
            same("unknown-event refusals retain ready context Run", persisted(context_before), persisted(after))
            same("unknown-event refusals retain full event view", context_history, history(new, "new-events-after-refusals", context_id))
            same("unknown-event refusals retain all workspace bytes and markers", before_files, workspace_files())
            assert after["cut"] == context_before["cut"]
            same("compatible state3 peer remains unchanged", persisted(baseline[completed_id]), persisted(cli(new, "new-peer-after-refusals", "run", "status", completed_id)))
            same("continued state3 remains unchanged", persisted(continued), persisted(cli(new, "new-continued-after-refusals", "run", "status", ready_id)))
            assert (project / "prifly.json").read_bytes() == old_config and (project / "definitions.json").read_bytes() == old_registry
            fixed_telemetry("after-context-pinned")

            # Dispatch relies on the Run's pinned config2 and resource closure,
            # not the restored config1/registry1 or a silent migration.
            context_finished = drive(new, "new-drive-pinned-context", context_id)
            settled_attempts(context_finished, 1)
            assert context_finished["run"]["schema_version"] == "core-state/4" and context_finished["run"]["status"] == "completed"
            attempt = next(iter(context_finished["run"]["attempts"].values()))
            transport = attempt["context"]
            assert transport["schema_version"] == "local-context/2" and len(transport["sources"]) == 1
            _, raw_manifest, _ = export(new, "context-manifest", transport["manifest"]["ref"])
            _, raw_rendering, _ = export(new, "context-rendering", transport["rendering"]["ref"])
            manifest, rendering = json.loads(raw_manifest), json.loads(raw_rendering)
            assert rendering["manifest"] == manifest and len(manifest["entries"]) == 1
            assert (manifest["entries"][0]["role"], manifest["entries"][0]["trust"]) == ("instruction", "trusted_instruction")
            assert rendering["sources"][0]["value"].encode() == instructions
            assert (Path(attempt["workspace"]) / transport["sources"][0]["path"]).read_bytes() == instructions
            final_events = history(new, "new-final-context-events", context_id)
            same("state4 continuation preserves initial pinned event prefix", context_history["events"], final_events["events"][:len(context_history["events"])])
            redriven = cli(new, "new-terminal-context-redrive", "run", "drive", context_id)
            same("terminal context drive is inert", persisted(context_finished), persisted(redriven))
            assert redriven["cut"] == context_finished["cut"]
            for index, (ref, saved, data) in enumerate(samples):
                metadata, actual, _ = export(new, f"new-old-artifact-{index:03d}", ref)
                same(f"old artifact metadata retained {index}", saved, metadata)
                assert actual == data, ("old artifact bytes changed", ref)
            for index, (identifier, receipt) in enumerate(receipts.items()):
                same(f"old owner receipt retained {index}", receipt, cli(new, f"new-old-receipt-{index:03d}", "command", "receipt", "--id", identifier))
            fixed_telemetry("after-all-writes")
            same("terminal old events retain all facts", histories[completed_id]["events"], history(new, "new-final-old-completed-events", completed_id)["events"])
            same("continued old history is unchanged by state4", continued_events, history(new, "new-final-old-continued-events", ready_id)["events"])
            for label, identifiers, expected in (("old-cohort", list(baseline), ("core-telemetry/2", "core-timing/1")), ("context-cohort", [context_id], ("core-telemetry/3", "core-timing/2"))):
                write("verification/" + label + ".json", {"schema_version": "telemetry-query/1", "mode": "records", "run_ids": identifiers, "metrics": ["timing.elapsed"], "limit": 1000})
                current = cli(new, "new-final-" + label, "telemetry", "query", "--file", "verification/" + label + ".json")
                assert (current["calculator_revision"], current["timing_revision"]) == expected and not current.get("next_cursor")
            population = cli(new, "new-final-population", "telemetry", "catalog")["population"]
            assert population["matched"] == 3 and population["attempts"] == 5 and population["invocations"] == 7
            report["context"] = {"run_id": context_id, "compatible_peer_run_id": completed_id, "pinned_event_cut": context_before["cut"], "pinned_event_sequence": facts[0]["seq"], "old_authority_refusals": 4, "final_cut": context_finished["cut"], "context_manifest_ref": transport["manifest"]["ref"], "context_rendering_ref": transport["rendering"]["ref"], "population": population}
            report["outcome"] = "passed"
        except Exception as error:
            report["failure"] = {"type": type(error).__name__, "message": str(error)[:4000]}
        finally:
            for name, binary in (("old", old), ("new", new)):
                identities[name]["unchanged_after_verification"] = binary.exists() and digest(binary.read_bytes()) == identities[name]["sha256"]
                if not identities[name]["unchanged_after_verification"]:
                    report["outcome"] = "failed"
                    report["failure"] = {"type": "BinaryChanged", "message": name + " executable changed during verification"}
            if digest(Path(__file__).read_bytes()) != report["harness_sha256"]:
                report["outcome"] = "failed"
                report["failure"] = {"type": "HarnessChanged", "message": "verification script changed during execution"}
            evidence_file.write(encoded(report))
        print(json.dumps({"outcome": report["outcome"], "evidence": str(evidence), "temporary_directory": str(temporary), "commands": len(report["commands"]), "comparisons": len(report["comparisons"]), "failure": report.get("failure")}, indent=2))
        return 0 if report["outcome"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
