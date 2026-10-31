#!/usr/bin/env python3
"""Offline native protocol tests: no account, media, or audio device access."""
import os
from pathlib import Path
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[2]
def main():
    suffix=".exe" if sys.platform=="win32" else ""
    env=os.environ.copy()
    env["YTMGO_TEST_NATIVE_HOST"]=str(ROOT / "build/runtime" / ("ytmgo-playback-host"+suffix))
    env["YTMGO_TEST_PROOF_DOM"]="1"
    env.pop("YTMGO_TEST_ANONYMOUS_WARM",None)
    for command in (["cargo","test","--locked","--manifest-path",str(ROOT / "native/playback-host/Cargo.toml")],["go","test","./internal/streaming","-count=1"]):
        result=subprocess.run(command,cwd=ROOT,env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        if result.returncode:print("Native runtime tests failed");return 1
    print("Native runtime tests passed (offline; audio disabled)");return 0
if __name__=="__main__":raise SystemExit(main())
