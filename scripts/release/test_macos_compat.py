import json
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch

import macos
import macos_compat
import publish


class CompatibilityTests(unittest.TestCase):
    def test_newer_embedded_binary_and_wrong_architecture_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            app = root / 'App.app'
            (app / 'Contents').mkdir(parents=True)
            (root / 'mygo.json').write_text(json.dumps({'version': '0.6.4'}))
            (app / 'Contents/Info.plist').write_bytes(plistlib.dumps({
                'CFBundleShortVersionString': '0.6.4', 'LSMinimumSystemVersion': '13.0'}))
            binary = app / 'Contents/helper'
            with patch.object(macos, 'ROOT', root), patch.object(macos, 'macho_files', return_value=[binary]*4), \
                 patch.object(macos, 'run', side_effect=[b'x86_64', b' minos 27.0\n']):
                with self.assertRaisesRegex(macos.PackageError, 'requires newer'):
                    macos_compat.inspect(app, 'amd64', '13.0')
            with patch.object(macos, 'ROOT', root), patch.object(macos, 'macho_files', return_value=[binary]*4), \
                 patch.object(macos, 'run', return_value=b'arm64'):
                with self.assertRaisesRegex(macos.PackageError, 'architecture mismatch'):
                    macos_compat.inspect(app, 'amd64', '13.0')

    def test_receipt_is_bound_to_exact_signed_archive(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'dist/receipts').mkdir(parents=True)
            config = {'version': '0.6.4', 'macos': {'minimumSystemVersion': '13.0'}}
            for arch in ('arm64', 'amd64'):
                archive = root / ('ytmgo-0.6.4-' + arch + '-notarized.zip')
                archive.write_bytes(b'synthetic signed bundle')
                (root / 'dist/receipts' / ('signed-' + arch + '.json')).write_text(json.dumps({
                    'architecture': arch, 'version': '0.6.4', 'commit': 'a'*40, 'native_smoke': True,
                    'signed': True, 'minimum_macos': '13.0', 'archive_sha256': publish.sha256(archive)}))
            with patch.object(publish, 'ROOT', root):
                publish.validate_smoke(root, config, 'a'*40)
                archive.write_bytes(b'swapped bundle')
                with self.assertRaises(publish.ReleaseError):
                    publish.validate_smoke(root, config, 'a'*40)
