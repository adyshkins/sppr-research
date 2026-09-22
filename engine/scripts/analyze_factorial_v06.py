#!/usr/bin/env python3
"""Descriptive paired DEV-F06 analysis. Standard library only.
Bootstrap resamples whole matched replicate IDs, NOT days or forecast paths.
Intervals are individual, unadjusted; no confirmatory significance claims.
"""
from __future__ import annotations
import argparse,csv,gzip,hashlib,json,math,pathlib,random,statistics
ROOT=pathlib.Path(__file__).resolve().parents[1]
def read(p):return json.loads(pathlib.Path(p).read_text(encoding='utf8'))
def rows(p):
 with gzip.open(p,'rt',encoding='utf8') as f:
  for line in f:yield json.loads(line)
def quantile(x,p):
 a=sorted(x);v=(len(a)-1)*p;i=int(v);return a[i] if i==len(a)-1 else a[i]+(a[i+1]-a[i])*(v-i)
def empirical_cvar(x,alpha=.95):
 a=sorted(x,reverse=True);left=len(a)*(1-alpha);den=left;acc=0.
 for v in a:
  take=min(left,1.);acc+=v*take;left-=take
  if left<=0:break
 return acc/den
def mean(x):return sum(x)/len(x)
def bootstrap(v,indices):
 return [quantile([mean([v[i] for i in ix]) for ix in indices],q) for q in (.025,.975)]
