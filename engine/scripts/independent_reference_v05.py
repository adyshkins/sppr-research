#!/usr/bin/env python3
"""Independent Python checks of executed balances, command limits and A6 arithmetic.
No Go import/call. Reads the NEW synthetic records; this is not external validation.
"""
from __future__ import annotations
import argparse, gzip, json, math
from pathlib import Path

class Audit:
    def __init__(self): self.checks=0; self.max_abs=0.0; self.physical_days=0; self.choices=0; self.alternatives=0
    def eq(self,a,b,tag):
        self.checks+=1
        if isinstance(a,(int,float)) and not isinstance(a,bool) and isinstance(b,(int,float)) and not isinstance(b,bool):
            assert math.isfinite(a) and math.isfinite(b),tag
            d=abs(a-b);self.max_abs=max(self.max_abs,d)
            assert d<=1e-10*max(1,abs(a),abs(b)),(tag,a,b)
        else: assert a==b,(tag,a,b)
    def vector(self,a,b,tag):
        self.eq(len(a),len(b),tag)
        for i,(x,y) in enumerate(zip(a,b)):self.eq(x,y,f'{tag}[{i}]')

def risk(z,w,alpha):
    total=sum(w); pairs=sorted((v,p/total) for v,p in zip(z,w) if p>0)
    mean=sum(v*p for v,p in pairs); mass=0; var=pairs[-1][0]
    for v,p in pairs:
        mass+=p
        if mass>=alpha:var=v;break
    tail=1-alpha; left=tail; acc=0
    for v,p in reversed(pairs):
        take=min(p,left);acc+=take*v;left-=take
        if left<=0:break
    return mean,var,acc/tail

def period_loss(k):
    lo=(.95,.95,.7,3);hi=(1,1,.9,7);wt=(.35,.25,.15,.25);up=(0,0,.5,.25)
    return sum(w*(max(l-v,0)+u*max(v-h,0))/l for v,l,h,w,u in zip(k,lo,hi,wt,up))

def checks_for(action,limits):
    p=action['permanent_plan']; out={
    'action_permission':0 if action['id'] in limits['allowed_action_ids'] else 1,
    'overtime_permission':action['overtime']-limits['max_overtime'],
    'extra_raw_permission':action['extra_raw']-limits['max_extra_raw'],
    'single_action_cost':action['cost']-limits['max_action_cost'],
    'remaining_budget':action['cost']-limits['remaining_normalized_action_budget']}
    for j,name in enumerate(('a','b')):
        out[f'replan_{name}_lower']=0 if p is None else limits['replan_min'][j]-p[j]
        out[f'replan_{name}_upper']=0 if p is None else p[j]-limits['replan_max'][j]
    return out

