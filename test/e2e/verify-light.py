#!/usr/bin/env python3
"""Actual local Chromium replay of Light and full monitor over native fixtures.

Uses only an already built binary, an existing Chromium/Playwright and exported
throwaway authorities. Production hosts and consumer packages are never changed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import time
import urllib.request


def port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--target', type=Path, required=True)
    parser.add_argument('--playwright-module', required=True)
    parser.add_argument('--chromium', required=True)
    args = parser.parse_args()
    target = args.target.resolve()
    binary = args.binary.resolve()
    fixtures = target / 'authorities'
    for name in ['performance', 'typed-review', 'queue-continuations', 'long-history']:
        if not (fixtures / name / '.prifly/installation.json').exists():
            parser.error(f'missing exported native fixture: {name}')
    home = target / 'home'
    registry = home / '.prifly/monitor/sources'
    registry.mkdir(parents=True, exist_ok=True)
    for folder in fixtures.iterdir():
        if not folder.is_dir() or not (folder / '.prifly/installation.json').exists():
            continue
        key = hashlib.sha256(str(folder.resolve()).encode()).hexdigest()
        (registry / key).write_text(str(folder.resolve()))
    mapping = json.loads((fixtures / 'typed-review-mapping.json').read_text())
    feature_mapping = fixtures / 'feature-mapping.json'
    if feature_mapping.exists():
        mapping['workflows'] += json.loads(feature_mapping.read_text())['workflows']
    combined_mapping = target / 'replay-mapping.json'
    combined_mapping.write_text(json.dumps(mapping))
    full_port, light_port = port(), port()
    full_base = f'http://127.0.0.1:{full_port}/'
    light_base = f'http://127.0.0.1:{light_port}/light/'
    processes, logs = [], []
    env = {**os.environ, 'HOME': str(home), 'XDG_CONFIG_HOME': str(home / '.config')}
    try:
        for mode, addr in [('full', full_port), ('light', light_port)]:
            command = [str(binary), '--project', str(fixtures / 'performance'), 'monitor', '--addr', f'127.0.0.1:{addr}']
            if mode == 'light':
                command += ['--ui', 'light', '--base-path', '/light/', '--full-monitor-url', full_base,
                            '--light-projection', str(combined_mapping)]
            log = (target / f'{mode}-server.log').open('w')
            logs.append(log)
            processes.append(subprocess.Popen(command, env=env, stdout=log, stderr=subprocess.STDOUT))
        deadline = time.monotonic() + 90
        while True:
            if any(p.poll() is not None for p in processes):
                raise RuntimeError('monitor exited; inspect local server logs')
            try:
                with urllib.request.urlopen(light_base + 'api/light/board', timeout=3) as response:
                    body = json.load(response)
                if body['page']['complete'] and (body['page']['total'] or 0) >= 2003:
                    break
            except (OSError, ValueError):
                pass
            if time.monotonic() > deadline:
                raise RuntimeError('native fixture index did not become complete')
            time.sleep(.25)
        subprocess.run(['node', str(Path(__file__).with_suffix('.cjs')), light_base, full_base, str(target),
                        args.playwright_module, args.chromium], check=True, timeout=180)
    finally:
        for process in processes:
            process.terminate()
        for process in processes:
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        for log in logs:
            log.close()


if __name__ == '__main__':
    main()
