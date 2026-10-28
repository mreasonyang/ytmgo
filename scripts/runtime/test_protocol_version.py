"""Synthetic protocol-contract fixtures; never read credentials or run Node."""
from pathlib import Path
import tempfile
import unittest

from protocol_version import FAILURE, check_protocol_version


SDK = """export const CLIENTS = {
    YTMUSIC: {
        NAME: 'WEB_REMIX',
        VERSION: '1.20250219.01.00'
    },
};
"""
GO = '''package catalog
const (
    musicClientVersion = "1.20250219.01.00"
)
'''


class ProtocolVersionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.sdk = self.root / "Constants.js"
        self.go = self.root / "client.go"
        self.sdk.write_text(SDK, encoding="utf-8")
        self.go.write_text(GO, encoding="utf-8")

    def check(self):
        return check_protocol_version(self.sdk, self.go)

    def rejected(self):
        with self.assertRaisesRegex(RuntimeError, "^" + FAILURE + "$"):
            self.check()

    def test_matching_installed_sdk_and_explicit_constant(self):
        self.assertIsNone(self.check())
        self.go.write_text('package catalog\nconst musicClientVersion = "1.20250219.01.00"\n')
        self.assertIsNone(self.check())

    def test_mismatched_version_fails(self):
        self.go.write_text(GO.replace("1.20250219.01.00", "1.20250220.01.00"))
        self.rejected()

    def test_missing_duplicate_or_rewritten_go_constant_fails(self):
        cases = [
            "package catalog\n",
            GO + 'const musicClientVersion = "1.20250219.01.00"\n',
            GO.replace("const (", "var ("),
            GO.replace('"1.20250219.01.00"', "otherConstant"),
            GO.replace('"1.20250219.01.00"', '"1.20250219.01.00" + ""'),
            GO.replace('"1.20250219.01.00"', '"invalid-version"'),
            GO.replace("musicClientVersion =", "musicClientVersion string ="),
        ]
        for source in cases:
            with self.subTest(case=cases.index(source)):
                self.go.write_text(source)
                self.rejected()

    def test_missing_duplicate_or_changed_sdk_block_fails(self):
        cases = [
            "export const CLIENTS = {};\n",
            SDK + SDK,
            SDK.replace("WEB_REMIX", "WEB"),
            SDK.replace("1.20250219.01.00", "1.2025.1.0"),
            SDK.replace("VERSION:", "OTHER_VERSION:"),
            SDK.replace("NAME: 'WEB_REMIX',", "NAME: 'WEB_REMIX', NAME: 'WEB_REMIX',"),
            SDK.replace("VERSION: '1.20250219.01.00'", "VERSION: '1.20250219.01.00', VERSION: '1.20250219.01.00'"),
            SDK.replace("VERSION: '1.20250219.01.00'", "VERSION: { value: '1.20250219.01.00' }"),
            SDK.replace("YTMUSIC: {", "YTMUSIC: makeClient({").replace("    },", "    }),"),
            SDK.replace("VERSION: '1.20250219.01.00'", "VERSION: '1.20250219.01.00' + ''"),
        ]
        for number, source in enumerate(cases):
            with self.subTest(case=number):
                self.sdk.write_text(source)
                self.rejected()

    def test_missing_unreadable_or_invalid_utf8_files_fail_with_fixed_error(self):
        self.sdk.unlink()
        self.rejected()
        self.sdk.mkdir()
        self.rejected()
        self.sdk.rmdir()
        self.sdk.write_bytes(b"\xff")
        self.rejected()
        self.sdk.write_text(SDK)
        self.go.unlink()
        self.rejected()


if __name__ == "__main__":
    unittest.main()
