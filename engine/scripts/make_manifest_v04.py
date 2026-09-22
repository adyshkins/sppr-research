#!/usr/bin/env python3
"""Create the current release inventory (release maintainer operation)."""
import hashlib, json
from pathlib import Path
root=Path(__file__).resolve().parents[1]
files={}
for p in sorted(root.rglob('*')):
    if not p.is_file() or p.name=='MANIFEST.json' or any(v in p.parts for v in ['verification_reruns','__pycache__','.git']):
        continue
    rel=p.relative_to(root).as_posix();b=p.read_bytes()
    files[rel]={'bytes':len(b),'sha256':hashlib.sha256(b).hexdigest()}
obj={'version':'0.4.0','date':'2026-09-15','scope':'new synthetic diagnostic training and engineering forecasting; no closed-loop efficacy claims',
     'hash_semantics':'file identity only, not independent authentication or scientific validity','files':files}
(root/'MANIFEST.json').write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(f'Created inventory for {len(files)} files')
