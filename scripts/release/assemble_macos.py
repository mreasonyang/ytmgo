"""Combine two checksum-verified macOS packages without overwriting assets."""
import json
from pathlib import Path
import re
import shutil
import sys

from macos import ROOT, digest, PackageError


def assemble(root=ROOT):
    destination = root / "dist/macos"
    if destination.exists():
        raise PackageError("Combined output already exists")
    sources = []
    metadata = []
    for arch in ("arm64", "amd64"):
        folder = root / ("dist/macos-" + arch)
        checksums = {}
        for line in (folder / "SHA256SUMS").read_text().splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._-]+)", line)
            if not match or match[2] in checksums:
                raise PackageError("Invalid architecture checksums")
            checksums[match[2]] = match[1]
        if {path.name for path in folder.iterdir()} != set(checksums) | {"SHA256SUMS"}:
            raise PackageError("Incomplete architecture assets")
        meta = json.loads((folder / ("build.json" if arch == "arm64" else "build-amd64.json")).read_text())
        if meta.get("architecture") != arch:
            raise PackageError("Architecture metadata mismatch")
        metadata.append({key: value for key, value in meta.items() if key != "architecture"})
        for name, expected in checksums.items():
            path = folder / name
            if path.is_symlink() or digest(path) != expected:
                raise PackageError("Architecture asset checksum mismatch")
            sources.append(path)
    if metadata[0] != metadata[1] or len({path.name for path in sources}) != len(sources):
        raise PackageError("Architecture source or package mode mismatch")
    destination.mkdir()
    for source in sources:
        shutil.copyfile(source, destination / source.name)
    (destination / "SHA256SUMS").write_text("".join(digest(path) + "  " + path.name + "\n" for path in sorted(destination.iterdir())))


if __name__ == "__main__":
    try:
        assemble()
        print("Assembled both macOS architectures")
    except Exception:
        sys.exit("Dual architecture assembly failed")
