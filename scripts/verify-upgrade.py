#!/usr/bin/env python3
"""Verify actual F1 -> F2 CLI compatibility without copying an authority.

Supply two distinct saved executables. Both must run natively; local process
and Unix socket access are required. Projects and raw command outputs remain
in a new temporary directory. Existing evidence is never overwritten.
"""

import argparse
import hashlib
import json
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[1]
F1_RC_SHA256 = "129575e90ee3372f43d25e937c0d5d32b882f18418dbfa064dbe2ea55f3b7355"


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def json_bytes(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False) + "\n").encode()


def persisted(view):
    # as_of and the live timing view are observations of this read, not stored
    # facts. Compare all Run metadata exposed by the public view and counters;
    # private replay payloads, credentials and executable config stay private.
    return {key: view[key] for key in ("schema_version", "run_version", "event_sequence", "run")}


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--old-binary", required=True, type=Path)
    parser.add_argument("--new-binary", required=True, type=Path)
    parser.add_argument("--evidence", required=True, type=Path)
    args = parser.parse_args()
    old, new = args.old_binary.resolve(strict=True), args.new_binary.resolve(strict=True)
    identities = {name: {"path": str(path), "sha256": sha256(path.read_bytes())} for name, path in (("old", old), ("new", new))}
    if identities["old"]["sha256"] == identities["new"]["sha256"]:
        parser.error("old and new must be distinct actual executables")
    evidence = args.evidence.absolute()
    evidence.parent.mkdir(parents=True, exist_ok=True)
    # Keep an exclusive handle so failures are recorded without replacing a
    # previous result or mistaking a partially executed verification for PASS.
    with evidence.open("xb") as evidence_file:
        temporary = Path(tempfile.mkdtemp(prefix="prifly-upgrade-", dir="/tmp")).resolve()
        f1, f2, outputs = temporary / "foundation", temporary / "extended", temporary / "transcript"
        outputs.mkdir()
        report = {
            "schema_version": "upgrade-compatibility-evidence/1", "outcome": "failed",
            "host": {"platform": platform.platform(), "machine": platform.machine(), "python": platform.python_version()},
            "temporary_directory": str(temporary), "f1_project": str(f1), "f2_project": str(f2),
            "binaries": identities, "reference_f1_rc_1_sha256": F1_RC_SHA256,
            "old_matches_f1_rc_1": identities["old"]["sha256"] == F1_RC_SHA256,
            "harness_sha256": sha256(Path(__file__).read_bytes()),
            "fixtures": {name: sha256((ROOT / "test/fixtures/foundation" / name).read_bytes()) for name in ("create-demo.py", "demo-step.py", "prifly_step.py", "empty-step.sh")},
            "commands": [], "comparisons": [],
            "limitations": [
                "Native local cooperative executors only; no isolation or external-effect claim.",
                "F1 authority stays at its original path; F2 uses a separate fresh authority.",
                "No direct SQLite edits, backup restore, storage migration, event upcast or mixed-version concurrent writers are exercised.",
                "Downgrade means an old binary refuses the selected F2 configuration/Run; no unsupported old-binary writes are authorized.",
                "Read comparisons exclude volatile as_of/live timing and compare all exposed Run metadata/counters, not private replay payloads; fixed-cut telemetry is compared in full.",
                "This smoke does not replace crash, disk-full, hardware suspend/resume, F1 qualification or full F2 acceptance.",
            ],
        }

        def invoke(label, argv, *, reject=False):
            argv = [str(item) for item in argv]
            result = subprocess.run(argv, capture_output=True, timeout=60)
            entry = {"label": label, "argv": argv, "exit_code": result.returncode}
            for stream in ("stdout", "stderr"):
                data = getattr(result, stream)
                filename = f"{len(report['commands']):03d}-{label}.{stream}"
                (outputs / filename).write_bytes(data)
                entry[stream] = {"file": filename, "sha256": sha256(data), "size_bytes": len(data)}
            report["commands"].append(entry)
            if reject:
                assert result.returncode > 0 and not result.stdout, f"{label}: expected a clean refusal"
                problem = json.loads(result.stderr)
                # RC.1 sanitizes a mismatched configuration schema to generic
                # invalid_input. The inspected fixture must still be runnable
                # by the new binary; unrelated IO/permission failures do not pass.
                codes = {reject} if isinstance(reject, str) else {"invalid_input", "unsupported_configuration", "unsupported_profile", "incompatible_run", "unsupported_storage_version"}
                assert problem["code"] in codes, (label, problem)
                entry["expected_rejection"] = problem
                return problem
            if result.returncode != 0:
                raise RuntimeError(f"{label} failed ({result.returncode}); inspect {outputs / entry['stderr']['file']}")
            return json.loads(result.stdout)

        def cli(binary, project, label, *arguments, reject=False):
            return invoke(label, [binary, "--project", project, "--json", *arguments], reject=reject)

        def write(project, name, value):
            path = project / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(json_bytes(value))

        def same(label, before, after):
            data = json_bytes(before)
            assert data == json_bytes(after), f"{label}: values differ"
            report["comparisons"].append({"check": label, "outcome": "passed", "sorted_json_sha256": sha256(data)})

        def events(binary, project, label, run_id):
            value = cli(binary, project, label, "run", "events", run_id, "--limit", "1000")["view"]
            assert value["more"] is False, "fixture event history must fit one complete page"
            return value

        try:
            for name, binary in (("old", old), ("new", new)):
                identities[name]["version"] = invoke(name + "-version", [binary, "--json", "version"])
            assert identities["old"]["version"]["semantics_profile"] == "foundation-sequence/1"
            invoke("create-foundation-demo", [sys.executable, "-B", ROOT / "test/fixtures/foundation/create-demo.py", "--binary", old, "--target", f1])
            terminal = cli(old, f1, "old-transform", "run", "start", "--workflow", "workflows/transform.json", "--brief", "brief.json", "--input", "source=inputs/unchanged.txt", "--command-id", "command:upgrade-transform", "--drive")
            terminal_id = terminal["run"]["id"]
            assert terminal["run"]["status"] == "completed" and terminal["run"]["outcome"] == "succeeded"
            assert len(terminal["run"]["attempts"]) == 1
            assert all(a.get("accepted") and a["process_outcome"]["exit_code"] == 0 for a in terminal["run"]["attempts"].values())
            assert any(d["severity"] == "warn" for d in terminal["run"]["diagnostics"])
            pending = cli(old, f1, "old-ready-start", "run", "start", "--workflow", "workflows/shell.json", "--brief", "brief.json", "--command-id", "command:upgrade-ready")
            ready_id = pending["receipt"]["run_id"]
            cli(old, f1, "old-pause", "run", "pause", ready_id, "--reason", "Pause before upgrade", "--command-id", "command:upgrade-pause")
            baseline = {identifier: cli(old, f1, "old-status-" + label, "run", "status", identifier) for label, identifier in (("terminal", terminal_id), ("ready", ready_id))}
            cut = baseline[terminal_id]["cut"]
            assert baseline[ready_id]["cut"] == cut and not baseline[ready_id]["run"]["attempts"]
            assert all(not view["driver_live"] for view in baseline.values())
            history = {identifier: events(old, f1, "old-events-" + label, identifier) for label, identifier in (("terminal", terminal_id), ("ready", ready_id))}
            receipt = cli(old, f1, "old-receipt", "command", "receipt", "--id", "command:upgrade-transform")
            artifacts = {}
            for port, ref in terminal["run"]["output_artifacts"].items():
                write(f1, f"verification/{port}-ref.json", ref)
                artifacts[port] = cli(old, f1, "old-artifact-" + port, "artifact", "inspect", "--ref", f"verification/{port}-ref.json")
            assert set(artifacts) == {"text", "report"}
            queries = {
                "records": ["core.command_requests", "core.command_duration", "os.cpu_total", "step.processed_total", "step.quality_warnings", "timing.elapsed"],
                "aggregate": ["core.failed_run_fraction", "core.succeeded_run_fraction", "core.warning_run_fraction", "core.first_attempt_pass_fraction"],
            }
            telemetry = {}
            for mode, metrics in queries.items():
                write(f1, f"verification/{mode}-query.json", {"schema_version": "telemetry-query/1", "mode": mode, "run_ids": [terminal_id, ready_id], "metrics": metrics, "cut": cut, "limit": 1000})
                telemetry[mode] = cli(old, f1, "old-telemetry-" + mode, "telemetry", "query", "--file", f"verification/{mode}-query.json")
                assert telemetry[mode]["cut"] == cut and not telemetry[mode].get("next_cursor")
                assert telemetry[mode]["calculator_revision"] == "foundation-telemetry/1" and telemetry[mode]["timing_revision"] == "foundation-timing/1"
            records = telemetry["records"]["records"]
            assert any(r["metric"] == "core.command_duration" and r["value"] is not None for r in records), "no historical core samples captured"
            assert any(r["metric"] == "os.cpu_total" and r["quality"] == "measured" for r in records), "no actual process CPU observation"
            assert any(r["metric"] == "step.quality_warnings" for r in records), "no historical warning observation"
            report.update({"historical_cut": cut, "old_terminal_run": terminal_id, "upgraded_ready_run": ready_id})
            for label, identifier in (("terminal", terminal_id), ("ready", ready_id)):
                view = cli(new, f1, "new-status-" + label, "run", "status", identifier)
                same("new reader preserves " + label + " Run", persisted(baseline[identifier]), persisted(view))
                same("new reader preserves " + label + " events", history[identifier], events(new, f1, "new-events-" + label, identifier))
                assert view["cut"] == cut
            same("new reader preserves receipt", receipt, cli(new, f1, "new-receipt", "command", "receipt", "--id", "command:upgrade-transform"))
            for mode in queries:
                same("new reader preserves fixed-cut " + mode, telemetry[mode], cli(new, f1, "new-telemetry-" + mode, "telemetry", "query", "--file", f"verification/{mode}-query.json"))
            for port, artifact in artifacts.items():
                same("new reader preserves artifact " + port, artifact, cli(new, f1, "new-artifact-" + port, "artifact", "inspect", "--ref", f"verification/{port}-ref.json"))
                cli(new, f1, "new-export-" + port, "artifact", "export", "--ref", f"verification/{port}-ref.json", "--output", f"verification/exported-{port}")
                content = (f1 / f"verification/exported-{port}").read_bytes()
                assert "sha256:" + sha256(content) == artifact["ref"]["digest"]
            source = (f1 / "inputs/unchanged.txt").read_bytes()
            assert (f1 / "verification/exported-text").read_bytes() == source.decode().upper().encode()
            assert json.loads((f1 / "verification/exported-report").read_bytes()) == {"bytes": len(source), "changed": False}
            doctor = cli(new, f1, "new-doctor", "doctor")
            report["sqlite"] = doctor["sqlite"]
            assert doctor["sqlite"]["storage_version"] == 1 and doctor["sqlite"]["read_only"] is True
            stopped = cli(new, f1, "new-drive-paused", "run", "drive", ready_id)
            same("paused drive preserves Run", persisted(baseline[ready_id]), persisted(stopped))
            assert stopped["cut"] == cut and not stopped["run"]["attempts"]
            # --drive is a CLI action after Start, not part of its protected
            # command. Ask for the receipt on this retry, without driving again.
            retry = cli(new, f1, "new-exact-start-retry", "run", "start", "--workflow", "workflows/transform.json", "--brief", "brief.json", "--input", "source=inputs/unchanged.txt", "--command-id", "command:upgrade-transform")
            assert retry["duplicate"] is True
            same("exact Start retry preserves original receipt and receipt cut", receipt["receipt"], retry["receipt"])
            retried = cli(new, f1, "new-status-after-start-retry", "run", "status", terminal_id)
            same("exact Start retry preserves Run and event sequence", persisted(baseline[terminal_id]), persisted(retried))
            same("exact Start retry does not add events", history[terminal_id]["events"], events(new, f1, "new-events-after-start-retry", terminal_id)["events"])
            population = cli(new, f1, "new-population-after-start-retry", "telemetry", "catalog")["population"]
            assert population["matched"] == 2 and population["attempts"] == 1, "Start retry duplicated a Run or Attempt"
            # Instrumentation of this new request may append optional samples.
            # Receipt.Cut stays fixed; the authority's latest cut need not.
            assert retried["cut"] >= cut
            report["exact_start_retry"] = {"duplicate": True, "receipt_cut": retry["receipt"]["cut"], "before_cut": cut, "after_cut": retried["cut"]}
            stop = next(s for s in stopped["run"]["stops"] if s["status"] == "active")
            cli(new, f1, "new-release", "run", "release", ready_id, "--expected-epoch", str(stopped["run"]["control_epoch"]), "--stop", f"{stop['id']}:{stop['generation']}", "--reason", "Release after verified upgrade", "--command-id", "command:upgrade-release")
            released = cli(new, f1, "new-drive-released", "run", "drive", ready_id)
            assert released["run"]["resume_required"] and not released["run"]["attempts"]
            cli(new, f1, "new-resume", "run", "resume", ready_id, "--expected-version", str(released["run_version"]), "--reason", "Explicit continuation", "--command-id", "command:upgrade-resume")
            completed = cli(new, f1, "new-drive-resumed", "run", "drive", ready_id)
            assert completed["run"]["status"] == "completed" and completed["run"]["outcome"] == "succeeded"
            assert completed["run"]["semantics_profile"] == "foundation-sequence/1" and len(completed["run"]["attempts"]) == 1
            after = events(new, f1, "new-completed-events", ready_id)
            assert sum(e["type"] == "attempt.dispatching" for e in after["events"]) == 1
            again = cli(new, f1, "new-repeat-terminal-drive", "run", "drive", ready_id)
            same("terminal drive is inert", persisted(completed), persisted(again))
            assert again["cut"] == completed["cut"]
            terminal_after = cli(new, f1, "new-original-terminal-status", "run", "status", terminal_id)
            same("original accepted Run remains unchanged", persisted(baseline[terminal_id]), persisted(terminal_after))
            same("original events remain unchanged", history[terminal_id]["events"], events(new, f1, "new-original-events", terminal_id)["events"])
            same("original receipt remains unchanged", receipt, cli(new, f1, "new-original-receipt", "command", "receipt", "--id", "command:upgrade-transform"))
            for mode in queries:
                same("later writes preserve old-cut " + mode, telemetry[mode], cli(new, f1, "new-historical-" + mode, "telemetry", "query", "--file", f"verification/{mode}-query.json"))
            report["f1_final_cut"] = completed["cut"]

            # Earlier F1 compilers treated ref-shaped JSON Schema annotations
            # as dependencies. Preserve the original lock on an exact retry,
            # even though the new compiler correctly treats annotations as data.
            annotation = temporary / "annotation"
            cli(old, annotation, "old-init-annotation", "init", annotation)
            old_inventory = cli(old, annotation, "old-annotation-inventory", "inventory")["definitions"]
            core_refs = [entry["ref"] for entry in old_inventory]
            assert all(ref["id"].startswith("core:") for ref in core_refs)
            annotation_entries = []

            def annotation_define(kind, name, identifier, value):
                write(annotation, name, value)
                ref = cli(old, annotation, "old-ref-annotation-" + Path(name).stem, "ref", name, "--id", identifier, "--version", "1.0.0")
                assert set(ref) == {"id", "version", "digest"}
                annotation_entries.append({"ref": ref, "kind": kind, "path": name})
                write(annotation, "definitions.json", {"schema_version": "1", "entries": annotation_entries})
                return ref

            extra_ref = annotation_define("resource", "resources/annotation.json", "upgrade:resource/annotation", {"description": "Schema annotation, not executable behavior"})
            boolean_ref = annotation_define("schema", "schemas/boolean.json", "upgrade:schema/boolean", {
                "$schema": "https://json-schema.org/draft/2020-12/schema", "type": "boolean", "x-metadata": extra_ref,
            })
            policy_ref = next(ref for ref in core_refs if ref["id"] == "core:policy/local")
            resolver_ref = next(ref for ref in core_refs if ref["id"] == "core:resolver/local")
            annotation_define("workflow", "workflows/annotation.json", "upgrade:workflow/annotation", {
                "schema_version": "1", "id": "upgrade:workflow/annotation", "version": "1.0.0", "title": "Historical F1 schema annotation",
                "inputs": {"flag": {"format": "json", "schema_ref": boolean_ref, "required": True}},
                "outputs": {"flag": {"format": "json", "schema_ref": boolean_ref, "required_for": []}},
                "allowed_outcomes": ["succeeded"], "definition": {"entry": "done", "stages": {
                    "done": {"kind": "finish", "outcome": "succeeded", "output_bindings": {"flag": {"from": "workflow_input", "port": "flag"}}},
                }}, "limits": {"max_step_instances": 1, "max_control_transitions": 4, "max_parallelism": 1, "max_child_depth": 0}, "policy_ref": policy_ref,
            })
            shutil.copyfile(f1 / "brief.json", annotation / "brief.json")
            write(annotation, "inputs/false.json", False)
            write(annotation, "inputs/true.json", True)
            annotation_start = ["run", "start", "--workflow", "workflows/annotation.json", "--brief", "brief.json", "--command-id", "command:upgrade-annotation"]
            original = cli(old, annotation, "old-start-annotation", *annotation_start, "--input", "flag=inputs/false.json")
            annotation_id = original["receipt"]["run_id"]
            old_view = cli(old, annotation, "old-drive-annotation", "run", "drive", annotation_id)
            assert old_view["run"]["status"] == "completed" and not old_view["run"]["attempts"]
            assert old_view["run"]["authority_id"] != baseline[terminal_id]["run"]["authority_id"]
            old_events = events(old, annotation, "old-annotation-events", annotation_id)["events"]
            # Read only the generated reference records, never SQLite/private
            # replay. Reconstruct the lock from this fresh authority's one Start
            # and ask the old canonical ref API to prove its exact digest.
            pins = [json.loads(path.read_bytes())["ref"] for path in sorted((annotation / ".prifly/inventory").glob("*.json"))]
            assert extra_ref in pins, "old compiler did not pin the schema annotation fixture"
            closure = sorted(core_refs + pins, key=lambda ref: ref["id"] + "@" + ref["version"] + "#" + ref["digest"])
            lock_ref = old_view["run"]["package_lock_ref"]
            write(annotation, "verification/original-lock.json", {
                "schema_version": "1", "id": lock_ref["id"], "version": "1.0.0", "core_protocol": "1", "closure": closure, "resolver_ref": resolver_ref,
            })
            reconstructed = cli(old, annotation, "old-annotation-lock-ref", "ref", "verification/original-lock.json", "--id", lock_ref["id"], "--version", lock_ref["version"])
            same("historical annotation is in the original exact PackageLock", lock_ref, reconstructed)
            retried = cli(new, annotation, "new-exact-annotation-retry", *annotation_start, "--input", "flag=inputs/false.json")
            assert retried["duplicate"] is True
            same("historical annotation retry preserves the original receipt", original["receipt"], retried["receipt"])
            retried_view = cli(new, annotation, "new-annotation-status", "run", "status", annotation_id)
            same("historical annotation retry preserves Run and exact PackageLock", persisted(old_view), persisted(retried_view))
            same("historical annotation retry does not append events", old_events, events(new, annotation, "new-annotation-events", annotation_id)["events"])
            # The existing F1 input ArtifactID is derived from CommandID/port.
            # Different bytes conflict with that immutable identity before the
            # command digest is checked; the CLI sanitizes this to invalid_input.
            cli(new, annotation, "new-rejects-changed-annotation-input", *annotation_start, "--input", "flag=inputs/true.json", reject="invalid_input")
            unchanged = cli(new, annotation, "new-annotation-after-conflict", "run", "status", annotation_id)
            same("changed input refusal preserves historical Run and PackageLock", persisted(old_view), persisted(unchanged))
            same("changed input refusal preserves historical events", old_events, events(new, annotation, "new-annotation-events-after-conflict", annotation_id)["events"])
            same("changed input refusal preserves original receipt", original["receipt"], cli(new, annotation, "new-annotation-receipt-after-conflict", "command", "receipt", "--id", "command:upgrade-annotation")["receipt"])
            population = cli(new, annotation, "new-annotation-population", "telemetry", "catalog")["population"]
            assert population["matched"] == 1 and population["attempts"] == 0
            report["historical_annotation_retry"] = {"project": str(annotation), "run_id": annotation_id, "annotation_ref": extra_ref, "original_lock_ref": lock_ref, "duplicate": True, "changed_input_refused": True}

            # Initialize a separate authority normally, then reuse only authored
            # fixture files. Never copy .prifly, installation identity or DB.
            cli(new, f2, "new-init-f2", "init", "--profile", "core-workflow/1", f2)
            for name in ("steps/shell.json", "workflows/shell.json", "scripts/empty-step.sh", "brief.json"):
                destination = f2 / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(f1 / name, destination)
            registry = json.loads((f1 / "definitions.json").read_bytes())
            registry["entries"] = [entry for entry in registry["entries"] if entry["path"] in {"steps/shell.json", "workflows/shell.json"}]
            assert len(registry["entries"]) == 2
            write(f2, "definitions.json", registry)
            config = json.loads((f2 / "prifly.json").read_bytes())
            config["configuration"]["executors"] = {"demo:step/shell": json.loads((f1 / "prifly.json").read_bytes())["configuration"]["executors"]["demo:step/shell"]}
            write(f2, "prifly.json", config)
            created = cli(new, f2, "new-start-f2", "run", "start", "--workflow", "workflows/shell.json", "--brief", "brief.json", "--command-id", "command:upgrade-f2")
            f2_id = created["receipt"]["run_id"]
            before = cli(new, f2, "new-status-f2-before", "run", "status", f2_id)
            assert before["run"]["semantics_profile"] == "core-workflow/1" and not before["run"]["attempts"]
            assert before["run"]["authority_id"] != baseline[terminal_id]["run"]["authority_id"]
            before_events = events(new, f2, "new-events-f2-before", f2_id)
            workspace = f2 / config["configuration"]["workspace_root"]
            assert not list(workspace.iterdir()), "fresh F2 fixture unexpectedly has a workspace"
            for operation in ("status", "drive"):
                cli(old, f2, "old-refuses-f2-" + operation, "run", operation, f2_id, reject=True)
                inspected = cli(new, f2, "new-inspects-f2-after-" + operation, "run", "status", f2_id)
                same("old " + operation + " leaves F2 Run unchanged", persisted(before), persisted(inspected))
                assert inspected["cut"] == before["cut"] and not list(workspace.iterdir())
                same("old " + operation + " leaves F2 journal unchanged", before_events, events(new, f2, "new-events-f2-after-" + operation, f2_id))
            f2_completed = cli(new, f2, "new-drive-f2", "run", "drive", f2_id)
            assert f2_completed["run"]["status"] == "completed" and f2_completed["run"]["outcome"] == "succeeded"
            assert len(f2_completed["run"]["attempts"]) == 1
            assert sum(e["type"] == "attempt.dispatching" for e in events(new, f2, "new-events-f2-completed", f2_id)["events"]) == 1
            report["f2_run"] = {"id": f2_id, "before_cut": before["cut"], "final_cut": f2_completed["cut"], "attempts": 1, "status": f2_completed["run"]["status"]}
            report["outcome"] = "passed"
        except Exception as error:
            report["failure"] = {"type": type(error).__name__, "message": str(error)[:4000]}
        finally:
            for name, binary in (("old", old), ("new", new)):
                identities[name]["unchanged_after_verification"] = binary.exists() and sha256(binary.read_bytes()) == identities[name]["sha256"]
                if not identities[name]["unchanged_after_verification"]:
                    report["outcome"] = "failed"
                    report["failure"] = {"type": "BinaryChanged", "message": name + " executable changed during verification"}
            evidence_file.write(json_bytes(report))
        print(json.dumps({"outcome": report["outcome"], "evidence": str(evidence), "temporary_directory": str(temporary), "commands": len(report["commands"]), "failure": report.get("failure")}, indent=2))
        return 0 if report["outcome"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
