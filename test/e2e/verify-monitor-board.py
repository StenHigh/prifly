#!/usr/bin/env python3
"""Replay Full in Chromium over saved CLI/runtime authorities and isolated Runs.

Requires a built binary, Go for the opt-in fixture generators, and an existing
Playwright module/Chromium. Nothing is installed. All mutations affect this
replay's copied fixtures only; ordinary host authorities are outside its HOME.
"""
import argparse
from contextlib import ExitStack
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


def command_json(binary, *args, env=None):
    result = subprocess.run([str(binary), '--json', *args], capture_output=True, text=True, timeout=30, env=env)
    if result.returncode:
        raise RuntimeError(f'{args[:4]}: {result.stderr.splitlines()[-1:]}')
    return json.loads(result.stdout)


def run_cli(binary, authority, *args, env=None):
    return command_json(binary, '--project', str(authority), *args, env=env)


def prepare_observation(binary, target, fixtures, live, env):
    # The same saved Run from another physical source must keep its own card.
    shutil.copytree(fixtures / 'uncertain', fixtures / 'uncertain-copy')
    phases = {'a': ('build-a', 'verify-a'), 'b': ('compare-b', 'publish-b')}
    fixtures_data = {}
    for variant, (first, second) in phases.items():
        authority = fixtures / f'observe-{variant}'
        project = target / f'fixture-project-{variant}'
        project.mkdir()
        command_json(binary, 'project', 'init', '--repository', str(project),
                     '--state-root', str(authority), env=env)
        command_json(binary, 'capacity', 'set', '--project', str(authority), '--capacity', '1',
                     '--reason', 'Isolate browser observation fixture', env=env)
        command_json(binary, 'project', 'local', 'set', '--repository', str(project),
                     '--allow-executable', 'shell=/bin/sh', env=env)
        package = project / '.prifly/workflows/observe'
        (package / 'steps').mkdir(parents=True)
        (project / '.prifly/project.yaml').write_text("""schema_version: prifly-project-profile/4
packages:
  observe: {source: .prifly/workflows/observe}
launches:
  observe:
    title: Observation fixture
    description: Run a local program with observable progress.
    kind: workflow
    workflow: .prifly/workflows/observe/workflow.yaml
""")
        (package / 'workflow.yaml').write_text("""authoring: prifly-project-workflow/1
package:
  id: observe:package/fixture
  version: 1.0.0
  description: Local browser observation fixture
  requires_core_protocol: "1"
  references:
    adapter: core:adapter/local-process@2.0.0
    policy: core:policy/local@3.0.0
    result: core:schema/step-result@1.0.0
id: observe:workflow/fixture
version: 1.0.0
title: Observation fixture
inputs: {}
outputs: {}
entry: observe
policy_ref: "{{policy}}"
limits: {max_step_instances: 1, max_control_transitions: 2}
stages:
  observe: {kind: step, step_ref: "{{step_observe}}", input_bindings: {}, on: {pass: done}}
  done: {kind: finish, outcome: succeeded, output_bindings: {}}
execution_bindings:
  steps:
    observe:step/observe:
      executable: shell
      args: [observe-step.sh]
      files: {observe-step.sh: observe-step.sh}
      timeout_ms: 200000
      grace_ms: 100
      max_output_bytes: 1048576
      live_output: true
""")
        (package / 'steps/observe.yaml').write_text("""authoring: prifly-step/1
id: observe:step/observe
version: 1.0.0
title: Browser observation
kind: worker
inputs: {}
outputs: {}
executor: {adapter_ref: "{{adapter}}", operation: process}
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
""")
        script = package / 'observe-step.sh'
        ending = 'exit 9' if variant == 'a' else r'''printf '{"schema_version":"1","run_id":"%s","step_instance_id":"%s","attempt_id":"%s","envelope_digest":"%s","verdict":"pass","outputs":{},"evidence_refs":[],"effect_receipt_refs":[],"summary":"Observation fixture completed."}\n' \
 "$PRIFLY_RUN_ID" "$PRIFLY_STEP_ID" "$PRIFLY_ATTEMPT_ID" "$PRIFLY_ENVELOPE_DIGEST" >&3'''
        def progress(phase, count):
            report = json.dumps({"schema_version": "program-progress/1", "phase": phase,
                                 "current": count, "total": 7})
            return f"printf '%s\\n' '{report}' >&4"

        advance = f"wait_for advance\n{progress(second, 4)}" if variant == 'a' else ''
        finish = progress(second, 7) if variant == 'b' else ''
        script.write_text(f"""#!/bin/sh
set -eu
cat >/dev/null
test "$PRIFLY_PROGRESS_FD" = 4
wait_for() {{
  i=0
  while [ ! -f "$1" ]; do
    [ "$i" -lt 1800 ] || exit 21
    sleep 0.1
    i=$((i+1))
  done
}}
{progress(first, 2)}
printf 'Output from Run {variant.upper()}\\n'
{advance}
wait_for finish
{finish}
{ending}
""")
        brief_path = project / 'brief.json'
        brief = json.loads((live / 'brief.json').read_text())
        brief['subject'] = f'Наблюдение · Run {variant.upper()}'
        brief_path.write_text(json.dumps(brief, ensure_ascii=False) + '\n')
        launch = ['--repository', str(project), '--launch', 'observe', '--brief', str(brief_path),
                  '--allow-execution']
        review = command_json(binary, 'project', 'questionnaire', '--prepare', *launch, env=env)
        fixtures_data[variant] = {'authority': str(authority), 'phases': [first, second],
                                  'command': ['project', 'start', *launch, '--expected-launch-digest',
                                              review['review_digest']]}
    shell_start = ('run', 'start', '--workflow', 'workflows/shell.json', '--brief', 'brief.json')
    history = run_cli(binary, live, *shell_start, '--command-id', 'command:observe-history', env=env)['receipt']['run_id']
    for n in range(69):
        run_cli(binary, live, 'run', 'pause', history, '--reason', 'Browser journal fixture',
                '--command-id', f'command:observe-history-pause-{n}', env=env)
        view = run_cli(binary, live, 'run', 'status', history, env=env)
        stop = next(s for s in view['run']['stops'] if s['status'] == 'active')
        run_cli(binary, live, 'run', 'release', history, '--expected-epoch', str(view['run']['control_epoch']),
                '--stop', f"{stop['id']}:{stop['generation']}", '--reason', 'Browser journal fixture',
                '--command-id', f'command:observe-history-release-{n}', env=env)
        view = run_cli(binary, live, 'run', 'status', history, env=env)
        run_cli(binary, live, 'run', 'resume', history, '--expected-version', str(view['run_version']),
                '--reason', 'Browser journal fixture', '--command-id', f'command:observe-history-resume-{n}', env=env)
    fixtures_data['history'] = history
    (target / 'observation-fixtures.json').write_text(json.dumps(fixtures_data))
    return [fixtures_data[variant] for variant in phases]


