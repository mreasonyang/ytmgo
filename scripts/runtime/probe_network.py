"""Public download diagnostics: fixed categories and counts, never raw errors."""
import json
import socket
import ssl
import subprocess
import urllib.error
import urllib.request

from prepare import FFMPEG


def category(error):
    reason = error.reason if isinstance(error, urllib.error.URLError) else error
    if isinstance(reason, ssl.SSLCertVerificationError):
        return "tls_certificate_verification_failed"
    if isinstance(reason, ssl.SSLError):
        return "tls_handshake_failed"
    if isinstance(reason, socket.gaierror):
        return "dns_failed"
    if isinstance(reason, TimeoutError):
        return "timeout"
    if isinstance(error, urllib.error.HTTPError):
        return "http_" + str(error.code) if type(error.code) is int and 100 <= error.code <= 599 else "http_error"
    return "network_error"


def main():
    try:
        ca_count = ssl.create_default_context().cert_store_stats()["x509_ca"]
    except Exception:
        ca_count = -1
    urls = (
        ("script_sources", "https://registry.npmjs.org/meriyah/-/meriyah-7.4.1.tgz"),
        ("ffmpeg", "https://ffmpeg.org/releases/ffmpeg-" + FFMPEG + ".tar.xz"),
        ("ffmpeg_macports", "https://distfiles.macports.org/ffmpeg/ffmpeg-" + FFMPEG + ".tar.xz"),
        ("ffmpeg_blfs", "https://ftp.osuosl.org/pub/blfs/conglomeration/ffmpeg/ffmpeg-" + FFMPEG + ".tar.xz"),
    )
    for source, url in urls:
        result = {"source": source, "python_ca_count": ca_count}
        try:
            with urllib.request.urlopen(urllib.request.Request(url, method="HEAD"), timeout=15) as response:
                result["python_status"] = int(response.status)
        except Exception as error:
            result["python_error"] = category(error)
        try:
            curl = subprocess.run(
                ["/usr/bin/curl", "--disable", "--head", "--silent", "--location", "--fail",
                 "--proto", "=https", "--proto-redir", "=https", "--connect-timeout", "10", "--max-time", "15",
                 "--output", "/dev/null", "--write-out", "%{http_code}", url],
                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=20, check=False,
            )
            result["curl_exit_code"] = curl.returncode
            result["curl_http_status"] = int(curl.stdout) if len(curl.stdout) == 3 and curl.stdout.isdigit() else 0
        except Exception:
            result["curl_exit_code"] = -1
        print("::notice title=Runtime network probe::" + json.dumps(result), flush=True)


if __name__ == "__main__":
    main()
