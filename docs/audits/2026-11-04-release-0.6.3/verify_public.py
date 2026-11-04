"""Verify anonymous public downloads; persist only release and integrity metadata."""
import hashlib
import json
from pathlib import Path
import plistlib
import re
import subprocess
import urllib.request

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
DOWNLOADS = ROOT / "build/release-0.6.3-public"
VERSION = "0.6.3"
COMMIT = "a582d25bb3f5b793f4d3e57160db3e4c90f86099"
BASE = "https://github.com/mreasonyang/ytmgo/releases/download/v" + VERSION + "/"


def sha(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def get(url):
    request = urllib.request.Request(url, headers={"User-Agent": "ytmgo-release-verification"})
    return urllib.request.urlopen(request, timeout=60)


def command(args):
    result = subprocess.run(list(map(str, args)), cwd=ROOT, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, timeout=180)
    if result.returncode:
        raise ValueError("public_integrity_command_failed")
    return result.stdout


def main():
    DOWNLOADS.mkdir(exist_ok=True)
    with get("https://api.github.com/repos/mreasonyang/ytmgo/releases/tags/v" + VERSION) as response:
        release = json.load(response)
    assert release["draft"] is False and release["prerelease"] is False
    assert release["target_commitish"] == COMMIT
    names = ["ytmgo-" + VERSION + "-arm64-notarized.zip",
             "ytmgo-" + VERSION + "-arm64-notarized.dmg",
             "ytmgo-" + VERSION + "-darwin-arm64.tar.gz",
             "build.json", "update-darwin-arm64.json", "SHA256SUMS"]
    assets = {a["name"]: a for a in release["assets"]}
    assert set(assets) == set(names)
    digests = {}
    for name in names:
        asset = assets[name]
        assert asset["browser_download_url"] == BASE + name
        path = DOWNLOADS / name
        if not path.exists():
            with get(BASE + name) as source, path.open("xb") as dest:
                while chunk := source.read(1024 * 1024):
                    dest.write(chunk)
        digest = sha(path)
        assert path.stat().st_size == asset["size"] and asset["digest"] == "sha256:" + digest
        digests[name] = digest
    sums = {}
    for line in (DOWNLOADS / "SHA256SUMS").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._-]+)", line)
        assert match and match[2] not in sums
        sums[match[2]] = match[1]
    assert set(sums) == set(names) - {"SHA256SUMS"}
    assert all(digests[name] == digest for name, digest in sums.items())
    metadata = json.loads((DOWNLOADS / "build.json").read_text())
    assert metadata["commit"] == COMMIT and metadata["version"] == VERSION
    assert metadata["architecture"] == "arm64" and metadata["minimum_macos"] == "27.0"
    assert all(metadata[key] is True for key in ("developer_id_signed", "notarized", "signed_updates"))
    with get("https://github.com/mreasonyang/ytmgo/releases/latest/download/update-darwin-arm64.json") as response:
        assert response.read() == (DOWNLOADS / "update-darwin-arm64.json").read_bytes()
    command(["go", "run", "./cmd/update-package", "--verify", "--out", DOWNLOADS])
    command(["ditto", "-x", "-k", DOWNLOADS / names[0], DOWNLOADS])
    app = DOWNLOADS / "ytmGO.app"
    plist = plistlib.loads((app / "Contents/Info.plist").read_bytes())
    assert plist["CFBundleShortVersionString"] == VERSION and plist["LSMinimumSystemVersion"] == "27.0"
    binary = app / "Contents/MacOS" / plist["CFBundleExecutable"]
    assert command([binary, "--version"]).decode().strip() == VERSION
    for args in (["codesign", "--verify", "--deep", "--strict", app],
                 ["xcrun", "stapler", "validate", app],
                 ["spctl", "--assess", "--type", "execute", app]):
        command(args)
    dmg = DOWNLOADS / names[1]
    for args in (["hdiutil", "verify", dmg], ["codesign", "--verify", "--strict", dmg],
                 ["xcrun", "stapler", "validate", dmg],
                 ["spctl", "--assess", "--type", "open", "--context", "context:primary-signature", dmg]):
        command(args)
    result = {"version": VERSION, "source_commit": COMMIT, "github_asset_digests_verified": len(names),
              "sha256": digests, "stable_feed_matches_versioned": True, "ed25519_verified": True,
              "app_signature_ticket_gatekeeper": True, "dmg_signature_ticket_gatekeeper": True,
              "cli_version": VERSION, "executable_sha256": sha(binary)}
    (OUT / "public-verification.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception:
        raise SystemExit("Public release verification failed; no raw diagnostics retained")
