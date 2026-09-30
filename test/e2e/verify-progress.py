#!/usr/bin/env python3
"""What a second reader sees of a program step's own progress, from a built binary.

The fixture's shell step is rebound to a script that reports two phases over
fd 4 and waits in between, so a separate `run progress` client reads the
running phase, then the last one after the step ends. A second project variant
reports a phase and then fails while printing "pass": the failure stays the
outcome and the phase is only the program's last word. Nothing here is specific
to tests or to any project.

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

WAIT_SECONDS = 20


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", type=Path, help="absent or empty directory; otherwise a new temporary directory")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    root = (args.target or Path(tempfile.mkdtemp(prefix="prifly-progress-", dir="/tmp"))).resolve()
    if root.exists() and (not root.is_dir() or any(root.iterdir())):
        parser.error("target must be absent or empty; no existing project files were changed")
    report = {"binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "root": str(root)}
    for variant in ("pass", "fail"):
        report[variant] = verify(binary, root / variant, variant)
    report["outcome"] = "passed"
    print(json.dumps(report, ensure_ascii=False, indent=2))


def verify(binary, target, variant):
    fixture = Path(__file__).resolve().parents[2] / "test/fixtures/foundation/create-demo.py"
    subprocess.run([sys.executable, "-B", str(fixture), "--binary", str(binary), "--target", str(target)], check=True, timeout=120, stdout=subprocess.PIPE)
    ending = """printf 'pass\\n'
exit 9""" if variant == "fail" else """printf '{"schema_version":"1","run_id":"%s","step_instance_id":"%s","attempt_id":"%s","envelope_digest":"%s","verdict":"pass","outputs":{},"evidence_refs":[],"effect_receipt_refs":[],"summary":"Reported its phases."}\\n' \\
  "$PRIFLY_RUN_ID" "$PRIFLY_STEP_ID" "$PRIFLY_ATTEMPT_ID" "$PRIFLY_ENVELOPE_DIGEST" >&3"""
    (target / "scripts/phased-step.sh").write_text(f"""#!/bin/sh
set -eu
cat >/dev/null
test "$PRIFLY_PROGRESS_FD" = 4
printf '{{"schema_version":"program-progress/1","phase":"product","current":4200,"total":6994}}\\n' >&4
printf '{{"schema_version":"program-progress/1","phase":"<b>bad</b>"}}\\n' >&4
i=0
while [ ! -f go ] && [ $i -lt {WAIT_SECONDS * 10} ]; do sleep 0.1; i=$((i+1)); done
printf '{{"schema_version":"program-progress/1","phase":"baseline","message":"comparing"}}\\n' >&4
{ending}
""")
    settings_path = target / "prifly.json"
    settings = json.loads(settings_path.read_text())
    executor = settings["configuration"]["executors"]["demo:step/shell"]
    executor["args"] = ["phased-step.sh"]
    executor["files"] = {"phased-step.sh": "scripts/phased-step.sh"}
    executor["timeout_ms"] = (WAIT_SECONDS + 30) * 1000
    settings_path.write_text(json.dumps(settings, ensure_ascii=False, indent=2) + "\n")

    def cli(*arguments):
        command = subprocess.run([str(binary), "--project", str(target), "--json", *arguments], capture_output=True, text=True, timeout=60)
        assert command.returncode == 0, f"CLI {arguments[:2]} exited {command.returncode}: {command.stderr[:400]}"
        return json.loads(command.stdout.strip().splitlines()[-1])

    driver = subprocess.Popen([str(binary), "--project", str(target), "--json", "run", "start", "--workflow", "workflows/shell.json",
                               "--brief", "brief.json", "--command-id", f"command:progress-{variant}", "--drive"],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        deadline, run_id, running = time.monotonic() + WAIT_SECONDS, "", None
        while time.monotonic() < deadline and running is None:
            runs = cli("run", "list")["runs"]
            run_id = runs[0]["run_id"] if runs else ""
            if run_id:
                attempts = cli("run", "progress", run_id)["attempts"]
                if attempts and attempts[0].get("phase"):
                    running = attempts[0]
            time.sleep(0.2)
        assert running, "no progress of the running program reached a second reader"
        assert running["state"] == "reported" and not running["settled"], running
        assert (running["phase"], running["current"], running["total"]) == ("product", 4200, 6994), running
        version_while_running = cli("run", "status", run_id)["run_version"]
        workspaces = list((target / ".prifly").rglob("phased-step.sh"))
        assert workspaces, "the attempt's scratch was not found"
        (workspaces[0].parent / "go").write_text("")
    finally:
        driver.wait(timeout=WAIT_SECONDS + 60)
    final = cli("run", "progress", run_id)["attempts"][0]
    assert final["phase"] == "baseline" and final["settled"] and final["rejected"] == 1, final
    status = cli("run", "status", run_id)
    attempt = next(iter(status["run"]["attempts"].values()))
    if variant == "fail":
        assert attempt.get("accepted") is None and attempt["process_outcome"]["exit_code"] == 9, attempt
        assert final["attempt_status"] != "succeeded", final
    else:
        assert attempt.get("accepted") is not None, attempt
    text = subprocess.run([str(binary), "--project", str(target), "run", "progress", run_id], capture_output=True, text=True, timeout=60)
    assert text.returncode == 0 and "phase=baseline" in text.stdout and "last report, not its outcome" in text.stdout, text.stdout
    return {"run_id": run_id, "version_while_running": version_while_running, "final_phase": final["phase"], "attempt_status": final["attempt_status"]}


if __name__ == "__main__":
    main()
