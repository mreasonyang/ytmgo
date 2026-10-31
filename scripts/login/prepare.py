#!/usr/bin/env python3
"""Build and verify the temporary account-login helper for one desktop target.

build/login is a single-target staging directory, copied unchanged by MyGo.
Build on each target OS; this command never installs compilers or SDKs. The
media runtime is separate and its existing preparation remains macOS-only.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import plistlib
import shutil
import struct
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
CRATE = ROOT / "native/account-login"
OUT = ROOT / "build/login"
TARGET_DIR = ROOT / "build/login-target"
BINARY = "ytmgo-account-login"
MAC_BUNDLE = "ytmgo Account Login.app"
MAC_IDENTIFIER = "si.kyun.ytmgo.account-login"
MAC_PLIST = ROOT / "scripts/login/macos/Info.plist"
MAC_ICON = ROOT / "scripts/login/macos/AppIcon.icns"
TARGETS = {
    ("darwin", "arm64"): "aarch64-apple-darwin",
    ("darwin", "amd64"): "x86_64-apple-darwin",
    ("linux", "arm64"): "aarch64-unknown-linux-gnu",
    ("linux", "amd64"): "x86_64-unknown-linux-gnu",
    ("windows", "arm64"): "aarch64-pc-windows-msvc",
    ("windows", "amd64"): "x86_64-pc-windows-msvc",
}


def host_platform():
    system = {"darwin": "darwin", "linux": "linux", "win32": "windows"}.get(sys.platform)
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(platform.machine().lower())
    if (system, arch) not in TARGETS:
        raise RuntimeError("unsupported login-helper build host")
    return system, arch


def parse_platform(value):
    parts = tuple(value.split("/"))
    if parts not in TARGETS:
        raise ValueError("unsupported login-helper target; use OS/ARCH")
    return parts


def executable_name(system):
    return BINARY + (".exe" if system == "windows" else "")


def executable_path(system):
    if system == "darwin":
        return MAC_BUNDLE + "/Contents/MacOS/" + BINARY
    return executable_name(system)


def sign_macos_bundle(bundle):
    result = subprocess.run(
        ["/usr/bin/codesign", "--force", "--sign", "-", str(bundle)],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False,
    )
    if result.returncode:
        raise RuntimeError("login-helper app bundle signing failed")


def verify_macos_bundle(bundle):
    result = subprocess.run(
        ["/usr/bin/codesign", "--verify", "--strict", str(bundle)],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False,
    )
    if result.returncode:
        raise RuntimeError("login-helper app bundle signature is invalid")


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def binary_platform(path):
    """Read only executable headers, without running an unverified artifact."""
    with path.open("rb") as stream:
        header = stream.read(64)
        if len(header) < 64:
            raise ValueError("truncated login-helper executable")
        if header[:4] == b"\xcf\xfa\xed\xfe":
            arch = {0x01000007: "amd64", 0x0100000C: "arm64"}.get(struct.unpack_from("<I", header, 4)[0])
            system = "darwin"
        elif header[:4] == b"\x7fELF" and header[4:6] == b"\x02\x01":
            arch = {62: "amd64", 183: "arm64"}.get(struct.unpack_from("<H", header, 18)[0])
            system = "linux"
        elif header[:2] == b"MZ":
            offset = struct.unpack_from("<I", header, 60)[0]
            if offset > 1024 * 1024:
                raise ValueError("invalid login-helper PE header")
            stream.seek(offset)
            pe = stream.read(6)
            if len(pe) != 6 or pe[:4] != b"PE\0\0":
                raise ValueError("invalid login-helper PE signature")
            arch = {0x8664: "amd64", 0xAA64: "arm64"}.get(struct.unpack_from("<H", pe, 4)[0])
            system = "windows"
        else:
            raise ValueError("unsupported login-helper executable format")
    if arch is None:
        raise ValueError("unsupported login-helper executable architecture")
    return system, arch


def verify(folder, target):
    manifest_path = folder / "manifest.json"
    if manifest_path.is_symlink() or manifest_path.stat().st_size > 16384:
        raise ValueError("invalid login-helper manifest")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if not isinstance(manifest, dict) or type(manifest.get("schema")) is not int:
        raise ValueError("invalid login-helper manifest")
    system, arch = target
    name = executable_path(system)
    if (manifest.get("schema"), manifest.get("platform"), manifest.get("architecture"), manifest.get("executable")) != (1, system, arch, name):
        raise ValueError("login-helper manifest target mismatch")
    binary = folder / name
    if binary.is_symlink() or not binary.is_file() or binary_platform(binary) != target:
        raise ValueError("login-helper executable target mismatch")
    if digest(binary) != manifest.get("sha256"):
        raise ValueError("login-helper executable checksum mismatch")
    expected = {name, "manifest.json"}
    if system == "darwin":
        expected.update({MAC_BUNDLE + "/Contents/Info.plist", MAC_BUNDLE + "/Contents/_CodeSignature/CodeResources",
                         MAC_BUNDLE + "/Contents/Resources/AppIcon.icns"})
    files = set()
    for path in folder.rglob("*"):
        if path.is_symlink():
            raise ValueError("unexpected symlink in login-helper staging directory")
        if path.is_file():
            files.add(path.relative_to(folder).as_posix())
    if files != expected:
        raise ValueError("unexpected files in login-helper staging directory")
    if system == "darwin":
        bundle = folder / MAC_BUNDLE
        plist = plistlib.loads((bundle / "Contents/Info.plist").read_bytes())
        if not isinstance(plist, dict) or plist.get("CFBundleIdentifier") != MAC_IDENTIFIER or plist.get("CFBundleExecutable") != BINARY or plist.get("CFBundlePackageType") != "APPL" or plist.get("LSUIElement") is not False or plist.get("LSBackgroundOnly", False) is not False or plist.get("CFBundleIconFile") != "AppIcon.icns":
            raise ValueError("login-helper app bundle identity mismatch")
        if digest(bundle / "Contents/Resources/AppIcon.icns") != digest(MAC_ICON):
            raise ValueError("login-helper app icon mismatch")
        verify_macos_bundle(bundle)
    return manifest


def stage(binary, target, lockfile, destination=OUT):
    if binary_platform(binary) != target:
        raise ValueError("compiled login-helper target mismatch")
    system, arch = target
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=".login-stage-", dir=destination.parent))
    backup = destination.with_name(".login-previous")
    try:
        name = executable_path(system)
        executable = temporary / name
        executable.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(binary, executable)
        executable.chmod(0o755)
        if system == "darwin":
            bundle = temporary / MAC_BUNDLE
            shutil.copyfile(MAC_PLIST, bundle / "Contents/Info.plist")
            resources = bundle / "Contents/Resources"
            resources.mkdir()
            shutil.copyfile(MAC_ICON, resources / "AppIcon.icns")
            sign_macos_bundle(bundle)
        manifest = {
            "schema": 1,
            "platform": system,
            "architecture": arch,
            "target": TARGETS[target],
            "executable": name,
            "sha256": digest(executable),
            "cargo_lock_sha256": digest(lockfile),
        }
        (temporary / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        verify(temporary, target)
        # Keep the old target until the complete new stage passes validation.
        if backup.exists():
            raise RuntimeError("unfinished login-helper stage replacement")
        if destination.exists():
            destination.rename(backup)
        try:
            temporary.rename(destination)
        except OSError:
            if backup.exists():
                backup.rename(destination)
            raise
        if backup.exists():
            shutil.rmtree(backup)
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)


def build(target):
    host = host_platform()
    if target[0] != host[0]:
        raise RuntimeError("build the login helper on its target operating system")
    if not (CRATE / "Cargo.lock").is_file():
        raise RuntimeError("login-helper Cargo.lock is required; dependencies must be locked")
    cargo = shutil.which("cargo")
    if not cargo:
        raise RuntimeError("Rust cargo is required to build the login helper")
    env = os.environ.copy()
    env["CARGO_TARGET_DIR"] = str(TARGET_DIR)
    env["CARGO_BUILD_JOBS"] = "2"
    result = subprocess.run(
        [cargo, "build", "--manifest-path", str(CRATE / "Cargo.toml"), "--release", "--locked", "--no-default-features",
         "--target", TARGETS[target], "--bin", BINARY],
        cwd=ROOT, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False,
    )
    if result.returncode:
        raise RuntimeError("login-helper Cargo build failed; check the target toolchain and platform dependencies")
    stage(TARGET_DIR / TARGETS[target] / "release" / executable_name(target[0]), target, CRATE / "Cargo.lock")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", type=parse_platform, help="target OS/ARCH; defaults to this machine")
    parser.add_argument("--verify", action="store_true", help="verify the staged helper without compiling")
    args = parser.parse_args()
    try:
        target = args.platform or host_platform()
        if not args.verify:
            build(target)
        verify(OUT, target)
    except (OSError, ValueError, RuntimeError, plistlib.InvalidFileException) as error:
        # Do not forward compiler logs, command environments or arbitrary file
        # contents into build reports. These messages contain only categories.
        if isinstance(error, RuntimeError):
            message = str(error)
        elif isinstance(error, ValueError) and not isinstance(error, json.JSONDecodeError):
            message = str(error)
        else:
            message = "login-helper staging data is missing or invalid"
        print("Login helper: " + message, file=sys.stderr)
        return 1
    print("Verified login helper for " + "/".join(target), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
