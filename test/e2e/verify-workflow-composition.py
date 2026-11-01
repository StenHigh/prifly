#!/usr/bin/env python3
"""Synthetic source composition acceptance through the public CLI; no real browser or publication."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


ROOT = Path(__file__).resolve().parents[2]
transport = module('composition_transport', ROOT / 'test/e2e/verify-call-extensions.py')
source = module('composition_source', ROOT / 'test/fixtures/workflow-composition/setup.py')


class Fixture(transport.Fixture):
    def __init__(self, binary, root, operator='parallel', mode='required'):
        self.binary, self.root, self.host = binary, root, 'codex-app'
        self.repository, self.authority = root / 'repository', root / 'authority'
        self.environment = dict(os.environ, HOME=str(root / 'home'))
        (root / 'home').mkdir()
        self.repository.mkdir()
        self.git('init', '-q')
        self.cli('project', 'init', '--repository', self.repository, '--state-root', self.authority)
        source.build_repository(self.repository, operator)
        self.cli('project', 'runners', 'add', '--repository', self.repository, '--host', self.host)
        self.cli('project', 'local', 'set', '--repository', self.repository,
                 '--allow-executable', 'fixture-python=' + str(Path(sys.executable).resolve()))
        self.git('add', '.')
        self.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                 '-c', 'commit.gpgsign=false', 'commit', '-qm', 'composition fixture')
        self.folder = self.repository / '.prifly/workflows/smsplace'
        self.mode, self.operator = mode, operator
        self.task_body = dict(request='Update the interface rendered by templates/view.php', fixture_mode=mode)
        self.task_file = root / 'task.json'
        self.task_file.write_text(json.dumps(self.task_body))
        self.commit_verdict = 'pass'
        self.candidate_version = 1
        self.seen = []
        self.claim = None
        self.expected_outcome = None
        self.audit_expected = 'audit'

    def args(self):
        return ['--repository', self.repository, '--launch', 'smsplace', '--host', self.host,
                '--input', 'task=' + str(self.task_file), '--allow-execution']

    def prepare(self):
        before, files = transport.snapshot(self.authority), transport.snapshot(self.repository / '.prifly')
        review = self.authority_cli('project', 'questionnaire', '--prepare', *self.args())
        assert before == transport.snapshot(self.authority), 'prepare mutated authority'
        assert files == transport.snapshot(self.repository / '.prifly'), 'prepare mutated sources'
        assert review['schema_version'] == 'project-launch-summary/6'
        assert review['composition']['parents'] == ['classic-ui', 'classic-audit']
        assert review['composition']['sources'].count('.prifly/workflows/classic') == 1
        assert any('worker.py' in json.dumps(x) for x in review['execution'])
        return review

    def start(self, review=None):
        review = review or self.prepare()
        started = self.authority_cli('project', 'start', *self.args(),
                                     '--expected-launch-digest', review['review_digest'])
        assert started['schema_version'] == 'project-start/6'
        self.run = started['run_id']
        return started

    def view(self):
        return self.authority_cli('run', 'status', self.run)['run']

    def role(self, task):
        return self.view()['steps'][task['step_instance_id']]['definition_ref']['id'].split('/')[-1]

    def submit(self, task, verdict='pass'):
        role = self.role(task)
        self.seen.append(role)
        workspace = Path(task['workspace'])
        inputs = {name: json.loads((workspace / port['path']).read_text())
                  for name, port in task['context']['inputs'].items()}
        values = {}
        if role == 'prepare':
            assert inputs['task'] == self.task_body
            self.claim = task['claim_id']
            self.tree = Path(task['repository_workspace'])
            candidate = self.tree / 'templates/view.php'
            candidate.parent.mkdir(exist_ok=True)
            data = ('<button>Candidate ' + str(self.candidate_version) + '</button>\n').encode()
            candidate.write_bytes(data)
            self.implementation = dict(path='templates/view.php', digest=transport.digest(data),
                                       candidate_version=self.candidate_version)
            values['implementation'] = self.implementation
        elif role in ['applicability', 'reader']:
            assert inputs['task'] == self.task_body, 'original task lost'
            assert inputs['implementation'] == self.implementation, 'candidate evidence lost'
            assert transport.digest((self.tree / 'templates/view.php').read_bytes()) == self.implementation['digest']
            if task.get('claim_id'):
                assert task['claim_id'] == self.claim, 'child claim changed'
            if role == 'applicability':
                state = 'required' if self.mode in ['unavailable', 'partial'] else self.mode
                values['decision'] = dict(state=state, reason='Synthetic applicability for ' + self.implementation['digest'])
            else:
                values['report'] = dict(synthetic=True, task=inputs['task'], implementation=inputs['implementation'])
        report = self.authority_cli('session', 'submit', '--template', '--run', self.run,
                                    '--attempt', task['attempt_id'])
        if role == 'reader' and self.mode in ['unavailable', 'partial']:
            verdict = 'fail' if self.mode == 'unavailable' else 'needs_revision'
        report['result']['verdict'] = self.commit_verdict if role == 'commit' else verdict
        report['result']['summary'] = 'Synthetic ' + role + ' (' + self.mode + '); no browser, commit or external publication.'
        report['answered_questions'] = []
        if report['result']['verdict'] == 'pass':
            for name, slot in task['context']['outputs'].items():
                data = (json.dumps(values[name], sort_keys=True) + '\n').encode()
                (workspace / slot['path']).write_bytes(data)
                report['result']['outputs'][name]['digest'] = transport.digest(data)
        else:
            report['result']['outputs'] = {}
        path = self.root / 'submission.json'
        path.write_text(json.dumps(report))
        self.authority_cli('session', 'submit', '--file', path)

    def finish(self):
        for _ in range(120):
            next_action = self.authority_cli('run', 'next', self.run)
            if next_action['action'] == 'terminal':
                break
            if next_action['action'] == 'awaiting_host':
                self.submit(self.task())
            else:
                assert next_action['action'] == 'stage', next_action
                self.authority_cli('run', 'drive', self.run)
        else:
            raise AssertionError('composition did not settle')
        view = self.view()
        identities = [step['definition_ref']['id'].split('/')[-1] for step in view['steps'].values()]
        assert identities.count('prepare') == identities.count('tests') == 1, identities
        assert self.seen.count('applicability') == 1, self.seen
        assert self.seen.count('reader') == (self.mode in ['required', 'unavailable', 'partial']), self.seen
        expected = 'rejected' if self.mode in ['unknown', 'unavailable', 'partial'] else 'partial' if self.commit_verdict != 'pass' else 'succeeded'
        expected = self.expected_outcome or expected
        assert view['outcome'] == expected, (view['outcome'], identities)
        assert identities.count('commit') == (expected in ['succeeded', 'partial']), identities
        assert identities.count('draft-mr') == (expected == 'succeeded'), identities
        unselected = 'audit' if self.audit_expected == 'audit-alt' else 'audit-alt'
        assert unselected not in identities, 'choice executed its unselected alternative'
        if expected in ['succeeded', 'partial']:
            assert identities.count(self.audit_expected) == 1, 'mandatory audit did not execute exactly once'
            assert self.seen.count(self.audit_expected) == 1, 'mandatory audit was not handed to the host'
        invocations = view['invocations']
        assert len([v for v in invocations.values() if not v.get('parent_invocation_id')]) == 1
        assert all(v['run_id'] == self.run for v in invocations.values())
        assert not view['active_attempt_ids']
        return dict(operator=self.operator, mode=self.mode, outcome=view['outcome'],
                    run_id=self.run, invocations=len(invocations), steps=identities,
                    applicability_calls=self.seen.count('applicability'), synthetic=True)


def checkpoint(fixture):
    fixture.git('add', '.')
    fixture.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                '-c', 'commit.gpgsign=false', 'commit', '--allow-empty', '-qm', 'fixture update')


def edit(fixture, relative, update):
    path = fixture.repository / relative
    document = json.loads(path.read_text())
    update(document)
    path.write_text(json.dumps(document, indent=2) + '\n')


def freshness(binary):
    changes = {
        'parent': ('.prifly/workflows/classic/workflow.yaml', lambda d: d.update(title='Changed parent')),
        'provider': ('.prifly/workflows/ui/workflows/review-for-change.yaml', lambda d: d.update(title='Changed public component')),
        'layer': ('.prifly/layers/delivery.yaml', lambda d: d.update(resources=dict(machine=dict(limit=1, stages=['tests'])))),
        'group': ('.prifly/workflows/smsplace/extend.yaml', lambda d: d['arrange'][0].update(sequence=d['arrange'][0].pop('parallel'))),
        'import': ('.prifly/workflows/smsplace/workflow.yaml', lambda d: d.update(imports=dict(extra='delivery'))),
    }
    results = []
    with tempfile.TemporaryDirectory(prefix='prifly-composition-freshness-') as tmp:
        fixture = Fixture(binary, Path(tmp))
        for name, (relative, update) in changes.items():
            review = fixture.prepare()
            path = fixture.repository / relative
            original = path.read_bytes()
            edit(fixture, relative, update)
            checkpoint(fixture)
            before = transport.snapshot(fixture.authority)
            result = fixture.raw('--project', fixture.authority, 'project', 'start', *fixture.args(),
                                 '--expected-launch-digest', review['review_digest'])
            assert result.returncode and json.loads(result.stderr)['code'] == 'project_start_stale_launch', (name, result.stderr)
            assert before == transport.snapshot(fixture.authority), 'stale start mutated authority'
            path.write_bytes(original)
            checkpoint(fixture)
            results.append(dict(change=name, code='project_start_stale_launch', effects=False))
        review = fixture.prepare()
        path = fixture.repository / '.prifly/workflows/delivery/programs/worker.py'
        path.write_text(path.read_text() + '# changed supporting bytes\n')
        checkpoint(fixture)
        result = fixture.raw('--project', fixture.authority, 'project', 'start', *fixture.args(),
                             '--expected-launch-digest', review['review_digest'])
        assert result.returncode and json.loads(result.stderr)['code'] == 'project_start_stale_launch', result.stderr
        results.append(dict(change='program supporting bytes', code='project_start_stale_launch', effects=False))
    return results


def refusals(binary):
    changes = {
        'missing export': ('.prifly/workflows/ui/workflow.yaml', lambda d: d.update(exports=dict(workflows=dict(renamed='{{workflow_review-for-change}}'))), 'project_composition_private_export'),
        'duplicate contribution': ('.prifly/workflows/smsplace/extend.yaml', lambda d: d['arrange'][0].update(parallel=['classic-ui.interface-review', 'classic-ui.interface-review']), 'project_composition_duplicate_contribution'),
        'missing contribution': ('.prifly/workflows/smsplace/extend.yaml', lambda d: d['arrange'][0].update(parallel=['classic-ui.interface-review']), 'project_composition_arrange_required'),
        'unknown contribution': ('.prifly/workflows/smsplace/extend.yaml', lambda d: d['arrange'][0].update(parallel=['classic-ui.interface-review', 'missing.audit']), 'project_composition_unknown_contribution'),
        'sibling output': ('.prifly/workflows/classic-ui/extend.yaml', lambda d: d['extensions'][0]['input_bindings'].update(implementation='$stages.test-audit.result'), 'project_composition_binding_scope'),
        'missing root input': ('.prifly/workflows/classic-ui/extend.yaml', lambda d: d['extensions'][0]['input_bindings'].update(task='$inputs.missing'), 'project_composition_binding_scope'),
        'unmapped partial': ('.prifly/workflows/classic-ui/extend.yaml', lambda d: d['extensions'][0]['on'].pop('partial'), 'project_extension_call_outcome_unmapped'),
        'parallel capacity': ('.prifly/workflows/smsplace/workflow.yaml', lambda d: d['limits'].update(max_parallelism=1), 'unsupported'),
        'moved resource scope': ('.prifly/workflows/classic-ui/extend.yaml', lambda d: d.update(resources=dict(browser=dict(limit=1, stages=['interface-review']))), 'project_resource_stage_unknown'),
        'program allowlist': ('.prifly/workflows/delivery/workflow.yaml', lambda d: d['execution_bindings']['steps']['fixture:step/tests'].update(executable='not-allowed'), 'project_execution_not_allowed'),
    }
    results = []
    with tempfile.TemporaryDirectory(prefix='prifly-composition-refusals-') as tmp:
        fixture = Fixture(binary, Path(tmp))
        for name, (relative, update, expected) in changes.items():
            path = fixture.repository / relative
            original = path.read_bytes()
            edit(fixture, relative, update)
            checkpoint(fixture)
            before = transport.snapshot(fixture.authority)
            refused = fixture.raw('--project', fixture.authority, 'project', 'questionnaire', '--prepare', *fixture.args())
            assert refused.returncode and json.loads(refused.stderr)['code'] == expected, (name, refused.stderr)
            assert before == transport.snapshot(fixture.authority), (name, 'refusal mutated authority')
            path.write_bytes(original)
            checkpoint(fixture)
            results.append(dict(case=name, code=expected, effects=False))
        args = fixture.args()
        args.remove('--allow-execution')
        refused = fixture.raw('--project', fixture.authority, 'project', 'questionnaire', '--prepare', *args)
        assert refused.returncode and json.loads(refused.stderr)['code'] == 'project_execution_approval_required', refused.stderr
        results.append(dict(case='program consent', code='project_execution_approval_required', effects=False))
    return results


def limits(binary):
    results = []
    for case in ['parent depth', 'group depth', 'source folders', 'layer count', 'document bytes']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-limit-') as tmp:
            fixture = Fixture(binary, Path(tmp))
            if case == 'parent depth':
                registrations = {}
                previous = 'classic'
                for index in range(9):
                    name = 'ancestor-' + str(index)
                    registrations[name] = dict(source='.prifly/workflows/' + name)
                    declaration = dict(authoring='prifly-project-workflow/2', package=source.package(name),
                                       id='fixture:workflow/' + name, version='1.0.0', title=name,
                                       extends=previous, limits=source.LIMITS)
                    source.write(fixture.repository / '.prifly/workflows' / name, 'workflow.yaml', declaration)
                    previous = name
                edit(fixture, '.prifly/project.yaml', lambda d: d['packages'].update(registrations))
                edit(fixture, '.prifly/workflows/smsplace/workflow.yaml', lambda d: d.update(extends=previous))
            elif case == 'group depth':
                member = 'classic-audit.test-audit'
                for _ in range(8):
                    member = dict(sequence=[member])
                edit(fixture, '.prifly/workflows/smsplace/extend.yaml',
                     lambda d: d['arrange'][0].update(parallel=['classic-ui.interface-review', member]))
            elif case == 'source folders':
                registrations, imports = {}, {}
                for index in range(33):
                    name = 'library-' + str(index)
                    registrations[name] = dict(source='.prifly/workflows/' + name)
                    imports['lib-' + str(index)] = name
                    source.write(fixture.repository / '.prifly/workflows' / name, 'workflow.yaml', source.root_workflow(name))
                edit(fixture, '.prifly/project.yaml', lambda d: d['packages'].update(registrations))
                edit(fixture, '.prifly/workflows/smsplace/workflow.yaml', lambda d: d.update(imports=imports))
            else:
                layers = []
                count = 33 if case == 'layer count' else 1
                for index in range(count):
                    name = 'limit-' + str(index) + '.yaml'
                    content = 'authoring: prifly-extension/3\n'
                    if case == 'document bytes':
                        content += '#' + 'x' * (2 << 20) + '\n'
                    source.write(fixture.repository / '.prifly/layers', name, content)
                    layers.append('../../layers/' + name)
                edit(fixture, '.prifly/workflows/smsplace/extend.yaml', lambda d: d.update(layers=layers))
            before = transport.snapshot(fixture.authority)
            refused = fixture.raw('--project', fixture.authority, 'project', 'questionnaire', '--prepare', *fixture.args())
            assert refused.returncode and json.loads(refused.stderr)['code'] == 'project_composition_limit', (case, refused.stderr)
            assert before == transport.snapshot(fixture.authority), (case, 'limit refusal mutated authority')
            results.append(dict(case=case, code='project_composition_limit', effects=False))
    return results


def parallel_wait(binary, mode):
    with tempfile.TemporaryDirectory(prefix='prifly-composition-wait-') as tmp:
        fixture = Fixture(binary, Path(tmp), 'parallel', mode)
        fixture.start()
        held = None
        for _ in range(60):
            next_action = fixture.authority_cli('run', 'next', fixture.run)
            if next_action['action'] == 'stage':
                fixture.authority_cli('run', 'drive', fixture.run)
                continue
            assert next_action['action'] == 'awaiting_host', next_action
            tasks = fixture.authority_cli('session', 'task', '--run', fixture.run, '--all')['tasks']
            other = []
            for task in tasks:
                if fixture.role(task) == 'audit':
                    held = task
                else:
                    other.append(task)
            if held and not other:
                break
            for task in other:
                fixture.authority_cli('session', 'take', '--run', fixture.run, '--attempt', task['attempt_id'])
                fixture.submit(task)
        else:
            raise AssertionError('no outstanding parallel audit')
        view = fixture.view()
        assert view['outcome'] is None and view['active_attempt_ids'], view['status']
        assert 'fixture:step/commit' not in [step['definition_ref']['id'] for step in view['steps'].values()]
        fixture.authority_cli('session', 'take', '--run', fixture.run, '--attempt', held['attempt_id'])
        fixture.submit(held)
        result = fixture.finish()
        result['waited_for_every_branch'] = True
        return result


def replay(binary):
    with tempfile.TemporaryDirectory(prefix='prifly-composition-replay-') as tmp:
        fixture = Fixture(binary, Path(tmp), 'sequence')
        fixture.commit_verdict = 'needs_revision'
        fixture.start()
        fixture.finish()
        original = fixture.run
        original_view = fixture.view()
        args = ['--repository', fixture.repository, '--launch', 'smsplace', '--host', fixture.host,
                '--source-run', original, '--allow-execution']
        review = fixture.authority_cli('project', 'continue', '--prepare', *args)
        assert 'extra-checks' in json.dumps(review['recovery']['reused']), review['recovery']
        provider = fixture.repository / '.prifly/workflows/ui/steps/reader.yaml'
        original_bytes = provider.read_bytes()
        edit(fixture, '.prifly/workflows/ui/steps/reader.yaml', lambda d: d.update(title='Changed reader contract'))
        checkpoint(fixture)
        rejected = fixture.raw('--project', fixture.authority, 'project', 'continue', '--prepare', *args)
        assert rejected.returncode and json.loads(rejected.stderr)['code'] == 'recover_prefix_changed', rejected.stderr
        replay_review = fixture.authority_cli('project', 'continue', '--prepare', *args, '--from-stage', 'extra-checks')
        assert all(item.get('stage_id') != 'extra-checks' for item in replay_review['recovery']['reused'])
        provider.write_bytes(original_bytes)
        checkpoint(fixture)
        review = fixture.authority_cli('project', 'continue', '--prepare', *args)
        fixture.commit_verdict = 'pass'
        started = fixture.authority_cli('project', 'continue', *args, '--expected-launch-digest', review['review_digest'])
        fixture.run = started['run_id']
        task = fixture.task()
        assert fixture.role(task) == 'commit', 'unchanged imported subtree was not reused'
        fixture.submit(task)
        for _ in range(12):
            if fixture.authority_cli('run', 'next', fixture.run)['action'] == 'terminal':
                break
            fixture.authority_cli('run', 'drive', fixture.run)
        assert fixture.view()['outcome'] == 'succeeded'
        assert original_view == fixture.authority_cli('run', 'status', original)['run'], 'source history changed'
        return dict(unchanged_subtree_reused=True, changed_subtree_refused=True, whole_child_replay_available=True)


def parallel_replay(binary):
    with tempfile.TemporaryDirectory(prefix='prifly-composition-parallel-replay-') as tmp:
        fixture = Fixture(binary, Path(tmp))
        fixture.commit_verdict = 'needs_revision'
        fixture.start()
        fixture.finish()
        args = ['--repository', fixture.repository, '--launch', 'smsplace', '--host', fixture.host,
                '--source-run', fixture.run, '--allow-execution']
        refused = fixture.raw('--project', fixture.authority, 'project', 'continue', '--prepare', *args)
        assert refused.returncode and json.loads(refused.stderr)['code'] == 'recover_topology_unsupported', refused.stderr
        review = fixture.authority_cli('project', 'continue', '--prepare', *args, '--from-stage', 'extra-checks')
        assert all(item.get('stage_id') != 'extra-checks' for item in review['recovery']['reused'])
        return dict(native_prefix_code='recover_topology_unsupported', whole_group_replay_available=True)


def candidate_recheck(binary):
    with tempfile.TemporaryDirectory(prefix='prifly-composition-candidate-') as tmp:
        fixture = Fixture(binary, Path(tmp))
        digests = []
        for version in [1, 2]:
            fixture.candidate_version = version
            fixture.seen = []
            fixture.start()
            fixture.finish()
            digests.append(fixture.implementation['digest'])
        assert digests[0] != digests[1]
        return dict(explicit_runs=2, new_candidate_rechecked=True, digests=digests)


def choice_controls(binary):
    results = []
    for case in ['default', 'unknown']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-choice-') as tmp:
            fixture = Fixture(binary, Path(tmp), 'choice')
            if case == 'default':
                fixture.task_body['fixture_mode'] = 'never'
                fixture.audit_expected = 'audit-alt'
            else:
                fixture.task_body.pop('fixture_mode')
                fixture.expected_outcome = 'rejected'
            fixture.task_file.write_text(json.dumps(fixture.task_body))
            fixture.start()
            result = fixture.finish()
            assert ('audit-alt' in result['steps']) == (case == 'default')
            if case == 'unknown':
                assert 'audit' not in result['steps']
            result['choice_control'] = case
            results.append(result)
    return results


def cancellation(binary):
    with tempfile.TemporaryDirectory(prefix='prifly-composition-cancel-') as tmp:
        fixture = Fixture(binary, Path(tmp))
        edit(fixture, '.prifly/workflows/ui/steps/reader.yaml', lambda d: d.update(effects=dict(**{'class': 'none'}, retry_class='never')))
        checkpoint(fixture)
        fixture.start()
        for _ in range(60):
            next_action = fixture.authority_cli('run', 'next', fixture.run)
            if next_action['action'] == 'stage':
                fixture.authority_cli('run', 'drive', fixture.run)
                continue
            assert next_action['action'] == 'awaiting_host', next_action
            task = fixture.task()
            if fixture.role(task) == 'reader':
                break
            fixture.submit(task)
        else:
            raise AssertionError('no imported reader')
        before = fixture.view()
        activation = before['activations'][before['attempts'][task['attempt_id']]['stage_activation_id']]
        invocation = activation['workflow_invocation_id']
        fixture.authority_cli('run', 'cancel', fixture.run, '--invocation', invocation, '--reason', 'synthetic cancellation')
        waited = False
        for _ in range(8):
            driven = fixture.raw('--project', fixture.authority, 'run', 'drive', fixture.run)
            if driven.returncode == 0:
                break
            assert json.loads(driven.stderr)['code'] == 'unsettled', driven.stderr
            waited = True
            tasks = fixture.authority_cli('session', 'task', '--run', fixture.run, '--all')['tasks']
            siblings = [pending for pending in tasks if pending['attempt_id'] != task['attempt_id']]
            assert siblings, 'cancellation stayed unsettled without a sibling'
            for pending in siblings:
                fixture.authority_cli('session', 'take', '--run', fixture.run, '--attempt', pending['attempt_id'])
                fixture.submit(pending)
        else:
            raise AssertionError('cancelled subtree did not settle')
        assert fixture.view()['outcome'] is None
        fixture.authority_cli('run', 'cancel', fixture.run, '--reason', 'settle synthetic cancellation')
        fixture.authority_cli('run', 'drive', fixture.run)
        view = fixture.view()
        assert view['status'] == 'cancelled' and not view['active_attempt_ids']
        assert not any(step['definition_ref']['id'] in ['fixture:step/commit', 'fixture:step/draft-mr'] for step in view['steps'].values())
        return dict(imported_reader_cancelled=True, waited_for_sibling=waited,
                    publication=False, external_actor_stop_proven=False)


def regrouped(binary):
    results = []
    for mode in ['required', 'unknown']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-regrouped-') as tmp:
            fixture = Fixture(binary, Path(tmp), 'parallel', mode)
            profile_path = fixture.repository / '.prifly/project.yaml'
            profile = json.loads(profile_path.read_text())
            profile['packages']['regrouped'] = dict(source='.prifly/workflows/regrouped')
            profile['launches']['smsplace']['workflow'] = '.prifly/workflows/regrouped/workflow.yaml'
            source.write(fixture.repository, '.prifly/project.yaml', profile)
            folder = fixture.repository / '.prifly/workflows/regrouped'
            source.write(folder, 'workflow.yaml', dict(
                authoring='prifly-project-workflow/2', package=source.package('regrouped'),
                id='fixture:workflow/regrouped', version='1.0.0', title='Regrouped acceptance',
                extends='smsplace', limits=source.LIMITS))
            source.write(folder, 'extend.yaml', dict(authoring='prifly-extension/3', arrange=[dict(
                id='outer-checks', between=dict(**{'from': 'tests'}, to='commit'),
                sequence=['smsplace.extra-checks'], on=dict(succeeded='commit', rejected='rejected'))]))
            checkpoint(fixture)
            review = fixture.authority_cli('project', 'questionnaire', '--prepare', *fixture.args())
            fixture.start(review)
            result = fixture.finish()
            result['regrouped_inherited_group'] = True
            results.append(result)
    return results


def technical_errors(binary):
    results = []
    for case in ['choice', 'sequence', 'parallel', 'step', 'workflow']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-error-') as tmp:
            fixture = Fixture(binary, Path(tmp), 'choice' if case in ['choice', 'sequence', 'parallel'] else 'parallel')
            def group_route(document):
                group = document['arrange'][0]
                group['on_error'] = 'partial'
                if case == 'parallel':
                    group['parallel'] = group.pop('sequence')
            edit(fixture, '.prifly/workflows/smsplace/extend.yaml', group_route)
            if case in ['choice', 'sequence', 'parallel']:
                fixture.task_body['fixture_mode'] = []
                fixture.task_file.write_text(json.dumps(fixture.task_body))
                if case == 'choice':
                    def direct_choice(document):
                        group = document['arrange'][0]
                        members = group.pop('sequence')
                        group['choice'] = dict(selection='exclusive', branches=[dict(
                            predicate=dict(op='eq', left='$inputs.task#/fixture_mode', right=True), use=members[0])],
                            default=members[1], on_unknown='rejected')
                    edit(fixture, '.prifly/workflows/smsplace/extend.yaml', direct_choice)
            elif case == 'step':
                def audit_process(document):
                    document['executor'] = dict(adapter_ref='{{process}}', operation='process')
                edit(fixture, '.prifly/workflows/delivery/steps/audit.yaml', audit_process)
                def failing_binding(document):
                    document['execution_bindings']['steps']['fixture:step/audit'] = dict(
                        executable='fixture-python', args=['audit-failure.py'],
                        files={'audit-failure.py': 'programs/audit-failure.py'},
                        timeout_ms=10000, grace_ms=100, max_output_bytes=4096)
                edit(fixture, '.prifly/workflows/delivery/workflow.yaml', failing_binding)
                source.write(fixture.repository, '.prifly/workflows/delivery/programs/audit-failure.py',
                             'import sys\nsys.exit(17)\n')
            else:
                def provider_fault(document):
                    choice = document['stages']['choose']
                    choice.pop('on_error')
                    choice['branches'][0]['predicate'] = dict(op='eq', left='$inputs.task#/fixture_mode', right=True)
                edit(fixture, '.prifly/workflows/ui/workflows/review-for-change.yaml', provider_fault)
                fixture.task_body['fixture_mode'] = []
                fixture.task_file.write_text(json.dumps(fixture.task_body))
            checkpoint(fixture)
            fixture.start()
            for _ in range(120):
                action = fixture.authority_cli('run', 'next', fixture.run)
                if action['action'] == 'terminal':
                    break
                if action['action'] == 'awaiting_host':
                    fixture.submit(fixture.task())
                else:
                    assert action['action'] == 'stage', action
                    fixture.authority_cli('run', 'drive', fixture.run)
            else:
                raise AssertionError('technical error did not settle')
            view = fixture.view()
            assert view['outcome'] == 'partial', (case, view['outcome'])
            identities = [step['definition_ref']['id'].split('/')[-1] for step in view['steps'].values()]
            assert 'commit' not in identities and 'draft-mr' not in identities, (case, identities)
            assert not view['active_attempt_ids'], case
            failed = [inv for inv in view['invocations'].values() if inv['status'] == 'failed']
            assert failed and all(inv.get('outcome') is None for inv in failed), case
            assert all(inv.get('settled') for inv in view['invocations'].values()), case
            results.append(dict(case=case, outcome=view['outcome'], failed_invocations=len(failed),
                                publication=False, active_attempts=0, synthetic=True))
    return results


def settings_inheritance(binary):
    results = []
    for case in ['forward', 'reverse', 'independent-reset', 'resolved']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-settings-') as tmp:
            fixture = Fixture(binary, Path(tmp))
            def root_input(document):
                document['inputs']['browser'] = source.port(required=False,
                    configuration=dict(scope='project', default={}))
            edit(fixture, '.prifly/workflows/classic/workflow.yaml', root_input)
            def base_options(document):
                document['resources'] = dict(machine=dict(limit=1, stages=['tests']))
                document['settings'] = dict(root=dict(browser=dict(width=800, height=600)))
            edit(fixture, '.prifly/workflows/classic/extend.yaml', base_options)
            def left_options(document):
                document['resources'] = dict(machine=dict(limit=2, stages=['tests']))
                document['settings'] = dict(root=dict(browser=dict(width=1280)))
            edit(fixture, '.prifly/workflows/classic-ui/extend.yaml', left_options)
            if case == 'reverse':
                edit(fixture, '.prifly/workflows/smsplace/workflow.yaml',
                     lambda d: d['extends'].reverse())
            if case in ['independent-reset', 'resolved']:
                edit(fixture, '.prifly/workflows/classic-audit/extend.yaml',
                     lambda d: d.update(resources=dict(machine=dict(limit=1, stages=['tests']))))
            if case == 'resolved':
                edit(fixture, '.prifly/workflows/smsplace/extend.yaml',
                     lambda d: d.update(resources=dict(machine=dict(limit=4, stages=['tests']))))
            checkpoint(fixture)
            output = fixture.root / 'compiled'
            compiled = fixture.raw('--project', fixture.authority, 'project', 'compile',
                                   '--repository', fixture.repository, '--package', 'smsplace', '--output', output)
            if case == 'independent-reset':
                assert compiled.returncode == 2, compiled.stdout
                assert json.loads(compiled.stderr)['code'] == 'project_composition_setting_conflict', compiled.stderr
                assert not output.exists(), 'conflict produced a partial package'
                results.append(dict(case=case, refused=True, output_created=False))
                continue
            assert compiled.returncode == 0, compiled.stderr
            components = json.loads(compiled.stdout)['components']
            root = next(c for c in components if c['ref']['id'] == 'fixture:workflow/smsplace')
            workflow = json.loads((output / root['path']).read_text())
            assert workflow['inputs']['browser']['configuration']['default'] == dict(width=1280), workflow['inputs']
            review = fixture.authority_cli('project', 'questionnaire', '--prepare', *fixture.args())
            fixture.start(review)
            limit = 4 if case == 'resolved' else 2
            view = fixture.view()
            assert view['resource_limits']['machine'] == limit, view.get('resource_limits')
            assert view['stage_resources']['tests'] == ['machine'], view.get('stage_resources')
            result = fixture.finish()
            result.update(setting_case=case, resource_limit=limit, browser_default=dict(width=1280))
            results.append(result)
    return results


def program_ownership(binary):
    results = []
    for case in ['foreign-provider', 'unselected-private']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-program-owner-') as tmp:
            fixture = Fixture(binary, Path(tmp))
            config = dict(executable='fixture-python', args=['FOREIGN_PROGRAM'], files={},
                          timeout_ms=10000, grace_ms=100, max_output_bytes=4096)
            if case == 'foreign-provider':
                edit(fixture, '.prifly/workflows/delivery/workflow.yaml',
                     lambda d: d['execution_bindings']['steps'].pop('fixture:step/tests'))
            else:
                private = source.worker('tests', program=True)
                private['version'] = '2.0.0'
                source.write(fixture.repository, '.prifly/workflows/ui/steps/tests.yaml', private)
                config['args'] = ['PRIVATE_PROGRAM']
                config['files'] = {'worker.py': 'missing-private-worker.py'}
            edit(fixture, '.prifly/workflows/ui/workflow.yaml',
                 lambda d: d.update(execution_bindings=dict(steps={'fixture:step/tests': config})))
            checkpoint(fixture)
            output = fixture.root / 'compiled'
            before = transport.snapshot(fixture.authority)
            compiled = fixture.raw('--project', fixture.authority, 'project', 'compile',
                                   '--repository', fixture.repository, '--package', 'smsplace', '--output', output)
            assert before == transport.snapshot(fixture.authority), 'compile changed authority'
            if case == 'foreign-provider':
                assert compiled.returncode == 2, compiled.stdout
                assert json.loads(compiled.stderr)['code'] == 'project_execution_invalid', compiled.stderr
                assert not output.exists(), 'foreign program left a partial package'
                results.append(dict(case=case, refused=True, output_created=False, effects=False))
                continue
            assert compiled.returncode == 0, compiled.stderr
            payload = json.loads((output / 'execution-bindings.json').read_text())
            bindings = [b for b in payload['bindings'] if b['definition_ref']['id'] == 'fixture:step/tests']
            assert len(bindings) == 1 and bindings[0]['config']['args'] == ['worker.py'], bindings
            assert 'PRIVATE_PROGRAM' not in json.dumps(payload), payload
            fixture.start()
            result = fixture.finish()
            result.update(program_ownership=case, private_program_executed=False)
            results.append(result)
    return results


def explicit_layers(binary):
    results = []
    for case in ['siblings-forward', 'siblings-reverse', 'common-ancestor']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-explicit-layers-') as tmp:
            fixture = Fixture(binary, Path(tmp))
            if case != 'common-ancestor':
                edit(fixture, '.prifly/workflows/classic/extend.yaml', lambda d: d.pop('layers'))
                for name, path in [('classic-ui', '../../layers/delivery.yaml'),
                                   ('classic-audit', '../../layers/../layers/delivery.yaml')]:
                    edit(fixture, '.prifly/workflows/' + name + '/extend.yaml',
                         lambda d, path=path: d.update(layers=[path]))
                if case == 'siblings-reverse':
                    edit(fixture, '.prifly/workflows/smsplace/workflow.yaml', lambda d: d['extends'].reverse())
            checkpoint(fixture)
            output = fixture.root / 'compiled'
            before = transport.snapshot(fixture.authority)
            compiled = fixture.raw('--project', fixture.authority, 'project', 'compile',
                                   '--repository', fixture.repository, '--package', 'smsplace', '--output', output)
            assert before == transport.snapshot(fixture.authority), 'compile changed authority'
            if case != 'common-ancestor':
                assert compiled.returncode == 2, 'repeated explicit layer was accepted'
                assert json.loads(compiled.stderr)['code'] == 'project_composition_duplicate_layer', compiled.stderr
                assert not output.exists(), 'duplicate layer left partial output'
                results.append(dict(case=case, refused=True, output_created=False, effects=False))
                continue
            assert compiled.returncode == 0, compiled.stderr
            composition = json.loads(compiled.stdout)['composition']
            assert composition['layers'] == ['.prifly/layers/delivery.yaml'], composition['layers']
            fixture.start()
            result = fixture.finish()
            result.update(explicit_layers=case, common_layer_count=1)
            results.append(result)
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    cases = []
    for operator in ['sequence', 'parallel', 'choice']:
        for mode in ['required', 'not-required', 'unknown']:
            with tempfile.TemporaryDirectory(prefix='prifly-composition-') as tmp:
                fixture = Fixture(binary, Path(tmp), operator, mode)
                fixture.start()
                cases.append(fixture.finish())
    for mode in ['unavailable', 'partial']:
        with tempfile.TemporaryDirectory(prefix='prifly-composition-unavailable-') as tmp:
            fixture = Fixture(binary, Path(tmp), 'parallel', mode)
            fixture.start()
            cases.append(fixture.finish())
    report = dict(binary_sha256=transport.digest(binary.read_bytes()), synthetic=True,
                  live_browser_qualified=False, cases=cases, freshness=freshness(binary),
                  refusals=refusals(binary),
                  limits=limits(binary),
                  joins=[parallel_wait(binary, 'required'), parallel_wait(binary, 'unknown')],
                  replay=replay(binary), parallel_replay=parallel_replay(binary),
                  candidate_recheck=candidate_recheck(binary), cancellation=cancellation(binary))
    report['choice_controls'] = choice_controls(binary)
    report['regrouped'] = regrouped(binary)
    report['technical_errors'] = technical_errors(binary)
    report['settings_inheritance'] = settings_inheritance(binary)
    report['program_ownership'] = program_ownership(binary)
    report['explicit_layers'] = explicit_layers(binary)
    encoded = json.dumps(report, indent=2) + '\n'
    if args.report:
        args.report.write_text(encoded)
    print(encoded, end='')


if __name__ == '__main__':
    main()
