#!/usr/bin/env python3
"""Verify the ACTIVE release manifest, not historical manifests from earlier versions."""
import hashlib
import json
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
manifest = json.loads((root / "MANIFEST.json").read_text(encoding="utf-8"))
failed = []
for relative, entry in manifest["files"].items():
    path = (root / relative).resolve()
    if not path.is_relative_to(root.resolve()) or not path.is_file():
        failed.append((relative, "missing or invalid path"))
        continue
    if path.stat().st_size != entry["bytes"]:
        failed.append((relative, "size"))
        continue
    if hashlib.sha256(path.read_bytes()).hexdigest() != entry["sha256"]:
        failed.append((relative, "sha256"))
if failed:
    print(json.dumps(failed, ensure_ascii=False, indent=2))
    sys.exit(1)
print(f"Verified {len(manifest['files'])} files in {manifest['edition']}; historical raw data not listed are not implied.")
