#!/usr/bin/env python3
"""Fixed 24 checkpoints from the predeclared H arm; skips are never replaced."""
import json,pathlib,subprocess,concurrent.futures,os,time
ROOT=pathlib.Path(__file__).resolve().parents[1];SUITE=ROOT/'results/recovery_v08'
def main():
 jobs=json.loads((SUITE/'plan.json').read_text());root=SUITE/'references';root.mkdir(exist_ok=True);items=[]
 for i,c in enumerate(jobs):
  if c['arm']!='H' or c['profile']=='normal' or c['repeat']>1:continue
  p={'S':1,'C':2,'D':3,'DS':4}[c['profile']]
  assert (SUITE/'episodes'/f'ep{i:03d}'/'COMPLETED.json').is_file()
  for day in [19,20,22]:items.append({'job':i,'profile':c['profile'],'repeat':c['repeat'],'day':day,'seed':1010001+1000*p+100*c['repeat']+day,'paths':2048})
 assert len(items)==24
 (root/'plan.json').write_text(json.dumps(items,indent=2)+'\n')
 def one(x):
  folder=root/f"ep{x['job']:03d}_day{x['day']:02d}";cmd=['/mnt/data/recoverycheck','-reference',str(SUITE/'episodes'/f"ep{x['job']:03d}"),'-day',str(x['day']),'-seed',str(x['seed']),'-paths','2048','-out',str(folder)];t=time.monotonic();r=subprocess.run(cmd,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,env={**os.environ,'GOMAXPROCS':'1'},timeout=180);(root/f"ep{x['job']:03d}_day{x['day']:02d}.log").write_text(r.stdout);return {**x,'exit':r.returncode,'seconds':time.monotonic()-t,'skipped':(folder/'SKIPPED.json').is_file()}
 with concurrent.futures.ThreadPoolExecutor(max_workers=2) as ex:
  results=list(ex.map(one,items))
 (root/'commands.json').write_text(json.dumps(results,indent=2)+'\n');assert all(x['exit']==0 for x in results),results
 (root/'SERIES_COMPLETED.json').write_text(json.dumps({'requested':24,'skipped':sum(x['skipped'] for x in results),'evaluated':sum(not x['skipped'] for x in results)},indent=2)+'\n');print(results)
if __name__=='__main__':main()
