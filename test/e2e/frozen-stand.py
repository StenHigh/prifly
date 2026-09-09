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
  * the comparison must prove it can see a difference. The two binaries must
    disagree on at least one observable discriminator — the reported version, or
    failing that the file digest; if neither moved, the comparison is broken and
    says so instead of reporting "same" everywhere. Version alone is not enough:
    a binary built from a tag rather than by the release workflow reports the
    unreleased placeholder, so two different local builds share a version and
    only their digests differ.
  * and, because a stand outlives the binaries it was built for, it records the
    version that built it and refuses to be read by anything older.

Refusals are compared as well as readings: a set of commands that are refused
before they act, read from both binaries and diffed by code, message,
violations and safe_next_actions. Three releases running were mostly refusal
texts, each confirmed once by a person meeting it live and never again.

Two shapes, and the second is not a better version of the first. A `settled`
stand holds a Run driven to completion; a `paused` one holds a Run still open
with its stop unreleased, which is the only shape that can be refused
`active_stop` — a text rewritten in 0.13.3 that nothing compared until now. The
price is measured, not guessed: on the settled stand 6 and 2 fields move between
two reads of one binary, on the paused stand 22 and 30. A Run that is still open
derives more from the clock, so more is masked, so its "same" covers less. Read
both, and trust the settled one further.

What the settled stand holds, measured rather than intended: one Run of
`workflows/transform.json`, driven to completion, `completed / succeeded`, one
step, one attempt whose start was observed and which settled, no session, no
checks, no diagnostics. Node kinds present: run, workflow_invocation,
stage_activation, step_instance, attempt.

What it does not reach — and a change in any of these passes this comparison in
silence, which is why the list is written down rather than remembered:

  * an assisted attempt, in any state. The package session's stand holds a
    settled assisted attempt, so their stand and this one are complements, not
    duplicates.
  * an attempt that settled without ever starting, and one still waiting to
    start. Together with the assisted case these are three of the four
    dispatch_latency branches; this stand covers the fourth.
  * check executions — no `check_execution` node occurs here at all, so
    everything that hangs off one is unmeasured.
  * refusals beyond the seven read here — every refusal that needs a state this
    stand does not hold: a claim conflict, a capacity conflict, a driver lock,
    a recovery, a stale digest, anything from a failed Run.
  * claims and worktrees: nothing is claimed, so the claim record, its identity
    and its release are outside the reading.
  * capacity and the driver lock, which need a second Run to collide with.
  * context profiles, invocation trees, repeats, parallel stages and choices —
    the fixture's graph is a single step.
  * packages beyond the demo fixture, and any authority holding more than one
    edition.
  * the monitor, which reads over HTTP rather than through the CLI.
  * history: one Run, so nothing about lists, paging or ordering.

The fix for any of these is another stand with its own list, not a wider claim
about this one.
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
    # Refusals are read here too, and they are the reason this stand exists at
    # all: three releases in a row were mostly refusal texts, each confirmed
    # once by a person reading it on a live miss and never again. A refusal that
    # quietly gets worse is invisible to every gate. None of these change
    # anything -- each is refused before it acts.
    # Each of these was checked to be refused, and to be refused with the code
    # named beside it — a probe whose command quietly starts succeeding would
    # otherwise go on being compared under a name it no longer earns.
    ("refuse not_found", lambda run: ["run", "status", "run:" + "0" * 64]),
    ("refuse invalid_usage", lambda run: ["run", "cancel", run]),
    ("refuse schema_invalid", lambda run: ["validate", "--workflow", "prifly.json"]),
    ("refuse invalid_json", lambda run: ["artifact", "export", "--ref", "inputs/source.txt", "--output", "unwritten"]),
    ("refuse terminal_run", lambda run: ["run", "pause", run, "--reason", "frozen stand probe"], ("settled",)),
    # Only a paused Run with a stop still held can be refused this way, and the
    # text was rewritten in 0.13.3 -- until this stand existed nothing compared
    # it between releases.
    ("refuse active_stop", lambda run: ["run", "resume", run, "--expected-version", "2", "--reason", "frozen stand probe"], ("paused",)),
    ("refuse version_conflict", lambda run: ["run", "resume", run, "--expected-version", "1", "--reason", "frozen stand probe"]),
    ("refuse cancel_not_reversible", lambda run: ["run", "release", run, "--expected-epoch", "0", "--stop", "nope:1", "--reason", "frozen stand probe"]),
    ("refuse missing attempt", lambda run: ["run", "resolve", run, "--attempt", "attempt:none", "--outcome", "applied", "--reason", "frozen stand probe"]),
)

