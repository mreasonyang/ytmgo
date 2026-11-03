#!/usr/bin/env python3
"""Offline native protocol tests: no account, media, or audio device access."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
ROOT = Path(__file__).resolve().parents[2]
FAILURE_CATEGORIES = {"context deadline exceeded", "script_process_failed", "script_protocol_failed",
                      "script_result_failed", "script_timeout", "runtime_missing", "cache_limit"}


def run_tests(component, command, env):
    # Dependency/test output can contain private values. Keep it local and emit
    # only static test identifiers and source locations, never raw diagnostics.
    result = subprocess.run(command, cwd=ROOT, env=env, capture_output=True,
                            encoding="utf-8", errors="replace")
    failed, locations, categories = set(), set(), set()
    for line in result.stdout.splitlines():
        if component == "go":
            try:
                event = json.loads(line)
            except (ValueError, TypeError):
                continue
            if not isinstance(event, dict):
                continue
            name = str(event.get("Test", "")).split("/", 1)[0]
            if event.get("Action") == "fail" and re.fullmatch(r"Test[A-Za-z0-9_]{1,120}", name):
                failed.add(name)
            location = re.search(r"\b((?:native|streaming)_test\.go:\d+):", str(event.get("Output", "")))
            if location:
                locations.add(location[1])
                message = str(event.get("Output", ""))[location.end():].strip()
                if message in FAILURE_CATEGORIES:
                    categories.add(message)
        else:
            match = re.fullmatch(r"test ([A-Za-z_][A-Za-z0-9_:]{0,160}) \.\.\. FAILED", line)
            if match:
                failed.add(match[1])
    summary = {"native_runtime_component": component, "status": "pass" if result.returncode == 0 else "fail",
               "exit_code": result.returncode, "failed_tests": sorted(failed)}
    if result.returncode:
        summary["test_locations"] = sorted(locations)
        summary["failure_categories"] = sorted(categories)
    print(json.dumps(summary), flush=True)
    return result.returncode == 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--count", type=int, choices=(1, 3, 5), default=1)
    args = parser.parse_args()
    suffix=".exe" if sys.platform=="win32" else ""
    env=os.environ.copy()
    env["YTMGO_TEST_NATIVE_HOST"]=str(ROOT / "build/runtime" / ("ytmgo-playback-host"+suffix))
    env["YTMGO_TEST_PROOF_DOM"]="1"
    env.pop("YTMGO_TEST_ANONYMOUS_WARM",None)
    # Match the packaged build profile and reuse its dependencies, rather than
    # rebuilding the entire WebView stack in a second, debug target directory.
    commands = [("rust", ["cargo", "test", "--release", "--locked", "--target-dir", str(ROOT / "build/playback-target"),
                          "--manifest-path", str(ROOT / "native/playback-host/Cargo.toml")]),
                ("go", ["go", "test", "-json", "./internal/streaming", "-count=" + str(args.count)])]
    print(json.dumps({"native_runtime_test_count": args.count}), flush=True)
    for component, command in commands:
        print(json.dumps({"native_runtime_component": component, "status": "running"}), flush=True)
        if not run_tests(component, command, env):
            print("Native runtime tests failed")
            return 1
    print("Native runtime tests passed (offline; audio disabled)");return 0
if __name__=="__main__":raise SystemExit(main())
