#!/usr/bin/env python3
"""Drive the local run monitor as a real process over a real socket.

The handlers are covered by Go tests, but nothing checked that `prifly monitor`
starts, finds an authority on disk, counts its Runs, pages them, and — through
the same HTTP surface a browser uses — deletes exactly the Runs it previewed.
The deletion path is destructive, so it is exercised here against a project this
script creates and against no other.

The monitor is given its own HOME so its source catalog cannot see, record or
touch authorities belonging to the person running this.

Requires an explicitly built binary. Keeps its temporary project for inspection.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

RUNS = 6
PAGE = 50


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def main():
    if not __debug__:
        raise RuntimeError("Verification requires enabled Python assertions")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--target", type=Path, help="absent or empty directory; otherwise a new temporary project")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    target = (args.target or Path(tempfile.mkdtemp(prefix="prifly-monitor-", dir="/tmp"))).resolve()
    if target.exists() and (not target.is_dir() or any(target.iterdir())):
        parser.error("target must be absent or empty; no existing project files were changed")
    project, home = target / "project", target / "home"
    home.mkdir(parents=True)
    fixture = Path(__file__).resolve().parents[2] / "test/fixtures/foundation/create-demo.py"
    subprocess.run([sys.executable, "-B", str(fixture), "--binary", str(binary), "--target", str(project)], check=True, timeout=120, stdout=subprocess.PIPE)
    for i in range(RUNS):
        subprocess.run([str(binary), "--project", str(project), "--json", "run", "start", "--workflow", "workflows/shell.json",
                        "--brief", "brief.json", "--command-id", f"command:monitor-{i}", "--drive"],
                       check=True, timeout=60, stdout=subprocess.DEVNULL)

    port = free_port()
    base = f"http://127.0.0.1:{port}"
    monitor = subprocess.Popen([str(binary), "monitor", "--addr", f"127.0.0.1:{port}", "--scan-root", str(target)],
                               env={**os.environ, "HOME": str(home), "XDG_CONFIG_HOME": str(home / ".config")},
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    try:
        def get(path):
            with urllib.request.urlopen(base + path, timeout=10) as response:
                return json.loads(response.read())

        def post(body, token, origin=None, expect=200):
            request = urllib.request.Request(base + "/api/maintenance", data=json.dumps(body).encode(), method="POST")
            request.add_header("Content-Type", "application/json")
            request.add_header("Origin", base if origin is None else origin)
            if token is not None:
                request.add_header("X-PriFly-Maintenance", token)
            try:
                with urllib.request.urlopen(request, timeout=180) as response:
                    status, payload = response.status, response.read()
            except urllib.error.HTTPError as refusal:
                status, payload = refusal.code, refusal.read()
            assert status == expect, f"maintenance {body.get('action')} answered {status}, wanted {expect}: {payload[:300]}"
            return json.loads(payload) if status == 200 else payload.decode(errors="replace")

        deadline, listing = time.monotonic() + 60, None
        while time.monotonic() < deadline:
            assert monitor.poll() is None, f"the monitor exited early: {monitor.stdout.read()[:400]}"
            try:
                listing = get("/api/runs?page=1")
            except (urllib.error.URLError, ConnectionError, OSError):
                time.sleep(0.3)
                continue
            if listing["total"] >= RUNS:
                break
            time.sleep(0.3)
        assert listing and listing["total"] >= RUNS, f"discovery never reported the {RUNS} Runs of the fixture: {listing}"

        # Only this script's authority may be in view: the monitor was given its
        # own HOME, so nothing else on this machine is discovered or written to.
        sources = [s for s in listing["sources"] if not s.get("error")]
        assert len(sources) == 1, f"the monitor saw more than the fixture authority: {[s['root'] for s in sources]}"
        source = sources[0]
        assert Path(source["root"]).resolve() == project, f"the discovered authority is not the fixture: {source['root']}"
        assert listing["total"] == RUNS and listing["pages"] == 1 and len(listing["runs"]) == min(RUNS, PAGE), listing["total"]
        # A page beyond the last is answered with the last, never with an error.
        assert get("/api/runs?page=99")["page"] == listing["pages"]
        assert get("/api/runs?page=1&status=completed")["filtered"] == RUNS
        assert get("/api/runs?page=1&q=" + "no-such-workflow")["filtered"] == 0

        # Maintenance is same-origin and token-bound: the read-only monitor must
        # not delete anything for a page it did not serve.
        token = get("/api/maintenance-token")["token"]
        plan_request = {"source": source["id"], "run": "", "action": "preview", "digest": ""}
        post(plan_request, None, expect=403)
        post(plan_request, "not-the-token", expect=403)
        post(plan_request, token, origin="http://example.invalid", expect=403)
        assert get("/api/runs?page=1")["total"] == RUNS, "a refused maintenance request still changed the authority"

        plan = post(plan_request, token)
        assert set(plan["runs"]) and len(plan["runs"]) == RUNS, f"the plan does not name the settled Runs: {len(plan['runs'])}"
        assert not plan["protected"], f"nothing in this fixture is protected, yet: {plan['protected']}"
        assert Path(plan["root"]).resolve() == project and plan["files"] > 0 and plan["bytes"] > 0, plan

        # A plan is what was shown to the reader; the server refuses to act on
        # any other, so an unattended page cannot delete a list nobody saw.
        stale = {**plan_request, "action": "delete", "digest": "sha256:" + "0" * 64}
        assert "cleanup_plan_changed" in post(stale, token, expect=409), "a stale plan was not refused by name"
        after_refusal = get("/api/runs?page=1")
        assert after_refusal["total"] == RUNS, f"a refused delete left {after_refusal['total']} of {RUNS} Runs"

        deleted = post({**plan_request, "action": "delete", "digest": plan["digest"]}, token)
        assert deleted["deleted_runs"] == RUNS and deleted["deleted_files"] == plan["files"], (deleted, plan["files"])
        # The list is a background index refreshed twice a second, so it catches
        # up rather than answering from the write. It must still catch up.
        emptied, deadline = None, time.monotonic() + 15
        while time.monotonic() < deadline:
            emptied = get("/api/runs?page=1")
            if emptied["total"] == 0:
                break
            time.sleep(0.2)
        assert emptied["total"] == 0, f"the list still shows {emptied['total']} Runs the server reported deleted"
        # Settings, definitions and the authority itself outlive a cleanup.
        for kept in ("definitions.json", "workflows/shell.json", "brief.json"):
            assert (project / kept).is_file(), f"cleanup removed {kept}, which it promises to keep"
        assert [s["root"] for s in get("/api/runs?page=1")["sources"]] == [source["root"]], "the emptied authority left the catalog"
    finally:
        monitor.terminate()
        try:
            monitor.wait(timeout=10)
        except subprocess.TimeoutExpired:
            monitor.kill()

    report = {"binary_sha256": binary_digest, "project": str(project), "port": port,
              "runs": RUNS, "deleted": {k: deleted[k] for k in ("deleted_runs", "deleted_files", "deleted_bytes")},
              "outcome": "passed"}
    results = target / "verification"
    results.mkdir()
    (results / "summary.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
