"""Check every shipped Mach-O target, then optionally smoke a native CI app."""
import argparse
import json
import os
from pathlib import Path
import platform
import plistlib
import re
import subprocess
import sys
import time

import macos


def version_tuple(value):
    if not re.fullmatch(r"[0-9]+(?:\.[0-9]+){1,2}", value):
        raise macos.PackageError("Invalid deployment version")
    return tuple((list(map(int, value.split("."))) + [0, 0])[:3])


def inspect(app, arch, minimum):
    config = json.loads((macos.ROOT / "mygo.json").read_text())
    plist = plistlib.loads((app / "Contents/Info.plist").read_bytes())
    if (plist.get("CFBundleShortVersionString") != config["version"]
            or version_tuple(plist.get("LSMinimumSystemVersion", "")) != version_tuple(minimum)):
        raise macos.PackageError("App version or minimum macOS mismatch")
    cpu = {"arm64": "arm64", "amd64": "x86_64"}[arch]
    records = []
    for binary in macos.macho_files(app):
        actual = macos.run(["lipo", "-archs", binary], "Mach-O architecture inspection", capture=True).decode().split()
        if actual != [cpu]:
            raise macos.PackageError("Embedded architecture mismatch")
        build = macos.run(["xcrun", "vtool", "-show-build", binary], "Mach-O deployment inspection", capture=True).decode()
        targets = re.findall(r"^\s*minos\s+([0-9.]+)\s*$", build, re.MULTILINE)
        if not targets or any(version_tuple(target) > version_tuple(minimum) for target in targets):
            raise macos.PackageError("Embedded binary requires newer macOS")
        libraries = macos.run(["otool", "-L", binary], "Mach-O library inspection", capture=True).decode().splitlines()[1:]
        if any(not line.strip().startswith(("/usr/lib/", "/System/Library/")) for line in libraries if line.strip()):
            raise macos.PackageError("Non-system native dependency")
        records.append({"path": binary.relative_to(app).as_posix(), "architecture": cpu, "minimum_macos": targets[0]})
    if len(records) < 4:
        raise macos.PackageError("Missing native bundle components")
    for nested in app.rglob("Info.plist"):
        nested_plist = plistlib.loads(nested.read_bytes())
        if "LSMinimumSystemVersion" in nested_plist and version_tuple(nested_plist["LSMinimumSystemVersion"]) > version_tuple(minimum):
            raise macos.PackageError("Nested bundle requires newer macOS")
    return records


def smoke(app, arch, signed):
    native = {"arm64": "arm64", "x86_64": "amd64"}.get(platform.machine())
    if os.environ.get("GITHUB_ACTIONS") != "true" or native != arch:
        raise macos.PackageError("Smoke requires a fresh native CI runner")
    data = Path.home() / "Library/Application Support/ytmgo"
    if data.exists():
        raise macos.PackageError("Smoke requires an empty runner profile")
    if signed:
        macos.run(["codesign", "--verify", "--deep", "--strict", app], "Signed native app verification")
        macos.run(["xcrun", "stapler", "validate", app], "Native stapled ticket verification")
        macos.run(["spctl", "--assess", "--type", "execute", app], "Native Gatekeeper assessment")
    executable = app / "Contents/MacOS/ytmGO"
    config = json.loads((macos.ROOT / "mygo.json").read_text())
    if macos.run([executable, "--version"], "Native app CLI", capture=True).decode().strip() != config["version"]:
        raise macos.PackageError("Native app CLI version mismatch")
    resources = app / "Contents/Resources"
    macos.run([resources / "runtime/ytmgo-playback-host", "check"], "Native script host check")
    macos.run([resources / "runtime/ffmpeg", "-version"], "Native decoder check")
    login = json.loads((resources / "login/manifest.json").read_text())
    reply = macos.run([resources / "login" / login["executable"]], "Native login helper check", capture=True, input_bytes=b"")
    if json.loads(reply).get("status") != "cancelled":
        raise macos.PackageError("Native login helper protocol mismatch")
    process = subprocess.Popen([str(executable)], cwd=app.parent, stdin=subprocess.DEVNULL,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise macos.PackageError("Native app exited during smoke")
            time.sleep(.2)
        if not (data / "native-diagnostics/audio-status.json").is_file():
            raise macos.PackageError("Native app did not initialize")
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app", type=Path, required=True)
    parser.add_argument("--arch", choices=("arm64", "amd64"), required=True)
    parser.add_argument("--smoke", action="store_true")
    parser.add_argument("--signed", action="store_true")
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--receipt", type=Path, required=True)
    args = parser.parse_args()
    try:
        cfg = json.loads((macos.ROOT / "mygo.json").read_text())
        records = inspect(args.app, args.arch, cfg["macos"]["minimumSystemVersion"])
        if args.smoke:
            smoke(args.app, args.arch, args.signed)
        args.receipt.parent.mkdir(parents=True, exist_ok=True)
        args.receipt.write_text(json.dumps({"version": cfg["version"], "commit": os.environ.get("GITHUB_SHA", "local"),
            "architecture": args.arch, "minimum_macos": cfg["macos"]["minimumSystemVersion"],
            "host_macos": platform.mac_ver()[0], "native_smoke": args.smoke,
            "signed": args.signed, "archive_sha256": macos.digest(args.archive) if args.archive else None, "binaries": records}, indent=2) + "\n")
        print("Verified macOS deployment and architecture" + ("; native startup passed" if args.smoke else ""))
        return 0
    except Exception as error:
        print(str(error) if isinstance(error, macos.PackageError) else "macOS compatibility check failed", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
