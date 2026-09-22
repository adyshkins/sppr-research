#!/usr/bin/env python3
"""Build and run the local workbench. Source engine and plan are verified first.

python tools/run.py --install       # install frontend dependencies, build, run
python tools/run.py                 # rebuild Go and frontend, then run
python tools/run.py --api-only      # Go API only, no Node.js required
python tools/run.py --build-only    # build without starting a server
"""
from __future__ import annotations
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys
from verify_sources import verify
ROOT = Path(__file__).resolve().parents[1]

def run(command: list[str], cwd: Path) -> None:
    print('>', ' '.join(command), flush=True)
    subprocess.run(command, cwd=cwd, check=True)

def main() -> None:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--install', action='store_true')
    p.add_argument('--api-only', action='store_true')
    p.add_argument('--build-only', action='store_true')
    p.add_argument('--port', type=int, default=18081)
    args = p.parse_args()
    if not 1024 <= args.port <= 65535:
        p.error('port must be between 1024 and 65535')
    verify()
    go = shutil.which('go')
    if not go:
        raise RuntimeError('Go is missing from PATH. Install Go; the archived engine was checked with Go 1.23.2.')
    ext = '.exe' if os.name == 'nt' else ''
    (ROOT / 'bin').mkdir(exist_ok=True)
    engine = ROOT / 'bin' / ('recoverycheck' + ext)
    api = ROOT / 'bin' / ('research-api' + ext)
    run([go, 'build', '-o', str(engine), './cmd/recoverycheck'], ROOT / 'engine')
    run([go, 'build', '-o', str(api), './cmd/research-api'], ROOT / 'backend')
    static = ROOT / 'frontend/dist'
    if not args.api_only:
        npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
        if not npm:
            raise RuntimeError('Node.js/npm are missing. Install Node.js 22.12+ or use --api-only.')
        if args.install or not (ROOT / 'frontend/node_modules').exists():
            run([npm, 'ci'], ROOT / 'frontend')
        # Build every time so source changes cannot be hidden by stale assets.
        run([npm, 'run', 'build'], ROOT / 'frontend')
    if args.build_only:
        print('Build completed. No server has been started.')
        return
    addr = f'127.0.0.1:{args.port}'
    cmd = [str(api), '-addr', addr, '-data', str(ROOT / 'data'), '-engine', str(engine),
           '-plan', str(ROOT / 'engine/results/recovery_v08/plan.json'), '-origins',
           f'http://127.0.0.1:{args.port},http://localhost:{args.port},http://localhost:5173,http://127.0.0.1:5173']
    if not args.api_only:
        cmd += ['-static', str(static)]
    print(f'Open http://{addr}' + ('/health (API only)' if args.api_only else ''), flush=True)
    print('Stop with Ctrl+C. Source results are never overwritten.', flush=True)
    run(cmd, ROOT)

if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print('\nStopped.')
    except (OSError, RuntimeError, subprocess.CalledProcessError) as e:
        print(f'Cannot start: {e}', file=sys.stderr)
        raise SystemExit(1)