def physical(a:Audit,folder:Path,config:dict):
    ec=config['environment']; pp=ec['parameters']; before=ec['initial_state']; total_loss=0.;total_cost=0.;total_kpi=0.; demand=[0.,0.];shipped=[0.,0.];viol=[0]*4
    with gzip.open(folder/'physical_evaluation.jsonl.gz','rt',encoding='utf8') as f:
        for day,line in enumerate(f):
            r=json.loads(line);a.physical_days+=1;a.eq(day,r['day'],'day');a.eq(before,r['true_state_before_evaluation_only'],'state sequence')
            action=r['execution']['authorized_action'];w=r['tape_evaluation_only']['forcing'];actual=r['physical_result_evaluation_only']
            plan=before['plan'] if action['permanent_plan'] is None else action['permanent_plan']
            ep=[x*y for x,y in zip(plan,action['temporary_factors'])];cap=w['capacity']*(1+action['overtime']);raw=before['raw']+w['raw_arrival']+action['extra_raw']
            reqc=sum(x*y for x,y in zip(ep,pp['capacity_per_unit']));reqr=sum(x*y for x,y in zip(ep,pp['raw_per_unit']))
            scale=min(1,cap/reqc if reqc else 1,raw/reqr if reqr else 1)
            output=[scale*x for x in ep];need=[w['demand'][j]+before['backlog'][j] for j in range(2)];stock=[before['finished'][j]+output[j] for j in range(2)]
            ship=[min(x,y) for x,y in zip(need,stock)];back=[need[j]-ship[j] for j in range(2)];fin=[stock[j]-ship[j] for j in range(2)]
            usedc=sum(x*y for x,y in zip(output,pp['capacity_per_unit']));usedr=sum(x*y for x,y in zip(output,pp['raw_per_unit']));endraw=max(0,raw-usedr)
            k=[sum(ship)/sum(need) if sum(need) else 1,max(0,1-sum(abs(x-y) for x,y in zip(output,ep))/sum(ep)) if sum(ep) else 1,usedc/cap if cap else 0,endraw/pp['raw_days_denominator']]
            for name,v in [('executed_plan',ep),('output',output),('shipped',ship),('kpi',k)]:a.vector(v,actual[name],name)
            for name,v in [('scale',scale),('effective_capacity',cap),('raw_used',usedr),('capacity_used',usedc),('action_cost',action['cost'])]:a.eq(v,actual[name],name)
            a.vector(plan,actual['next']['plan'],'next plan');a.vector(back,actual['next']['backlog'],'backlog');a.vector(fin,actual['next']['finished'],'finished');a.eq(endraw,actual['next']['raw'],'raw')
            loss=period_loss(k);a.eq(loss,r['period_kpi_loss'],'period loss');a.eq(loss+action['cost'],r['physical_loss_plus_action_cost'],'cost once')
            if r['execution']['authorized_assignment_id'] is not None:a.eq(day-1,r['execution']['requested_assignment']['basis']['closed_day'],'no lookahead')
            total_loss+=loss+action['cost'];total_cost+=action['cost'];total_kpi+=loss
            for j in range(2):demand[j]+=w['demand'][j];shipped[j]+=ship[j]
            for j,(v,l,h) in enumerate(zip(k,(.95,.95,.7,3),(1,1,.9,7))):viol[j]+=int(v<l or v>h)
            before=actual['next']
    s=json.loads((folder/'summary.json').read_text())
    for name,v in [('sum_physical_kpi_loss',total_kpi),('sum_executed_action_cost',total_cost),('undiscounted_episode_loss',total_loss),('final_raw',endraw)]:a.eq(v,s[name],name)
    a.vector(back,s['final_backlog'],'final backlog');a.vector(demand,s['total_new_demand'],'demand');a.vector(shipped,s['total_shipped'],'ship');a.vector(viol,s['physical_kpi_outside_soft_range_days'],'soft violations')

