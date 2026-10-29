#!/usr/bin/env python3
"""Build the relocatable macOS YouTube.js and native decoder runtime.

Node is the official standalone build; npm's integrity-locked dependencies and
our IPC worker ship together. FFmpeg receives media through the Node loopback
bridge. No Homebrew, npm, Python, or external resolver is needed at app runtime.
Every download is checked before extraction/execution. Build tools are required
only on the packaging machine (Xcode command-line tools, make, Python 3).
"""
import concurrent.futures
import hashlib
import http.client
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tarfile
import time
import urllib.error
import urllib.request

from runtime_manifest import NODE_MODULES_HASH_POLICY, node_modules_digest
from protocol_version import check_protocol_version

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "build/runtime"
CACHE = ROOT / "build/runtime-cache"
YOUTUBEJS = "18.3.0"
BGUTILS = "4.0.3"
JSDOM = "29.1.1"
AXIOS = "1.20.0"
NODE = "26.12.1"
FFMPEG = "9.0.2"
FFMPEG_SHA256 = "8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e"
FFMPEG_SOURCE_URL = "https://ffmpeg.org/releases/ffmpeg-" + FFMPEG + ".tar.xz"
FFMPEG_DOWNLOAD_URL = "https://distfiles.macports.org/ffmpeg/ffmpeg-" + FFMPEG + ".tar.xz"
ARCH = {"arm64": "arm64", "x86_64": "x64"}.get(platform.machine())


def failure_category(error):
    """Return only fixed categories or a validated numeric public HTTP status."""
    if isinstance(error, urllib.error.HTTPError):
        code = error.code
        return "public_download_http_" + str(code) if type(code) is int and 100 <= code <= 599 else "public_download_http_error"
    if isinstance(error, (urllib.error.URLError, ConnectionError, http.client.IncompleteRead)):
        return "public_download_network_error"
    if isinstance(error, TimeoutError):
        return "public_download_timeout"
    if type(error) is RuntimeError:
        return {
            "release checksum mismatch": "release_checksum_mismatch",
            "release checksum missing": "release_checksum_missing",
            "runtime preparation command failed: configure": "ffmpeg_configure_failed",
            "runtime preparation command failed: make": "ffmpeg_build_failed",
            "runtime preparation command failed: node": "node_dependency_install_failed",
            "runtime preparation command failed: tar": "archive_extract_failed",
            "runtime preparation command failed: codesign": "adhoc_sign_failed",
            "runtime dependency lock version mismatch": "runtime_lock_version_mismatch",
            "runtime must contain one YouTube.js dependency": "runtime_sdk_count_mismatch",
            "locked runtime dependency missing": "runtime_dependency_missing",
            "dependency license missing": "runtime_license_missing",
            "at least 2 GiB free is required": "insufficient_disk_space",
        }.get(str(error), "runtime_preparation_failed")
    return "runtime_preparation_failed"


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for part in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(part)
    return h.hexdigest()


def download(url, path):
    if path.exists():
        return path
    part = path.with_name(path.name + ".part")
    for attempt in range(4):
        try:
            # Only public release URLs enter these requests; no auth or cookies.
            with urllib.request.urlopen(url, timeout=60) as response, part.open("wb") as sink:
                shutil.copyfileobj(response, sink, 1024 * 1024)
            part.replace(path)
            return path
        except (urllib.error.URLError, TimeoutError, ConnectionError, http.client.IncompleteRead) as error:
            part.unlink(missing_ok=True)
            # Retry transport failures, throttling and temporary server errors.
            # Permanent HTTP errors still fail immediately; all callers retain
            # the existing checksum/version verification after the download.
            permanent_http = isinstance(error, urllib.error.HTTPError) and error.code not in {408, 425, 429, 500, 502, 503, 504}
            if permanent_http or attempt == 3:
                raise
            print("Retrying public runtime download (" + str(attempt + 2) + "/4)", flush=True)
            time.sleep(2 ** (attempt + 1))
    return path


