import json
from pathlib import Path
import plistlib
import struct
import tempfile
import unittest
from unittest.mock import patch

import prepare


def executable_header(system, arch):
    data = bytearray(128)
    if system == "darwin":
        data[:4] = b"\xcf\xfa\xed\xfe"
        struct.pack_into("<I", data, 4, {"amd64": 0x01000007, "arm64": 0x0100000C}[arch])
    elif system == "linux":
        data[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", data, 18, {"amd64": 62, "arm64": 183}[arch])
    else:
        data[:2] = b"MZ"
        struct.pack_into("<I", data, 60, 64)
        data[64:68] = b"PE\0\0"
        struct.pack_into("<H", data, 68, {"amd64": 0x8664, "arm64": 0xAA64}[arch])
    return bytes(data)


class PrepareTests(unittest.TestCase):
    def setUp(self):
        # Binary headers here are synthetic, not runnable or signable. Real
        # signature checks run when prepare stages the native release build.
        def synthetic_signature(bundle):
            signature = bundle / "Contents/_CodeSignature/CodeResources"
            signature.parent.mkdir()
            signature.write_bytes(b"synthetic signature fixture")
        self.addCleanup(patch.stopall)
        patch.object(prepare, "sign_macos_bundle", side_effect=synthetic_signature).start()
        patch.object(prepare, "verify_macos_bundle").start()

    # Cross-target bytes must never pass as the selected platform's helper.
    def test_platform_headers_and_wrong_target(self):
        with tempfile.TemporaryDirectory() as temp:
            binary = Path(temp) / "helper"
            lock = Path(temp) / "Cargo.lock"
            lock.write_text("synthetic lock fixture", encoding="utf-8")
            for target in prepare.TARGETS:
                binary.write_bytes(executable_header(*target))
                self.assertEqual(prepare.binary_platform(binary), target)
                wrong = (target[0], "amd64" if target[1] == "arm64" else "arm64")
                with self.assertRaisesRegex(ValueError, "target mismatch"):
                    prepare.stage(binary, wrong, lock, Path(temp) / "login")

    # Rebuilding another OS replaces the stage instead of shipping both helpers.
    def test_stage_switch_and_tampering(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            binary, lock, out = folder / "helper", folder / "Cargo.lock", folder / "login"
            lock.write_text("synthetic lock fixture", encoding="utf-8")
            for target in [("windows", "amd64"), ("darwin", "arm64"), ("linux", "arm64")]:
                binary.write_bytes(executable_header(*target))
                prepare.stage(binary, target, lock, out)
                prepare.verify(out, target)
            self.assertFalse((out / "ytmgo-account-login.exe").exists())
            self.assertFalse((out / prepare.MAC_BUNDLE).exists())
            with self.assertRaisesRegex(ValueError, "manifest target mismatch"):
                prepare.verify(out, ("windows", "amd64"))
            helper = out / "ytmgo-account-login"
            helper.write_bytes(helper.read_bytes() + b"changed")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                prepare.verify(out, ("linux", "arm64"))

    # Unknown files and path substitutions cannot enter a verified stage.
    def test_unexpected_file_and_manifest_path(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            binary, lock, out = folder / "helper", folder / "Cargo.lock", folder / "login"
            target = ("darwin", "arm64")
            binary.write_bytes(executable_header(*target))
            lock.write_text("synthetic lock fixture", encoding="utf-8")
            prepare.stage(binary, target, lock, out)
            extra = out / "unintended-file"
            extra.write_text("fixture", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "unexpected files"):
                prepare.verify(out, target)
            extra.unlink()
            manifest = json.loads((out / "manifest.json").read_text(encoding="utf-8"))
            manifest["executable"] = "../helper"
            (out / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "manifest target mismatch"):
                prepare.verify(out, target)

    # macOS gets an independent application identity, not a bare executable.
    def test_macos_bundle_identity_and_old_path_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            binary, lock, out = folder / "helper", folder / "Cargo.lock", folder / "login"
            target = ("darwin", "arm64")
            binary.write_bytes(executable_header(*target))
            lock.write_text("synthetic lock fixture", encoding="utf-8")
            prepare.stage(binary, target, lock, out)
            manifest = prepare.verify(out, target)
            self.assertEqual(manifest["executable"], "ytmgo Account Login.app/Contents/MacOS/ytmgo-account-login")
            self.assertFalse((out / prepare.BINARY).exists())
            original = dict(manifest)
            manifest["executable"] = prepare.BINARY
            (out / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "manifest target mismatch"):
                prepare.verify(out, target)
            (out / "manifest.json").write_text(json.dumps(original), encoding="utf-8")
            plist_path = out / prepare.MAC_BUNDLE / "Contents/Info.plist"
            plist = plistlib.loads(plist_path.read_bytes())
            self.assertEqual(plist["CFBundleIdentifier"], "si.kyun.ytmgo.account-login")
            plist["CFBundleIdentifier"] = "si.kyun.ytmgo"
            plist_path.write_bytes(plistlib.dumps(plist))
            with self.assertRaisesRegex(ValueError, "identity mismatch"):
                prepare.verify(out, target)


if __name__ == "__main__":
    unittest.main()
