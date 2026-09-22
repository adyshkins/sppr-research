#!/usr/bin/env python3
"""Post-hoc descriptive mechanism audit; NOT new independent trials."""
import gzip,json,pathlib
ROOT=pathlib.Path(__file__).resolve().parents[1]
def load(p):return json.loads(pathlib.Path(p).read_text())
def stream(p):
 with gzip.open(p,'rt',encoding='utf8') as f:
  for l in f:yield json.loads(l)
def main():
 suite=ROOT/'results/factorial_v06';jobs=load(suite/'plan.json');counts={};example=None
 for i,j in enumerate(jobs):
  folder=suite/'episodes'/f'episode_{i:03d}';cc=list(stream(folder/'compact_controller.jsonl.gz'));pp=list(stream(folder/'physical_evaluation.jsonl.gz'));key=(j['profile'],j['policy'])
  s=counts.setdefault(key,{'forecast_cases':0,'all_three_features_false':0,'features_false_after_event':0,'true_kpi_in_soft_ranges':0,'risk_empty':0,'risk_empty_also_excludes_u0':0,'risk_empty_followed_by_unchanged_plan_next_day':0,'risk_empty_on_terminal_day':0})
  for r,p in zip(cc,pp):
   if r['forecast_status']=='ready':
    s['forecast_cases']+=1;d=r['analysis']['diagnosis'];false=len(d['features'])==3 and all(v['value'] is False for v in d['features']);s['all_three_features_false']+=false;s['features_false_after_event']+=false and not p['tape_evaluation_only']['introduced_cause_active']
    k=p['physical_result_evaluation_only']['kpi'];s['true_kpi_in_soft_ranges']+=all(lo<=v<=hi for v,lo,hi in zip(k,(.95,.95,.7,3),(1,1,.9,7)))
   if r['selection']['reason']=='RISK_EMPTY':
    s['risk_empty']+=1;u0=next(v for v in r['selection']['decision']['evaluations'] if v['id']=='u0');s['risk_empty_also_excludes_u0']+=u0['risk_admissible'] is False
    if r['day']==len(cc)-1:s['risk_empty_on_terminal_day']+=1
    else:
     nxt=cc[r['day']+1];s['risk_empty_followed_by_unchanged_plan_next_day']+=(nxt['execution']['authorized_action']['id']=='u0' and nxt['execution']['requested_assignment'] is None)
   if i==52 and r['day']==25:
    ref=load(suite/'references'/'S_r01_d25'/'summary.json');example={'notice':'Illustration selected after inspection from a checkpoint that was prespecified; no new trial and no retuning.','source_job':i,'source_record_hash':r['hash'],'closed_day_zero_based':25,'calendar_day_one_based':26,'introduced_event_still_active':p['tape_evaluation_only']['introduced_cause_active'],'true_kpi':p['physical_result_evaluation_only']['kpi'],'observed_kpi':r['analysis']['monitor']['kpi'],'monitor_signals':r['analysis']['monitor']['signals'],'diagnostic_features':r['analysis']['diagnosis']['features'],'leading_hypothesis':r['analysis']['diagnosis']['leading_id'],'forecast_weights':r['analysis']['diagnosis']['forecast_weights'],'fallback_templates':j['config']['controller']['pipeline']['forecast']['templates'],'prediction_recovery_probability':.25,'R00_action':r['selection']['decision']['recommendation_id'],'same_information_choices':r['same_information_choices'],'risk_u0_pred':ref['predictor_risk']['u0'],'risk_u0_env_ref':ref['environment_reference_risk']['u0']}
 out={'notice':'Post-hoc mechanism accounting, not additional independent replications. Empty risk-admissible set does not certify continuation of the old plan.','by_profile_policy':[{'profile':p,'policy':m,**v} for (p,m),v in counts.items()],'representative_case':example}
 (suite/'analysis'/'mechanism_audit.json').write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({'risk_empty':sum(x['risk_empty'] for x in counts.values()),'u0_also_excluded':sum(x['risk_empty_also_excludes_u0'] for x in counts.values()),'continued_next_day':sum(x['risk_empty_followed_by_unchanged_plan_next_day'] for x in counts.values()),'terminal':sum(x['risk_empty_on_terminal_day'] for x in counts.values())},indent=2))
if __name__=='__main__':main()
