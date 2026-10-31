#!/usr/bin/env python3
"""Build native playback on macOS/Linux/Windows, without Node or npm.

Windows FFmpeg builds in MSYS2 UCRT64. WebKitGTK/WebView2 remain system
prerequisites, as for the existing account login helper.
"""
import base64
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from download import download, verified, digest, run, failure_category
from protocol_version import check_protocol_version, _sdk_version

ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT / "build/runtime"
CACHE=ROOT / "build/runtime-cache"
HOST=ROOT / "native/playback-host"
FFMPEG="9.0.2"
FFMPEG_SHA256="8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e"
FFMPEG_SOURCE_URL="https://ffmpeg.org/releases/ffmpeg-"+FFMPEG+".tar.xz"
FFMPEG_DOWNLOAD_URL="https://distfiles.macports.org/ffmpeg/ffmpeg-"+FFMPEG+".tar.xz"

def extract(archive,destination,prefix):
    total=0
    with tarfile.open(archive) as tar:
        for member in tar:
            parts=PurePosixPath(member.name).parts
            if not parts or parts[0]!=prefix or ".." in parts or member.name.startswith("/") or "\\" in member.name or ":" in member.name:
                raise RuntimeError("archive_member_invalid")
            if member.isdir():continue
            if not member.isfile() or member.size>64*1024**2:
                raise RuntimeError("archive_member_invalid")
            total+=member.size
            if total>512*1024**2:raise RuntimeError("archive_size_limit")
            target=destination.joinpath(*parts[1:]);target.parent.mkdir(parents=True,exist_ok=True)
            with tar.extractfile(member) as source,target.open("wb") as sink:
                shutil.copyfileobj(source,sink)
            target.chmod(0o755 if member.mode&0o111 else 0o644)

def prepare_assets(output):
    lock=json.loads((ROOT / "scripts/runtime/native/assets.lock.json").read_text())
    modules=CACHE / "native-script-sources";modules.mkdir(exist_ok=True)
    for package in lock["packages"]:
        name=package["name"]
        if name not in {"youtubei.js","meriyah","bgutils-js"}:raise RuntimeError("runtime_lock_version_mismatch")
        archive=download(package["url"],CACHE / (name+"-"+package["version"]+".tgz"))
        actual="sha512-"+base64.b64encode(hashlib.sha512(archive.read_bytes()).digest()).decode()
        if actual!=package["integrity"]:raise RuntimeError("release checksum mismatch")
        target=modules / name
        if target.exists():shutil.rmtree(target)
        extract(archive,target,"package")
        notices=[p for p in target.iterdir() if p.is_file() and p.name.lower().startswith(("license","licence","copying","notice"))]
        if not notices:raise RuntimeError("dependency license missing")
        for notice in notices:shutil.copy2(notice,output / "licenses" / (name+"-"+notice.name))
    constants=modules / "youtubei.js/dist/src/utils/Constants.js"
    check_protocol_version(constants,ROOT / "internal/catalog/client.go")
    version=_sdk_version(constants.read_text())
    if re.findall(r'const musicVersion = "([0-9.]+)"',(ROOT / "internal/streaming/types.go").read_text())!=[version]:
        raise RuntimeError("runtime_protocol_version_check_failed")
    agents=re.findall(r'const proofUserAgent = "([^"\n]+)"',(ROOT / "internal/streaming/types.go").read_text())
    upstream_agents=re.findall(r"export const USER_AGENT = '([^'\n]+)'",(modules / "bgutils-js/dist/utils/constants.js").read_text())
    if len(agents)!=1 or agents!=upstream_agents or (HOST / "assets/user-agent.txt").read_text().strip()!=agents[0]:
        raise RuntimeError("runtime_protocol_version_check_failed")
    for name in ("extractor","proof"):
        run(["go","run","-mod=readonly",".",str(ROOT / "scripts/runtime/native" / (name+".mjs")),str(modules),str(HOST / "assets" / (name+".js"))],ROOT / "scripts/bundle")
    shutil.copy2(ROOT / "scripts/runtime/licenses/bgutils-examples-MIT",output / "licenses/bgutils-examples-MIT")
    return {p["name"]:p["version"] for p in lock["packages"]}

def prepare_host(output,suffix):
    run(["cargo","build","--release","--locked","--target-dir",str(ROOT / "build/playback-target"),"--manifest-path",str(HOST / "Cargo.toml")])
    shutil.copy2(ROOT / "build/playback-target/release" / ("ytmgo-playback-host"+suffix),output / ("ytmgo-playback-host"+suffix))
    run([str(output / ("ytmgo-playback-host"+suffix)),"check"])
    compiler=subprocess.run(["rustc","-vV"],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,check=False)
    targets=re.findall(rb"^host: ([A-Za-z0-9_-]+)$",compiler.stdout,re.MULTILINE)
    if compiler.returncode or len(targets)!=1:raise RuntimeError("runtime_preparation_failed")
    # Only the native target's dependency closure is linked into this helper.
    result=subprocess.run(["cargo","metadata","--locked","--format-version=1","--filter-platform",targets[0].decode(),"--manifest-path",str(HOST / "Cargo.toml")],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,check=False)
    if result.returncode:raise RuntimeError("runtime_preparation_failed")
    records=[]
    for package in json.loads(result.stdout)["packages"]:
        if package["source"] is None:continue
        folder=Path(package["manifest_path"]).parent
        records.append({k:package.get(k) for k in ("name","version","license")})
        for path in folder.rglob("*"):
            if path.is_file() and path.name.upper().startswith(("LICENSE","LICENCE","COPYING","NOTICE")) and path.stat().st_size<256*1024:
                name=package["name"]+"-"+package["version"]+"-"+str(path.relative_to(folder)).replace(os.sep,"_")
                shutil.copy2(path,output / "licenses" / name)
    (output / "licenses/native-dependencies.json").write_text(json.dumps(records,indent=2)+"\n")

