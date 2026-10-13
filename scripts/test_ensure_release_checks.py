"""CI evidence is exact and complete; preparation counts actual dispatches."""
import contextlib
from copy import deepcopy
import importlib.util
import io
from pathlib import Path
import sys
import unittest
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).parent))
import release_checks as checks

s=importlib.util.spec_from_file_location('ensure_release',Path(__file__).with_name('ensure-release-checks.py'))
ensure=importlib.util.module_from_spec(s);s.loader.exec_module(ensure)
SHA='a'*40
NOW=1800000000
STAMP='2027-01-15T07:59:00Z'


def run(name='qualify',ident=1,status='completed',conclusion='success',event='workflow_dispatch'):
    return {'id':ident,'workflow_id':10 if name=='qualify' else 20,'path':'.github/workflows/'+name+'.yml',
            'head_sha':SHA,'repository':{'full_name':'owner/repo'},'head_repository':{'full_name':'owner/repo'},
            'event':event,'run_attempt':1,'run_started_at':STAMP,'created_at':STAMP,'status':status,
            'conclusion':conclusion,'html_url':'https://github.com/owner/repo/actions/runs/'+str(ident)}


def jobs(name):
    return [{'name':n,'status':'completed','conclusion':'success','completed_at':STAMP,
             'steps':[{'name':step,'status':'completed','conclusion':'success','started_at':STAMP,'completed_at':STAMP}]} for n,step in checks.REQUIRED[name].items()]


class Fake:
    repo='owner/repo'
    def __init__(self):self.data={'verify':[],'qualify':[]};self.job_data={};self.sent=[];self.ref=SHA
    def workflow(self,name):return {'id':10 if name=='qualify' else 20,'path':'.github/workflows/'+name+'.yml'}
    def runs(self,w,sha):return self.data['qualify' if w['id']==10 else 'verify']
    def jobs(self,r):return self.job_data.get(r['id'],jobs('qualify' if r['workflow_id']==10 else 'verify'))
    def ref_sha(self,ref):return self.ref
    def dispatch(self,*args):self.sent.append(args)


class Selector(unittest.TestCase):
    def setUp(self):
        self.client=Fake();self.r=run();self.client.data['qualify']=[self.r]
        self.clock=patch.object(checks.time,'time',return_value=NOW);self.clock.start();self.addCleanup(self.clock.stop)
    def choice(self,**kwargs):return checks.select(self.client,'qualify',SHA,now=NOW,**kwargs)
    def test_direct_full_success(self):self.assertEqual(self.choice()['state'],'success')
    def test_identity_and_foreign_sha_or_pr(self):
        for key,value in [('head_sha','b'*40),('workflow_id',99),('path','.github/workflows/fake.yml'),('event','pull_request'),('repository',{'full_name':'fork/repo'}),('head_repository',{'full_name':'fork/repo'})]:
            with self.subTest(key=key):
                changed=deepcopy(self.r);changed[key]=value;self.client.data['qualify']=[changed]
                self.assertEqual(self.choice()['state'],'missing')
    def test_skipped_missing_duplicate_or_empty_step_never_qualify(self):
        cases=[[],jobs('qualify')[:1],jobs('qualify')+jobs('qualify')[:1]]
        for field,value in [('conclusion','skipped'),('steps',[])]:
            j=jobs('qualify');j[0][field]=value;cases.append(j)
        for j in cases:
            self.client.job_data[1]=j;self.assertEqual(self.choice()['state'],'missing')
    def test_reuse_wrapper_is_ignored_and_chain_without_direct_source_refused(self):
        self.client.data['qualify']=[run(ident=2),self.r];self.client.job_data[2]=[{'name':'preflight','steps':[{'name':'proof'}]}]
        self.assertEqual(self.choice()['run_id'],1)
        self.client.data['qualify']=[run(ident=2)];self.assertEqual(self.choice()['state'],'missing')
    def test_later_failure_revokes_prior_success(self):
        self.client.data['qualify']=[run(ident=2,conclusion='failure'),self.r]
        self.assertEqual(self.choice()['state'],'failed')
    def test_pending_cancel_and_preflight_self_do_not_become_evidence(self):
        self.client.data['qualify']=[run(ident=2,conclusion='cancelled'),self.r];self.client.job_data[2]=[]
        self.assertEqual(self.choice()['run_id'],1)
        self.client.data['qualify']=[run(ident=3,status='in_progress'),self.r]
        self.assertEqual(self.choice()['state'],'wait');self.assertEqual(self.choice(exclude=3,completed_only=True)['run_id'],1)
    def test_age_and_incomplete_clock_are_not_pass(self):
        self.assertEqual(checks.select(self.client,'qualify',SHA,now=NOW+checks.MAX_AGE)['state'],'missing')
        self.assertEqual(checks.select(self.client,'qualify',SHA,now=0)['state'],'missing')
    def test_all_pages_are_read(self):
        client=checks.GitHub('owner/repo');calls=[]
        def api(path):
            calls.append(path);return {'workflow_runs':[self.r]*100 if path.rsplit('&page=',1)[-1]=='1' else [self.r]}
        with patch.object(client,'api',side_effect=api):self.assertEqual(len(client.pages('runs?head_sha='+SHA,'workflow_runs')),101)
        self.assertEqual(len(calls),2)
    def test_expected_sha_prevents_execution_on_moved_candidate(self):
        with self.assertRaises(ValueError):checks.preflight('qualify',self.client,SHA,expected='b'*40,force=True)
    def test_preflight_success_force_and_failure(self):
        with patch('sys.stdout',new=io.StringIO()):
            self.assertEqual(checks.preflight('qualify',self.client,SHA,expected=SHA,force=False,run_id=99)['execute'],'false')
            self.assertEqual(checks.preflight('qualify',self.client,SHA,expected=SHA,force=True)['execute'],'true')
            self.client.data['qualify']=[run(conclusion='failure')]
            with self.assertRaises(ValueError):checks.preflight('qualify',self.client,SHA,expected=SHA,force=False)


