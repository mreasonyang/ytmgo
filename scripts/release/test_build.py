"""Regressions for local packages using the production updater accidentally."""
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import build
from build_config import release_config


class BuildTests(unittest.TestCase):
    def test_checked_in_default_has_no_update_source(self):
        config = json.loads((Path(__file__).resolve().parents[2] / "mygo.json").read_text())
        # Release jobs configure their own checkout before running these tests.
        if os.environ.get("GITHUB_ACTIONS") != "true":
            self.assertNotIn("updates", config)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.base = {"version": "0.0.64", "macos": {"minimumSystemVersion": "13.0"}}
        (self.root / "mygo.json").write_text(json.dumps(self.base))

    def git(self, *args):
        if args == ("rev-parse", "HEAD"): return "a" * 40
        if args == ("rev-parse", "--abbrev-ref", "HEAD"): return "feature/ui"
        if args == ("show", "HEAD:mygo.json"): return json.dumps(self.base)
        if args[0] == "status": return ""
        self.fail("unexpected git operation")

    def test_development_clears_feed_even_with_saved_release_config_and_custom_flags(self):
        (self.root / "mygo.json").write_text(json.dumps(release_config(self.base, "0.0.65")))
        with patch.object(build, "ROOT", self.root), patch.object(build, "git", side_effect=self.git), \
                patch.dict(os.environ, {"MYGO_LDFLAGS": "synthetic inherited flags", "MYGO_UPDATER_PRIVATE_KEY": "synthetic fixture"}):
            env = build.build_environment("development", None)
        self.assertNotIn("MYGO_UPDATER_PRIVATE_KEY", env)
        self.assertNotIn("synthetic", env["MYGO_LDFLAGS"])
        self.assertIn("mygo.packageUpdateFeed=", env["MYGO_LDFLAGS"])
        self.assertIn("main.buildKind=development", env["MYGO_LDFLAGS"])

    def test_release_requires_explicit_version_matching_exact_source_configuration(self):
        with patch.object(build, "ROOT", self.root), patch.object(build, "git", side_effect=self.git), \
                patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError): build.build_environment("release", None)
            with self.assertRaises(ValueError): build.build_environment("release", "0.0.65")
            cfg = release_config(self.base, "0.0.65")
            (self.root / "mygo.json").write_text(json.dumps(cfg))
            env = build.build_environment("release", "0.0.65")
            self.assertIn("main.buildKind=release", env["MYGO_LDFLAGS"])
            self.assertNotIn("mygo.packageUpdateFeed=", env["MYGO_LDFLAGS"])
            cfg["updates"]["github"] = "fixture/wrong-source"
            (self.root / "mygo.json").write_text(json.dumps(cfg))
            with self.assertRaises(ValueError): build.build_environment("release", "0.0.65")

    def test_release_refuses_dirty_source(self):
        (self.root / "mygo.json").write_text(json.dumps(release_config(self.base, "0.0.65")))
        def dirty_git(*args):
            return " M main.go" if args[0] == "status" else self.git(*args)
        with patch.object(build, "ROOT", self.root), patch.object(build, "git", side_effect=dirty_git), \
                patch.dict(os.environ, {}, clear=True):
            with self.assertRaisesRegex(ValueError, "clean source"): build.build_environment("release", "0.0.65")

    def test_prerelease_retains_semver_but_uses_numeric_apple_fields(self):
        config = release_config(self.base, "0.0.65-rc.1", "123.2")
        self.assertEqual(config["version"], "0.0.65-rc.1")
        self.assertEqual(config["macos"]["infoPlist"]["CFBundleShortVersionString"], "0.0.65")
        self.assertEqual(config["macos"]["infoPlist"]["CFBundleVersion"], "123.2")
        self.assertNotIn("updates", self.base)

    def test_missing_release_version_never_uses_source_baseline(self):
        with self.assertRaisesRegex(ValueError, "Explicit RELEASE_VERSION"):
            release_config(self.base, None)
