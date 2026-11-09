#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Collect target-specific notices and corresponding source without raw logs."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "build/legal"
DOCS = ("LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md")
INPUTS = DOCS + ("go.mod", "go.sum", "native/account-login/Cargo.toml",
    "native/account-login/Cargo.lock", "native/playback-host/Cargo.toml",
    "native/playback-host/Cargo.lock", "scripts/runtime/native/assets.lock.json",
    "scripts/licensing/prepare.py", "scripts/licensing/rebuild_ffmpeg.py")


# Fonts embedded in the app, with how each shipped file was derived.
FONTS = (
    {"name": "Bricolage Grotesque", "file": "resources/fonts/ytmgo-display.ttf",
     "license": "OFL-1.1", "notice": "resources/fonts/OFL-BricolageGrotesque.txt",
     "source": "https://github.com/ateliertriay/bricolage",
     "modifications": "static instance (wght 720, wdth 88, opsz 96), Latin subset, OS/2 weight class 800"},
)


class LicenseError(ValueError):
    """A fixed diagnostic safe to show without dependency tool output."""


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def input_checksums():
    paths = set(INPUTS)
    paths.add("scripts/licensing/upstream-notices.json")
    for directory in ("scripts/licensing/upstream", "scripts/runtime/licenses", "resources/fonts"):
        paths.update(p.relative_to(ROOT).as_posix() for p in (ROOT / directory).rglob("*") if p.is_file())
    return {name: digest(ROOT / name) for name in sorted(paths)}


