#!/usr/bin/env python3
"""Run archived R08 configurations via the local API, compare all five files, replay.

Start: python tools/run.py --api-only
Check: python tools/check_r08_api.py --output r08-check.json
Default scope: repeat 0, five profiles, four arms = 20 episodes, not F10/E11.
New jobs are created; existing jobs are not deleted. No external services are used.
"""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
ARMS = ['B0', 'H', 'EM', 'ET']
PROFILES = ['normal', 'S', 'C', 'D', 'DS']

def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', default='http://127.0.0.1:18081')
    parser.add_argument('--repeat', type=int, choices=range(12), default=0)
    parser.add_argument('--profiles', nargs='+', choices=PROFILES, default=PROFILES)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    address = urllib.parse.urlparse(args.base)
    require(address.scheme == 'http' and address.hostname in ('127.0.0.1', 'localhost')
            and not address.username and not address.password and not address.query
            and not address.fragment and address.path in ('', '/'), 'Only the local HTTP API is allowed')
    base = args.base.rstrip('/')
    expected = {e['plan_index']: e for e in json.loads((ROOT/'evidence/r08/archive-summaries.json').read_text(encoding='utf-8'))['episodes']}
    def call(path: str, value: dict | None = None, raw: bool = False):
        body = None if value is None else json.dumps(value).encode('utf-8')
        request = urllib.request.Request(base + path, data=body, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=30) as response:
            data = response.read()
        return data if raw else json.loads(data)
    def wait(job_id: str, replay: bool = False) -> dict:
        deadline = time.monotonic() + 900
        while time.monotonic() < deadline:
            job = call('/api/research/r08/jobs/' + job_id)
            state = job['replay_status'] if replay else job['status']
            if state not in ('queued', 'running'):
                require(state == ('verified' if replay else 'complete'), f'Job {job_id} stopped: {state}')
                return job
            time.sleep(0.25)
        raise RuntimeError(f'Job {job_id} timed out; inspect it in the interface')
    report = {'scope': 'new DEV-R08 jobs compared against supplied archive hashes; not F10/E11',
              'repeat': args.repeat, 'profiles': [], 'episodes': 0, 'files': 0, 'replay_records': 0, 'passed': False}
    try:
        require(call('/health').get('r08_ready') is True, 'R08 engine is not connected')
        for profile in dict.fromkeys(args.profiles):
            created = call('/api/research/r08/experiments', {'profile': profile, 'repeat': args.repeat, 'arms': ARMS})
            job = wait(created['id'])
            require(len(job['episodes']) == 4, 'Expected exactly four completed episodes')
            for episode in job['episodes']:
                ref = expected[episode['plan_index']]
                require(episode['summary'] == ref['summary'], 'Archived summary mismatch')
                require(episode['file_hashes'] == ref['files'], 'Archived primary file hash mismatch')
                for name, sha in ref['files'].items():
                    data = call(f"/api/research/r08/jobs/{job['id']}/episodes/{episode['plan_index']}/files/{name}", raw=True)
                    require(hashlib.sha256(data).hexdigest() == sha, 'Downloaded file hash mismatch: ' + name)
                    report['files'] += 1
                report['episodes'] += 1
            call('/api/research/r08/jobs/' + job['id'] + '/replay', {})
            replay = wait(job['id'], True)
            count = sum(x['replay_records'] for x in replay['episodes'])
            require(count == 240, 'Expected 240 replay records in a four-episode block')
            report['replay_records'] += count
            report['profiles'].append({'profile': profile, 'job_id': job['id']})
            print(f'{profile}: four episodes, 20 matching files, 240 replay records', file=sys.stderr)
        report['passed'] = True
    except Exception as error:
        report['error'] = str(error)
        raise
    finally:
        text = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
        if args.output:
            args.output.write_text(text, encoding='utf-8')
        else:
            print(text, end='')

if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError, urllib.error.HTTPError) as error:
        print('Verification failed:', error, file=sys.stderr)
        raise SystemExit(1)
