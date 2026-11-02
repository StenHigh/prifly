#!/usr/bin/env python3
"""Write a self-contained unified-authoring example into an initialized project."""
import argparse
import json
from pathlib import Path

REFS = {"assisted": "core:adapter/assisted-session@1.0.0", "policy": "core:policy/local@4.0.0", "result": "core:schema/step-result@2.0.0", "carrier": "core:schema/context-json@1.0.0"}
LIMITS = {"max_step_instances": 64, "max_control_transitions": 256, "max_child_depth": 8, "max_parallelism": 4}


def write(root, path, value):
    target = root / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(value, indent=2) + "\n")


def package(name):
    return {"id": "unified:package/" + name, "version": "1.0.0", "description": "Generic composition and declared points example.", "requires_core_protocol": "1", "references": REFS}


def routes(success):
    return {verdict: success if verdict == "pass" else "rejected" for verdict in ("pass", "needs_revision", "blocked", "fail", "no_work")}


def worker(name, inputs=None, outputs=None):
    return {"authoring": "prifly-step/2", "id": "unified:step/" + name, "version": "1.0.0", "title": name, "kind": "worker", "repository_workspace": "read_only", "inputs": inputs or {}, "outputs": outputs or {}, "executor": {"adapter_ref": "{{assisted}}", "operation": "session"}, "instructions_ref": "{{context_instructions}}", "effects": {"class": "none", "retry_class": "pure"}, "result_schema_ref": "{{result}}"}


def workflow(name, stages, entry, inputs=None, outputs=None):
    return {"authoring": "prifly-workflow/1", "schema_version": "8", "id": "unified:workflow/" + name, "version": "1.0.0", "title": name, "inputs": inputs or {}, "outputs": outputs or {}, "allowed_outcomes": ["succeeded", "rejected"], "policy_ref": "{{policy}}", "limits": LIMITS, "entry": entry, "stages": stages}


