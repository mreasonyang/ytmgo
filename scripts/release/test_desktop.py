import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import desktop


class DesktopReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.commit = "a" * 40
        self.tag = "v0.6.3"
        self.version = "0.6.3"
        self.metadata = {"version": self.version, "commit": self.commit, "notarized": True}
        self.data = json.dumps(self.metadata).encode()
        self.remote = {"tag_name": self.tag, "draft": False, "prerelease": False, "target_commitish": self.commit,
                       "assets": [{"name": "build.json", "digest": "sha256:" + hashlib.sha256(self.data).hexdigest(),
                                   "browser_download_url": "https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/build.json"}]}

    def resolve(self, expected=""):
        with patch.object(desktop, "gh", return_value=json.dumps(self.remote).encode()), \
                patch.object(desktop, "public", return_value=self.data):
            return desktop.resolve(self.tag, expected)

    def test_resolve_binds_release_to_verified_source(self):
        self.assertEqual(self.resolve()["source_sha"], self.commit)
        self.assertEqual(self.resolve(self.commit)["enabled"], "true")
        self.assertEqual(self.resolve("b" * 40)["enabled"], "false")
        self.remote["assets"][0]["digest"] = "sha256:" + "b" * 64
        with self.assertRaises(desktop.DesktopError):
            self.resolve()

    def test_resolve_rejects_drafts_prereleases_and_tag_injection(self):
        for field in ("draft", "prerelease"):
            self.remote[field] = True
            with self.subTest(field=field), self.assertRaises(desktop.DesktopError):
                self.resolve()
            self.remote[field] = False
        for tag in ("../main", "v0.6.003", "v0.6.1000", "v0.6.3-rc.1"):
            with self.subTest(tag=tag), patch.object(desktop, "gh") as gh, self.assertRaises(desktop.DesktopError):
                desktop.resolve(tag)
            gh.assert_not_called()

    def test_already_complete_release_is_idempotent(self):
        for which in desktop.PLATFORMS:
            self.remote["assets"] += [{"name": name} for name in desktop.asset_names(self.version, which, True)]
        self.assertEqual(self.resolve()["enabled"], "false")

    def fixture_artifacts(self):
        for which in desktop.PLATFORMS:
            names = desktop.asset_names(self.version, which, True)
            for name in names:
                (self.root / name).write_bytes(b"synthetic public artifact")
            metadata = {"version": self.version, "commit": self.commit, "platform": which,
                        "native_build": True, "native_smoke": True, "signed_updates": True}
            (self.root / ("build-" + which + ".json")).write_text(json.dumps(metadata))
            (self.root / ("SHA256SUMS-" + which)).write_text("".join(
                desktop.digest(self.root / name) + "  " + name + "\n" for name in names if not name.startswith("SHA256SUMS")))
        return {"release_tag": self.tag, "source_sha": self.commit}

    def test_platform_assets_require_matching_source_and_complete_checksums(self):
        release = self.fixture_artifacts()
        self.assertEqual(len(desktop.validate(self.root, release)), 11)
        with self.assertRaises(desktop.DesktopError):
            desktop.validate(self.root, dict(release, source_sha="b" * 40))
        archive = self.root / "ytmgo-0.6.3-linux-amd64.tar.gz"
        archive.write_bytes(b"changed")
        with self.assertRaises(desktop.DesktopError):
            desktop.validate(self.root, release)

    def test_checksum_paths_cannot_escape_release_directory(self):
        release = self.fixture_artifacts()
        with (self.root / "SHA256SUMS-linux-amd64").open("a") as stream:
            stream.write("a" * 64 + "  ../outside\n")
        with self.assertRaises(desktop.DesktopError):
            desktop.validate(self.root, release)

    def test_existing_assets_are_never_overwritten_or_partially_uploaded_on_collision(self):
        release = self.fixture_artifacts()
        files = desktop.validate(self.root, release)
        before = {"assets": [{"name": files[-1].name, "digest": "sha256:" + "b" * 64}], "body": "public release"}
        with patch.object(desktop, "resolve", return_value=release), patch.object(desktop, "run"), \
                patch.object(desktop, "gh", return_value=json.dumps(before).encode()) as gh:
            with self.assertRaisesRegex(desktop.DesktopError, "existing_release_asset_differs"):
                desktop.publish(self.root, self.tag)
        self.assertFalse(any(call.args[0][:2] == ["release", "upload"] for call in gh.call_args_list))

    def test_incomplete_bundle_and_unexpected_symlink_are_rejected(self):
        with self.assertRaises(desktop.DesktopError):
            desktop.entries(self.root, self.version, "linux-amd64")
        (self.root / "outside").write_text("fixture")
        try:
            (self.root / "link").symlink_to(self.root / "outside")
        except OSError:
            self.skipTest("symlink unavailable")
        with self.assertRaisesRegex(desktop.DesktopError, "symlink"):
            desktop.entries(self.root, self.version, "linux-amd64")

    def test_child_diagnostics_are_not_forwarded(self):
        with patch.object(desktop.subprocess, "run") as child:
            child.return_value.returncode = 1
            child.return_value.stdout = b"synthetic private diagnostic"
            with self.assertRaisesRegex(desktop.DesktopError, "^desktop_command_failed$"):
                desktop.run(["fixture"])
        self.assertEqual(child.call_args.kwargs["stderr"], desktop.subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
