#!/usr/bin/env python3
"""Verify a downloaded reconstruction snapshot; executes no Go code."""
from __future__ import annotations
import hashlib
import json
from pathlib import Path, PurePosixPath
import sys

def main() -> int:
    root = Path(__file__).resolve().parents[1]
    manifest = json.loads((root / 'MANIFEST.json').read_text(encoding='utf-8'))
    failures: list[str] = []
    for rel, expected in manifest['files'].items():
        posix = PurePosixPath(rel)
        if posix.is_absolute() or '..' in posix.parts:
            raise ValueError('Unsafe manifest path')
        p = root.joinpath(*posix.parts)
        if not p.is_file():
            failures.append(f'Missing: {rel}')
            continue
        data = p.read_bytes()
        if len(data) != expected['bytes'] or hashlib.sha256(data).hexdigest() != expected['sha256']:
            failures.append(f'Changed: {rel}')
    if failures:
        print('\n'.join(failures), file=sys.stderr)
        return 1
    print(f"Verified {len(manifest['files'])} snapshot files. Hashes check identity, not scientific validity.")
    return 0

if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (OSError, ValueError, KeyError) as exc:
        print(f'verify_manifest: {exc}', file=sys.stderr)
        raise SystemExit(2)
