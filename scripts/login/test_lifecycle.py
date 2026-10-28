#!/usr/bin/env python3
"""Exercise the fixture-only WebView helper without a Google account.

Build with `cargo build --locked --features fixture` first. The fixture stores
an HttpOnly synthetic cookie in the real private WebView cookie store. Raw
stdout, stderr, URLs and session values are never printed or saved.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import threading
import time


MAX_REPLY = 129 * 1024
HELPER_ERRORS = frozenset(("runtime_missing", "runtime_outdated", "login_timeout", "login_failed"))
STAGES = (
    "view_created", "store_empty", "cookie_set", "load_started", "load_expected",
    "load_other", "complete_received", "metadata_requested", "metadata_received",
    "metadata_valid", "cookies_read_started", "cookies_selected", "cookies_missing",
    "metadata_after_requested", "metadata_after_valid", "session_ready",
    "cleanup_started", "cleanup_finished",
)
STAGE_LINES = {(b"ytmgo-fixture-stage:" + stage.encode()): stage for stage in STAGES}
MAX_STAGE_LINE = 96


class FixtureStages:
    """Keep only exact allowlisted stages; all other stderr is discarded."""
    def __init__(self):
        self.seen = []
        self.pending = bytearray()
        self.dropping = False

    def feed(self, chunk):
        for byte in chunk:
            if byte == 10:
                stage = None if self.dropping else STAGE_LINES.get(bytes(self.pending).removesuffix(b"\r"))
                if stage is not None and stage not in self.seen:
                    self.seen.append(stage)
                self.pending.clear()
                self.dropping = False
            elif not self.dropping:
                if len(self.pending) >= MAX_STAGE_LINE:
                    self.pending.clear()
                    self.dropping = True
                else:
                    self.pending.append(byte)


def reply_failure(payload, returncode, case):
    """Return only fixed diagnostic categories, never any helper-provided text."""
    if len(payload) > MAX_REPLY:
        return "reply_oversized"
    if not payload:
        return "reply_empty"
    try:
        result = json.loads(payload)
    except (ValueError, UnicodeError):
        return "reply_invalid_json"
    if returncode != 0:
        return "process_exit_failure"
    if not isinstance(result, dict) or result.get("version") != 1:
        return "reply_invalid_envelope"
    status = result.get("status")
    if case == "timeout" and status == "error" and result.get("error") == "login_timeout":
        return None
    if status == "error":
        code = result.get("error")
        return "helper_" + code if isinstance(code, str) and code in HELPER_ERRORS else "helper_unknown_error"
    if case == "success":
        if status != "session":
            return "unexpected_status"
        session = result.get("session")
        if not isinstance(session, dict):
            return "session_shape"
        cookies = session.get("cookies")
        if (session.get("accountIndex") != 3
                or session.get("onBehalfOfUser") != "fixture_channel"
                or not isinstance(cookies, list)
                or not any(isinstance(cookie, dict)
                           and cookie.get("name") == "SAPISID"
                           and cookie.get("domain") == ".youtube.com"
                           and cookie.get("path") == "/"
                           for cookie in cookies)):
            return "session_shape"
        return None
    return None if status == "cancelled" and case != "timeout" else "unexpected_status"


def run_case(helper, case, seen_stages=None):
    reply = []
    process = subprocess.Popen(
        [str(helper)], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    stages = FixtureStages()
    stop_stages = threading.Event()

    def receive_stages():
        # Python 3.13 in CI supports nonblocking pipe reads on all three OSes.
        # A descendant holding a pipe cannot keep this diagnostic reader alive.
        try:
            descriptor = process.stderr.fileno()
            os.set_blocking(descriptor, False)
            while not stop_stages.is_set():
                try:
                    chunk = os.read(descriptor, 4096)
                except BlockingIOError:
                    stop_stages.wait(0.05)
                    continue
                if not chunk:
                    return
                stages.feed(chunk)
        except (OSError, ValueError):
            pass

    stage_reader = threading.Thread(target=receive_stages, daemon=True)
    stage_reader.start()

    def receive():
        try:
            reply.append(process.stdout.read(MAX_REPLY + 1))
        except (OSError, ValueError):
            reply.append(b"")

    reader = threading.Thread(target=receive, daemon=True)
    reader.start()

    def send(value):
        try:
            process.stdin.write(json.dumps(value).encode() + b"\n")
            process.stdin.flush()
            return True
        except (OSError, ValueError):
            return False

    send({"version": 1, "title": "ytmgo synthetic login test", "timeoutSeconds": 2 if case == "timeout" else 0})
    try:
        if case == "success":
            # The control command is compiled out of production. Repeating it
            # covers both slow WebView creation and the later DOM load.
            deadline = time.monotonic() + 12
            while process.poll() is None and time.monotonic() < deadline:
                send({"command": "fixture-complete"})
                time.sleep(0.25)
        elif case == "cancel":
            time.sleep(1)
            send({"command": "cancel"})
        elif case == "parent_eof":
            time.sleep(1)
            process.stdin.close()
        process.wait(timeout=18)
        reader.join(timeout=3)
        if reader.is_alive() or len(reply) != 1:
            return "reply_incomplete"
        return reply_failure(reply[0], process.returncode, case)
    except subprocess.TimeoutExpired:
        return "reply_oversized" if reply and len(reply[0]) > MAX_REPLY else "process_timeout"
    except OSError:
        return "pipe_failure"
    finally:
        if process.poll() is None:
            process.kill()
            try:
                process.wait(timeout=5)
            except (OSError, subprocess.TimeoutExpired):
                pass
        stage_reader.join(timeout=0.2)
        stop_stages.set()
        stage_reader.join(timeout=1)
        if seen_stages is not None:
            seen_stages.extend(stages.seen)
        stages.pending.clear()
        for stream in (process.stdin, process.stdout, process.stderr):
            try:
                stream.close()
            except (OSError, ValueError):
                pass
        reader.join(timeout=1)
        reply.clear()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--helper", required=True, type=Path)
    parser.add_argument("--success-runs", type=int, choices=range(1, 11), default=1)
    args = parser.parse_args()
    helper = args.helper.resolve()
    if not helper.is_file():
        print(json.dumps({"login_lifecycle": "helper_missing"}))
        return 1
    passed = True
    cases = [("success", attempt) for attempt in range(1, args.success_runs + 1)]
    cases += [(case, 1) for case in ("cancel", "parent_eof", "timeout")]
    for case, attempt in cases:
        stages = []
        started = time.monotonic()
        try:
            reason = run_case(helper, case, stages)
        except OSError:
            reason = "process_start_failure"
        ok = reason is None
        passed = passed and ok
        result = {"login_lifecycle": case, "attempt": attempt, "status": "pass" if ok else "fail",
                  "elapsed_ms": int((time.monotonic() - started) * 1000), "stages": stages}
        if reason is not None:
            result["reason"] = reason
        print(json.dumps(result), flush=True)
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
