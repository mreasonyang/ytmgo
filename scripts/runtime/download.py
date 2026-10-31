"""Bounded public build downloads and fixed diagnostics."""
import hashlib
import http.client
import urllib.request
import urllib.error
import shutil
import time
import subprocess
from pathlib import Path

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
            "runtime preparation command failed: cargo": "native_host_build_failed",
            "runtime preparation command failed: go": "script_bundle_failed",
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


def run(args, cwd=None, env=None):
    # Build and binary diagnostics are intentionally not printed or retained.
    result = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        raise RuntimeError("runtime preparation command failed: " + Path(args[0]).name)
