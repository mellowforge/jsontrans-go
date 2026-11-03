"""Regression for a PR's detached checkout with no advertised local branches."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import check_module as check


class DetachedCheckout(unittest.TestCase):
    def test_minimum_core_resolves_without_local_branches(self):
        go = str(Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip()) / 'bin' / 'go')
        version = 'v1.1.1-0.20261103173822-e9b9c0c71fe0'
        with tempfile.TemporaryDirectory(prefix='jsontrans-detached-check-') as directory:
            work = Path(directory)
            source = work / 'detached.git'
            check.run(['git', 'clone', '--mirror', '--no-hardlinks', str(check.ROOT), str(source)], cwd=work, env=os.environ.copy(), capture=True)
            head = check.run(['git', '--git-dir', str(source), 'rev-parse', 'HEAD'], cwd=work, env=os.environ.copy(), capture=True).strip()
            check.run(['git', '--git-dir', str(source), 'update-ref', '--no-deref', 'HEAD', head], cwd=work, env=os.environ.copy())
            refs = check.run(['git', '--git-dir', str(source), 'for-each-ref', '--format=%(refname)', 'refs/heads/'], cwd=work, env=os.environ.copy(), capture=True).splitlines()
            for ref in refs:
                check.run(['git', '--git-dir', str(source), 'update-ref', '-d', ref], cwd=work, env=os.environ.copy())
            env = os.environ.copy() | dict(GOWORK='off', GOTOOLCHAIN='local', GOMODCACHE=str(work / 'cache'), GOSUMDB='off', GOPRIVATE=check.CORE)
            check.download_core(go, version, work, env, source)
            self.assertTrue((work / 'cache/cache/download' / check.CORE / '@v' / (version + '.zip')).is_file())
            after = check.run(['git', '--git-dir', str(source), 'for-each-ref', '--format=%(refname)', 'refs/heads/'], cwd=work, env=os.environ.copy(), capture=True)
            self.assertEqual(after, '', 'checker mutated caller repository refs')


if __name__ == '__main__':
    unittest.main()