def analyze(suite):
 done=read(suite/'SERIES_COMPLETED.json');jobs=read(suite/'plan.json')
 assert done['jobs']==len(jobs)
 if hashlib.sha256((suite/'plan.json').read_bytes()).hexdigest()!=done['plan_sha256']:raise ValueError('plan changed')
 episodes=[];shadow={};group_tapes={};pred_cases=[]
 for i,j in enumerate(jobs):
  folder=suite/'episodes'/f'episode_{i:03d}';c=read(folder/'experiment_config.json');assert c==j['config'];complete=read(folder/'COMPLETED.json');s=read(folder/'summary.json');cs=s['controller_summary'];physical=list(rows(folder/'physical_evaluation.jsonl.gz'));compact=list(rows(folder/'compact_controller.jsonl.gz'))
  assert len(physical)==len(compact)==complete['records']==c['environment']['days']
  pair=(j['profile'],j['repeat']);group_tapes.setdefault(pair,set()).add(complete['exogenous_tape_hash'])
  total_d=sum(s['total_new_demand']);total_s=sum(s['total_shipped']);daily=[x['physical_loss_plus_action_cost'] for x in physical]
  e={'job':i,'profile':j['profile'],'repeat':j['repeat'],'policy':j['policy'],'loss':s['undiscounted_episode_loss'],'kpi_loss':s['sum_physical_kpi_loss'],'action_cost':s['sum_executed_action_cost'],'shipment_ratio_new_demand':total_s/total_d,'mean_daily_service_kpi':mean([x['physical_result_evaluation_only']['kpi'][0] for x in physical]),'backlog_unit_days':sum(sum(x['physical_result_evaluation_only']['next']['backlog']) for x in physical),'final_backlog':sum(s['final_backlog']),'daily_loss_empirical_cvar95':empirical_cvar(daily),'max_daily_loss':max(daily),'forecasts':cs['forecasts'],'risk_empty':cs['selection_reason_counts'].get('RISK_EMPTY',0),'adm_empty':cs['selection_reason_counts'].get('ADM_EMPTY',0),'data_check':cs['route_counts'].get('data_check',0),'recommendations':cs['recommendations'],'assignments':cs['assignments'],'applied_assignments':cs['applied_assignments_including_u0'],'corrections':cs['executed_non_u0_actions'],'research_bypasses':cs['applied_research_route_bypasses'],'terminal_assignment':int(cs['terminal_unexecuted_assignment'] is not None),'hard_violations':s['assigned_hard_permission_violations']}
  for k,v in enumerate(s['physical_kpi_outside_soft_range_days']):e[f'kpi{k+1}_violation_days']=v
  episodes.append(e)
  key=(j['profile'],j['policy']);sh=shadow.setdefault(key,{'cases':0,'objective_change_no_limit':0,'constraint_change_no_weight':0,'combined_change':0,'constraint_veto_R01':0,'constraint_veto_R11':0,'objective_change_with_limit':0,'constraint_change_with_weight':0})
  for r in compact:
   choices=r['same_information_choices']
   if not choices:continue
   a={q['policy']['id']:q for q in choices};v={p:x['recommendation_id'] for p,x in a.items()};sh['cases']+=1
   sh['objective_change_no_limit']+=v['R10']!=v['R00'];sh['constraint_change_no_weight']+=v['R01']!=v['R00'];sh['combined_change']+=v['R11']!=v['R00'];sh['constraint_veto_R01']+=a['R01']['reason']=='RISK_EMPTY';sh['constraint_veto_R11']+=a['R11']['reason']=='RISK_EMPTY';sh['objective_change_with_limit']+=v['R11']!=v['R01'];sh['constraint_change_with_weight']+=v['R11']!=v['R10']
 for key,h in group_tapes.items():
  if len(h)!=1:raise ValueError(f'Unmatched exogenous tape: {key}')
 profiles=list(dict.fromkeys(j['profile'] for j in jobs));policies=['R00','R10','R01','R11'];metrics=[k for k in episodes[0] if k not in ('job','profile','repeat','policy')]
 group=[];contrasts=[]
 definitions={
 'R10-R00':{'R10':1,'R00':-1},'R01-R00':{'R01':1,'R00':-1},'R11-R00':{'R11':1,'R00':-1},'R11-R10':{'R11':1,'R10':-1},'R11-R01':{'R11':1,'R01':-1},'interaction':{'R11':1,'R10':-1,'R01':-1,'R00':1},'objective_marginal':{'R10':.5,'R00':-.5,'R11':.5,'R01':-.5},'constraint_marginal':{'R01':.5,'R00':-.5,'R11':.5,'R10':-.5}}
 for pi,profile in enumerate(profiles):
  by={p:sorted([e for e in episodes if e['profile']==profile and e['policy']==p],key=lambda x:x['repeat']) for p in policies};n=len(by['R00']);assert all(len(v)==n for v in by.values())
  assert all([e['repeat'] for e in by[p]]==[e['repeat'] for e in by['R00']] for p in policies)
  for p in policies:group.append({'profile':profile,'policy':p,'n':n,**{m:mean([e[m] for e in by[p]]) for m in metrics},'loss_sd':statistics.stdev([e['loss'] for e in by[p]]) if n>1 else 0})
  rng=random.Random(510001+pi);indices=[[rng.randrange(n) for _ in range(n)] for _ in range(10000)]
  for name,coef in definitions.items():
   v=[sum(coef[p]*by[p][r]['loss'] for p in coef) for r in range(n)];ci=bootstrap(v,indices)
   contrasts.append({'profile':profile,'contrast':name,'n_pairs':n,'mean_difference':mean(v),'individual_bootstrap95_lower':ci[0],'individual_bootstrap95_upper':ci[1],'negative_differences':sum(x<0 for x in v),'zero_differences':sum(x==0 for x in v),'positive_differences':sum(x>0 for x in v),'paired_differences':v})
 totals={m:sum(e[m] for e in episodes) for m in ['forecasts','recommendations','assignments','applied_assignments','corrections','research_bypasses','terminal_assignment','risk_empty','hard_violations']}
 out={'notice':'DEVELOPMENT SERIES ONLY. Individual paired 95% bootstrap intervals, 10000 resamples, not multiplicity-adjusted; no formal significance decision. Days/checkpoints/forecast paths are not independent replication units.','jobs':len(jobs),'physical_days':done['days'],'paired_quartets':len(group_tapes),'totals':totals,'groups':group,'contrasts':contrasts,'same_information_descriptive':[{'profile':p,'origin_policy':m,**v} for (p,m),v in shadow.items()]}
 return episodes,out

