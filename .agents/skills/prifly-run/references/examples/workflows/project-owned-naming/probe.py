#!/usr/bin/env python3
"""Replay issue #19 through one exact Core executable, without an AI host."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import re
import subprocess
import sys
import tempfile


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def git(root, *args):
    return subprocess.run(["git", "-C", str(root), "-c", "user.name=Naming probe",
                           "-c", "user.email=probe@example.invalid", "-c", "commit.gpgsign=false", *args],
                          check=True, capture_output=True, text=True, timeout=60).stdout.strip()


class Probe:
    def __init__(self, binary, root):
        self.binary = binary
        self.root = root
        self.repository = root / "repository"
        self.authority = root / "authority"
        self.calls = []
        self.runs = []
        self.repository.mkdir()
        git(self.repository, "init", "-q", "-b", "main")
        self.call("project", "init", "--repository", self.repository,
                  "--state-root", self.authority, "--host", "codex-cli")

    def call(self, *args, refusal=None):
        command = [str(self.binary), "--json", *map(str, args)]
        result = subprocess.run(command, capture_output=True, text=True, timeout=180)
        self.calls.append({"arguments": list(map(str, args)), "exit": result.returncode})
        if refusal:
            assert result.returncode != 0, result.stdout
            problem = json.loads(result.stderr.splitlines()[-1])
            assert problem["code"] == refusal, result.stderr
            return problem
        assert result.returncode == 0, (command, result.stderr)
        return json.loads(result.stdout)

    def authority_call(self, *args, **kwargs):
        return self.call("--project", self.authority, *args, **kwargs)

    def configure(self, launch, package=None):
        package = package or launch
        path = self.repository / ".prifly/project.yaml"
        if launch == "named":
            profile = {"schema_version": "prifly-project-profile/5",
                       "hosts": {"codex-cli": ".codex/skills"},
                       "packages": {package: {"source": ".prifly/workflows/" + package}},
                       "launches": {launch: {"title": "Naming probe", "description": "Closed task input",
                                             "kind": "workflow", "workflow": ".prifly/workflows/" + package + "/workflow.yaml",
                                             "workspace": "worktree"}}}
            path.write_text(json.dumps(profile))
        else:
            text = path.read_text().replace("prifly-project-profile/3", "prifly-project-profile/5")
            path.write_text(text)
        text = path.read_text()
        # JSON is also YAML; use a YAML block only with a YAML profile.
        naming = {"template": "demo-{source}-{id}",
                  "inputs": {"identity": {"schema": ".prifly/naming/identity.schema.json"}},
                  "bindings": {"source": {"naming_input": "identity", "pointer": "/source"},
                               "id": {"naming_input": "identity", "pointer": "/id"}}}
        if text.lstrip().startswith("{"):
            profile = json.loads(text)
            profile["worktree_naming"] = naming
            path.write_text(json.dumps(profile))
        else:
            path.write_text(text + "\nworktree_naming: " + json.dumps(naming) + "\n")
        schema = self.repository / ".prifly/naming/identity.schema.json"
        schema.parent.mkdir()
        schema.write_text(json.dumps({"$schema": "https://json-schema.org/draft/2020-12/schema",
                                      "type": "object", "required": ["source", "id"],
                                      "properties": {"source": {"type": "string"}, "id": {"type": "string"}, "note": {"type": "string"}},
                                      "additionalProperties": False}))
        self.naming = self.repository / "identity.json"
        self.naming.write_text('{"source":"tracker","id":"issue-19","note":"original"}')
        task = self.repository / "task.json"
        task.write_text(json.dumps({"title": "Prose mentions issue 900", "description": "The naming ID comes only from identity.json.", "reference": "https://example.invalid/900"}))
        self.args = ["--repository", self.repository, "--launch", launch, "--host", "codex-cli", "--workspace", "worktree", "--input", "task=" + str(task)]
        self.naming_args = ["--naming-input", "identity=" + str(self.naming)]
        git(self.repository, "add", ".")
        git(self.repository, "commit", "-qm", "consumer naming configuration")

    def prepare(self, args=None):
        return self.call("project", "questionnaire", "--prepare", *(args or self.args + self.naming_args))

    def start(self):
        review = self.prepare()
        result = self.call("project", "start", *self.args, *self.naming_args,
                           "--expected-launch-digest", review["review_digest"], "--command-id", "probe:start")
        self.runs.append(result["run_id"])
        assert result["schema_version"] == "project-start/8"
        assert result["workspace"]["path"] == ".prifly/work/claims/demo-tracker-issue-19", result
        assert "identity" not in result["run"]["run"]["input_artifacts"]
        return result

    def submit(self, run, verdict):
        task = self.authority_call("session", "task", "--run", run)
        self.authority_call("session", "take", "--run", run, "--attempt", task["attempt_id"])
        submission = {"schema_version": task["schema_version"], "run_id": run,
                      "attempt_id": task["attempt_id"], "envelope_digest": task["envelope_digest"],
                      "result": {"schema_version": "1", "run_id": run, "step_instance_id": task["step_instance_id"],
                                 "attempt_id": task["attempt_id"], "envelope_digest": task["envelope_digest"],
                                 "verdict": verdict, "outputs": {}, "evidence_refs": [], "effect_receipt_refs": [],
                                 "summary": "Synthetic naming contract probe; no product work asserted."}}
        if task.get("question_report") == "required":
            submission["answered_questions"] = []
        path = self.root / "submission.json"
        path.write_text(json.dumps(submission))
        self.authority_call("session", "submit", "--file", path)
        self.authority_call("run", "next", run)
        return self.authority_call("run", "drive", run)

    def cleanup(self):
        for run in self.runs:
            view = self.authority_call("run", "status", run)
            if view["run"]["status"] not in ("completed", "cancelled"):
                self.authority_call("run", "cancel", run, "--reason", "Naming probe cleanup", "--command-id", "cleanup:" + run)
                self.authority_call("run", "drive", run)
        claims = self.authority_call("claim", "list")
        for claim in claims["claims"]:
            if claim["status"] == "active":
                self.authority_call("claim", "release", "--id", claim["id"], "--generation", claim["generation"], "--command-id", "cleanup:" + claim["id"])


def generic(probe):
    source = Path(__file__).resolve().parents[1] / "named-worktree"
    folder = probe.repository / ".prifly/workflows/named"
    shutil.copytree(source, folder)
    # Synthetic upstream deliberately has the same closed task surface as the issue.
    (folder / "schemas/task.yaml").write_text('''id: example:schema/task
version: 1.0.0
type: object
additionalProperties: false
required: [title, description]
properties:
  title: {type: string}
  description: {type: string}
  reference: {type: string}
''')
    workflow = folder / "workflow.yaml"
    text = workflow.read_text().replace("entry: work", "entry: prepare").replace("max_step_instances: 1, max_control_transitions: 2", "max_step_instances: 2, max_control_transitions: 3")
    text = text.replace("stages:\n", "stages:\n  prepare:\n    kind: step\n    step_ref: work\n    input_bindings: {task: $inputs.task}\n    on: {pass: work}\n    impossible_verdicts: [fail, needs_revision, no_work, blocked]\n", 1)
    workflow.write_text(text)
    upstream = {str(p.relative_to(folder)): digest(p.read_bytes()) for p in folder.rglob("*") if p.is_file()}
    probe.configure("named")
    before = probe.authority_call("claim", "list")
    probe.call("project", "questionnaire", "--prepare", *probe.args, refusal="project_worktree_input_missing")
    original = probe.prepare()
    probe.naming.write_text('{"source":"tracker","id":"issue-19","note":"changed"}')
    probe.call("project", "start", *probe.args, *probe.naming_args,
               "--expected-launch-digest", original["review_digest"], refusal="project_start_stale_launch")
    probe.naming.write_text('{"source":"tracker","id":19}')
    probe.call("project", "questionnaire", "--prepare", *probe.args, *probe.naming_args, refusal="project_worktree_input_invalid")
    assert probe.authority_call("claim", "list") == before
    probe.naming.write_text('{"source":"tracker","id":"issue-19","note":"original"}')
    result = probe.start()
    snapshot = result["workspace"]["naming_snapshot"]
    probe.submit(result["run_id"], "pass")
    probe.submit(result["run_id"], "no_work")
    profile_path = probe.repository / ".prifly/project.yaml"
    profile = json.loads(profile_path.read_text())
    del profile["worktree_naming"]
    profile_path.write_text(json.dumps(profile))
    probe.naming.unlink()
    (probe.repository / ".prifly/naming/identity.schema.json").unlink()
    args = ["--repository", probe.repository, "--launch", "named", "--host", "codex-cli", "--source-run", result["run_id"]]
    review = probe.call("project", "continue", "--prepare", *args)
    resumed = probe.call("project", "continue", *args, "--expected-launch-digest", review["review_digest"], "--command-id", "probe:resume")
    probe.runs.append(resumed["run_id"])
    assert resumed["workspace"]["naming_snapshot"] == snapshot
    assert resumed["workspace"]["path"] == result["workspace"]["path"]
    assert resumed["workspace"]["generation"] > result["workspace"]["generation"]
    probe.submit(resumed["run_id"], "pass")
    assert upstream == {str(p.relative_to(folder)): digest(p.read_bytes()) for p in folder.rglob("*") if p.is_file()}
    return {"upstream": upstream, "claim": result["workspace"]["path"], "resumed_claim": resumed["workspace"]["path"], "snapshot_preserved": True}


def official(probe):
    checkout = probe.root / "official-source"
    subprocess.run(["git", "clone", "--depth", "1", "--branch", "v1.57.0",
                    "https://github.com/StenHigh/prifly-aif-workflows.git", str(checkout)],
                   check=True, capture_output=True, text=True, timeout=180)
    assert git(checkout, "rev-parse", "HEAD") == "ee76702dec0d8e0958ef068ef58074ecf2db37e9"
    installation = subprocess.run([sys.executable, str(checkout / "tools/install_composition_contracts.py"),
                                   "--binary", str(probe.binary), "--project", str(probe.authority)],
                                  check=True, capture_output=True, text=True, timeout=180)
    dependency = json.loads(installation.stdout)
    installed = probe.call("project", "workflows", "add", "https://github.com/StenHigh/prifly-aif-workflows.git",
                           "--path", "aif-classic", "--name", "aif-classic", "--ref", "v1.57.0", "--repository", probe.repository)
    folder = probe.repository / ".prifly/workflows/aif-classic"
    upstream = {str(p.relative_to(folder)): digest(p.read_bytes()) for p in folder.rglob("*") if p.is_file() and p.relative_to(folder).parts[0] not in ("extend.yaml", "project")}
    # Inert host skill placeholders permit compilation only. No AI skill runs.
    for context in (folder / "contexts").rglob("*.yaml"):
        for relative in re.findall(r"root: host_skills, path: ([^}\s]+)", context.read_text()):
            target = probe.repository / ".codex/skills" / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text("# Synthetic compile-only host skill\n")
    extend = folder / "extend.yaml"
    extend.write_text(json.dumps({"extensions": [], "profile": "full", "answers": {"decision_policy": "attended", "preflight": {"plan_tests": True, "plan_docs": False, "plan_logging": "minimal", "plan_constraints": "", "gate_warnings": "ignore", "gate_checks": "true"}}}))
    local = folder / "project/consumer.txt"
    local.parent.mkdir(exist_ok=True)
    local.write_text("Consumer-owned naming journey.\n")
    launches = probe.call("project", "workflows", "--repository", probe.repository)
    # Install creates one launch; use its declared ID from the public result.
    launch = launches["launches"][0]["id"]
    probe.configure(launch, "aif-classic")
    configuration = {str(p): p.read_bytes() for p in (extend, local, probe.repository / ".prifly/project.yaml", probe.repository / ".prifly/naming/identity.schema.json")}
    updated = probe.call("project", "workflows", "update", "aif-classic", "--repository", probe.repository)
    assert all(p.read_bytes() == data for name, data in configuration.items() for p in [Path(name)])
    assert upstream == {str(p.relative_to(folder)): digest(p.read_bytes()) for p in folder.rglob("*") if p.is_file() and p.relative_to(folder).parts[0] not in ("extend.yaml", "project")}
    result = probe.start()
    return {"dependency": dependency, "source_commit": git(checkout,"rev-parse","HEAD"), "install": installed, "update": updated, "upstream": upstream, "claim": result["workspace"]["path"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="exact source-built or released executable")
    parser.add_argument("--official", action="store_true", help="also fetch pinned aif-classic v1.57.0 and exercise install/update/prepare/start")
    parser.add_argument("--evidence", type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    report = {"binary": str(binary), "binary_digest": digest(binary.read_bytes()), "released_acceptance": False}
    for name, journey in [("generic", generic)] + ([("official", official)] if args.official else []):
        with tempfile.TemporaryDirectory(prefix="prifly-naming-19-") as temporary:
            probe = Probe(binary, Path(temporary))
            try:
                report[name] = journey(probe)
            finally:
                probe.cleanup()
            report[name]["commands"] = probe.calls
    report["version"] = json.loads(subprocess.check_output([str(binary), "version", "--json"], text=True, timeout=30))
    output = json.dumps(report, indent=2, ensure_ascii=False) + "\n"
    if args.evidence:
        args.evidence.write_text(output)
    print(output)


if __name__ == "__main__":
    main()
