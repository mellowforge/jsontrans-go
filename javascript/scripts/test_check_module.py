"""Regression for a PR's detached checkout with no advertised local branches."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

import check_module as check


class DetachedCheckout(unittest.TestCase):
    def test_packaging_uses_only_current_dependencies_from_cold_cache(self):
        original = os.environ.copy()
        goroot = Path(check.run(['go', 'env', 'GOROOT'], cwd=check.ROOT, env=original, capture=True).strip())
        old_cache = Path(check.run(['go', 'env', 'GOMODCACHE'], cwd=check.ROOT, env=original, capture=True).strip())
        # Populate exactly the currently selected module versions, as a fresh CI
        # runner does. Do not copy historical versions from a developer cache.
        stream = check.run(['go', 'list', '-m', '-json', 'all'], cwd=check.ROOT / 'javascript', env=original, capture=True)
        modules = []
        decoder = json.JSONDecoder()
        while stream.strip():
            value, end = decoder.raw_decode(stream.lstrip())
            modules.append(value)
            stream = stream.lstrip()[end:]
        with tempfile.TemporaryDirectory(prefix='jsontrans-current-deps-') as directory:
            cache = Path(directory) / 'cache'
            for module in modules:
                if not module.get('Version') or module.get('Replace'):
                    continue
                # Go's module download cache escapes uppercase letters.
                path = ''.join('!' + c.lower() if c.isupper() else c for c in module['Path'])
                source = old_cache / 'cache/download' / path / '@v'
                target = cache / 'cache/download' / path / '@v'
                target.mkdir(parents=True, exist_ok=True)
                for item in source.glob(module['Version'] + '.*'):
                    if item.is_file() and not item.name.endswith(('.lock', '.tmp')):
                        shutil.copyfile(item, target / item.name)
            env = original | dict(GOTOOLCHAIN='local', GOMODCACHE=str(cache),
                                  GOPROXY='off', GOSUMDB='off',
                                  PATH=str(goroot / 'bin') + os.pathsep + original['PATH'])
            result = subprocess.run([sys.executable, str(check.ROOT / 'javascript/scripts/check_module.py')],
                                    cwd=check.ROOT, env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_minimum_core_resolves_without_local_branches(self):
        go = str(Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip()) / 'bin' / 'go')
        manifest = (check.ROOT / 'javascript/go.mod').read_text()
        minimum = re.search(r'github.com/mellowforge/jsontrans-go\s+(\S+)', manifest).group(1)
        # Keep the original pseudo-version regression as well as testing the
        # current minimum, which may be a stable tag in a detached CI checkout.
        versions = dict.fromkeys([minimum, 'v1.1.1-0.20261103173822-e9b9c0c71fe0'])
        for version in versions:
            with self.subTest(version=version), tempfile.TemporaryDirectory(prefix='jsontrans-detached-check-') as directory:
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
