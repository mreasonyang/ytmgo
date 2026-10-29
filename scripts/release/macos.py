#!/usr/bin/env python3
"""Package a complete macOS arm64 build; never emit raw signing diagnostics."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[2]
SECRET_NAMES = (
    "MACOS_CERTIFICATE_P12_BASE64", "MACOS_CERTIFICATE_PASSWORD",
    "APPLE_API_KEY_P8_BASE64", "APPLE_API_KEY_ID", "APPLE_API_ISSUER_ID",
)
MACH_HEADERS = {bytes.fromhex(value) for value in (
    "feedface", "cefaedfe", "feedfacf", "cffaedfe", "cafebabe", "bebafeca",
    "cafebabf", "bfbafeca",
)}


class PackageError(Exception):
    pass


def run(args, stage, capture=False):
    # Credentials go only to the intended Apple tools. Child build/packaging
    # commands do not inherit the signing secrets, and no command or raw output
    # is logged. CalledProcessError must never escape with its argv attached.
    child_env = {key: value for key, value in os.environ.items() if key not in SECRET_NAMES}
    result = subprocess.run(
        [str(arg) for arg in args], cwd=ROOT, env=child_env,
        stdout=subprocess.PIPE if capture else subprocess.DEVNULL,
        stderr=subprocess.DEVNULL, check=False,
    )
    if result.returncode:
        raise PackageError(stage + " failed")
    return result.stdout if capture else b""


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def required(name):
    value = os.environ.get(name, "")
    if not value:
        raise PackageError("Missing setting: " + name)
    return value


def write_secret(name, destination):
    try:
        data = base64.b64decode(required(name), validate=True)
    except ValueError:
        raise PackageError("Invalid base64 setting: " + name) from None
    if not data:
        raise PackageError("Empty file setting: " + name)
    with destination.open("xb") as stream:
        destination.chmod(0o600)
        stream.write(data)


def signing_directory():
    return Path(required("RUNNER_TEMP")) / "ytmgo-signing"


def cleanup():
    directory = signing_directory()
    keychain = directory / "signing.keychain-db"
    if keychain.exists():
        run(["security", "delete-keychain", keychain], "Temporary keychain removal")
    if directory.exists():
        shutil.rmtree(directory)


def import_identity(directory):
    directory.mkdir(mode=0o700)
    certificate = directory / "certificate.p12"
    write_secret("MACOS_CERTIFICATE_P12_BASE64", certificate)
    password = required("MACOS_CERTIFICATE_PASSWORD")
    team = required("APPLE_TEAM_ID")
    if not re.fullmatch(r"[A-Z0-9]{10}", team):
        raise PackageError("Invalid APPLE_TEAM_ID")
    keychain = directory / "signing.keychain-db"
    keychain_password = secrets.token_urlsafe(32)
    run(["security", "create-keychain", "-p", keychain_password, keychain], "Keychain creation")
    run(["security", "set-keychain-settings", "-lut", "7200", keychain], "Keychain configuration")
    run(["security", "unlock-keychain", "-p", keychain_password, keychain], "Keychain unlock")
    run(["security", "import", certificate, "-P", password, "-k", keychain,
         "-T", "/usr/bin/codesign"], "Certificate import")
    certificate.unlink()
    run(["security", "set-key-partition-list", "-S", "apple-tool:,apple:,codesign:",
         "-s", "-k", keychain_password, keychain], "Signing key access configuration")
    identities = run(["security", "find-identity", "-v", "-p", "codesigning", keychain],
                     "Signing identity lookup", capture=True).decode()
    matches = re.findall(r'\b([A-F0-9]{40}) "Developer ID Application: [^"\n]+ \(' + team + r'\)"', identities)
    if len(matches) != 1:
        raise PackageError("Expected exactly one valid Developer ID Application identity for APPLE_TEAM_ID")
    return matches[0], keychain


def macho_files(app):
    paths = []
    for path in app.rglob("*"):
        if path.is_symlink() or not path.is_file():
            continue
        with path.open("rb") as stream:
            if stream.read(4) in MACH_HEADERS:
                paths.append(path)
    return sorted(paths, key=lambda path: (-len(path.parts), path.as_posix()))


def sign(path, identity, keychain, entitlements=None):
    args = ["codesign", "--force", "--sign", identity, "--keychain", keychain,
            "--timestamp"]
    if path.suffix != ".dmg":
        args += ["--options", "runtime"]
    if entitlements:
        args += ["--entitlements", entitlements]
    run(args + [path], "Code signing")


def refresh_manifests(app):
    resources = app / "Contents/Resources"
    runtime = resources / "runtime"
    manifest_path = runtime / "manifest.json"
    manifest = json.loads(manifest_path.read_text())
    for name in manifest["sha256"]:
        path = runtime / name
        if not path.resolve().is_relative_to(runtime.resolve()) or not path.is_file():
            raise PackageError("Invalid runtime manifest entry")
        manifest["sha256"][name] = digest(path)
    # Signing may also change native node_modules if any are added in future.
    sys.path.insert(0, str(ROOT / "scripts/runtime"))
    from runtime_manifest import node_modules_digest
    manifest["node_modules_sha256"] = node_modules_digest(runtime)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    login = resources / "login"
    manifest_path = login / "manifest.json"
    manifest = json.loads(manifest_path.read_text())
    helper = login / manifest["executable"]
    if not helper.resolve().is_relative_to(login.resolve()) or not helper.is_file():
        raise PackageError("Invalid login manifest entry")
    manifest["sha256"] = digest(helper)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")


def sign_app(app, identity, keychain):
    binaries = macho_files(app)
    if not binaries:
        raise PackageError("No Mach-O code in app")
    node = app / "Contents/Resources/runtime/node"
    for path in binaries:
        sign(path, identity, keychain, ROOT / "scripts/release/node-entitlements.plist" if path == node else None)
    bundles = [path for path in app.rglob("*") if path.is_dir() and not path.is_symlink()
               and path.suffix in {".app", ".framework", ".xpc", ".appex", ".bundle", ".plugin"}
               and any(path in binary.parents for binary in binaries)]
    for path in sorted(bundles, key=lambda path: -len(path.parts)):
        sign(path, identity, keychain)
    refresh_manifests(app)
    sign(app, identity, keychain)
    run(["codesign", "--verify", "--deep", "--strict", app], "App signature verification")
    for path in binaries:
        run(["codesign", "--verify", "--strict", path], "Embedded code signature verification")
    # A signature check alone would miss broken V8 JIT entitlements.
    run([node, "-e", "for(let i=0;i<10000;i++)new Function('return 1')()"], "Signed Node execution")
    run([app / "Contents/Resources/runtime/ffmpeg", "-version"], "Signed FFmpeg execution")


def notarize(path, directory):
    key = directory / "notary.p8"
    if not key.exists():
        write_secret("APPLE_API_KEY_P8_BASE64", key)
    output = run(["xcrun", "notarytool", "submit", path, "--key", key,
                  "--key-id", required("APPLE_API_KEY_ID"), "--issuer", required("APPLE_API_ISSUER_ID"),
                  "--wait", "--timeout", "25m", "--output-format", "json"], "Apple notarization", capture=True)
    result = json.loads(output)
    if result.get("status") != "Accepted":
        raise PackageError("Apple notarization was not accepted")
    submission_id = str(uuid.UUID(result["id"]))
    print("Apple notarization accepted; submission " + submission_id, flush=True)


def staple(path):
    run(["xcrun", "stapler", "staple", path], "Ticket stapling")
    run(["xcrun", "stapler", "validate", path], "Stapled ticket verification")


def package(mode):
    config = json.loads((ROOT / "mygo.json").read_text())
    version = config["version"]
    if not re.fullmatch(r"[0-9A-Za-z.+-]+", version):
        raise PackageError("Invalid app version")
    app = ROOT / "build/darwin-arm64/ytmgo.app"
    if not app.is_dir():
        raise PackageError("Build the complete arm64 app first")
    output = ROOT / "dist/macos"
    if output.exists():
        raise PackageError("Output directory already exists; use a clean build workspace")
    output.mkdir(parents=True)
    directory = None
    try:
        if mode != "unsigned":
            directory = signing_directory()
            identity, keychain = import_identity(directory)
            sign_app(app, identity, keychain)
            if mode == "notarized":
                submission = directory / "app.zip"
                run(["ditto", "-c", "-k", "--keepParent", app, submission], "Notarization archive creation")
                notarize(submission, directory)
                staple(app)
                run(["spctl", "--assess", "--type", "execute", app], "App Gatekeeper assessment")
        else:
            run(["codesign", "--verify", "--deep", "--strict", app], "Ad-hoc app signature verification")
        name = "ytmgo-" + version + "-arm64-" + mode
        archive = output / (name + ".zip")
        run(["ditto", "-c", "-k", "--keepParent", app, archive], "App archive creation")
        dmg = output / (name + ".dmg")
        with tempfile.TemporaryDirectory(prefix="ytmgo-dmg-") as temporary:
            contents = Path(temporary)
            run(["ditto", app, contents / app.name], "Disk image app copy")
            (contents / "Applications").symlink_to("/Applications", target_is_directory=True)
            run(["hdiutil", "create", "-volname", "ytmgo", "-srcfolder", contents,
                 "-format", "UDZO", "-ov", dmg], "Disk image creation")
        if mode != "unsigned":
            sign(dmg, identity, keychain)
            run(["codesign", "--verify", "--strict", dmg], "Disk image signature verification")
            if mode == "notarized":
                notarize(dmg, directory)
                staple(dmg)
                run(["spctl", "--assess", "--type", "open", "--context", "context:primary-signature", dmg],
                    "Disk image Gatekeeper assessment")
        metadata = {
            "version": version, "architecture": "arm64", "mode": mode,
            "minimum_macos": config["macos"]["minimumSystemVersion"],
            "commit": os.environ.get("GITHUB_SHA", "local"),
            "run_url": "https://github.com/" + os.environ.get("GITHUB_REPOSITORY", "mreasonyang/ytmgo")
                       + "/actions/runs/" + os.environ.get("GITHUB_RUN_ID", "local"),
            "developer_id_signed": mode != "unsigned", "notarized": mode == "notarized",
        }
        (output / "build.json").write_text(json.dumps(metadata, indent=2) + "\n")
        (output / "SHA256SUMS").write_text("".join(
            digest(path) + "  " + path.name + "\n" for path in (archive, dmg, output / "build.json")))
        print("Packaged macOS arm64 build: " + mode, flush=True)
    finally:
        if directory is not None:
            cleanup()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    build = commands.add_parser("package")
    build.add_argument("--mode", choices=("unsigned", "developer-id", "notarized"), required=True)
    commands.add_parser("cleanup")
    args = parser.parse_args()
    try:
        if args.command == "cleanup":
            cleanup()
        elif sys.platform != "darwin":
            raise PackageError("macOS is required")
        else:
            package(args.mode)
    except Exception as error:
        # Print only our fixed categories, never arbitrary exception messages,
        # commands, environment values, notary responses or tool diagnostics.
        print("Packaging: " + (str(error) if isinstance(error, PackageError) else "operation failed"), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
