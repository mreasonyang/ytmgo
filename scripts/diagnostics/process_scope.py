"""PID-only ownership discovery and selected-process metadata filtering.

Importing this module does not inspect processes or load a system library.
Discovery callbacks expose only PID relationships; executable metadata is
requested separately, after a PID has been selected.
"""

import ctypes


def _valid_pid(pid):
    return isinstance(pid, int) and not isinstance(pid, bool) and pid > 0


def discover_owned_pids(root_pid, list_pids, responsible_pid, child_pids):
    """Return root, its responsibility-owned processes and all their descendants.

    There is no executable-name filter or fixed ancestry-depth limit. Processes
    may exit during a snapshot; a per-process lookup failure is skipped. Failure
    to enumerate the system PID list is propagated rather than presenting a
    silently incomplete scope as a successful measurement.
    """
    if not _valid_pid(root_pid):
        raise ValueError("invalid_root_pid")
    # Cache one responsibility snapshot, then traverse both edge types to a
    # fixed point. An orphaned XPC may belong to a helper rather than directly
    # to the root app; its PPID alone would not include it.
    responsibility_children = {}
    for pid in set(pid for pid in list_pids() if _valid_pid(pid)):
        if pid == root_pid:
            continue
        try:
            responsible = responsible_pid(pid)
        except OSError:
            continue
        if _valid_pid(responsible):
            responsibility_children.setdefault(responsible, set()).add(pid)

    owned = {root_pid}
    pending = [root_pid]
    visited = set()
    while pending:
        pid = pending.pop()
        if pid in visited:
            continue
        visited.add(pid)
        children = set(responsibility_children.get(pid, ()))
        try:
            children.update(child for child in child_pids(pid) if _valid_pid(child))
        except OSError:
            pass
        for child in children:
            if _valid_pid(child) and child not in owned:
                owned.add(child)
                pending.append(child)
    return owned


def collect_owned_metadata(pids, metadata_for_pid):
    """Call the approved collector only for selected PIDs, retaining its allowlist."""
    result = {}
    for pid in sorted(set(pid for pid in pids if _valid_pid(pid))):
        try:
            rows = metadata_for_pid(pid)
        except (OSError, ValueError):
            continue
        if not isinstance(rows, list):
            continue
        for row in rows:
            if not isinstance(row, dict) or type(row.get("pid")) is not int or row["pid"] != pid:
                continue
            executable = row.get("executable")
            if not isinstance(executable, str) or not executable or len(executable) > 4096:
                continue
            item = {"pid": pid, "executable": executable}
            for field in ("ppid", "uid"):
                if type(row.get(field)) is int and row[field] >= 0:
                    item[field] = row[field]
            if isinstance(row.get("state"), str) and len(row["state"]) <= 32:
                item["state"] = row["state"]
            result[pid] = item
            break
    return result


class DarwinPIDSource:
    """libproc adapter using PID arrays and responsibility identifiers only.

    No proc_pidpath, argv, environment or general process-info query is used.
    A library can be injected for offline adapter tests.
    """

    def __init__(self, library=None):
        self._library = library if library is not None else ctypes.CDLL(
            "/usr/lib/libSystem.B.dylib", use_errno=True
        )
        self._all = self._library.proc_listallpids
        self._all.argtypes = [ctypes.c_void_p, ctypes.c_int]
        self._all.restype = ctypes.c_int
        self._children = self._library.proc_listchildpids
        self._children.argtypes = [ctypes.c_int, ctypes.c_void_p, ctypes.c_int]
        self._children.restype = ctypes.c_int
        self._responsible = self._library.responsibility_get_pid_responsible_for_pid
        self._responsible.argtypes = [ctypes.c_int]
        self._responsible.restype = ctypes.c_int

    @staticmethod
    def _read_pids(function, *prefix):
        # These libproc convenience functions return PID counts, while their
        # buffer-size argument is in bytes. Retry a full buffer after growth.
        count = function(*prefix, None, 0)
        if count < 0:
            raise OSError("pid_enumeration_failed")
        capacity = max(count + 64, 64)
        for _ in range(8):
            if capacity > 1_048_576:
                raise OSError("pid_enumeration_capacity")
            buffer = (ctypes.c_int * capacity)()
            count = function(*prefix, buffer, ctypes.sizeof(buffer))
            if count < 0:
                raise OSError("pid_enumeration_failed")
            if count < capacity:
                return [int(buffer[i]) for i in range(count) if buffer[i] > 0]
            capacity = max(capacity * 2, count + 64)
        raise OSError("pid_enumeration_unstable")

    def list_pids(self):
        return self._read_pids(self._all)

    def responsible_pid(self, pid):
        return self._responsible(pid)

    def child_pids(self, pid):
        return self._read_pids(self._children, pid)
