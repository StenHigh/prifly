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
from release_checks import GitHub, REQUIRED, select


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout.strip()


def missing_checks(states):
    return [name+": "+r['state']+" ("+r['reason']+")" for name,r in states.items() if r['state']!='success']


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
    client = GitHub()
    states = {name: select(client, name, sha) for name in REQUIRED}
    missing = missing_checks(states)
    if missing:
        sys.exit(f"tag-release: {sha[:7]} is not qualified for a release -- " + "; ".join(missing)
                 + ". Use scripts/ensure-release-checks.py --sha SHA --ref REF; retry a failed batch explicitly with --force.")
    print(f"tag-release: {sha[:7]} has full verify and qualify on exact SHA")
    for name, evidence in states.items():
        print(name+": "+evidence['url']+" (direct batch, attempt "+str(evidence['attempt'])+")")
    if args.dry_run:
        return 0
    run("git", "tag", "-a", args.version, sha, "-m", args.version)
    run("git", "push", "origin", args.version)
    print(f"tag-release: {args.version} pushed on {sha[:7]}; approve the release environment of its workflow run")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        sys.exit("tag-release: evidence unavailable: "+str(error))