def verified(url, path, expected):
    download(url, path)
    if digest(path) != expected:
        raise RuntimeError("release checksum mismatch")
    return path


def release_hash(base, filename, manifest):
    file = download(base + "/" + manifest, CACHE / (filename + ".checksums"))
    for line in file.read_text().splitlines():
        fields = line.split()
        if len(fields) == 2 and fields[1].lstrip("*") == filename:
            return fields[0]
    raise RuntimeError("release checksum missing")


def run(args, cwd=None, env=None):
    # Build and binary diagnostics are intentionally not printed or retained.
    result = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        raise RuntimeError("runtime preparation command failed: " + Path(args[0]).name)


def remove_legacy_provider(output):
    """Incremental builds must not redistribute the removed GPL implementation."""
    vendor = output / "vendor"
    if vendor.exists():
        shutil.rmtree(vendor)
    for notice in (output / "licenses").glob("bgutil-provider-*"):
        notice.unlink()


def reset_notices(output):
    """Rebuild notices so removed npm dependencies leave no stale material."""
    notices = output / "licenses"
    if notices.exists():
        shutil.rmtree(notices)
    notices.mkdir()


def prepare_resolver():
    source = ROOT / "scripts/runtime"
    # Ship only this implementation; removed helper modules must not survive
    # an incremental prepare and enter the next bundle.
    sources = {path.name for path in source.glob("*.mjs")}
    for path in OUT.glob("*.mjs"):
        if path.name not in sources:
            path.unlink()
    for path in [*source.glob("*.mjs"), source / "package.json", source / "package-lock.json"]:
        shutil.copy2(path, OUT / path.name)
    remove_legacy_provider(OUT)
    lock = json.loads((OUT / "package-lock.json").read_text())
    pins = {"youtubei.js": YOUTUBEJS, "bgutils-js": BGUTILS, "jsdom": JSDOM,
            "axios": AXIOS}
    for package, version in pins.items():
        if lock["packages"]["node_modules/" + package]["version"] != version:
            raise RuntimeError("runtime dependency lock version mismatch")
    if sum(path.endswith("node_modules/youtubei.js") for path in lock["packages"]) != 1:
        raise RuntimeError("runtime must contain one YouTube.js dependency")
    node_folder = "node-v" + NODE + "-darwin-" + ARCH
    npm_root = CACHE / node_folder / "lib/node_modules/npm"
    if not (npm_root / "bin/npm-cli.js").exists():
        run(["/usr/bin/tar", "-xzf", str(CACHE / (node_folder + ".tar.gz")),
             "-C", str(CACHE), node_folder + "/lib/node_modules/npm"])
    # Explicit config files and a minimal environment prevent reading the
    # packager's npm credentials/configuration. Install scripts are disabled.
    user_config = CACHE / "npm-empty-user.npmrc"
    global_config = CACHE / "npm-empty-global.npmrc"
    user_config.write_text("")
    global_config.write_text("")
    run([str(OUT / "node"), str(npm_root / "bin/npm-cli.js"), "ci", "--omit=dev",
         "--omit=optional", "--ignore-scripts", "--no-audit", "--no-fund", "--userconfig=" + str(user_config),
         "--globalconfig=" + str(global_config), "--registry=https://registry.npmjs.org",
         "--cache=" + str(CACHE / "npm-cache")], OUT,
        {"PATH": str(OUT) + ":/usr/bin:/bin:/usr/sbin:/sbin"})
    # Check the actual installed dependency, not only its npm lock version.
    # A changed upstream client contract must stop packaging for review.
    check_protocol_version(OUT / "node_modules/youtubei.js/dist/src/utils/Constants.js",
                           ROOT / "internal/catalog/client.go")
    for package, metadata in lock["packages"].items():
        if not package:
            continue
        folder = OUT / package
        if not folder.is_dir():
            if metadata.get("optional") or metadata.get("dev"):
                continue
            raise RuntimeError("locked runtime dependency missing")
        license_files = [path for path in folder.iterdir() if path.is_file()
                         and path.name.lower().startswith(("license", "licence", "copying", "notice"))]
        supplemental = {
            ("agent-base", "6.0.2"): "agent-base-6.0.2-*",
            ("https-proxy-agent", "5.0.1"): "https-proxy-agent-5.0.1-*",
            ("saxes", "6.0.0"): "saxes-6.0.0-*",
        }.get((folder.name, metadata.get("version")))
        if supplemental:
            license_files.extend((source / "licenses").glob(supplemental))
        if package == "node_modules/@bufbuild/protobuf":
            # Its npm tarball omits the Apache license. Vendor the upstream
            # v2.16.0 text plus the BSD notice already present in varint.js.
            license_files = list((source / "licenses").glob("bufbuild-protobuf-*"))
        if not license_files:
            raise RuntimeError("dependency license missing")
        name = package.removeprefix("node_modules/").replace("/", "-").replace("@", "")
        for path in license_files:
            target = path.name if path.parent == source / "licenses" else name + "-" + path.name
            shutil.copy2(path, OUT / "licenses" / target)
    shutil.copy2(source / "licenses/README.md", OUT / "licenses/SOURCES.md")
    shutil.copy2(source / "licenses/bgutils-examples-MIT", OUT / "licenses/bgutils-examples-MIT")
    shutil.copy2(source / "licenses/proof-session-APACHE-2.0", OUT / "licenses/proof-session-APACHE-2.0")
    # Old generated runtime components must not enter a new app bundle.
    for path in [OUT / "yt-dlp", OUT / "licenses/yt-dlp-LICENSE"]:
        path.unlink(missing_ok=True)
    print("Prepared YouTube.js " + YOUTUBEJS + " and MIT BgUtils " + BGUTILS + " with locked dependencies", flush=True)


