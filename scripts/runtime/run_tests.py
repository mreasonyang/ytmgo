#!/usr/bin/env python3
"""Run offline JS tests against the prepared, locked runtime dependencies.

No user npm configuration or source-tree node_modules are used. Raw test output
stays in memory: only fixed outcomes and numeric TAP counters are printed.
"""
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess


ROOT = Path(__file__).resolve().parents[2]
RUNTIME = ROOT / "build/runtime"
SOURCE = ROOT / "scripts/runtime/tests"
STAGED = RUNTIME / "tests"
EXTENSION = ROOT / "scripts/account-extension"
COUNTERS = ("tests", "pass", "fail", "cancelled", "skipped", "todo")


def report(category, **values):
    print(json.dumps({"runtime_tests": category, **values}, sort_keys=True), flush=True)


def run():
    if not (RUNTIME / "node").is_file() or not (RUNTIME / "node_modules").is_dir():
        report("runtime_missing")
        return 1
    # Never overwrite or remove another invocation's test staging directory.
    try:
        STAGED.mkdir()
    except FileExistsError:
        report("test_directory_exists")
        return 1
    except OSError:
        report("test_staging_failed")
        return 1
    process = None
    result = 1
    category = "test_runner_failed"
    counts = {}
    try:
        shutil.copytree(SOURCE, STAGED, dirs_exist_ok=True)
        tests = sorted(STAGED.glob("*.test.mjs"))
        extension_tests = sorted(EXTENSION.glob("*.test.mjs"))
        if not tests or not extension_tests:
            category = "test_files_missing"
        else:
            process = subprocess.Popen(
                [str(RUNTIME / "node"), "--no-warnings", "--test", "--test-reporter=tap",
                 *map(str, tests), *map(str, extension_tests)],
                cwd=RUNTIME, env={"PATH": str(RUNTIME) + ":/usr/bin:/bin"},
                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL, start_new_session=True)
            try:
                output, _ = process.communicate(timeout=120)
                counts = {name.decode("ascii"): int(value) for name, value in re.findall(
                    rb"^# (tests|pass|fail|cancelled|skipped|todo) ([0-9]+)\r?$", output, re.MULTILINE)}
                del output
                complete = all(name in counts for name in COUNTERS) and counts["tests"] > 0
                passed = complete and process.returncode == 0 and counts["fail"] == counts["cancelled"] == 0
                category = "pass" if passed else "test_failed" if complete else "test_summary_missing"
                result = 0 if passed else 1
            except subprocess.TimeoutExpired:
                category = "test_timeout"
            except KeyboardInterrupt:
                category = "test_interrupted"
    except (OSError, ValueError):
        category = "test_runner_failed"
    finally:
        if process is not None and process.poll() is None:
            # The runner owns this process group; no PID discovery, argv or
            # environment inspection is needed to reap test fixture children.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.communicate()
        try:
            shutil.rmtree(STAGED)
        except OSError:
            category, result = "test_cleanup_failed", 1
    report(category, **counts)
    return result


if __name__ == "__main__":
    raise SystemExit(run())
