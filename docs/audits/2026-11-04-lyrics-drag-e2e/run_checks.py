"""Run acceptance checks, persisting only allowlisted result metadata."""
import datetime
import json
import os
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
checks = [
    ("go-race", ["go", "test", "-race", "-json", "-count=1", "./..."]),
    ("go-vet", ["go", "vet", "./..."]),
    ("i18n", ["go", "tool", "mygo", "i18n", "check"]),
    ("runtime-manifest", ["python3", "scripts/runtime/test_runtime_manifest.py"]),
    ("runtime-protocol", ["python3", "scripts/runtime/test_protocol_version.py"]),
    ("runtime-tests", ["python3", "scripts/runtime/run_tests.py"]),
    ("login-tests", ["python3", "-m", "unittest", "discover", "-s", "scripts/login", "-p", "test_*.py"]),
    ("license-tests", ["python3", "-m", "unittest", "discover", "-s", "scripts/licensing", "-p", "test_*.py"]),
    ("diff-check", ["git", "diff", "--check"]),
    ("installed-signature", ["codesign", "--verify", "--deep", "--strict", "/Applications/ytmGO.app"]),
]
results = []
for name, command in checks:
    started = time.monotonic()
    env = os.environ.copy()
    env["YTMGO_UI_SNAPSHOT_DIR"] = str(ROOT / "build/lyrics-drag-e2e-20261104/snapshots")
    p = subprocess.run(command, cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    result = {"name": name, "command": command, "passed": p.returncode == 0,
              "exit_code": p.returncode, "elapsed_seconds": round(time.monotonic()-started, 3)}
    if name == "go-race":
        tests = {"pass": [], "skip": [], "fail": []}
        packages = []
        for line in p.stdout.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            action = event.get("Action")
            if action in tests and event.get("Test"):
                tests[action].append({"package": event.get("Package"), "test": event["Test"], "elapsed": event.get("Elapsed")})
            elif action in ("pass", "skip", "fail") and not event.get("Test"):
                packages.append({"package": event.get("Package"), "action": action, "elapsed": event.get("Elapsed")})
        result["counts"] = {k: len(v) for k,v in tests.items()}
        result["packages"] = packages
        (OUT / "go-test-cases.json").write_text(json.dumps(tests, indent=2)+"\n")
    results.append(result)
    (OUT / "checks.json").write_text(json.dumps({"updated": datetime.datetime.now(datetime.timezone.utc).isoformat(), "checks": results}, indent=2)+"\n")
    print(json.dumps(result), flush=True)
raise SystemExit(0 if all(r["passed"] for r in results) else 1)
