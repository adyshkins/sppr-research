"""Rebuild numerical article tables from the NEW C12 raw CSV files.
No F10/E11 numbers are imported. Bootstrap units are independent seed blocks.
"""
from __future__ import annotations
import argparse, json, hashlib
from pathlib import Path
import numpy as np
import pandas as pd

def main() -> None:
    ap=argparse.ArgumentParser(); ap.add_argument('--input',default='results/c12_final'); ap.add_argument('--out',default='results/analysis_final');a=ap.parse_args()
    src,out=Path(a.input),Path(a.out);out.mkdir(parents=True,exist_ok=True)
    run_summary=json.loads((src/'summary.json').read_text())
    assert run_summary['mismatches']==0 and run_summary['episodes']==2880 and run_summary['reviews']==83520
    d=pd.read_csv(src/'reviews.csv',float_precision='round_trip')
    e=pd.read_csv(src/'episodes.csv',float_precision='round_trip')
    assert len(e)==2880 and len(d)==83520, 'This analysis expects the locked main C12 design.'
    assert e.seed.nunique()==32 and not d.duplicated(['seed','profile','delay','lambda','filter','day']).any()
    assert set(d['lambda'])=={0.0,.35,1.0}
    assert (d.full_queries==512).all()
    modes=['cost','natural','tail']
    for mode in modes: assert (d[f'{mode}_queries']<=d.full_queries).all()
    # Independently check totals between daily and episode files.
    grouped=d.groupby(['seed','profile','delay','lambda','filter'])
    for mode in ['full']+modes:
        sums=grouped[f'{mode}_queries'].sum().sort_index()
        expected=e.set_index(['seed','profile','delay','lambda','filter'])[f'{mode}_queries'].sort_index()
        assert sums.equals(expected)
    primary=e[(e['lambda']==.35)&(e['filter']==1)&(e.profile!='normal')].copy()
    assert len(primary)==384
    primary['tail_full']=1-primary.tail_queries/primary.full_queries
    primary['tail_cost']=1-primary.tail_queries/primary.cost_queries
    primary['tail_natural']=1-primary.tail_queries/primary.natural_queries
    primary['total_tail_full']=1-(primary.proposal_queries+primary.tail_queries)/(primary.proposal_queries+primary.full_queries)
    contrast_names=['tail_full','tail_cost','tail_natural','total_tail_full']
    blocks=primary.groupby('seed')[contrast_names].mean().to_numpy()
    assert blocks.shape==(32,4)
    rng=np.random.default_rng(880012); ix=rng.integers(0,32,size=(20000,32))
    boot=blocks[ix].mean(axis=1)*100; point=blocks.mean(axis=0)*100
    contrasts=[]
    for j,name in enumerate(contrast_names):
        lo,hi=np.quantile(boot[:,j],[.00625,.99375])
        contrasts.append(dict(contrast=name,mean_pct=float(point[j]),lo_pct=float(lo),hi_pct=float(hi),blocks=32))
    pd.DataFrame(contrasts).to_csv(out/'primary_contrasts.csv',index=False)
    tables=[]
    for (lam,fil),v in d.groupby(['lambda','filter']):
        rec=dict(lam=float(lam),filter=int(fil),reviews=len(v),full_mean=512,
                 cost_mean=float(v.cost_queries.mean()),natural_mean=float(v.natural_queries.mean()),tail_mean=float(v.tail_queries.mean()),
                 saved_vs_full=100*(1-v.tail_queries.sum()/v.full_queries.sum()),saved_vs_cost=100*(1-v.tail_queries.sum()/v.cost_queries.sum()),
                 selector_speed=v.full_ns.sum()/v.tail_ns.sum(),with_common_speed=(v.full_ns.sum()+v.common_ns.sum())/(v.tail_ns.sum()+v.common_ns.sum()),empty=int((v.reason=='RISK_EMPTY').sum()))
        tables.append(rec)
    pd.DataFrame(tables).to_csv(out/'criterion_grid.csv',index=False)
    by_profile=[]
    for name,v in d.groupby('profile'):
        ep=e[e.profile==name]
        by_profile.append(dict(profile=name,reviews=len(v),saved_vs_full=100*(1-v.tail_queries.sum()/v.full_queries.sum()),
                               saved_vs_cost=100*(1-v.tail_queries.sum()/v.cost_queries.sum()),
                               selector_speed=v.full_ns.sum()/v.tail_ns.sum(),
                               with_common_speed=(v.full_ns.sum()+v.common_ns.sum())/(v.tail_ns.sum()+v.common_ns.sum()),
                               including_proposals_speed=(ep.proposal_ns.sum()+ep.review_common_ns.sum()+ep.review_full_ns.sum())/(ep.proposal_ns.sum()+ep.review_common_ns.sum()+ep.review_tail_ns.sum())))
    pd.DataFrame(by_profile).to_csv(out/'by_profile.csv',index=False)
    # Secondary: the pre-specified range of possible physical shock dates,
    # not an assertion that every day in this window is disturbed.
    shock=d[(d.profile!='normal')&(d.day>=17)&(d.day<=30)]
    validation=d[d.validation_risk.notna()].copy()
    vr=[]
    for name,v in validation.groupby('profile'):
        vr.append(dict(profile=name,admitted=len(v),exceed=int((v.validation_risk>1.25).sum()),
                       exceed_pct=float(100*(v.validation_risk>1.25).mean()),max_risk=float(v.validation_risk.max())))
    pd.DataFrame(vr).to_csv(out/'independent_validation.csv',index=False)
    summary=dict(episodes=len(e),physical_days=int(len(e)*60),reviews=len(d),selector_comparisons=int(len(d)*3),mismatches=run_summary['mismatches'],
                 full_queries=int(d.full_queries.sum()),cost_queries=int(d.cost_queries.sum()),natural_queries=int(d.natural_queries.sum()),tail_queries=int(d.tail_queries.sum()),
                 proposal_queries=int(e.proposal_queries.sum()),risk_empty=int((d.reason=='RISK_EMPTY').sum()),
                 pooled_saved_vs_full=100*(1-d.tail_queries.sum()/d.full_queries.sum()),pooled_saved_vs_cost=100*(1-d.tail_queries.sum()/d.cost_queries.sum()),
                 pooled_selector_speed=float(d.full_ns.sum()/d.tail_ns.sum()),pooled_with_common_speed=float((d.full_ns.sum()+d.common_ns.sum())/(d.tail_ns.sum()+d.common_ns.sum())),
                 pooled_including_proposals_speed=float((e.proposal_ns.sum()+e.review_common_ns.sum()+e.review_full_ns.sum())/(e.proposal_ns.sum()+e.review_common_ns.sum()+e.review_tail_ns.sum())),
                 pooled_total_queries_saved=100*(1-(e.proposal_queries.sum()+d.tail_queries.sum())/(e.proposal_queries.sum()+d.full_queries.sum())),
                 tail_worse_than_natural_cases=int((d.tail_queries>d.natural_queries).sum()),
                 tail_worse_than_natural_pct=float(100*(d.tail_queries>d.natural_queries).mean()),
                 shock_window_reviews=len(shock),shock_window_saved_vs_full=100*(1-shock.tail_queries.sum()/shock.full_queries.sum()),
                 shock_window_saved_vs_cost=100*(1-shock.tail_queries.sum()/shock.cost_queries.sum()),
                 validation_admitted=len(validation),validation_exceed=int((validation.validation_risk>1.25).sum()),
                 validation_exceed_pct=float(100*(validation.validation_risk>1.25).mean()),validation_max_risk=float(validation.validation_risk.max()),
                 validation_queries=int(d.validation_queries.sum()),contrasts=contrasts,grid=tables,by_profile=by_profile)
    (out/'article_numbers.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf-8')
    # Canonical deterministic records exclude all machine timing fields.
    deterministic=d[[c for c in d if not c.endswith('_ns')]].to_csv(index=False,float_format='%.17g')
    (out/'deterministic_reviews.sha256').write_text(hashlib.sha256(deterministic.encode()).hexdigest()+'\n')
    print(json.dumps(summary,ensure_ascii=False,indent=2))
if __name__=='__main__':main()
