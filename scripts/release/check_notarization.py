"""Validate existing CI notarization credentials without submitting software."""
from pathlib import Path
import secrets
import sys
import tempfile

import macos


def check():
    with tempfile.TemporaryDirectory(prefix="ytmgo-notary-check-") as temporary:
        keychain = Path(temporary) / "check.keychain-db"
        try:
            macos.run(["security", "create-keychain", "-p", secrets.token_hex(24), keychain],
                      "Diagnostic keychain creation")
            macos.run(["xcrun", "notarytool", "store-credentials", "ytmgo-notary-check",
                       "--apple-id", macos.required("APPLE_ID"),
                       "--team-id", macos.required("APPLE_TEAM_ID"), "--keychain", keychain],
                      "Notarization credential validation",
                      input_bytes=(macos.required("APPLE_APP_SPECIFIC_PASSWORD") + "\n").encode())
        finally:
            if keychain.exists():
                macos.run(["security", "delete-keychain", keychain], "Diagnostic keychain removal")
    print("Existing CI notarization credentials validated")


if __name__ == "__main__":
    try:
        check()
    except Exception as error:
        print(str(error) if isinstance(error, macos.PackageError) else "Notarization check failed",
              file=sys.stderr)
        sys.exit(1)
