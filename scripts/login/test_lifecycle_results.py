import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from test_lifecycle import FixtureStages, MAX_REPLY, MAX_STAGE_LINE, STAGES, main, reply_failure, run_case


class LifecycleResultTests(unittest.TestCase):
    def test_repeated_success_preserves_every_attempt_and_prior_failure(self):
        with tempfile.TemporaryDirectory() as folder:
            helper = Path(folder) / "synthetic-helper"
            helper.touch()
            output = io.StringIO()
            outcomes = [None, "process_timeout", None, None, None, None, None, None]
            with patch.object(sys, "argv", ["test_lifecycle", "--helper", str(helper), "--success-runs", "5"]), \
                    patch("test_lifecycle.run_case", side_effect=outcomes) as run, \
                    contextlib.redirect_stdout(output):
                self.assertEqual(main(), 1)
            self.assertEqual(run.call_count, 8)
            results = [json.loads(line) for line in output.getvalue().splitlines()]
            self.assertEqual([(row["login_lifecycle"], row["attempt"]) for row in results],
                             [("success", n) for n in range(1, 6)] + [("cancel", 1), ("parent_eof", 1), ("timeout", 1)])
            self.assertEqual(results[1]["reason"], "process_timeout")
            self.assertEqual(results[-1]["status"], "pass")
            self.assertTrue(all(isinstance(row["elapsed_ms"], int) and row["elapsed_ms"] >= 0 for row in results))

    def test_real_pipe_diagnostics_emit_only_known_stages(self):
        result = {"version": 1, "status": "session", "session": {
            "accountIndex": 3, "onBehalfOfUser": "fixture_channel",
            "cookies": [{"name": "SAPISID", "domain": ".youtube.com", "path": "/"}],
        }}
        program = (
            "import sys\n"
            "sys.stdin.readline()\n"
            "sys.stderr.write('synthetic-private-data' * 1000 + '\\n')\n"
            "sys.stderr.write('ytmgo-fixture-stage:view_created\\n')\n"
            "sys.stderr.write('ytmgo-fixture-stage:session_ready\\n')\n"
            f"sys.stdout.write({json.dumps(result)!r})\n"
        )
        launch = subprocess.Popen
        stages = []
        with patch("test_lifecycle.subprocess.Popen", side_effect=lambda _command, **kwargs:
                   launch([sys.executable, "-c", program], **kwargs)):
            self.assertIsNone(run_case("synthetic helper", "success", stages))
        self.assertEqual(stages, ["view_created", "session_ready"])

    def test_stderr_stages_are_exact_bounded_and_deduplicated(self):
        parser = FixtureStages()
        for stage in STAGES:
            line = b"ytmgo-fixture-stage:" + stage.encode() + b"\r\n"
            parser.feed(line[:12])
            parser.feed(line[12:])
            parser.feed(line)
        self.assertEqual(parser.seen, list(STAGES))
        self.assertEqual(parser.pending, b"")
        parser = FixtureStages()
        parser.feed(b"synthetic-private-data:" + b"x" * 10000)
        self.assertLessEqual(len(parser.pending), MAX_STAGE_LINE)
        parser.feed(b"ytmgo-fixture-stage:session_ready\n")
        parser.feed(b"prefix:ytmgo-fixture-stage:session_ready\n")
        parser.feed(b"ytmgo-fixture-stage:session_ready:synthetic-private-data\n")
        parser.feed(b"ytmgo-fixture-stage:synthetic-private-data\n")
        self.assertEqual(parser.seen, [])
        self.assertEqual(parser.pending, b"")
        parser.feed(b"ytmgo-fixture-stage:session_ready\n")
        self.assertEqual(parser.seen, ["session_ready"])

    def test_fixed_helper_failures_and_expected_timeout(self):
        for code in ("runtime_missing", "runtime_outdated", "login_timeout", "login_failed"):
            payload = json.dumps({"version": 1, "status": "error", "error": code}).encode()
            self.assertEqual(reply_failure(payload, 0, "success"), "helper_" + code)
        self.assertIsNone(reply_failure(payload.replace(b"login_failed", b"login_timeout"), 0, "timeout"))

    def test_untrusted_output_never_enters_reason(self):
        marker = "synthetic-private-data"
        cases = [
            (marker.encode(), "reply_invalid_json"),
            (json.dumps({"version": 1, "status": "error", "error": marker}).encode(), "helper_unknown_error"),
            (json.dumps({"version": 1, "status": marker}).encode(), "unexpected_status"),
            (json.dumps({"version": 1, "status": "session", "session": marker}).encode(), "session_shape"),
            (b"x" * (MAX_REPLY + 1), "reply_oversized"),
            (b"", "reply_empty"),
            (b"[]", "reply_invalid_envelope"),
        ]
        for payload, expected in cases:
            self.assertEqual(reply_failure(payload, 0, "success"), expected)
        self.assertEqual(reply_failure(b'{"version":1}', 2, "success"), "process_exit_failure")

    def test_success_requires_synthetic_session_shape(self):
        result = {"version": 1, "status": "session", "session": {
            "accountIndex": 3, "onBehalfOfUser": "fixture_channel",
            "cookies": [{"name": "SAPISID", "domain": ".youtube.com", "path": "/"}],
        }}
        self.assertIsNone(reply_failure(json.dumps(result).encode(), 0, "success"))
        result["session"]["cookies"] = ["synthetic-private-data"]
        self.assertEqual(reply_failure(json.dumps(result).encode(), 0, "success"), "session_shape")


if __name__ == "__main__":
    unittest.main()
