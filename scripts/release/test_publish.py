"""Release gates against incomplete, mismatched or unsigned artifacts."""
import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import publish


class ReleaseTests(unittest.TestCase):
    def test_patch_versions_use_zero_to_999_without_padding(self):
        for version in ("0.6.0", "0.6.1", "0.6.9", "0.6.10", "0.6.99", "0.6.100", "0.6.999", "0.6.999-rc.1"):
            with self.subTest(version=version), patch.object(publish, "ROOT", self.root), \
                    patch.dict(os.environ, {"PUBLISH_RELEASE": "false", "RELEASE_VERSION": version}), patch("builtins.print"):
                (self.root / "mygo.json").write_text(json.dumps(self.config))
                publish.configure()
                self.assertEqual(json.loads((self.root / "mygo.json").read_text())["version"], version)
        for version in ("0.6.00", "0.6.001", "0.6.010", "0.6.1000", "0.6.-1", "0.6.1-rc.01"):
            with self.subTest(version=version), patch.object(publish, "ROOT", self.root), \
                    patch.dict(os.environ, {"PUBLISH_RELEASE": "false", "RELEASE_VERSION": version}):
                (self.root / "mygo.json").write_text(json.dumps(self.config))
                with self.assertRaises(publish.ReleaseError):
                    publish.configure()
                self.assertEqual(json.loads((self.root / "mygo.json").read_text()), self.config)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.commit = "a" * 40
        self.config = {"version": "0.5.0"}
        self.directory = self.root / "dist/macos"
        self.directory.mkdir(parents=True)
        self.meta = {"version": "0.5.0", "commit": self.commit, "mode": "notarized", "architecture": "arm64",
                     "notarized": True, "developer_id_signed": True, "signed_updates": True}
        self.names = ["ytmgo-0.5.0-arm64-notarized.dmg", "ytmgo-0.5.0-arm64-notarized.zip",
                      "ytmgo-0.5.0-darwin-arm64.tar.gz", "update-darwin-arm64.json", "build.json"]
        for name in self.names[:3]:
            (self.directory / name).write_bytes(b"public artifact fixture")
        (self.directory / "update-darwin-arm64.json").write_text(json.dumps({"version": "0.5.0", "size": 23,
            "url": "https://github.com/mreasonyang/ytmgo/releases/download/v0.5.0/ytmgo-0.5.0-darwin-arm64.tar.gz"}))
        self.update_metadata()

    def update_metadata(self):
        (self.directory / "build.json").write_text(json.dumps(self.meta))
        (self.directory / "SHA256SUMS").write_text("".join(
            hashlib.sha256((self.directory / name).read_bytes()).hexdigest() + "  " + name + "\n" for name in self.names))

    def test_release_requires_source_notarization_and_update_signature(self):
        with patch.object(publish, "ROOT", self.root), patch.object(publish.subprocess, "run") as verifier:
            verifier.return_value.returncode = 0
            self.assertEqual(len(publish.validate(self.directory, self.config, self.commit)), 6)
            verifier.return_value.returncode = 1
            with self.assertRaisesRegex(publish.ReleaseError, "signature verification"):
                publish.validate(self.directory, self.config, self.commit)
            for key, value in (("notarized", False), ("signed_updates", False), ("commit", "b" * 40)):
                original = self.meta[key]
                self.meta[key] = value
                self.update_metadata()
                with self.assertRaisesRegex(publish.ReleaseError, "matching notarized"):
                    publish.validate(self.directory, self.config, self.commit)
                self.meta[key] = original

    def test_changed_artifact_or_injected_checksum_cannot_publish(self):
        with (self.directory / self.names[0]).open("ab") as stream:
            stream.write(b"tampered")
        with self.assertRaisesRegex(publish.ReleaseError, "checksum mismatch"):
            publish.validate(self.directory, self.config, self.commit)
        with (self.directory / "SHA256SUMS").open("a") as stream:
            stream.write("a" * 64 + "  ../../private\n")
        with self.assertRaisesRegex(publish.ReleaseError, "Invalid release checksums"):
            publish.validate(self.directory, self.config, self.commit)

    def test_configuration_rejects_unsafe_versions_and_unsigned_publication(self):
        (self.root / "mygo.json").write_text(json.dumps(self.config))
        with patch.object(publish, "ROOT", self.root), patch.dict(os.environ, {"PUBLISH_RELEASE": "true", "PACKAGE_MODE": "unsigned"}):
            with self.assertRaisesRegex(publish.ReleaseError, "requires notarized"):
                publish.configure()
        with patch.object(publish, "ROOT", self.root), patch.dict(os.environ, {"PUBLISH_RELEASE": "false", "RELEASE_VERSION": "../bad"}):
            with self.assertRaises(publish.ReleaseError):
                publish.configure()
        with patch.object(publish, "ROOT", self.root), patch.dict(os.environ, {"PUBLISH_RELEASE": "false", "RELEASE_VERSION": "0.5.0-rc.1"}), patch("builtins.print"):
            publish.configure()
        self.assertEqual(json.loads((self.root / "mygo.json").read_text())["version"], "0.5.0-rc.1")

    def test_never_overwrites_existing_release(self):
        (self.root / "mygo.json").write_text(json.dumps(self.config))
        with patch.object(publish, "ROOT", self.root), patch.dict(os.environ, {
                "GITHUB_REF": "refs/heads/main", "GITHUB_REPOSITORY": publish.REPOSITORY, "GITHUB_SHA": self.commit}), \
                patch.object(publish, "validate", return_value=[]), patch.object(publish, "gh", return_value=b'{"id": 1}') as gh:
            with self.assertRaisesRegex(publish.ReleaseError, "already exists"):
                publish.publish()
        gh.assert_called_once()

    def test_failed_asset_verification_leaves_draft_unpublished(self):
        (self.root / "mygo.json").write_text(json.dumps(self.config))
        with patch.object(publish, "ROOT", self.root), patch.dict(os.environ, {
                "GITHUB_REF": "refs/heads/main", "GITHUB_REPOSITORY": publish.REPOSITORY, "GITHUB_SHA": self.commit}), \
                patch.object(publish, "validate", return_value=[self.directory / self.names[0]]), \
                patch.object(publish, "gh", side_effect=[None, b'[]', b'', b'',
                    b'{"apiUrl":"https://api.github.com/repos/mreasonyang/ytmgo/releases/123"}',
                    b'{"draft":true,"assets":[]}']) as gh:
            with self.assertRaisesRegex(publish.ReleaseError, "asset set mismatch"):
                publish.publish()
        self.assertFalse(any(call.args[0][:2] == ["release", "edit"] for call in gh.call_args_list))
        self.assertIn(["api", "https://api.github.com/repos/mreasonyang/ytmgo/releases/123"], [call.args[0] for call in gh.call_args_list])


if __name__ == "__main__":
    unittest.main()
