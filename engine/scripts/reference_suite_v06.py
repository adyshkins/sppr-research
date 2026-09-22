#!/usr/bin/env python3
"""Evaluate the 24 checkpoints specified BEFORE the DEV-F06 run. No replacement."""
import argparse,concurrent.futures,json,pathlib,subprocess,time
ROOT=pathlib.Path(__file__).resolve().parents[1]
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--suite',default='results/factorial_v06');p.add_argument('--workers',type=int,default=2);p.add_argument('--verify',action='store_true');a=p.parse_args();suite=ROOT/a.suite
 if not (suite/'SERIES_COMPLETED.json').exists():raise ValueError('Primary series must complete first')
 jobs=json.loads((suite/'plan.json').read_text());binary=ROOT/'checks_v0_6/factorcheck';tasks=[]
 for i,j in enumerate(jobs):
  if j['profile'] not in ('S','C','D','DS') or j['repeat'] not in (0,1) or j['policy']!='R00':continue
  profile_index={'S':1,'C':2,'D':3,'DS':4}[j['profile']]
  for day in (19,22,25):tasks.append((i,j,day,410001+1000*profile_index+100*j['repeat']+day))
 assert len(tasks)==24
 dest=suite/'references';dest.mkdir(exist_ok=True)
 def one(t):
  i,j,day,seed=t;name=f"{j['profile']}_r{j['repeat']:02d}_d{day:02d}";out=dest/name
  start=time.monotonic()
  if a.verify:
   if (out/'SKIPPED.json').exists():return {'checkpoint':name,'status':'skipped_as_prespecified'}
   cmd=[str(binary),'-verify-reference',str(out)]
  else:cmd=[str(binary),'-reference',str(suite/'episodes'/f'episode_{i:03d}'),'-ref-day',str(day),'-ref-seed',str(seed),'-ref-paths','2048','-out',str(out)]
  log=dest/f'{name}_{"verify" if a.verify else "run"}.txt'
  with log.open('w') as f:r=subprocess.run(cmd,cwd=ROOT,stdout=f,stderr=subprocess.STDOUT,timeout=180)
  status='failed' if r.returncode else ('skipped_not_ready' if (out/'SKIPPED.json').exists() else 'passed')
  rec={'checkpoint':name,'source_job':i,'profile':j['profile'],'repeat':j['repeat'],'day':day,'seed':seed,'paths':2048,'exit_code':r.returncode,'status':status,'seconds':time.monotonic()-start};print(json.dumps(rec),flush=True)
  if r.returncode:raise RuntimeError(f'Checkpoint failed {name}')
  return rec
 with concurrent.futures.ThreadPoolExecutor(max_workers=a.workers) as pool:records=list(pool.map(one,tasks))
 (dest/('verification.json' if a.verify else 'run_summary.json')).write_text(json.dumps(records,indent=2)+'\n')
if __name__=='__main__':main()