def controller(a:Audit,folder:Path):
    with gzip.open(folder/'controller_trace.jsonl.gz','rt',encoding='utf8') as f:
        header=json.loads(next(f));cfg=header['spec']['config'];choice=cfg['choice'];policy=cfg['risk_policy'];pending=None;before=header['spec']['initial_register'];rows=header['spec']['days']
        for day in range(rows):
            r=json.loads(next(f))['payload'];inp=r['input'];out=r['output'];s=out['selection'];ex=r['execution'];action=ex['authorized_action']
            a.eq(pending,ex['requested_assignment'],'next-day pending')
            if pending is None:a.eq(action['id'],'u0','hold baseline');a.eq(None,ex['authorized_assignment_id'],'no fabricated selection')
            else:
                a.eq(day,pending['execute_day'],'execute day');a.eq(day-1,pending['basis']['closed_day'],'information cutoff');a.eq(pending['action'],action,'use saved action snapshot')
                if pending['execution_mode']=='honor_routes_no_human_approval':a.eq(pending['original_route'],'auto','no implicit human approval')
                a.eq(pending['basis']['plan_version'],before['plan']['plan_version'],'plan version at use');a.eq(pending['basis']['constraint_version'],before['permissions']['version'],'permissions at use')
            a.eq(before['permissions']['remaining_normalized_action_budget']-action['cost'],ex['register_after_successful_execution']['permissions']['remaining_normalized_action_budget'],'budget only actual cost')
            before=ex['register_after_successful_execution']
            a.eq(before['plan'],inp['forecast_input']['known_plan'],'known plan');a.eq(before['permissions'],inp['known_permissions'],'known permissions')
            d=s['decision'];fcast=out['pipeline']['forecast']
            if d is not None:
                a.choices+=1;weights={p['id']:p['probability'] for p in fcast['ensemble']['paths']};eligible=[]
                evaluations={e['id']:e for e in d['evaluations']}
                adm=[];riskids=[]
                for alt in fcast['alternatives']:
                    a.alternatives+=1;act=alt['action'];id=act['id'];ev=evaluations[id];cs=checks_for(act,inp['known_permissions'])
                    for g in s['hard_checks'][id]:a.eq(cs[g['id']],g['residual'],'hard residual')
                    ok=all(v<=0 for v in cs.values());a.eq(ok,ev['hard_admissible'],'hard admissible')
                    if not ok:continue
                    adm.append(id);trs=sorted(alt['trajectories'],key=lambda t:t['scenario_id'])
                    zs=[sum(v*choice['discount']**h for h,v in enumerate(t['period_losses']))+act['cost'] for t in trs];ws=[weights[t['scenario_id']] for t in trs]
                    mean,var,cvar=risk(zs,ws,choice['alpha'])
                    # The quantile boundary is also checked numerically, not rounded.
                    for name,v in [('expected',mean),('var',var),('cvar',cvar)]:a.eq(v,ev['risk'][name],name)
                    rok=cvar<=choice['risk_limit'];a.eq(rok,ev['risk_admissible'],'risk flag')
                    if rok:riskids.append(id)
                    base=inp['forecast_input']['known_plan']['base_plan'];plan=base if act['permanent_plan'] is None else act['permanent_plan'];dist=sum(abs(plan[j]*act['temporary_factors'][j]-base[j]) for j in range(2))/max(1,sum(base));a.eq(dist,ev['plan_distance'],'distance')
                    if rok or not policy['risk_constraint']:
                        weight=choice['risk_weight'] if policy['risk_in_objective'] else 0
                        objective=(1-weight)*mean+weight*cvar;a.eq(objective,ev['objective'],'objective');eligible.append((objective,act['cost'],dist,id))
                a.eq(sorted(adm),d['admissible_ids'],'admissible set');a.eq(sorted(riskids),d['risk_ids'],'risk set');a.eq(sorted(x[3] for x in eligible),d['eligible_ids'],'eligible set')
                a.eq(min(eligible)[3] if eligible else None,d['recommendation_id'],'argmin')
            pending=s['assignment']
        footer=json.loads(next(f));a.eq(pending,footer['terminal_unexecuted_assignment'],'terminal pending not execution');a.eq(rows,footer['records'],'count');assert not f.read().strip(),'extra trace lines'

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--suite',default='results/closed_v05');p.add_argument('--out',default='checks/independent_reference_v05.json');args=p.parse_args()
    root=Path(__file__).resolve().parents[1];suite=root/args.suite;cs=json.loads((suite/'suite_config.json').read_text());a=Audit()
    for i,c in enumerate(cs):
        folder=suite/f'episode_{i:02d}';physical(a,folder,c);controller(a,folder)
    result={'notice':'Independent arithmetic and recorded chronology, NOT external validity, not a company trial','episodes':len(cs),'physical_days':a.physical_days,'selection_cases':a.choices,'candidate_checks':a.alternatives,'comparisons':a.checks,'maximum_absolute_difference':a.max_abs,'passed':True}
    dest=Path(args.out);dest=dest if dest.is_absolute() else root/dest;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False,indent=2))
if __name__=='__main__':main()
