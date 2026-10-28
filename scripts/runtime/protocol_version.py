"""Fail closed if the native catalog and installed SDK disagree on WEB_REMIX.

Only the pinned SDK's simple YTMUSIC literal and a single explicit Go constant
are accepted. This is a build check, not a JavaScript evaluator or generator.
No file contents, protocol values, paths, or parser details enter its errors.
"""
from pathlib import Path
import re


FAILURE = "runtime_protocol_version_check_failed"
VERSION = r"[0-9]+\.[0-9]{8}\.[0-9]{2}\.[0-9]{2}"


def _sdk_version(source):
    # Counting the key separately also rejects duplicate or malformed blocks.
    if len(re.findall(r"\bYTMUSIC\s*:", source)) != 1:
        raise RuntimeError(FAILURE)
    blocks = re.findall(r"\bYTMUSIC\s*:\s*\{([^{}]*)\}", source)
    if len(blocks) != 1:
        raise RuntimeError(FAILURE)
    match = re.fullmatch(
        r"\s*NAME\s*:\s*(['\"])WEB_REMIX\1\s*,\s*"
        r"VERSION\s*:\s*(['\"])(" + VERSION + r")\2\s*,?\s*",
        blocks[0],
    )
    if match is None:
        raise RuntimeError(FAILURE)
    return match.group(3)


def _go_version(source):
    declaration = r'musicClientVersion\s*=\s*"(' + VERSION + r')"\s*(?://[^\n]*)?'
    # Accept the existing const (...) layout or an explicit standalone const.
    # A var, alias, computed expression, typed/rewritten declaration, or a
    # second candidate requires deliberate review rather than guessing.
    candidates = re.findall(
        r"(?m)^[ \t]*(?:(?:const|var)[ \t]+)?musicClientVersion\b", source
    )
    if len(candidates) != 1:
        raise RuntimeError(FAILURE)
    versions = re.findall(r"(?m)^[ \t]*const[ \t]+" + declaration + r"$", source)
    for block in re.findall(r"(?ms)^const[ \t]*\([ \t]*\n(.*?)^\)", source):
        versions.extend(re.findall(r"(?m)^[ \t]*" + declaration + r"$", block))
    if len(versions) != 1:
        raise RuntimeError(FAILURE)
    return versions[0]


def check_protocol_version(sdk_constants, go_client):
    """Validate installed SDK bytes against catalog source, without execution."""
    try:
        sdk_source = Path(sdk_constants).read_text(encoding="utf-8")
        go_source = Path(go_client).read_text(encoding="utf-8")
    except (OSError, UnicodeError):
        raise RuntimeError(FAILURE) from None
    if _sdk_version(sdk_source) != _go_version(go_source):
        raise RuntimeError(FAILURE)
