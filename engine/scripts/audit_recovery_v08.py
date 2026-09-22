#!/usr/bin/env python3
"""Independent balances/risk/exception audit. No Go calls. Not model validation."""
import gzip,json,pathlib,math
from pathlib import Path
from independent_reference_v05 import Audit,risk,period_loss,checks_for
ROOT=Path(__file__).resolve().parents[1]
def physical(a:Audit,folder:Path,config:dict):
    ec=config['environment']; pp=ec['parameters']; before=ec['initial_state']; total_loss=0.;total_cost=0.;total_kpi=0.; demand=[0.,0.];shipped=[0.,0.];viol=[0]*4
    with gzip.open(folder/'physical.jsonl.gz','rt',encoding='utf8') as f:
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
    for name,v in [('kpi_loss',total_kpi),('action_cost',total_cost),('total_loss',total_loss)]:a.eq(v,s[name],name)
    a.vector(back,s['final_state']['backlog'],'final backlog');a.vector(demand,s['new_demand'],'demand');a.vector(shipped,s['shipped'],'ship');a.vector(viol,s['kpi_outside_soft_range'],'soft violations')

def controller(a,folder,job):
 cfg=job['base'];choice=cfg['controller']['choice'];pol=cfg['controller']['risk_policy'];pending=None;prev_rec=False;prev_empty=False;budget=cfg['initial_command_permissions']['remaining_normalized_action_budget'];rec_props=rec_assigned=rec_exec=rec_noop=pri_exec=empty_hold=0
 with gzip.open(folder/'controller.jsonl.gz','rt') as f:
  for day,line in enumerate(f):
   rr=json.loads(line);r=rr['base'];rec=rr['risk_exception'];s=r['selection'];inp=r['input'];ex=r['execution'];act=ex['authorized_action'];lim=inp['known_permissions'];d=s['decision']
   a.eq(r['day'],day,'day');a.eq(pending,ex['requested_assignment'],'pending chronology');a.eq(rec['risk_certified'],False,'never certify exception');a.eq(rec['unchanged_risk_limit'],choice['risk_limit'],'limit never relaxed')
   if pending is None:
    a.eq(ex['authorized_assignment_id'],None,'no fabricated execution');a.eq(act['id'],'u0','continuation explicit');empty_hold+=int(prev_empty)
   else:
    a.eq(pending['execute_day'],day,'one day delay');a.eq(pending['basis']['closed_day'],day-1,'no future observations');a.eq(pending['action'],act,'exact saved action')
    if prev_rec:rec_exec+=1;rec_noop+=int(act['id']=='u0')
    else:pri_exec+=1
    # execution limits are the previous balance BEFORE current cost deduction.
    pre=dict(lim);pre['remaining_normalized_action_budget']=budget
    a.eq(all(v<=0 for v in checks_for(act,pre).values()),True,'execution hard limits')
   a.eq(budget-act['cost'],lim['remaining_normalized_action_budget'],'cost once');budget=lim['remaining_normalized_action_budget']
   values={}
   if d is not None:
    a.choices+=1;ws=r['ordered_scenario_weights'];actions={x['id']:x for x in r['actions']};adm=[];rids=[]
    for ev in d['evaluations']:
     a.alternatives+=1;id=ev['id'];ac=actions[id];cs=checks_for(ac,lim)
     for x in s['hard_checks'][id]:a.eq(x['residual'],cs[x['id']],'hard check')
     hard=all(v<=0 for v in cs.values());a.eq(hard,ev['hard_admissible'],'hard eligibility')
     if not hard:continue
     adm.append(id);zs=ev['scenario_losses'];a.eq(len(zs),len(ws),'path sizes');a.eq(sum(ws),1.,'mass')
     mu,var,cv=risk(zs,ws,choice['alpha'])
     for name,val in [('expected',mu),('var',var),('cvar',cv)]:a.eq(val,ev['risk'][name],'risk arithmetic')
     rok=cv<=choice['risk_limit'];a.eq(rok,ev['risk_admissible'],'risk flag')
     if rok:rids.append(id)
     base=inp['forecast_input']['known_plan']['base_plan'];pl=base if ac['permanent_plan'] is None else ac['permanent_plan'];dist=sum(abs(pl[k]*ac['temporary_factors'][k]-base[k]) for k in [0,1])/max(1,sum(base));a.eq(dist,ev['plan_distance'],'distance')
     values[id]=(mu,cv,ac['cost'],dist,rok,ev)
    a.eq(sorted(adm),d['admissible_ids'],'hard set');a.eq(sorted(rids),d['risk_ids'],'risk set')
    for shadow in r['same_information_choices']:
     p=shadow['policy'];w=choice['risk_weight'] if p['risk_in_objective'] else 0;elig=[]
     for id,(mu,cv,cost,dist,rok,ev) in values.items():
      if rok or not p['risk_constraint']:
       j=(1-w)*ev['risk']['expected']+w*ev['risk']['cvar'];ji=(1-w)*mu+w*cv;elig.append((j,cost,dist,id,ji))
       if p['id']==pol['id']:a.eq(j,ev['objective'],'objective')
     best=min(elig) if elig else None;a.eq(sorted(x[3] for x in elig),shadow['eligible_ids'],'eligible shadow');a.eq(best[3] if best else None,shadow['recommendation_id'],'argmin shadow')
     if best:a.eq(best[0],min(x[4] for x in elig),'independent min up to roundoff')
     reason='RECOMMENDATION' if best else ('ADM_EMPTY' if not adm else 'RISK_EMPTY');a.eq(reason,shadow['reason'],'reason')
     if p['id']==pol['id']:a.eq(shadow['recommendation_id'],d['recommendation_id'],'same primary')
   triggered=s['reason']=='RISK_EMPTY';a.eq(triggered,rec['triggered'],'recovery only risk empty')
   if triggered:
    a.eq(s['recommendation'],None,'primary recommendation stays empty');a.eq(s['assignment'],None,'no fake primary assignment');a.eq(d['risk_ids'],[],'empty remains empty')
    saved=rec['hard_admissible_candidates'];a.eq(sorted(x['id'] for x in saved),d['admissible_ids'],'exception keeps hard set')
    for x in saved:
     mu,cv,cost,dist,_,ev=values[x['id']]
     for key,val in [('expected',mu),('cvar',cv),('risk_limit_excess',cv-choice['risk_limit']),('action_cost',cost),('plan_distance',dist)]:a.eq(x[key],val,'exception score')
     a.eq(x['risk_limit_excess']>0,True,'violation explicitly positive')
    if job['recovery']['policy']=='hold_base_plan':a.eq(rec['selected'],None,'hold no exception')
    else:
     key=lambda x: ((x['risk_limit_excess'],) if job['recovery']['policy']=='min_cvar_excess_exception' else ())+(x['expected'],x['action_cost'],x['plan_distance'],x['id'])
     best=min(saved,key=key);a.eq(best,rec['selected'],'lexicographic exception');a.eq(best['id'],rec['contingency_proposal_not_primary_recommendation']['id'],'saved action')
     if rec['baseline_if_hard_admissible'] and job['recovery']['policy']=='min_cvar_excess_exception':a.eq(best['cvar']<=rec['baseline_if_hard_admissible']['cvar'],True,'model CVaR <= baseline on SAME distribution')
     if rec['research_exception_assignment'] is not None:
      a.eq(job['recovery']['permit_research_assignment_without_human'],True,'separate permission');a.eq(cfg['controller']['execution_mode'],'simulation_no_review_not_human_approval','research only');a.eq(rec['research_exception_assignment']['original_route'],'committee','no fabricated auto approval')
   else:a.eq(rec['contingency_proposal_not_primary_recommendation'],None,'no exception on other statuses')
   rec_props+=int(rec['contingency_proposal_not_primary_recommendation'] is not None);rec_assigned+=int(rec['research_exception_assignment'] is not None)
   pending=s['assignment'] or rec['research_exception_assignment'];prev_rec=rec['research_exception_assignment'] is not None;prev_empty=triggered
 sm=json.loads((folder/'summary.json').read_text())
 for name,v in [('recovery_proposals',rec_props),('recovery_assignments',rec_assigned),('recovery_executions',rec_exec),('recovery_noop_executions',rec_noop),('primary_executions',pri_exec),('risk_empty_followed_by_no_assignment',empty_hold)]:a.eq(sm[name],v,'summary '+name)
 a.eq(sm['pending_unexecuted_at_end'],pending,'terminal not counted executed')

def main():
 suite=ROOT/'results/recovery_v08';jobs=json.loads((suite/'plan.json').read_text());a=Audit();tapes={}
 for i,j in enumerate(jobs):
  folder=suite/'episodes'/f'ep{i:03d}';a.eq(json.loads((folder/'experiment_config.json').read_text()),j,'frozen configuration')
  done=json.loads((folder/'COMPLETED.json').read_text());tapes.setdefault((j['profile'],j['repeat']),set()).add(done['exogenous_tape_hash']);physical(a,folder,j['base']);controller(a,folder,j)
 for v in tapes.values():a.eq(len(v),1,'same forcing and errors within four arms')
 out={'notice':'Independent numerical balances, stored distributions and decision rules; not independent implementation of full forecast generator or model validation','episodes':len(jobs),'physical_days':a.physical_days,'paired_blocks':len(tapes),'forecast_cases':a.choices,'alternative_evaluations':a.alternatives,'comparisons':a.checks,'max_absolute_difference':a.max_abs,'passed':True}
 (ROOT/'checks_v0_8/independent_audit.json').write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out,indent=2))
if __name__=='__main__':main()