def command(arguments):
    result = subprocess.run(arguments, cwd=ROOT, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        raise LicenseError("dependency metadata command failed")
    return result.stdout.decode("utf-8")


def json_stream(value):
    decoder = json.JSONDecoder()
    while value.strip():
        item, end = decoder.raw_decode(value.lstrip())
        yield item
        value = value.lstrip()[end:]


def notices(folder, destination, recursive=False):
    paths = folder.rglob("*") if recursive else folder.iterdir()
    copied = []
    for path in sorted(paths):
        if (path.is_file() and not path.is_symlink()
                and path.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE"))
                and path.stat().st_size < 256 * 1024):
            relative = path.relative_to(folder)
            target = destination / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(path, target)
            copied.append(relative.as_posix())
    return copied


def go_notices(output):
    modules = {}
    for package in json_stream(command(["go", "list", "-mod=readonly", "-deps", "-json", "."])):
        module = package.get("Module")
        if not module or module.get("Main"):
            continue
        if module.get("Replace"):
            raise LicenseError("review replacement module licensing before packaging")
        key = (module["Path"], module["Version"])
        entry = modules.setdefault(key, {"dir": Path(module["Dir"]), "packages": set()})
        entry["packages"].add(Path(package["Dir"]))
    records = []
    for (name, version), entry in sorted(modules.items()):
        folder = entry["dir"]
        target = output / "go" / (name.replace("/", "__") + "@" + version)
        if not notices(folder, target):
            raise LicenseError("Go module license missing")
        # Imported subpackages can have a different license from their module.
        for package in sorted(entry["packages"]):
            while package != folder:
                if not package.is_relative_to(folder):
                    raise LicenseError("Go package outside module")
                notices(package, target / package.relative_to(folder))
                package = package.parent
        records.append({"name": name, "version": version})
    goroot = Path(command(["go", "env", "GOROOT"]).strip())
    shutil.copyfile(goroot / "LICENSE", output / "go/Go-LICENSE")
    return records


def rust_notices(crate, target, output):
    manifest = ROOT / "native" / crate / "Cargo.toml"
    metadata = json.loads(command(["cargo", "metadata", "--locked", "--format-version=1",
        "--filter-platform", target, "--manifest-path", str(manifest)]))
    # Cargo's generated registry package fields are unescaped scalar strings.
    # Read only these checksum fields; keep the build compatible with Python 3.9.
    checksums = {}
    for section in re.split(r"(?m)^\[\[package\]\]\s*$", manifest.with_name("Cargo.lock").read_text())[1:]:
        fields = dict(re.findall(r'(?m)^(name|version|checksum) = "([^"\r\n]+)"$', section))
        if {"name", "version", "checksum"}.issubset(fields):
            checksums[(fields["name"], fields["version"])] = fields["checksum"]
    records = []
    for package in sorted(metadata["packages"], key=lambda p: (p["name"], p["version"])):
        if package["source"] is None:
            continue
        name, version = package["name"], package["version"]
        folder = Path(package["manifest_path"]).parent
        license_id = package.get("license")
        if not license_id and not package.get("license_file"):
            raise LicenseError("Rust dependency license missing")
        destination = output / crate / (name + "-" + version)
        upstream_notice = None
        if not notices(folder, destination, recursive=True):
            # Some registry archives omit their workspace-level license. Keep
            # an original copy from the archive's exact upstream VCS revision.
            index = json.loads((ROOT / "scripts/licensing/upstream-notices.json").read_text())
            upstream_notice = index.get(name + "@" + version)
            vcs = json.loads((folder / ".cargo_vcs_info.json").read_text())
            if not upstream_notice or upstream_notice["revision"] != vcs["git"]["sha1"]:
                raise LicenseError("Rust dependency notice missing")
            original = ROOT / "scripts/licensing" / upstream_notice["path"]
            if digest(original) != upstream_notice["sha256"]:
                raise LicenseError("pinned upstream notice checksum mismatch")
            destination.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(original, destination / "LICENSE-upstream")
            if upstream_notice.get("terms"):
                terms = ROOT / "scripts/licensing" / upstream_notice["terms"]["path"]
                if digest(terms) != upstream_notice["terms"]["sha256"]:
                    raise LicenseError("pinned upstream notice checksum mismatch")
                shutil.copyfile(terms, destination / "LICENSE-MIT-terms")
        record = {"name": name, "version": version, "license": license_id,
                  "authors": package.get("authors", []),
                  "source": "https://crates.io/api/v1/crates/" + name + "/" + version + "/download"}
        if upstream_notice:
            record["notice_source"] = upstream_notice["source"]
            if upstream_notice.get("terms"):
                record["selected_license"] = "MIT"
        if license_id and re.search(r"(?:MPL|LGPL|GPL|AGPL)-", license_id):
            # Ship the immutable, unmodified registry archive, never local stores.
            archive = folder.parents[2] / "cache" / folder.parent.name / (name + "-" + version + ".crate")
            expected = checksums.get((name, version))
            if not expected or not archive.is_file() or digest(archive) != expected:
                raise LicenseError("corresponding Rust source checksum mismatch")
            source = output / "sources" / archive.name
            source.parent.mkdir(exist_ok=True)
            shutil.copyfile(archive, source)
            record.update({"source_archive": source.relative_to(output).as_posix(), "sha256": expected,
                           "modifications": "None"})
        records.append(record)
    return records


def script_notices(output):
    lock = json.loads((ROOT / "scripts/runtime/native/assets.lock.json").read_text())
    runtime = ROOT / "build/runtime/licenses"
    destination = output / "scripts"
    destination.mkdir()
    for package in lock["packages"]:
        paths = sorted(runtime.glob(package["name"] + "-*"))
        if not paths:
            raise LicenseError("script dependency notice missing")
        for path in paths:
            shutil.copyfile(path, destination / path.name)
    shutil.copyfile(ROOT / "scripts/runtime/licenses/bgutils-examples-MIT", destination / "bgutils-examples-MIT")
    return [{"name": p["name"], "version": p["version"], "integrity": p["integrity"], "source": p["url"]}
            for p in lock["packages"]]


def font_notices(output):
    destination = output / "fonts"
    destination.mkdir()
    records = []
    for font in FONTS:
        notice = ROOT / font["notice"]
        if not notice.is_file() or not (ROOT / font["file"]).is_file():
            raise LicenseError("font or font notice missing")
        shutil.copyfile(notice, destination / notice.name)
        records.append(dict(font, notice=f"fonts/{notice.name}", sha256=digest(ROOT / font["file"])))
    return records


def ffmpeg_source(output):
    runtime = ROOT / "build/runtime/licenses"
    metadata = json.loads((runtime / "ffmpeg-build.json").read_text())
    options = metadata["configure"]
    if metadata["license"] != "LGPL-2.1-or-later" or any(
            flag in options for flag in ("--enable-gpl", "--enable-nonfree", "--enable-version3")):
        raise LicenseError("FFmpeg license configuration changed; review required")
    filename = "ffmpeg-" + metadata["version"] + ".tar.xz"
    archive = ROOT / "build/runtime-cache" / filename
    if digest(archive) != metadata["sha256"]:
        raise LicenseError("corresponding FFmpeg source checksum mismatch")
    folder = output / "ffmpeg"
    folder.mkdir()
    for source, name in ((archive, filename), (runtime / "ffmpeg-LGPLv2.1", "COPYING.LGPLv2.1"),
            (runtime / "ffmpeg-build.json", "build.json"),
            (ROOT / "scripts/licensing/rebuild_ffmpeg.py", "rebuild.py")):
        shutil.copyfile(source, folder / name)
    return {"version": metadata["version"], "license": metadata["license"], "source": metadata["source"],
            "source_archive": "ffmpeg/" + filename, "sha256": metadata["sha256"]}


def verify_helper_inputs(manifest, resources, legal=None):
    legal = legal or resources / "legal"
    runtime = json.loads((resources / "runtime/manifest.json").read_text())
    if any(runtime["versions"].get(p["name"]) != p["version"] for p in manifest["scripts"]):
        raise LicenseError("script source versions do not match runtime")
    dependencies = json.loads((resources / "runtime/licenses/native-dependencies.json").read_text())
    identity = lambda p: (p["name"], p["version"], p["license"])
    if {identity(p) for p in dependencies} != {identity(p) for p in manifest["playback"]}:
        raise LicenseError("Rust dependency versions do not match runtime")
    login = json.loads((resources / "login/manifest.json").read_text())
    if login["cargo_lock_sha256"] != manifest["inputs"]["native/account-login/Cargo.lock"]:
        raise LicenseError("login dependency lock does not match runtime")
    if digest(resources / "runtime/licenses/ffmpeg-build.json") != digest(
            legal / "ffmpeg/build.json"):
        raise LicenseError("packaged FFmpeg source does not match build")


def verify(folder, check_inputs=False, check_staged_runtime=True):
    manifest = json.loads((folder / "manifest.json").read_text())
    if manifest.get("schema") != 1 or manifest.get("project_license") != "Apache-2.0":
        raise LicenseError("legal manifest invalid")
    actual = {p.relative_to(folder).as_posix() for p in folder.rglob("*") if p.is_file()}
    if actual != set(manifest["sha256"]) | {"manifest.json"}:
        raise LicenseError("legal resources missing or unexpected")
    for name, expected in manifest["sha256"].items():
        path = folder / name
        if not path.resolve().is_relative_to(folder.resolve()) or digest(path) != expected:
            raise LicenseError("legal resource checksum mismatch")
    if any(p.is_symlink() for p in folder.rglob("*")) or not set(DOCS).issubset(actual):
        raise LicenseError("legal resources invalid")
    for record in [manifest["ffmpeg"]] + manifest["login"] + manifest["playback"]:
        if record.get("source_archive"):
            path = folder / record["source_archive"]
            if not path.is_file() or digest(path) != record["sha256"]:
                raise LicenseError("corresponding source missing or changed")
    for font in manifest.get("fonts", []):
        if not (folder / font["notice"]).is_file():
            raise LicenseError("font notice missing")
    if check_inputs:
        if input_checksums() != manifest["inputs"]:
            raise LicenseError("licensing inputs changed; regenerate notices")
        if check_staged_runtime and digest(ROOT / "build/runtime/licenses/ffmpeg-build.json") != digest(folder / "ffmpeg/build.json"):
            raise LicenseError("FFmpeg build changed; regenerate notices")
    return manifest


def prepare():
    OUT.parent.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".legal-stage-", dir=OUT.parent) as directory:
        output = Path(directory)
        for name in DOCS:
            shutil.copyfile(ROOT / name, output / name)
        target = command(["rustc", "-vV"])
        matches = re.findall(r"^host: ([A-Za-z0-9_-]+)$", target, re.MULTILINE)
        if len(matches) != 1:
            raise LicenseError("unsupported Rust build target")
        manifest = {"schema": 1, "project_license": "Apache-2.0", "target": matches[0],
                    "go": go_notices(output), "scripts": script_notices(output),
                    "login": rust_notices("account-login", matches[0], output),
                    "playback": rust_notices("playback-host", matches[0], output),
                    "ffmpeg": ffmpeg_source(output),
                    "fonts": font_notices(output),
                    "inputs": input_checksums()}
        manifest["sha256"] = {p.relative_to(output).as_posix(): digest(p)
                              for p in sorted(output.rglob("*")) if p.is_file()}
        (output / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
        verify(output, check_inputs=True)
        verify_helper_inputs(manifest, ROOT / "build", legal=output)
        if OUT.exists():
            shutil.rmtree(OUT)
        output.rename(OUT)
    print("Prepared verified project license, dependency notices and corresponding source")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify", action="store_true")
    parser.add_argument("--verify-build", action="store_true")
    parser.add_argument("--bundle", type=Path, help="verify an app's Resources directory")
    options = parser.parse_args()
    if options.verify_build:
        config = json.loads((ROOT / "mygo.json").read_text())
        target = json.loads(command(["go", "env", "-json", "GOOS", "GOARCH"]))
        options.bundle = ROOT / config.get("out", "build") / (target["GOOS"] + "-" + target["GOARCH"])
        if target["GOOS"] == "darwin":
            options.bundle = options.bundle / (config["name"] + ".app") / "Contents/Resources"
    if options.bundle:
        manifest = verify(options.bundle / "legal", check_inputs=True, check_staged_runtime=False)
        verify_helper_inputs(manifest, options.bundle)
        print("Verified packaged licenses and corresponding source")
    elif options.verify:
        manifest = verify(OUT, check_inputs=True)
        verify_helper_inputs(manifest, ROOT / "build")
        print("Verified current license resources")
    else:
        prepare()


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, TypeError) as error:
        reason = str(error) if type(error) is LicenseError else "missing, stale or invalid licensing inputs"
        print("License preparation failed: " + reason, file=sys.stderr)
        sys.exit(1)
