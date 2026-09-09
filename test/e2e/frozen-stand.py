#!/usr/bin/env python3
"""Freeze one settled Run, then read it with two binaries and diff the readings.

An end-to-end check says a build works today. It cannot say what changed between
two builds, because it rebuilds its fixture every time — so a release could
alter what every reader sees and no gate would notice. This freezes the data
instead: one authority with a settled Run, kept, and re-read by each binary.
Anything that differs is the difference between the binaries and nothing else.

    frozen-stand.py build   --binary bin/prifly [--stand DIR]      once
    frozen-stand.py compare --old BINARY --new BINARY [--stand DIR]

Not part of `make e2e`: comparing needs a second binary, which the repository
does not carry, and a gate that depends on something absent will one day pass
having read nothing.

The two ideas that keep it from lying about itself came from the package
session's own stand and are kept here deliberately:

  * the noise mask is observed, not listed. Every command is read twice by each
    binary and whatever moved between two reads of the same one is masked, so
    clocks and ids drop out unnamed and a field that starts moving next month
    needs no edit here.
  * the comparison must prove it can see a difference. `version` is read first
    and the two binaries must disagree on it; if they agree, the comparison is
    broken and says so instead of reporting "same" everywhere.
  * and, because a stand outlives the binaries it was built for, it records the
    version that built it and refuses to be read by anything older.
"""

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

READS = (
    # First, and deliberately: two different binaries must disagree here. If
    # they do not, the reading and diffing below cannot see a difference at all
    # and every "same" it prints is worthless.
    ("version", lambda run: ["version"]),
    ("run status", lambda run: ["run", "status", run]),
    ("run explain", lambda run: ["run", "explain", run]),
    ("run events", lambda run: ["run", "events", run]),
    ("run timing", lambda run: ["run", "timing", run]),
    ("package list", lambda run: ["package", "list"]),
    ("capacity show", lambda run: ["capacity", "show"]),
)


def version_of(binary):
    out = subprocess.run([str(binary), "version"], capture_output=True, text=True, timeout=30, check=True)
    return json.loads(out.stdout)["version"]


def as_tuple(version):
    return tuple(int(part) for part in re.findall(r"\d+", version)[:3])


def read(binary, project, arguments):
    out = subprocess.run([str(binary), "--project", str(project), "--json", *arguments],
                         capture_output=True, text=True, timeout=120)
    return out.stdout if out.returncode == 0 else f"<refused {out.returncode}> {out.stderr.strip()}"


def flatten(value, prefix=""):
    """Every leaf as path -> repr, so a moved field is named, not just diffed."""
    if isinstance(value, dict):
        for key in value:
            yield from flatten(value[key], f"{prefix}/{key}")
    elif isinstance(value, list):
        for i, item in enumerate(value):
            yield from flatten(item, f"{prefix}/{i}")
    else:
        yield prefix, repr(value)


def leaves(text):
    try:
        return dict(flatten(json.loads(text)))
    except json.JSONDecodeError:
        return {"/raw": text}


def build(args):
    stand = args.stand.resolve()
    if stand.exists() and any(stand.iterdir()):
        raise SystemExit(f"{stand} is not empty; a stand is built once and then only read")
    binary = args.binary.resolve(strict=True)
    fixture = Path(__file__).resolve().parents[2] / "test/fixtures/foundation/create-demo.py"
    project = stand / "project"
    subprocess.run([sys.executable, "-B", str(fixture), "--binary", str(binary), "--target", str(project)],
                   check=True, timeout=180, stdout=subprocess.PIPE)
    started = subprocess.run([str(binary), "--project", str(project), "--json", "run", "start",
                              "--workflow", "workflows/transform.json", "--brief", "brief.json",
                              "--command-id", "command:frozen-stand", "--drive",
                              "--input", "source=inputs/source.txt"],
                             capture_output=True, text=True, timeout=180, check=True)
    run = json.loads(started.stdout)["run"]["id"]
    (stand / "stand.json").write_text(json.dumps({
        "schema_version": "prifly-frozen-stand/1",
        "built_by": version_of(binary),
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "run": run,
    }, ensure_ascii=False, indent=2) + "\n")
    print(f"stand built at {stand} by {version_of(binary)}, run {run}")


def compare(args):
    stand = args.stand.resolve(strict=True)
    manifest = json.loads((stand / "stand.json").read_text())
    project, run = stand / "project", manifest["run"]
    old, new = args.old.resolve(strict=True), args.new.resolve(strict=True)
    versions = {"old": version_of(old), "new": version_of(new)}

    # A stand carries state written by the build; reading it with something
    # older compares the stand against the binaries, and that difference is
    # indistinguishable from a real one.
    for side, version in versions.items():
        if as_tuple(version) < as_tuple(manifest["built_by"]):
            raise SystemExit(f"the stand was built with {manifest['built_by']}, newer than the {side} binary "
                             f"{version}: rebuild it with the oldest binary you mean to compare")
    if versions["old"] == versions["new"]:
        raise SystemExit(f"both binaries report {versions['old']}: this compares a binary with itself "
                         "and can only say 'same'")

    rows, differing = [], 0
    for name, argv in READS:
        readings, moved = {}, set()
        for side, binary in (("old", old), ("new", new)):
            first, second = leaves(read(binary, project, argv(run))), leaves(read(binary, project, argv(run)))
            # Observed noise: anything that moved between two reads of one
            # binary cannot carry a difference between two binaries.
            moved |= {k for k in first.keys() | second.keys() if first.get(k) != second.get(k)}
            readings[side] = second
        stable = {side: {k: v for k, v in reading.items() if k not in moved} for side, reading in readings.items()}
        changed = sorted(k for k in stable["old"].keys() | stable["new"].keys()
                         if stable["old"].get(k) != stable["new"].get(k))
        differing += bool(changed)
        rows.append({"read": name, "verdict": "DIFFERS" if changed else "same",
                     "masked_fields": len(moved), "changed": changed[:20]})
        marker = "DIFFERS" if changed else "same   "
        print(f"{marker}  {name:<14} ({len(moved)} fields move between reads)")
        for path in changed[:20]:
            print(f"            {path}: {stable['old'].get(path)} -> {stable['new'].get(path)}")

    if rows[0]["read"] != "version" or rows[0]["verdict"] != "DIFFERS":
        raise SystemExit("the version read did not come back different: this comparison cannot see a "
                         "difference and its 'same' verdicts prove nothing")
    print(json.dumps({"stand": str(stand), "built_by": manifest["built_by"], "versions": versions,
                      "reads_differing": differing, "rows": rows}, ensure_ascii=False, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    default = Path.home() / ".prifly-stands" / "engine"
    sub = parser.add_subparsers(dest="action", required=True)
    b = sub.add_parser("build")
    b.add_argument("--binary", required=True, type=Path)
    b.add_argument("--stand", type=Path, default=default)
    b.set_defaults(run=build)
    c = sub.add_parser("compare")
    c.add_argument("--old", required=True, type=Path)
    c.add_argument("--new", required=True, type=Path)
    c.add_argument("--stand", type=Path, default=default)
    c.set_defaults(run=compare)
    args = parser.parse_args()
    args.run(args)


if __name__ == "__main__":
    main()
