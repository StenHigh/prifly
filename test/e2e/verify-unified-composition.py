#!/usr/bin/env python3
"""Unified composition/point transport. Synthetic replies; native engine and real ArtifactRefs."""
import argparse
import hashlib
import importlib.util
import json
import shutil
from pathlib import Path

spec = importlib.util.spec_from_file_location("point_transport", Path(__file__).with_name("verify-extension-points.py"))
transport = importlib.util.module_from_spec(spec)
spec.loader.exec_module(transport)


def mixed_setup(repository, folder):
    provider = folder.parent / "provider"
    shutil.copytree(folder, provider)
    # Private standalone choices must not execute through a library import.
    for source in (folder, provider):
        for path in source.rglob("*.yaml"):
            path.write_text(path.read_text().replace("{{schema_carrier}}", "{{any_carrier}}"))
        path = source / "workflow.yaml"
        text = path.read_text().replace("prifly-project-workflow/1", "prifly-project-workflow/3")
        text = text.replace("    adapter:", "    any_carrier: core:schema/context-json@1.0.0\n    adapter:")
        path.write_text(text)
    path = provider / "workflow.yaml"
    path.write_text(path.read_text() + """
exports:
  workflows:
    review: "{{workflow_round}}"
  points:
    before-read: {workflow: review, point: before-read}
    after-repair: {workflow: review, point: after-repair}
""")
    path = folder / "workflow.yaml"
    text = path.read_text().replace('body_workflow_ref: "{{workflow_round}}"', "body_workflow_ref: imported_round")
    path.write_text(text.replace("inputs: {}", "refs: {imported_round: a.review}\ninputs: {}", 1))
    path = folder / "extend.yaml"
    text = path.read_text().replace("prifly-extension/5", "prifly-extension/5\nimports: {a: provider}")
    for point in ("before-read", "after-repair"):
        text = text.replace("point: " + point, "point: {import: a, export: " + point + "}")
    path.write_text(text)
    profile = repository / ".prifly/project.yaml"
    profile.write_text(profile.read_text().replace("  transport: {source:", "  provider: {source: .prifly/workflows/provider}\n  transport: {source:", 1))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--cases", nargs="+", choices=transport.CASES, default=transport.CASES)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    result = {"binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "authoring": "prifly-extension/5", "boundary": __doc__, "cases": [], "complete": False}
    for case in args.cases:
        result["cases"].append(transport.run_case(binary, case, "prifly-extension/5", mixed_setup))
        print(case, "PASS", flush=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
    result["complete"] = True
    args.output.write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
