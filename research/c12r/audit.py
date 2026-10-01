#!/usr/bin/env python3
"""Independent arithmetic audit of saved C12 snapshots and deterministic replay.
No call into the Go selector or simulator is made; risk uses the variational
formula, not the Go sorted fractional-tail formula. This is a second implementation,
not an independent organization's review and not a proof of model validity.
"""
from __future__ import annotations
import csv, hashlib, json, math, platform, argparse
from pathlib import Path

ROOT = Path(__file__).resolve().parent
TOL = 2e-12

def risk(z: list[float], lam: float, denominator: int) -> dict[str,float]:
    n = len(z)
    mean = sum(z) / n
    # Evaluate the CVaR variational formula at every distinct observed atom.
    r = min(q + denominator * sum(max(v-q, 0.0) for v in z)/n for q in set(z))
    return {'Mean': mean, 'Risk': r, 'Objective': (1-lam)*mean + lam*r}

def rollout(state: dict, path: list, action: dict) -> float:
    raw = state['Raw']; finished = state['Finished'][:]
    backlog = state['Backlog'][:]; plan = state['Plan'][:]
    total = action['Cost']
    for h,w in enumerate(path):
        if h == 0:
            over,extra,factors = action['Overtime'],action['ExtraRaw'],action['Factors']
            if action['Permanent']: plan = action['Replan'][:]
        else:
            over,extra,factors = 0.0,0.0,[1.0,1.0]
        p = [plan[i]*factors[i] for i in range(2)]
        cap = w['Capacity']*(1+over); avail_raw = raw+w['RawArrival']+extra
        req_c = p[0]+1.6*p[1]; req_r = p[0]+2*p[1]
        factor = min(1.0,cap/req_c if req_c>0 else 1.0,avail_raw/req_r if req_r>0 else 1.0)
        output = [v*factor for v in p]
        need = [w['Demand'][i]+backlog[i] for i in range(2)]
        stock = [finished[i]+output[i] for i in range(2)]
        shipped = [min(need[i],stock[i]) for i in range(2)]
        backlog = [need[i]-shipped[i] for i in range(2)]
        finished = [stock[i]-shipped[i] for i in range(2)]
        raw = max(0.0, avail_raw-(output[0]+2*output[1]))
        k = [sum(shipped)/sum(need) if sum(need)>0 else 1.0,
             max(0.0,1-sum(abs(output[i]-p[i]) for i in range(2))/sum(p)) if sum(p)>0 else 1.0,
             (output[0]+1.6*output[1])/cap if cap>0 else 0.0,raw/90]
        lo=[.95,.95,.7,3]; hi=[1,1,.9,7]; wt=[.35,.25,.15,.25]; upper=[0,0,.5,.25]
        total += sum(wt[i]*(max(lo[i]-k[i],0)+upper[i]*max(k[i]-hi[i],0))/lo[i] for i in range(4))
    return total

def read_csv(path: Path) -> list[dict[str,str]]:
    with path.open(newline='') as f: return list(csv.DictReader(f))

