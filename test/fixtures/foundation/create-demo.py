#!/usr/bin/env python3
"""Create explicit local examples using the installed binary's own ref API.

No dependencies are downloaded. The target must be absent or empty. The demo
uses only local scratch files; no credentials, provider calls or Git are needed.
"""

import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    target = args.target.resolve()
    if target.exists() and (not target.is_dir() or any(target.iterdir())):
        parser.error("target must be an empty directory; no existing files were changed")

    def call(*command, parse=True):
        result = subprocess.run([str(binary), *map(str, command)], check=True, capture_output=True, text=True)
        return json.loads(result.stdout) if parse else result.stdout

    call("init", target, parse=False)
    inventory = call("--project", target, "inventory", "--json")
    if isinstance(inventory, dict):
        inventory = inventory["definitions"]
    builtins = {entry["ref"]["id"]: entry["ref"] for entry in inventory}
    config_path = target / "prifly.json"
    config = json.loads(config_path.read_text())
    entries = []

    def write(path, value):
        destination = target / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("x", encoding="utf-8") as stream:
            json.dump(value, stream, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False)
            stream.write("\n")
        return destination

    def define(kind, path, identifier, value):
        destination = write(path, value)
        response = call("ref", destination, "--id", identifier, "--version", "1.0.0")
        ref = response.get("ref", response)
        if set(ref) != {"id", "version", "digest"}:
            raise ValueError("ref command returned an unexpected contract")
        entries.append({"ref": ref, "kind": kind, "path": path})
        return ref

    def schema(path, identifier, properties, required):
        return define("schema", path, identifier, {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": properties, "required": required, "additionalProperties": False})

    document = schema("schemas/document.json", "demo:schema/document", {"key": {"type": "string", "minLength": 1}, "text": {"type": "string", "maxLength": 100000}}, ["key", "text"])
    report = schema("schemas/report.json", "demo:schema/report", {"ok": {"type": "boolean"}, "notes": {"type": "array", "items": {"type": "string"}, "maxItems": 16}}, ["ok", "notes"])
    transform_report = schema("schemas/transform-report.json", "demo:schema/transform-report", {"bytes": {"type": "integer", "minimum": 0}, "changed": {"type": "boolean"}}, ["bytes", "changed"])
    progress = schema("schemas/progress.json", "demo:schema/progress", {"phase": {"type": "string", "enum": ["working", "finished"]}, "completed": {"type": "integer", "minimum": 0, "maximum": 1000}}, ["phase", "completed"])
    warning = schema("schemas/warning.json", "demo:schema/warning", {"reason": {"type": "string", "enum": ["empty_document", "short_document", "unchanged_text"]}}, ["reason"])

    def port(ref=None, *, required=None, required_for=None):
        value = {"format": "json", "schema_ref": ref} if ref else {"format": "blob", "media_types": ["text/plain"]}
        if required is not None:
            value["required"] = required
        if required_for is not None:
            value["required_for"] = required_for
        return value

    def step(identifier, inputs, outputs, hooks=True):
        value = {"schema_version": "2" if hooks else "1", "id": identifier, "version": "1.0.0", "title": identifier, "kind": "command", "inputs": inputs, "outputs": outputs,
                 "executor": {"adapter_ref": config["adapter_bindings"]["local_process"], "operation": "process"}, "context_refs": [], "required_capabilities": [],
                 "effects": {"class": "workspace_write" if outputs else "none", "retry_class": "pure"}, "result_check_refs": [], "result_schema_ref": builtins["core:schema/step-result"]}
        if hooks:
            value["hooks"] = {
                "progress_changed": {"kind": "state", "schema_ref": progress, "description": "Current bounded progress", "classification": "internal", "read_policy": "owner", "max_payload_bytes": 1024, "max_count": 20, "max_per_minute": 60, "allow_during_stop": True, "freshness_ms": 30000},
                "warning_raised": {"kind": "event", "schema_ref": warning, "description": "A structured diagnostic occurrence", "classification": "internal", "read_policy": "owner", "max_payload_bytes": 1024, "max_count": 20, "max_per_minute": 60, "allow_during_stop": False},
            }
            value["telemetry"] = [
                {"name": "processed_total", "revision": "1.0.0", "description": "Completed items in this Attempt", "hook": "progress_changed", "kind": "counter", "field": "/completed", "unit": "1", "aggregation": "delta", "reset": "attempt", "minimum": 0, "maximum": 1000, "dimensions": {"phase": "/phase"}},
                {"name": "quality_warnings", "revision": "1.0.0", "description": "Declared quality warnings", "hook": "warning_raised", "kind": "diagnostic", "aggregation": "occurrences", "reset": "none", "severity": "warn", "code": "quality_warning", "message": "A declared quality warning was reported", "dimensions": {"reason": "/reason"}},
            ]
        return define("step", "steps/" + identifier.split("/")[-1] + ".json", identifier, value)

    transform = step("demo:step/transform", {"source": port(required=True)}, {"text": port(required_for=["pass"]), "report": port(transform_report, required_for=["pass"])})
    check = step("demo:step/check", {"document": port(document, required=True)}, {"report": port(report, required_for=["pass", "fail"])})
    shell = step("demo:step/shell", {}, {}, hooks=False)

    def source(stage, name):
        return {"from": "stage_output", "stage_id": stage, "port": name}

    def finish(outcome, bindings):
        return {"kind": "finish", "outcome": outcome, "output_bindings": bindings}

    def workflow(name, inputs, outputs, entry, stages, outcomes, count):
        identifier = "demo:workflow/" + name
        return define("workflow", "workflows/" + name + ".json", identifier, {
            "schema_version": "1", "id": identifier, "version": "1.0.0", "title": name,
            "inputs": inputs, "outputs": outputs, "allowed_outcomes": outcomes, "definition": {"entry": entry, "stages": stages},
            "limits": {"max_step_instances": count, "max_control_transitions": 16, "max_parallelism": 1, "max_child_depth": 0}, "policy_ref": config["default_policy_ref"],
        })

    workflow("transform", {"source": port(required=True)}, {"text": port(required_for=["succeeded"]), "report": port(transform_report, required_for=["succeeded"])}, "transform", {
        "transform": {"kind": "step", "step_ref": transform, "input_bindings": {"source": {"from": "workflow_input", "port": "source"}}, "on": {"pass": "done"}},
        "done": finish("succeeded", {"text": source("transform", "text"), "report": source("transform", "report")}),
    }, ["succeeded"], 1)
    workflow("two-checks", {"first": port(document, required=True), "second": port(document, required=True)}, {"report_first": port(report, required_for=["succeeded", "rejected"]), "report_second": port(report, required_for=["succeeded"])}, "check_first", {
        "check_first": {"kind": "step", "step_ref": check, "input_bindings": {"document": {"from": "workflow_input", "port": "first"}}, "on": {"pass": "check_second", "fail": "rejected_first"}},
        "check_second": {"kind": "step", "step_ref": check, "input_bindings": {"document": {"from": "workflow_input", "port": "second"}}, "on": {"pass": "done", "fail": "rejected_second"}},
        "done": finish("succeeded", {"report_first": source("check_first", "report"), "report_second": source("check_second", "report")}),
        "rejected_first": finish("rejected", {"report_first": source("check_first", "report")}),
        "rejected_second": finish("rejected", {"report_first": source("check_first", "report"), "report_second": source("check_second", "report")}),
    }, ["succeeded", "rejected"], 2)
    workflow("shell", {}, {}, "shell", {"shell": {"kind": "step", "step_ref": shell, "input_bindings": {}, "on": {"pass": "done"}}, "done": finish("succeeded", {})}, ["succeeded"], 1)

    source_dir = Path(__file__).resolve().parent
    (target / "scripts").mkdir()
    for filename in ("prifly_step.py", "demo-step.py", "empty-step.sh"):
        shutil.copyfile(source_dir / filename, target / "scripts" / filename)
    python = str(Path(sys.executable).resolve())
    config["configuration"]["executors"] = {
        identifier: {"executable": python if mode != "shell" else "/bin/sh", "args": ["demo-step.py", mode] if mode != "shell" else ["empty-step.sh"],
                     "files": {"prifly_step.py": "scripts/prifly_step.py", "demo-step.py": "scripts/demo-step.py"} if mode != "shell" else {"empty-step.sh": "scripts/empty-step.sh"},
                     "environment": {}, "timeout_ms": 30000, "grace_ms": 1000, "max_output_bytes": 1048576}
        for identifier, mode in (("demo:step/transform", "transform"), ("demo:step/check", "check"), ("demo:step/shell", "shell"))
    }
    config_path.write_text(json.dumps(config, ensure_ascii=False, indent=2) + "\n")
    (target / "definitions.json").write_text(json.dumps({"schema_version": "1", "entries": entries}, ensure_ascii=False, indent=2) + "\n")
    write("brief.json", {"schema_version": "1", "id": "demo:brief/local", "subject": "Local deterministic example", "desired_outcome": "Produce the declared local reports", "in_scope": ["Scratch input/output files"], "out_of_scope": ["Network, AI, external writes"], "completion_criteria": ["Declared outputs pass their schemas"], "source_refs": [], "assumptions": [], "confirmation": "explicit"})
    write("inputs/first.json", {"key": "first", "text": "First complete document."})
    write("inputs/second.json", {"key": "second", "text": "Second complete document."})
    write("inputs/rejected.json", {"key": "rejected", "text": ""})
    (target / "inputs/source.txt").write_text("Hello, Pri-Fly!\n", encoding="utf-8")
    (target / "inputs/unchanged.txt").write_text("ALREADY UPPERCASE\n", encoding="utf-8")
    print(json.dumps({"project": str(target), "workflows": ["workflows/transform.json", "workflows/two-checks.json", "workflows/shell.json"], "runtime_execution": "not_started"}, indent=2))


if __name__ == "__main__":
    main()
