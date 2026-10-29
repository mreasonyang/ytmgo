"""Synthetic resource-copy tests; never inspect a prepared or installed runtime."""
import shutil
from pathlib import Path
import tempfile
import unittest
import urllib.error

from prepare import failure_category
from runtime_manifest import NODE_MODULES_HASH_POLICY, node_modules_digest


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
