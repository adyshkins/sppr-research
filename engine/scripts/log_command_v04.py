#!/usr/bin/env python3
"""Run one check, retain its output/exit code; no background execution."""
import datetime, hashlib, json, subprocess, sys, time
from pathlib import Path
root=Path(__file__).resolve().parents[1]
name=sys.argv[1];cmd=sys.argv[2:];out=root/'checks'/name
start=time.monotonic()
with out.open('w',encoding='utf-8') as log:
    try:
        p=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT,timeout=120,check=False)
        code=p.returncode
    except (subprocess.TimeoutExpired,OSError) as e:
        log.write(str(e)+'\n');code=-1
elapsed=time.monotonic()-start
entry={'command':cmd,'exit_code':code,'seconds':elapsed,'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'output':str(out.relative_to(root)),'sha256':hashlib.sha256(out.read_bytes()).hexdigest()}
with (root/'checks'/'commands_v04.jsonl').open('a',encoding='utf-8') as f:f.write(json.dumps(entry)+'\n')
print(json.dumps(entry));sys.exit(0 if code==0 else 1)
