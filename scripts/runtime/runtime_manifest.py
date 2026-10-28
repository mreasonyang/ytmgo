"""Hash the node_modules resource projection shipped by MyGo 0.7.1.

MyGo's cmd/mygo/resources.go hiddenName/walk skips every dot-prefixed entry
before descending. Hash that same visible tree, rather than npm's build-only
hidden metadata. This module never prints dependency paths or file contents.
"""
import hashlib
from pathlib import Path


NODE_MODULES_HASH_POLICY = "mygo-visible-v1"


def _file_digest(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for part in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(part)
    return value.hexdigest()


def node_modules_digest(runtime):
    """Return SHA256 of visible relative names and their file SHA256 values.

    The byte sequence retains the original manifest's path/content format: each
    sorted file contributes its UTF-8 runtime-relative path followed by the
    ASCII hexadecimal content digest. Internal file links hash their target
    bytes, as before; link targets must survive the same resource projection.
    Directory links are represented by their literal link text and a type tag,
    because MyGo copies these as links rather than traversing them.
    """
    runtime = Path(runtime)
    root = runtime / "node_modules"
    boundary = root.resolve(strict=True)
    records = []

    def visit(directory):
        for path in directory.iterdir():
            if path.name.startswith("."):
                continue
            if path.is_symlink():
                try:
                    target = path.resolve(strict=True)
                except (OSError, RuntimeError):
                    raise RuntimeError("runtime_dependency_link_invalid") from None
                if not target.is_relative_to(boundary):
                    raise RuntimeError("runtime_dependency_link_outside_tree")
                if any(part.startswith(".") for part in target.relative_to(boundary).parts):
                    raise RuntimeError("runtime_dependency_link_target_not_packaged")
                if target.is_dir():
                    records.append((path, "directory-link:" + str(path.readlink())))
                elif target.is_file():
                    records.append((path, None))
                else:
                    raise RuntimeError("runtime_dependency_entry_unsupported")
            elif path.is_dir():
                visit(path)
            elif path.is_file():
                records.append((path, None))
            else:
                raise RuntimeError("runtime_dependency_entry_unsupported")

    visit(root)
    value = hashlib.sha256()
    for path, link in sorted(records):
        value.update(path.relative_to(runtime).as_posix().encode("utf-8"))
        value.update(_file_digest(path).encode("ascii") if link is None else link.encode("utf-8"))
    return value.hexdigest()
