#!/usr/bin/env python3
"""Verify unchanged upstream engine files. No network or third-party packages."""
from __future__ import annotations
import hashlib
import json
from pathlib import Path
ROOT = Path(__file__).resolve().parents[1]

def verify() -> int:
    spec = json.loads((ROOT / 'docs/upstream-files.json').read_text(encoding='utf-8'))
    failures: list[str] = []
    for name, expected in spec['files'].items():
        path = ROOT / 'engine' / name
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            failures.append(name)
    if failures:
        raise RuntimeError('Upstream files changed or missing: ' + ', '.join(failures))
    print(f"Verified {len(spec['files'])} preserved upstream files; this is not an F10/E11 reproduction.")
    return len(spec['files'])

if __name__ == '__main__':
    try:
        verify()
    except (OSError, ValueError, KeyError, RuntimeError) as e:
        raise SystemExit(str(e))
