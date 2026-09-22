#!/usr/bin/env python3
import subprocess, pathlib, time, json, hashlib, os
ROOT=pathlib.Path(__file__).resolve().parents[1];logs=ROOT/'checks_v0_8/final';logs.mkdir(exist_ok=True);rows=[]
commands=[('tests_json',['go','test','-json','./...','-count=1']),('repeat10',['go','test','./...','-count=10','-shuffle=81']),('vet',['go','vet','./...']),('build',['go','build','./...']),('race',['go','test','-race','./...','-count=1'])]
for tag,cmd in commands:
 start=time.monotonic();r=subprocess.run(cmd,cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,env={**os.environ,'GOMAXPROCS':'2'},timeout=240);p=logs/(tag+'.txt');p.write_bytes(r.stdout);row={'tag':tag,'command':cmd,'exit':r.returncode,'seconds':time.monotonic()-start,'log':p.name,'sha256':hashlib.sha256(r.stdout).hexdigest()};rows.append(row);print(row,flush=True)
(logs/'verification_commands.json').write_text(json.dumps(rows,indent=2)+'\n')
assert all(x['exit']==0 for x in rows)
