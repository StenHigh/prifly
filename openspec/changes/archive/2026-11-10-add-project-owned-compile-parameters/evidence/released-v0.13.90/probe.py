#!/usr/bin/env python3
"""Smoke the release executable using the source acceptance fixture."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--binary', type=Path, required=True)
parser.add_argument('--version', required=True)
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
binary = args.binary.resolve()
source = Path('cmd/prifly/project_compile_parameters_test.go').read_text()
folder = re.search(r'const compileParameterFolder = "([^"]+)"', source)[1]
declaration = re.search(r'const compileParameterDeclaration = `([^`]+)`', source)[1]
fixture = source.split('func compileParameterFixture(', 1)[1].split('\n\treturn root, authority', 1)[0]
entries = re.findall(r'writeFixtureFile\(t, root, (compileParameterFolder\+)?("(?:\\.|[^"\\])*")\s*,\s*(`[^`]*`|"(?:\\.|[^"\\])*")\)', fixture)
assert len(entries) == 6, len(entries)
checks = []
with tempfile.TemporaryDirectory(prefix='prifly-release-issue22-') as temporary:
    work = Path(temporary)
    root, authority = work / 'repository', work / 'authority'
    root.mkdir()
    env = dict(os.environ, HOME=str(work / 'home'), XDG_CONFIG_HOME=str(work / 'home' / '.config'))
    Path(env['HOME']).mkdir()

    def cli(*command, failure=None):
        result = subprocess.run([str(binary), *map(str, command), '--json'], capture_output=True, text=True, env=env, timeout=90)
        if failure is not None:
            assert result.returncode != 0 and failure in result.stderr, (command, result.returncode, result.stderr)
            return result.stderr
        assert result.returncode == 0, (command, result.returncode, result.stderr)
        return json.loads(result.stdout)

    version = cli('version')
    assert version['version'] == args.version, version
    capabilities = cli('capabilities')
    assert 'project_compile_parameters' in json.dumps(capabilities)
    for document in ('compile-parameters-v1', 'compile-values-v1'):
        schema = cli('schema', document)
        assert schema['title'] and schema['$id']
    checks.append('version-capability-editor-contracts')
    cli('project', 'init', '--repository', root, '--state-root', authority, '--host', 'codex-cli')
    for prefixed, filename, content in entries:
        target = root / ((folder if prefixed else '') + json.loads(filename))
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content[1:-1] if content.startswith('`') else json.loads(content))
    (root / folder / 'compile-parameters.yaml').write_text(declaration)
    owner = root / folder / 'project/compile-values.yaml'

    def set_owner(value):
        owner.write_text('authoring: prifly-compile-values/1\nvalues: {task_description_limit: ' + str(value) + '}\n')

    def compiled_limit(limit):
        output = work / ('compiled-' + str(limit))
        result = cli('--project', authority, 'project', 'compile', '--repository', root, '--package', 'limits', '--output', output)
        schemas = [json.loads((output / component['path']).read_text()) for component in result['components'] if component['kind'] == 'schema']
        assert len(schemas) == 1 and schemas[0]['properties']['description']['maxLength'] == limit, schemas
        return result

    default = compiled_limit(12000)
    set_owner(32000)
    configured = compiled_limit(32000)
    assert default['build_key'] != configured['build_key']
    checks.append('default-12000-owner-32000-distinct-builds')
    task_path = work / 'task.json'

    def task(length):
        value = {'description': 'я😀' * (length // 2) + 'я' * (length % 2)}
        assert len(value['description']) == length
        task_path.write_text(json.dumps(value, ensure_ascii=False))
        return value

    launch = ['--repository', root, '--launch', 'limits', '--host', 'codex-cli', '--input', 'task=' + str(task_path)]
    original = task(16557)
    review = cli('project', 'questionnaire', '--prepare', *launch)
    started = cli('project', 'start', *launch, '--expected-launch-digest', review['review_digest'])
    run = started['run']['run']['id']
    cli('--project', authority, 'run', 'drive', run)
    session = cli('--project', authority, 'session', 'task', '--run', run)
    delivered = Path(session['workspace']) / session['context']['inputs']['task']['path']
    assert json.loads(delivered.read_text()) == original
    result = dict(schema_version='1', run_id=run, step_instance_id=session['step_instance_id'], attempt_id=session['attempt_id'], envelope_digest=session['envelope_digest'], verdict='pass', outputs={}, evidence_refs=[], effect_receipt_refs=[], summary='Release issue 22 smoke: synthetic Unicode input preserved')
    submission = dict(schema_version=session['schema_version'], run_id=run, attempt_id=session['attempt_id'], envelope_digest=session['envelope_digest'], result=result)
    if session.get('question_report') == 'required':
        submission['answered_questions'] = []
    report = work / 'submission.json'
    report.write_text(json.dumps(submission))
    cli('--project', authority, 'session', 'submit', '--file', report)
    cli('--project', authority, 'run', 'drive', run)
    view = cli('--project', authority, 'run', 'status', run)
    assert view['run']['status'] == 'completed' and view['run']['outcome'] == 'succeeded', view['run']
    checks.append('16557-unicode-root-child-worker-completed')
    for length in (32000, 32001):
        task(length)
        if length == 32000:
            cli('project', 'questionnaire', '--prepare', *launch)
        else:
            for command in (['project', 'questionnaire', '--prepare'], ['project', 'start']):
                diagnostic = cli(*command, *launch, failure='schema_invalid')
                for expected in ('/description', '32000', '32001'):
                    assert expected in diagnostic, diagnostic
    checks.append('32000-accepts-32001-refuses-with-unicode-diagnostic')
    task(16557)
    before = cli('project', 'questionnaire', '--prepare', *launch)
    set_owner(40000)
    after = cli('project', 'questionnaire', '--prepare', *launch)
    assert before['build_key'] != after['build_key'] and before['review_digest'] != after['review_digest']
    cli('project', 'start', *launch, '--expected-launch-digest', before['review_digest'], failure='project_start_stale_launch')
    checks.append('owner-change-invalidates-reviewed-launch')
    set_owner(1000001)
    cli('--project', authority, 'project', 'compile', '--repository', root, '--package', 'limits', '--output', work / 'invalid', failure='project_compile_parameter_invalid')
    checks.append('out-of-range-owner-value-refuses')

args.output.write_text(json.dumps(dict(binary=str(binary), binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), version=args.version, result='passed', checks=checks), indent=2) + '\n')
print(json.dumps(dict(result='passed', checks=checks)))
