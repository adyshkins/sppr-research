"""Offline tests for publication guards. No GitHub requests are made."""
import contextlib
import hashlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import publish

class FakeClient:
    calls=[]
    wrong_author=False
    changed_head=False
    def __init__(self,token): self.head=publish.EXPECTED_HEAD
    def call(self,method,path,payload=None):
        self.calls.append((method,path,payload))
        if path=='/user':return {'login':'adyshkins'}
        if path.endswith('/sppr-prototype-back') and method=='GET':return {'id':publish.REPO_ID,'owner':{'login':'adyshkins'},'full_name':'adyshkins/sppr-prototype-back','default_branch':'main','name':'sppr-prototype-back','html_url':'https://github.com/adyshkins/sppr-prototype-back'}
        if path.endswith('/git/ref/heads/main'):
            return {'object':{'sha':'changed' if self.changed_head else self.head}}
        if '/git/ref/heads/backup/' in path:raise RuntimeError('GET backup: HTTP 404: Not Found')
        if path.endswith('/git/refs') and method=='POST':return {'object':{'sha':publish.EXPECTED_HEAD}}
        if path.endswith('/git/trees'):return {'sha':'tree'}
        if path.endswith('/git/commits'):
            identity=dict(publish.AUTHOR)
            if self.wrong_author:identity['email']='different@example.test'
            return {'sha':'newcommit','author':identity,'committer':dict(publish.AUTHOR)}
        if path.endswith('/git/refs/heads/main') and method=='PATCH':
            assert payload['force'] is False
            self.head=payload['sha'];return {'object':{'sha':self.head}}
        if path.endswith('/sppr-prototype-back') and method=='PATCH':return {'id':publish.REPO_ID,'name':payload['name'],'html_url':'https://github.com/adyshkins/'+payload['name']}
        raise AssertionError((method,path))

class PublisherTests(unittest.TestCase):
    def setUp(self):FakeClient.calls=[];FakeClient.wrong_author=False;FakeClient.changed_head=False
    def invoke(self,args):
        with patch.object(publish,'Client',FakeClient),patch.object(publish,'source_files',return_value=[{'path':'README.md','mode':'100644','type':'blob','content':'test'}]),patch.object(publish.getpass,'getpass',return_value='offline-test'),patch.dict(publish.os.environ,{'GH_TOKEN':''}),patch('sys.argv',['publish.py']+args),contextlib.redirect_stdout(io.StringIO()):publish.main()
    def test_dry_run_has_no_writes(self):
        self.invoke([]);self.assertTrue(all(m=='GET' for m,_,_ in FakeClient.calls))
    def test_explicit_author_and_fast_forward(self):
        self.invoke(['--apply','--rename','sppr-research'])
        commit=next(p for m,u,p in FakeClient.calls if u.endswith('/git/commits'))
        self.assertEqual(commit['author'],publish.AUTHOR);self.assertEqual(commit['committer'],publish.AUTHOR)
        self.assertEqual(commit['parents'],[publish.EXPECTED_HEAD])
        self.assertTrue(any(m=='PATCH' and u.endswith('/git/refs/heads/main') and p['force'] is False for m,u,p in FakeClient.calls))
    def test_identity_mismatch_never_moves_main(self):
        FakeClient.wrong_author=True
        with self.assertRaises(RuntimeError):self.invoke(['--apply'])
        self.assertFalse(any(m=='PATCH' for m,_,_ in FakeClient.calls))
    def test_changed_remote_never_writes(self):
        FakeClient.changed_head=True
        with self.assertRaises(RuntimeError):self.invoke(['--apply'])
        self.assertTrue(all(m=='GET' for m,_,_ in FakeClient.calls))
    def test_manifest_and_secret_guards(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);f=root/'a.txt';f.write_text('safe')
            manifest=root/'release-files.json';manifest.write_text(json.dumps({'files':{'a.txt':hashlib.sha256(f.read_bytes()).hexdigest()}}))
            with patch.object(publish,'ROOT',root):
                self.assertEqual(len(publish.source_files()),2)
                f.write_text('changed')
                with self.assertRaises(RuntimeError):publish.source_files()
                f.write_text('gh'+'p_'+'A'*36)
                manifest.write_text(json.dumps({'files':{'a.txt':hashlib.sha256(f.read_bytes()).hexdigest()}}))
                with self.assertRaises(RuntimeError):publish.source_files()
                manifest.write_text(json.dumps({'files':{'../escape':'x'}}))
                with self.assertRaises(RuntimeError):publish.source_files()

if __name__=='__main__':unittest.main()
