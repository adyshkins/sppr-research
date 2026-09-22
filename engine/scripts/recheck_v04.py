#!/usr/bin/env python3
"""Recheck the snapshot locally. Writes logs to a NEW directory, not checks/."""
from __future__ import annotations
import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess
import sys


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--race', action='store_true', help='also run Go race tests')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    tag = dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
    out = root / 'verification_reruns' / tag
    out.mkdir(parents=True, exist_ok=False)
    manifest = json.loads((root / 'MANIFEST.json').read_text(encoding='utf-8'))
    commands = [
        [sys.executable, 'scripts/verify_manifest.py'],
        ['go', 'test', './...', '-count=1'],
        ['go', 'vet', './...'],
        ['go', 'build', './...'],
        ['go', 'run', './cmd/corecheck', '-verify', 'checks_v0_1/core_trace.json'],
    ]
    for rel in ['checks_v0_2/monitor_trace.json', 'checks_v0_2/quality_boundary_trace.json']:
        expected = manifest['files'][rel]['sha256']
        commands.append(['go', 'run', './cmd/monitorcheck', '-verify', rel, '-expected-sha256', expected])
    for rel in ['checks_v0_3/analysis_guarded_trace.json', 'checks_v0_3/analysis_aggregate_trace.json']:
        expected = manifest['files'][rel]['sha256']
        commands.append(['go', 'run', './cmd/casecheck', '-verify', rel, '-expected-sha256', expected])
    commands.extend([
        ['go', 'run', './cmd/traincheck', '-verify', 'results/fit_v04'],
        ['go', 'run', './cmd/forecastcheck', '-replay', 'results/forecast_v04/forecast_trace.jsonl.gz'],
        [sys.executable, 'scripts/independent_reference_v04.py', '--out', str(out/'independent_reference.json')],
        [sys.executable, 'scripts/diagnostic_cases_v04.py', '--out', str(out/'diagnostic_cases.json')],
        [sys.executable, 'scripts/dependency_check_v04.py', '--out', str(out/'dependency_check.json')],
    ])
    if args.race:
        commands.append(['go', 'test', '-race', './...', '-count=1'])
    results = []
    for i, command in enumerate(commands):
        logfile = out / f'{i:02d}.txt'
        print(' '.join(command), flush=True)
        try:
            with logfile.open('w', encoding='utf-8') as stream:
                completed = subprocess.run(command, cwd=root, stdout=stream,
                                           stderr=subprocess.STDOUT, timeout=180, check=False)
            code = completed.returncode
        except (OSError, subprocess.TimeoutExpired) as exc:
            logfile.write_text(str(exc) + '\n', encoding='utf-8')
            code = -1
        results.append({'command': command, 'exit_code': code,
                        'log_sha256': hashlib.sha256(logfile.read_bytes()).hexdigest()})
        (out / 'summary.json').write_text(json.dumps(results, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        if code != 0:
            print(f'Check failed; details: {logfile}', file=sys.stderr)
            return 1
    print(f'Checks completed. Logs: {out}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
