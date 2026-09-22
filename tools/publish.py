#!/usr/bin/env python3
"""Publish this verified source bundle under the repository owner's identity.

Dry check: python tools/publish.py
Publish and rename: python tools/publish.py --apply --rename sppr-research
The token is read from a hidden prompt (or GH_TOKEN); it is never saved.
No force push, deletion of old history or deletion of the frontend repo.
"""
from __future__ import annotations
import argparse
import getpass
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
OWNER = 'adyshkins'
REPO_ID = 1243562059
EXPECTED_HEAD = '897857750855167be9690bad89a375ba4d821fec'
AUTHOR = {'name': 'Адышкин Сергей Сергеевич', 'email': 'adyshkinss@gmail.com'}
API = 'https://api.github.com'
SECRET = re.compile(rb'(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}|-----BEGIN (?:RSA |OPENSSH |EC )?PRIVATE KEY-----)')

def source_files() -> list[dict]:
    manifest = json.loads((ROOT / 'release-files.json').read_text(encoding='utf-8'))
    files = []
    for name, expected in manifest['files'].items():
        rel = Path(name)
        if rel.is_absolute() or '..' in rel.parts:
            raise RuntimeError('Unsafe release manifest path')
        p = ROOT / rel
        if p.is_symlink() or not p.is_file():
            raise RuntimeError(f'Missing or non-regular file: {name}')
        data = p.read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f'Release file changed: {name}')
        if SECRET.search(data):
            raise RuntimeError(f'Potential credential detected in {name}; nothing will be published')
        files.append({'path': name, 'mode': '100644', 'type': 'blob', 'content': data.decode('utf-8')})
    # This manifest is a release descriptor, not a self-referential hash claim.
    files.append({'path': 'release-files.json', 'mode': '100644', 'type': 'blob',
                  'content': (ROOT / 'release-files.json').read_text(encoding='utf-8')})
    return files

class Client:
    def __init__(self, token: str):
        self.token = token
    def call(self, method: str, path: str, payload: dict | None = None) -> dict:
        if not path.startswith('/') or path.startswith('//'):
            raise RuntimeError('Invalid GitHub API path')
        data = None if payload is None else json.dumps(payload, ensure_ascii=False).encode('utf-8')
        request = urllib.request.Request(API + path, data=data, method=method, headers={
            'Authorization': 'Bearer ' + self.token, 'Accept': 'application/vnd.github+json',
            'X-GitHub-Api-Version': '2022-11-28', 'User-Agent': 'sppr-research-publisher',
            'Content-Type': 'application/json',
        })
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                return json.load(response)
        except urllib.error.HTTPError as e:
            # Never include the request or headers in an error message.
            try:
                message = json.loads(e.read()).get('message', 'GitHub request failed')
            except (ValueError, UnicodeError):
                message = 'GitHub request failed'
            raise RuntimeError(f'{method} {path}: HTTP {e.code}: {message}') from None
        except urllib.error.URLError as e:
            raise RuntimeError('GitHub is unreachable; no automatic retry of writes. Check the repository before retrying.') from None

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default='adyshkins/sppr-prototype-back')
    parser.add_argument('--apply', action='store_true', help='authorize writes; otherwise only inspect')
    parser.add_argument('--rename', help='optional new repository name, e.g. sppr-research')
    args = parser.parse_args()
    if not re.fullmatch(r'adyshkins/[A-Za-z0-9_.-]+', args.repo):
        parser.error('Only the explicitly authorized owner adyshkins is supported')
    if args.rename and not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]{0,99}', args.rename):
        parser.error('Invalid repository name')
    files = source_files()
    print(f'Checked {len(files)} source files. Runtime data and node_modules are not included.')
    token = os.environ.get('GH_TOKEN') or getpass.getpass('GitHub token (hidden; do not paste it in a chat): ')
    if not token.strip():
        raise RuntimeError('No token supplied')
    client = Client(token.strip())
    profile = client.call('GET', '/user')
    if profile.get('login') != OWNER:
        raise RuntimeError('Authenticated account must be adyshkins')
    repo = client.call('GET', '/repos/' + args.repo)
    if repo.get('id') != REPO_ID or repo.get('owner', {}).get('login') != OWNER:
        raise RuntimeError('Repository ID/owner differs from the inspected repository; no writes')
    name = repo['full_name']
    branch = repo['default_branch']
    if branch != 'main':
        raise RuntimeError('Default branch changed; review before publishing')
    base = '/repos/' + name
    ref = client.call('GET', base + '/git/ref/heads/main')
    head = ref['object']['sha']
    if head != EXPECTED_HEAD:
        raise RuntimeError('main changed since the verified baseline. Merge the new changes before publishing; no files were overwritten.')
    print(f'Target: {name}, main={head}. New author and committer: {AUTHOR["name"]} <{AUTHOR["email"]}>')
    if not args.apply:
        print('Read-only check completed. No GitHub changes. Add --apply to publish; --rename is optional.')
        return
    backup = 'backup/pre-monorepo-' + head[:12]
    # Resolve backup once. A missing ref is handled distinctly from permission errors.
    try:
        old = client.call('GET', base + '/git/ref/heads/' + backup)
    except RuntimeError as e:
        if 'HTTP 404:' not in str(e):
            raise
        old = client.call('POST', base + '/git/refs', {'ref': 'refs/heads/' + backup, 'sha': head})
    if old['object']['sha'] != head:
        raise RuntimeError('Backup branch points elsewhere; no main update')
    tree = client.call('POST', base + '/git/trees', {'tree': files})
    commit = client.call('POST', base + '/git/commits', {
        'message': 'Unify research workbench and R08 engine', 'tree': tree['sha'], 'parents': [head],
        'author': AUTHOR, 'committer': AUTHOR,
    })
    for role in ('author', 'committer'):
        if any(commit.get(role, {}).get(k) != v for k, v in AUTHOR.items()):
            raise RuntimeError('Commit identity mismatch; main has not been updated')
    if client.call('GET', base + '/git/ref/heads/main')['object']['sha'] != head:
        raise RuntimeError('main moved during preparation; the candidate commit was not published to main')
    updated = client.call('PATCH', base + '/git/refs/heads/main', {'sha': commit['sha'], 'force': False})
    if updated['object']['sha'] != commit['sha']:
        raise RuntimeError('Unexpected ref response; check GitHub before doing anything else')
    verified = client.call('GET', base + '/git/ref/heads/main')
    if verified['object']['sha'] != commit['sha']:
        raise RuntimeError('Could not verify the published head; check GitHub')
    print('Published:', repo['html_url'] + '/commit/' + commit['sha'])
    if args.rename and args.rename != repo['name']:
        try:
            changed = client.call('PATCH', base, {'name': args.rename})
            if changed.get('id') != REPO_ID or changed.get('name') != args.rename:
                raise RuntimeError('Unexpected rename response')
            print('Renamed:', changed['html_url'])
        except RuntimeError:
            print('Code was published successfully, but rename was not confirmed. Rename the repository in GitHub Settings; do not repeat the source publication.', file=sys.stderr)
            raise
    print('Original history and backup branch retained. The frontend repository was not changed.')

if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError) as e:
        print('Publication stopped:', str(e), file=sys.stderr)
        raise SystemExit(1)
