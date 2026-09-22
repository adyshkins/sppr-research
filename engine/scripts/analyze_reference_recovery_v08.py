#!/usr/bin/env python3
"""Independent fractional-tail check and descriptive fixed-state comparisons."""
import json,gzip,csv,pathlib
from independent_reference_v05 import Audit,risk
ROOT=pathlib.Path(__file__).resolve().parents[1];BASE=ROOT/'results/recovery_v08/references'
def main():
 plan=json.loads((BASE/'plan.json').read_text());a=Audit();rows=[];skip=[]
 for x in plan:
  folder=BASE/f"ep{x['job']:03d}_day{x['day']:02d}"
  if (folder/'SKIPPED.json').exists():skip.append({**x,**json.loads((folder/'SKIPPED.json').read_text())});continue
  r=json.loads(gzip.open(folder/'reference_losses.json.gz','rt').read());record=json.loads((folder/'source_record.json').read_text());cs=record['risk_exception']['hard_admissible_candidates'];em=min(cs,key=lambda z:(z['expected'],z['action_cost'],z['plan_distance'],z['id']));et=min(cs,key=lambda z:(z['risk_limit_excess'],z['expected'],z['action_cost'],z['plan_distance'],z['id']));u0=record['risk_exception']['baseline_if_hard_admissible'];risks={}
  for id,zs in r['horizon_losses'].items():
   vals=risk(zs,[1/len(zs)]*len(zs),.95);risks[id]=dict(zip(('expected','var','cvar'),vals))
   for name,val in risks[id].items():a.eq(val,r['environment_reference_risk'][id][name],name)
   for k in [0,1]:
    sub=zs[k*len(zs)//2:(k+1)*len(zs)//2];v=risk(sub,[1/len(sub)]*len(sub),.95)
    for name,val in zip(('expected','var','cvar'),v):a.eq(val,r['two_half_sample_estimates'][id][k][name],name)
  row={k:x[k] for k in ['profile','repeat','day','job']}
  row.update({'em_id':em['id'],'et_id':et['id'],'same_action':em['id']==et['id'],'u0_pred_cvar':u0['cvar'] if u0 else None,'em_pred_cvar':em['cvar'],'et_pred_cvar':et['cvar'],'u0_env_cvar':risks['u0']['cvar'],'em_env_cvar':risks[em['id']]['cvar'],'et_env_cvar':risks[et['id']]['cvar'],'u0_env_expected':risks['u0']['expected'],'em_env_expected':risks[em['id']]['expected'],'et_env_expected':risks[et['id']]['expected']});rows.append(row)
 with (BASE/'comparison.csv').open('w',encoding='utf-8-sig',newline='') as f:w=csv.DictWriter(f,fieldnames=list(rows[0]));w.writeheader();w.writerows(rows)
 report={'requested':24,'evaluated':len(rows),'skipped':len(skip),'et_env_cvar_below_u0':sum(r['et_env_cvar']<r['u0_env_cvar'] for r in rows),'em_env_cvar_below_u0':sum(r['em_env_cvar']<r['u0_env_cvar'] for r in rows),'et_env_cvar_below_em':sum(r['et_env_cvar']<r['em_env_cvar'] for r in rows),'et_env_cvar_above_em':sum(r['et_env_cvar']>r['em_env_cvar'] for r in rows),'et_env_cvar_still_above_limit':sum(r['et_env_cvar']>1.25 for r in rows),'independent_risk_comparisons':a.checks,'max_absolute_difference':a.max_abs,'rows':rows,'skips':skip}
 (BASE/'analysis.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 t=['# Независимые продолжения в предопределённых точках RISK_EMPTY','',f"Из 24 точек проверены {len(rows)}, пропущены {len(skip)} без замены. В каждой 2048 новых продолжений среды, единых для 8 сохранённых альтернатив. Числа — CVaR потерь семисуточного продолжения, не наблюдённый риск предприятия.",'','Reference знает истинное текущее состояние и расписание события; прогноз ими не располагает. Различие включает оценку состояния, предпосылки восстановления и Монте-Карло. На остатке горизонта действует базовый план, не замкнутая переоптимизация. Это не тождественная проверка вероятностной калибровки.','', '| Профиль, повтор, день (0-based) | EM | ET | CVaR среды u0 | EM | ET |','|---|---|---|---:|---:|---:|']
 for r in rows:t.append(f"| {r['profile']}, {r['repeat']}, {r['day']} | {r['em_id']} | {r['et_id']} | {r['u0_env_cvar']:.6f} | {r['em_env_cvar']:.6f} | {r['et_env_cvar']:.6f} |")
 t+=['','Нет вывода о необходимости выбирать ET везде. Минимум прогнозного риска и минимум риска reference могут не совпадать. Все исключительные предложения всё ещё выше первоначального лимита по используемой прогнозной модели; положительное сравнение reference не меняет их статус задним числом.','',f"Независимый Python-пересчёт риска по сохранённым массивам: {a.checks} сравнений, максимальное абсолютное расхождение {a.max_abs:.3g}. Балансовый reference-оператор унаследован, независимый Python здесь проверяет именно агрегацию риска."]
 (BASE/'RESULTS.md').write_text('\n'.join(t)+'\n');print('\n'.join(t));print({k:v for k,v in report.items() if k not in ('rows','skips')})
if __name__=='__main__':main()
