#!/usr/bin/env python3
"""Exercise two Runs working side by side on one repository, through the real CLI.

The engine exists so a team can hold several tasks on one branch at once, a
worktree and a Run each. Until 0.13.0 exclusivity was decided by the shared git
directory, so the second task was refused. Go tests cover the rule; nothing
covered the trees themselves — that both are real, independent checkouts an
executor can commit in.

Requires an explicitly built binary. Keeps its temporary project for inspection.
"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", type=Path, help="absent or empty directory; otherwise a new temporary project")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    target = (args.target or Path(tempfile.mkdtemp(prefix="prifly-parallel-", dir="/tmp"))).resolve()
    if target.exists() and (not target.is_dir() or any(target.iterdir())):
        parser.error("target must be absent or empty; no existing project files were changed")
    authority, repository = target / "authority", target / "repository"
    repository.mkdir(parents=True)

    def git(tree, *arguments, capture=False):
        command = subprocess.run(["git", "-C", str(tree), *arguments], capture_output=True, text=True, timeout=30)
        assert command.returncode == 0, f"git {arguments[0]} in {tree} failed: {command.stderr}"
        return command.stdout.strip() if capture else None

    git(repository, "init", "-q", "--initial-branch=development")
    git(repository, "config", "user.email", "verify@example.invalid")
    git(repository, "config", "user.name", "Verification")
    (repository / "README.md").write_text("one branch, many tasks\n")
    git(repository, "add", "README.md")
    git(repository, "commit", "-q", "-m", "base")

    def cli(*arguments):
        command = subprocess.run([str(binary), "--project", str(authority), "--json", *arguments], capture_output=True, text=True, timeout=30)
        if command.returncode:
            raise RuntimeError(f"CLI {arguments[:2]} failed ({command.returncode}): {command.stderr}")
        return json.loads(command.stdout)

    subprocess.run([str(binary), "init", str(authority)], check=True, timeout=30, capture_output=True)

    def claim(owner):
        return cli("claim", "create", "--repository", str(repository), "--owner", owner)["claim"]

    first, second = claim("session:first"), claim("session:second")
    assert first["id"] != second["id"], "two owners were given one claim"
    assert first["path"] != second["path"], f"two owners were given one tree: {first['path']}"
    assert first["branch"] != second["branch"], f"two owners were given one branch: {first['branch']}"
    assert first["repository"] == second["repository"], "the fixture built two repositories, so this proves nothing"
    assert first["base_commit"] == second["base_commit"], "the two claims did not start from one commit"

    # Each claim must be a working tree an executor can work in, not just a
    # record: one branch checked out, and a commit in one invisible in the other.
    trees = {}
    for owner, held in (("first", first), ("second", second)):
        assert not Path(held["path"]).is_absolute(), f"a claim path escaped the authority: {held['path']}"
        tree = authority / held["path"]
        assert (tree / "README.md").is_file(), f"the {owner} claim holds no checkout at {held['path']}"
        assert git(tree, "rev-parse", "--abbrev-ref", "HEAD", capture=True) == held["branch"]
        (tree / f"{owner}.txt").write_text(f"work of the {owner} run\n")
        git(tree, "add", f"{owner}.txt")
        git(tree, "-c", "user.email=verify@example.invalid", "-c", "user.name=Verification", "commit", "-q", "-m", f"{owner} run")
        trees[owner] = tree
    assert git(trees["first"], "rev-parse", "HEAD", capture=True) != git(trees["second"], "rev-parse", "HEAD", capture=True), "both runs committed onto one branch"
    assert not (trees["first"] / "second.txt").exists() and not (trees["second"] / "first.txt").exists(), "one run's work appeared in the other's tree"

    # Ending one task leaves the other running: a release is not a repository-wide event.
    released = cli("claim", "release", "--id", first["id"], "--generation", str(first["generation"]))["claim"]
    assert released["status"] == "released", released["status"]
    listed = {c["id"]: c["status"] for c in cli("claim", "list")["claims"]}
    assert listed == {first["id"]: "released", second["id"]: "active"}, listed
    assert (trees["second"] / "second.txt").is_file(), "releasing one claim disturbed the other's tree"
    # And the repository is free again for the next task while the second still holds its own.
    third = claim("session:third")
    assert third["path"] not in (first["path"], second["path"]), f"a new task was handed a tree already in use: {third['path']}"

    report = {
        "binary_sha256": binary_digest,
        "project": str(authority),
        "repository": str(repository),
        "claims": [{"id": c["id"], "owner_id": c["owner_id"], "path": c["path"], "branch": c["branch"]} for c in (first, second, third)],
        "outcome": "passed",
    }
    results = target / "verification"
    results.mkdir()
    (results / "summary.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
