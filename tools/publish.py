#!/usr/bin/env python3
"""Push existing main commits to sppr-research without replacing its history.

Read-only remote check: python tools/publish.py
Publish committed work: python tools/publish.py --apply
Git handles authentication. This script never asks for an access token, creates
commits, changes repository names, or uses force push.
"""
from __future__ import annotations

import argparse
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
TARGETS = frozenset({
    'https://github.com/adyshkins/sppr-research.git',
    'git@github.com:adyshkins/sppr-research.git',
    'ssh://git@github.com/adyshkins/sppr-research.git',
})
AUTHOR_NAMES = frozenset({'AdSS', 'adyshkins', 'Адышкин Сергей Сергеевич'})
AUTHOR_EMAILS = frozenset({
    'adyshkinss@gmail.com',
    '56836526+adyshkins@users.noreply.github.com',
    'adyshkins@users.noreply.github.com',
})
SECRET = re.compile(rb'(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}|-----BEGIN (?:RSA |OPENSSH |EC )?PRIVATE KEY-----)')


def git(root: Path, *args: str) -> bytes:
    """Run without a shell, with normal Git credential helpers and hooks."""
    env = os.environ.copy()
    for key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR'):
        env.pop(key, None)
    try:
        result = subprocess.run(['git', *args], cwd=root, env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=180, check=False)
    except FileNotFoundError:
        raise RuntimeError('Git is not installed or is not on PATH') from None
    except subprocess.TimeoutExpired:
        raise RuntimeError('Git timed out. Check the remote state before retrying.') from None
    if result.returncode:
        # Do not expose URLs, credentials or private paths from Git stderr.
        raise RuntimeError(f'Git {args[0]} failed (exit {result.returncode}). '
                           'No force retry was attempted; inspect Git locally.')
    return result.stdout


def text(root: Path, *args: str) -> str:
    return git(root, *args).decode('utf-8').strip()


def check_target(root: Path, allowed: frozenset[str]) -> None:
    for args in (('remote', 'get-url', '--all', 'origin'),
                 ('remote', 'get-url', '--push', '--all', 'origin')):
        urls = text(root, *args).splitlines()
        if len(urls) != 1 or urls[0] not in allowed:
            raise RuntimeError('origin must point only to adyshkins/sppr-research; '
                               'no remote changes were made.')


def check_outgoing(root: Path, base: str, head: str) -> int:
    """Check every unpublished commit, including files later removed again."""
    commits = text(root, 'rev-list', base + '..' + head).splitlines()
    checked_blobs: set[str] = set()
    for commit in commits:
        fields = text(root, 'show', '-s', '--format=%an%x00%ae%x00%cn%x00%ce%x00%B',
                      commit).split('\0', 4)
        if (len(fields) != 5 or fields[0] not in AUTHOR_NAMES
                or fields[2] not in AUTHOR_NAMES or fields[1] not in AUTHOR_EMAILS
                or fields[3] not in AUTHOR_EMAILS):
            raise RuntimeError('Outgoing author/committer is not an approved owner identity.')
        if re.search(r'(?im)^\s*co-authored-by\s*:', fields[4]):
            raise RuntimeError('Review additional co-author trailers before publication.')
        if SECRET.search(fields[4].encode('utf-8')):
            raise RuntimeError('Potential credential in an outgoing commit message.')
        # Inspect complete outgoing snapshots, not only the final diff. The
        # pattern check is deliberately limited; it is not a full secret audit.
        for entry in git(root, 'ls-tree', '-r', '-z', '--full-tree', commit).split(b'\0'):
            if not entry:
                continue
            metadata, _ = entry.split(b'\t', 1)
            mode, kind, oid = metadata.decode('ascii').split()
            if kind != 'blob' or mode not in ('100644', '100755'):
                raise RuntimeError('Review non-regular files or submodules before publication.')
            if oid not in checked_blobs:
                if SECRET.search(git(root, 'cat-file', 'blob', oid)):
                    raise RuntimeError('Potential credential in an outgoing file snapshot.')
                checked_blobs.add(oid)
    return len(commits)


def publish(root: Path, apply: bool, *, allowed: frozenset[str] = TARGETS) -> dict:
    """Update an existing main. `allowed` is injectable for offline unit tests."""
    root = root.resolve()
    if Path(text(root, 'rev-parse', '--show-toplevel')).resolve() != root:
        raise RuntimeError('Run from the project clone, not a nested Git repository.')
    if text(root, 'symbolic-ref', '--quiet', '--short', 'HEAD') != 'main':
        raise RuntimeError('Only the local main branch can be published by this helper.')
    if text(root, 'status', '--porcelain', '--untracked-files=normal'):
        raise RuntimeError('Commit or move local changes before publishing; the worktree is not clean.')
    check_target(root, allowed)
    head = text(root, 'rev-parse', 'HEAD')
    # Fetch changes local metadata only. Dry run never modifies the remote.
    git(root, 'fetch', '--no-tags', 'origin', 'refs/heads/main')
    base = text(root, 'rev-parse', 'FETCH_HEAD')
    if base == head:
        return {'status': 'up-to-date', 'commit': head, 'commits': 0}
    git(root, 'merge-base', '--is-ancestor', base, head)
    count = check_outgoing(root, base, head)
    if not apply:
        return {'status': 'dry-run', 'commit': head, 'commits': count}
    # Recheck local state and destination; a concurrent remote advance is also
    # protected by normal Git non-fast-forward rejection at the push itself.
    check_target(root, allowed)
    if text(root, 'rev-parse', 'HEAD') != head or text(root, 'status', '--porcelain'):
        raise RuntimeError('Local state changed during verification. Nothing was pushed.')
    git(root, 'push', '--no-follow-tags', 'origin', head + ':refs/heads/main')
    refs = text(root, 'ls-remote', '--refs', 'origin', 'refs/heads/main').splitlines()
    if len(refs) != 1 or refs[0].split('\t')[0] != head:
        raise RuntimeError('Push was not confirmed by the remote. Check GitHub before retrying.')
    return {'status': 'published', 'commit': head, 'commits': count}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true', help='push verified, already committed changes')
    args = parser.parse_args()
    result = publish(ROOT, args.apply)
    print(f"{result['status']}: {result['commit']} ({result['commits']} outgoing commits)")
    if result['status'] == 'dry-run':
        print('Remote unchanged. Add --apply to send these existing commits.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, RuntimeError) as error:
        print('Publication stopped:', error, file=sys.stderr)
        raise SystemExit(1)
