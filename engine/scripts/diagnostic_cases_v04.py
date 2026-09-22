#!/usr/bin/env python3
"""Extract declared-label graph exclusions; never alters model or data."""
from __future__ import annotations
import argparse, gzip, json
from pathlib import Path

def main() -> int:
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--out',type=Path,required=True)
    args=ap.parse_args()
    root=Path(__file__).resolve().parents[1]
    cases=[]
    with gzip.open(root/'results/fit_v04/validation_audit.jsonl.gz','rt',encoding='utf-8') as f:
        for line in f:
            x=json.loads(line)
            if not x['eligible_for_conditional_accuracy']:
                continue
            s=x['synthetic_source']; a=x['analysis']; d=a['diagnosis']; m=a['monitor']
            label=s['experiment_label_not_inference_input']
            if label in d['candidate_ids']:
                continue
            cases.append({'id':s['id'],'calendar_day':s['calendar_day_one_based'],
                'introduced_label':label,'true_kpi':s['true_kpi_evaluation_only'],
                'observed_kpi':m['kpi'],'signals':m['signals'],
                'candidates':d['candidate_ids'],'leader':d['leading_id'],
                'confidence':d['confidence'],'status':d['status'],
                'ranking':d['ranked']})
    result={'evidence':'new synthetic development validation; introduced label is not proven cause of every monitor signal',
            'count':len(cases),'cases':cases}
    args.out.parent.mkdir(parents=True,exist_ok=True)
    args.out.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(f'Extracted {len(cases)} graph exclusions, without refitting or editing the graph.')
    return 0

if __name__=='__main__':
    raise SystemExit(main())
