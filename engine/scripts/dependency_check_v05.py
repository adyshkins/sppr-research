#!/usr/bin/env python3
"""Check production import paths, not a formal proof of arbitrary information flow."""
import argparse,json,subprocess
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--out',default='checks/dependency_check_v05.json');a=p.parse_args()
root=Path(__file__).resolve().parents[1];prefix='dissertation.local/sppr-reconstruction/'
forbidden={prefix+x for x in ['simenv','sensor','closedrun','training','internal/testfixture']}
checks=[]
for package in ['./control','./execution','./controltrace']:
 r=subprocess.run(['go','list','-deps',package],cwd=root,text=True,capture_output=True,check=True)
 deps=set(r.stdout.splitlines());bad=sorted(deps&forbidden)
 checks.append({'package':package,'forbidden_dependencies':bad,'local_dependencies':sorted(x for x in deps if x.startswith(prefix))})
 assert not bad,(package,bad)
out={'passed':True,'scope':'Production import dependency check only; plant.Step remains shared pure balance model','checks':checks}
dest=Path(a.out);dest=dest if dest.is_absolute() else root/dest;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out,indent=2))
