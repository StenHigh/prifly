"""Release tagging cannot be authorized by incomplete or foreign CI evidence."""
import contextlib
import importlib.util
import io
from pathlib import Path
import sys
import unittest
from unittest.mock import patch
from test_ensure_release_checks import Fake, run, SHA, NOW

spec=importlib.util.spec_from_file_location('tag_release',Path(__file__).with_name('tag-release.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class Qualification(unittest.TestCase):
    def setUp(self):
        self.client=Fake()
        for name,ident in [('verify',1),('qualify',2)]:self.client.data[name]=[run(name,ident)]
        self.commands=[]
    def command(self,*args):
        self.commands.append(args)
        if args[:2]==('git','rev-parse'):return SHA
        if args[:2]==('git','log'):return self.message
        return ''
    def main(self,extra=(),message='candidate'):
        self.message=message
        with patch.object(m,'GitHub',return_value=self.client),patch.object(m,'run',side_effect=self.command),patch('sys.argv',['tag-release.py','v1.2.3',*extra]),patch.object(m.select.__globals__['time'],'time',return_value=NOW),contextlib.redirect_stdout(io.StringIO()):
            return m.main()
    def mutations(self):return [x for x in self.commands if x[1] in ('tag','push')]
    def test_full_evidence_allows_tag_and_push(self):
        self.assertEqual(self.main(),0);self.assertEqual([x[1] for x in self.mutations()],['tag','push'])
    def test_dry_run_never_tags(self):
        self.assertEqual(self.main(['--dry-run']),0);self.assertEqual(self.mutations(),[])
    def test_missing_failed_active_or_foreign_checks_refuse_before_tag(self):
        for candidate in [None,run(ident=2,conclusion='failure'),run(ident=2,status='in_progress'),{**run(ident=2),'head_sha':'b'*40}]:
            with self.subTest(candidate=candidate):
                self.client.data['qualify']=[] if candidate is None else [candidate]
                with self.assertRaises(SystemExit):self.main()
                self.assertEqual(self.mutations(),[])
    def test_skipped_wrapper_is_not_qualification(self):
        self.client.job_data[2]=[]
        with self.assertRaises(SystemExit):self.main()
        self.assertEqual(self.mutations(),[])
    def test_skip_ci_and_invalid_version_refuse(self):
        with self.assertRaises(SystemExit):self.main(message='docs [skip ci]')
        self.assertEqual(self.mutations(),[])
        with patch('sys.argv',['tag-release.py','latest']):
            with self.assertRaises(SystemExit):m.main()


if __name__=='__main__':unittest.main()
