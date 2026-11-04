#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Rebuild the accompanied, unmodified FFmpeg source with its recorded options."""
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile


def main():
    root = Path(__file__).resolve().parent
    metadata = json.loads((root / "build.json").read_text())
    name = "ffmpeg-" + metadata["version"]
    archive = root / (name + ".tar.xz")
    if hashlib.sha256(archive.read_bytes()).hexdigest() != metadata["sha256"]:
        raise SystemExit("FFmpeg source checksum mismatch")
    # Keep the installed application and its signed resources unchanged.
    workspace = Path(tempfile.mkdtemp(prefix=name + "-rebuild-"))
    source, build = workspace / name, workspace / "build"
    # The upstream archive contains regular files only; never extract links,
    # devices or paths outside its one expected top-level directory.
    with tarfile.open(archive) as stream:
        for member in stream:
            parts = Path(member.name).parts
            if not parts or parts[0] != name or ".." in parts or "\\" in member.name or ":" in member.name:
                raise SystemExit("Invalid FFmpeg source path")
            if member.isdir():
                continue
            if not member.isfile():
                raise SystemExit("Invalid FFmpeg source entry")
            path = workspace.joinpath(*parts)
            path.parent.mkdir(parents=True, exist_ok=True)
            with stream.extractfile(member) as incoming, path.open("wb") as outgoing:
                while True:
                    chunk = incoming.read(1024 * 1024)
                    if not chunk:
                        break
                    outgoing.write(chunk)
            path.chmod(0o755 if member.mode & 0o111 else 0o644)
    build.mkdir(exist_ok=True)
    executable = "ffmpeg.exe" if "--target-os=mingw32" in metadata["configure"] else "ffmpeg"
    for arguments in (["sh", (source / "configure").as_posix(), *metadata["configure"]],
                      ["make", "-j2", executable]):
        result = subprocess.run(arguments, cwd=build, stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, check=False)
        if result.returncode:
            raise SystemExit("FFmpeg rebuild failed; check the native C build prerequisites")
    print("Rebuilt FFmpeg in " + str(build))


if __name__ == "__main__":
    main()
