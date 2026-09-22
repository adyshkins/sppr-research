"""Validate immutable numerical sources plus explicit reader-only repair ledger."""
import hashlib,json,pathlib
def verify_frozen(root,suite):
 root=pathlib.Path(root);suite=pathlib.Path(suite);f=json.loads((suite/'freeze.json').read_text(encoding='utf8'));expected=dict(f['source_sha256']);patch=suite/'freeze_patches.json'
 if patch.exists():
  for event in json.loads(patch.read_text(encoding='utf8')):
   for path,p in event['files'].items():
    if path!='factorial/reference.go':raise ValueError('Only explicitly declared reference-reader repair is allowed')
    if expected.get(path)!=p['before_sha256']:raise ValueError('Patch does not follow frozen original')
    expected[path]=p['after_sha256']
   reg=event.get('added_regression_test')
   if reg:expected[reg['path']]=reg['sha256']
 for path,h in expected.items():
  if hashlib.sha256((root/path).read_bytes()).hexdigest()!=h:raise ValueError(f'Frozen file changed: {path}')
 return len(expected)
