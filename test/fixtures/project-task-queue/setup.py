#!/usr/bin/env python3
"""Create an isolated queue-intake fixture using only the selected public CLI.

This creates no claim or Run and does not qualify live host question waiting.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--target', required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    target = args.target.resolve()
    target.mkdir(parents=True, exist_ok=False)
    home = target / 'home'
    home.mkdir()
    env = {**os.environ, 'HOME': str(home), 'XDG_CONFIG_HOME': str(home / '.config')}

    root = Path(__file__).resolve().parents[3]
    subprocess.run(['python3', '-B', str(root / 'test/fixtures/project-launch-advice/setup.py'),
                    '--prifly', str(binary), '--destination', str(target / 'project'), '--variant', 'finite'],
                   check=True, capture_output=True, text=True, env=env, timeout=120)
    repository = target / 'project/repository'
    authority = target / 'project/authority'
    raw = (repository / 'original-task.txt').read_text()
    request = {
        'schema_version': 'task-input/1', 'id': 'task:queue-fixture', 'title': 'Preserve the original request',
        'raw_text': raw, 'desired_outcome': 'Prepare the selected workflow with explicit typed inputs',
        'in_scope': ['Isolated fixture only'], 'out_of_scope': ['Network and external tracker writes'],
        'completion_criteria': ['Owner records a local decision'],
        'source': {'type': 'local', 'external_id': 'fixture:queue', 'fetched_at': '2026-10-15T00:00:00Z'},
        'source_refs': [], 'assumptions': [], 'confirmation': 'explicit',
    }
    request_file = repository / 'queue-request.json'
    request_file.write_text(json.dumps(request, ensure_ascii=False, indent=2) + '\n')
    def cli(*argv):
        result = subprocess.run([str(binary), '--json', *map(str, argv)], check=True,
                                capture_output=True, text=True, env=env, timeout=60)
        return json.loads(result.stdout)
    added = cli('project', 'queue', 'add', '--repository', repository, '--input', request_file,
                '--priority', '10', '--command-id', 'command:fixture-add')
    inventory = {name: cli('--project', authority, *command) for name, command in {
        'packages': ['package', 'list'], 'claims': ['claim', 'list'], 'runs': ['run', 'list']}.items()}
    metadata = {'binary': str(binary), 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                'repository': str(repository), 'authority': str(authority), 'task_id': added['result']['id'],
                'original_request_sha256': hashlib.sha256(request_file.read_bytes()).hexdigest(),
                'inventory_before_host': inventory,
                'owner': 'session:live-queue', 'claim_command_id': 'command:live-queue-claim',
                'host_evidence': {host: 'pending' for host in ['codex-app', 'codex-cli', 'claude-code']}}
    (target / 'fixture.json').write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(metadata, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
