"""Synthetic resource-copy tests; never inspect a prepared or installed runtime."""
import shutil
import io
from pathlib import Path
import tempfile
import unittest
import urllib.error
from unittest.mock import patch

import prepare
from prepare import failure_category
from runtime_manifest import NODE_MODULES_HASH_POLICY, node_modules_digest


class RemovedProviderTests(unittest.TestCase):
    def test_incremental_build_removes_gpl_source_maps_and_notices(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            provider = output / "vendor/bgutil/build"
            provider.mkdir(parents=True)
            for name in ("session_manager.js", "session_manager.js.map"):
                (provider / name).write_text("synthetic old provider")
            notices = output / "licenses"
            notices.mkdir()
            for name in ("bgutil-provider-GPL-3.0", "bgutil-provider-SOURCE.md", "node-LICENSE"):
                (notices / name).write_text("synthetic notice")
            prepare.remove_legacy_provider(output)
            prepare.remove_legacy_provider(output)
            self.assertFalse((output / "vendor").exists())
            self.assertEqual([p.name for p in notices.iterdir()], ["node-LICENSE"])

    def test_full_prepare_rebuilds_all_notices_without_touching_binaries(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            (output / "node").write_text("synthetic binary")
            (output / "licenses").mkdir()
            (output / "licenses/removed-dependency-LICENSE").write_text("synthetic notice")
            prepare.reset_notices(output)
            self.assertEqual(list((output / "licenses").iterdir()), [])
            self.assertEqual((output / "node").read_text(), "synthetic binary")


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
        with patch.object(prepare.urllib.request, "urlopen", side_effect=[
                urllib.error.URLError("synthetic transport fixture"), io.BytesIO(b"complete fixture")]) as request, \
             patch.object(prepare.time, "sleep") as sleep, patch("builtins.print"):
            self.assertEqual(prepare.download(url, self.target), self.target)
        self.assertEqual(self.target.read_bytes(), b"complete fixture")
        self.assertEqual(request.call_count, 2)
        self.assertTrue(all(call.args[0] == url for call in request.call_args_list))
        sleep.assert_called_once_with(2)

    def test_repeated_failure_stops_after_four_attempts(self):
        with patch.object(prepare.urllib.request, "urlopen", side_effect=urllib.error.URLError("fixture")) as request, \
             patch.object(prepare.time, "sleep") as sleep, patch("builtins.print"):
            with self.assertRaises(urllib.error.URLError):
                prepare.download("https://example.invalid/fixture.tar.gz", self.target)
        self.assertEqual(request.call_count, 4)
        self.assertEqual(sleep.call_count, 3)
        self.assertFalse(self.target.exists())
        self.assertFalse(self.target.with_name(self.target.name + ".part").exists())

    def test_permanent_http_failure_is_not_retried(self):
        error = urllib.error.HTTPError("https://example.invalid/fixture.tar.gz", 403, "fixture", {}, None)
        with patch.object(prepare.urllib.request, "urlopen", side_effect=error) as request, \
             patch.object(prepare.time, "sleep") as sleep:
            with self.assertRaises(urllib.error.HTTPError):
                prepare.download("https://example.invalid/fixture.tar.gz", self.target)
        request.assert_called_once()
        sleep.assert_not_called()

    def test_partial_transfer_is_discarded_before_retry(self):
        class Interrupted(io.BytesIO):
            def read(self, size=-1):
                if self.tell():
                    raise ConnectionResetError("synthetic interrupted transfer")
                return super().read(size)
        with patch.object(prepare.urllib.request, "urlopen", side_effect=[
                Interrupted(b"partial fixture"), io.BytesIO(b"complete fixture")]), \
             patch.object(prepare.time, "sleep"), patch("builtins.print"):
            prepare.download("https://example.invalid/fixture.tar.gz", self.target)
        self.assertEqual(self.target.read_bytes(), b"complete fixture")
        self.assertFalse(self.target.with_name(self.target.name + ".part").exists())


class RuntimeManifestTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        self.modules = self.source / "node_modules"
        (self.modules / "fixture").mkdir(parents=True)
        (self.modules / "fixture/index.js").write_text("export const fixture = 1;\n")

    def projected_copy(self):
        output = self.root / "app-runtime"
        shutil.copytree(self.source, output, symlinks=True,
                        ignore=lambda _folder, names: [name for name in names if name.startswith(".")])
        return output

    def test_hidden_files_and_nested_directories_match_packaged_projection(self):
        original = node_modules_digest(self.source)
        (self.modules / ".metadata").write_text("build metadata")
        (self.modules / "fixture/.nested/deep").mkdir(parents=True)
        (self.modules / "fixture/.nested/deep/hidden.js").write_text("build fixture")
        self.assertEqual(node_modules_digest(self.source), original)
        self.assertEqual(node_modules_digest(self.projected_copy()), original)
        self.assertEqual(NODE_MODULES_HASH_POLICY, "mygo-visible-v1")

    def test_visible_content_and_name_changes_are_detected(self):
        original = node_modules_digest(self.source)
        path = self.modules / "fixture/index.js"
        path.write_text("export const fixture = 2;\n")
        changed = node_modules_digest(self.source)
        self.assertNotEqual(changed, original)
        path.rename(path.with_name("entry.js"))
        self.assertNotEqual(node_modules_digest(self.source), changed)

    def test_internal_links_survive_resource_copy(self):
        (self.modules / "fixture/entry.js").symlink_to("index.js")
        (self.modules / "fixture-alias").symlink_to("fixture", target_is_directory=True)
        self.assertEqual(node_modules_digest(self.source), node_modules_digest(self.projected_copy()))

    def test_external_link_is_not_hashed(self):
        outside = self.root / "outside-fixture"
        outside.write_text("synthetic outside data")
        (self.modules / "fixture/outside.js").symlink_to(outside)
        with self.assertRaisesRegex(RuntimeError, "^runtime_dependency_link_outside_tree$"):
            node_modules_digest(self.source)

    def test_link_to_omitted_hidden_target_fails(self):
        (self.modules / "fixture/.hidden.js").write_text("synthetic hidden data")
        (self.modules / "fixture/visible.js").symlink_to(".hidden.js")
        with self.assertRaisesRegex(RuntimeError, "^runtime_dependency_link_target_not_packaged$"):
            node_modules_digest(self.source)


if __name__ == "__main__":
    unittest.main()
