"""The real workflow contract must reject lost jobs and unsafe skip paths."""
import contextlib
import importlib.util
import io
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
s=importlib.util.spec_from_file_location('guard',ROOT/'test/e2e/verify-release-ci.py')
guard=importlib.util.module_from_spec(s);s.loader.exec_module(guard)


class Contract(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup);self.root=Path(self.temp.name)
        for path in ['Makefile','scripts/install.sh','.github/workflows/verify.yml','.github/workflows/qualify.yml','.github/workflows/release.yml']:
            dst=self.root/path;dst.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(ROOT/path,dst)
    def check(self):
        with patch.object(guard,'ROOT',self.root),patch.object(guard,'WORKFLOWS',self.root/'.github/workflows'),contextlib.redirect_stdout(io.StringIO()),contextlib.redirect_stderr(io.StringIO()):
            return guard.main()
    def mutate(self,path,old,new):
        p=self.root/path;s=p.read_text();self.assertIn(old,s);p.write_text(s.replace(old,new,1))
    def test_real_workflows_pass(self):self.assertEqual(self.check(),0)
    def test_batch_job_cannot_bypass_preflight(self):
        self.mutate('.github/workflows/verify.yml',"if: needs.preflight.outputs.execute == 'true'","if: false")
        self.assertEqual(self.check(),1)
    def test_lost_shard_fails(self):
        self.mutate('.github/workflows/verify.yml','shard: [0, 1, 2]','shard: [0, 1]');self.assertEqual(self.check(),1)
    def test_full_e2e_and_race_stay_required(self):
        self.mutate('.github/workflows/qualify.yml','make e2e','make test-changed');self.assertEqual(self.check(),1)
    def test_sha_binding_and_serialization_cannot_disappear(self):
        self.mutate('.github/workflows/qualify.yml','EXPECTED_SHA: ${{ inputs.expected_sha }}','EXPECTED_SHA: ignored');self.assertEqual(self.check(),1)
    def test_release_never_repeats_product_tests(self):
        p=self.root/'.github/workflows/release.yml';p.write_text(p.read_text()+'\n# make race\n');self.assertEqual(self.check(),1)


if __name__=='__main__':unittest.main()
