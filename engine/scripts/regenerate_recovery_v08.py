#!/usr/bin/env python3
import pathlib,json,subprocess,concurrent.futures,hashlib,time,os
ROOT=pathlib.Path(__file__).resolve().parents[1];SUITE=ROOT/'results/recovery_v08';OUT=ROOT/'checks_v0_8/regenerated';OUT.mkdir(exist_ok=True)
def main():
 jobs=json.loads((SUITE/'plan.json').read_text());ids=[i for i,c in enumerate(jobs) if c['repeat']==0];assert len(ids)==20
 def one(i):
  out=OUT/f'ep{i:03d}';start=time.monotonic();r=subprocess.run(['/mnt/data/recoverycheck_final','-plan',str(SUITE/'plan.json'),'-job',str(i),'-out',str(out)],capture_output=True,text=True,env={**os.environ,'GOMAXPROCS':'1'},timeout=180);(OUT/f'{i:03d}.log').write_text(r.stdout+r.stderr)
  assert r.returncode==0,(i,r.stderr)
  files={}
  for name in ['experiment_config.json','controller.jsonl.gz','physical.jsonl.gz','summary.json','COMPLETED.json']:
   a=hashlib.sha256((out/name).read_bytes()).hexdigest();b=hashlib.sha256((SUITE/'episodes'/f'ep{i:03d}'/name).read_bytes()).hexdigest();assert a==b,(i,name);files[name]=a
  return {'job':i,'seconds':time.monotonic()-start,'all_files_bitwise_equal':True,'files':files}
 with concurrent.futures.ThreadPoolExecutor(max_workers=2) as ex:res=list(ex.map(one,ids))
 (ROOT/'checks_v0_8/regeneration_verified.json').write_text(json.dumps(res,indent=2)+'\n');print('20 episodes, 100 files identical')
 # Repeat all 24 predefined reference requests after the non-numerical literal fix.
 refs=json.loads((SUITE/'references/plan.json').read_text());rs=[]
 for x in refs:
  name=f"ep{x['job']:03d}_day{x['day']:02d}";dest=OUT/name
  cmd=['/mnt/data/recoverycheck_final','-reference',str(SUITE/'episodes'/f"ep{x['job']:03d}"),'-day',str(x['day']),'-seed',str(x['seed']),'-paths','2048','-out',str(dest)]
  p=subprocess.run(cmd,capture_output=True,env={**os.environ,'GOMAXPROCS':'1'},timeout=60);assert p.returncode==0,p.stderr
  old=SUITE/'references'/name;fs=[]
  for q in old.iterdir():
   if q.is_file():assert q.read_bytes()==(dest/q.name).read_bytes(),name+q.name;fs.append(q.name)
  rs.append({'checkpoint':name,'identical_files':fs})
 (ROOT/'checks_v0_8/reference_regenerated.json').write_text(json.dumps(rs,indent=2)+'\n');print('24 checkpoint outcomes identical after named-field correction')
if __name__=='__main__':main()
