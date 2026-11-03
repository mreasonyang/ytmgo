import contextlib
import io
import json
import subprocess
import unittest
from unittest.mock import patch

from run_tests import run_tests


class RuntimeTestDiagnosticsTests(unittest.TestCase):
    def run_fixture(self, component, stdout, stderr="", returncode=1):
        output = io.StringIO()
        result = subprocess.CompletedProcess([], returncode, stdout, stderr)
        with patch("run_tests.subprocess.run", return_value=result), contextlib.redirect_stdout(output):
            passed = run_tests(component, ["synthetic-command"], {})
        return passed, output.getvalue(), json.loads(output.getvalue())

    def test_go_failure_emits_static_name_and_source_without_private_values(self):
        events = [
            {"Action": "output", "Test": "TestNativeProofDOMHasNoPlaybackAPIs",
             "Output": "    native_test.go:65: synthetic-private-diagnostic\n"},
            {"Action": "fail", "Test": "TestNativeProofDOMHasNoPlaybackAPIs/synthetic-private-case"},
        ]
        passed, raw, summary = self.run_fixture("go", "\n".join(map(json.dumps, events)), "synthetic-private-stderr")
        self.assertFalse(passed)
        self.assertEqual(summary["failed_tests"], ["TestNativeProofDOMHasNoPlaybackAPIs"])
        self.assertEqual(summary["test_locations"], ["native_test.go:65"])
        self.assertNotIn("synthetic-private", raw)

    def test_rust_failure_omits_panic_payload_and_stderr(self):
        passed, raw, summary = self.run_fixture("rust", "test tests::bounded_protocol ... FAILED\nsynthetic-private-panic", "synthetic-private-stderr")
        self.assertFalse(passed)
        self.assertEqual(summary["failed_tests"], ["tests::bounded_protocol"])
        self.assertNotIn("synthetic-private", raw)

    def test_only_closed_error_categories_are_reported(self):
        event = {"Action": "output", "Output": "    native_test.go:65: context deadline exceeded\n"}
        _, _, summary = self.run_fixture("go", json.dumps(event))
        self.assertEqual(summary["failure_categories"], ["context deadline exceeded"])

    def test_malformed_output_cannot_enter_summary(self):
        for component, data in [("go", "synthetic-private\n[]\nnull"),
                                ("go", json.dumps({"Action": "fail", "Test": "synthetic-private"})),
                                ("rust", "test invalid name synthetic-private ... FAILED")]:
            with self.subTest(component=component):
                passed, raw, summary = self.run_fixture(component, data)
                self.assertFalse(passed)
                self.assertEqual(summary["failed_tests"], [])
                self.assertNotIn("synthetic-private", raw)

    def test_success_still_requires_successful_exit(self):
        passed, _, summary = self.run_fixture("go", "synthetic-private-output", returncode=0)
        self.assertTrue(passed)
        self.assertEqual(summary["status"], "pass")


if __name__ == "__main__":
    unittest.main()
