#!/usr/bin/env python3
"""Run every predeclared job, logging failures rather than selecting outcomes."""
import argparse, concurrent.futures, hashlib, json, os, pathlib, subprocess, time
from datetime import datetime, timezone

def main():
 p=argparse.ArgumentParser();p.add_argument('--plan',required=True);p.add_argument('--dir',required=True);p.add_argument('--binary',default='/mnt/data/recoverycheck');p.add_argument('--workers',type=int,default=4);p.add_argument('--replay',action='store_true');a=p.parse_args()
 root=pathlib.Path(a.dir);root.mkdir(parents=True,exist_ok=True);plan=pathlib.Path(a.plan);jobs=json.loads(plan.read_text());logs=root/'logs';logs.mkdir(exist_ok=True)
 def one(i):
  dest=root/'episodes'/f'ep{i:03d}'
  cmd=[a.binary,'-replay',str(dest)] if a.replay else [a.binary,'-plan',str(plan),'-job',str(i),'-out',str(dest)]
  t=time.monotonic();r=subprocess.run(cmd,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,env={**os.environ,'GOMAXPROCS':'2'},timeout=600)
  tag='replay' if a.replay else 'run';lp=logs/f'{tag}_{i:03d}.txt';lp.write_text(r.stdout)
  return {'job':i,'command':cmd,'exit':r.returncode,'seconds':time.monotonic()-t,'log':str(lp),'sha256':hashlib.sha256(lp.read_bytes()).hexdigest()}
 all_results=[]
 with concurrent.futures.ThreadPoolExecutor(max_workers=a.workers) as ex:
  for f in concurrent.futures.as_completed([ex.submit(one,i) for i in range(len(jobs))]):
   r=f.result();all_results.append(r)
   with (root/('replay_commands.jsonl' if a.replay else 'run_commands.jsonl')).open('a') as out:out.write(json.dumps(r)+'\n')
   print(f"{len(all_results)}/{len(jobs)} job {r['job']} exit={r['exit']} {r['seconds']:.2f}s",flush=True)
 if any(r['exit'] for r in all_results):raise SystemExit('Incomplete series: see logs; no success marker')
 (root/('REPLAY_COMPLETED.json' if a.replay else 'SERIES_COMPLETED.json')).write_text(json.dumps({'jobs':len(jobs),'utc':datetime.now(timezone.utc).isoformat(),'plan_sha256':hashlib.sha256(plan.read_bytes()).hexdigest(),'seconds_sum':sum(x['seconds'] for x in all_results)},indent=2)+'\n')
if __name__=='__main__':main()