def stop_process(process):
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--scenario', choices=('board', 'observe'), default='board')
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--target', type=Path, help='absent or empty directory; observe requires a target outside Git checkouts')
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
    home = target / 'home'
    home.mkdir()
    env = {**os.environ, 'HOME': str(home), 'XDG_CONFIG_HOME': str(home / '.config')}
    # Copies are inspected read-only. A separate authority created at its
    # final path exercises live CLI writes without rewriting relocation guards.
    live = fixtures / 'live'
    subprocess.run(['python3', '-B', str(root / 'test/fixtures/foundation/create-demo.py'), '--binary', str(binary), '--target', str(live)], check=True, stdout=subprocess.PIPE, timeout=120, env=env)
    start = subprocess.run([str(binary), '--json', '--project', str(live), 'run', 'start', '--workflow', 'workflows/shell.json', '--brief', 'brief.json', '--command-id', 'command:board-live'], check=True, capture_output=True, text=True, timeout=30, env=env)
    run_id = json.loads(start.stdout)['receipt']['run_id']
    (fixtures / 'live-fixture.json').write_text(json.dumps({'run_id': run_id}))
    observations = prepare_observation(binary, target, fixtures, live, env) if args.scenario == 'observe' else []
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    base = f'http://127.0.0.1:{port}'
    with (target / 'monitor.log').open('w') as log:
        process = subprocess.Popen([str(binary), 'monitor', '--addr', f'127.0.0.1:{port}', '--scan-root', str(fixtures)],
                                   env=env, stdout=log, stderr=subprocess.STDOUT)
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
            with ExitStack() as owned:
                drivers = []
                for run in observations:
                    log_path = target / f"driver-{Path(run['authority']).name}.log"
                    log_file = owned.enter_context(log_path.open('w'))
                    driver = subprocess.Popen([str(binary), '--json', *run['command']], env=env,
                                              stdout=log_file, stderr=subprocess.STDOUT)
                    owned.callback(stop_process, driver)
                    drivers.append(driver)
                script = 'verify-monitor-observation.cjs' if observations else 'verify-monitor-board.cjs'
                subprocess.run(['node', str(root / 'test/e2e' / script), base, str(target), str(binary),
                                args.playwright_module, args.chromium], check=True, timeout=180, env=env)
                for driver in drivers:
                    driver.wait(timeout=15)
                    if driver.returncode not in (0, 1, 4):
                        raise RuntimeError(f'Unexpected fixture driver exit {driver.returncode}')
        finally:
            stop_process(process)
    report = json.loads((target / 'browser.json').read_text())
    report.update(binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), fixture_boundary='Saved CLI/runtime fixtures on this source tree; no release/remote host qualification claim.')
    if observations:
        report['fixture_boundary'] = 'Two simultaneous native program Runs in temporary authorities; only the browser read failure is simulated.'
        report['owned_processes_exited'] = all(p.poll() is not None for p in [process, *drivers])
    (target / 'summary.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
