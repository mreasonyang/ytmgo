"""Verify actual bundled decoder PCM without printing dependency diagnostics."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

root = Path(__file__).resolve().parents[3]
env = os.environ.copy()
env["YTMGO_RUNTIME_DIR"] = "/Applications/ytmGO.app/Contents/Resources/runtime"
env["YTMGO_FIXTURE_FFMPEG"] = shutil.which("ffmpeg") or ""
names = ["TestFFmpegSeekUsesRecordingTimeline", "TestFFmpegTailSeekUsesRangesAndMatchesPCM",
         "TestEmptySeekPreservesPauseAndReachesNaturalEnd", "TestFFmpegActualSeek", "TestFFmpegEndedRestartLocal"]
command = ["go", "test", "-race", "-json", "-count=1", "-run", "^("+"|".join(names)+")$", "./internal/player"]
started = time.monotonic()
p = subprocess.run(command, cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
events = []
for line in p.stdout.splitlines():
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if e.get("Action") in ("pass", "skip", "fail") and e.get("Test"):
        events.append({"test": e["Test"], "action": e["Action"], "elapsed": e.get("Elapsed")})
result = {"command": command, "passed": p.returncode == 0 and not any(e["action"] != "pass" for e in events),
          "elapsed_seconds": round(time.monotonic()-started, 3), "cases": events,
          "installed_decoder": env["YTMGO_RUNTIME_DIR"]+"/ffmpeg"}
(Path(__file__).parent / "decoder-checks.json").write_text(json.dumps(result, indent=2)+"\n")
print(json.dumps(result))
raise SystemExit(0 if result["passed"] else 1)
