#!/usr/bin/env python3
"""Collect measurements from executed checks, without inventing run outcomes."""
from __future__ import annotations
import argparse, hashlib, json, platform, subprocess
from pathlib import Path

def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()

def main() -> int:
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--source',type=Path,help='optional original DOCX, read only')
    p.add_argument('--out',type=Path,required=True)
    a=p.parse_args(); r=Path(__file__).resolve().parents[1]
    old=json.loads((r/'MANIFEST_v0_3.json').read_text())['files']
    oldgo={k:v for k,v in old.items() if k.endswith('.go')}
    changed=[k for k,v in oldgo.items() if sha(r/k)!=v['sha256']]
    tests=[json.loads(v) for v in (r/'checks/go_test.jsonl').read_text().splitlines()]
    passed={(v['Package'],v['Test']) for v in tests if v.get('Action')=='pass' and v.get('Test')}
    failures=[v for v in tests if v.get('Action')=='fail']
    commands=[json.loads(v) for v in (r/'checks/commands_v04.jsonl').read_text().splitlines()]
    badlogs=[v['output'] for v in commands if sha(r/v['output'])!=v['sha256']]
    fit=json.loads((r/'results/fit_v04/summary.json').read_text())
    ref=json.loads((r/'checks/independent_reference_v04.json').read_text())
    deps=json.loads((r/'checks/dependency_check_v04.json').read_text())
    result={'version':'0.4.0','evidence':'new synthetic fit and engineering forecast verification; not closed-loop effectiveness',
        'original_source_sha256':sha(a.source) if a.source else None,
        'source_was_not_modified': sha(a.source)=='a6593b885aeef9b0323cbb6a650ac501f1a8c4ad457434d8bd872cf542d00fcf' if a.source else None,
        'baseline_manifest_files':len(old),'old_go_files_checked':len(oldgo),'changed_old_go_files':changed,
        'test_functions_passed':sum('/' not in v[1] for v in passed),'subtests_passed':sum('/' in v[1] for v in passed),
        'new_test_functions_vs_v03':sum('/' not in v[1] for v in passed)-149,'failed_test_events':len(failures),
        'recorded_commands':len(commands),'nonzero_exit_commands':[v for v in commands if v['exit_code']!=0],
        'changed_command_logs':badlogs,
        'training_episodes':fit['training_episodes'],'training_rows':fit['training_days'],
        'retained_examples':fit['retained_training_examples'],'validation_episodes':fit['validation_episodes'],
        'validation_rows':fit['validation_days'],'validation_counts':fit['development_validation_counts'],
        'forecast_preview':json.loads((r/'results/forecast_v04/summary.json').read_text()),
        'independent_arithmetic':{'passed':ref['passed'],'numeric_comparisons':ref['numeric_comparisons'],
            'max_abs_error':ref['maximum_absolute_error'],'forecast_counts':ref['forecasts']},
        'inference_dependencies_passed':deps['passed'],
        'environment':{'go':subprocess.check_output(['go','version'],text=True).strip(),
            'go_platform':subprocess.check_output(['go','env','GOOS','GOARCH','CGO_ENABLED'],text=True).splitlines(),
            'python':platform.python_version(),'platform':platform.system()},
    }
    result['passed']=not changed and not failures and not badlogs and all(v['exit_code']==0 for v in commands) and ref['passed'] and deps['passed']
    a.out.parent.mkdir(parents=True,exist_ok=True); a.out.write_text(json.dumps(result,indent=2,ensure_ascii=False)+'\n')
    print(json.dumps({k:result[k] for k in ['passed','test_functions_passed','subtests_passed','new_test_functions_vs_v03','old_go_files_checked','environment']},ensure_ascii=False))
    return 0 if result['passed'] else 1

if __name__=='__main__': raise SystemExit(main())
