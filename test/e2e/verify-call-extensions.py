#!/usr/bin/env python3
"""Synthetic E5 acceptance through the public CLI; never claims live-host qualification."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

CORPUS = Path(__file__).resolve().parents[1] / 'fixtures/project-call-extensions'


def digest(data):
    return 'sha256:' + hashlib.sha256(data).hexdigest()


def snapshot(root):
    return {str(p.relative_to(root)): digest(p.read_bytes()) for p in root.rglob('*')
            if p.is_file() and '.git' not in p.parts and not p.name.endswith('-shm')
            and not (p.name.endswith('-wal') and p.stat().st_size == 0)}


class Fixture:
    def __init__(self, binary, root, case='review', host='codex-app'):
        self.binary, self.root, self.host = binary, root, host
        self.case = case
        self.commit_verdict = 'pass'
        self.repository, self.authority = root / 'repository', root / 'authority'
        self.environment = dict(os.environ, HOME=str(root / 'home'))
        (root / 'home').mkdir()
        self.repository.mkdir()
        self.git('init', '-q')
        self.cli('project', 'init', '--repository', self.repository, '--state-root', self.authority)
        shutil.copytree(CORPUS / 'repository', self.repository, dirs_exist_ok=True)
        self.folder = self.repository / '.prifly/workflows/quality'
        shutil.copyfile(CORPUS / 'extensions' / (case + '.yaml'), self.folder / 'extend.yaml')
        for declared_host in ['codex-app', 'codex-cli', 'claude-code']:
            self.cli('project', 'runners', 'add', '--repository', self.repository, '--host', declared_host)
            skills={'codex-app':'.agents/skills','codex-cli':'.codex/skills','claude-code':'.claude/skills'}[declared_host]
            text=(self.repository/skills/'prifly-run/SKILL.md').read_text()
            assert 'Native calls, including project call extensions' in text
            assert 'Never launch nested' in text
        self.git('add', '.')
        self.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                 '-c', 'commit.gpgsign=false', 'commit', '-qm', 'fixture')
        self.source_hashes = snapshot(self.folder)
        self.claim, self.claim_generation, self.seen = None, None, []
        self.values = {}
        self.program = False
        self.prepare_values = {}

    def git(self, *args):
        p = subprocess.run(['git', '-C', str(self.repository), *map(str, args)],
                           env=self.environment, capture_output=True, text=True, timeout=30)
        assert p.returncode == 0, p.stderr
        return p.stdout.strip()

    def raw(self, *args):
        return subprocess.run([str(self.binary), '--json', *map(str, args)],
                              env=self.environment, capture_output=True, text=True, timeout=60)

    def cli(self, *args):
        p = self.raw(*args)
        assert p.returncode == 0, (args, p.returncode, p.stdout, p.stderr)
        return json.loads(p.stdout)

    def authority_cli(self, *args):
        return self.cli('--project', self.authority, *args)

    def prepare(self):
        args = ['--repository', self.repository, '--launch', 'quality', '--host', self.host]
        q = self.authority_cli('project', 'questionnaire', *args)
        before = snapshot(self.authority)
        review = self.authority_cli('project', 'questionnaire', *args, '--prepare',
                                    '--expected-decision-catalog-digest', q['decision_sheet']['catalog_digest'],
                                    *(['--allow-execution'] if self.program else []))
        assert before == snapshot(self.authority), 'prepare mutated authority'
        assert self.source_hashes == snapshot(self.folder), 'prepare mutated sources'
        return review

    def start(self, review=None):
        review = review or self.prepare()
        started = self.authority_cli('project', 'start', '--repository', self.repository,
                                     '--launch', 'quality', '--host', self.host,
                                     '--expected-launch-digest', review['review_digest'], '--allow-execution')
        self.run = started['run_id']
        return started

    def task(self):
        task = self.authority_cli('session', 'task', '--run', self.run, '--all')['tasks'][0]
        self.authority_cli('session', 'take', '--run', self.run, '--attempt', task['attempt_id'])
        return task

    def submit(self, task, verdict='pass'):
        workspace = Path(task['workspace'])
        inputs = {name: json.loads((workspace / port['path']).read_text())
                  for name, port in task['context']['inputs'].items()}
        outputs = task['context']['outputs']
        if 'request' in outputs:
            role = 'prepare'
        elif 'report' in outputs or 'ui_report' in outputs:
            role = 'assessment'
        elif inputs:
            role = 'reader'
        else:
            role = 'commit'
        self.seen.append(role)
        if role == 'reader':
            if self.program:
                self.values={name:{'port':name,'inputs':{'request':self.prepare_values['request']}} for name in ['report','gate']}
            expected_ports = ['report','gate'] if self.case=='review' else ['gate','ui_report','ux_report','environment','release']
            assert set(inputs)==set(expected_ports), inputs
            assert inputs == {name:self.values[name] for name in expected_ports}, ('exports not wired', inputs, self.values)
        if role == 'assessment':
            assert inputs, 'child did not receive its inputs'
            assert task['claim_id'] == self.claim and task['claim_generation'] == self.claim_generation
        if role == 'prepare':
            self.claim, self.claim_generation = task['claim_id'], task['claim_generation']
            tree = Path(task['repository_workspace'])
            self.tree = tree
            (tree / 'candidate.txt').write_text('same candidate\n')
        report = self.authority_cli('session', 'submit', '--template', '--run', self.run,
                                    '--attempt', task['attempt_id'])
        report['result']['verdict'] = verdict if role == 'assessment' else self.commit_verdict if role == 'commit' else 'pass'
        report['result']['summary'] = 'synthetic ' + role
        report['answered_questions'] = []
        if report['result']['verdict'] == 'pass':
            for name, port in outputs.items():
                value = {'port': name, 'inputs': inputs}
                data = (json.dumps(value, sort_keys=True) + '\n').encode()
                (workspace / port['path']).write_bytes(data)
                report['result']['outputs'][name]['digest'] = digest(data)
                if role == 'assessment':
                    self.values[name] = value
                if role == 'prepare':
                    self.prepare_values[name] = value
        else:
            report['result']['outputs'] = {}
        path = self.root / 'submission.json'
        path.write_text(json.dumps(report))
        self.authority_cli('session', 'submit', '--file', path)

    def finish(self, verdict='pass'):
        for _ in range(30):
            n = self.authority_cli('run', 'next', self.run)
            if n['action'] == 'terminal':
                break
            if n['action'] == 'awaiting_host':
                self.submit(self.task(), verdict)
            else:
                assert n['action'] == 'stage', n
                self.authority_cli('run', 'drive', self.run)
        else:
            raise AssertionError('no terminal outcome')
        view = self.authority_cli('run', 'status', self.run)['run']
        assert self.source_hashes == snapshot(self.folder), 'upstream/sibling source changed'
        invocations = view['invocations']
        assert len(invocations) >= 2, invocations
        roots = [v for v in invocations.values() if not v.get('caller_stage_activation_id')]
        assert len(roots) == 1, invocations
        assert self.seen.count('prepare') == 1
        assert (self.tree/'candidate.txt').read_text()=='same candidate\n'
        assert view['trust_profile']=='core-local/cooperative'
        assert all(v['run_id']==self.run for v in invocations.values()), 'child escaped the Run'
        for child in invocations.values():
            if child.get('parent_invocation_id'):
                assert child['parent_invocation_id'] in invocations
                assert child['caller_stage_activation_id'] in view['activations']
        expected = {'pass': 'succeeded', 'fail': 'rejected', 'needs_revision': 'partial'}[verdict]
        if self.commit_verdict == 'needs_revision' and verdict=='pass':
            expected='partial'
        assert view['outcome'] == expected, view['outcome']
        if verdict == 'pass':
            assert self.seen.count('reader') == 1 and self.seen.count('commit') == 1, self.seen
        else:
            assert 'reader' not in self.seen and 'commit' not in self.seen, self.seen
        checked=set()
        for invocation in invocations.values():
            for ref in invocation['output_refs'].values():
                if ref['artifact_id'] in checked:
                    continue
                checked.add(ref['artifact_id'])
                path=self.root/'artifact-ref.json';path.write_text(json.dumps(ref))
                artifact=self.authority_cli('artifact','inspect','--ref',path)['artifact']
                assert artifact['digest']==ref['digest']
                assert artifact['schema_ref']['id']=='fixture:schema/object'
                producer=artifact['producer']
                assert producer['run_id']==self.run and producer['attempt_id'] in view['attempts']
                assert view['attempts'][producer['attempt_id']].get('accepted')
                assert producer['workflow_invocation_id'] in invocations
                # Ordinary step artifacts record their exact producer; input refs
                # stay in the Attempt envelope, not invented artifact lineage.
                assert artifact['provenance'] in (None, [])
        return {'checked_exports':len(checked),'case_outcome': expected, 'run_id': self.run, 'invocations': len(invocations),
                'claim_id': self.claim, 'claim_generation': self.claim_generation,
                'sessions': self.seen, 'sources_unchanged': True}



def edit(f, relative, update):
    path = f.folder / relative
    value = json.loads(path.read_text())
    update(value)
    path.write_text(json.dumps(value, indent=2) + "\n")


def checkpoint(f):
    f.git('add', '.')
    f.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
          '-c', 'commit.gpgsign=false', 'commit', '--allow-empty', '-qm', 'fixture change')
    f.source_hashes = snapshot(f.folder)


def configure_program(f, mode='pass'):
    edit(f, 'project/steps/review-leaf.yaml', lambda d: d.update(
        executor={'adapter_ref': '{{process}}', 'operation': 'process'},
        effects={'class': 'none', 'retry_class': 'never'}))
    worker = f.folder / 'project/worker.py'
    worker.write_text("""import hashlib,json,os,sys,time