def render(a):
 lines=['# Результаты разработочной серии DEV-F06','',a['notice'],'','## Средние суммы потерь за 60 суток','', '| Профиль | Повторов на режим | R00 | R10 | R01 | R11 |','|---|---:|---:|---:|---:|---:|']
 profiles=list(dict.fromkeys(g['profile'] for g in a['groups']))
 for p in profiles:
  d={g['policy']:g for g in a['groups'] if g['profile']==p};lines.append('| '+p+' | '+str(d['R00']['n'])+' | '+' | '.join(f"{d[m]['loss']:.6f}" for m in ['R00','R10','R01','R11'])+' |')
 lines+=['','Меньше — лучше только по объявленной нормированной сумме; это не рубли. R10 меняет критерий, R01 вводит ограничение, R11 делает оба изменения.','', '## Парные различия сумм потерь','', '| Профиль | Контраст | Среднее | 95% индивидуальный интервал | Отрицательных / нулевых / положительных |','|---|---|---:|---|---|']
 for c in a['contrasts']:
  if c['contrast'] not in ('R10-R00','R01-R00','R11-R00','interaction'):continue
  lines.append(f"| {c['profile']} | {c['contrast']} | {c['mean_difference']:.6f} | [{c['individual_bootstrap95_lower']:.6f}; {c['individual_bootstrap95_upper']:.6f}] | {c['negative_differences']} / {c['zero_differences']} / {c['positive_differences']} |")
 lines+=['','Контрасты R11-R10, R11-R01 и средние главные эффекты также сохранены в JSON/CSV. Интервалы не являются одновременными, не скорректированы на множественность.','', '## Другие характеристики, среднее по эпизодам','', '| Профиль | Режим | KPI-потери | Стоимость | Отгрузка / новый спрос | Средний суточный KPI сервиса | Задолженность, ед.×сутки | RISK_EMPTY | Коррекции |','|---|---|---:|---:|---:|---:|---:|---:|---:|']
 for g in a['groups']:
  lines.append(f"| {g['profile']} | {g['policy']} | {g['kpi_loss']:.6f} | {g['action_cost']:.6f} | {g['shipment_ratio_new_demand']:.6f} | {g['mean_daily_service_kpi']:.6f} | {g['backlog_unit_days']:.3f} | {g['risk_empty']:.2f} | {g['corrections']:.2f} |")
 lines+=['','Суммарная отгрузка может быть высокой при задержках: поэтому рядом приведены суточный сервис и накопленная задолженность. Исходный долг равен нулю. Мягкие выходы KPI, маршруты, последние неисполненные назначения и распределения действий сохранены в первичных файлах.','', '## Четыре выбора на одном и том же состоянии: только состояния траекторий R00','', '| Профиль | Готовых случаев | Изменение R10 против R00 | Изменение R01 против R00 | RISK_EMPTY для R01 | Изменение R11 против R00 |','|---|---:|---:|---:|---:|---:|']
 for s in a['same_information_descriptive']:
  if s['origin_policy']=='R00':lines.append(f"| {s['profile']} | {s['cases']} | {s['objective_change_no_limit']} | {s['constraint_change_no_weight']} | {s['constraint_veto_R01']} | {s['combined_change']} |")
 lines+=['','Это описательные вложенные случаи, не дополнительные независимые повторы. Четыре теневых выбора не исполняются одновременно и не изменяют историю монитора.','', '## Ограничения вывода','', 'Все рекомендации в серии исполняются по одинаковому исследовательскому соглашению без ожидания эксперта. Маршруты не перенастраиваются. Выводы относятся к фиксированной синтетической среде, известным профилям и текущим параметрам риска/прогноза. Для окончательной проверки нужна последующая заморозка метода и отдельная серия. Прежние 1650 прогонов не воспроизводятся этим расчётом.','']
 return '\n'.join(lines)

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--suite',default='results/factorial_v06');args=p.parse_args();suite=ROOT/args.suite;e,a=analyze(suite);out=suite/'analysis';out.mkdir(exist_ok=True)
 (out/'analysis.json').write_text(json.dumps(a,indent=2,ensure_ascii=False)+'\n',encoding='utf8');(out/'RESULTS.md').write_text(render(a),encoding='utf8')
 for name,data in [('episode_metrics.csv',e),('group_means.csv',a['groups']),('paired_contrasts.csv',[{k:v for k,v in c.items() if k!='paired_differences'} for c in a['contrasts']]),('same_information.csv',a['same_information_descriptive'])]:
  with (out/name).open('w',encoding='utf-8-sig',newline='') as f:w=csv.DictWriter(f,fieldnames=list(data[0]));w.writeheader();w.writerows(data)
 print(json.dumps({'jobs':a['jobs'],'days':a['physical_days'],'totals':a['totals']},indent=2))
if __name__=='__main__':main()
