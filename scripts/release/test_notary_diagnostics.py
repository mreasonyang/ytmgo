"""Ensure notarization diagnostics distinguish failures without exposing inputs."""
import os
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

import check_notarization
import macos


class NotaryDiagnosticsTests(unittest.TestCase):
    def test_status_categories_do_not_assume_every_failure_is_a_bad_password(self):
        for message, category in (
            (b"HTTP status code: 401. Invalid credentials", "authentication_rejected"),
            (b"HTTP status code: 403", "access_denied"),
            (b"HTTP status code: 429", "rate_limited"),
            (b"HTTP status code: 503", "service_unavailable"),
            (b"The request timed out", "network_timeout"),
            (b"Could not resolve host", "dns_failed"),
            (b"SSL certificate verification failed", "tls_failed"),
            (b"Could not connect to server", "connection_failed"),
            (b"Inappropriate ioctl for device", "prompt_input_failed"),
            (b"Could not unlock keychain", "keychain_failed"),
            (b"unrecognized private diagnostic", "notary_tool_error"),
        ):
            with self.subTest(category=category):
                self.assertEqual(macos.notary_failure_category(message), category)

    def test_failure_retains_only_category_and_exit_code(self):
        private = b"HTTP status code: 401, account=private@example.invalid password=private-fixture"
        with patch.object(macos.subprocess, "run") as child:
            child.return_value.returncode = 1
            child.return_value.stderr = private
            child.return_value.stdout = b"Profile for private@example.invalid"
            with self.assertRaises(macos.PackageError) as error:
                macos.run(["xcrun", "notarytool", "store-credentials", "fixture"],
                          "Notarization credential validation", input_bytes=b"private-fixture\n")
        self.assertEqual(str(error.exception),
                         "Notarization credential validation failed (authentication_rejected, exit 1)")
        self.assertNotIn("private", str(error.exception))
        self.assertEqual(child.call_args.kwargs["stdout"], subprocess.PIPE)
        self.assertEqual(child.call_args.kwargs["stderr"], subprocess.PIPE)
        self.assertEqual(child.call_args.kwargs["timeout"], 120)

    def test_failure_on_stdout_is_also_classified_without_forwarding_output(self):
        with patch.object(macos.subprocess, "run") as child:
            child.return_value.returncode = 1
            child.return_value.stderr = b""
            child.return_value.stdout = b"HTTP status code: 503, private diagnostic"
            with self.assertRaises(macos.PackageError) as error:
                macos.run(["xcrun"], "Notarization credential validation")
        self.assertEqual(str(error.exception),
                         "Notarization credential validation failed (service_unavailable, exit 1)")

    def test_process_timeout_does_not_claim_a_network_or_credential_cause(self):
        with patch.object(macos.subprocess, "run", side_effect=subprocess.TimeoutExpired(
                ["xcrun", "private-fixture"], 120, stderr=b"private diagnostic")):
            with self.assertRaises(macos.PackageError) as error:
                macos.run(["xcrun"], "Notarization credential validation")
        self.assertEqual(str(error.exception),
                         "Notarization credential validation failed (validation_timeout)")

    def test_check_reuses_existing_inputs_without_submitting_software(self):
        fixture = {"APPLE_ID": "fixture@example.invalid", "APPLE_TEAM_ID": "AAAAAAAAAA",
                   "APPLE_APP_SPECIFIC_PASSWORD": "synthetic-password-fixture"}
        with patch.dict(os.environ, fixture), patch.object(macos, "run") as run, \
                patch("builtins.print"):
            check_notarization.check()
        call = run.call_args_list[1]
        self.assertIn("store-credentials", call.args[0])
        self.assertNotIn("--no-validate", call.args[0])
        self.assertNotIn("--password", call.args[0])
        self.assertNotIn(fixture["APPLE_APP_SPECIFIC_PASSWORD"], call.args[0])
        self.assertEqual(call.kwargs["input_bytes"], b"synthetic-password-fixture\n")
        self.assertFalse(any("submit" in c.args[0] for c in run.call_args_list))

    def test_temporary_keychain_is_removed_after_validation_fails(self):
        created = []

        def command(args, *unused, **kwargs):
            if "create-keychain" in args:
                created.append(Path(args[-1]))
                created[-1].touch()
            elif "store-credentials" in args:
                raise macos.PackageError("fixture validation failure")

        fixture = {"APPLE_ID": "fixture@example.invalid", "APPLE_TEAM_ID": "AAAAAAAAAA",
                   "APPLE_APP_SPECIFIC_PASSWORD": "synthetic-password-fixture"}
        with patch.dict(os.environ, fixture), patch.object(macos, "run", side_effect=command) as run:
            with self.assertRaisesRegex(macos.PackageError, "fixture validation failure"):
                check_notarization.check()
        self.assertIn("delete-keychain", run.call_args_list[-1].args[0])
        self.assertFalse(created[0].parent.exists())
