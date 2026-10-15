#!/usr/bin/env python3
"""Replay queue concurrency, unknown dispatch and a lost start response.

Uses only public CLI commands and declared fixture preflight programs. No DB
writes, private payloads or product crash switches. A development run is never
released qualification. Live generated-host waiting is a separate observation.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import fcntl
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--target', required=True, type=Path)
    parser.add_argument('--qualification', choices=['development', 'released'], default='development')
    parser.add_argument('--expected-sha256', help='digest of the actual release asset, recorded by the owner')
    parser.add_argument('--release-tag', help='required for released evidence; owner must verify exact release asset digest')
    args = parser.parse_args()
    if args.qualification == 'released' and (not args.release_tag or not args.expected_sha256):
        parser.error('--qualification released requires --release-tag and --expected-sha256')
    binary = args.binary.resolve(strict=True)
    digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    if args.expected_sha256 and args.expected_sha256.removeprefix('sha256:') != digest:
        parser.error('binary differs from the expected release asset digest')
    target = args.target.resolve()
    target.mkdir(parents=True, exist_ok=False)
    home = target / 'home'
    home.mkdir()
    env = {**os.environ, 'HOME': str(home), 'XDG_CONFIG_HOME': str(home / '.config')}

    root = Path(__file__).resolve().parents[2]
    subprocess.run([sys.executable, '-B', str(root / 'test/fixtures/project-task-queue/setup.py'),
                    '--binary', str(binary), '--target', str(target / 'fixture')],
                   check=True, capture_output=True, text=True, env=env, timeout=120)
    version = subprocess.run([str(binary), '--json', 'version'], check=True, capture_output=True, text=True, env=env, timeout=30)
    version = json.loads(version.stdout)
    if args.qualification == 'released' and version['version'].removeprefix('v') != args.release_tag.removeprefix('v'):
        parser.error('binary version differs from the release tag')
    fixture = json.loads((target / 'fixture/fixture.json').read_text())
    repository, authority = Path(fixture['repository']), Path(fixture['authority'])
    log = []
    def cli(*argv, refusal=None):
        process = subprocess.run([str(binary), '--json', *map(str, argv)], capture_output=True, text=True, env=env, timeout=60)
        log.append({'argv': list(map(str, argv)), 'exit': process.returncode, 'stdout': process.stdout, 'stderr': process.stderr})
        if refusal:
            assert process.returncode != 0, (argv, 'expected refusal')
            problem = json.loads(process.stderr)
            assert problem['code'] == refusal, (argv, problem)
            return problem
        assert process.returncode == 0, (argv, process.stderr)
        return json.loads(process.stdout)
    def queue(operation, *argv, **kwargs):
        return cli('project', 'queue', operation, '--repository', repository, *argv, **kwargs)
    def selection(task):
        return ['--task-id', task['id'], '--task-owner', task['claim']['owner'],
                '--task-revision', str(task['revision']), '--task-claim-generation', str(task['claim']['generation'])]
    def show(task):
        return queue('show', '--task-id', task['id'])['task']
    request = repository / 'queue-request.json'
    queue('add', '--input', request, '--priority', '10', '--command-id', 'command:second-add')
    def claim(index):
        return queue('claim-next', '--owner', f'session:host-{index}', '--command-id', f'command:claim-{index}')
    with ThreadPoolExecutor(max_workers=2) as workers:
        claimed = list(workers.map(claim, range(2)))
    tasks = [item['result'] for item in claimed]
    assert len({task['id'] for task in tasks}) == 2
    original_claim = claimed[0]['result']
    assert claim(0)['result'] == original_claim
    queue('claim-next', '--owner', 'session:changed', '--command-id', 'command:claim-0', refusal='command_conflict')
    assert show(tasks[0])['claim'] == original_claim['claim']
    assert cli('--project', authority, 'run', 'list')['runs'] == []
    # The public fixture's first launch requires a real decision and typed input.
    launch = ['--repository', repository, '--launch', 'repair-fast', '--host', 'codex-cli', *selection(tasks[0])]
    missing = subprocess.run([str(binary), '--json', 'project', 'questionnaire', '--prepare', *map(str, launch)],
                             capture_output=True, text=True, env=env, timeout=60)
    assert missing.returncode != 0
    assert cli('--project', authority, 'run', 'list')['runs'] == []
    assert cli('--project', authority, 'claim', 'list')['claims'] == []
    # An explicitly declared preflight barrier exposes a real unknown dispatch.
    script = repository / 'barrier.py'
    script.write_text('''import json, os, pathlib, time
root=pathlib.Path.cwd()
with (root/'dispatch-count').open('a') as f: f.write('dispatch\\n')
(root/'barrier-ready.json').write_text(json.dumps({'pid':os.getpid(),'pgid':os.getpgrp()}))
while not (root/'barrier-release').exists(): time.sleep(.02)
''')
    cli('project', 'local', 'set', '--repository', repository, '--allow-executable', f'fixture-python={Path(sys.executable).resolve()}')
    profile = repository / '.prifly/project.yaml'
    original_profile = profile.read_text()
    marker = '    workflow: .prifly/workflows/diagnostics/workflow.yaml\n'
    assert marker in original_profile
    profile.write_text(original_profile.replace(marker, marker + '    preflight: {executable: fixture-python, args: [barrier.py], timeout_ms: 120000}\n', 1))
    launch = ['--repository', repository, '--launch', 'diagnostics', '--host', 'codex-cli', *selection(tasks[0])]
    prepared = cli('project', 'questionnaire', '--prepare', *launch)
    start = ['project', 'start', *launch, '--command-id', 'command:unknown-dispatch', '--expected-launch-digest', prepared['review_digest']]
    stderr_file = (target / 'unknown-start.stderr').open('w')
    process = subprocess.Popen([str(binary), '--json', *map(str, start)], stdout=subprocess.PIPE, stderr=stderr_file, start_new_session=True, env=env)
    child = None
    try:
        deadline = time.monotonic() + 30
        while not (repository / 'barrier-ready.json').exists():
            assert process.poll() is None, 'start exited before the declared preflight barrier'
            assert time.monotonic() < deadline, 'preflight barrier timed out'
            time.sleep(.02)
        child = json.loads((repository / 'barrier-ready.json').read_text())
        assert child['pid'] == child['pgid'], 'declared preflight does not own its process group'
        process.kill()
        process.wait(timeout=10)
        # This fixture owns exactly this process group; no recovered runtime
        # identity is used as signalling authorization.
        try:
            os.killpg(child['pgid'], signal.SIGTERM)
        except ProcessLookupError:
            pass
        deadline = time.monotonic() + 10
        while True:
            table = subprocess.check_output(['ps', '-axo', 'pid=,pgid=,stat='], text=True, env=env, timeout=5)
            members = [line.split() for line in table.splitlines() if len(line.split()) == 3 and line.split()[1] == str(child['pgid'])]
            if not members or all(member[2].startswith('Z') for member in members):
                break
            assert time.monotonic() < deadline, 'fixture-owned preflight group has not terminated'
            time.sleep(.02)
        (repository / 'barrier-release').touch()
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=10)
        stderr_file.close()
        if process.stdout:
            process.stdout.close()
    count = (repository / 'dispatch-count').read_bytes()
    cli(*start, refusal='queue_reconcile_required')
    cli('--project', authority, 'run', 'start', '--workflow', 'unavailable-raw-workflow.json',
        '--brief', 'unavailable-raw-brief.json', '--command-id', 'command:unknown-dispatch', refusal='command_conflict')
    assert (repository / 'dispatch-count').read_bytes() == count
    assert cli('--project', authority, 'run', 'list')['runs'] == []
    unknown = show(tasks[0])
    intent = next(iter(unknown['intents'].values()))
    assert intent['state'] == 'dispatching' and not unknown['associations']
    queue('release', *selection(tasks[0]), '--reason', 'PID timeout is not settlement', '--command-id', 'command:unsafe-release', refusal='queue_task_busy')
    queue('reconcile', *selection(tasks[0]), '--intent-id', intent['id'], '--resolution', 'effects_resolved',
          '--reason', 'Fixture-owned preflight process group terminated; no other effects were declared', '--command-id', 'command:resolve-effects')
    queue('release', *selection(tasks[0]), '--reason', 'Explicitly resolved fixture dispatch', '--command-id', 'command:safe-release')
    # Known admission: block stdout after commit, then lose that response.
    # Linux can reduce the pipe. Other hosts still run every preceding case;
    # this exact lost-output case remains pending unless actually observed.
    profile.write_text(original_profile)
    launch = ['--repository', repository, '--launch', 'diagnostics', '--host', 'codex-cli', *selection(tasks[1])]
    prepared = cli('project', 'questionnaire', '--prepare', *launch)
    start = ['project', 'start', *launch, '--command-id', 'command:lost-result', '--expected-launch-digest', prepared['review_digest']]
    lost_output = 'pending'
    if hasattr(fcntl, 'F_SETPIPE_SZ'):
        read_fd, write_fd = os.pipe()
        fcntl.fcntl(write_fd, fcntl.F_SETPIPE_SZ, 4096)
        stderr_file = (target / 'known-start.stderr').open('w')
        process = subprocess.Popen([str(binary), '--json', *map(str, start)], stdout=write_fd, stderr=stderr_file, env=env)
        os.close(write_fd)
        try:
            deadline = time.monotonic() + 30
            while True:
                known = show(tasks[1])
                intents = list(known['intents'].values())
                if intents and intents[0].get('result_bytes'):
                    assert process.poll() is None, 'stdout did not block; lost-response window was not observed'
                    break
                assert process.poll() is None, 'start exited without reaching the observed result boundary'
                assert time.monotonic() < deadline, 'result boundary timed out'
                time.sleep(.02)
            process.kill()
            process.wait(timeout=10)
            lost_output = 'passed'
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=10)
            os.close(read_fd)
            stderr_file.close()
    else:
        cli(*start)
    before = show(tasks[1])
    replay = cli(*start)
    assert replay['run_id'] == before['associations'][0]['run_id']
    assert len(show(tasks[1])['associations']) == 1
    queue('complete', *selection(tasks[1]), '--reason', 'Explicit local fixture processing completed', '--command-id', 'command:complete-local')
    assert show(tasks[1])['state'] == 'done'
    report = {'qualification': args.qualification, 'release_tag': args.release_tag,
              'binary': str(binary), 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'binary_version': version, 'concurrency_and_exact_claim_replay': 'passed',
              'missing_answers_create_no_run': 'passed', 'unknown_dispatch_no_redispatch': 'passed',
              'legacy_entrypoint_cannot_spend_reserved_identity': 'passed',
              'explicit_effect_resolution_and_release': 'passed', 'lost_response_observed': lost_output,
              'existing_run_replay_and_local_completion': 'passed',
              'live_generated_hosts': {host: 'pending' for host in ['codex-app', 'codex-cli', 'claude-code']},
              'e2_a10': 'pending', 'result': 'passed' if lost_output == 'passed' else 'partial'}
    (target / 'commands.json').write_text(json.dumps(log, ensure_ascii=False, indent=2) + '\n')
    (target / 'summary.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
