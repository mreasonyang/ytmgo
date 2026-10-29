"""Verify a released baseline against the production feed in an isolated bundle."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / "evidence/auto-update-check"
BASE = "https://github.com/mreasonyang/ytmgo/releases/download/v"


class CheckError(Exception):
    pass


def command(args, timeout=120):
    result = subprocess.run(list(map(str, args)), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=timeout)
    if result.returncode:
        raise CheckError("verification_command_failed")
    return result.stdout


def sha(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def fetch(url, path):
    # Public downloads, with no authentication headers or account state.
    with urllib.request.urlopen(url, timeout=45) as source, path.open("xb") as dest:
        while chunk := source.read(1024 * 1024):
            dest.write(chunk)


def release(version):
    directory = EVIDENCE / version
    directory.mkdir()
    for name in ("SHA256SUMS", "build.json", "update-darwin-arm64.json"):
        fetch(BASE + version + "/" + name, directory / name)
    sums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._-]+)", line)
        if not match or match[2] in sums:
            raise CheckError("invalid_public_checksums")
        sums[match[2]] = match[1]
    for name in ("build.json", "update-darwin-arm64.json"):
        if sha(directory / name) != sums[name]:
            raise CheckError("public_metadata_checksum_mismatch")
    meta = json.loads((directory / "build.json").read_text())
    if meta["version"] != version or not all(meta[key] is True for key in ("notarized", "developer_id_signed", "signed_updates")):
        raise CheckError("invalid_public_release_metadata")
    name = "ytmgo-" + version + "-arm64-notarized.zip"
    fetch(BASE + version + "/" + name, directory / name)
    if sha(directory / name) != sums[name]:
        raise CheckError("public_archive_checksum_mismatch")
    command(["ditto", "-x", "-k", directory / name, directory])
    return directory / "ytmgo.app", meta, sums[name]


def integrity(app):
    result = {}
    for label, args in (
        ("apple_signature", ["codesign", "--verify", "--deep", "--strict", app]),
        ("stapled_ticket", ["xcrun", "stapler", "validate", app]),
        ("gatekeeper", ["spctl", "--assess", "--type", "execute", app]),
    ):
        process = subprocess.run(list(map(str, args)), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=120)
        result[label] = process.returncode == 0
    result["version"] = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleShortVersionString"]
    result["cli_version"] = command([app / "Contents/MacOS/ytmgo", "--version"]).decode().strip()
    result["executable_sha256"] = sha(app / "Contents/MacOS/ytmgo")
    if not all(result[key] for key in ("apple_signature", "stapled_ticket", "gatekeeper")):
        raise CheckError("bundle_integrity_failed")
    return result


def inventory(app):
    entries = {}
    for path in app.rglob("*"):
        name = str(path.relative_to(app))
        if path.is_symlink():
            entries[name] = ["link", str(path.readlink())]
        elif path.is_file():
            entries[name] = ["file", path.stat().st_mode & 0o777, sha(path)]
        elif path.is_dir():
            entries[name] = ["directory", path.stat().st_mode & 0o777]
    return entries


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--from-version", required=True)
    parser.add_argument("--to-version", required=True)
    args = parser.parse_args()
    version = r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-rc\.[1-9][0-9]*)?"
    if not re.fullmatch(version, args.from_version) or not re.fullmatch(version, args.to_version) or args.from_version == args.to_version:
        raise CheckError("invalid_acceptance_versions")
    if sys.platform != "darwin":
        raise CheckError("macos_required")
    EVIDENCE.mkdir(parents=True, exist_ok=False)
    app, baseline, baseline_digest = release(args.from_version)
    before = integrity(app)
    if before["cli_version"] != args.from_version:
        raise CheckError("baseline_version_mismatch")
    print("Public baseline verified", flush=True)
    checked = json.loads(command([app / "Contents/MacOS/ytmgo", "--check-for-updates"]))
    if checked.get("available_version") != args.to_version:
        raise CheckError("expected_stable_update_not_offered")
    staged = json.loads(command([app / "Contents/MacOS/ytmgo", "--install-update"], timeout=900))
    if staged.get("install_staged") is not True:
        raise CheckError("update_not_staged")
    for _ in range(200):
        try:
            if plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleShortVersionString"] == args.to_version:
                break
        except (FileNotFoundError, plistlib.InvalidFileException):
            pass
        time.sleep(.1)
    else:
        raise CheckError("bundle_replacement_timeout")
    after = integrity(app)
    published, metadata, digest = release(args.to_version)
    same = inventory(app) == inventory(published)
    if after["cli_version"] != args.to_version or not same:
        raise CheckError("updated_bundle_does_not_match_public_release")
    receipt = {
        "baseline_version": args.from_version, "baseline_source": baseline["commit"], "baseline_zip_sha256": baseline_digest,
        "baseline": before, "stable_version": args.to_version, "stable_source": metadata["commit"], "stable_zip_sha256": digest,
        "checked": checked, "staged": staged, "installed": after, "bundle_matches_public_zip": same,
        "scope": "isolated released bundles; production feed; maintenance CLI; no account store initialized",
        "runner": {"macos": command(["sw_vers", "-productVersion"]).decode().strip(), "architecture": command(["uname", "-m"]).decode().strip()},
        "workflow_url": "https://github.com/mreasonyang/ytmgo/actions/runs/" + os.environ.get("GITHUB_RUN_ID", "local"),
    }
    (EVIDENCE / "upgrade-receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
    print("YTMGO_UPGRADE_RECEIPT=" + json.dumps(receipt), flush=True)
    if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
        with Path(summary).open("a") as stream:
            stream.write("### Public release upgrade PASS\n\n```json\n" + json.dumps(receipt, indent=2) + "\n```\n")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print("::error title=Public update check::" + (str(error) if isinstance(error, CheckError) else "acceptance_operation_failed"))
        sys.exit(1)
