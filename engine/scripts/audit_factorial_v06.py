#!/usr/bin/env python3
"""Independent Python arithmetic and pairing checks. No Go execution/import.
Checks all executed physical days and A6 predictions/choices from compact traces.
"""
from __future__ import annotations
import argparse,gzip,json,pathlib
from independent_reference_v05 import Audit,physical,risk,checks_for
ROOT=pathlib.Path(__file__).resolve().parents[1]
def read(p):return json.loads(pathlib.Path(p).read_text(encoding='utf8'))
def audit_controller(a,folder,cfg):
 pending=None;previous_budget=cfg['initial_command_permissions']['remaining_normalized_action_budget'];count=0
 with gzip.open(folder/'compact_controller.jsonl.gz','rt',encoding='utf8') as f:
  for day,line in enumerate(f):
   r=json.loads(line);count+=1;inp=r['input'];s=r['selection'];ex=r['execution'];act=ex['authorized_action'];lim=inp['known_permissions'];choice=cfg['controller']['choice'];policy=cfg['controller']['risk_policy']
   a.eq(day,r['day'],'index');a.eq(pending,ex['requested_assignment'],'pending not fabricated')
   if pending is None:a.eq(act['id'],'u0','continue base plan');a.eq(ex['authorized_assignment_id'],None,'no fake execution')
   else:a.eq(pending['execute_day'],day,'action time');a.eq(pending['basis']['closed_day'],day-1,'information cutoff');a.eq(pending['action'],act,'saved action executed')
   a.eq(previous_budget-act['cost'],lim['remaining_normalized_action_budget'],'budget once');previous_budget=lim['remaining_normalized_action_budget']
   d=s['decision']
   if d is not None:
    a.choices+=1;ws=r['ordered_scenario_weights'];actmap={x['id']:x for x in r['actions']};values={};adm=[];rids=[]
    for ev in d['evaluations']:
     a.alternatives+=1;id=ev['id'];action=actmap[id];cs=checks_for(action,lim)
     for g in s['hard_checks'][id]:a.eq(cs[g['id']],g['residual'],'permission residual')
     ok=all(v<=0 for v in cs.values());a.eq(ok,ev['hard_admissible'],'hard flag')
     if not ok:continue
     adm.append(id);zs=ev['scenario_losses'];a.eq(len(zs),len(ws),'scenario sizes');a.eq(sum(ws),1.,'probability mass')
     mean,var,cv=risk(zs,ws,choice['alpha'])
     for name,v in [('expected',mean),('var',var),('cvar',cv)]:a.eq(v,ev['risk'][name],'risk arithmetic')
     rok=cv<=choice['risk_limit'];a.eq(rok,ev['risk_admissible'],'risk limit');
     if rok:rids.append(id)
     base=inp['forecast_input']['known_plan']['base_plan'];pl=base if action['permanent_plan'] is None else action['permanent_plan'];dist=sum(abs(pl[k]*action['temporary_factors'][k]-base[k]) for k in (0,1))/max(1,sum(base));a.eq(dist,ev['plan_distance'],'distance')
     values[id]=(mean,cv,action['cost'],dist,rok,ev)
    a.eq(sorted(adm),d['admissible_ids'],'admissible set');a.eq(sorted(rids),d['risk_ids'],'risk set')
    for shadow in r['same_information_choices']:
     p=shadow['policy'];w=choice['risk_weight'] if p['risk_in_objective'] else 0;elig=[]
     for id,(mu,cv,cost,dist,rok,ev) in values.items():
      if rok or not p['risk_constraint']:
       # Preserve stored float tie-breaking after independently validating each
       # risk value. A separate tolerance check verifies the independent minimum.
       j=(1-w)*ev['risk']['expected']+w*ev['risk']['cvar'];ji=(1-w)*mu+w*cv;elig.append((j,cost,dist,id,ji))
       if p['id']==policy['id']:a.eq(j,ev['objective'],'objective')
     best=min(elig) if elig else None;a.eq(sorted(x[3] for x in elig),shadow['eligible_ids'],'shadow eligible');a.eq(best[3] if best else None,shadow['recommendation_id'],'shadow argmin')
     if best:a.eq(best[0],min(x[4] for x in elig),'independent objective minimum within rounding')
     reason='RECOMMENDATION' if best else ('ADM_EMPTY' if not adm else 'RISK_EMPTY');a.eq(reason,shadow['reason'],'shadow reason')
     if p['id']==policy['id']:a.eq(shadow['recommendation_id'],d['recommendation_id'],'actual matches shadow')
   pending=s['assignment']
 a.eq(count,cfg['environment']['days'],'episode complete')
 summary=read(folder/'summary.json');a.eq(pending,summary['controller_summary']['terminal_unexecuted_assignment'],'terminal pending')

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--suite',default='results/factorial_v06');p.add_argument('--out',default='checks_v0_6/independent_audit.json');args=p.parse_args();suite=ROOT/args.suite;jobs=read(suite/'plan.json');done=read(suite/'SERIES_COMPLETED.json');a=Audit();tapes={}
 for i,j in enumerate(jobs):
  folder=suite/'episodes'/f'episode_{i:03d}';cfg=read(folder/'experiment_config.json');a.eq(cfg,j['config'],'frozen config');co=read(folder/'COMPLETED.json');tapes.setdefault((j['profile'],j['repeat']),set()).add(co['exogenous_tape_hash']);physical(a,folder,cfg);audit_controller(a,folder,cfg)
 for h in tapes.values():a.eq(len(h),1,'matched common-random-number tape')
 a.eq(a.physical_days,done['days'],'all days');out={'notice':'Independent arithmetic, not industrial/model validation. CVaR from fractional upper-tail mass, not conditional average above VaR. Exact tie-break uses stored risks after independent tolerance validation.','episodes':len(jobs),'paired_quartets':len(tapes),'physical_days':a.physical_days,'choice_cases':a.choices,'alternatives':a.alternatives,'comparisons':a.checks,'maximum_absolute_difference':a.max_abs};(ROOT/args.out).write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out,indent=2))
if __name__=='__main__':main()
