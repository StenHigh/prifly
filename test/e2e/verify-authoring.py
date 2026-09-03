#!/usr/bin/env python3
"""Check project YAML authoring fixtures through the public Pri-Fly CLI."""

import argparse
import json
from pathlib import Path
import shutil
import subprocess
import tempfile


CORPUS = Path(__file__).resolve().parents[1] / "fixtures" / "project-authoring"
LAUNCH_CORPUS = Path(__file__).resolve().parents[1] / "fixtures" / "project-launch"


def run(binary, *arguments):
    return subprocess.run([str(binary), "--json", *map(str, arguments)], capture_output=True, text=True, timeout=30)


def launch_fixture_repository(binary, source, root):
    repository = root / "repository"
    authority = root / "authority"
    subprocess.run(["git", "init", "-q", str(repository)], check=True, timeout=30)
    initialized = run(binary, "project", "init", "--repository", repository, "--state-root", authority)
    assert initialized.returncode == 0, initialized.stderr
    shutil.copytree(source / "repository", repository, dirs_exist_ok=True)
    subprocess.run(["git", "-C", str(repository), "add", "."], check=True, timeout=30)
    subprocess.run(
        ["git", "-C", str(repository), "-c", "user.name=Pri-Fly fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"],
        check=True,
        timeout=30,
    )
    brief = root / "brief.json"
    brief.write_text(json.dumps({
        "schema_version": "1",
        "id": "test:brief/workspace",
        "subject": "Workspace fixture",
        "desired_outcome": "Reach the declared host handoff.",
        "in_scope": ["handoff"],
        "out_of_scope": [],
        "completion_criteria": ["handoff"],
        "source_refs": [],
        "assumptions": [],
        "confirmation": "explicit",
    }))
    return repository, authority, brief


def check_launch_case(binary, source):
    expectation = json.loads((source / "expect.json").read_text())
    for mode in expectation["modes"]:
        with tempfile.TemporaryDirectory(prefix="prifly-project-launch-", dir="/tmp") as temporary:
            repository, authority, brief = launch_fixture_repository(binary, source, Path(temporary))
            result = run(binary, "--project", authority, "project", "start", "--repository", repository, "--launch", expectation["launch"], "--host", expectation["host"], "--brief", brief, "--workspace", mode)
            assert result.returncode == 0, result.stderr
            launch = json.loads(result.stdout)
            assert launch["schema_version"] == "project-start/1", launch
            assert launch["workspace"]["mode"] == mode, launch
            assert launch["run"]["run"]["id"], launch
    if expectation.get("negative") == "checkout_dirty":
        with tempfile.TemporaryDirectory(prefix="prifly-project-launch-", dir="/tmp") as temporary:
            repository, authority, brief = launch_fixture_repository(binary, source, Path(temporary))
            (repository / "dirty.txt").write_text("not committed\n")
            result = run(binary, "--project", authority, "project", "start", "--repository", repository, "--launch", expectation["launch"], "--host", expectation["host"], "--brief", brief, "--workspace", "checkout")
            assert result.returncode != 0, result.stdout
            assert "checkout_dirty" in result.stderr, result.stderr


def check_case(binary, source):
    expectation = json.loads((source / "expect.json").read_text())
    with tempfile.TemporaryDirectory(prefix="prifly-authoring-", dir="/tmp") as temporary:
        root = Path(temporary)
        repository = root / "repository"
        shutil.copytree(source / "repository", repository)
        subprocess.run(["git", "init", "-q", str(repository)], check=True, timeout=30)
        command = expectation["command"]
        if expectation["accepted"]:
            authority = root / "authority"
            initialized = run(binary, "init", "--profile", "core-workflow/1", authority)
            assert initialized.returncode == 0, initialized.stderr
            listed = run(binary, "--project", authority, "project", "workflows", "--repository", repository)
            assert listed.returncode == 0, listed.stderr
            launches = json.loads(listed.stdout)["launches"]
            assert [launch["id"] for launch in launches] == [expectation["launch"]], launches
            assert launches[0]["inputs"] == expectation.get("inputs", [{"name": "task", "required": True, "format": "json"}]), launches
            before = run(binary, "--project", authority, "package", "list")
            output = root / "sealed"
            compiled = run(binary, "--project", authority, "project", "compile", "--repository", repository, "--package", expectation["package"], "--host", "codex-cli", "--output", output)
            assert compiled.returncode == 0, compiled.stderr
            assert (output / "prifly.package.json").is_file()
            if options := expectation.get("workflow_options"):
                result = json.loads(compiled.stdout)
                component = next(item for item in result["components"] if item["kind"] == "workflow" and item["ref"]["id"] == options["workflow"])
                workflow = json.loads((output / component["path"]).read_text())
                assert "features" not in workflow, workflow
                inputs = workflow["inputs"]
                for name, expected in options["defaults"].items():
                    assert inputs[name]["configuration"]["default"] == expected, inputs
            after = run(binary, "--project", authority, "package", "list")
            assert before.stdout == after.stdout, "project compile changed the authority"
            return
        prefix = []
        if expectation.get("authority"):
            authority = root / "authority"
            initialized = run(binary, "init", "--profile", "core-workflow/1", authority)
            assert initialized.returncode == 0, initialized.stderr
            prefix = ["--project", authority]
        if command == "workflows":
            result = run(binary, *prefix, "project", "workflows", "--repository", repository)
            output = None
        else:
            output = root / "sealed"
            result = run(binary, *prefix, "project", "compile", "--repository", repository, "--package", expectation["package"], "--host", "codex-cli", "--output", output)
        assert result.returncode != 0, f"{source.name} was accepted: {result.stdout}"
        assert expectation["diagnostic"] in result.stderr, result.stderr
        if output is not None:
            assert not output.exists(), f"{source.name} left a sealed package"