# How many refusals this set actually tells apart. A set of eight probes that
# all land on invalid_usage discriminates one outcome while reading like eight,
# so the count is pinned rather than inferred: three of the first eight probes
# here were argument parsing, and it took a dependent session counting their own
# to notice that "seven of seven passed" meant four distinct outcomes.
DISTINCT_REFUSAL_CODES = {"settled": 7, "paused": 7}

# A probe that stops being refused stops testing a refusal. The comparison
# checks this on every run rather than trusting the names above.
EXPECTED_REFUSALS = {
    "refuse not_found": "not_found",
    "refuse invalid_usage": "invalid_usage",
    "refuse schema_invalid": "schema_invalid",
    "refuse invalid_json": "invalid_json",
    "refuse terminal_run": "terminal_run",
    "refuse active_stop": "active_stop",
    "refuse version_conflict": "version_conflict",
    "refuse cancel_not_reversible": "cancel_not_reversible",
    "refuse missing attempt": "not_found",
}


def version_of(binary):
    out = subprocess.run([str(binary), "version"], capture_output=True, text=True, timeout=30, check=True)
    return json.loads(out.stdout)["version"]


def as_tuple(version):
    return tuple(int(part) for part in re.findall(r"\d+", version)[:3])


def released(version):
    """A build made by the release workflow carries the tag; anything else
    reports the placeholder compiled into the source, which orders nothing."""
    return re.fullmatch(r"\d+\.\d+\.\d+", version) is not None


def read(binary, project, arguments):
    """A refusal is part of the reading, not an error: its envelope is the thing
    being compared, and the exit status is compared with it."""
    out = subprocess.run([str(binary), "--project", str(project), "--json", *arguments],
                         capture_output=True, text=True, timeout=120)
    if out.returncode == 0:
        return out.stdout
    envelope = out.stderr.strip().splitlines()[-1] if out.stderr.strip() else "{}"
    try:
        problem = json.loads(envelope)
    except json.JSONDecodeError:
        return json.dumps({"exit": out.returncode, "raw": out.stderr.strip()[:2000]})
    problem["exit"] = out.returncode
    return json.dumps(problem)


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
                              "--command-id", "command:frozen-stand",
                              *(["--drive"] if args.shape == "settled" else []),
                              "--input", "source=inputs/source.txt"],
                             capture_output=True, text=True, timeout=180, check=True)
    # A driven start answers with the Run; an undriven one answers with the
    # receipt that created it.
    answer = json.loads(started.stdout)
    run = answer["run"]["id"] if "run" in answer else answer["receipt"]["run_id"]
    if args.shape == "paused":
        subprocess.run([str(binary), "--project", str(project), "--json", "run", "pause", run,
                        "--reason", "frozen stand: a Run held open with its stop unreleased"],
                       check=True, timeout=60, stdout=subprocess.PIPE)
    (stand / "stand.json").write_text(json.dumps({
        "schema_version": "prifly-frozen-stand/1",
        "built_by": version_of(binary),
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "run": run,
        "shape": args.shape,
    }, ensure_ascii=False, indent=2) + "\n")
    print(f"{args.shape} stand built at {stand} by {version_of(binary)}, run {run}")


def fingerprint(binary, project, run):
    """What the stand must still be afterwards. Four of the refusal probes are
    mutating commands refused only by the Run's current state; if a release ever
    reorders those checks they would succeed and quietly rewrite the very thing
    this stand exists to hold still.

    The subject is the Run, not the authority. A refused command still records
    its receipt — measured here: three reads leave `cut` at 70, one refused
    `run pause` moves it to 71 while `run_version` stays put. That is the
    authority being honest about a refusal, not the stand being damaged, so
    `cut` stays out of this fingerprint; including it reported damage on every
    single run."""
    status = json.loads(read(binary, project, ["run", "status", run]))
    body = status.get("run", {})
    return {"run_version": status.get("run_version"), "status": body.get("status"),
            "outcome": body.get("outcome"), "stops": len(body.get("stops", []) or []),
            "attempts": len(body.get("attempts", {}) or {}),
            "verdicts": sorted((a or {}).get("accepted", {}).get("verdict", "") for a in (body.get("attempts", {}) or {}).values())}