from pathlib import Path
envelope=json.load(sys.stdin)
context=json.loads(Path(os.environ['PRIFLY_CONTEXT_FILE']).read_text())
inputs={name:json.loads(Path(port['path']).read_text()) for name,port in context['inputs'].items()}
result={'schema_version':'1','run_id':envelope['run_id'],'step_instance_id':envelope['step_instance_id'],'attempt_id':envelope['attempt_id'],'envelope_digest':os.environ['PRIFLY_ENVELOPE_DIGEST'],'verdict':'pass','outputs':{},'evidence_refs':[],'effect_receipt_refs':[],'summary':'synthetic program'}
for name,port in context['outputs'].items():
 data=(json.dumps({'port':name,'inputs':inputs},sort_keys=True)+'\\n').encode()
 Path(port['path']).write_bytes(data)
 result['outputs'][name]={'artifact_id':port['artifact_id'],'revision':port['revision'],'digest':'sha256:'+hashlib.sha256(data).hexdigest()}
json.dump(result,os.fdopen(3,'w'))
""")
    if mode == 'error':
        worker.write_text('raise SystemExit(2)\n')
    if mode == 'timeout':
        worker.write_text('import time; time.sleep(10)\n')
    binding = {'executable':'python3', 'args':['worker.py'],
               'files':{'worker.py':'project/worker.py'}, 'timeout_ms':200 if mode == 'timeout' else 30000,
               'grace_ms':100, 'max_output_bytes':65536}
    edit(f, 'extend.yaml', lambda d: d.update(execution_bindings={'steps':{'review-leaf':binding}}))
    local = f.repository / '.prifly/local.yaml'
    # The machine binding is explicit, local and part of the reviewed program.
    local.write_text(local.read_text() + '\nexecutables:\n  python3: ' + json.dumps(os.path.realpath(shutil.which('python3'))) + '\n')
    f.program = True
    checkpoint(f)


def program_case(binary):
    with tempfile.TemporaryDirectory(prefix='prifly-call-program-') as tmp:
        f = Fixture(binary, Path(tmp))
        configure_program(f)
        review = f.prepare()
        assert review['execution'], 'program absent from reviewed launch'
        compiled=f.authority_cli('project','compile','--repository',f.repository,'--package','quality','--host',f.host,'--output',f.root/'compiled')
        assert '.prifly/workflows/quality/project/worker.py' in compiled['sources'],compiled['sources']
        assert any('worker.py' in json.dumps(x) for x in review['execution']), review['execution']
        f.start(review)
        result = f.finish()
        result['program'] = True
        return result


def freshness_cases(binary):
    changes = {
        'callee': lambda f: edit(f, 'project/workflows/product.yaml', lambda d: d.update(title='changed callee')),
        'descendant': lambda f: edit(f, 'project/steps/product-leaf.yaml', lambda d: d.update(title='changed leaf')),
        'nested-descendant': lambda f: edit(f, 'project/workflows/review-inner.yaml', lambda d: d.update(title='changed nested workflow')),
        'outcome': lambda f: edit(f, 'extend.yaml', lambda d: d['extensions'][0]['on'].update(partial='rejected')),
        'error': lambda f: edit(f, 'extend.yaml', lambda d: d['extensions'][0].update(on_error='partial')),
        'resource': lambda f: edit(f, 'extend.yaml', lambda d: d['resources']['quality'].update(limit=2)),
        'limits': lambda f: edit(f, 'workflow.yaml', lambda d: d['limits'].update(max_step_instances=25)),
        'context': lambda f: edit(f, 'project/contexts/task.yaml', lambda d: d.update(text='changed declared context')),
    }
    for port in ['configuration','ui','ux']:
        changes['binding:'+port] = lambda f, port=port: edit(f, 'extend.yaml', lambda d: d['extensions'][0]['input_bindings'].update({port:'$stages.prepare.request'}))
    changes['local-resource']=lambda f: (f.repository/'.prifly/local.yaml').write_text((f.repository/'.prifly/local.yaml').read_text()+'\nresources:\n  quality: {limit: 2}\n')
    results=[]
    for name, change in changes.items():
        with tempfile.TemporaryDirectory(prefix='prifly-call-freshness-') as tmp:
            f=Fixture(binary,Path(tmp),'review' if name=='nested-descendant' else 'product')
            review=f.prepare()
            before=f.authority_cli('run','list')
            change(f)
            checkpoint(f)
            p=f.raw('--project',f.authority,'project','start','--repository',f.repository,
                    '--launch','quality','--host',f.host,'--expected-launch-digest',review['review_digest'],'--allow-execution')
            assert p.returncode != 0, name
            assert json.loads(p.stderr)['code']=='project_start_stale_launch',(name,p.stderr)
            assert before==f.authority_cli('run','list'), 'stale review dispatched work'
            results.append({'mutation':name,'code':'project_start_stale_launch','dispatched':False})
    with tempfile.TemporaryDirectory(prefix='prifly-call-file-freshness-') as tmp:
        f=Fixture(binary,Path(tmp))
        configure_program(f)
        review=f.prepare()
        worker=f.folder/'project/worker.py'
        worker.write_text(worker.read_text()+'# changed supporting bytes\n')
        checkpoint(f)
        p=f.raw('--project',f.authority,'project','start','--repository',f.repository,'--launch','quality','--host',f.host,'--expected-launch-digest',review['review_digest'],'--allow-execution')
        assert p.returncode!=0 and json.loads(p.stderr)['code']=='project_start_stale_launch',p.stderr
        results.append({'mutation':'supporting-file','code':'project_start_stale_launch','dispatched':False})
    return results


def assessment_task(f):
    f.submit(f.task())
    f.authority_cli('run','drive',f.run)
    task=f.task()
    assert 'report' in task['context']['outputs'],task
    return task


def control_cases(binary):
    results=[]
    with tempfile.TemporaryDirectory(prefix='prifly-call-resource-') as tmp:
        import copy
        first=Fixture(binary,Path(tmp))
        first.start()
        held=assessment_task(first)
        second=copy.copy(first)
        second.seen=[];second.values={};second.prepare_values={}
        second.start()
        second.submit(second.task())
        refusal=second.raw('--project',second.authority,'run','drive',second.run)
        assert refusal.returncode!=0 and json.loads(refusal.stderr)['code']=='resource_busy',refusal.stderr
        second_view=second.authority_cli('run','status',second.run)['run']
        assert len(second_view['attempts'])==1, 'resource refusal dispatched child'
        first.submit(held)
        first.finish()
        second.finish()
        assert first.claim!=second.claim
        results.append({'control':'resource competition','code':'resource_busy','released_after_settlement':True,'runs':[first.run,second.run]})
    with tempfile.TemporaryDirectory(prefix='prifly-call-cancel-') as tmp:
        f=Fixture(binary,Path(tmp))
        edit(f,'project/steps/review-leaf.yaml',lambda d:d.update(effects={'class':'none','retry_class':'never'}))
        checkpoint(f);f.start();task=assessment_task(f)
        before=f.authority_cli('run','status',f.run)['run']
        attempt=before['attempts'][task['attempt_id']]
        activation=before['activations'][attempt['stage_activation_id']]
        child=activation['workflow_invocation_id']
        f.authority_cli('run','cancel',f.run,'--invocation',child,'--reason','synthetic child cancellation')
        f.authority_cli('run','drive',f.run)
        n=f.authority_cli('run','next',f.run)
        assert n['action']=='blocked_child',n
        waiting=f.authority_cli('run','status',f.run)['run']
        assert waiting['status']=='waiting' and waiting['outcome'] is None
        assert waiting['invocations'][child]['status']=='cancelled'
        assert not waiting['invocations'][child]['output_refs']
        f.authority_cli('run','cancel',f.run,'--reason','settle fixture')
        f.authority_cli('run','drive',f.run)
        ended=f.authority_cli('run','status',f.run)['run']
        assert ended['status']=='cancelled' and not ended['active_attempt_ids']
        results.append({'control':'child cancellation','waiting':'blocked_child','settled':'cancelled','external_actor_stop_proven':False})
    with tempfile.TemporaryDirectory(prefix='prifly-call-unknown-') as tmp:
        f=Fixture(binary,Path(tmp));f.start();task=assessment_task(f)
        state=f.authority_cli('run','status',f.run)['run']
        activation=state['activations'][state['attempts'][task['attempt_id']]['stage_activation_id']]
        child=activation['workflow_invocation_id']
        f.authority_cli('run','cancel',f.run,'--invocation',child,'--reason','synthetic unknown effect')
        p=f.raw('--project',f.authority,'run','drive',f.run)
        assert p.returncode!=0 and json.loads(p.stderr)['code']=='recovery_required',p.stderr
        p=f.raw('--project',f.authority,'project','continue','--prepare','--repository',f.repository,'--launch','quality','--host',f.host,'--source-run',f.run)
        assert p.returncode!=0 and json.loads(p.stderr)['code']=='recover_source_unsettled',p.stderr
        f.authority_cli('run','resolve',f.run,'--attempt',task['attempt_id'],'--outcome','not_applied','--reason','controlled synthetic host performed no work')
        f.authority_cli('run','cancel',f.run,'--reason','settle controlled fixture')
        f.authority_cli('run','drive',f.run)
        assert f.authority_cli('run','status',f.run)['run']['status']=='cancelled'
        results.append({'control':'unknown effect','code':'recovery_required','resume_refusal':'recover_source_unsettled','settlement':'explicit controlled-host not_applied attestation'})
    for mode in ['error','timeout']:
        for handled in [True,False]:
            with tempfile.TemporaryDirectory(prefix='prifly-call-error-') as tmp:
                f=Fixture(binary,Path(tmp));configure_program(f,mode)
                for path in ['project/workflows/review-inner.yaml','project/workflows/review.yaml']:
                    edit(f,path,lambda d:d['stages']['assess'].pop('on_error',None))
                if not handled:
                    edit(f,'extend.yaml',lambda d:d['extensions'][0].pop('on_error',None))
                checkpoint(f);f.start();f.submit(f.task())
                f.authority_cli('run','drive',f.run)
                v=f.authority_cli('run','status',f.run)['run']
                assert any(x['status']=='failed' and x['outcome'] is None for x in v['invocations'].values()),v
                if handled:
                    assert v['outcome']=='rejected',v['status']
                else:
                    assert v['status']=='failed' and v['outcome'] is None,v['status']
                assert all(not x['output_refs'] for x in v['invocations'].values())
                results.append({'control':mode,'on_error':handled,'status':v['status'],'outcome':v['outcome']})
    return results


def limit_cases(binary):
    results=[]
    for field,value in [('max_child_depth',0),('max_step_instances',1),('max_control_transitions',1)]:
        with tempfile.TemporaryDirectory(prefix='prifly-call-limits-') as tmp:
            f=Fixture(binary,Path(tmp))
            edit(f,'workflow.yaml',lambda d:d['limits'].update({field:value}))
            checkpoint(f)
            p=f.raw('--project',f.authority,'project','compile','--repository',f.repository,'--package','quality','--host',f.host,'--output',f.root/'refused')
            assert p.returncode!=0 and json.loads(p.stderr)['code']=='limit_exceeded',(field,p.stderr)
            assert not (f.root/'refused/prifly.package.json').exists()
            results.append({'limit':field,'code':'limit_exceeded','dispatched':False})
    for mode,code in [('destructive','unsupported'),('legacy-policy','resource_limit')]:
        with tempfile.TemporaryDirectory(prefix='prifly-call-policy-') as tmp:
            f=Fixture(binary,Path(tmp))
            if mode=='destructive':
                edit(f,'project/steps/review-leaf.yaml',lambda d:d.update(effects={'class':'destructive','retry_class':'never'}))
            else:
                edit(f,'workflow.yaml',lambda d:d['package']['references'].update(policy='core:policy/local@1.0.0'))
            checkpoint(f)
            p=f.raw('--project',f.authority,'project','questionnaire','--repository',f.repository,'--launch','quality','--host',f.host,'--prepare')
            assert p.returncode!=0 and json.loads(p.stderr)['code']==code,(mode,p.stderr)
            results.append({'policy':mode,'code':code,'dispatched':False})
    with tempfile.TemporaryDirectory(prefix='prifly-call-trust-') as tmp:
        f=Fixture(binary,Path(tmp));configure_program(f)
        args=['--project',f.authority,'project','questionnaire','--repository',f.repository,'--launch','quality','--host',f.host,'--prepare']
        p=f.raw(*args)
        assert p.returncode!=0 and json.loads(p.stderr)['code']=='project_execution_approval_required',p.stderr
        edit(f,'extend.yaml',lambda d:d['execution_bindings']['steps']['review-leaf'].update(executable='untrusted'))
        p=f.raw(*args,'--allow-execution')
        assert p.returncode!=0 and json.loads(p.stderr)['code']=='project_execution_not_allowed',p.stderr
        results.append({'trust':'explicit local allowlist and consent','profile':'core-local/cooperative','dispatched':False})
    return results


def replay_cases(binary):
    with tempfile.TemporaryDirectory(prefix='prifly-call-replay-') as tmp:
        f=Fixture(binary,Path(tmp));f.commit_verdict='needs_revision';f.start();f.finish()
        original=f.run
        original_view=f.authority_cli('run','status',original)
        claim=f.claim
        inherited=[]
        def resume(source, from_stage=None):
            args=['--repository',f.repository,'--launch','quality','--host',f.host,'--source-run',source]
            if from_stage:
                args+=['--from-stage',from_stage]
            review=f.authority_cli('project','continue','--prepare',*args)
            started=f.authority_cli('project','continue',*args,'--expected-launch-digest',review['review_digest'])
            assert started['workspace']['id']==claim, started['workspace']
            f.run=started['run_id']
            f.claim_generation=started['workspace']['generation']
            return review
        for _ in range(2):
            previous=f.run
            review=resume(previous)
            inherited.append(len(review['recovery']['reused']))
            assert 'quality-check' in json.dumps(review['recovery']['reused'])
            # Only the final root step is executed; accepted child exports are reused.
            task=f.task()
            assert not task['context']['inputs'] and not task['context']['outputs'],task
            f.submit(task)
            f.authority_cli('run','drive',f.run)
            now=f.authority_cli('run','status',f.run)['run']
            assert now['outcome']=='partial' and len(now['attempts'])==1,now['status']
            assert original_view['run']==f.authority_cli('run','status',original)['run'], 'source history changed'
            assert (f.tree/'candidate.txt').read_text()=='same candidate\n'
        source=f.run
        edit(f,'project/steps/review-leaf.yaml',lambda d:d['outputs'].update(extra={'schema_ref':'{{schema_object}}','required_for':['pass']}))
        checkpoint(f)
        args=['--repository',f.repository,'--launch','quality','--host',f.host,'--source-run',source]
        rejected=f.raw('--project',f.authority,'project','continue','--prepare',*args)
        assert rejected.returncode!=0 and json.loads(rejected.stderr)['code']=='recover_prefix_changed',rejected.stderr
        for from_stage,code in [('prepare','resume_prefix_empty'),('assess','resume_from_stage_invalid')]:
            p=f.raw('--project',f.authority,'project','continue','--prepare',*args,'--from-stage',from_stage)
            assert p.returncode!=0 and json.loads(p.stderr)['code']==code,p.stderr
        review=resume(source,'quality-check')
        assert all(x.get('stage_id')!='quality-check' for x in review['recovery']['reused'])
        f.commit_verdict='pass';f.values={};f.seen=[]
        for _ in range(20):
            n=f.authority_cli('run','next',f.run)
            if n['action']=='terminal':break
            if n['action']=='awaiting_host':f.submit(f.task())
            else:f.authority_cli('run','drive',f.run)
        v=f.authority_cli('run','status',f.run)['run']
        assert v['outcome']=='succeeded' and f.seen.count('assessment')==1 and 'prepare' not in f.seen,f.seen
        assert original_view['run']==f.authority_cli('run','status',original)['run']
        return [{'replay':'inherited prefix through two resumes','reused_counts':inherited},
                {'replay':'changed accepted child','refusal':'recover_prefix_changed','whole_child_replayed':True,'source_unchanged':True,'claim_id':claim}]


def negative_cases(binary):
    def target(f,name):
        edit(f,'extend.yaml',lambda d:d['extensions'][0].update(call=name))
    def ambiguous(f):
        child=json.loads((f.folder/'project/workflows/review.yaml').read_text())
        child['id']='other:workflow/review'
        (f.folder/'project/workflows/ambiguous.yaml').write_text(json.dumps(child))
    def missing(f):
        edit(f,'extend.yaml',lambda d:d['extensions'][0].update(input_bindings={}))
    def late(f):
        edit(f,'steps/commit.yaml',lambda d:d.update(outputs={'request':{'schema_ref':'{{schema_object}}','required_for':['pass']}}))
        edit(f,'extend.yaml',lambda d:d['extensions'][0]['input_bindings'].update(request='$stages.commit.request'))
    def wrong_schema(f):
        schema=json.loads((f.folder/'project/schemas/object.yaml').read_text());schema['id']='fixture:schema/other'
        (f.folder/'project/schemas/other.yaml').write_text(json.dumps(schema))
        edit(f,'steps/prepare.yaml',lambda d:d['outputs']['request'].update(schema_ref='{{schema_other}}'))
    changes=[
        ('both','project_extension_invalid',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][0].update(step='prepare'))),
        ('neither','project_extension_invalid',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][0].pop('call'))),
        ('unmarked','project_extension_unsupported_authoring',lambda f:edit(f,'extend.yaml',lambda d:d.pop('authoring'))),
        ('unknown','project_extension_unknown_call',lambda f:target(f,'missing')),
        ('wrong-kind','project_extension_call_wrong_kind',lambda f:target(f,'review-leaf')),
        ('ambiguous','project_extension_call_ambiguous',ambiguous),
        ('missing-input','missing_binding',missing),
        ('late-producer','unavailable_output',late),
        ('unknown-producer','project_extension_unknown_stage',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][0]['input_bindings'].update(request='$stages.missing.request'))),
        ('schema-mismatch','port_type_mismatch',wrong_schema),
        ('incomplete-outcomes','project_extension_call_outcome_unmapped',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][0]['on'].pop('partial'))),
        ('extra-outcome','invalid_outcome',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][0]['on'].update(no_work='partial'))),
        ('partial-export','unavailable_output',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][1]['between'].update(to='partial'))),
        ('invalid-on-error','project_extension_unknown_stage',lambda f:edit(f,'extend.yaml',lambda d:d['extensions'][0].update(on_error='missing'))),
    ]
    result=[]
    for name,code,change in changes:
        with tempfile.TemporaryDirectory(prefix='prifly-call-negative-') as tmp:
            f=Fixture(binary,Path(tmp));change(f)
            before=f.authority_cli('run','list')
            output=f.root/'refused'
            p=f.raw('--project',f.authority,'project','compile','--repository',f.repository,'--package','quality','--host',f.host,'--output',output)
            assert p.returncode!=0 and json.loads(p.stderr)['code']==code,(name,p.stderr)
            assert not (output/'prifly.package.json').exists()
            assert before==f.authority_cli('run','list'),'compile refusal dispatched a Run'
            result.append({'negative':name,'code':code,'partial_package':False,'dispatched':False})
    return result

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--section', choices=['all','journeys','program','freshness','controls','limits','replay','negatives'], default='all')
    args = p.parse_args()
    binary = args.binary.resolve()
    assert binary.is_file()
    results = []
    for case in (['review', 'product'] if args.section in ['all','journeys'] else []):
        for verdict in ['pass', 'fail', 'needs_revision']:
            with tempfile.TemporaryDirectory(prefix='prifly-call-extension-') as tmp:
                f = Fixture(binary, Path(tmp), case)
                f.start()
                results.append(dict(case=case, verdict=verdict, host='codex-app', **f.finish(verdict)))
    for host in (['codex-cli', 'claude-code'] if args.section in ['all','journeys'] else []):
        with tempfile.TemporaryDirectory(prefix='prifly-call-host-') as tmp:
            f = Fixture(binary, Path(tmp), host=host)
            f.start()
            results.append(dict(case='review', host=host, **f.finish()))
    if args.section in ['all','program']:
        results.append(program_case(binary))
    if args.section in ['all','freshness']:
        results.extend(freshness_cases(binary))
    if args.section in ['all','controls']:
        results.extend(control_cases(binary))
    if args.section in ['all','limits']:
        results.extend(limit_cases(binary))
    if args.section in ['all','replay']:
        results.extend(replay_cases(binary))
    if args.section in ['all','negatives']:
        results.extend(negative_cases(binary))
    print(json.dumps({'binary': str(binary), 'sha256': digest(binary.read_bytes()),
                      'scope': 'source-built/synthetic native CLI; live hosts and released qualification pending',
                      'results': results}, indent=2))


if __name__ == '__main__':
    main()