class Preparation(unittest.TestCase):
    def setUp(self):
        self.clock=patch.object(checks.time,'time',return_value=NOW);self.clock.start();self.addCleanup(self.clock.stop)
        self.client=Fake();self.output=io.StringIO();self.silence=contextlib.redirect_stdout(self.output);self.silence.__enter__();self.addCleanup(self.silence.__exit__,None,None,None)
    def prep(self,**kwargs):
        with patch.object(checks,'time') as clock:
            clock.time.return_value=NOW
            return ensure.prepare(self.client,SHA,'main',**kwargs)
    def green(self):
        for name,ident in [('verify',1),('qualify',2)]:self.client.data[name]=[run(name,ident)]
    def test_success_reuses_and_missing_dispatches_once(self):
        self.green();self.assertEqual(self.prep()[0],0);self.assertEqual(self.client.sent,[])
        self.client.data['qualify']=[];self.assertEqual(self.prep()[0],3);self.assertEqual(len(self.client.sent),1)
    def test_active_and_failure_never_dispatch_implicitly(self):
        self.green();self.client.data['qualify']=[run(ident=2,status='in_progress')]
        self.assertEqual(self.prep()[0],3);self.assertEqual(self.client.sent,[])
        self.client.data['qualify']=[run(ident=2,conclusion='failure')]
        self.assertEqual(self.prep()[0],1);self.assertEqual(self.client.sent,[])
    def test_timeout_visibility_gap_never_dispatch_twice(self):
        tick=[0]
        def pause(seconds):tick[0]+=seconds
        self.assertEqual(self.prep(wait=True,timeout=31,clock=lambda:tick[0],pause=pause)[0],3)
        self.assertEqual([x[0] for x in self.client.sent],['verify','qualify'])
    def test_api_timeout_remains_pending(self):
        with patch.object(self.client,'workflow',side_effect=TimeoutError('deadline')):
            self.assertEqual(self.prep(wait=True)[0],3)
        self.assertEqual(self.client.sent,[])

    def test_force_cannot_accept_the_old_pass_before_new_run_is_visible(self):
        self.green();tick=[0]
        def pause(seconds):tick[0]+=seconds
        self.assertEqual(self.prep(force=True,wait=True,timeout=16,clock=lambda:tick[0],pause=pause)[0],3)
        self.assertEqual(len(self.client.sent),2)
    def test_force_waits_for_new_attempt_after_failed_source(self):
        self.green();self.client.data['qualify']=[run(ident=2,conclusion='failure')];tick=[0]
        def pause(seconds):tick[0]+=seconds
        self.assertEqual(self.prep(force=True,wait=True,timeout=16,clock=lambda:tick[0],pause=pause)[0],3)
        self.assertEqual(len(self.client.sent),2)

    def test_moved_ref_and_api_failure_never_pass(self):
        self.client.ref='b'*40
        with self.assertRaises(ValueError):self.prep()
        self.assertEqual(self.client.sent,[])
        with patch.object(self.client,'workflow',side_effect=ValueError('API unavailable')):
            with self.assertRaises(ValueError):self.prep()
    def test_serialized_second_preflight_reuses_actual_first_batch(self):
        with patch('sys.stdout',new=io.StringIO()):
            self.assertEqual(checks.preflight('qualify',self.client,SHA,expected=SHA,force=False)['execute'],'true')
            self.client.data['qualify']=[run(ident=8)]
            self.assertEqual(checks.preflight('qualify',self.client,SHA,expected=SHA,force=False,run_id=9)['execute'],'false')


if __name__=='__main__':unittest.main()