def prepare_node():
    name = "node-v" + NODE + "-darwin-" + ARCH
    archive = name + ".tar.gz"
    base = "https://nodejs.org/dist/v" + NODE
    source = verified(base + "/" + archive, CACHE / archive,
                      release_hash(base, archive, "SHASUMS256.txt"))
    with tarfile.open(source) as tar:
        for member, target in [(name + "/bin/node", OUT / "node"),
                               (name + "/LICENSE", OUT / "licenses/node-LICENSE")]:
            data = tar.extractfile(member)
            if data is None:
                raise RuntimeError("Node release member missing")
            # Do not mutate a signed executable's inode while macOS may have
            # its code-signing pages cached or a playback process is using it.
            staging = target.with_name(target.name + ".new")
            with staging.open("wb") as sink:
                shutil.copyfileobj(data, sink)
            staging.chmod(0o755 if target.name == "node" else 0o644)
            staging.replace(target)
    (OUT / "node").chmod(0o755)
    print("Prepared Node " + NODE, flush=True)


def prepare_ffmpeg():
    name = "ffmpeg-" + FFMPEG
    # The MacPorts distfile is the unmodified upstream archive, verified with
    # the same pinned SHA256. ffmpeg.org intermittently times out on hosted
    # macOS runners; do not change versions, codecs or integrity checks.
    source = verified(FFMPEG_DOWNLOAD_URL, CACHE / (name + ".tar.xz"), FFMPEG_SHA256)
    folder = CACHE / name
    if not folder.exists():
        run(["/usr/bin/tar", "-xf", str(source), "-C", str(CACHE)])
    config = ["./configure", "--disable-everything", "--disable-autodetect",
              "--disable-doc", "--disable-debug", "--disable-ffplay", "--disable-ffprobe",
              "--enable-ffmpeg", "--enable-small", "--enable-static", "--disable-shared",
              "--enable-protocol=pipe,file,http,tcp",
              "--enable-demuxer=concat,matroska,mov", "--enable-decoder=opus,aac,mp3,pcm_f32le,h264,vp9,av1",
              "--enable-parser=opus,aac,mpegaudio,h264,vp9,av1", "--enable-encoder=pcm_f32le,rawvideo",
              "--enable-bsf=h264_mp4toannexb",
              "--enable-muxer=pcm_f32le,rawvideo", "--enable-swscale",
              "--enable-filter=aresample,aformat,anull,scale,fps,format"]
    marker = folder / "ytmgo-config.json"
    if not marker.exists() or json.loads(marker.read_text()) != config:
        run(config, folder)
        marker.write_text(json.dumps(config))
    run(["/usr/bin/make", "-j", str(min(os.cpu_count() or 2, 8)), "ffmpeg"], folder)
    shutil.copy2(folder / "ffmpeg", OUT / "ffmpeg.new")
    (OUT / "ffmpeg.new").replace(OUT / "ffmpeg")
    shutil.copy2(folder / "COPYING.LGPLv2.1", OUT / "licenses/ffmpeg-LGPLv2.1")
    (OUT / "licenses/ffmpeg-build.json").write_text(json.dumps({
        "version": FFMPEG, "source": FFMPEG_SOURCE_URL, "download_url": FFMPEG_DOWNLOAD_URL,
        "sha256": FFMPEG_SHA256, "configure": config,
        "license": "LGPL-2.1-or-later", "modifications": "None; build configuration only"
    }, indent=2) + "\n")
    print("Built minimal FFmpeg " + FFMPEG, flush=True)


