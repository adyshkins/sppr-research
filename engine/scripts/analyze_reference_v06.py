#!/usr/bin/env python3
"""Describe independent environment references; explicitly different conditioning.
Independent Python CVaR arithmetic, no Go import or subprocess.
"""
from __future__ import annotations
import csv,gzip,json,pathlib
from independent_reference_v05 import Audit,risk
ROOT=pathlib.Path(__file__).resolve().parents[1]
def read(p):return json.loads(pathlib.Path(p).read_text(encoding='utf8'))
def getrow(p,day):
 with gzip.open(p,'rt',encoding='utf8') as f:
  for i,line in enumerate(f):
   if i==day:return json.loads(line)
 raise ValueError('missing source day')
def main():
 suite=ROOT/'results/factorial_v06';base=suite/'references';runs=read(base/'run_summary.json');rows=[];checks=Audit();attempts=[]
 for task in runs:
  name=task['checkpoint'];folder=base/name
  if task['status'].startswith('skipped'):
   attempts.append({**task,**read(folder/'SKIPPED.json')});continue
  assert task['status']=='passed' and task['exit_code']==0
  s=read(folder/'reference_spec.json');summary=read(folder/'summary.json')
  with gzip.open(folder/'reference_losses.json.gz','rt',encoding='utf8') as f:r=json.load(f)
  source=suite/'episodes'/f"episode_{task['source_job']:03d}";controller=getrow(source/'compact_controller.jsonl.gz',task['day']);physical=getrow(source/'physical_evaluation.jsonl.gz',task['day'])
  checks.eq(s['true_start_evaluator_only'],physical['physical_result_evaluation_only']['next'],'use state after closed day')
  checks.eq(s['saved_actions_not_recomputed_from_truth'],controller['actions'],'same saved actions including u6')
  checks.eq(s['source_record_hash'],controller['hash'],'bound source')
  cs={x['policy']['id']:x for x in summary['same_information_choices']};selected=cs['R00']['recommendation_id'];attempts.append(task)
  for id,z in r['horizon_losses'].items():
   n=s['paths'];checks.eq(len(z),n,'sample size');m,v,c=risk(z,[1/n]*n,s['alpha'])
   for key,x in [('expected',m),('var',v),('cvar',c)]:checks.eq(x,r['environment_reference_risk'][id][key],'reference risk')
   for b in (0,1):
    mu,va,cv=risk(z[b*(n//2):(b+1)*(n//2)],[2/n]*(n//2),s['alpha'])
    for key,x in [('expected',mu),('var',va),('cvar',cv)]:checks.eq(x,r['two_half_sample_estimates'][id][b][key],'half estimate')
   p=summary['predictor_risk'][id];ref=r['environment_reference_risk'][id];rows.append({'checkpoint':name,'source_job':task['source_job'],'profile':task['profile'],'repeat':task['repeat'],'closed_day':task['day'],'action':id,'selected_R00':id==selected,'R01_reason':cs['R01']['reason'],'paths':n,'pred_mean':p['expected'],'pred_cvar':p['cvar'],'reference_mean':ref['expected'],'reference_cvar':ref['cvar'],'half_1_cvar':r['two_half_sample_estimates'][id][0]['cvar'],'half_2_cvar':r['two_half_sample_estimates'][id][1]['cvar'],'pred_above_limit':p['cvar']>summary['risk_limit'],'reference_above_limit':ref['cvar']>summary['risk_limit']})
 selected=[x for x in rows if x['selected_R00']]
 out={'notice':'Different conditioning: predictor uses observed-state estimate and geometric recovery, reference uses true current state and declared event schedule. Discrepancy is not an unconditional calibration test or a real-world safety guarantee. Each two half-batch estimates are MC sensitivity information, not confidence intervals.','preselected_checkpoints':len(runs),'evaluated_checkpoints':len(selected),'skipped_checkpoints':len(runs)-len(selected),'continuations_per_checkpoint':2048,'shared_continuations':len(selected)*2048,'action_evaluations':len(rows)*2048,'reference_balance_steps':len(rows)*2048*7,'selected_R00_above_pred_limit_but_below_reference_limit':sum(x['pred_above_limit'] and not x['reference_above_limit'] for x in selected),'checks':checks.checks,'maximum_absolute_difference':checks.max_abs,'attempts':attempts,'selected_R00_actions':selected}
 (base/'analysis.json').write_text(json.dumps(out,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
 with (base/'all_action_risks.csv').open('w',encoding='utf-8-sig',newline='') as f:w=csv.DictWriter(f,fieldnames=list(rows[0]));w.writeheader();w.writerows(rows)
 lines=['# Независимые продолжения среды: DEV-F06','',out['notice'],'',f"Заранее выбрано {len(runs)} состояний; оценено {len(selected)}; пропущено {len(runs)-len(selected)} из-за отсутствия готового прогноза, без замены.",'', '| Профиль / повторение / день (индекс) | Действие R00 | CVaR_pred | CVaR_env_ref | Две партии по 1024 | R01 |','|---|---|---:|---:|---|---|']
 for x in selected:lines.append(f"| {x['profile']} / {x['repeat']} / {x['closed_day']} | {x['action']} | {x['pred_cvar']:.6f} | {x['reference_cvar']:.6f} | {x['half_1_cvar']:.6f}; {x['half_2_cvar']:.6f} | {x['R01_reason']} |")
 lines+=['', 'В таблице показано действие R00; в CSV и первичных файлах сохранены результаты ВСЕХ восьми альтернатив. Продолжения общие для альтернатив, но независимы от ансамбля выбора. После первого действия новых корректировок на горизонте нет, как и в прогнозе.','', 'Истинное состояние и расписание прекращения сбоя доступны только оценочному модулю. Поэтому различие включает ошибку оценки состояния, различие модели неопределённости и конечность ансамблей. По этим 17 выбранным состояниям нельзя утверждать глобальную калибровку, внешнюю валидность или необходимость произвольного повышения порога риска.','']
 (base/'RESULTS.md').write_text('\n'.join(lines),encoding='utf8');print(json.dumps({k:v for k,v in out.items() if k not in ('attempts','selected_R00_actions')},indent=2))
if __name__=='__main__':main()
