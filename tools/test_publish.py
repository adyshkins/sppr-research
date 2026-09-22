"""Offline publication tests with local bare repositories; no GitHub calls."""
import json
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import publish


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='sppr-publish-test-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'clone'
        self.remote = self.root / 'remote.git'
        self.env = os.environ.copy()
        for key in list(self.env):
            if key.startswith('GIT_'):
                self.env.pop(key, None)
        self.env.update({'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': os.devnull})
        self.env_patch = patch.dict(os.environ, self.env, clear=True)
        self.env_patch.start()
        self.addCleanup(self.env_patch.stop)
        self.run_git(self.root, 'init', '--bare', '--initial-branch=main', str(self.remote))
        self.run_git(self.root, 'init', '--initial-branch=main', str(self.repo))
        self.run_git(self.repo, 'config', 'user.name', 'AdSS')
        self.run_git(self.repo, 'config', 'user.email', 'adyshkinss@gmail.com')
        self.run_git(self.repo, 'config', 'commit.gpgsign', 'false')
        self.run_git(self.repo, 'remote', 'add', 'origin', str(self.remote))
        self.commit('Initial', 'initial')
        self.run_git(self.repo, 'push', 'origin', 'main')
        self.base = self.head(self.remote)
        self.commit('Update', 'updated')
        self.tip = self.head(self.repo)
        self.allowed = frozenset({str(self.remote)})

    def run_git(self, cwd, *args):
        p = subprocess.run(['git', *args], cwd=cwd, env=self.env, check=True,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return p.stdout.decode('utf-8').strip()

    def commit(self, message, content):
        (self.repo / 'data.txt').write_text(content, encoding='utf-8')
        self.run_git(self.repo, 'add', 'data.txt')
        self.run_git(self.repo, 'commit', '-m', message)

    def head(self, root):
        return self.run_git(root, 'rev-parse', 'refs/heads/main')

    def invoke(self, apply=False):
        return publish.publish(self.repo, apply, allowed=self.allowed)

    def test_dry_run_has_no_remote_writes(self):
        self.assertEqual(self.invoke()['status'], 'dry-run')
        self.assertEqual(self.head(self.remote), self.base)

    def test_fast_forward_and_identity_are_preserved(self):
        self.assertEqual(self.invoke(True)['status'], 'published')
        self.assertEqual(self.head(self.remote), self.tip)
        self.assertEqual(self.run_git(self.remote, 'rev-parse', 'main^'), self.base)
        self.assertEqual(self.run_git(self.remote, 'show', '-s', '--format=%an:%cn', 'main'), 'AdSS:AdSS')

    def test_repeated_push_is_noop(self):
        self.invoke(True)
        self.assertEqual(self.invoke(True)['status'], 'up-to-date')

    def test_dirty_worktree_is_refused(self):
        (self.repo / 'data.txt').write_text('uncommitted')
        with self.assertRaises(RuntimeError): self.invoke(True)
        self.assertEqual(self.head(self.remote), self.base)

    def test_untracked_file_is_refused(self):
        (self.repo / 'extra.txt').write_text('untracked')
        with self.assertRaises(RuntimeError): self.invoke(True)

    def test_wrong_branch_is_refused(self):
        self.run_git(self.repo, 'switch', '-c', 'other')
        with self.assertRaises(RuntimeError): self.invoke(True)

    def test_detached_head_is_refused(self):
        self.run_git(self.repo, 'checkout', '--detach', 'HEAD')
        with self.assertRaises(RuntimeError): self.invoke(True)

    def test_old_repository_target_is_refused_without_network(self):
        self.run_git(self.repo, 'remote', 'set-url', 'origin',
                     'https://github.com/adyshkins/sppr-prototype-back.git')
        with self.assertRaises(RuntimeError): self.invoke(True)

    def test_other_push_url_is_refused(self):
        self.run_git(self.repo, 'remote', 'set-url', '--push', 'origin', str(self.root / 'other.git'))
        with self.assertRaises(RuntimeError): self.invoke(True)

    def test_unapproved_identity_is_refused(self):
        self.run_git(self.repo, 'config', 'user.email', 'other@example.test')
        self.commit('Other identity', 'third')
        with self.assertRaises(RuntimeError): self.invoke(True)
        self.assertEqual(self.head(self.remote), self.base)

    def test_all_owner_names_are_allowed(self):
        for index, name in enumerate(sorted(publish.AUTHOR_NAMES)):
            self.run_git(self.repo, 'config', 'user.name', name)
            self.commit('Owner change', str(index))
        self.assertEqual(self.invoke(True)['status'], 'published')

    def test_coauthor_trailer_is_refused(self):
        self.commit('Update\n\nCo-authored-by: Other <other@example.test>', 'third')
        with self.assertRaises(RuntimeError): self.invoke(True)

    def test_secret_in_intermediate_snapshot_is_refused(self):
        self.commit('Intermediate', 'gh' + 'p_' + 'A' * 36)
        self.commit('Remove intermediate value', 'safe again')
        with self.assertRaises(RuntimeError): self.invoke(True)
        self.assertEqual(self.head(self.remote), self.base)

    def test_secret_in_commit_message_is_refused(self):
        self.commit('gh' + 'p_' + 'A' * 36, 'safe')
        with self.assertRaises(RuntimeError): self.invoke(True)

    def competitor(self):
        other = self.root / 'competing'
        self.run_git(self.root, 'clone', str(self.remote), str(other))
        self.run_git(other, 'config', 'user.name', 'AdSS')
        self.run_git(other, 'config', 'user.email', 'adyshkinss@gmail.com')
        (other / 'other.txt').write_text('new remote work')
        self.run_git(other, 'add', 'other.txt')
        self.run_git(other, '-c', 'commit.gpgsign=false', 'commit', '-m', 'Concurrent update')
        self.run_git(other, 'push', 'origin', 'main')
        return self.head(other)

    def test_diverged_remote_is_not_overwritten(self):
        rival = self.competitor()
        with self.assertRaises(RuntimeError): self.invoke(True)
        self.assertEqual(self.head(self.remote), rival)

    def test_concurrent_advance_at_push_is_not_overwritten(self):
        real_git = publish.git
        rival = []
        def racing_git(root, *args):
            if args[0] == 'push': rival.append(self.competitor())
            return real_git(root, *args)
        with patch.object(publish, 'git', racing_git):
            with self.assertRaises(RuntimeError): self.invoke(True)
        self.assertEqual(self.head(self.remote), rival[0])


if __name__ == '__main__':
    unittest.main()
