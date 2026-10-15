#!/usr/bin/env python3
"""Replay the board in Chromium over saved CLI/runtime authorities.

Requires a built binary, Go for the opt-in fixture generators, and an existing
Playwright module/Chromium. Nothing is installed. All mutations affect this
replay's copied fixtures only; ordinary host authorities are outside its HOME.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--target', type=Path)
    parser.add_argument('--fixtures', type=Path, help='existing exported fixtures to copy instead of generating')
    parser.add_argument('--go', default='go')
    parser.add_argument('--playwright-module', required=True)
    parser.add_argument('--chromium', required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    binary = args.binary.resolve(strict=True)
    target = (args.target or Path(tempfile.mkdtemp(prefix='prifly-board-', dir='/tmp'))).resolve()
    if target.exists() and (not target.is_dir() or any(target.iterdir())):
        parser.error('target must be absent or empty')
    target.mkdir(parents=True, exist_ok=True)
    fixtures = target / 'authorities'
    if args.fixtures:
        shutil.copytree(args.fixtures, fixtures)
    else:
        fixtures.mkdir()
        with (target / 'fixture-generation.log').open('w') as log:
            subprocess.run([args.go, 'test', './cmd/prifly', './internal/runtime', '-run', '^TestMonitorBoardExportFixtures$', '-count=1'],
                           cwd=root, env={**os.environ, 'PRIFLY_MONITOR_FIXTURE_DIR': str(fixtures)}, stdout=log, stderr=subprocess.STDOUT, check=True, timeout=180)
    # Copies are inspected read-only. A separate authority created at its
    # final path exercises live CLI writes without rewriting relocation guards.
    live = fixtures / 'live'
    subprocess.run(['python3', '-B', str(root / 'test/fixtures/foundation/create-demo.py'), '--binary', str(binary), '--target', str(live)], check=True, stdout=subprocess.PIPE, timeout=120)
    start = subprocess.run([str(binary), '--json', '--project', str(live), 'run', 'start', '--workflow', 'workflows/shell.json', '--brief', 'brief.json', '--command-id', 'command:board-live'], check=True, capture_output=True, text=True, timeout=30)
    run_id = json.loads(start.stdout)['receipt']['run_id']
    (fixtures / 'live-fixture.json').write_text(json.dumps({'run_id': run_id}))
    home = target / 'home'
    home.mkdir()
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    base = f'http://127.0.0.1:{port}'
    with (target / 'monitor.log').open('w') as log:
        process = subprocess.Popen([str(binary), 'monitor', '--addr', f'127.0.0.1:{port}', '--scan-root', str(fixtures)],
                                   env={**os.environ, 'HOME': str(home), 'XDG_CONFIG_HOME': str(home / '.config')}, stdout=log, stderr=subprocess.STDOUT)
        try:
            deadline = time.monotonic() + 45
            while True:
                if process.poll() is not None:
                    raise RuntimeError('monitor exited; see monitor.log')
                try:
                    with urllib.request.urlopen(base + '/api/runs?view=board', timeout=10) as response:
                        listing = json.load(response)
                    if listing['total'] >= 260:
                        break
                except (urllib.error.URLError, OSError):
                    pass
                if time.monotonic() >= deadline:
                    raise RuntimeError('fixture catalog did not become complete')
                time.sleep(0.2)
            subprocess.run(['node', str(root / 'test/e2e/verify-monitor-board.cjs'), base, str(target), str(binary),
                            args.playwright_module, args.chromium], check=True, timeout=180)
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
    report = json.loads((target / 'browser.json').read_text())
    report.update(binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), fixture_boundary='Saved CLI/runtime fixtures on this source tree; no release/remote host qualification claim.')
    (target / 'summary.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
