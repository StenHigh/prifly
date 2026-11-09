#!/usr/bin/env python3
"""Synthetic public-CLI acceptance for change migrate-revoked-shared-schemas.

Preparation/development evidence never closes task 6.2. Released mode requires
an independently verified release asset digest and read-only exact-SHA gates.
No push, tag, release, CI dispatch or private authority repair is implemented.
"""
import argparse
import ast
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def encoded(value):
    # These fixtures contain ASCII keys/strings and integer numbers only.
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False, allow_nan=False).encode()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def reference(identifier, data):
    return dict(id=identifier, version='1.0.0', digest='sha256:'+sha(data))


WORKER = '''import json, os, sys
from pathlib import Path
raw = sys.stdin.buffer.read((2 << 20) + 1)
if len(raw) > 2 << 20: raise RuntimeError("oversized envelope")
e = json.loads(raw)
c = json.loads(Path(os.environ["PRIFLY_CONTEXT_FILE"]).read_text())
if json.loads(Path(c["inputs"]["left"]["path"]).read_text()) != {"value": 7}: raise RuntimeError("left input mismatch")
if json.loads(Path(c["inputs"]["right"]["path"]).read_text()) != {"enabled": True}: raise RuntimeError("right input mismatch")
r = dict(schema_version="1", run_id=e["run_id"], step_instance_id=e["step_instance_id"],
         attempt_id=e["attempt_id"], envelope_digest=os.environ["PRIFLY_ENVELOPE_DIGEST"],
         verdict="pass", outputs={}, evidence_refs=[], effect_receipt_refs=[],
         summary="Both exact shared schemas were consumed by a native process")
with os.fdopen(3, "w", closefd=False) as f:
    json.dump(r, f, separators=(",", ":")); f.write("\\n"); f.flush()
'''


def fixture(builtins):
    schemas = [encoded({'type':'object','required':['value'],'properties':{'value':{'type':'integer'}},'additionalProperties':False}),
               encoded({'type':'object','required':['enabled'],'properties':{'enabled':{'type':'boolean'}},'additionalProperties':False})]
    refs = [reference('acceptance:schema/left', schemas[0]), reference('acceptance:schema/right', schemas[1])]
    inputs = {name:dict(format='json', schema_ref=ref, required=True) for name,ref in zip(['left','right'],refs)}
    step = dict(schema_version='1', id='acceptance:step/read', version='1.0.0', title='Read both shared schemas', kind='command',
                inputs=inputs, outputs={}, executor=dict(adapter_ref=builtins['core:adapter/local-process'], operation='process'),
                context_refs=[], required_capabilities=[], effects=dict(class_='none'), result_check_refs=[],
                result_schema_ref=builtins['core:schema/step-result'])
    step['effects']={'class':'none','retry_class':'pure'}
    step_bytes=encoded(step); step_ref=reference(step['id'],step_bytes)
    resource_bytes=encoded({'description':'Fixture-only durable timeout source; no external events'})
    source_ref=reference('acceptance:source/restart-boundary',resource_bytes)
    bindings={name:{'from':'workflow_input','port':name} for name in inputs}
    def work(next_stage):
        return dict(kind='step',step_ref=step_ref,input_bindings=bindings,on={'pass':next_stage})
    stages={'first':work('restart'), 'second':work('done'),
            'restart':dict(kind='wait',source_ref=source_ref,event_type='fixture',event_schema_ref=refs[0],
                           correlation_input={'from':'literal','value':{'value':7},'schema_ref':refs[0]},
                           timeout_seconds=1,on_event='second',on_timeout='second'),
            'done':dict(kind='finish',outcome='succeeded',output_bindings={})}
    workflow=dict(schema_version='2',id='acceptance:workflow/shared-schemas',version='1.0.0',title='Two workers across a CLI restart',
                  inputs=inputs,outputs={},allowed_outcomes=['succeeded'],definition=dict(entry='first',stages=stages),
                  limits=dict(max_step_instances=2,max_control_transitions=16,max_parallelism=1,max_child_depth=0),policy_ref=builtins['core:policy/local'])
    workflow_bytes=encoded(workflow)
    return dict(schemas=schemas,refs=refs,step=step_bytes,step_ref=step_ref,source=resource_bytes,source_ref=source_ref,
                workflow=workflow_bytes,workflow_ref=reference(workflow['id'],workflow_bytes))


