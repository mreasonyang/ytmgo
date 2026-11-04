"""Native Windows/Linux packaging; fixed diagnostics and immutable release assets."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[2]
REPO = "mreasonyang/ytmgo"
TAG = re.compile(r"v0\.6\.(?:0|[1-9][0-9]{0,2})")
PLATFORMS = ("windows-amd64", "linux-amd64")


class DesktopError(Exception):
    pass


def run(args, cwd=ROOT, env=None, input_bytes=None, timeout=1800):
    result = subprocess.run(list(map(str, args)), cwd=cwd, env=env, input=input_bytes,
                            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=timeout)
    if result.returncode:
        raise DesktopError("desktop_command_failed")
    return result.stdout


def gh(args):
    return run(["gh", *args], timeout=300)


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            h.update(chunk)
    return h.hexdigest()


def public(url):
    request = urllib.request.Request(url, headers={"User-Agent": "ytmgo-desktop-release"})
    with urllib.request.urlopen(request, timeout=60) as response:
        data = response.read(1048577)
    if len(data) > 1048576:
        raise DesktopError("public_metadata_too_large")
    return data


def resolve(tag, expected=""):
    endpoint = "repos/" + REPO + "/releases/" + ("tags/" + tag if tag else "latest")
    if tag and not TAG.fullmatch(tag):
        raise DesktopError("invalid_desktop_release_tag")
    release = json.loads(gh(["api", endpoint]))
    tag = release["tag_name"]
    if not TAG.fullmatch(tag) or release["draft"] or release["prerelease"]:
        raise DesktopError("public_stable_release_required")
    asset = next(a for a in release["assets"] if a["name"] == "build.json")
    url = "https://github.com/" + REPO + "/releases/download/" + tag + "/build.json"
    if asset["browser_download_url"] != url:
        raise DesktopError("unexpected_release_metadata_url")
    data = public(url)
    if asset.get("digest") != "sha256:" + hashlib.sha256(data).hexdigest():
        raise DesktopError("public_metadata_digest_mismatch")
    metadata = json.loads(data)
    commit = metadata.get("commit", "")
    if (metadata.get("version") != tag[1:] or not re.fullmatch(r"[0-9a-f]{40}", commit)
            or metadata.get("notarized") is not True or release.get("target_commitish") != commit):
        raise DesktopError("release_source_mismatch")
    complete = all(name in {a["name"] for a in release["assets"]}
                   for which in PLATFORMS for name in asset_names(tag[1:], which, True))
    enabled = (not expected or expected == commit) and not complete
    return {"release_tag": tag, "source_sha": commit, "enabled": str(enabled).lower()}


def target():
    arch = platform.machine().lower()
    if arch not in ("amd64", "x86_64"):
        raise DesktopError("native_x64_build_host_required")
    value = {"win32": "windows-amd64", "linux": "linux-amd64"}.get(sys.platform)
    if value not in PLATFORMS:
        raise DesktopError("native_windows_or_linux_host_required")
    return value


def config(source):
    value = json.loads((source / "mygo.json").read_text())
    if not TAG.fullmatch("v" + value["version"]) or value["name"] != "ytmGO":
        raise DesktopError("unsupported_desktop_configuration")
    return value


def asset_names(version, which, signed=False):
    prefix = "ytmgo-" + version
    names = [prefix + "-" + which + ".tar.gz", "build-" + which + ".json"]
    if which == "windows-amd64":
        names += [prefix + "-windows-amd64.zip", "ytmgo-setup-" + version + "-amd64.exe"]
    elif which == "linux-amd64":
        names += ["ytmgo_" + version + "_amd64.deb"]
    else:
        raise DesktopError("unsupported_desktop_target")
    if signed:
        names += ["update-" + which + ".json", "SHA256SUMS-" + which]
    return names


def entries(folder, version, which):
    excluded = {"ytmgo-setup-" + version + "-amd64.exe", "install.sh", "install.ps1"}
    result = []
    for path in sorted(folder.rglob("*")):
        relative = path.relative_to(folder)
        if relative.parts[0] in excluded or relative.parts[0].endswith((".deb", ".tar.gz")):
            continue
        if path.is_symlink():
            raise DesktopError("unexpected_bundle_symlink")
        if path.is_file():
            result.append(path)
    required = {"runtime/manifest.json", "login/manifest.json", "legal/manifest.json",
                "ytmGO.exe" if which == "windows-amd64" else "ytmgo"}
    if not required.issubset({p.relative_to(folder).as_posix() for p in result}):
        raise DesktopError("incomplete_desktop_bundle")
    return result


def stage_notices(source):
    """Supplement only pinned legal data; keep the released application code."""
    config(source)
    index_path = source / "scripts/licensing/upstream-notices.json"
    previous = json.loads(index_path.read_text())
    current = json.loads((ROOT / "scripts/licensing/upstream-notices.json").read_text())
    overlay = []
    for identity, record in current.items():
        if identity in previous:
            if previous[identity] != record:
                raise DesktopError("existing_pinned_notice_changed")
            continue
        for item in [record] + ([record["terms"]] if record.get("terms") else []):
            relative = Path(item["path"])
            if relative.is_absolute() or ".." in relative.parts or not relative.parts or relative.parts[0] != "upstream":
                raise DesktopError("invalid_pinned_notice_path")
            path = ROOT / "scripts/licensing" / relative
            if digest(path) != item["sha256"]:
                raise DesktopError("pinned_notice_checksum_mismatch")
            destination = source / "scripts/licensing" / relative
            if destination.exists() and digest(destination) != item["sha256"]:
                raise DesktopError("existing_notice_file_changed")
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, destination)
            overlay.append({"path": "scripts/licensing/" + relative.as_posix(), "sha256": item["sha256"]})
        previous[identity] = record
    if overlay:
        index_path.write_text(json.dumps(previous, indent=2, sort_keys=True) + "\n")
        overlay.append({"path": "scripts/licensing/upstream-notices.json", "sha256": digest(index_path)})
    overlay = list({item["path"]: item for item in overlay}.values())
    (source / "build").mkdir(exist_ok=True)
    (source / "build/desktop-notice-overlay.json").write_text(json.dumps(overlay, indent=2) + "\n")
    print("Staged checksum-pinned supplemental license notices", flush=True)


def package(source, out):
    which = target()
    cfg = config(source)
    version = cfg["version"]
    commit = run(["git", "rev-parse", "HEAD"], source).decode().strip()
    expected = os.environ.get("DESKTOP_SOURCE_SHA", commit)
    if commit != expected or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise DesktopError("desktop_checkout_mismatch")
    bundle = source / "build" / which
    files = entries(bundle, version, which)
    spec = importlib.util.spec_from_file_location("desktop_login", source / "scripts/login/prepare.py")
    login = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(login)
    suffix = ".exe" if which.startswith("windows") else ""
    for relative in ("ytmGO.exe" if suffix else "ytmgo", "runtime/ffmpeg" + suffix,
                     "runtime/ytmgo-playback-host" + suffix, "login/ytmgo-account-login" + suffix):
        if login.binary_platform(bundle / relative) != (which.split("-")[0], "amd64"):
            raise DesktopError("desktop_executable_architecture_mismatch")
    run([sys.executable, "scripts/licensing/prepare.py", "--verify-build"], source)
    out.mkdir(parents=True, exist_ok=True)
    archive = out / ("ytmgo-" + version + "-" + which + ".tar.gz")
    with tarfile.open(archive, "w:gz") as tar:
        for path in files:
            tar.add(path, arcname=path.relative_to(bundle).as_posix(), recursive=False)
    if suffix:
        with zipfile.ZipFile(out / ("ytmgo-" + version + "-windows-amd64.zip"), "w", zipfile.ZIP_DEFLATED) as zip_file:
            for path in files:
                zip_file.write(path, path.relative_to(bundle).as_posix())
        name = "ytmgo-setup-" + version + "-amd64.exe"
    else:
        name = "ytmgo_" + version + "_amd64.deb"
    shutil.copy2(bundle / name, out / name)
    metadata = {"version": version, "commit": commit, "platform": which, "architecture": "amd64",
                "native_build": True, "os_code_signed": False, "signed_updates": False,
                "bundled_files": len(files), "native_smoke": False,
                "packaging_tools_commit": run(["git", "rev-parse", "HEAD"]).decode().strip(),
                "license_notice_overlay": json.loads((source / "build/desktop-notice-overlay.json").read_text()),
                "run_url": "https://github.com/" + REPO + "/actions/runs/" + os.environ.get("GITHUB_RUN_ID", "local")}
    (out / ("build-" + which + ".json")).write_text(json.dumps(metadata, indent=2) + "\n")
    print("Packaged native desktop target: " + which, flush=True)


def smoke(source, out):
    which, version = target(), config(source)["version"]
    install = source / "build" / ("installed-" + which)
    install.mkdir(exist_ok=True)
    if which == "windows-amd64":
        setup = out / ("ytmgo-setup-" + version + "-amd64.exe")
        run([setup, "/S", "/D=" + str(install)], timeout=180)
        executable = install / "ytmGO.exe"
    else:
        deb = out / ("ytmgo_" + version + "_amd64.deb")
        run(["sudo", "apt-get", "install", "-y", str(deb)], timeout=300)
        executable = Path("/opt/ytmgo/ytmgo")
    if run([executable, "--version"]).decode().strip() != version:
        raise DesktopError("installed_desktop_version_mismatch")
    run([sys.executable, "scripts/licensing/prepare.py", "--bundle", executable.parent], source)
    suffix = ".exe" if which.startswith("windows") else ""
    run([executable.parent / "runtime" / ("ytmgo-playback-host" + suffix), "check"])
    run([executable.parent / "runtime" / ("ffmpeg" + suffix), "-version"])
    state = source / "build" / "desktop-smoke-profile"
    state.mkdir(exist_ok=True)
    env = os.environ.copy()
    if which.startswith("windows"):
        env.update(APPDATA=str(state / "config"), LOCALAPPDATA=str(state / "local"))
        data = state / "config" / "ytmgo"
    else:
        env.update(XDG_CONFIG_HOME=str(state / "config"), XDG_CACHE_HOME=str(state / "cache"))
        data = state / "config" / "ytmgo"
    process = subprocess.Popen([str(executable)], cwd=executable.parent, env=env,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise DesktopError("installed_desktop_exited_during_smoke")
            time.sleep(.2)
        if not (data / "native-diagnostics/audio-status.json").is_file():
            raise DesktopError("installed_desktop_did_not_initialize")
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)
    metadata_path = out / ("build-" + which + ".json")
    metadata = json.loads(metadata_path.read_text())
    metadata.update(native_smoke=True, installed_cli_version=version,
                    smoke_scope="native installer, installed resources, startup in an empty account profile; no signed-in E2E")
    metadata_path.write_text(json.dumps(metadata, indent=2) + "\n")
    print("Installed native desktop smoke passed: " + which, flush=True)


def build(source):
    target()
    run(["go", "tool", "mygo", "build", "-skip-dmg"], source)
    print("Built native desktop app and installer", flush=True)


def sign(source, out):
    cfg = config(source)
    version = cfg["version"]
    key = os.environ.pop("MYGO_UPDATER_PRIVATE_KEY", "")
    if not key:
        raise DesktopError("update_signing_setting_missing")
    for which in PLATFORMS:
        metadata_path = out / ("build-" + which + ".json")
        metadata = json.loads(metadata_path.read_text())
        if metadata.get("version") != version or metadata.get("platform") != which or metadata.get("native_smoke") is not True:
            raise DesktopError("native_package_acceptance_required")
        command = ["go", "run", "./cmd/sign-desktop-update", "--config", source / "mygo.json", "--out", out, "--platform", which]
        run(command, input_bytes=key.encode())
        run([*command, "--verify"])
        metadata["signed_updates"] = True
        metadata_path.write_text(json.dumps(metadata, indent=2) + "\n")
        names = asset_names(version, which, True)
        checksums = out / ("SHA256SUMS-" + which)
        checksums.write_text("".join(digest(out / name) + "  " + name + "\n" for name in names if name != checksums.name))
    print("Signed and independently verified Windows/Linux update archives", flush=True)


def validate(out, release):
    version = release["release_tag"][1:]
    files = []
    for which in PLATFORMS:
        names = asset_names(version, which, True)
        metadata = json.loads((out / ("build-" + which + ".json")).read_text())
        if (metadata.get("version"), metadata.get("commit"), metadata.get("platform")) != (version, release["source_sha"], which):
            raise DesktopError("desktop_release_provenance_mismatch")
        if any(metadata.get(key) is not True for key in ("native_build", "native_smoke", "signed_updates")):
            raise DesktopError("desktop_release_gates_not_passed")
        checksums = {}
        for line in (out / ("SHA256SUMS-" + which)).read_text().splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._-]+)", line)
            if not match or match[2] in checksums:
                raise DesktopError("invalid_desktop_checksums")
            checksums[match[2]] = match[1]
        if set(checksums) != set(names) - {"SHA256SUMS-" + which} or any(digest(out / name) != checksum for name, checksum in checksums.items()):
            raise DesktopError("desktop_release_checksum_mismatch")
        files += [out / name for name in names]
    return files


def publish(out, tag, source=ROOT):
    release = resolve(tag)
    files = validate(out, release)
    for which in PLATFORMS:
        run(["go", "run", "./cmd/sign-desktop-update", "--verify", "--config", source / "mygo.json",
             "--out", out, "--platform", which])
    endpoint = "repos/" + REPO + "/releases/tags/" + tag
    before = json.loads(gh(["api", endpoint]))
    remote = {a["name"]: a for a in before["assets"]}
    for path in files:
        if path.name in remote:
            # A matching previous upload is resumable; never replace assets.
            if remote[path.name].get("digest") != "sha256:" + digest(path):
                raise DesktopError("existing_release_asset_differs")
    for path in files:
        if path.name not in remote:
            gh(["release", "upload", tag, "--repo", REPO, str(path)])
    after = json.loads(gh(["api", endpoint]))
    remote_after = {a["name"]: a for a in after["assets"]}
    for name, asset in remote.items():
        if remote_after[name]["digest"] != asset["digest"]:
            raise DesktopError("existing_release_asset_changed")
    if any(remote_after[p.name].get("digest") != "sha256:" + digest(p) for p in files):
        raise DesktopError("uploaded_desktop_checksum_mismatch")
    marker = "## Windows and Linux downloads"
    if marker not in before["body"]:
        body = before["body"].replace("This release targets macOS arm64.", "The release now includes macOS arm64, Windows x64 and Linux x64 packages.")
        body += ("\n\n" + marker + "\n\nWindows x64: use the setup EXE or portable ZIP. WebView2 Runtime is required for sign-in and non-Premium playback. "
                 "The Windows installer is not Authenticode signed.\n\nLinux x64: use the DEB on Ubuntu 24.04 or compatible systems, or extract the Linux tar.gz. "
                 "GTK 3, WebKitGTK 4.1 (2.50.2+), Secret Service and a desktop display/audio session are required.\n\n"
                 "Both platforms include their native playback/login helpers, licenses and corresponding sources. "
                 "Their update archives carry Ed25519 signatures; installer/startup smoke tests ran on the corresponding OS. "
                 "Signed-in account/audio/UI E2E and long-running acceptance are separate and have not been claimed.\n")
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "notes.md"
            path.write_text(body)
            gh(["release", "edit", tag, "--repo", REPO, "--notes-file", str(path)])
    print("Published verified Windows/Linux assets without replacing existing files", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("resolve", "notices", "build", "package", "smoke", "sign", "publish"))
    parser.add_argument("--source", type=Path, default=ROOT)
    parser.add_argument("--out", type=Path, default=ROOT / "dist/desktop")
    parser.add_argument("--tag", default="")
    args = parser.parse_args()
    source, out = args.source.resolve(), args.out.resolve()
    if args.command == "resolve":
        values = resolve(args.tag, os.environ.get("EXPECTED_MAC_SOURCE", ""))
        values["tools_sha"] = run(["git", "rev-parse", "HEAD"]).decode().strip()
        if output := os.environ.get("GITHUB_OUTPUT"):
            with open(output, "a") as stream:
                for key, value in values.items():
                    stream.write(key + "=" + value + "\n")
        print(json.dumps(values), flush=True)
    elif args.command == "notices":
        stage_notices(source)
    elif args.command == "build":
        build(source)
    elif args.command == "package":
        package(source, out)
    elif args.command == "smoke":
        smoke(source, out)
    elif args.command == "sign":
        sign(source, out)
    else:
        publish(out, args.tag, source)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        message = str(error) if isinstance(error, DesktopError) else "desktop_release_operation_failed"
        raise SystemExit("Desktop release: " + message)