def compare(args):
    stand = args.stand.resolve(strict=True)
    manifest = json.loads((stand / "stand.json").read_text())
    project, run = stand / "project", manifest["run"]
    old, new = args.old.resolve(strict=True), args.new.resolve(strict=True)
    versions = {"old": version_of(old), "new": version_of(new)}

    digests = {"old": hashlib.sha256(old.read_bytes()).hexdigest(),
               "new": hashlib.sha256(new.read_bytes()).hexdigest()}

    # A stand carries state written by the build; reading it with something
    # older compares the stand against the binaries, and that difference is
    # indistinguishable from a real one. An unreleased build reports a
    # placeholder version, so its age cannot be established — say so rather
    # than refusing a comparison or silently letting it through.
    age = "checked"
    for side, version in versions.items():
        if not released(version) or not released(manifest["built_by"]):
            age = f"not established: {version} or {manifest['built_by']} is an unreleased build"
            continue
        if as_tuple(version) < as_tuple(manifest["built_by"]):
            raise SystemExit(f"the stand was built with {manifest['built_by']}, newer than the {side} binary "
                             f"{version}: rebuild it with the oldest binary you mean to compare")

    # The discriminator must separate builds, not releases: two binaries built
    # from tags share the placeholder version and differ only by digest.
    if versions["old"] == versions["new"] and digests["old"] == digests["new"]:
        raise SystemExit(f"both binaries report {versions['old']} and hash to {digests['old'][:16]}…: "
                         "this compares a binary with itself and can only say 'same'")
    discriminator = "version" if versions["old"] != versions["new"] else "digest"

    shape = manifest.get("shape", "settled")
    reachable = {name for entry in READS for name in [entry[0]]
                 if shape in (entry[2] if len(entry) > 2 else ("settled", "paused"))}
    distinct = len({code for name, code in EXPECTED_REFUSALS.items() if name in reachable})
    if distinct != DISTINCT_REFUSAL_CODES[shape]:
        raise SystemExit(f"the {shape} stand's refusal probes tell {distinct} outcomes apart, not "
                         f"{DISTINCT_REFUSAL_CODES[shape]}: "
                         "a probe was added or changed without the count, and a set reads as wider coverage "
                         "than it has")

    before = fingerprint(new, project, run)
    rows, differing = [], 0
    for entry in READS:
        name, argv = entry[0], entry[1]
        shapes = entry[2] if len(entry) > 2 else ("settled", "paused")
        if shape not in shapes:
            continue
        readings, moved = {}, set()
        for side, binary in (("old", old), ("new", new)):
            first, second = leaves(read(binary, project, argv(run))), leaves(read(binary, project, argv(run)))
            # Observed noise: anything that moved between two reads of one
            # binary cannot carry a difference between two binaries.
            moved |= {k for k in first.keys() | second.keys() if first.get(k) != second.get(k)}
            readings[side] = second
        if name in EXPECTED_REFUSALS:
            for side, reading in readings.items():
                got = reading.get("/code", "").strip("'")
                if got != EXPECTED_REFUSALS[name]:
                    raise SystemExit(f"{name!r} is no longer refused as {EXPECTED_REFUSALS[name]} by the {side} "
                                     f"binary but as {got or 'nothing at all'}: this probe has stopped testing "
                                     "a refusal and its verdict means something else now")
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

    after = fingerprint(new, project, run)
    if after != before:
        raise SystemExit(f"reading the stand changed it: {before} became {after}. A probe that was refused "
                         "only by the Run's state has started succeeding; rebuild the stand and drop that probe")

    if discriminator == "version" and (rows[0]["read"] != "version" or rows[0]["verdict"] != "DIFFERS"):
        raise SystemExit("the version read did not come back different although the two binaries report "
                         "different versions: this comparison cannot see a difference and its 'same' "
                         "verdicts prove nothing")
    print(json.dumps({"stand": str(stand), "built_by": manifest["built_by"], "versions": versions,
                      "digests": {k: v[:16] for k, v in digests.items()}, "discriminator": discriminator,
                      "stand_age": age, "reads_differing": differing, "rows": rows}, ensure_ascii=False, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    default = Path.home() / ".prifly-stands" / "engine"
    sub = parser.add_subparsers(dest="action", required=True)
    b = sub.add_parser("build")
    b.add_argument("--binary", required=True, type=Path)
    b.add_argument("--stand", type=Path, default=default)
    b.add_argument("--shape", choices=("settled", "paused"), default="settled",
                   help="settled: a Run driven to completion. paused: a Run held open with its stop unreleased, "
                        "which is the only shape that can be refused active_stop")
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
