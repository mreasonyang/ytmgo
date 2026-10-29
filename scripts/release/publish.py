#!/usr/bin/env python3
"""Publish a complete notarized release; expose only fixed diagnostic categories."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
REPOSITORY = "mreasonyang/ytmgo"
VERSION = re.compile(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-rc\.[1-9][0-9]*)?")


class ReleaseError(Exception):
    pass


def configure():
    if os.environ.get("PUBLISH_RELEASE") == "true" and os.environ.get("PACKAGE_MODE") != "notarized":
        raise ReleaseError("Publishing requires notarized mode")
    path = ROOT / "mygo.json"
    config = json.loads(path.read_text())
    version = os.environ.get("RELEASE_VERSION") or config["version"]
    if not VERSION.fullmatch(version):
        raise ReleaseError("Invalid release version; use X.Y.Z or X.Y.Z-rc.N")
    config["version"] = version
    path.write_text(json.dumps(config, indent=2) + "\n")
    print("Configured release version: " + version)


def gh(args, stage, allow_missing=False):
    # The authenticated client consumes its token from the environment. Never
    # surface argv, raw HTTP bodies, stderr or exception text as diagnostics.
    result = subprocess.run(["gh", *args], cwd=ROOT, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, check=False, timeout=300)
    if result.returncode:
        if allow_missing:
            return None
        raise ReleaseError(stage + " failed")
    return result.stdout


def sha256(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def validate(directory, config, commit):
    version = config["version"]
    if not VERSION.fullmatch(version) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ReleaseError("Invalid version or source revision")
    metadata = json.loads((directory / "build.json").read_text())
    if (metadata.get("version") != version or metadata.get("commit") != commit
            or metadata.get("mode") != "notarized" or metadata.get("architecture") != "arm64"
            or any(metadata.get(field) is not True for field in ("notarized", "developer_id_signed", "signed_updates"))):
        raise ReleaseError("Release requires matching notarized, signed update artifacts")
    prefix = "ytmgo-" + version
    names = [prefix + "-arm64-notarized.dmg", prefix + "-arm64-notarized.zip",
             prefix + "-darwin-arm64.tar.gz", "update-darwin-arm64.json", "build.json"]
    expected = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._-]+)", line)
        if not match or match[2] in expected:
            raise ReleaseError("Invalid release checksums")
        expected[match[2]] = match[1]
    if set(expected) != set(names) or any(sha256(directory / name) != expected[name] for name in names):
        raise ReleaseError("Release checksum mismatch")
    manifest = json.loads((directory / "update-darwin-arm64.json").read_text())
    archive = directory / names[2]
    if (manifest.get("version") != version or manifest.get("size") != archive.stat().st_size
            or manifest.get("url") != "https://github.com/" + REPOSITORY + "/releases/download/v" + version + "/" + archive.name):
        raise ReleaseError("Update manifest does not match release")
    # Verify the updater signature with the public key, independently from its
    # generation. This process receives neither Apple nor update credentials.
    helper = ROOT / "build/release/update-package"
    result = subprocess.run([str(helper), "--verify", "--out", str(directory)], cwd=ROOT,
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        raise ReleaseError("Update signature verification failed")
    return [directory / name for name in [*names, "SHA256SUMS"]]


def publish():
    if os.environ.get("GITHUB_REF") != "refs/heads/main" or os.environ.get("GITHUB_REPOSITORY") != REPOSITORY:
        raise ReleaseError("Releases are restricted to the repository main branch")
    config = json.loads((ROOT / "mygo.json").read_text())
    commit = os.environ.get("GITHUB_SHA", "")
    files = validate(ROOT / "dist/macos", config, commit)
    version = config["version"]
    tag = "v" + version
    # Never overwrite a published version or repoint an existing tag. A failed
    # upload leaves an unpublished draft for operator inspection.
    if gh(["release", "view", tag, "--repo", REPOSITORY, "--json", "id"], "Existing release lookup", True) is not None:
        raise ReleaseError("Release already exists; choose a new version")
    existing = json.loads(gh(["api", "repos/" + REPOSITORY + "/git/matching-refs/tags/" + tag], "Tag lookup"))
    if any(ref.get("ref") == "refs/tags/" + tag for ref in existing):
        raise ReleaseError("Release tag already exists")
    notes = ("Native update checks with signed downloads and restart installation.\n\n"
             "macOS 27 or newer, Apple Silicon. Install the DMG once when upgrading from 0.4.0.\n"
             "The app checks for updates after launch and daily. Check manually from the ytmgo menu.\n\n"
             "The App and DMG are Developer ID signed, notarized and stapled. "
             "The updater verifies a separate Ed25519 signature before installation.\n\n"
             "Source: " + commit + "\n")
    with tempfile.TemporaryDirectory(prefix="ytmgo-release-") as temporary:
        body = Path(temporary) / "notes.md"
        body.write_text(notes)
        args = ["release", "create", tag, "--repo", REPOSITORY, "--target", commit,
                "--draft", "--title", "ytmgo " + version, "--notes-file", str(body)]
        if "-" in version:
            args.append("--prerelease")
        gh(args, "Draft release creation")
    gh(["release", "upload", tag, "--repo", REPOSITORY, *map(str, files)], "Release asset upload")
    release = json.loads(gh(["api", "repos/" + REPOSITORY + "/releases/tags/" + tag], "Uploaded release verification"))
    remote = {asset["name"]: asset for asset in release["assets"]}
    if not release["draft"] or set(remote) != {path.name for path in files}:
        raise ReleaseError("Draft asset set mismatch")
    for path in files:
        asset = remote[path.name]
        if asset["size"] != path.stat().st_size or asset.get("digest") != "sha256:" + sha256(path):
            raise ReleaseError("Uploaded asset checksum mismatch")
    gh(["release", "edit", tag, "--repo", REPOSITORY, "--draft=false", "--latest=" + ("false" if "-" in version else "true")],
       "Release publication")
    print("Published verified release: https://github.com/" + REPOSITORY + "/releases/tag/" + tag)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("configure", "publish"))
    args = parser.parse_args()
    try:
        configure() if args.command == "configure" else publish()
    except Exception as error:
        message = str(error) if isinstance(error, ReleaseError) else "operation failed"
        print("::error title=Release publication::" + message)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
