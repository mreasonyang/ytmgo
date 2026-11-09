"""Reject release-enabled or unidentified apps before creating dev artifacts."""
import copy
import json
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch

import verify_development as dev


class DevelopmentArtifactTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.app = Path(self.temp.name) / "ytmGO.app"
        (self.app / "Contents").mkdir(parents=True)
        (self.app / "Contents/Info.plist").write_bytes(plistlib.dumps({
            "CFBundleShortVersionString": "0.0.65", "CFBundleVersion": "0.0.65"}))
        self.commit = "a" * 40
        self.branch = "feature/ui"
        self.config = {"version": "0.0.65"}
        self.info = {"version": "0.0.65", "build_number": "0.0.65", "kind": "development",
                     "commit": self.commit, "branch": self.branch, "dirty": False, "updates_enabled": False}

    def fake_run(self, args):
        if args[-1] == "--build-info":
            return 0, json.dumps(self.info).encode()
        if args[0] == "codesign":
            return 0, b""
        return 1, b'{"enabled": false, "error": "updates_disabled"}'

    def verify(self):
        return dev.identity(self.app, self.config, self.commit, self.branch)

    def test_verified_dev_app_checks_both_update_entry_points(self):
        with patch.object(dev, "run", side_effect=self.fake_run) as run:
            self.assertEqual(self.verify(), self.info)
        self.assertEqual([call.args[0][-1] for call in run.call_args_list][-2:],
                         ["--check-for-updates", "--install-update"])

    def test_release_kind_update_capability_dirty_or_wrong_source_cannot_pass(self):
        original = copy.deepcopy(self.info)
        for key, value in (("kind", "release"), ("updates_enabled", True), ("dirty", True),
                           ("commit", "b"*40), ("branch", "main"), ("version", "0.0.64"),
                           ("build_number", "999")):
            with self.subTest(key=key):
                self.info = dict(original, **{key: value})
                with patch.object(dev, "run", side_effect=self.fake_run), self.assertRaises(dev.DevelopmentError):
                    self.verify()

    def test_release_config_cannot_be_used_for_dev_artifacts(self):
        self.config["updates"] = {"github": "fixture/release"}
        with patch.object(dev, "run", side_effect=self.fake_run), self.assertRaises(dev.DevelopmentError):
            self.verify()

    def test_either_working_update_entry_point_blocks_the_artifact(self):
        for flag in ("--check-for-updates", "--install-update"):
            def unsafe(args):
                return (0, b'{"enabled": true}') if args[-1] == flag else self.fake_run(args)
            with self.subTest(flag=flag), patch.object(dev, "run", side_effect=unsafe), \
                    self.assertRaisesRegex(dev.DevelopmentError, "not blocked"):
                self.verify()

    def test_signature_failure_blocks_the_artifact(self):
        def unsigned(args):
            return (1, b"") if args[0] == "codesign" else self.fake_run(args)
        with patch.object(dev, "run", side_effect=unsigned), \
                self.assertRaisesRegex(dev.DevelopmentError, "signature"):
            self.verify()

    def test_valid_ad_hoc_signature_is_rejected_for_distributable_dev_package(self):
        def adhoc(args):
            if args[0] == "codesign" and any(str(arg).startswith("-R=") for arg in args):
                return 1, b""
            return self.fake_run(args)
        with patch.object(dev, "run", side_effect=adhoc):
            self.assertEqual(self.verify(), self.info)
            with self.assertRaisesRegex(dev.DevelopmentError, "signature"):
                dev.identity(self.app, self.config, self.commit, self.branch, "AAAAAAAAAA")

    def test_developer_id_gate_requires_the_app_identifier_team_and_apple_chain(self):
        with patch.object(dev, "run", side_effect=self.fake_run) as run:
            dev.identity(self.app, self.config, self.commit, self.branch, "AAAAAAAAAA")
        command = next(c.args[0] for c in run.call_args_list if c.args[0][0] == "codesign")
        requirement = next(arg[3:] for arg in command if str(arg).startswith("-R="))
        self.assertIn('identifier "si.kyun.ytmgo"', requirement)
        self.assertIn('anchor apple generic', requirement)
        self.assertIn('subject.OU] = "AAAAAAAAAA"', requirement)
        self.assertIn('1.2.840.113635.100.6.1.13', requirement)
