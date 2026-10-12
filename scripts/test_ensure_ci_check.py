"""Local evidence must count actual executions and reject changed inputs."""
import importlib.util
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('ensure_ci',Path(__file__).with_name('ensure-ci-check.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class Evidence(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
        self.output=io.StringIO();self.stdout=contextlib.redirect_stdout(self.output);self.stdout.__enter__();self.addCleanup(self.stdout.__exit__,None,None,None)
        self.count=[];self.env={'PATH':os.environ['PATH'],'CUSTOM_SECRET':'canary'};self.context='test-authorized'
        self.git('init','-q');self.git('config','user.name','test');self.git('config','user.email','test@example.invalid')
        (self.root/'Makefile').write_text('ci-check: test vuln-check\n');(self.root/'code.go').write_text('first')
        (self.root/'.gitignore').write_text('.cache/\nignored*\n');self.git('add','.');self.git('commit','-qm','base')
        # The fixture substitutes a deterministic tiny toolchain, not the identity policy or command counting.
        self.patcher=patch.object(m,'tool_context',side_effect=lambda r,g,c,e,s: self.tools(g,c,e,s));self.patcher.start();self.addCleanup(self.patcher.stop)
        self.runner=patch.object(m,'execute',side_effect=self.execute);self.runner.start();self.addCleanup(self.runner.stop)
        self.modpatch=patch.object(m.subprocess,'check_call',return_value=0);self.mod=self.modpatch.start();self.addCleanup(self.modpatch.stop)

    def git(self,*args):
        return subprocess.check_output(['git','-C',str(self.root),*args]).decode().strip()

    def tools(self,go,context,env,salt):
        if not context:raise ValueError('unnamed context')
        return {'go':go,'context':context,'env':m.digest(salt+json.dumps(env,sort_keys=True).encode())}

    def execute(self,root,go,target,log,env):
        self.count.append(target);log.write_text('actual '+target);return 0

    def ensure(self,**kwargs):
        return m.ensure(self.root,kwargs.pop('go','/fake/go'),kwargs.pop('context',self.context),env=kwargs.pop('env',self.env),**kwargs)

    def receipt(self):
        return self.root/'.cache/verification/success.json'

    def test_unchanged_pipeline_and_fresh_security(self):
        self.assertEqual(self.ensure(),0);record=self.receipt().read_bytes()
        self.git('add','code.go')
        self.assertEqual(self.ensure(for_commit=True),0);self.assertEqual(self.count,['ci-check','vuln-check'])
        self.assertEqual(self.receipt().read_bytes(),record)
        self.assertNotIn('canary',record.decode())
        self.assertEqual(self.mod.call_count,2)

    def test_dirty_bytes_modes_deletion_and_untracked_or_ignored_inputs(self):
        self.ensure()
        for change in [lambda:(self.root/'code.go').write_text('second'),lambda:(self.root/'code.go').write_text('third'),lambda:(self.root/'code.go').chmod(0o755),lambda:(self.root/'code.go').unlink(),lambda:(self.root/'new.fixture').write_text('new'),lambda:(self.root/'ignored.fixture').write_text('hidden'),lambda:(self.root/'README.md').write_text('docs')]:
            change();self.assertEqual(self.ensure(),0);self.assertEqual(self.count[-1],'ci-check')

    def test_context_toolchain_environment_and_head_invalidate(self):
        self.ensure();self.ensure(context='restricted');self.ensure(go='/new/go');self.ensure(env={**self.env,'UNKNOWN_OVERRIDE':'x'})
        self.git('commit','--allow-empty','-qm','new head');self.ensure()
        self.assertEqual(self.count,['ci-check']*5)

    def test_unknown_context_is_execute_only(self):
        self.ensure(context='');self.ensure(context='');self.assertEqual(self.count,['ci-check']*2);self.assertFalse(self.receipt().exists())

    def test_expiry_force_corruption_and_missing_log(self):
        self.ensure();r=json.loads(self.receipt().read_text());r['finished']-=m.MAX_AGE;self.receipt().write_text(json.dumps(r));self.ensure()
        self.ensure(force=True);self.receipt().write_text('broken');self.ensure()
        r=json.loads(self.receipt().read_text());(self.receipt().parent/r['log']).unlink();self.ensure()
        self.assertEqual(self.count,['ci-check']*5)

    def test_failure_and_interrupt_do_not_leave_success(self):
        self.ensure()
        with patch.object(m,'execute',return_value=1):self.assertEqual(self.ensure(),1)
        self.assertFalse(self.receipt().exists())
        self.ensure()
        with patch.object(m,'execute',side_effect=KeyboardInterrupt):
            with self.assertRaises(KeyboardInterrupt):self.ensure()
        self.assertFalse(self.receipt().exists())

    def test_change_during_execution_never_records_pass(self):
        def mutate(root,go,target,log,env):
            log.write_text('ran');(root/'code.go').write_text('racing edit');return 0
        with patch.object(m,'execute',side_effect=mutate):self.assertEqual(self.ensure(),2)
        self.assertFalse(self.receipt().exists())

    def test_partial_staging_is_not_a_tested_commit(self):
        (self.root/'code.go').write_text('staged');self.git('add','code.go');(self.root/'code.go').write_text('different')
        self.assertEqual(self.ensure(for_commit=True),2);self.assertEqual(self.count,[])

    def test_concurrent_calls_have_one_full_batch(self):
        def slow(*args):time.sleep(.05);return self.execute(*args)
        result=[]
        with patch.object(m,'execute',side_effect=slow):
            workers=[threading.Thread(target=lambda:result.append(self.ensure())) for _ in range(2)]
            for t in workers:t.start()
            for t in workers:t.join()
        self.assertEqual(result,[0,0]);self.assertEqual(self.count,['ci-check','vuln-check'])

    def test_real_make_execution_counts_complete_and_security_batches(self):
        self.runner.stop()
        (self.root/'Makefile').write_text('ci-check:\n\t@echo executed\n\t@printf full\\n >> .cache/count\n'+'vuln-check:\n\t@printf scan\\n >> .cache/count\n')
        self.assertEqual(self.ensure(go=__import__('sys').executable),0)
        self.assertEqual(self.ensure(go=__import__('sys').executable),0)
        text=(self.root/'.cache/count').read_text()
        self.assertEqual(text.count('full'),1);self.assertEqual(text.count('scan'),1)

    def test_only_generated_go_debug_map_is_normalized(self):
        a={'GOGCCFLAGS':'-arch arm64 -ffile-prefix-map=/tmp/go-build123=/tmp/go-build -g'}
        b={'GOGCCFLAGS':'-arch arm64 -ffile-prefix-map=/tmp/go-build456=/tmp/go-build -g'}
        self.assertEqual(m.go_settings(a),m.go_settings(b))
        b['GOGCCFLAGS']=b['GOGCCFLAGS'].replace('arm64','amd64')
        self.assertNotEqual(m.go_settings(a),m.go_settings(b))
        self.assertIn('go-build123',a['GOGCCFLAGS'])

    def test_controller_flags_do_not_change_the_test_environment(self):
        self.ensure(env={**self.env,'CHECK_FOR_COMMIT':'1','CHECK_CONTEXT':self.context})
        self.ensure(env=self.env)
        self.assertEqual(self.count,['ci-check','vuln-check'])

    def test_tool_directory_links_bind_the_actual_target_bytes(self):
        tool=self.root/'tool';(tool/'real').mkdir(parents=True);(tool/'real/header.h').write_text('first')
        (tool/'alias').symlink_to('real',target_is_directory=True)
        first=m.tool_tree(tool);(tool/'real/header.h').write_text('changed')
        self.assertNotEqual(first,m.tool_tree(tool))
        (tool/'outside').symlink_to(self.root,target_is_directory=True)
        with self.assertRaises(ValueError):m.tool_tree(tool)

    def test_untrusted_log_path_is_not_read(self):
        self.ensure();r=json.loads(self.receipt().read_text());r['log']='../outside';self.receipt().write_text(json.dumps(r));self.ensure()
        self.assertEqual(self.count,['ci-check']*2)


if __name__=='__main__':unittest.main()
