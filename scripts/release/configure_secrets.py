#!/usr/bin/env python3
"""Interactively transfer signing and notarization credentials to GitHub.

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


def upload(name, value):
    result = subprocess.run(
        ["gh", "secret", "set", name, "--repo", REPOSITORY],
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
            apple_id = input("Apple ID email for your Developer team: ").strip()
            password = getpass.getpass("Apple app-specific password (hidden): ").strip()
            if not apple_id or not password:
                raise RuntimeError("Apple ID and app-specific password are required")
            upload("AC_APPLE_ID", apple_id)
            upload("AC_PASSWORD", password)
    except (Exception, KeyboardInterrupt):
        print("Configuration incomplete. Check gh login, file selection and repository access; rerunning is safe.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
