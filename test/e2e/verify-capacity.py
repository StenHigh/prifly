#!/usr/bin/env python3
"""What a second Run meets while the first one is still running.

Parallel Runs are opened by the claim rule, and two more things stand between a
second Run and the machine: one authority drives one Run at a time, and it
admits a bounded number of attempts. The first is what a reader actually hits,
and its refusal carried no message at all until this check was written.

The fixture's shell step is rebound to a script that sleeps, so a local Run
holds the driver and its admission slot long enough to collide with. No assisted
host is involved — which is also why `capacity_conflict` itself is out of scope
here: reaching it needs an outstanding attempt with no live driver, and only an
assisted step leaves one.

Requires an explicitly built binary. Keeps its temporary project for inspection.
"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time

HOLD_SECONDS = 12
STATE_REFUSAL, USAGE_REFUSAL = 3, 2


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", type=Path, help="absent or empty directory; otherwise a new temporary project")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    target = (args.target or Path(tempfile.mkdtemp(prefix="prifly-capacity-", dir="/tmp"))).resolve()
    if target.exists() and (not target.is_dir() or any(target.iterdir())):
        parser.error("target must be absent or empty; no existing project files were changed")
    fixture = Path(__file__).resolve().parents[2] / "test/fixtures/foundation/create-demo.py"
    subprocess.run([sys.executable, "-B", str(fixture), "--binary", str(binary), "--target", str(target)], check=True, timeout=120, stdout=subprocess.PIPE)

    (target / "scripts/slow-step.sh").write_text(f"""#!/bin/sh
set -eu
cat >/dev/null
sleep {HOLD_SECONDS}
printf '{{"schema_version":"1","run_id":"%s","step_instance_id":"%s","attempt_id":"%s","envelope_digest":"%s","verdict":"pass","outputs":{{}},"evidence_refs":[],"effect_receipt_refs":[],"summary":"Slow process completed."}}\\n' \\
  "$PRIFLY_RUN_ID" "$PRIFLY_STEP_ID" "$PRIFLY_ATTEMPT_ID" "$PRIFLY_ENVELOPE_DIGEST" >&3
""")
    settings_path = target / "prifly.json"
    settings = json.loads(settings_path.read_text())
    executor = settings["configuration"]["executors"]["demo:step/shell"]
    executor["args"] = ["slow-step.sh"]
    executor["files"] = {"slow-step.sh": "scripts/slow-step.sh"}
    executor["timeout_ms"] = (HOLD_SECONDS + 30) * 1000
    settings_path.write_text(json.dumps(settings, ensure_ascii=False, indent=2) + "\n")

    def cli(*arguments, expect=0, timeout=120):
        command = subprocess.run([str(binary), "--project", str(target), "--json", *arguments], capture_output=True, text=True, timeout=timeout)
        assert command.returncode == expect, f"CLI {arguments[:2]} exited {command.returncode}, wanted {expect}: {command.stderr[:400]}"
        stream = command.stdout if expect == 0 else command.stderr
        return json.loads(stream.strip().splitlines()[-1]) if stream.strip() else {}

    # The whole answer is pinned, so a new key cannot arrive unnoticed and an old
    # one cannot leave. `would_refuse` is the reading that used to be obtainable
    # only by attempting a start -- and that refusal creates and queues a Run, so
    # asking the question changed its answer, which is why nothing compared this
    # refusal between releases.
    idle = cli("capacity", "show")
    assert idle == {"schema_version": "1", "capacity": 1, "held": {}, "waiting": {}, "available": 1, "would_refuse": ""}, f"a fresh authority does not admit exactly one attempt: {idle}"

    holder = subprocess.Popen([str(binary), "--project", str(target), "--json", "run", "start", "--workflow", "workflows/shell.json",
                               "--brief", "brief.json", "--command-id", "command:capacity-holder", "--drive"],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        deadline, held = time.monotonic() + HOLD_SECONDS, {}
        while time.monotonic() < deadline:
            held = cli("capacity", "show")["held"]
            if held:
                break
            time.sleep(0.2)
        assert len(held) == 1, f"a driven attempt never took the single slot: {held}"
        # With the only slot taken, the read must name the refusal a start would
        # meet -- while creating nothing, unlike the start that would prove it.
        full = cli("capacity", "show")
        assert full["available"] == 0 and full["would_refuse"] == "capacity_conflict", f"a full authority did not name the refusal: {full}"
        assert full["waiting"] == {}, f"reading the capacity joined the admission queue: {full['waiting']}"

        # The refusal a reader meets first. It used to arrive with no message,
        # pointing at doctor and run status, neither of which reports a driver
        # lock — and raising capacity does not lift one either.
        refused = cli("run", "start", "--workflow", "workflows/shell.json", "--brief", "brief.json",
                      "--command-id", "command:capacity-second", "--drive", expect=USAGE_REFUSAL, timeout=60)
        assert refused["code"] == "driver_already_active", refused
        assert refused["safe_next_actions"] == ["run.status", "run.events"], refused["safe_next_actions"]
        # An engine-authored refusal keeps its own words even when it wraps a
        # cause, and it keeps them in `message`: violations names places in a
        # document the caller supplied, and there is no such place here. Both
        # fields carried explanations until 0.13.8, so a reader could not know
        # which to read.
        assert refused["violations"] == [], refused["violations"]
        detail = refused["message"]
        assert "violations" not in detail, f"the message points elsewhere instead of explaining: {detail}"
        assert "one authority drives one Run at a time" in detail, detail
        assert "capacity set does not lift it" in detail, detail
        assert "run:" in detail, f"the refusal does not name the Run holding the driver: {detail}"
        assert "flock" not in detail and "resource temporarily unavailable" not in detail.lower(), f"the cause's own text leaked: {detail}"
    finally:
        holder.wait(timeout=HOLD_SECONDS + 60)
    assert holder.returncode == 0, f"the holding Run failed: {holder.stderr.read()[:300]}"
    freed = cli("capacity", "show")
    assert freed["held"] == {}, "a settled attempt kept its slot"
    assert freed["available"] == 1 and freed["would_refuse"] == "", f"a freed slot still reads as refused: {freed}"

    # The bound is authority state: it moves only with a stated reason, and a
    # refused change leaves it where it was.
    assert cli("capacity", "set", "--capacity", "4", expect=USAGE_REFUSAL)["code"] == "invalid_usage"
    assert cli("capacity", "show")["capacity"] == 1, "a refused capacity change still moved the bound"
    cli("capacity", "set", "--capacity", "2", "--reason", "verification: two attempts side by side")
    assert cli("capacity", "show")["capacity"] == 2, "the raised bound was not recorded"

    report = {"binary_sha256": binary_digest, "project": str(target), "hold_seconds": HOLD_SECONDS, "outcome": "passed"}
    results = target / "verification"
    results.mkdir()
    (results / "summary.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
