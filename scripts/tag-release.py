#!/usr/bin/env python3
"""Tag a release only on a commit GitHub has qualified.

The everyday push check (`verify`) and the pre-release qualification
(`qualify`: race detector and full e2e) must both have succeeded on the exact
commit. A tag on anything else is refused with the check that is missing, and
so is a tag on a `[skip ci]` commit, which the release workflow never builds.
The tag push itself starts the release workflow; approving its environment
stays a separate, deliberate step.
"""

import argparse
import json
import re
import subprocess
import sys

REQUIRED = ("verify", "qualify")


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout.strip()


def missing_checks(runs):
    """The required workflows without a successful run, each with what it found."""
    missing = []
    for name in REQUIRED:
        mine = [r for r in runs if r.get("name") == name]
        if any(r.get("status") == "completed" and r.get("conclusion") == "success" for r in mine):
            continue
        if not mine:
            missing.append(f"{name}: never ran on this commit")
        elif any(r.get("status") != "completed" for r in mine):
            missing.append(f"{name}: still running")
        else:
            missing.append(f"{name}: {', '.join(sorted({r.get('conclusion') or 'unknown' for r in mine}))}")
    return missing


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("version", help="vMAJOR.MINOR.PATCH")
    parser.add_argument("--sha", default="HEAD")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if not re.fullmatch(r"v\d+\.\d+\.\d+", args.version):
        sys.exit(f"tag-release: {args.version} is not vMAJOR.MINOR.PATCH")
    sha = run("git", "rev-parse", args.sha + "^{commit}")
    if "[skip ci]" in run("git", "log", "-1", "--format=%B", sha):
        sys.exit(f"tag-release: {sha[:7]} is a [skip ci] commit; the release workflow builds nothing from it")
    repo = run("gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
    runs = json.loads(run("gh", "api", f"repos/{repo}/actions/runs?head_sha={sha}&per_page=100", "--jq", "[.workflow_runs[] | {name, status, conclusion}]"))
    missing = missing_checks(runs)
    if missing:
        sys.exit(f"tag-release: {sha[:7]} is not qualified for a release -- " + "; ".join(missing)
                 + ". Run `gh workflow run qualify.yml --ref main` for this commit and tag once both are green.")
    print(f"tag-release: {sha[:7]} has verify and qualify green")
    if args.dry_run:
        return 0
    run("git", "tag", "-a", args.version, sha, "-m", args.version)
    run("git", "push", "origin", args.version)
    print(f"tag-release: {args.version} pushed on {sha[:7]}; approve the release environment of its workflow run")
    return 0


if __name__ == "__main__":
    sys.exit(main())