def build(repository, operator="parallel"):
    base = repository / ".prifly/workflows"
    profile = {"schema_version": "prifly-project-profile/3", "packages": {name: {"source": ".prifly/workflows/" + name} for name in ("classic", "ui", "delivery")}, "launches": {"delivery": {"title": "Unified quality checks", "description": "UI and audit checks with an explicit nested carrier Call.", "kind": "workflow", "workflow": ".prifly/workflows/delivery/workflow.yaml", "workspace": "worktree"}}, "hosts": {"codex-app": ".agents/skills"}}
    write(repository, ".prifly/project.yaml", profile)
    port = {"schema_ref": "{{carrier}}"}
    output = {**port, "required_for": ["succeeded"]}
    for name in ("classic", "ui", "delivery"):
        write(base / name, "contexts/instructions.yaml", {"id": "unified:context/instructions", "version": "1.0.0", "media_type": "text/plain", "text": "Generic example. Preserve exact carrier and nested ArtifactRefs. A successful synthetic reply does not qualify a browser or publication."})
    classic = workflow("classic", {"produce": {"kind": "step", "step_ref": "{{step_produce}}", "on": routes("commit")}, "commit": {"kind": "step", "step_ref": "{{step_commit}}", "input_bindings": {"carrier": "$stages.produce.carrier"}, "on": routes("done")}, "done": {"kind": "finish", "outcome": "succeeded"}, "rejected": {"kind": "finish", "outcome": "rejected"}}, "produce")
    classic.update(authoring="prifly-project-workflow/1", package=package("classic"))
    write(base / "classic", "workflow.yaml", classic)
    write(base / "classic", "steps/produce.yaml", worker("produce", outputs={"carrier": {**port, "required_for": ["pass"]}}))
    write(base / "classic", "steps/commit.yaml", worker("commit", inputs={"carrier": port}))
    write(base / "classic", "extend.yaml", {"authoring": "prifly-extension/4", "extensions": []})
    write(base / "classic", "extension-points.yaml", {"authoring": "prifly-extension-points/1", "points": {"before-commit": {"workflow": "unified:workflow/classic", "between": {"from": "produce", "to": "commit"}, "input_bindings": {"carrier": "$stages.produce.carrier"}, "consumers": {}, "routes": {"step": {"on": routes("commit"), "on_error": "rejected"}, "call": {"on": {"succeeded": "commit", "rejected": "rejected"}, "on_error": "rejected"}}}}})
    library = workflow("ui-root", {"done": {"kind": "finish", "outcome": "succeeded"}}, "done")
    library.update(authoring="prifly-project-workflow/3", package=package("ui"), exports={"workflows": {"review": "{{workflow_ui-review}}"}, "points": {"before-read": {"workflow": "review", "point": "before-read-private"}}})
    write(base / "ui", "workflow.yaml", library)
    body = workflow("ui-body", {"read": {"kind": "step", "step_ref": "{{step_read}}", "input_bindings": {"carrier": "$inputs.carrier"}, "on": routes("done")}, "done": {"kind": "finish", "outcome": "succeeded", "output_bindings": {"carrier": "$inputs.carrier"}}, "rejected": {"kind": "finish", "outcome": "rejected"}}, "read", {"carrier": port}, {"carrier": output})
    write(base / "ui", "workflows/ui-body.yaml", body)
    public = workflow("ui-review", {"inspect": {"kind": "call", "workflow_ref": "{{workflow_ui-body}}", "input_bindings": {"carrier": "$inputs.carrier"}, "on": {"succeeded": "done", "rejected": "rejected"}}, "done": {"kind": "finish", "outcome": "succeeded", "output_bindings": {"carrier": "$stages.inspect.carrier"}}, "rejected": {"kind": "finish", "outcome": "rejected"}}, "inspect", {"carrier": port}, {"carrier": output})
    write(base / "ui", "workflows/ui-review.yaml", public)
    write(base / "ui", "steps/read.yaml", worker("read", {"carrier": port}))
    write(base / "ui", "extension-points.yaml", {"authoring": "prifly-extension-points/1", "points": {"before-read-private": {"workflow": "unified:workflow/ui-body", "entry": "read", "input_bindings": {"carrier": "$inputs.carrier"}, "consumers": {"reader": {"stage": "read", "input": "carrier"}, "finish": {"stage": "done", "output": "carrier"}}, "routes": {"step": {"on": routes("read"), "on_error": "rejected"}, "call": {"on": {"succeeded": "read", "rejected": "rejected"}, "on_error": "rejected"}}}}})
    write(base / "delivery", "workflow.yaml", {"authoring": "prifly-project-workflow/3", "package": package("delivery"), "id": "unified:workflow/delivery", "version": "1.0.0", "title": "Quality composition", "extends": "classic", "limits": LIMITS})
    write(base / "delivery", "project/steps/audit.yaml", worker("audit", {"carrier": port}))
    write(base / "delivery", "project/steps/measure.yaml", worker("measure", {"carrier": port}, {"carrier": {**port, "required_for": ["pass"]}}))
    typed = workflow("measurement", {"measure": {"kind": "step", "step_ref": "{{step_measure}}", "input_bindings": {"carrier": "$inputs.carrier"}, "on": routes("done")}, "done": {"kind": "finish", "outcome": "succeeded", "output_bindings": {"carrier": "$stages.measure.carrier"}}, "rejected": {"kind": "finish", "outcome": "rejected"}}, "measure", {"carrier": port}, {"carrier": output})
    write(base / "delivery", "project/workflows/measurement.yaml", typed)
    members = ["delivery.interface-review", "delivery.audit"]
    group = {"id": "quality", "point": {"parent": "classic", "name": "before-commit"}}
    if operator == "choice":
        group[operator] = {"selection": "first_match", "branches": [{"predicate": True, "use": members[0]}], "default": members[1], "on_unknown": "rejected"}
    else:
        group[operator] = members
    write(base / "delivery", "extend.yaml", {"authoring": "prifly-extension/5", "imports": {"ui": "ui"}, "extensions": [{"id": "interface-review", "point": "before-commit", "call": "ui.review", "output_bindings": {}}, {"id": "audit", "point": {"parent": "classic", "name": "before-commit"}, "step": "audit", "output_bindings": {}}, {"id": "measure-before-read", "point": {"import": "ui", "export": "before-read"}, "call": "measurement", "output_bindings": {"reader": "carrier", "finish": "carrier"}}], "arrange": [group]})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository", type=Path, required=True)
    parser.add_argument("--operator", choices=["sequence", "parallel", "choice"], default="parallel")
    args = parser.parse_args()
    build(args.repository.resolve(strict=True), args.operator)


if __name__ == "__main__":
    main()
