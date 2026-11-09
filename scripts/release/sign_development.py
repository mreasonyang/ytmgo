"""Sign CI-built dev bundles without executing their code in the credential job."""
import argparse
import json
from pathlib import Path, PurePosixPath
import posixpath
import re
import stat
import sys
import zipfile

import macos
import verify_development as dev


def validate_input(directory, commit, branch, version):
    receipt = json.loads((directory / "build.json").read_text())
    arch = receipt.get("architecture")
    build = receipt.get("build", {})
    name = "ytmgo-" + version + "-dev-" + commit[:12] + "-darwin-" + str(arch) + ".zip"
    if (arch not in {"arm64", "amd64"} or receipt.get("archive") != name
            or build.get("commit") != commit or build.get("branch") != branch
            or build.get("version") != version or build.get("kind") != "development"
            or build.get("updates_enabled") is not False or build.get("dirty") is not False
            or receipt.get("artifact_only") is not True
            or macos.digest(directory / name) != receipt.get("archive_sha256")):
        raise macos.PackageError("Development signing input identity or digest mismatch")
    archive = directory / name
    # Reject paths that could escape the unpack directory before invoking ditto.
    with zipfile.ZipFile(archive) as bundle:
        seen = set()
        size = 0
        for entry in bundle.infolist():
            path = PurePosixPath(entry.filename)
            if (path.is_absolute() or ".." in path.parts or "\\" in entry.filename
                    or not path.parts or path.parts[0] != "ytmGO.app"
                    or entry.filename in seen):
                raise macos.PackageError("Unsafe development archive path")
            seen.add(entry.filename)
            size += entry.file_size
            if size > 2 * 1024**3:
                raise macos.PackageError("Development archive exceeds size limit")
            if stat.S_ISLNK(entry.external_attr >> 16):
                target = bundle.read(entry).decode()
                resolved = posixpath.normpath(posixpath.join(str(path.parent), target))
                if target.startswith("/") or not resolved.startswith("ytmGO.app/"):
                    raise macos.PackageError("Unsafe development archive symlink")
    return arch, archive


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--branch", required=True)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.commit) or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", args.version):
        raise macos.PackageError("Invalid development signing source")
    # Validate every input before importing any credentials.
    inputs = [validate_input(p, args.commit, args.branch, args.version)
              for p in sorted(args.input.iterdir())]
    if not inputs or len({arch for arch, _ in inputs}) != len(inputs):
        raise macos.PackageError("Missing or duplicated development architecture")
    args.out.mkdir(parents=True, exist_ok=False)
    directory = macos.signing_directory()
    try:
        identity, keychain = macos.import_identity(directory)
        team = macos.required("APPLE_TEAM_ID")
        for arch, archive in inputs:
            unpack = args.out / arch
            unpack.mkdir()
            macos.run(["ditto", "-x", "-k", archive, unpack], "Development app extraction")
            app = unpack / "ytmGO.app"
            macos.sign_app(app, identity, keychain)
            dev.verify_signature(app, team)
            macos.run(["ditto", "-c", "-k", "--keepParent", app,
                       args.out / ("signed-app-" + arch + ".zip")], "Signed development archive creation")
        print("Developer ID signed development bundles; native verification still required")
    finally:
        macos.cleanup()


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error) if isinstance(error, (macos.PackageError, dev.DevelopmentError))
              else "Development signing failed", file=sys.stderr)
        sys.exit(1)
