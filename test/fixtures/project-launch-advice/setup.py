#!/usr/bin/env python3
"""Create a disposable, local-only project through the public CLI."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--prifly", required=True, type=Path)
parser.add_argument("--destination", required=True, type=Path)
parser.add_argument("--variant", choices=["multiple", "renamed", "advice-off", "vague", "pages", "single", "empty", "finite"], default="multiple")
args = parser.parse_args()
binary = args.prifly.resolve(strict=True)
destination = args.destination.resolve()
# Refuse reuse rather than erase somebody's project or authority.
destination.mkdir(parents=True, exist_ok=False)
repository = destination / "repository"
authority = destination / "authority"
repository.mkdir()

def cli(*argv):
    result = subprocess.run([str(binary), *map(str, argv), "--json"], capture_output=True, text=True)
    if result.returncode:
        raise SystemExit(f"Pri-Fly refused {argv}: {result.stderr}")
    return json.loads(result.stdout)

def write(relative, text):
    path = repository / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")

cli("project", "init", "--repository", repository, "--state-root", authority,
    "--host", "codex-cli", "--host", "codex-app", "--host", "claude-code")
base = """authoring: prifly-project-workflow/1
package:
  id: test:package/{name}
  version: 1.0.0
  description: Local choice observation fixture.
  requires_core_protocol: "1"
  references: {{policy: core:policy/local@3.0.0}}
id: test:workflow/{name}
version: 1.0.0
refs: {{policy: "{{{{policy}}}}"{refs}}}
inputs: {inputs}
outputs: {{}}
entry: done
policy_ref: policy
limits: {{max_step_instances: 1, max_control_transitions: 1}}
stages:
  done: {{kind: finish, outcome: succeeded}}
"""
write(".prifly/workflows/alpha/workflow.yaml", base.format(name="alpha", refs=', request: "{{schema_request}}"', inputs='\n  request: {schema_ref: request, required: true}'))
write(".prifly/workflows/alpha/schemas/request.yaml", """id: test:schema/request
version: 1.0.0
type: object
required: [description, references]
additionalProperties: false
properties:
  description: {type: string, minLength: 1}
  references: {type: array, items: {type: string}}
""")
if args.variant == "finite":
    alpha = repository / ".prifly/workflows/alpha/workflow.yaml"
    text = alpha.read_text().replace('refs: {policy: "{{policy}}", request: "{{schema_request}}"}', 'refs: {policy: "{{policy}}", request: "{{schema_request}}", mode: "{{schema_mode}}"}')
    text = text.replace('  request: {schema_ref: request, required: true}', '  request: {schema_ref: request, required: true}\n  mode: {schema_ref: mode, required: true}')
    text = text.replace('entry: done', 'decision_catalog: [.prifly/workflows/alpha/decisions/mode.yaml]\nentry: done')
    alpha.write_text(text)
    write(".prifly/workflows/alpha/schemas/mode.yaml", "id: test:schema/mode\nversion: 1.0.0\ntype: string\nenum: [compact, detailed]\n")
    write(".prifly/workflows/alpha/decisions/mode.yaml", """authoring: prifly-run-decision/1
id: output_mode
title: Output detail
description: Choose the declared output detail for this request.
phase: preflight
choices:
  - {id: compact, title: Compact, value: compact}
  - {id: detailed, title: Detailed, value: detailed}
destination: {kind: launch_input, name: mode}
""")
write(".prifly/workflows/beta/workflow.yaml", base.format(name="beta", refs="", inputs='\n  document: {format: blob, media_types: [text/plain], required: true}'))
write(".prifly/workflows/diagnostics/workflow.yaml", base.format(name="diagnostics", refs="", inputs="{}"))
for name in ["alpha", "beta", "diagnostics"]:
    write(f".prifly/workflows/{name}/extend.yaml", "extensions: []\n")
ids = ["repair-fast", "review-deep", "diagnostics"]
if args.variant == "renamed":
    ids = ["review-deep", "repair-fast", "probe"]
entries = [
    (ids[0], "Compose a structured request", "Prepare a structured request from the original description and references.", "alpha"),
    (ids[1], "Inspect a supplied document", "Inspect a supplied text file; this workflow accepts only a document file.", "beta"),
    (ids[2], "Local diagnostics", "A technical diagnostic with no task or document input.", "diagnostics"),
]
if args.variant == "pages":
    entries += [(f"launch-{i:02}", f"Document option {i}", "Inspect an explicitly supplied text document.", "beta") for i in range(1, 13)]
if args.variant == "single":
    entries = entries[1:2]
if args.variant == "empty":
    entries = []
profile = """schema_version: prifly-project-profile/3
hosts:
  codex-cli: .codex/skills
  codex-app: .agents/skills
  claude-code: .claude/skills
packages:
  alpha: {source: .prifly/workflows/alpha}
  beta: {source: .prifly/workflows/beta}
  diagnostics: {source: .prifly/workflows/diagnostics}
launches:""" + ("\n" if entries else " {}\n")
for identity, title, description, package in entries:
    if args.variant == "vague":
        title, description = "Workflow", "Process the input."
    profile += f"  {identity}:\n    title: {json.dumps(title)}\n    description: {json.dumps(description)}\n    kind: workflow\n    workflow: .prifly/workflows/{package}/workflow.yaml\n"
write(".prifly/project.yaml", profile)
overlay = f"Do not recommend {ids[2]} by task; keep it selectable. Use descriptions, never ID words.\n"
if args.variant == "advice-off":
    overlay += "Do not offer launch recommendations. Show all choices directly.\n"
for directory in [".codex/skills/prifly-run", ".agents/skills/prifly-run", ".claude/skills/prifly-run"]:
    write(f"{directory}/PROJECT.md", overlay)
write("original-task.txt", "Prepare this request, preserving its description and URL: https://example.invalid/source#detail\n")
write("request.json", json.dumps({"description": (repository / "original-task.txt").read_text(), "references": ["https://example.invalid/source#detail"]}, ensure_ascii=False) + "\n")
write("document.txt", (repository / "original-task.txt").read_text())
listed = cli("project", "workflows", "--repository", repository)
# Read-only observations start from this public inventory; setup is not evidence
# that a host waited. Compare the same commands before and after the question.
inventory = {name: cli("--project", authority, *command) for name, command in {
    "packages": ["package", "list"], "claims": ["claim", "list"], "runs": ["run", "list"]}.items()}
metadata = {"repository": str(repository), "authority": str(authority), "binary": str(binary),
    "variant": args.variant, "launches": listed, "inventory_before_question": inventory,
    "runner_sha256": {str(p.relative_to(repository)): hashlib.sha256(p.read_bytes()).hexdigest()
                      for p in repository.glob(".*/skills/prifly-run/SKILL.md")}}
(destination / "baseline.json").write_text(json.dumps(metadata, indent=2, ensure_ascii=False) + "\n")
print(json.dumps(metadata, indent=2, ensure_ascii=False))
