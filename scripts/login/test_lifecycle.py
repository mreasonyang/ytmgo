#!/usr/bin/env python3
"""Exercise the fixture-only WebView helper without a Google account.

Build with `cargo build --locked --features fixture` first. The fixture stores
an HttpOnly synthetic cookie in the real private WebView cookie store. Raw
stdout, stderr, URLs and session values are never printed or saved.
"""
import argparse
import json
from pathlib import Path
import subprocess
import threading
import time


MAX_REPLY = 129 * 1024


def run_case(helper, case):
    reply = []
    process = subprocess.Popen(
        [str(helper)], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )

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
        if reader.is_alive() or len(reply) != 1 or not 0 < len(reply[0]) <= MAX_REPLY:
            return False
        try:
            result = json.loads(reply[0])
        except (ValueError, UnicodeError):
            return False
        if process.returncode != 0 or result.get("version") != 1:
            return False
        if case == "success":
            session = result.get("session", {})
            cookies = session.get("cookies", [])
            return (result.get("status") == "session"
                    and session.get("accountIndex") == 3
                    and session.get("onBehalfOfUser") == "fixture_channel"
                    and any(cookie.get("name") == "SAPISID"
                            and cookie.get("domain") == ".youtube.com"
                            and cookie.get("path") == "/"
                            for cookie in cookies))
        if case == "timeout":
            return result.get("status") == "error" and result.get("error") == "login_timeout"
        return result.get("status") == "cancelled"
    except (OSError, subprocess.TimeoutExpired):
        return False
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        for stream in (process.stdin, process.stdout):
            try:
                stream.close()
            except (OSError, ValueError):
                pass
        reader.join(timeout=1)
        reply.clear()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--helper", required=True, type=Path)
    args = parser.parse_args()
    helper = args.helper.resolve()
    if not helper.is_file():
        print(json.dumps({"login_lifecycle": "helper_missing"}))
        return 1
    passed = True
    for case in ("success", "cancel", "parent_eof", "timeout"):
        try:
            ok = run_case(helper, case)
        except OSError:
            ok = False
        passed = passed and ok
        print(json.dumps({"login_lifecycle": case, "status": "pass" if ok else "fail"}), flush=True)
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