def git(*arguments):
    subprocess.run(
        ["git", "-c", "user.name=Pri-Fly fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", *map(str, arguments)],
        check=True,
        capture_output=True,
        timeout=30,
    )


def check_workflow_catalog(binary):
    """Install the accepted corpus folder through a local workflow repository and catalog; no network."""
    fixture = CORPUS / "accept-workflow-folder" / "repository"
    with tempfile.TemporaryDirectory(prefix="prifly-workflow-catalog-", dir="/tmp") as temporary:
        root = Path(temporary)
        source = root / "source"
        source.mkdir()
        git("-C", source, "init", "-q", "-b", "main")
        shutil.copytree(fixture / ".prifly" / "workflows" / "sample", source / "flows" / "sample")
        git("-C", source, "add", ".")
        git("-C", source, "commit", "-qm", "workflow")
        git("-C", source, "tag", "v1")
        commit = subprocess.run(["git", "-C", str(source), "rev-parse", "HEAD"], check=True, capture_output=True, text=True, timeout=30).stdout.strip()
        catalog = root / "catalog"
        catalog.mkdir()
        git("-C", catalog, "init", "-q", "-b", "main")
        (catalog / "catalog.yaml").write_text(
            "schema_version: prifly-workflow-catalog/1\n"
            "categories:\n"
            "  samples: {title: Samples}\n"
            "workflows:\n"
            "  sample:\n"
            "    title: Sample workflow\n"
            "    description: Fixture folder from the authoring corpus.\n"
            "    category: samples\n"
            f"    repository: {source}\n"
            "    path: flows/sample\n"
            "    ref: v1\n"
            f"    commit: {commit}\n"
        )
        git("-C", catalog, "add", ".")
        git("-C", catalog, "commit", "-qm", "catalog")
        repository = root / "repository"
        authority = root / "authority"
        git("init", "-q", "-b", "main", repository)
        initialized = run(binary, "project", "init", "--repository", repository, "--state-root", authority)
        assert initialized.returncode == 0, initialized.stderr
        shutil.copytree(fixture / ".codex", repository / ".codex", dirs_exist_ok=True)
        searched = run(binary, "project", "workflows", "search", "sample", "--catalog", catalog)
        assert searched.returncode == 0, searched.stderr
        listing = json.loads(searched.stdout)
        assert listing["schema_version"] == "project-workflow-catalog/1", listing
        assert [entry["name"] for entry in listing["workflows"]] == ["sample"], listing
        assert not (repository / ".prifly" / "workflows" / "sample").exists(), "search copied a folder"
        added = run(binary, "project", "workflows", "add", "sample", "--catalog", catalog, "--repository", repository)
        assert added.returncode == 0, added.stderr
        result = json.loads(added.stdout)
        assert result["schema_version"] == "project-workflow-add/1", result
        assert result["origin"]["commit"] == commit and result["origin"]["ref"] == "v1" and result["origin"]["catalog"] == str(catalog), result
        assert (repository / ".prifly" / "workflows" / "sample" / "workflow.yaml").is_file()
        listed = run(binary, "--project", authority, "project", "workflows", "--repository", repository)
        assert listed.returncode == 0, listed.stderr
        assert [launch["id"] for launch in json.loads(listed.stdout)["launches"]] == ["sample"], listed.stdout
        before = run(binary, "--project", authority, "package", "list")
        compiled = run(binary, "--project", authority, "project", "compile", "--repository", repository, "--package", "sample", "--host", "codex-cli", "--output", root / "sealed")
        assert compiled.returncode == 0, compiled.stderr
        assert (root / "sealed" / "prifly.package.json").is_file()
        after = run(binary, "--project", authority, "package", "list")
        assert before.stdout == after.stdout, "workflow install or compile changed the authority"
        updated = run(binary, "project", "workflows", "update", "sample", "--repository", repository)
        assert updated.returncode == 0, updated.stderr
        assert json.loads(updated.stdout)["up_to_date"] is True, updated.stdout
        twin = run(binary, "project", "workflows", "add", source, "--ref", "v1", "--name", "twin", "--repository", repository)
        assert twin.returncode != 0 and "project_workflow_package_conflict" in twin.stderr, twin.stderr
        removed = run(binary, "project", "workflows", "remove", "sample", "--repository", repository)
        assert removed.returncode == 0, removed.stderr
        assert not (repository / ".prifly" / "workflows" / "sample").exists(), "remove left the folder"


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    cases = sorted(path for path in CORPUS.iterdir() if path.is_dir())
    assert cases, "authoring corpus is empty"
    for case in cases:
        check_case(binary, case)
    launch_cases = sorted(path for path in LAUNCH_CORPUS.iterdir() if path.is_dir())
    assert launch_cases, "project launch corpus is empty"
    for case in launch_cases:
        check_launch_case(binary, case)
    check_workflow_catalog(binary)
    print(json.dumps({"outcome": "passed", "cases": [case.name for case in cases], "launch_cases": [case.name for case in launch_cases], "workflow_catalog": "passed"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
