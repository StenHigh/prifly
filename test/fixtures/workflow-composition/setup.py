#!/usr/bin/env python3
"""Write a synthetic smsplace-shaped source tree; never executes a workflow."""
import json
from pathlib import Path

LIMITS = dict(max_step_instances=32, max_control_transitions=256,
              max_child_depth=8, max_parallelism=4)
REFERENCES = dict(assisted='core:adapter/assisted-session@1.0.0',
                  process='core:adapter/local-process@2.0.0',
                  policy='core:policy/local@3.0.0', result='core:schema/step-result@1.0.0')


def write(root, name, value):
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + '\n' if not isinstance(value, str) else value)


def port(schema='object', **extra):
    return dict(schema_ref='{{schema_' + schema + '}}', **extra)


def routes(success, failure='rejected'):
    return dict(pass_=success, fail=failure, needs_revision=failure,
                no_work=failure, blocked=failure)


def step_routes(success, failure='rejected'):
    return {k.rstrip('_'): v for k, v in routes(success, failure).items()}


def package(name):
    return dict(id='fixture:package/' + name, version='1.0.0',
                description='Synthetic workflow composition fixture.',
                requires_core_protocol='1', references=REFERENCES)


def workflow(name, stages, entry, inputs=None, outputs=None):
    return dict(authoring='prifly-workflow/1', schema_version='8',
                id='fixture:workflow/' + name, version='1.0.0', title=name,
                inputs=inputs or {}, outputs=outputs or {}, allowed_outcomes=['succeeded', 'rejected'],
                entry=entry, limits=LIMITS, policy_ref='{{policy}}', stages=stages)


def root_workflow(name, stages=None):
    value = workflow(name, stages or {'done': dict(kind='finish', outcome='succeeded')}, 'done')
    value.update(authoring='prifly-project-workflow/2', package=package(name))
    return value


def worker(name, inputs=None, outputs=None, program=False, effect='none'):
    return dict(authoring='prifly-step/1', id='fixture:step/' + name, version='1.0.0',
                title=name, kind='worker', inputs=inputs or {}, outputs=outputs or {},
                executor=dict(adapter_ref='{{process}}' if program else '{{assisted}}',
                              operation='process' if program else 'session'),
                effects=dict(**{'class': effect}, retry_class='never'),
                instructions_ref='{{context_composition}}',
                result_schema_ref='{{result}}')


def extension(name, target, source, dest, call=False, bindings=None):
    return dict(id=name, workflow='root', between=dict(**{'from': source}, to=dest),
                **({'call': target} if call else {'step': target}),
                input_bindings=bindings or {},
                on=dict(succeeded=dest, partial='rejected', rejected='rejected') if call else step_routes(dest),
                **({'on_error': 'rejected'} if call else {}))


