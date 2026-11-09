"""Verify a native development app and create an artifact, without publishing."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys

import macos

ROOT = Path(__file__).resolve().parents[2]


class DevelopmentError(Exception):
    pass


def run(args):
    result = subprocess.run(list(map(str, args)), stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, timeout=120,
                            env={k: v for k, v in os.environ.items() if k not in macos.SECRET_NAMES})
    return result.returncode, result.stdout


def verify_signature(app, team=None):
    command = ["codesign", "--verify", "--deep", "--strict"]
    if team is not None:
        if not re.fullmatch(r"[A-Z0-9]{10}", team):
            raise DevelopmentError("Invalid Developer ID team")
        command += ['-R=identifier "si.kyun.ytmgo" and anchor apple generic and '
                    'certificate leaf[subject.OU] = "' + team + '" and '
                    'certificate leaf[field.1.2.840.113635.100.6.1.13] exists']
    code, _ = run(command + [app])
    if code:
        raise DevelopmentError("Development app signature verification failed")


def identity(app, config, commit, branch, team=None):
    plist = plistlib.loads((app / "Contents/Info.plist").read_bytes())
    executable = app / "Contents/MacOS/ytmGO"
    code, reply = run([executable, "--build-info"])
    info = json.loads(reply)
    version = config["version"]
    if (code or config.get("updates") is not None or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version)
            or info.get("kind") != "development" or info.get("version") != version
            or info.get("commit") != commit or info.get("branch") != branch
            or info.get("dirty") is not False or info.get("updates_enabled") is not False
            or plist.get("CFBundleShortVersionString") != version
            or plist.get("CFBundleVersion") != info.get("build_number")):
        raise DevelopmentError("Development build identity or update isolation mismatch")
    verify_signature(app, team)
    for flag in ("--check-for-updates", "--install-update"):
        code, reply = run([executable, flag])
        result = json.loads(reply)
        if code != 1 or result.get("enabled") is not False or result.get("error") != "updates_disabled":
            raise DevelopmentError("Development update command was not blocked")
    return info


def sha(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app", type=Path, required=True)
    parser.add_argument("--arch", choices=("arm64", "amd64"), required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--commit")
    parser.add_argument("--branch")
    parser.add_argument("--version")
    parser.add_argument("--team-id", help="Require a Developer ID signature from this team")
    args = parser.parse_args()
    config = json.loads((ROOT / "mygo.json").read_text())
    if args.version:
        config["version"] = args.version
    code, commit = run(["git", "-C", ROOT, "rev-parse", "HEAD"])
    commit = commit.decode().strip()
    if code or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise DevelopmentError("Development source revision unavailable")
    code, branch = run(["git", "-C", ROOT, "rev-parse", "--abbrev-ref", "HEAD"])
    if code:
        raise DevelopmentError("Development source branch unavailable")
    commit = args.commit or commit
    branch = args.branch or branch.decode().strip()
    info = identity(args.app, config, commit, branch, args.team_id)
    args.out.mkdir(parents=True, exist_ok=False)
    archive = args.out / ("ytmgo-" + info["version"] + "-dev-" + commit[:12] + "-darwin-" + args.arch + ".zip")
    code, _ = run(["ditto", "-c", "-k", "--keepParent", args.app, archive])
    if code:
        raise DevelopmentError("Development archive creation failed")
    receipt = args.out / "build.json"
    receipt.write_text(json.dumps({"build": info, "architecture": args.arch,
        "artifact_only": True, "code_signature_verified": True,
        "developer_id_signed": args.team_id is not None, "team_identifier": args.team_id,
        "notarized": False,
        "archive": archive.name, "archive_sha256": sha(archive),
        "run_id": os.environ.get("GITHUB_RUN_ID", "local")}, indent=2) + "\n")
    (args.out / "SHA256SUMS").write_text("".join(sha(path) + "  " + path.name + "\n" for path in (archive, receipt)))
    print("Verified development artifact; updates disabled; no release publication")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error) if isinstance(error, DevelopmentError) else "Development artifact verification failed", file=sys.stderr)
        sys.exit(1)
