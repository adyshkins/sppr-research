#!/usr/bin/env python3
"""Descriptive paired episode analysis, all profiles. No parameter selection."""
import csv,json,gzip,pathlib,itertools,hashlib
import numpy as np
ROOT=pathlib.Path(__file__).resolve().parents[1];SUITE=ROOT/'results/recovery_v08'
def read(p):return json.loads(p.read_text())
def csvsave(path,rows):
 if not rows:return
 with path.open('w',newline='',encoding='utf-8-sig') as f:
  w=csv.DictWriter(f,fieldnames=list(rows[0]));w.writeheader();w.writerows(rows)
def main():
 done=read(SUITE/'SERIES_COMPLETED.json');jobs=read(SUITE/'plan.json');assert done['jobs']==len(jobs)==240
 dest=SUITE/'analysis';dest.mkdir(exist_ok=True);rows=[];phase=[];same=[]
 for i,c in enumerate(jobs):
  folder=SUITE/'episodes'/f'ep{i:03d}';s=read(folder/'summary.json');bypass=0;services=[];backlog=0.;comp=[0.]*4;actual_recovery_risk=[]
  with gzip.open(folder/'physical.jsonl.gz','rt') as f:
   for line in f:
    p=json.loads(line);k=p['physical_result_evaluation_only']['kpi'];services.append(k[0]);backlog+=sum(p['physical_result_evaluation_only']['next']['backlog'])
    for j,(v,l,h,w,a) in enumerate(zip(k,(.95,.95,.7,3),(1,1,.9,7),(.35,.25,.15,.25),(0,0,.5,.25))):comp[j]+=w*(max(l-v,0)+a*max(v-h,0))/l
    pending=p['execution']['requested_assignment']
    if p['execution']['authorized_assignment_id'] is not None and pending['original_route']!='auto':bypass+=1
  row={'job':i,'profile':c['profile'],'repeat':c['repeat'],'arm':c['arm'],'total_loss':s['total_loss'],'kpi_loss':s['kpi_loss'],'cost':s['action_cost'],'shipment_ratio':sum(s['shipped'])/sum(s['new_demand']),'service_mean':sum(services)/len(services),'backlog_unit_days':backlog,'service_loss':comp[0],'plan_loss':comp[1],'capacity_loss':comp[2],'raw_loss':comp[3],'risk_empty':s['primary_reasons'].get('RISK_EMPTY',0),'forecasts':s['forecasts'],'corrections':sum(n for k,n in s['executed_actions'].items() if k!='u0'),'exception_proposed':s['recovery_proposals'],'exception_assigned':s['recovery_assignments'],'exception_executed':s['recovery_executions'],'exception_u0':s['recovery_noop_executions'],'primary_executed':s['primary_executions'],'refusal_then_hold':s['risk_empty_followed_by_no_assignment'],'pending':int(s['pending_unexecuted_at_end'] is not None),'pending_recovery':int(s['pending_is_recovery']),'hard_violations':s['executed_assignment_hard_permission_violations'],'research_review_bypasses':bypass}
  assert abs(sum(comp)-s['kpi_loss'])<1e-9
  rows.append(row)
  for name,v in s['phases'].items():phase.append({'job':i,'profile':c['profile'],'repeat':c['repeat'],'arm':c['arm'],'phase':name,**v})
  # Fixed-state comparison confined to H cases: no conflation with state divergence.
  if c['arm']=='H':
   with gzip.open(folder/'controller.jsonl.gz','rt') as f:
    for line in f:
     rr=json.loads(line);r=rr['base'];rec=rr['risk_exception']
     if not rec['triggered']:continue
     cs=rec['hard_admissible_candidates'];em=min(cs,key=lambda x:(x['expected'],x['action_cost'],x['plan_distance'],x['id']));et=min(cs,key=lambda x:(x['risk_limit_excess'],x['expected'],x['action_cost'],x['plan_distance'],x['id']));u0=rec['baseline_if_hard_admissible']
     same.append({'job':i,'profile':c['profile'],'repeat':c['repeat'],'day':r['day'],'mean_id':em['id'],'tail_id':et['id'],'same_action':em['id']==et['id'],'u0_cvar':u0['cvar'] if u0 else None,'mean_cvar':em['cvar'],'tail_cvar':et['cvar'],'mean_expected':em['expected'],'tail_expected':et['expected'],'tail_excess':et['risk_limit_excess'],'tail_pred_risk_le_u0':None if u0 is None else et['cvar']<=u0['cvar']})
 csvsave(dest/'episode_metrics.csv',rows);csvsave(dest/'phase_metrics.csv',phase);csvsave(dest/'same_information_cases.csv',same)
 metrics=[k for k in rows[0] if k not in ('job','profile','repeat','arm')];means=[]
 for profile,arm in itertools.product(['normal','S','C','D','DS'],['B0','H','EM','ET']):
  rs=[r for r in rows if r['profile']==profile and r['arm']==arm];assert len(rs)==12
  means.append({'profile':profile,'arm':arm,'n':12,**{k:float(np.mean([r[k] for r in rs])) for k in metrics}})
 csvsave(dest/'group_means.csv',means)
 rng=np.random.default_rng(810808);indices=rng.integers(0,12,(10000,12));contrasts=[]
 for profile in ['normal','S','C','D','DS']:
  for left,right in [('EM','H'),('ET','H'),('ET','EM'),('ET','B0'),('EM','B0')]:
   for metric in ['total_loss','cost','shipment_ratio','backlog_unit_days','risk_empty']:
    a=np.array([next(r[metric] for r in rows if r['profile']==profile and r['arm']==left and r['repeat']==i) for i in range(12)]);b=np.array([next(r[metric] for r in rows if r['profile']==profile and r['arm']==right and r['repeat']==i) for i in range(12)]);d=a-b;interval=np.quantile(d[indices].mean(axis=1),[.025,.975])
    contrasts.append({'profile':profile,'contrast':left+'-'+right,'metric':metric,'mean_difference':float(d.mean()),'ci95_low':float(interval[0]),'ci95_high':float(interval[1]),'negative_count':int(np.sum(d<0)),'positive_count':int(np.sum(d>0)),'tie_count':int(np.sum(d==0)),'n':12})
 csvsave(dest/'paired_contrasts.csv',contrasts)
 txt=['# DEV-R08: результаты разработочной серии','', '240 эпизодов по 60 суток, 12 согласованных повторений каждого профиля. Сумма нормированных потерь и исполненной стоимости за эпизод; не рубли. Основная модель, качество данных и E неизменны. Исключения EM/ET не выполняют риск-лимит и не названы риск-допустимыми.','', '| Профиль | B0: среднее без фильтра | H: продолжение плана | EM: исключение по среднему | ET: исключение по хвосту |','|---|---:|---:|---:|---:|']
 for p in ['normal','S','C','D','DS']:
  vals=[next(r['total_loss'] for r in means if r['profile']==p and r['arm']==arm) for arm in ['B0','H','EM','ET']];txt.append('| '+p+' | '+' | '.join(f'{x:.6f}' for x in vals)+' |')
 txt+=['','## Компоненты и процесс','', '| Профиль / вариант | KPI-потери | Стоимость | Отгрузка/новый спрос, % | RISK_EMPTY, среднее | Исполнения исключений, среднее |','|---|---:|---:|---:|---:|---:|']
 for r in means:txt.append(f"| {r['profile']} / {r['arm']} | {r['kpi_loss']:.6f} | {r['cost']:.6f} | {100*r['shipment_ratio']:.5f} | {r['risk_empty']:.3f} | {r['exception_executed']:.3f} |")
 txt+=['','## Парные разности потерь','', 'Интервалы описательные, 10 000 bootstrap-выборок по целым согласованным траекториям. Без поправки на множественность; не глобальные гарантии и не оценка переноса на реальные производства. Отрицательная разность выгоднее левому варианту по принятому функционалу.','', '| Профиль | Разность | Среднее | 95% интервал | Отрицательных из 12 |','|---|---|---:|---|---:|']
 for x in contrasts:
  if x['metric']=='total_loss':txt.append(f"| {x['profile']} | {x['contrast']} | {x['mean_difference']:.6f} | [{x['ci95_low']:.6f}; {x['ci95_high']:.6f}] | {x['negative_count']} |")
 totals={k:sum(r[k] for r in rows) for k in ['forecasts','risk_empty','corrections','exception_proposed','exception_assigned','exception_executed','exception_u0','primary_executed','refusal_then_hold','pending','hard_violations','research_review_bypasses']}
 txt+=['','## Границы интерпретации','', '- Это новая серия разработки после просмотра 0.6–0.7, не финальный hold-out.', '- Исследовательское исключение намеренно может исполнить действие выше расчётного лимита; это не сертифицированное безопасное управление.', '- Все неавтоматические назначения исполняются по исследовательскому соглашению без настоящего эксперта и без его задержки.', '- Нормированные KPI-потери не являются денежным ущербом; учитывайте отдельные компоненты и обслуживание.', '- Минимальный прогнозный CVaR на одном состоянии не гарантирует меньший фактический риск или потерю замкнутой траектории.', '', 'Файлы: `episode_metrics.csv`, `phase_metrics.csv`, `same_information_cases.csv`, `group_means.csv`, `paired_contrasts.csv`.']
 (dest/'RESULTS.md').write_text('\n'.join(txt)+'\n',encoding='utf8');(dest/'analysis.json').write_text(json.dumps({'jobs':240,'days':14400,'paired_blocks':60,'totals':totals,'same_H_cases':len(same),'same_H_mean_tail_differences':sum(not x['same_action'] for x in same),'groups':means,'contrasts':contrasts},indent=2)+'\n');print('\n'.join(txt[:14]));print(totals)
if __name__=='__main__':main()