def build_repository(repository, operator='parallel'):
    base = repository / '.prifly'
    folders = base / 'workflows'
    names = ['classic', 'ui', 'delivery', 'classic-ui', 'classic-audit', 'smsplace']
    profile = dict(schema_version='prifly-project-profile/3',
                   packages={name: dict(source='.prifly/workflows/' + name) for name in names},
                   launches=dict(smsplace=dict(title='Synthetic smsplace', description='Conditional synthetic acceptance.',
                                               kind='workflow', workflow='.prifly/workflows/smsplace/workflow.yaml',
                                               workspace='worktree')),
                   hosts={'codex-app': '.agents/skills'})
    write(base, 'project.yaml', profile)
    shared = dict(id='fixture:schema/object', version='1.0.0',
                  **{'$schema': 'https://json-schema.org/draft/2020-12/schema'}, type='object')
    for name in ['classic', 'ui', 'delivery']:
        write(folders / name, 'schemas/object.yaml', shared)
	# Synthetic instructions are pinned like every ordinary assisted task.
        write(folders / name, 'contexts/composition.yaml',
              dict(id='fixture:context/composition', version='1.0.0', media_type='text/plain',
                   text='Synthetic engine acceptance. Read exact task/candidate inputs. No real browser, Git commit or publication.'))

    classic = root_workflow('classic')
    classic.update(inputs=dict(task=port(required=True)), entry='prepare',
                   allowed_outcomes=['succeeded', 'rejected', 'partial'],
                   resumable=dict(from_outcomes=['partial']),
                   stages=dict(prepare=dict(kind='step', step_ref='{{step_prepare}}',
                                            input_bindings=dict(task='$inputs.task'),
                                            on=step_routes('commit'), on_error='rejected'),
                               commit=dict(kind='step', step_ref='{{step_commit}}',
                                           on=step_routes('done', 'partial'), on_error='rejected'),
                               done=dict(kind='finish', outcome='succeeded'),
                               rejected=dict(kind='finish', outcome='rejected'),
                               partial=dict(kind='finish', outcome='partial')))
    write(folders / 'classic', 'workflow.yaml', classic)
    write(folders / 'classic', 'steps/prepare.yaml', worker('prepare', dict(task=port(required=True)),
                                                           dict(implementation=port(required_for=['pass'])),
                                                           effect='workspace_write'))
    write(folders / 'classic', 'steps/commit.yaml', worker('commit'))
    write(folders / 'classic', 'extend.yaml', dict(authoring='prifly-extension/3',
                                                   layers=['../../layers/delivery.yaml']))
    delivery = root_workflow('delivery')
    delivery['exports'] = dict(steps={name: '{{step_' + name + '}}'
                                      for name in ['tests', 'draft-mr', 'audit', 'audit-alt']})
    config = dict(executable='fixture-python', args=['worker.py'], files={'worker.py': 'programs/worker.py'},
                  timeout_ms=10000, grace_ms=100, max_output_bytes=4096)
    delivery['execution_bindings'] = dict(steps={'fixture:step/' + name: config for name in ['tests', 'draft-mr']})
    write(folders / 'delivery', 'workflow.yaml', delivery)
    for name in ['tests', 'draft-mr', 'audit', 'audit-alt']:
        write(folders / 'delivery', 'steps/' + name + '.yaml',
              worker(name, dict(implementation=port(required=True)) if name == 'tests' else {},
                     program=name in ['tests', 'draft-mr']))
    write(folders / 'delivery', 'programs/worker.py', '''import json,os,sys
envelope=json.load(sys.stdin)
result=dict(schema_version='1',run_id=envelope['run_id'],step_instance_id=envelope['step_instance_id'],
attempt_id=envelope['attempt_id'],envelope_digest=os.environ['PRIFLY_ENVELOPE_DIGEST'],verdict='pass',
outputs={},evidence_refs=[],effect_receipt_refs=[],summary='Synthetic delivery worker; no real test, commit or publication.')
json.dump(result,os.fdopen(3,'w'))
''')
    write(base, 'layers/delivery.yaml', dict(authoring='prifly-extension/3', imports=dict(delivery='delivery'),
                                             extensions=[extension('tests', 'delivery.tests', 'prepare', 'commit',
                                                                   bindings=dict(implementation='$stages.prepare.implementation')),
                                                         extension('draft-mr', 'delivery.draft-mr', 'commit', 'done')]))

    ui = root_workflow('ui')
    ui['exports'] = dict(workflows={'review-for-change': '{{workflow_review-for-change}}'})
    write(folders / 'ui', 'workflow.yaml', ui)
    applicability = dict(id='fixture:schema/applicability', version='1.0.0',
                         **{'$schema': 'https://json-schema.org/draft/2020-12/schema'},
                         type='object', required=['state', 'reason'], additionalProperties=False,
                         properties=dict(state=dict(enum=['required', 'not-required', 'unknown']),
                                         reason=dict(type='string', minLength=1)))
    write(folders / 'ui', 'schemas/applicability.yaml', applicability)
    inputs = dict(task=port(required=True), implementation=port(required=True))
    write(folders / 'ui', 'steps/applicability.yaml', worker('applicability', inputs,
                                                           dict(decision=port('applicability', required_for=['pass']))))
    write(folders / 'ui', 'steps/reader.yaml', worker('reader', inputs, dict(report=port(required_for=['pass'])), effect='workspace_write'))
    stages = dict(applicability=dict(kind='step', step_ref='{{step_applicability}}',
                                    input_bindings={name: '$inputs.' + name for name in inputs},
                                    on=step_routes('choose'), on_error='rejected'),
                  choose=dict(kind='choice', selection='exclusive', branches=[
                      dict(id='required', predicate=dict(op='eq', left='$stages.applicability.decision#/state', right='required'), next='reader'),
                      dict(id='not-required', predicate=dict(op='eq', left='$stages.applicability.decision#/state', right='not-required'), next='done')],
                              default='rejected', on_unknown='rejected', on_error='rejected'),
                  reader=dict(kind='step', step_ref='{{step_reader}}',
                              input_bindings={name: '$inputs.' + name for name in inputs},
                              on={**step_routes('done'), 'needs_revision': 'partial'}, on_error='rejected'),
                  done=dict(kind='finish', outcome='succeeded',
                            output_bindings=dict(decision='$stages.applicability.decision')),
                  rejected=dict(kind='finish', outcome='rejected'),
                  partial=dict(kind='finish', outcome='partial'))
    review = workflow('review-for-change', stages, 'applicability', inputs,
                      dict(decision=port('applicability', required_for=['succeeded'])))
    review['allowed_outcomes'] = ['succeeded', 'partial', 'rejected']
    write(folders / 'ui', 'workflows/review-for-change.yaml', review)
    # A library import must not apply the standalone provider's overrides.
    write(folders / 'ui', 'extend.yaml', 'standalone: deliberately-invalid-and-never-read\n')
    for name in ['classic-ui', 'classic-audit', 'smsplace']:
        derived = dict(authoring='prifly-project-workflow/2', package=package(name),
                       id='fixture:workflow/' + name, version='1.0.0', title=name,
                       extends=['classic-ui', 'classic-audit'] if name == 'smsplace' else 'classic', limits=LIMITS)
        write(folders / name, 'workflow.yaml', derived)
    write(folders / 'classic-ui', 'extend.yaml', dict(authoring='prifly-extension/3', imports=dict(ui='ui'),
          extensions=[extension('interface-review', 'ui.review-for-change', 'tests', 'commit', True,
                                dict(task='$inputs.task', implementation='$stages.prepare.implementation'))]))
    audit_extensions = [extension('test-audit', 'audit.audit', 'tests', 'commit')]
    audit = dict(authoring='prifly-extension/3', imports=dict(audit='delivery'), extensions=audit_extensions)
    if operator == 'choice':
        audit_extensions.append(extension('test-audit-alt', 'audit.audit-alt', 'tests', 'commit'))
        audit['arrange'] = [dict(id='audit-path', between=dict(**{'from': 'tests'}, to='commit'),
                                 choice=dict(selection='first_match', branches=[dict(predicate=dict(op='ne', left='$inputs.task#/fixture_mode', right='never'), use='classic-audit.test-audit')],
                                             default='classic-audit.test-audit-alt', on_unknown='rejected'),
                                 on=dict(succeeded='commit', rejected='rejected'))]
    write(folders / 'classic-audit', 'extend.yaml', audit)
    group = dict(id='extra-checks', between=dict(**{'from': 'tests'}, to='commit'),
                 on=dict(succeeded='commit', rejected='rejected'))
    group['sequence' if operator == 'choice' else operator] = [
        'classic-ui.interface-review', 'classic-audit.audit-path' if operator == 'choice' else 'classic-audit.test-audit']
    write(folders / 'smsplace', 'extend.yaml', dict(authoring='prifly-extension/3', arrange=[group]))
    return profile
