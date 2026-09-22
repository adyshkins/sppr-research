#!/usr/bin/env python3
"""Create and run the predeclared DEV-F06 series. Never silently discard failures.
Python 3.10+ standard library and Go 1.23+; no external Python dependencies.
"""
from __future__ import annotations
import argparse, concurrent.futures, datetime, hashlib, json, os, pathlib, subprocess, sys, threading, time
from frozen_v06 import verify_frozen
ROOT=pathlib.Path(__file__).resolve().parents[1]
def sha(p):return hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--init',action='store_true');p.add_argument('--run',action='store_true');p.add_argument('--resume',action='store_true');p.add_argument('--workers',type=int,default=2);p.add_argument('--suite',default='results/factorial_v06');p.add_argument('--repeats',type=int,default=12);p.add_argument('--days',type=int,default=60);args=p.parse_args()
 if not 1<=args.workers<=8:raise ValueError('workers must be 1..8')
 suite=ROOT/args.suite;plan=suite/'plan.json';freeze=suite/'freeze.json'
 binary=ROOT/'checks_v0_6/factorcheck';binary=binary.with_suffix('.exe') if os.name=='nt' else binary
 binary.parent.mkdir(parents=True,exist_ok=True)
 subprocess.run(['go','build','-o',str(binary),'./cmd/factorcheck'],cwd=ROOT,check=True)
 if args.init:
  if freeze.exists():raise FileExistsError('Frozen plan already exists')
  subprocess.run([str(binary),'-init-model','results/fit_v04/diagnostic_model.json','-plan',str(plan),'-repeats',str(args.repeats),'-days',str(args.days)],cwd=ROOT,check=True)
  files=sorted(list(ROOT.rglob('*.go'))+[ROOT/'go.mod',ROOT/'results/fit_v04/diagnostic_model.json',ROOT/'docs/14_factorial_protocol_v0_6.md',plan])
  f={'schema':'DEV-F06-local-prefreeze-v1','utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'notice':'Saved before this development run, not independent preregistration. No final hold-out or company experiment.','source_sha256':{str(x.relative_to(ROOT)):sha(x) for x in files}}
  freeze.write_text(json.dumps(f,indent=2)+'\n',encoding='utf8')
 if not args.run:return
 f=json.loads(freeze.read_text(encoding='utf8'))
 verify_frozen(ROOT,suite)
 jobs=json.loads(plan.read_text(encoding='utf8'));(suite/'episodes').mkdir(exist_ok=True);(suite/'logs').mkdir(exist_ok=True)
 lock=threading.Lock()
 def one(i):
  job=jobs[i];dest=suite/'episodes'/f'episode_{i:03d}';log=suite/'logs'/f'episode_{i:03d}.txt'
  if dest.exists():
   if not args.resume:raise FileExistsError(dest)
   saved=json.loads((dest/'experiment_config.json').read_text(encoding='utf8'));complete=json.loads((dest/'COMPLETED.json').read_text(encoding='utf8'))
   if saved!=job['config'] or complete['records']!=job['config']['environment']['days']:raise ValueError('Invalid resume entry')
   return i
  start=time.monotonic();cmd=[str(binary),'-plan',str(plan),'-job',str(i),'-out',str(dest)]
  with log.open('w',encoding='utf8') as stream:
   proc=subprocess.run(cmd,cwd=ROOT,stdout=stream,stderr=subprocess.STDOUT,timeout=300)
  record={'job':i,'profile':job['profile'],'repeat':job['repeat'],'policy':job['policy'],'exit_code':proc.returncode,'seconds':time.monotonic()-start,'log_sha256':sha(log)}
  with lock:
   with (suite/'run_log.jsonl').open('a',encoding='utf8') as stream:stream.write(json.dumps(record)+'\n')
   print(json.dumps(record),flush=True)
  if proc.returncode!=0:raise RuntimeError(f'Job {i} failed; see {log}')
  return i
 # On failure, running jobs finish; the series completion marker is not written.
 with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
  for _ in pool.map(one,range(len(jobs))):pass
 done={'schema':'DEV-F06-completed-v1','jobs':len(jobs),'days':sum(j['config']['environment']['days'] for j in jobs),'plan_sha256':sha(plan),'freeze_sha256':sha(freeze),'utc':datetime.datetime.now(datetime.timezone.utc).isoformat()}
 (suite/'SERIES_COMPLETED.json').write_text(json.dumps(done,indent=2)+'\n',encoding='utf8');print(json.dumps(done),flush=True)
if __name__=='__main__':
 try:main()
 except Exception as exc:print(f'factorial runner failed: {exc}',file=sys.stderr);raise
