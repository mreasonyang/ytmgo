"""Shared, explicit release configuration. Ordinary source builds have no feed."""
import copy
import json
from pathlib import Path
import re

VERSION = re.compile(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]{0,2})(?:-rc\.[1-9][0-9]*)?")


def release_config(base, version, build_number=None):
    if not version or not VERSION.fullmatch(version):
        raise ValueError("Explicit RELEASE_VERSION required; use X.Y.Z or X.Y.Z-rc.N with patch 0-999")
    numeric = version.split("-", 1)[0]
    build_number = build_number or numeric
    if not re.fullmatch(r"[0-9]+(?:\.[0-9]+){0,2}", build_number):
        raise ValueError("Invalid numeric release build number")
    config = copy.deepcopy(base)
    config["version"] = version
    config["updates"] = json.loads(Path(__file__).with_name("update-source.json").read_text())
    plist = config.setdefault("macos", {}).setdefault("infoPlist", {})
    plist["CFBundleShortVersionString"] = numeric
    plist["CFBundleVersion"] = build_number
    return config
