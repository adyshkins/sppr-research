#!/usr/bin/env python3
"""Verify all distributed file hashes. Run from any directory."""
import hashlib,json,sys
from pathlib import Path
root=Path(__file__).resolve().parent
manifest=json.loads((root/'MANIFEST_SHA256.json').read_text())
bad=[]
for name,expected in manifest.items():
    path=root/name
    if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest()!=expected: bad.append(name)
print(f'Checked {len(manifest)} files; mismatches: {len(bad)}')
for name in bad: print(name)
sys.exit(bool(bad))
