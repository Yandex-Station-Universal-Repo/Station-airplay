#!/usr/bin/env python3
"""Offline pack/install regression; requires the real YAM and yam-publish tools."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import tomllib
import unittest

ROOT = Path(__file__).resolve().parent.parent
SPEC = importlib.util.spec_from_file_location('package_luna', ROOT / 'scripts/package-luna.py')
RECIPE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RECIPE)


class LunaPackageTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='airplay-luna-package-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.payload = self.root / 'input'
        self.payload.mkdir()
        self.descriptor = '''id = "station.airplay"
name = "Station airplay"
version = "1.4.15+luna.0.2.14"
entry = "launch-airplay.sh"
permissions = ["exec"]
[launch]
kind = "binary"
mode = "service"
interpreter = "sh"
path = "launch-airplay.sh"
[compatibility]
models = ["yandexmidi"]
'''
        (self.payload / 'plugin.toml').write_text(self.descriptor)
        (self.payload / 'launch-airplay.sh').write_text('#!/bin/sh\nexit 77\n')
        (self.payload / 'launch-airplay.sh').chmod(0o755)
        (self.payload / 'LICENSES').mkdir()
        (self.payload / 'LICENSES/GPL-2.0.txt').write_text('license fixture\n')
        self.work = self.root / 'staged'

    def stage(self, package='airplay/airplay-luna'):
        RECIPE.stage(self.payload, self.work, package, '1.4.16', 'cucumber', '0.2.14')

    def test_rejects_links_without_changing_source(self):
        (self.payload / 'foreign').symlink_to(self.payload / 'plugin.toml')
        with self.assertRaisesRegex(ValueError, 'regular files'):
            self.stage()
        self.assertFalse(self.work.exists())
        self.assertEqual((self.payload / 'plugin.toml').read_text(), self.descriptor)

    def test_rejects_staging_inside_payload(self):
        self.work = self.payload / 'staged'
        with self.assertRaisesRegex(ValueError, 'outside'):
            self.stage()

    def test_does_not_drop_runtime_dependency_aliases(self):
        with (self.payload / 'plugin.toml').open('a') as descriptor:
            descriptor.write('\n[dependencies]\nstation-volume = "station.volume"\n')
        with self.assertRaisesRegex(ValueError, 'dependency aliases'):
            self.stage()

    def test_real_yam_pack_install_and_luna_ownership(self):
        # These binaries are mandatory for the regression: no mock packer and
        # no handmade MANIFEST.yaml that bypasses YAM's namespace validator.
        publisher = Path(os.environ['YAM_PUBLISH']).resolve()
        yam = Path(os.environ['YAM']).resolve()
        profiles = self.root / 'profiles'
        profiles.mkdir()
        install_root = self.root / 'installed'
        profile = {'id': 'cucumber', 'arch': 'aarch64',
                   'triple': 'aarch64-unknown-linux-musl',
                   'paths': {key: str(self.root / key) for key in ('root', 'etc', 'var', 'run')}}
        (profiles / 'cucumber.yaml').write_text(json.dumps(profile))

        def pack(source, output, success=True):
            result = subprocess.run([str(publisher), 'pack', '--source', str(source),
                                     '--profiles', str(profiles), '--platform', 'cucumber',
                                     '--output', str(output)], capture_output=True, text=True)
            self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
            return result

        def manage(*args):
            return subprocess.run([str(yam), '--profiles', str(profiles), '--platform', 'cucumber',
                                   '--root', str(install_root), *args],
                                  check=True, capture_output=True, text=True).stdout

        # Reproduce the exact rejected path, including its license file.
        old = self.root / 'old'
        old.mkdir()
        old_source = {'name': 'airplay/airplay-luna', 'version': '1.4.15',
                      'license': 'GPL-3.0', 'description': 'broken layout', 'targets': ['cucumber'],
                      'files': {'share/luna/plugins/station.airplay/': str(self.payload)}}
        (old / 'yam.yaml').write_text(json.dumps(old_source))
        failure = pack(old, self.root / 'bad.yampkg', success=False)
        self.assertIn('must contain a path component starting with "airplay."', failure.stderr)
        self.assertFalse((self.root / 'bad.yampkg').exists())

        # A metadata-only Luna fixture satisfies the dependency without running
        # any runtime, postinstall hook, network request, or speaker binary.
        host = self.root / 'host'
        host.mkdir()
        (host / 'fixture').write_text('not an executable runtime\n')
        (host / 'yam.yaml').write_text(json.dumps({
            'name': 'yam/luna', 'version': '0.2.14', 'license': 'MIT',
            'description': 'non-executable dependency fixture', 'targets': ['cucumber'],
            'files': {'share/yam.luna/fixture': 'fixture'}}))
        host_archive = self.root / 'host.yampkg'
        pack(host, host_archive)
        manage('install', '--yes', str(host_archive))

        archive = self.root / 'airplay.yampkg'
        subprocess.run([os.sys.executable, str(ROOT / 'scripts/package-luna.py'),
                        '--payload', str(self.payload), '--package', 'airplay/airplay-luna',
                        '--version', '1.4.16', '--platform', 'cucumber', '--luna-version', '0.2.14',
                        '--work-dir', str(self.work), '--publisher', str(publisher),
                        '--profiles', str(profiles), '--output', str(archive)], check=True)
        manage('install', '--yes', str(archive))
        manage('doctor')
        shared = install_root / 'share/luna/plugins/airplay.airplay-luna'
        descriptor = tomllib.loads((shared / 'plugin.toml').read_text())
        namespace, name = descriptor['id'].split('.', 1)
        # This is Luna's pinned_root contract, not just YAM's looser prefix
        # rule: the descriptor must belong to this exact selected package.
        current = (install_root / 'pkgs' / namespace / name / 'current').resolve(strict=True)
        self.assertEqual((shared / 'plugin.toml').resolve(strict=True),
                         current / 'share/luna/plugins' / descriptor['id'] / 'plugin.toml')
        launcher = shared / descriptor['launch']['path']
        self.assertTrue(os.access(launcher, os.X_OK))
        self.assertIn('airplay/airplay-luna',
                      manage('which', 'share/luna/plugins/airplay.airplay-luna/launch-airplay.sh'))
        self.assertEqual((shared / 'LICENSES/GPL-2.0.txt').read_text(), 'license fixture\n')
        self.assertEqual(descriptor, {**tomllib.loads(self.descriptor),
                                     'id': 'airplay.airplay-luna', 'version': '1.4.16'})
        self.assertEqual((self.payload / 'plugin.toml').read_text(), self.descriptor)
        manage('remove', 'airplay/airplay-luna')
        self.assertFalse((shared / 'plugin.toml').exists())
        manage('remove', 'yam/luna')
        manage('doctor')


if __name__ == '__main__':
    unittest.main()