def self_check():
    ast.parse(WORKER)
    names=['core:adapter/local-process','core:schema/step-result','core:policy/local']
    f=fixture({name:reference(name,encoded({'fixture':name})) for name in names})
    require(len({r['digest'] for r in f['refs']})==2,'two distinct schemas required')
    w=json.loads(f['workflow']); require(w['definition']['stages']['first']['on']['pass']=='restart','restart route')
    require(w['definition']['stages']['restart']['on_timeout']=='second','second worker route')
    for ref,data in zip(f['refs'],f['schemas']): require(ref['digest']=='sha256:'+sha(data),'fixture digest')
    print('Preparation self-check passed; no binary, authority or release qualification was executed.')


class Acceptance:
    def __init__(self,args):
        self.args=args; self.binary=args.binary.resolve(strict=True)
        self.output=args.output.absolute(); self.output.mkdir(parents=True,exist_ok=False)
        self.logs=self.output/'transcript'; self.logs.mkdir()
        (self.output/'worker.py').write_text(WORKER)
        self.scratch=Path(tempfile.mkdtemp(prefix='owned-runtime-',dir=self.output)).resolve()
        self.roots=[]; self.counter=0
        self.report=dict(schema_version='schema-migration-release-acceptance/1',mode=args.mode,status='failed',
                         task='6.2',task_closed=False,private_acceptance='not_performed',
                         source_implementation='30c3635081f8f012aef3953072dcd45d0807d618',
                         binary=dict(path=str(self.binary),sha256=sha(self.binary.read_bytes())),
                         runner_sha256=sha(Path(__file__).read_bytes()),worker_sha256=sha(WORKER.encode()),
                         started_at=datetime.now(timezone.utc).isoformat(),commands=[],cases=[],cleanup={})

    def cli(self,label,*argv,root=None,refusal=None):
        self.counter+=1
        command=[str(self.binary),'--json']
        if root is not None: command+=['--project',str(root)]
        command += list(map(str,argv))
        started=time.monotonic()
        try:
            p=subprocess.run(command,cwd=str(root or self.output),capture_output=True,timeout=90)
        except subprocess.TimeoutExpired as err:
            self.report['commands'].append(dict(label=label,argv=command,timeout=True))
            raise RuntimeError('CLI timeout: '+label) from err
        entry=dict(label=label,argv=command,exit_code=p.returncode,elapsed_seconds=round(time.monotonic()-started,3))
        for name in ['stdout','stderr']:
            data=getattr(p,name); path=self.logs/f'{self.counter:03}-{label}.{name}'
            path.write_bytes(data); entry[name]=dict(file=str(path.relative_to(self.output)),sha256=sha(data),bytes=len(data))
        self.report['commands'].append(entry)
        if refusal:
            require(p.returncode!=0 and not p.stdout,'Expected clean refusal: '+label)
            problem=json.loads(p.stderr); require(problem.get('code')==refusal,f'{label}: expected {refusal}, got {problem.get("code")}')
            entry['expected_refusal']=refusal
            return problem
        require(p.returncode==0,f'{label}: exit {p.returncode}; inspect {entry["stderr"]["file"]}')
        return json.loads(p.stdout)

    def package(self,root,label,files,components,deps=()):
        folder=self.output/'fixtures'/label; folder.mkdir(parents=True)
        declared=[]
        for name,data in sorted(files.items()):
            p=folder/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(data)
            declared.append(dict(path=name,digest='sha256:'+sha(data),size_bytes=len(data),media_type='application/json',role='data'))
        manifest=dict(schema_version='1',id='acceptance:package/'+label,version='1.0.0',description='Synthetic release acceptance only',
                      requires_core_protocol='1',dependencies=list(deps),components=components,files=declared,requested_capabilities=[],license='MIT')
        data=encoded(manifest); (folder/'prifly.package.json').write_bytes(data)
        # Public manifest filename is checked by self-preparation against source.
        self.cli('import-'+label,'package','import','--dir',folder,'--reason','reviewed synthetic fixture','--command-id','command:import-'+label,root=root)
        return reference(manifest['id'],data)

    def lifecycle(self,root,ref,status,label):
        return self.cli(label,'package',status,'--id',ref['id'],'--version',ref['version'],'--reason','synthetic reviewed lifecycle',
                        '--command-id','command:'+label,root=root)

    def setup(self,label,two_legacy=False):
        root=self.scratch/label;root.mkdir()
        registration=Path.home()/'.prifly/monitor/sources'/sha(str(root).encode())
        require(not registration.exists() and not registration.is_symlink(),'Unexpected existing monitor source')
        self.roots.append((root,registration))
        self.cli('init-'+label,'init','--profile','core-workflow/1',root,root=root)
        inventory=self.cli('inventory-'+label,'inventory',root=root)['definitions']
        names=['core:adapter/local-process','core:schema/step-result','core:policy/local']
        builtins={n:next(d['ref'] for d in inventory if d['ref']['id']==n and d['ref']['version']=='1.0.0') for n in names}
        f=fixture(builtins)
        shared={'left.json':f['schemas'][0],'right.json':f['schemas'][1]}
        components=[dict(kind='schema',ref=ref,path=path) for ref,path in zip(f['refs'],shared)]
        (root/'workflow.json').write_bytes(f['workflow']);(root/'worker.py').write_text(WORKER)
        (root/'left.json').write_bytes(encoded({'value':7}));(root/'right.json').write_bytes(encoded({'enabled':True}))
        (root/'brief.json').write_bytes(encoded(dict(schema_version='1',id='acceptance:brief/scoped',subject='Synthetic shared schemas',
             desired_outcome='Two admitted workers across CLI process restart',in_scope=['Temporary authority only'],out_of_scope=['Private authority','Publishing'],
             completion_criteria=['Two exact schemas consumed twice'],source_refs=[],assumptions=[],confirmation='explicit')))
        config=json.loads((root/'prifly.json').read_bytes())
        config['configuration']['executors']['acceptance:step/read']=dict(executable=str(Path(sys.executable).resolve()),args=['-B','worker.py'],
             files={'worker.py':'worker.py'},environment={},timeout_ms=10000,grace_ms=100,max_output_bytes=1048576)
        (root/'prifly.json').write_bytes(encoded(config))
        (root/'step.json').write_bytes(f['step']);(root/'source.json').write_bytes(f['source'])
        entries=[dict(kind='step',ref=f['step_ref'],path='step.json'),dict(kind='resource',ref=f['source_ref'],path='source.json')]
        (root/'definitions.json').write_bytes(encoded(dict(schema_version='1',entries=entries)))
        legacy=[self.package(root,label+'-legacy-one',shared,components)]
        old=None
        if two_legacy:
            old=self.start(root,'old-run')['run']
            self.lifecycle(root,legacy[0],'revoke','initial-revoke-0')
            legacy.append(self.package(root,label+'-legacy-two',shared,components))
            self.lifecycle(root,legacy[1],'revoke','initial-revoke-1')
        if label=='remove': self.lifecycle(root,legacy[0],'remove',label+'-remove')
        supplier=legacy[0];consumer=None
        if two_legacy or label=='remove':
            supplier=self.package(root,label+'-supplier',shared,components)
            # This is the documented authored registry, not authority state.
            # Remove authored entries before importing those same components.
            (root/'definitions.json').write_bytes(encoded(dict(schema_version='1',entries=[d for d in entries if d['kind']=='resource'])))
            owned={'workflow.json':f['workflow'],'step.json':f['step'],'source.json':f['source']}
            consumer=self.package(root,label+'-consumer',owned,[dict(kind=k,ref=f[r],path=p) for k,r,p in
                                  [('workflow','workflow_ref','workflow.json'),('step','step_ref','step.json')]], [supplier])
        return root,f,legacy,supplier,consumer,old

    def start(self,root,label,decision=None,refusal=None):
        args=['run','start','--workflow','workflow.json','--brief','brief.json','--input','left=left.json','--input','right=right.json','--command-id','command:'+label]
        if decision: args+=['--schema-migration',decision]
        result=self.cli(label,*args,root=root,refusal=refusal)
        if refusal: return result
        return self.cli(label+'-status','run','status',result['receipt']['run_id'],root=root)

    def drive_two(self,root,run,label):
        one=self.cli(label+'-first','run','drive',run['id'],root=root)['run']
        require(one['status']=='waiting' and len(one['attempts'])==1,'Expected one settled worker and durable wait')
        require(not one.get('active_attempt_ids'),'Worker remained active at restart boundary')
        restarted=self.cli(label+'-new-process','run','status',run['id'],root=root)['run']
        require(restarted.get('schema_migration')==one.get('schema_migration'),'Restart changed binding')
        require(sorted(restarted['attempts'])==sorted(one['attempts']),'Restart changed first attempt')
        time.sleep(1.1)
        done=self.cli(label+'-second','run','drive',run['id'],root=root)['run']
        require(done['status']=='completed' and done['outcome']=='succeeded' and len(done['attempts'])==2,'Second worker did not finish after restart')
        require(not done.get('active_attempt_ids'),'Completed Run has active attempts')
        return done

    def baseline(self,mode):
        root,f,legacy,supplier,consumer,_=self.setup(mode)
        started=self.start(root,mode+'-start')['run'];done=self.drive_two(root,started,mode)
        require('schema_migration' not in done,'Ordinary Start acquired migration')
        self.report['cases'].append(dict(id='baseline-'+mode,status='passed',run_id=done['id'],attempts=2))

    def migration(self):
        root,f,legacy,supplier,consumer,old=self.setup('migration',two_legacy=True)
        def runs(): return self.cli('population','run','list',root=root)['runs']
        before=runs();self.start(root,'original-refusal',refusal='package_not_resolvable');require(runs()==before,'Refused Start created a Run')
        def request(selections):
            p=root/'request.json';p.write_bytes(encoded(dict(schema_version='schema-migration-request/1',consumer_ref=consumer,selections=selections)));return p
        def prepare(path,label): return self.cli(label,'package','schema-migration','prepare','--request',path,root=root)
        def population():
            return [self.cli('packages','package','list',root=root),self.cli('runs','run','list',root=root),self.cli('claims','claim','list',root=root),self.cli('cut','telemetry','catalog',root=root)['cut']]
        snapshot=population();diagnostic=prepare(request([]),'diagnostic');require(population()==snapshot,'Prepare changed authority')
        require(not diagnostic['applicable'] and len(diagnostic['schemas'])==2,'Incomplete diagnostic')
        require(all(len(s['revocations'])==2 for s in diagnostic['schemas']),'Not all revoked suppliers were shown')
        selections=[dict(schema_ref=ref,supplier_ref=supplier) for ref in f['refs']]
        request_path=request(selections);plan=prepare(request_path,'reviewed-before-stale')
        def apply(plan,label,reason='reviewed exact schemas',refusal=None):
            path=root/(label+'-plan.json');path.write_bytes(encoded(plan))
            return self.cli(label,'package','schema-migration','apply','--plan',path,'--expected-review-digest',plan['review_digest'],
                            '--reason',reason,'--command-id','command:'+label,root=root,refusal=refusal)
        sentinel=encoded({'type':'null'})
        self.package(root,'unrelated-after-prepare',{'sentinel.json':sentinel},[dict(kind='schema',ref=reference('acceptance:schema/sentinel',sentinel),path='sentinel.json')])
        facts=self.cli('facts-before-stale','package','list',root=root)
        apply(plan,'stale-apply',refusal='schema_migration_plan_stale')
        require(self.cli('facts-after-stale','package','list',root=root)==facts,'Stale apply changed package/trust facts')
        plan=prepare(request_path,'fresh-reviewed');require(plan['complete'] and plan['applicable'],'Fresh plan not applicable')
        applied=apply(plan,'accepted-apply');decision=applied['decision']['id']
        require(applied['decision']['reason']=='reviewed exact schemas','Owner reason lost')
        inspection=self.cli('inspect-after-new-process','package','schema-migration','inspect','--id',decision,root=root)
        require(inspection['valid'],'Recorded decision did not survive process restart')
        packages=self.cli('upgraded-package-state','package','list',root=root)
        require(applied['receipt']['version']==plan['packages_version']+1 and applied['receipt']['cut']==applied['decision']['commit_cut']==plan['cut']+1,'Decision was not one atomic authority commit')
        require(all(next(p for p in packages['packages'] if p['ref']==ref)['status']=='revoked' for ref in legacy),'Apply restored a revoked package')
        new=self.start(root,'explicit-start',decision)
        require(new['schema_version']=='core-read/45','New read boundary missing')
        binding=new['run']['schema_migration']; require(binding['consumer_ref']==consumer and binding['decision_id']==decision,'Wrong binding')
        require(binding['selections']==selections or sorted(binding['selections'],key=lambda x:x['schema_ref']['id'])==sorted(selections,key=lambda x:x['schema_ref']['id']),'Wrong supplier/schema binding')
        done=self.drive_two(root,new['run'],'migrated')
        self.start(root,'no-implicit-selection',refusal='package_not_resolvable')
        old_before=self.cli('old-history-before','run','status',old['id'],root=root)
        require('schema_migration' not in old_before['run'],'Old Run acquired a binding')
        self.cli('old-run-refusal','run','drive',old['id'],root=root,refusal='package_revoked')
        old_after=self.cli('old-history-after','run','status',old['id'],root=root)
        require(not old_after['run']['attempts'] and 'schema_migration' not in old_after['run'],'Old Run admitted work')
        require(old_before['run']['workflow_ref']==old_after['run']['workflow_ref'],'Old pinned workflow changed')
        partial=self.start(root,'pending-before-new-revoke',decision)['run']
        waiting=self.cli('one-worker-before-new-revoke','run','drive',partial['id'],root=root)['run']
        require(waiting['status']=='waiting' and len(waiting['attempts'])==1,'Invalidation fixture did not reach first worker')
        self.lifecycle(root,legacy[0],'revoke','repeat-security-revoke')
        require(not self.cli('invalidated-decision','package','schema-migration','inspect','--id',decision,root=root)['valid'],'Repeated revoke did not invalidate decision')
        time.sleep(1.1)
        self.cli('later-admission-refusal','run','drive',partial['id'],root=root,refusal='schema_migration_decision_invalid')
        require(len(self.cli('no-later-worker','run','status',partial['id'],root=root)['run']['attempts'])==1,'Worker admitted after decision invalidation')
        repeated=apply(plan,'accepted-apply')
        require(repeated['duplicate'] and repeated['receipt']==applied['receipt'] and repeated['decision']==applied['decision'],'Exact lost-response retry changed receipt')
        apply(plan,'accepted-apply',reason='changed payload',refusal='command_conflict')
        self.report['cases'].append(dict(id='scoped-migration',status='passed',schemas=f['refs'],legacy_packages=legacy,supplier=supplier,consumer=consumer,
             old_run=old['id'],new_run=done['id'],invalidated_run=partial['id'],decision_id=decision,review_digest=plan['review_digest'],
             subsequent_worker_attempts=2,covered_revocations_per_schema=2,restart='new CLI process across durable wait',
             old_run_refusal='package_revoked',stale_refusal='schema_migration_plan_stale',repeat_revoke_refusal='schema_migration_decision_invalid',
             exact_receipt_replay=True))

    def cleanup(self):
        removed=[]
        # Only this runner's newly-created roots and exact content-matched
        # registrations are eligible. Never prune or edit unrelated entries.
        for root,registration in self.roots:
            if registration.exists() or registration.is_symlink():
                require(not registration.is_symlink(),'Refuse cleanup of unexpected monitor symlink')
                require(registration.read_text()==str(root),'Monitor registration identity changed; manual inspection required')
                require(registration.stat().st_uid==os.getuid(),'Monitor registration owner changed')
                registration.unlink();removed.append(str(registration))
        for directory,dirs,_ in os.walk(self.scratch,followlinks=False):
            p=Path(directory)
            if not p.is_symlink(): p.chmod(p.stat().st_mode|0o700)
        shutil.rmtree(self.scratch)
        require(not self.scratch.exists(),'Temporary authorities remain')
        require(all(not p.exists() and not p.is_symlink() for _,p in self.roots),'Monitor registration remains')
        self.report['cleanup']=dict(status='passed',temporary_runtime_removed=True,monitor_registrations_removed=removed,
                                    retained='evidence and command transcripts only')

    def run(self):
        try:
            require(sys.platform=='linux','Prepared monitor cleanup is verified for Linux only')
            digest=self.report['binary']['sha256']
            if self.args.expected_sha256: require(digest==self.args.expected_sha256.removeprefix('sha256:'),'Binary digest mismatch')
            version=self.cli('version','version');self.report['binary']['version']=version
            caps=self.cli('capabilities','capabilities')
            core=next((p for p in caps['profiles'] if p['semantics_profile']=='core-workflow/1'),{})
            require('shared_schema_migration' in core.get('capabilities',[]) and 'core-read/45' in core.get('read_versions',[]),'Binary does not declare shared_schema_migration/read 45')
            if self.args.mode=='released':
                require(version['version'].removeprefix('v')==self.args.release_tag.removeprefix('v'),'Version/tag mismatch')
                gates=json.loads(self.args.gates.read_bytes())
                require(gates['sha']==self.args.release_sha and gates['release_tag']==self.args.release_tag,'Release/gate identity mismatch')
                require(gates['status']=='passed' and all(gates['checks'][k]['state']=='success' for k in ['verify','qualify']),'Required fixed-SHA gates did not pass')
                require(0<=time.time()-gates['collected_at_epoch']<86400,'Gate collection is expired or from future')
                self.report['release']=dict(tag=self.args.release_tag,sha=self.args.release_sha,asset_url=self.args.asset_url,
                      expected_binary_sha256=self.args.expected_sha256,gates=gates,
                      provenance_basis='Owner verifies binary digest against published signed manifest; runner checks supplied digest and read-only GitHub gate record')
            self.baseline('clean');self.baseline('remove');self.migration()
            require(sha(self.binary.read_bytes())==digest,'Binary changed during acceptance')
            self.report['status']='passed'
        except Exception as err:
            self.report['failure']=dict(type=type(err).__name__,message=str(err))
        finally:
            try: self.cleanup()
            except Exception as err:
                self.report['status']='failed';self.report['cleanup']=dict(status='failed',message=str(err),scratch=str(self.scratch))
            self.report['finished_at']=datetime.now(timezone.utc).isoformat()
            # Owner must review evidence and record task completion separately.
            self.report['task_closed']=False
            (self.output/'evidence.json').write_bytes(encoded(self.report)+b'\n')
        print(json.dumps(dict(status=self.report['status'],mode=self.args.mode,evidence=str(self.output/'evidence.json'),task_closed=False)))
        return 0 if self.report['status']=='passed' else 1


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--self-check',action='store_true');p.add_argument('--binary',type=Path);p.add_argument('--output',type=Path)
    p.add_argument('--mode',choices=['development','released'],default='development')
    p.add_argument('--release-tag');p.add_argument('--release-sha');p.add_argument('--expected-sha256');p.add_argument('--asset-url');p.add_argument('--gates',type=Path)
    a=p.parse_args()
    if a.self_check: self_check();return 0
    if not a.binary or not a.output: p.error('--binary and new --output are required')
    if a.mode=='released':
        if not all([a.release_tag,a.release_sha,a.expected_sha256,a.asset_url,a.gates]): p.error('released mode requires --release-tag --release-sha --expected-sha256 --asset-url --gates')
        if not re.fullmatch('[0-9a-f]{40}',a.release_sha): p.error('release-sha must be exact 40-hex commit SHA')
        if not re.fullmatch('(sha256:)?[0-9a-f]{64}',a.expected_sha256): p.error('expected-sha256 must be 64-hex SHA256')
        if not a.asset_url.startswith('https://github.com/StenHigh/prifly/releases/download/'+a.release_tag+'/'): p.error('asset-url must name the selected repository release asset')
    return Acceptance(a).run()


if __name__=='__main__':
    try: sys.exit(main())
    except (OSError,ValueError,RuntimeError) as err: sys.exit('acceptance: '+str(err))