def main():
    if sys.platform != "darwin" or ARCH is None:
        raise RuntimeError("macOS arm64 or x64 is required to prepare this runtime")
    if shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError("at least 2 GiB free is required")
    OUT.mkdir(parents=True, exist_ok=True)
    reset_notices(OUT)
    CACHE.mkdir(parents=True, exist_ok=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        tasks = [pool.submit(step) for step in [prepare_node, prepare_ffmpeg]]
        for task in tasks:
            task.result()
    prepare_resolver()
    for binary in ["node", "ffmpeg"]:
        # No developer-machine dylib or Homebrew path may reach the app bundle.
        result = subprocess.run(["/usr/bin/otool", "-L", str(OUT / binary)],
                                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=True)
        # Universal Mach-O files repeat a non-indented architecture header.
        dependencies = [line for line in result.stdout.decode().splitlines()
                        if line.startswith(("\t", " "))]
        if any(line.strip().startswith(("/opt/", "/usr/local/", "/Users/")) for line in dependencies):
            raise RuntimeError("non-system runtime dependency: " + binary)
        run(["/usr/bin/codesign", "--force", "--sign", "-", str(OUT / binary)])
    files = ["node", "ffmpeg", "package.json", "package-lock.json"]
    files.extend(path.name for path in sorted((ROOT / "scripts/runtime").glob("*.mjs")))
    files.extend(str(path.relative_to(OUT)) for path in sorted((OUT / "licenses").rglob("*")) if path.is_file())
    manifest = {"platform": "darwin", "architecture": ARCH,
                "versions": {"youtubei.js": YOUTUBEJS, "node": NODE, "ffmpeg": FFMPEG,
                             "bgutils-js": BGUTILS, "jsdom": JSDOM, "axios": AXIOS},
                "sha256": {name: digest(OUT / name) for name in files},
                "node_modules_hash_policy": NODE_MODULES_HASH_POLICY,
                "node_modules_sha256": node_modules_digest(OUT)}
    (OUT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print("Verified relocatable runtime at build/runtime", flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # Never forward an exception's URL, headers, command or arbitrary text.
        message = failure_category(error)
        if os.environ.get("GITHUB_ACTIONS") == "true":
            print("::error title=Media runtime preparation::" + message)
        print("Runtime preparation failed: " + message, file=sys.stderr)
        sys.exit(1)
