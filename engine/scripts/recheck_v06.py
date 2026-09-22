#!/usr/bin/env python3
"""Verify this reconstruction without modifying the frozen experimental code.
All tests are engineering/statistical-arithmetic checks, not field validation.
"""
from __future__ import annotations
import concurrent.futures,hashlib,json,os,pathlib,subprocess,tempfile,time
from frozen_v06 import verify_frozen
ROOT=pathlib.Path(__file__).resolve().parents[1]
def sha(p):return hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
def main():
 check=ROOT/'checks_v0_6';check.mkdir(exist_ok=True);binary=check/'factorcheck';binary=binary.with_suffix('.exe') if os.name=='nt' else binary
 commands=[]
 def command(name,cmd,timeout=300):
  start=time.monotonic();path=check/(name+'.txt')
  with path.open('w',encoding='utf8') as f:p=subprocess.run(cmd,cwd=ROOT,stdout=f,stderr=subprocess.STDOUT,timeout=timeout)
  rec={'name':name,'command':cmd,'exit_code':p.returncode,'seconds':time.monotonic()-start,'log_sha256':sha(path)};commands.append(rec);print(json.dumps(rec),flush=True)
  with (check/'verification_commands.jsonl').open('a',encoding='utf8') as f:f.write(json.dumps(rec)+'\n')
  if p.returncode:raise RuntimeError(f'Check failed: {name}')
 command('go_test_final',['go','test','-json','./...','-count=1'])
 command('go_test_repeat',['go','test','./...','-count=10','-shuffle=61'])
 command('go_vet',['go','vet','./...'])
 command('go_build',['go','build','./...'])
 command('go_race',['go','test','-race','./...','-count=1'])
 subprocess.run(['go','build','-o',str(binary),'./cmd/factorcheck'],cwd=ROOT,check=True)
 suite=ROOT/'results/factorial_v06';jobs=json.loads((suite/'plan.json').read_text(encoding='utf8'))
 if not (suite/'SERIES_COMPLETED.json').exists():raise RuntimeError('Series not complete')
 freeze=json.loads((suite/'freeze.json').read_text(encoding='utf8'))
 verify_frozen(ROOT,suite)
 def replay(i):
  dest=suite/'episodes'/f'episode_{i:03d}';r=subprocess.run([str(binary),'-replay',str(dest)],cwd=ROOT,capture_output=True,text=True,timeout=180)
  (check/f'replay_{i:03d}.txt').write_text(r.stdout+r.stderr,encoding='utf8')
  if r.returncode:raise RuntimeError(f'Replay failed {i}: {r.stderr}')
  return {'job':i,'exit_code':0,'days':jobs[i]['config']['environment']['days']}
 with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:rs=list(pool.map(replay,range(len(jobs))))
 (check/'all_replays.json').write_text(json.dumps(rs,indent=2)+'\n');print('Replayed all',len(rs),'episodes',flush=True)
 # Predefined first repeat of each profile, all four modes, not outcome selection.
 indices=[i for i,j in enumerate(jobs) if j['repeat']==0];regen=[]
 with tempfile.TemporaryDirectory(prefix='sppr-v06-regenerate-') as tmp:
  for i in indices:
   out=pathlib.Path(tmp)/f'episode_{i:03d}';r=subprocess.run([str(binary),'-plan',str(suite/'plan.json'),'-job',str(i),'-out',str(out)],cwd=ROOT,capture_output=True,text=True,timeout=180)
   if r.returncode:raise RuntimeError(r.stderr)
   old=suite/'episodes'/f'episode_{i:03d}'
   files=['experiment_config.json','physical_evaluation.jsonl.gz','compact_controller.jsonl.gz','summary.json','COMPLETED.json']
   for f in files:
    if sha(out/f)!=sha(old/f):raise RuntimeError(f'Regeneration mismatch {i}/{f}')
   regen.append({'job':i,'files_identical':files});print('Regenerated',i,flush=True)
 (check/'regenerated_subset.json').write_text(json.dumps(regen,indent=2)+'\n')
 command('reference_regeneration',['python','scripts/reference_suite_v06.py','--verify'],timeout=300)
 totals={'notice':'Engineering verification, not external validity','commands_passed':len(commands),'replay_episodes':len(rs),'replay_records':sum(x['days'] for x in rs),'regenerated_episodes':len(regen),'frozen_files_verified':len(freeze['source_sha256'])}
 (check/'recheck_summary.json').write_text(json.dumps(totals,indent=2)+'\n');print(json.dumps(totals),flush=True)
if __name__=='__main__':main()
