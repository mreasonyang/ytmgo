"""Synthetic signing-plan, manifest and credential-boundary regressions."""
import base64
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest.mock import patch

import configure_secrets
import macos


class PackagingTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def app_fixture(self):
        app = self.root / "ytmgo.app"
        resources = app / "Contents/Resources"
        runtime = resources / "runtime"
        runtime.mkdir(parents=True)
        helper = resources / "login/Login.app/Contents/MacOS/helper"
        main = app / "Contents/MacOS/ytmgo"
        for path in (runtime / "ytmgo-playback-host", runtime / "ffmpeg", helper, main):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(bytes.fromhex("cffaedfe") + b"unsigned fixture")
        (runtime / "manifest.json").write_text(json.dumps({
            "sha256": {"ytmgo-playback-host": "before", "ffmpeg": "before"}, "versions": {"ytmgo-playback-host": "fixture"},
        }))
        (resources / "login/manifest.json").write_text(json.dumps({
            "executable": "Login.app/Contents/MacOS/helper", "sha256": "before", "schema": 1,
        }))
        return app, runtime, helper

    def test_signing_updates_both_manifests_after_nested_code_changes(self):
        app, runtime, helper = self.app_fixture()
        original = macos.digest(runtime / "ytmgo-playback-host")
        (runtime / "ytmgo-playback-host").write_bytes(b"synthetic signed bytes")
        helper.write_bytes(b"synthetic signed helper")
        macos.refresh_manifests(app)
        manifest = json.loads((runtime / "manifest.json").read_text())
        self.assertNotEqual(manifest["sha256"]["ytmgo-playback-host"], original)
        self.assertEqual(manifest["sha256"]["ytmgo-playback-host"], macos.digest(runtime / "ytmgo-playback-host"))
        self.assertEqual(manifest["versions"], {"ytmgo-playback-host": "fixture"})
        self.assertNotIn("node_modules_sha256", manifest)
        login = json.loads((helper.parents[3] / "manifest.json").read_text())
        self.assertEqual(login["sha256"], macos.digest(helper))

    def test_manifest_cannot_hash_a_file_outside_the_runtime(self):
        app, runtime, _ = self.app_fixture()
        (runtime / "manifest.json").write_text(json.dumps({"sha256": {"../../../../outside": "before"}}))
        with self.assertRaisesRegex(macos.PackageError, "Invalid runtime manifest entry"):
            macos.refresh_manifests(app)

    def test_inner_code_precedes_bundles_and_manifests_precede_outer_seal(self):
        app, runtime, helper = self.app_fixture()
        calls = []
        def record_sign(path, identity, keychain, entitlements=None):
            calls.append(("sign", path, entitlements))
        with patch.object(macos, "sign", side_effect=record_sign), \
             patch.object(macos, "refresh_manifests", side_effect=lambda path: calls.append(("manifest", path, None))), \
             patch.object(macos, "run", return_value=b""):
            macos.sign_app(app, "fixture", self.root / "keychain")
        helper_index = next(i for i, call in enumerate(calls) if call[1] == helper)
        bundle_index = next(i for i, call in enumerate(calls) if call[1] == helper.parents[2])
        self.assertLess(helper_index, bundle_index)
        self.assertEqual(calls[-2][0], "manifest")
        self.assertEqual(calls[-1], ("sign", app, None))
        for kind, path, entitlements in calls:
            if kind == "sign":
                self.assertIsNone(entitlements)

    def test_secret_decode_is_private_and_invalid_value_is_never_echoed(self):
        with patch.dict(os.environ, {"FIXTURE_SETTING": base64.b64encode(b"synthetic fixture").decode()}):
            destination = self.root / "fixture"
            macos.write_secret("FIXTURE_SETTING", destination)
            self.assertEqual(destination.stat().st_mode & 0o777, 0o600)
        with patch.dict(os.environ, {"FIXTURE_SETTING": "invalid fixture!"}):
            with self.assertRaises(macos.PackageError) as error:
                macos.write_secret("FIXTURE_SETTING", self.root / "invalid")
            self.assertEqual(str(error.exception), "Invalid base64 setting: FIXTURE_SETTING")

    def test_external_tool_failure_does_not_forward_argv_or_diagnostics(self):
        with patch.dict(os.environ, {name: "synthetic fixture" for name in macos.SECRET_NAMES}), \
             patch.object(macos.subprocess, "run") as child:
            child.return_value.returncode = 1
            with self.assertRaisesRegex(macos.PackageError, "^Certificate import failed$"):
                macos.run(["security", "synthetic fixture"], "Certificate import")
            self.assertTrue(all(name not in child.call_args.kwargs["env"] for name in macos.SECRET_NAMES))
            self.assertEqual(child.call_args.kwargs["stderr"], macos.subprocess.DEVNULL)

    def test_wrong_team_identity_is_rejected(self):
        with patch.dict(os.environ, {"APPLE_TEAM_ID": "AAAAAAAAAA", "MACOS_CERTIFICATE_PASSWORD": "fixture"}), \
             patch.object(macos, "write_secret", side_effect=lambda name, path: path.write_bytes(b"fixture")), \
             patch.object(macos, "run", return_value=b'1) ' + b'A' * 40 + b' "Developer ID Application: Fixture (BBBBBBBBBB)"\n'):
            with self.assertRaisesRegex(macos.PackageError, "Expected exactly one valid Developer ID"):
                macos.import_identity(self.root / "private")

    def test_signing_failure_still_cleans_up(self):
        app = self.root / "build/darwin-arm64/ytmgo.app"
        app.mkdir(parents=True)
        (self.root / "mygo.json").write_text(json.dumps({"version": "0.4.0"}))
        with patch.object(macos, "ROOT", self.root), \
             patch.object(macos, "signing_directory", return_value=self.root / "private"), \
             patch.object(macos, "import_identity", side_effect=macos.PackageError("Certificate import failed")), \
             patch.object(macos, "cleanup") as cleanup:
            with self.assertRaises(macos.PackageError):
                macos.package("developer-id")
            cleanup.assert_called_once()

    def test_updater_packages_final_stapled_app_and_uses_stdin(self):
        app = self.root / "build/darwin-arm64/ytmgo.app"
        app.mkdir(parents=True)
        (self.root / "mygo.json").write_text(json.dumps({"version": "0.5.0", "updates": {"publicKey": "fixture"},
                                                       "macos": {"minimumSystemVersion": "27.0"}}))
        events = []
        def command(args, stage, **kwargs):
            events.append(stage)
            if stage == "Signed updater package creation":
                self.assertEqual(kwargs["input_bytes"], b"synthetic update fixture")
                self.assertNotIn("synthetic update fixture", args)
                self.assertIn("App Gatekeeper assessment", events)
                self.assertIn("Disk image Gatekeeper assessment", events)
                for name in ("ytmgo-0.5.0-darwin-arm64.tar.gz", "update-darwin-arm64.json"):
                    (self.root / "dist/macos" / name).write_bytes(b"signed fixture")
            return b""
        with patch.object(macos, "ROOT", self.root), patch.dict(os.environ, {"MYGO_UPDATER_PRIVATE_KEY": "synthetic update fixture"}), \
             patch.object(macos, "signing_directory", return_value=self.root / "private"), \
             patch.object(macos, "import_identity", return_value=("fixture", self.root / "keychain")), \
             patch.object(macos, "sign_app"), patch.object(macos, "sign"), patch.object(macos, "notarize"), \
             patch.object(macos, "staple"), patch.object(macos, "create_disk_image"), \
             patch.object(macos, "run", side_effect=command), patch.object(macos, "digest", return_value="a" * 64), \
             patch.object(macos, "cleanup") as cleanup, patch("builtins.print"):
            macos.package("notarized")
        metadata = json.loads((self.root / "dist/macos/build.json").read_text())
        self.assertTrue(metadata["signed_updates"])
        self.assertIn("update-darwin-arm64.json", (self.root / "dist/macos/SHA256SUMS").read_text())
        cleanup.assert_called_once()

    def test_rejected_notarization_cannot_proceed_to_stapling(self):
        (self.root / "notary-profile-ready").touch()
        with patch.object(macos, "run", return_value=b'{"status":"Invalid","id":"fixture"}'):
            with self.assertRaisesRegex(macos.PackageError, "not accepted"):
                macos.notarize(self.root / "app.zip", self.root)

    def test_notary_password_uses_stdin_and_profile_is_reused(self):
        fixture = {"APPLE_ID": "fixture@example.invalid", "APPLE_TEAM_ID": "AAAAAAAAAA",
                   "APPLE_APP_SPECIFIC_PASSWORD": "synthetic notary fixture"}
        accepted = b'{"status":"Accepted","id":"00000000-0000-4000-8000-000000000001"}'
        with patch.dict(os.environ, fixture), patch.object(macos, "run", return_value=accepted) as child, \
             patch("builtins.print"):
            macos.notarize(self.root / "app.zip", self.root)
            macos.notarize(self.root / "app.dmg", self.root)
        self.assertEqual(child.call_count, 3)
        store = child.call_args_list[0]
        self.assertIn("store-credentials", store.args[0])
        self.assertNotIn("--password", store.args[0])
        self.assertNotIn(fixture["APPLE_APP_SPECIFIC_PASSWORD"], store.args[0])
        self.assertNotIn("--no-validate", store.args[0])
        self.assertEqual(store.kwargs["input_bytes"], b"synthetic notary fixture\n")
        for submit in child.call_args_list[1:]:
            self.assertIn("--keychain-profile", submit.args[0])
            self.assertNotIn("--password", submit.args[0])

    def test_notary_missing_password_fails_before_creating_profile(self):
        fixture = {"APPLE_ID": "fixture@example.invalid", "APPLE_TEAM_ID": "AAAAAAAAAA",
                   "APPLE_APP_SPECIFIC_PASSWORD": ""}
        with patch.dict(os.environ, fixture), patch.object(macos, "run") as child:
            with self.assertRaisesRegex(macos.PackageError, "Missing setting: APPLE_APP_SPECIFIC_PASSWORD"):
                macos.notarize(self.root / "app.zip", self.root)
            child.assert_not_called()
        self.assertFalse((self.root / "notary-profile-ready").exists())

    def test_configuration_uploads_apple_id_and_app_password(self):
        with patch.object(sys, "argv", ["configure_secrets.py", "notarization"]), \
             patch("builtins.input", return_value="fixture@example.invalid"), \
             patch.object(configure_secrets.getpass, "getpass", return_value="synthetic notary fixture"), \
             patch.object(configure_secrets, "upload") as upload:
            self.assertEqual(configure_secrets.main(), 0)
        self.assertEqual([call.args[0] for call in upload.call_args_list],
                         ["APPLE_ID", "APPLE_APP_SPECIFIC_PASSWORD"])

    @unittest.skipUnless(sys.platform == "darwin" and shutil.which("xcrun"), "requires Apple's notarytool")
    def test_native_notary_secure_prompt_accepts_stdin_offline(self):
        # Synthetic credentials in an isolated file keychain. --no-validate
        # makes this an offline I/O-contract test; no Apple login is attempted.
        keychain = self.root / "fixture.keychain-db"
        macos.run(["security", "create-keychain", "-p", "synthetic-keychain-fixture", keychain],
                  "Fixture keychain creation")
        try:
            macos.run(["xcrun", "notarytool", "store-credentials", "ytmgo-fixture",
                       "--apple-id", "fixture@example.invalid", "--team-id", "AAAAAAAAAA",
                       "--keychain", keychain, "--no-validate"], "Offline secure prompt contract",
                      input_bytes=b"synthetic-notary-fixture\n")
        finally:
            macos.run(["security", "delete-keychain", keychain], "Fixture keychain removal")

    def test_secret_upload_uses_stdin_not_argv(self):
        with patch.object(configure_secrets.subprocess, "run") as child:
            child.return_value.returncode = 0
            configure_secrets.upload("FIXTURE_NAME", "synthetic fixture")
            self.assertNotIn("synthetic fixture", child.call_args.args[0])
            self.assertEqual(child.call_args.kwargs["input"], b"synthetic fixture")
            self.assertEqual(child.call_args.kwargs["stderr"], configure_secrets.subprocess.DEVNULL)

    def test_disk_image_errors_expose_only_fixed_categories(self):
        with patch.object(macos.subprocess, "run") as child:
            child.return_value.returncode = 1
            child.return_value.stderr = b"hdiutil: Resource busy; synthetic private fixture path"
            with self.assertRaises(macos.DiskImageError) as error:
                macos.run(["hdiutil", "create", "fixture"], "Disk image creation")
        self.assertEqual(str(error.exception), "Disk image creation failed (resource_busy, exit 1)")
        self.assertNotIn("fixture", str(error.exception))

    def test_disk_image_busy_retries_but_other_failures_do_not(self):
        busy = macos.DiskImageError("Disk image creation", "resource_busy", 1)
        with patch.object(macos, "run", side_effect=[busy, b""]) as child, \
             patch.object(macos.time, "sleep") as sleep:
            macos.disk_image_command(["create", "fixture"], "Disk image creation")
        self.assertEqual(child.call_count, 2)
        sleep.assert_called_once_with(2)
        full = macos.DiskImageError("Disk image creation", "insufficient_space", 1)
        with patch.object(macos, "run", side_effect=full) as child, \
             patch.object(macos.time, "sleep") as sleep:
            with self.assertRaises(macos.DiskImageError):
                macos.disk_image_command(["create", "fixture"], "Disk image creation")
        child.assert_called_once()
        sleep.assert_not_called()

    @unittest.skipUnless(sys.platform == "darwin" and shutil.which("hdiutil"), "requires macOS disk images")
    def test_native_disk_image_preserves_sparse_and_small_fixture_files(self):
        app, runtime, _ = self.app_fixture()
        sparse = runtime / "sparse.fixture"
        with sparse.open("wb") as stream:
            stream.seek(8 * 1024 * 1024)
            stream.write(b"fixture")
        for index in range(256):
            (runtime / ("small-" + str(index) + ".fixture")).write_bytes(b"fixture")
        image = self.root / "fixture.dmg"
        macos.create_disk_image(app, image)
        macos.run(["hdiutil", "verify", image], "Fixture image integrity")
        mount = self.root / "mounted"
        mount.mkdir()
        macos.run(["hdiutil", "attach", "-readonly", "-nobrowse", "-noautoopen", "-mountpoint", mount, image],
                  "Fixture image mount")
        try:
            self.assertEqual(macos.digest(sparse), macos.digest(mount / app.name / "Contents/Resources/runtime/sparse.fixture"))
            self.assertEqual(len(list((mount / app.name / "Contents/Resources/runtime").glob("small-*.fixture"))), 256)
            self.assertEqual((mount / "Applications").readlink(), Path("/Applications"))
        finally:
            macos.disk_image_command(["detach", mount], "Fixture image unmount")


if __name__ == "__main__":
    unittest.main()
