#!/usr/bin/env python3
"""Record the native macOS suspend/resume evidence for OBS-AC-06.

The script never sleeps the computer. It starts one owned local worker, waits
until it is running, and asks the operator to use the normal macOS sleep and
wake controls. It writes a new evidence file only after the worker has settled
and the observations prove the expected clock boundary.
"""

import argparse
import datetime as dt
import json
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import time


ROOT = Path(__file__).resolve().parents[1]


def utc_ms(value):
    return int(dt.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1000)


def sleep_boundary_observed(calendar_ms, executor_ms, minimum_ms):
    """Return whether the observed suspended interval meets the requested minimum."""
    return calendar_ms >= minimum_ms and calendar_ms-executor_ms >= minimum_ms


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--evidence", required=True, type=Path)
    parser.add_argument("--minimum-sleep-seconds", type=int, default=20)
    args = parser.parse_args()
    if platform.system() != "Darwin":
        parser.error("OBS-AC-06 is qualified only on the native macOS target")
    if args.minimum_sleep_seconds < 10:
        parser.error("minimum sleep must be at least 10 seconds")
    binary = args.binary.resolve(strict=True)
    evidence = args.evidence.absolute()
    if evidence.exists():
        parser.error("evidence path already exists; never overwrite an earlier qualification")

    root = Path(tempfile.mkdtemp(prefix="prifly-suspend-", dir="/private/tmp"))
    project = root / "project"
    ready, release = project / "suspend-worker-ready", project / "suspend-worker-release"
    transcript = []

    def invoke(label, *command, parse=True, timeout=60):
        result = subprocess.run([str(item) for item in command], capture_output=True, text=True, timeout=timeout)
        entry = {"label": label, "argv": [str(item) for item in command], "exit_code": result.returncode,
                 "stdout": result.stdout, "stderr": result.stderr}
        transcript.append(entry)
        if result.returncode:
            raise RuntimeError(f"{label} failed: {result.stderr.strip()}")
        return json.loads(result.stdout) if parse else result.stdout

    driver = None
    try:
        invoke("create-demo", sys.executable, "-B", ROOT / "test/fixtures/foundation/create-demo.py", "--binary", binary, "--target", project, parse=False)
        config_path = project / "prifly.json"
        config = json.loads(config_path.read_text())
        executor = config["configuration"]["executors"]["demo:step/shell"]
        executor["environment"] = {"READY_FILE": str(ready), "RELEASE_FILE": str(release)}
        executor["timeout_ms"] = 15 * 60 * 1000
        (project / "scripts/empty-step.sh").write_text(
            "#!/bin/sh\nset -eu\nprintf ready >\"$READY_FILE\"\nwhile [ ! -e \"$RELEASE_FILE\" ]; do sleep 1; done\nexec /bin/sh original-empty-step.sh\n"
        )
        shutil.copyfile(ROOT / "test/fixtures/foundation/empty-step.sh", project / "scripts/original-empty-step.sh")
        executor["files"] = {"empty-step.sh": "scripts/empty-step.sh", "original-empty-step.sh": "scripts/original-empty-step.sh"}
        config_path.write_text(json.dumps(config, indent=2) + "\n")
        started = invoke("start", binary, "--project", project, "--json", "run", "start", "--workflow", "workflows/shell.json", "--brief", "brief.json")
        run_id = started["receipt"]["run_id"]
        driver = subprocess.Popen([str(binary), "--project", str(project), "--json", "run", "drive", run_id], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        deadline = time.monotonic() + 30
        while not ready.exists():
            if driver.poll() is not None:
                stdout, stderr = driver.communicate()
                raise RuntimeError(f"driver exited before worker was ready: {stdout} {stderr}")
            if time.monotonic() >= deadline:
                raise RuntimeError("worker did not reach the owned ready marker")
            time.sleep(0.1)
        before = invoke("before-sleep", binary, "--project", project, "--json", "run", "status", run_id)
        print("Worker is running. Put this Mac to sleep now, wait at least %d seconds, wake it, then press Enter here." % args.minimum_sleep_seconds, flush=True)
        input()
        after_wake = invoke("after-wake", binary, "--project", project, "--json", "run", "status", run_id)
        if driver.poll() is not None:
            stdout, stderr = driver.communicate()
            raise RuntimeError(f"worker did not survive until the operator released it: {stdout} {stderr}")
        release.touch()
        stdout, stderr = driver.communicate(timeout=30)
        transcript.append({"label": "drive", "argv": [str(binary), "--project", str(project), "--json", "run", "drive", run_id], "exit_code": driver.returncode, "stdout": stdout, "stderr": stderr})
        if driver.returncode:
            raise RuntimeError(f"driver failed after wake: {stderr.strip()}")
        settled = invoke("settled", binary, "--project", project, "--json", "run", "status", run_id)
        attempt = next(iter(settled["run"]["attempts"].values()))
        started_at, ended_at = attempt["started"], attempt["executor_end"]
        if started_at["suspend_basis"] != "excludes_suspend_on_darwin" or started_at["session"] != ended_at["session"]:
            raise RuntimeError("worker timestamps did not retain the declared Darwin monotonic clock domain")
        calendar_ms = utc_ms(ended_at["utc"]) - utc_ms(started_at["utc"])
        executor_ms = ended_at["monotonic_ms"] - started_at["monotonic_ms"]
        minimum_ms = args.minimum_sleep_seconds * 1000
        if not sleep_boundary_observed(calendar_ms, executor_ms, minimum_ms):
            raise RuntimeError("sleep was not visible as calendar-only elapsed; inspect the retained transcript")
        report = {
            "schema_version": "suspend-recovery-evidence/1", "outcome": "passed",
            "case": "OBS-AC-06", "host": {"platform": platform.platform(), "machine": platform.machine()},
            "binary": str(binary), "run_id": run_id, "project": str(project),
            "before_sleep": before, "after_wake": after_wake, "settled": settled,
            "worker_clock": {"start": started_at, "end": ended_at, "calendar_elapsed_ms": calendar_ms, "executor_elapsed_ms": executor_ms},
            "transcript": transcript,
            "limitations": ["The operator chose the normal macOS sleep action; this script does not control power state.", "The evidence qualifies the stated native macOS target only, not another OS or a remote executor."],
        }
        evidence.parent.mkdir(parents=True, exist_ok=True)
        evidence.write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({"outcome": "passed", "evidence": str(evidence), "run_id": run_id}, indent=2))
    finally:
        if driver and driver.poll() is None:
            driver.terminate()
            driver.wait(timeout=5)


if __name__ == "__main__":
    main()
