"""Exact-commit CI evidence shared by preflight, preparation and release tagging."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
from urllib.parse import quote

MAX_AGE = 86400
REQUIRED = {
    'verify': {'static checks':'Static gates', 'engine tests 0/3':'Engine shard',
               'engine tests 1/3':'Engine shard','engine tests 2/3':'Engine shard',
               'other packages':'Every package but the engine'},
    'qualify': {'race':'Race detector suite','e2e':'End-to-end checks'},
}


def command(*args,timeout=45):
    return subprocess.check_output(args,text=True,timeout=timeout).strip()


class GitHub:
    def __init__(self,repo=None):
        self.deadline=None
        self.repo=repo or command('gh','repo','view','--json','nameWithOwner','--jq','.nameWithOwner')
        if not re.fullmatch(r'[\w.-]+/[\w.-]+',self.repo):raise ValueError('invalid repository identity')

    def api(self,path):
        remaining=45 if self.deadline is None else min(45,self.deadline-time.monotonic())
        if remaining<=0:raise TimeoutError('CI observation timeout')
        return json.loads(command('gh','api','--method','GET','repos/'+self.repo+'/'+path,timeout=remaining))

    def pages(self,path,field):
        result=[]
        separator='&' if '?' in path else '?'
        for page in range(1,1001):
            data=self.api(path+separator+f'per_page=100&page={page}')
            chunk=data[field]
            if not isinstance(chunk,list):raise ValueError('invalid paginated GitHub response')
            result.extend(chunk)
            if len(chunk)<100:return result
        raise ValueError('GitHub pagination incomplete; no evidence accepted')

    def workflow(self,name):
        info=self.api('actions/workflows/'+name+'.yml')
        if info['path']!='.github/workflows/'+name+'.yml' or info['state']!='active':
            raise ValueError('workflow identity is not active: '+name)
        return info

    def runs(self,workflow,sha):
        return self.pages(f'actions/workflows/{workflow["id"]}/runs?head_sha={sha}','workflow_runs')

    def jobs(self,run):
        return self.pages(f'actions/runs/{run["id"]}/attempts/{run["run_attempt"]}/jobs','jobs')

    def ref_sha(self,ref):
        return self.api('commits/'+quote(ref,safe=''))['sha']

    def dispatch(self,name,ref,sha,force):
        payload=json.dumps({'ref':ref,'inputs':{'expected_sha':sha,'force':bool(force)}})
        subprocess.run(['gh','api','--method','POST','repos/'+self.repo+'/actions/workflows/'+name+'.yml/dispatches','--input','-'],input=payload,text=True,check=True,timeout=45 if self.deadline is None else max(.1,min(45,self.deadline-time.monotonic())))


def timestamp(value):
    return datetime.fromisoformat(value.replace('Z','+00:00')).timestamp()


def full_batch(name,jobs):
    if len({j['name'] for j in jobs})!=len(jobs):return False
    by_name={j['name']:j for j in jobs}
    for job_name,step_name in REQUIRED[name].items():
        job=by_name.get(job_name,{})
        if job.get('status')!='completed' or job.get('conclusion')!='success':return False
        steps=[s for s in job.get('steps',[]) if s.get('name')==step_name]
        if len(steps)!=1 or steps[0].get('status')!='completed' or steps[0].get('conclusion')!='success':return False
        if not steps[0].get('started_at') or not steps[0].get('completed_at'):return False
    return True


def select(client,name,sha,exclude=None,completed_only=False,allow_pr=False,now=None):
    if not re.fullmatch('[0-9a-f]{40}',sha):raise ValueError('candidate must be an exact commit SHA')
    workflow=client.workflow(name);now=time.time() if now is None else now
    candidates=[]
    for r in client.runs(workflow,sha):
        if (str(r.get('id'))==str(exclude) or r.get('head_sha')!=sha or r.get('workflow_id')!=workflow['id']
            or r.get('path','').split('@')[0]!=workflow['path'] or r.get('repository',{}).get('full_name')!=client.repo
            or r.get('head_repository',{}).get('full_name')!=client.repo
            or r.get('event') not in (('push','workflow_dispatch','pull_request') if allow_pr else ('push','workflow_dispatch'))):continue
        if not isinstance(r.get('run_attempt'),int) or r['run_attempt']<1:continue
        candidates.append(r)
    candidates.sort(key=lambda r:(timestamp(r['run_started_at'] or r['created_at']),r['id']),reverse=True)
    for r in candidates:
        state=r['status'];url=r['html_url']
        if state!='completed':
            if not completed_only:return {'state':'wait','run_id':r['id'],'url':url,'reason':state}
            continue
        jobs=client.jobs(r)
        # A cancelled pending run never began a batch. A successful wrapper is not its source.
        started=any(j.get('steps') for j in jobs)
        if r['conclusion']=='cancelled' and not started:continue
        if r['conclusion']!='success':return {'state':'failed','run_id':r['id'],'url':url,'reason':r['conclusion'] or 'unknown'}
        if not full_batch(name,jobs):continue
        finish=max(timestamp(j['completed_at']) for j in jobs if j['name'] in REQUIRED[name])
        age=now-finish
        if not 0<=age<MAX_AGE:return {'state':'missing','reason':'full batch evidence expired'}
        return {'state':'success','run_id':r['id'],'attempt':r['run_attempt'],'url':url,'finished':finish,'reason':'direct full batch on exact SHA'}
    return {'state':'missing','reason':'no applicable full batch on exact SHA'}


def preflight(name,client=None,sha=None,expected=None,force=None,run_id=None):
    client=client or GitHub(os.environ['GITHUB_REPOSITORY']);sha=sha or os.environ['GITHUB_SHA']
    expected=os.environ.get('EXPECTED_SHA','') if expected is None else expected
    force=(os.environ.get('FORCE_CHECKS')=='true' or int(os.environ.get('GITHUB_RUN_ATTEMPT','1'))>1) if force is None else force
    if expected and sha!=expected:raise ValueError('candidate moved: expected SHA does not match tested SHA')
    if force:choice={'state':'missing','reason':'explicit force'}
    else:
        try:choice=select(client,name,sha,exclude=run_id or os.environ.get('GITHUB_RUN_ID'),completed_only=True,allow_pr=os.environ.get('GITHUB_EVENT_NAME')=='pull_request')
        except (OSError,ValueError,KeyError,TypeError,subprocess.SubprocessError) as err:
            choice={'state':'missing','reason':'API evidence unavailable; execute full batch: '+str(err)}
    if choice['state']=='failed':raise ValueError('previous batch failed; retry explicitly with force: '+choice['url'])
    execute=choice['state']!='success'
    values={'execute':str(execute).lower(),'source_run':str(choice.get('run_id',''))}
    print(json.dumps({'workflow':name,'sha':sha,'decision':'execute' if execute else 'reuse','source':choice},sort_keys=True),flush=True)
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'],'a') as out:
            for k,v in values.items():out.write(k+'='+v+'\n')
    if os.environ.get('GITHUB_STEP_SUMMARY'):
        with open(os.environ['GITHUB_STEP_SUMMARY'],'a') as out:
            out.write(f'{name}: '+('execute full batch' if execute else 'reuse ['+str(choice['run_id'])+']('+choice['url']+')')+' on `'+sha+'`. '+choice['reason']+'\n')
    return values


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--preflight',choices=REQUIRED,required=True)
    return preflight(p.parse_args().preflight)


if __name__=='__main__':
    try:main()
    except (OSError,ValueError,KeyError,TypeError,subprocess.SubprocessError) as err:sys.exit('release checks: '+str(err))
