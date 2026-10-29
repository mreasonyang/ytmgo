"""Synthetic signing-plan, manifest and credential-boundary regressions."""
import base64
import json
import os
from pathlib import Path
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
        (runtime / "node_modules/fixture").mkdir(parents=True)
        (runtime / "node_modules/fixture/index.js").write_text("fixture")
        helper = resources / "login/Login.app/Contents/MacOS/helper"
        main = app / "Contents/MacOS/ytmgo"
        for path in (runtime / "node", runtime / "ffmpeg", helper, main):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(bytes.fromhex("cffaedfe") + b"unsigned fixture")
        (runtime / "manifest.json").write_text(json.dumps({
            "sha256": {"node": "before", "ffmpeg": "before"}, "versions": {"node": "fixture"},
        }))
        (resources / "login/manifest.json").write_text(json.dumps({
            "executable": "Login.app/Contents/MacOS/helper", "sha256": "before", "schema": 1,
        }))
        return app, runtime, helper

    def test_signing_updates_both_manifests_after_nested_code_changes(self):
        app, runtime, helper = self.app_fixture()
        original = macos.digest(runtime / "node")
        (runtime / "node").write_bytes(b"synthetic signed bytes")
        helper.write_bytes(b"synthetic signed helper")
        macos.refresh_manifests(app)
        manifest = json.loads((runtime / "manifest.json").read_text())
        self.assertNotEqual(manifest["sha256"]["node"], original)
        self.assertEqual(manifest["sha256"]["node"], macos.digest(runtime / "node"))
        self.assertEqual(manifest["versions"], {"node": "fixture"})
        self.assertEqual(len(manifest["node_modules_sha256"]), 64)
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
                self.assertEqual(entitlements is not None, path == runtime / "node")

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

    def test_rejected_notarization_cannot_proceed_to_stapling(self):
        with patch.dict(os.environ, {"APPLE_API_KEY_ID": "fixture", "APPLE_API_ISSUER_ID": "fixture"}), \
             patch.object(macos, "write_secret"), \
             patch.object(macos, "run", return_value=b'{"status":"Invalid","id":"fixture"}'):
            with self.assertRaisesRegex(macos.PackageError, "not accepted"):
                macos.notarize(self.root / "app.zip", self.root)

    def test_secret_upload_uses_stdin_not_argv(self):
        with patch.object(configure_secrets.subprocess, "run") as child:
            child.return_value.returncode = 0
            configure_secrets.upload("FIXTURE_NAME", "synthetic fixture")
            self.assertNotIn("synthetic fixture", child.call_args.args[0])
            self.assertEqual(child.call_args.kwargs["input"], b"synthetic fixture")
            self.assertEqual(child.call_args.kwargs["stderr"], configure_secrets.subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
