# SPDX-License-Identifier: Apache-2.0
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import prepare


class DistributionLicenseTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.folder = self.root / "legal"
        self.folder.mkdir()
        for name in prepare.DOCS:
            (self.folder / name).write_text("synthetic project license fixture")
        self.archive = self.folder / "ffmpeg.tar.xz"
        self.archive.write_bytes(b"synthetic corresponding source fixture")
        self.manifest = {"schema": 1, "project_license": "Apache-2.0", "go": [],
            "login": [], "playback": [], "inputs": {}, "ffmpeg": {
                "source_archive": self.archive.name, "sha256": prepare.digest(self.archive)}}
        self.write_manifest()

    def write_manifest(self):
        self.manifest["sha256"] = {p.relative_to(self.folder).as_posix(): prepare.digest(p)
            for p in self.folder.rglob("*") if p.is_file() and p.name != "manifest.json"}
        (self.folder / "manifest.json").write_text(json.dumps(self.manifest))

    def test_missing_or_changed_notice_rejects_distribution(self):
        prepare.verify(self.folder)
        (self.folder / "NOTICE").write_text("changed fixture")
        with self.assertRaisesRegex(ValueError, "checksum"):
            prepare.verify(self.folder)
        self.write_manifest()
        (self.folder / "NOTICE").unlink()
        with self.assertRaisesRegex(ValueError, "missing"):
            prepare.verify(self.folder)

    def test_embedded_font_requires_its_notice(self):
        fonts = self.folder / "fonts"
        fonts.mkdir()
        (fonts / "OFL.txt").write_text("synthetic font license fixture")
        self.manifest["fonts"] = [{"name": "Display", "notice": "fonts/OFL.txt"}]
        self.write_manifest()
        prepare.verify(self.folder)
        (fonts / "OFL.txt").unlink()
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "font notice"):
            prepare.verify(self.folder)

    def test_shipped_fonts_have_notices(self):
        for font in prepare.FONTS:
            self.assertTrue((prepare.ROOT / font["file"]).is_file(), font["file"])
            self.assertIn("SIL Open Font License", (prepare.ROOT / font["notice"]).read_text())

    def test_corresponding_source_required_even_if_file_inventory_is_updated(self):
        self.archive.unlink()
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "corresponding source"):
            prepare.verify(self.folder)

    def test_mpl_source_is_required_in_addition_to_ffmpeg(self):
        self.manifest["login"] = [{"license": "MPL-2.0", "source_archive": "option-ext.crate",
                                    "sha256": hashlib.sha256(b"fixture").hexdigest()}]
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "corresponding source"):
            prepare.verify(self.folder)

    def test_dependency_lock_change_requires_regeneration(self):
        lock = self.root / "Cargo.lock"
        lock.write_text("fixture version one")
        self.manifest["inputs"] = {"Cargo.lock": prepare.digest(lock)}
        self.write_manifest()
        lock.write_text("fixture version two")
        with patch.object(prepare, "input_checksums", return_value={"Cargo.lock": prepare.digest(lock)}):
            with self.assertRaisesRegex(ValueError, "inputs changed"):
                prepare.verify(self.folder, check_inputs=True)

    def test_bundle_verification_does_not_depend_on_signer_staging(self):
        with patch.object(prepare, "ROOT", self.root), patch.object(prepare, "input_checksums", return_value={}):
            prepare.verify(self.folder, check_inputs=True, check_staged_runtime=False)
            with self.assertRaises(FileNotFoundError):
                prepare.verify(self.folder, check_inputs=True)

    def test_nested_upstream_notices_are_preserved_byte_for_byte(self):
        crate = self.root / "crate"
        (crate / "vendor").mkdir(parents=True)
        original = b"synthetic upstream permission notice\r\n"
        (crate / "vendor/LICENSE").write_bytes(original)
        prepare.notices(crate, self.folder / "crate", recursive=True)
        self.assertEqual((self.folder / "crate/vendor/LICENSE").read_bytes(), original)

    def test_symlinks_cannot_substitute_notices(self):
        (self.folder / "link").symlink_to(self.folder / "NOTICE")
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "invalid"):
            prepare.verify(self.folder)


if __name__ == "__main__":
    unittest.main()
