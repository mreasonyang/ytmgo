import json
from pathlib import Path
import stat
import tempfile
import unittest
import zipfile

import macos
import sign_development as signing


class SigningInputTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.commit = "a" * 40
        self.name = "ytmgo-0.0.65-dev-" + self.commit[:12] + "-darwin-arm64.zip"
        self.receipt = {"architecture": "arm64", "archive": self.name, "artifact_only": True,
                        "build": {"commit": self.commit, "branch": "feature/ui", "version": "0.0.65",
                                  "kind": "development", "updates_enabled": False, "dirty": False}}
        self.archive("ytmGO.app/Contents/Info.plist")

    def archive(self, name, symlink=None):
        with zipfile.ZipFile(self.root / self.name, "w") as z:
            entry = zipfile.ZipInfo(name)
            if symlink is not None:
                entry.external_attr = (stat.S_IFLNK | 0o777) << 16
            z.writestr(entry, symlink or "fixture")
        self.receipt["archive_sha256"] = macos.digest(self.root / self.name)
        self.save()

    def save(self):
        (self.root / "build.json").write_text(json.dumps(self.receipt))

    def verify(self):
        return signing.validate_input(self.root, self.commit, "feature/ui", "0.0.65")

    def test_source_bound_archive_is_accepted(self):
        self.assertEqual(self.verify(), ("arm64", self.root / self.name))

    def test_tampered_archive_is_rejected_before_signing(self):
        with (self.root / self.name).open("ab") as f:
            f.write(b"changed")
        with self.assertRaisesRegex(macos.PackageError, "digest"):
            self.verify()

    def test_release_updates_or_different_source_are_rejected(self):
        for key, value in (("updates_enabled", True), ("kind", "release"), ("commit", "b" * 40)):
            with self.subTest(key=key):
                original = self.receipt["build"][key]
                self.receipt["build"][key] = value
                self.save()
                with self.assertRaises(macos.PackageError):
                    self.verify()
                self.receipt["build"][key] = original

    def test_archive_cannot_write_outside_the_app(self):
        for name in ("../outside", "/outside", "ytmGO.app/../../outside", "other.app/file"):
            with self.subTest(name=name):
                self.archive(name)
                with self.assertRaisesRegex(macos.PackageError, "path"):
                    self.verify()

    def test_symlink_cannot_escape_the_app(self):
        for target in ("/tmp/outside", "../../../outside"):
            with self.subTest(target=target):
                self.archive("ytmGO.app/Contents/link", target)
                with self.assertRaisesRegex(macos.PackageError, "symlink"):
                    self.verify()

    def test_internal_framework_symlink_is_accepted(self):
        self.archive("ytmGO.app/Contents/Frameworks/Fixture.framework/Versions/Current", "A")
        self.verify()
