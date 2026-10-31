"""Offline package integrity and safe public-download tests."""
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
import urllib.error
from unittest.mock import patch
import prepare
import download as build_download
from download import failure_category

class PreparationDiagnosticsTests(unittest.TestCase):
    def test_arbitrary_error_values_do_not_enter_diagnostics(self):
        for error in (RuntimeError("synthetic private fixture"), ValueError("synthetic private fixture"),
                      urllib.error.URLError("synthetic private fixture")):
            self.assertNotIn("fixture", failure_category(error))

    def test_public_http_errors_report_only_validated_status(self):
        error = urllib.error.HTTPError("synthetic private fixture", 403, "fixture", {}, None)
        self.assertEqual(failure_category(error), "public_download_http_403")

    def test_compile_failure_has_fixed_actionable_category(self):
        self.assertEqual(failure_category(RuntimeError("runtime preparation command failed: configure")),
                         "ffmpeg_configure_failed")


class DownloadRetryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.target = Path(self.temp.name) / "fixture.tar.gz"

    def test_transient_network_failure_retries_without_changing_the_source(self):
        url = "https://example.invalid/fixture.tar.gz"
        with patch.object(build_download.urllib.request, "urlopen", side_effect=[
                urllib.error.URLError("synthetic transport fixture"), io.BytesIO(b"complete fixture")]) as request, \
             patch.object(build_download.time, "sleep") as sleep, patch("builtins.print"):
            self.assertEqual(build_download.download(url, self.target), self.target)
        self.assertEqual(self.target.read_bytes(), b"complete fixture")
        self.assertEqual(request.call_count, 2)
        self.assertTrue(all(call.args[0] == url for call in request.call_args_list))
        sleep.assert_called_once_with(2)

    def test_repeated_failure_stops_after_four_attempts(self):
        with patch.object(build_download.urllib.request, "urlopen", side_effect=urllib.error.URLError("fixture")) as request, \
             patch.object(build_download.time, "sleep") as sleep, patch("builtins.print"):
            with self.assertRaises(urllib.error.URLError):
                build_download.download("https://example.invalid/fixture.tar.gz", self.target)
        self.assertEqual(request.call_count, 4)
        self.assertEqual(sleep.call_count, 3)
        self.assertFalse(self.target.exists())
        self.assertFalse(self.target.with_name(self.target.name + ".part").exists())

    def test_permanent_http_failure_is_not_retried(self):
        error = urllib.error.HTTPError("https://example.invalid/fixture.tar.gz", 403, "fixture", {}, None)
        with patch.object(build_download.urllib.request, "urlopen", side_effect=error) as request, \
             patch.object(build_download.time, "sleep") as sleep:
            with self.assertRaises(urllib.error.HTTPError):
                build_download.download("https://example.invalid/fixture.tar.gz", self.target)
        request.assert_called_once()
        sleep.assert_not_called()

    def test_partial_transfer_is_discarded_before_retry(self):
        class Interrupted(io.BytesIO):
            def read(self, size=-1):
                if self.tell():
                    raise ConnectionResetError("synthetic interrupted transfer")
                return super().read(size)
        with patch.object(build_download.urllib.request, "urlopen", side_effect=[
                Interrupted(b"partial fixture"), io.BytesIO(b"complete fixture")]), \
             patch.object(build_download.time, "sleep"), patch("builtins.print"):
            build_download.download("https://example.invalid/fixture.tar.gz", self.target)
        self.assertEqual(self.target.read_bytes(), b"complete fixture")
        self.assertFalse(self.target.with_name(self.target.name + ".part").exists())



class NativeManifestTests(unittest.TestCase):
    def test_manifest_detects_stale_runtime_and_changed_files(self):
        with tempfile.TemporaryDirectory() as directory:
            output=Path(directory)
            def manifest():
                (output/"manifest.json").write_text(json.dumps({"sha256":{p.name:prepare.digest(p) for p in output.iterdir() if p.name!="manifest.json"}}))
            (output/"ytmgo-playback-host").write_bytes(b"fixture")
            manifest();prepare.verify(output)
            (output/"ytmgo-playback-host").write_bytes(b"changed")
            with self.assertRaises(RuntimeError):prepare.verify(output)
            manifest();prepare.verify(output)
            (output/"node").write_bytes(b"old fixture");manifest()
            with self.assertRaisesRegex(RuntimeError,"legacy_runtime_present"):prepare.verify(output)
    def test_archive_rejects_links_and_escape_paths(self):
        for name,kind in [("package/../escape",tarfile.REGTYPE),("package/a\\..\\escape",tarfile.REGTYPE),("package/link",tarfile.SYMTYPE),("/package/escape",tarfile.REGTYPE)]:
            with self.subTest(name=name),tempfile.TemporaryDirectory() as directory:
                root=Path(directory);archive=root/"fixture.tar"
                with tarfile.open(archive,"w") as tar:
                    entry=tarfile.TarInfo(name);entry.type=kind
                    if kind==tarfile.SYMTYPE:entry.linkname="outside"
                    tar.addfile(entry,io.BytesIO(b""))
                with self.assertRaises(RuntimeError):prepare.extract(archive,root/"output","package")

if __name__=="__main__":unittest.main()