def prepare_ffmpeg(output,suffix):
    name="ffmpeg-"+FFMPEG
    archive=verified(FFMPEG_DOWNLOAD_URL,CACHE / (name+".tar.xz"),FFMPEG_SHA256)
    source=CACHE / (name+"-native-source")
    if not source.exists():extract(archive,source,name)
    folder=CACHE / ("ffmpeg-build-"+sys.platform+"-"+platform.machine());folder.mkdir(exist_ok=True)
    # MSYS sh accepts forward-slash drive paths, not Python's Windows separators.
    config=["sh",(source / "configure").as_posix(),"--disable-everything","--disable-autodetect","--disable-doc","--disable-debug","--disable-ffplay","--disable-ffprobe","--enable-ffmpeg","--enable-small","--enable-static","--disable-shared","--enable-protocol=pipe,file,http,tcp","--enable-demuxer=concat,matroska,mov","--enable-decoder=opus,aac,mp3,pcm_f32le,h264,vp9,av1","--enable-parser=opus,aac,mpegaudio,h264,vp9,av1","--enable-encoder=pcm_f32le,rawvideo","--enable-bsf=h264_mp4toannexb","--enable-muxer=pcm_f32le,rawvideo","--enable-swscale","--enable-filter=aresample,aformat,anull,scale,fps,format"]
    if sys.platform=="win32":config += ["--target-os=mingw32","--cc=gcc","--extra-ldflags=-static","--disable-x86asm"]
    elif platform.machine().lower() in {"x86_64","amd64"} and not shutil.which("nasm"):config += ["--disable-x86asm"]
    marker=folder / "ytmgo-config.json"
    if not marker.exists() or json.loads(marker.read_text())!=config:
        run(config,folder);marker.write_text(json.dumps(config))
    run(["make","-j",str(min(os.cpu_count() or 2,8)),"ffmpeg"+suffix],folder)
    shutil.copy2(folder / ("ffmpeg"+suffix),output / ("ffmpeg"+suffix))
    shutil.copy2(source / "COPYING.LGPLv2.1",output / "licenses/ffmpeg-LGPLv2.1")
    (output / "licenses/ffmpeg-build.json").write_text(json.dumps({"version":FFMPEG,"source":FFMPEG_SOURCE_URL,"sha256":FFMPEG_SHA256,"configure":config[2:],"license":"LGPL-2.1-or-later","modifications":"None; build configuration only"},indent=2)+"\n")

def verify(output):
    manifest=json.loads((output / "manifest.json").read_text())
    actual={p.relative_to(output).as_posix() for p in output.rglob("*") if p.is_file() and p!=output / "manifest.json"}
    if actual!=set(manifest["sha256"]) or any(p.is_symlink() for p in output.rglob("*")):raise RuntimeError("runtime_manifest_mismatch")
    if any(name.split("/")[0] in {"node","node.exe","node_modules","package.json","package-lock.json"} or name.endswith(".mjs") for name in actual):raise RuntimeError("legacy_runtime_present")
    for name,value in manifest["sha256"].items():
        path=output / name
        if not path.resolve().is_relative_to(output.resolve()) or digest(path)!=value:raise RuntimeError("runtime_manifest_mismatch")

def main():
    if sys.argv[1:]==["--verify"]:verify(OUT);print("Verified native runtime");return
    if sys.argv[1:] or sys.platform not in {"darwin","linux","win32"}:raise RuntimeError("runtime_platform_unsupported")
    CACHE.mkdir(parents=True,exist_ok=True)
    if shutil.disk_usage(ROOT).free<2*1024**3:raise RuntimeError("at least 2 GiB free is required")
    suffix=".exe" if sys.platform=="win32" else ""
    with tempfile.TemporaryDirectory(prefix="native-runtime-",dir=ROOT / "build") as stage:
        output=Path(stage);(output / "licenses").mkdir()
        versions=prepare_assets(output);print("Prepared locked scripts without Node/npm",flush=True)
        prepare_host(output,suffix);print("Built native script host",flush=True)
        prepare_ffmpeg(output,suffix);print("Built minimal FFmpeg",flush=True)
        for name in ("ytmgo-playback-host"+suffix,"ffmpeg"+suffix):
            if sys.platform=="darwin":
                result=subprocess.run(["otool","-L",str(output / name)],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,check=True)
                if any(line.strip().startswith((b"/opt/",b"/usr/local/",b"/Users/")) for line in result.stdout.splitlines()[1:]):raise RuntimeError("non_system_runtime_dependency")
                run(["codesign","--force","--sign","-",str(output / name)])
        versions.update({"quickjs":"rquickjs-0.14.0","ffmpeg":FFMPEG})
        manifest={"schema":2,"platform":sys.platform,"architecture":platform.machine(),"versions":versions,"sha256":{p.relative_to(output).as_posix():digest(p) for p in sorted(output.rglob("*")) if p.is_file()}}
        (output / "manifest.json").write_text(json.dumps(manifest,indent=2)+"\n");verify(output)
        backup=ROOT / "build/runtime.previous"
        if backup.exists():raise RuntimeError("runtime_previous_exists")
        if OUT.exists():OUT.rename(backup)
        try:output.rename(OUT)
        except Exception:
            if backup.exists():backup.rename(OUT)
            raise
        if backup.exists():shutil.rmtree(backup)
    print("Prepared and verified native runtime; Node/jsdom absent",flush=True)

if __name__=="__main__":
    try:main()
    except Exception as error:
        print("Runtime preparation failed: "+failure_category(error),file=sys.stderr);sys.exit(1)
