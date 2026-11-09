#!/usr/bin/env python3
"""Build a development package by default; release builds explicitly opt in."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

from build_config import VERSION, release_config

ROOT = Path(__file__).resolve().parents[2]


def git(*args):
    result = subprocess.run(["git", *args], cwd=ROOT, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        raise ValueError("Build source metadata unavailable")
    return result.stdout.decode().strip()


def build_environment(kind, release_version):
    config = json.loads((ROOT / "mygo.json").read_text())
    commit = git("rev-parse", "HEAD")
    branch = git("rev-parse", "--abbrev-ref", "HEAD")
    if not re.fullmatch(r"[0-9a-f]{40}", commit) or not re.fullmatch(r"[A-Za-z0-9._/-]+", branch):
        raise ValueError("Unsupported source identity")
    dirty = bool(git("status", "--porcelain", "--untracked-files=normal"))
    if kind == "release":
        if not release_version or not VERSION.fullmatch(release_version) or config["version"] != release_version:
            raise ValueError("Release build requires matching explicit RELEASE_VERSION and configured mygo.json")
        base = json.loads(git("show", "HEAD:mygo.json"))
        expected = release_config(base, release_version, os.environ.get("RELEASE_BUILD_NUMBER") or os.environ.get("GITHUB_RUN_NUMBER"))
        if config != expected:
            raise ValueError("Release configuration differs from the explicit source configuration")
        exclusions = [":(exclude)mygo.json"]
        overlay = ROOT / "build/desktop-notice-overlay.json"
        if overlay.exists():
            for entry in json.loads(overlay.read_text()):
                relative = Path(entry["path"])
                if relative.is_absolute() or ".." in relative.parts or not relative.as_posix().startswith("scripts/licensing/"):
                    raise ValueError("Invalid supplemental license notice")
                if hashlib.sha256((ROOT / relative).read_bytes()).hexdigest() != entry["sha256"]:
                    raise ValueError("Supplemental license notice checksum mismatch")
                exclusions.append(":(exclude)" + relative.as_posix())
        if git("status", "--porcelain", "--untracked-files=normal", "--", ".", *exclusions):
            raise ValueError("Release builds require a clean source checkout")
        dirty = False  # Only the verified, intentional release configuration differs.
    # No secret or inherited custom linker flags can change the build identity
    # or silently re-enable updates in a development package.
    env = os.environ.copy()
    env.pop("MYGO_UPDATER_PRIVATE_KEY", None)
    flags = ["-X", "main.buildKind=" + kind, "-X", "main.buildCommit=" + commit,
             "-X", "main.buildBranch=" + branch, "-X", "main.buildDirty=" + str(dirty).lower()]
    if kind != "release":
        flags += ["-X", "github.com/egoist/mygo.packageUpdateFeed=", "-X", "github.com/egoist/mygo.packageUpdateKey="]
    env["MYGO_LDFLAGS"] = " ".join(flags)
    env["CGO_ENABLED"] = "0"
    return env


def main():
    global ROOT
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kind", choices=("development", "release"), default="development")
    parser.add_argument("--version", default=os.environ.get("RELEASE_VERSION"))
    parser.add_argument("--skip-dmg", action="store_true")
    parser.add_argument("--source", type=Path, default=ROOT)
    args = parser.parse_args()
    ROOT = args.source.resolve()
    try:
        env = build_environment(args.kind, args.version)
        result = subprocess.run(["go", "tool", "mygo", "build", *(["-skip-dmg"] if args.skip_dmg else [])], cwd=ROOT, env=env)
        return result.returncode
    except ValueError as error:
        print(str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
