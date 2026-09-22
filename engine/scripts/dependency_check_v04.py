#!/usr/bin/env python3
"""Verify inference/replay do not depend on environment or training packages."""
from __future__ import annotations
import argparse, json, subprocess
from pathlib import Path

def main() -> int:
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--out',required=True,type=Path);a=p.parse_args()
    root=Path(__file__).resolve().parents[1]; prefix='dissertation.local/sppr-reconstruction/'
    targets=['./forecastpipe','./forecasttrace']
    proc=subprocess.run(['go','list','-deps',*targets],cwd=root,capture_output=True,text=True,timeout=90,check=True)
    dependencies=[v for v in proc.stdout.splitlines() if v.startswith(prefix)]
    forbidden=[prefix+x for x in ['simenv','training','sensor','internal/testfixture']]
    violations=sorted(set(dependencies)&set(forbidden))
    result={'targets':targets,'project_dependencies':dependencies,'forbidden_dependencies':forbidden,
            'violations':violations,'passed':not violations,
            'scope':'dependency/interface check, not a proof against every possible information leak; plant is a pure shared balance function'}
    a.out.parent.mkdir(parents=True,exist_ok=True)
    a.out.write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps(result));return 1 if violations else 0
if __name__=='__main__': raise SystemExit(main())
