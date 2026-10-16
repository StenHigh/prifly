#!/usr/bin/env python3
"""Prepare/collect native Call fixtures; a real host must execute the leaf task."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shlex
import time
import uuid

spec = importlib.util.spec_from_file_location('call_fixtures', Path(__file__).with_name('verify-call-extensions.py'))
fixtures = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixtures)

PATH_FIELDS = {'binary', 'root', 'repository', 'authority', 'folder', 'tree'}
FIELDS = PATH_FIELDS | {'host', 'case', 'commit_verdict', 'source_hashes', 'claim',
                       'claim_generation', 'seen', 'values', 'program', 'prepare_values', 'run'}


def read_state(path):
    state = json.loads(path.read_text())
    f = fixtures.Fixture.__new__(fixtures.Fixture)
    for key, value in state['fixture'].items():
        setattr(f, key, Path(value) if key in PATH_FIELDS else value)
    f.environment = dict(os.environ, HOME=str(f.root / 'home'))
    return state, f


def prepare(args):
    root = args.root.resolve()
    root.mkdir(parents=True, exist_ok=True)
    assert not list(root.iterdir()), 'fixture root must be empty'
    f = fixtures.Fixture(args.binary.resolve(), root, host=args.host)
    nonce = uuid.uuid4().hex
    instructions = ('Live native Call host transport check. Read this Attempt and its actual input files. '
                    'Only the already issued leaf is assigned; never start another Run. '
                    'For each output PORT write a JSON object with port=PORT, inputs=the actual '
                    'input-name to decoded JSON map, and live_nonce=' + nonce + '. '
                    'Submit pass using session submit --template and SHA256 of actual port bytes. '
                    'Do not modify the candidate tree. For a control case, take and read the task, '
                    'then observe native child cancellation; write no output and submit no result. '
                    'Cancellation is not proof that an arbitrary external actor stopped.')
    fixtures.edit(f, 'project/contexts/task.yaml', lambda d: d.update(text=instructions))
    if args.mode == 'cancel':
        fixtures.edit(f, 'project/steps/review-leaf.yaml',
                      lambda d: d.update(effects={'class': 'none', 'retry_class': 'never'}))
    fixtures.checkpoint(f)
    review = f.prepare()
    (root / 'launch-review.json').write_text(json.dumps(review, indent=2))
    f.start(review)
    for _ in range(8):
        n = f.authority_cli('run', 'next', f.run)
        if n['action'] == 'stage':
            f.authority_cli('run', 'drive', f.run)
            continue
        assert n['action'] == 'awaiting_host', n
        task = f.authority_cli('session', 'task', '--run', f.run, '--all')['tasks'][0]
        if 'request' in task['context']['inputs']:
            break
        f.submit(f.task())  # Explicit synthetic producer, never the live leaf.
    else:
        raise AssertionError('no child leaf task')
    view = f.authority_cli('run', 'status', f.run)['run']
    attempt = view['attempts'][task['attempt_id']]
    child = view['activations'][attempt['stage_activation_id']]['workflow_invocation_id']
    assert view['invocations'][child]['parent_invocation_id']
    if args.mode == 'pass':
        assert task['claim_id'] == f.claim and task['claim_generation'] == f.claim_generation
    assert not attempt['session'].get('taken'), 'coordinator must not take the live task'
    state = {'fixture': {k: str(v) if k in PATH_FIELDS else v for k, v in vars(f).items() if k in FIELDS},
             'mode': args.mode, 'nonce': nonce, 'task': task, 'child_invocation': child,
             'binary_sha256': fixtures.digest(f.binary.read_bytes())}
    path = root / 'live-state.json'
    path.write_text(json.dumps(state, indent=2))
    skills = {'codex-app': '.agents/skills', 'codex-cli': '.codex/skills', 'claude-code': '.claude/skills'}
    skill = f.tree / skills[f.host] / 'prifly-run/SKILL.md'
    command = shlex.join([str(f.binary), '--project', str(f.authority)])
    prompt = f'''Perform one live Pri-Fly host qualification task, host {f.host}.
Read generated runner {skill}, then the already materialized task {task['workspace']}/task.json
and all its pinned context/skills and actual input files. Authority CLI prefix: {command}.
Run is {f.run}; only Attempt {task['attempt_id']} is assigned. Take it once using session take.
Never launch project start, invent a second Run, edit source/candidate/Git, or consume other tasks.
This disposable fixture and its port writes are explicitly authorized for live qualification.
'''
    if args.mode == 'pass':
        prompt += '''Write every declared output as the pinned instruction requires. Use the real input JSON,
port name and pinned live_nonce; obtain session submit --template, hash actual output bytes, set pass
and answered_questions (truthfully), submit --file. After acceptance read run next once and stop.
Return a brief JSON summary identifying Run/Attempt and acceptance. No implementation/commit work.
'''
    else:
        prompt += '''CONTROL CASE: after take, read the actual inputs, but do not write any output or submit a result.
The controller will cancel this child invocation after observing your native take record.
Poll run next until action blocked_child (allow up to 120 seconds); inspect run status to confirm
the child/Attempt cancellation and absence of output_refs. Stop and return JSON with observations.
Do not drive, resume, resolve or cancel anything yourself. Do not claim native cancel stopped an
arbitrary external actor. This controlled host process should exit cooperatively after observing it.
'''
    (root / 'host-prompt.txt').write_text(prompt)
    print(json.dumps({'state': str(path), 'prompt': str(root / 'host-prompt.txt'),
                      'run': f.run, 'attempt': task['attempt_id'], 'workspace': str(f.tree)}, indent=2))


def cancel_after_take(path):
    state, f = read_state(path)
    assert state['mode'] == 'cancel'
    for _ in range(600):
        view = f.authority_cli('run', 'status', f.run)['run']
        attempt = view['attempts'][state['task']['attempt_id']]
        if attempt['session'].get('taken'):
            break
        time.sleep(.2)
    else:
        raise AssertionError('live host did not take the task')
    f.authority_cli('run', 'cancel', f.run, '--invocation', state['child_invocation'],
                    '--reason', 'live host qualification controlled child cancellation')
    f.authority_cli('run', 'drive', f.run)
    n = f.authority_cli('run', 'next', f.run)
    assert n['action'] == 'blocked_child', n
    (f.root / 'controller-next.json').write_text(json.dumps(n, indent=2))
    print('native take observed; child cancelled; caller blocked_child')


def collect(path):
    state, f = read_state(path)
    task = state['task']
    view = f.authority_cli('run', 'status', f.run)['run']
    attempt = view['attempts'][task['attempt_id']]
    assert attempt['session'].get('taken'), 'missing real host take'
    result = {'host': f.host, 'mode': state['mode'], 'run_id': f.run,
              'attempt_id': task['attempt_id'], 'child_invocation': state['child_invocation'],
              'claim_id': f.claim, 'claim_generation': f.claim_generation,
              'taken': attempt['session']['taken'], 'binary_sha256': state['binary_sha256']}
    if state['mode'] == 'pass':
        assert attempt.get('accepted'), 'real host did not submit an accepted candidate'
        inputs = {n: json.loads((Path(task['workspace']) / p['path']).read_text())
                  for n, p in task['context']['inputs'].items()}
        for name, port in task['context']['outputs'].items():
            value = json.loads((Path(task['workspace']) / port['path']).read_text())
            assert value == {'port': name, 'inputs': inputs, 'live_nonce': state['nonce']}, value
            f.values[name] = value
        f.seen.append('assessment')
        result.update(f.finish())  # Synthetic downstream validates exact live-produced exports.
        result['accepted'] = attempt['accepted']
    else:
        assert f.authority_cli('run', 'next', f.run)['action'] == 'blocked_child'
        child = view['invocations'][state['child_invocation']]
        assert child['status'] == 'cancelled' and not child['output_refs']
        assert not attempt.get('accepted') and not attempt['session'].get('reported')
        f.authority_cli('run', 'cancel', f.run, '--reason', 'settle live host control fixture')
        f.authority_cli('run', 'drive', f.run)
        ended = f.authority_cli('run', 'status', f.run)['run']
        assert ended['status'] == 'cancelled' and not ended['active_attempt_ids']
        result.update(caller_action='blocked_child', final_status='cancelled',
                      no_exports=True, no_fabricated_report=True, external_actor_stop_proven=False)
    assert f.source_hashes == fixtures.snapshot(f.folder)
    assert (f.tree / 'candidate.txt').read_text() == 'same candidate\n'
    assert fixtures.digest(f.binary.read_bytes()) == state['binary_sha256']
    final = f.authority_cli('run', 'status', f.run)
    (f.root / 'final-run.json').write_text(json.dumps(final, indent=2))
    (f.root / 'live-result.json').write_text(json.dumps(result, indent=2))
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    p = sub.add_parser('prepare')
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--root', type=Path, required=True)
    p.add_argument('--host', choices=['codex-app', 'codex-cli', 'claude-code'], required=True)
    p.add_argument('--mode', choices=['pass', 'cancel'], required=True)
    for name in ['cancel-after-take', 'collect']:
        sub.add_parser(name).add_argument('--state', type=Path, required=True)
    args = parser.parse_args()
    if args.action == 'prepare':
        prepare(args)
    elif args.action == 'cancel-after-take':
        cancel_after_take(args.state)
    else:
        collect(args.state)