def main() -> None:
    ap=argparse.ArgumentParser(); ap.add_argument('--input',default='results/c12_final'); ap.add_argument('--replay',default='results/replay_final'); args=ap.parse_args()
    src=ROOT/args.input; replay_dir=ROOT/args.replay
    base_seed=json.loads((src/'summary.json').read_text())['options']['first_seed']
    out={'status':'PASS', 'snapshot_count':0, 'full_matrix_rollouts':0,
         'queried_losses_checked':0,'bound_values_checked':0,'pruning_records_checked':0,
         'snapshot_selector_mismatches':0,'max_loss_abs_difference':0.0,'max_score_abs_difference':0.0}
    for file in sorted((src/'snapshots').glob('*.json')):
        s=json.loads(file.read_text()); cfg=s['config']; n=len(s['paths']); lam=cfg['lambda']; den=cfg['tail_denominator']
        matrix={a['ID']:[rollout(s['state'],p,a) for p in s['paths']] for a in s['actions']}
        score={a:risk(z,lam,den) for a,z in matrix.items()}
        candidates={a['id']:a for a in s['candidates']}
        key=lambda aid:(score[aid]['Objective'],candidates[aid]['cost'],candidates[aid]['distance'],aid)
        eligible=[a for a,c in candidates.items() if c['hard'] and (not cfg['filter'] or score[a]['Risk']<=cfg['limit'])]
        chosen=min(eligible,key=key) if eligible else -1
        trace=s['certificate']; assert chosen==trace['choice'], (file.name,'choice')
        assert trace['reason']==('RECOMMENDATION' if chosen>=0 else 'RISK_EMPTY')
        if chosen>=0:
            for name in ('Mean','Risk','Objective'):
                diff=abs(score[chosen][name]-trace['score'][name]); out['max_score_abs_difference']=max(out['max_score_abs_difference'],diff)
                assert diff<TOL,(file.name,name,diff)
        incumbent=-1; queries=0
        for rec in trace['records']:
            aid=rec['candidate']; assert rec['incumbent']==incumbent
            low=[candidates[aid]['cost']]*n; seen=set()
            for v in rec.get('samples',[]):
                j=v['scenario']; assert j not in seen; seen.add(j); queries+=1
                diff=abs(matrix[aid][j]-v['loss']); out['max_loss_abs_difference']=max(out['max_loss_abs_difference'],diff)
                assert diff<TOL,(file.name,aid,j,diff)
                low[j]=v['loss']; out['queried_losses_checked']+=1
            rb=risk(low,lam,den)
            for name in ('Mean','Risk','Objective'):
                diff=abs(rb[name]-rec['bound'][name]); out['max_score_abs_difference']=max(out['max_score_abs_difference'],diff)
                assert diff<TOL,(file.name,'bound',name,diff)
                out['bound_values_checked']+=1
            reason=rec['reason']
            if reason=='RISK_BOUND': assert cfg['filter'] and rb['Risk']>cfg['limit']; out['pruning_records_checked']+=1
            elif reason=='OBJECTIVE_BOUND': assert incumbent>=0 and rb['Objective']>score[incumbent]['Objective']-TOL; out['pruning_records_checked']+=1
            elif reason=='RISK_EXACT': assert len(seen)==n and rb['Risk']>cfg['limit']-TOL
            elif reason=='COMPLETE':
                assert len(seen)==n
                assert not cfg['filter'] or rb['Risk']<=cfg['limit']+TOL
                if incumbent<0 or key(aid)<key(incumbent): incumbent=aid
            else: raise AssertionError((file.name,'unhandled reason',reason))
        assert incumbent==chosen and queries==trace['queries']
        out['snapshot_count']+=1; out['full_matrix_rollouts']+=n*len(candidates)
    # Exact string comparison of every deterministic CSV field, preserving floats.
    for name in ('reviews','episodes'):
        original=[r for r in read_csv(src/f'{name}.csv') if int(r['seed'])<base_seed+2]
        replay=read_csv(replay_dir/f'{name}.csv')
        assert len(original)==len(replay)
        cols=[k for k in original[0] if not k.endswith('_ns')]
        for a,b in zip(original,replay):
            assert all(a[k]==b[k] for k in cols),(name,a['seed'],[(k,a[k],b[k]) for k in cols if a[k]!=b[k]])
        out[f'replayed_{name}']=len(replay)
        out[f'replayed_{name}_deterministic_fields']=len(replay)*len(cols)
    summary=json.loads((src/'summary.json').read_text())
    assert summary['mismatches']==0 and summary['reviews']==83520 and summary['episodes']==2880
    out['production_comparisons_checked']=summary['reviews']*3
    out['notes']=['Replay excludes machine-dependent runtime fields.','Independent snapshots are engineering verification, not independent statistical replicates.','All bounds are conditional on the stated model and retained scenario weights.']
    dest=ROOT/'results/audit/independent_audit_final.json'; dest.write_text(json.dumps(out,ensure_ascii=False,indent=2))
    print(json.dumps(out,ensure_ascii=False,indent=2))
if __name__=='__main__': main()
