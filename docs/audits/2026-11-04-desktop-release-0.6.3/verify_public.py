"""Independently verify supplemental public packages, without credentials."""
import hashlib
import importlib.util
import json
from pathlib import Path, PurePosixPath
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
DOWNLOADS = ROOT / "build/desktop-release-0.6.3-public"
TAG = "v0.6.3"
COMMIT = "a582d25bb3f5b793f4d3e57160db3e4c90f86099"
BASE = "https://github.com/mreasonyang/ytmgo/releases/download/" + TAG + "/"


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def get(url):
    return urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "ytmgo-public-package-verification"}), timeout=90)


def main():
    desktop = load("desktop", ROOT / "scripts/release/desktop.py")
    login = load("login", ROOT / "scripts/login/prepare.py")
    with get("https://api.github.com/repos/mreasonyang/ytmgo/releases/tags/" + TAG) as response:
        release = json.load(response)
    assert release["target_commitish"] == COMMIT and not release["draft"] and not release["prerelease"]
    assets = {a["name"]: a for a in release["assets"]}
    original = json.loads((ROOT / "docs/audits/2026-11-04-release-0.6.3/public-verification.json").read_text())["sha256"]
    assert all(assets[name]["digest"] == "sha256:" + digest for name, digest in original.items())
    DOWNLOADS.mkdir(exist_ok=True)
    digests, platforms = {}, {}
    for which in desktop.PLATFORMS:
        names = desktop.asset_names("0.6.3", which, True)
        for name in names:
            asset = assets[name]
            assert asset["browser_download_url"] == BASE + name
            path = DOWNLOADS / name
            if not path.exists():
                part = path.with_suffix(path.suffix + ".partial")
                with get(BASE + name) as source, part.open("wb") as dest:
                    while chunk := source.read(1024 * 1024):
                        dest.write(chunk)
                part.replace(path)
            digest = desktop.digest(path)
            assert path.stat().st_size == asset["size"] and asset["digest"] == "sha256:" + digest
            digests[name] = digest
        metadata = json.loads((DOWNLOADS / ("build-" + which + ".json")).read_text())
        assert metadata["commit"] == COMMIT and metadata["platform"] == which and metadata["version"] == "0.6.3"
        assert all(metadata[key] is True for key in ("native_build", "native_smoke", "signed_updates"))
        assert metadata["installed_cli_version"] == "0.6.3"
        desktop.run(["go", "run", "./cmd/sign-desktop-update", "--verify", "--out", DOWNLOADS, "--platform", which])
        with get("https://github.com/mreasonyang/ytmgo/releases/latest/download/update-" + which + ".json") as response:
            assert response.read() == (DOWNLOADS / ("update-" + which + ".json")).read_bytes()
        archive = DOWNLOADS / ("ytmgo-0.6.3-" + which + ".tar.gz")
        with tempfile.TemporaryDirectory() as temporary, tarfile.open(archive) as tar:
            members = tar.getmembers()
            files = set()
            for member in members:
                parts = PurePosixPath(member.name).parts
                assert member.isfile() and member.size < 128 * 1024 * 1024
                assert parts and not member.name.startswith("/") and ".." not in parts and "\\" not in member.name and ":" not in member.name
                assert member.name not in files
                files.add(member.name)
            assert len(files) == metadata["bundled_files"]
            suffix = ".exe" if which.startswith("windows") else ""
            for name in ("ytmGO.exe" if suffix else "ytmgo", "runtime/ffmpeg" + suffix,
                         "runtime/ytmgo-playback-host" + suffix, "login/ytmgo-account-login" + suffix):
                member = tar.getmember(name)
                binary = Path(temporary) / Path(name).name
                with tar.extractfile(member) as source, binary.open("wb") as dest:
                    while chunk := source.read(1024 * 1024):
                        dest.write(chunk)
                assert login.binary_platform(binary) == (which.split("-")[0], "amd64")
                if not suffix:
                    assert member.mode & 0o111
            assert {"runtime/manifest.json", "login/manifest.json", "legal/manifest.json"}.issubset(files)
            if suffix:
                with zipfile.ZipFile(DOWNLOADS / "ytmgo-0.6.3-windows-amd64.zip") as zip_file:
                    assert set(zip_file.namelist()) == files
        platforms[which] = {key: metadata[key] for key in ("version", "commit", "packaging_tools_commit", "license_notice_overlay", "native_build", "native_smoke", "signed_updates", "os_code_signed", "bundled_files", "installed_cli_version", "smoke_scope", "run_url")}
    desktop.validate(DOWNLOADS, {"release_tag": TAG, "source_sha": COMMIT})
    result = {"release_tag": TAG, "application_source": COMMIT, "new_assets_verified": len(digests),
              "original_mac_asset_digests_unchanged": len(original), "sha256": digests,
              "platforms": platforms, "both_update_signatures_verified": True, "both_stable_feeds_match": True}
    (OUT / "public-verification.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception:
        raise SystemExit("Desktop public-package verification failed; raw diagnostics omitted")
