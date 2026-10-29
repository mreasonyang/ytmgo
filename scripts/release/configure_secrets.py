#!/usr/bin/env python3
"""Interactively transfer selected signing files to GitHub encrypted secrets.

Run in your own terminal. Paths and passwords are prompted locally; values are
sent to the authenticated gh client over stdin, never printed or put in argv.
"""
import argparse
import base64
import getpass
from pathlib import Path
import subprocess
import sys

REPOSITORY = "mreasonyang/ytmgo"
ENVIRONMENT = "macos-release"


def upload(name, value):
    result = subprocess.run(
        ["gh", "secret", "set", name, "--repo", REPOSITORY, "--env", ENVIRONMENT],
        input=value.encode(), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False,
    )
    if result.returncode:
        raise RuntimeError("GitHub secret upload failed")
    print("Configured " + name, flush=True)


def file_value(label, suffix):
    path = Path(input(label).strip()).expanduser()
    if path.suffix.lower() != suffix or not path.is_file():
        raise RuntimeError("Selected file is missing or has an unexpected extension")
    return base64.b64encode(path.read_bytes()).decode("ascii")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", choices=("signing", "notarization"))
    args = parser.parse_args()
    try:
        if args.kind == "signing":
            certificate = file_value("Developer ID Application .p12 path: ", ".p12")
            password = getpass.getpass(".p12 export password (hidden): ")
            if not password:
                raise RuntimeError("Use a password-protected .p12 export")
            upload("MACOS_CERTIFICATE_P12_BASE64", certificate)
            upload("MACOS_CERTIFICATE_PASSWORD", password)
        else:
            key = file_value("App Store Connect team API key .p8 path: ", ".p8")
            key_id = input("Key ID: ").strip()
            issuer = input("Issuer ID: ").strip()
            if not key_id or not issuer:
                raise RuntimeError("Key ID and Issuer ID are required")
            upload("APPLE_API_KEY_P8_BASE64", key)
            upload("APPLE_API_KEY_ID", key_id)
            upload("APPLE_API_ISSUER_ID", issuer)
    except (Exception, KeyboardInterrupt):
        print("Configuration incomplete. Check gh login, file selection and environment access; rerunning is safe.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
