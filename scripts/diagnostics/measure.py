#!/usr/bin/env python3
"""Measure an app and its helpers using executable identities and rusage only.

Never reads argv, environment, credentials, WebView stores or raw application
logs. Requires the user's metadata-only safe-processes collector.
"""
import argparse
import datetime
import json
from pathlib import Path
import subprocess
import time

from process_scope import DarwinPIDSource, collect_owned_metadata, discover_owned_pids


class MetadataCollectionError(RuntimeError):
    pass


def read_safe_metadata(collector, pid, check_output=subprocess.check_output):
    """An exited PID returns []; collector failures must not hide missing helpers."""
    try:
        rows = json.loads(check_output(
            [str(collector), "--pid", str(pid)], text=True,
            stderr=subprocess.DEVNULL, timeout=5))
        if not isinstance(rows, list) or any(not isinstance(row, dict) for row in rows):
            raise ValueError("invalid_metadata_shape")
        return rows
    except (OSError, ValueError, subprocess.SubprocessError):
        raise MetadataCollectionError("safe_metadata_collection_failed") from None


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--pid", type=int, required=True)
    p.add_argument("--seconds", type=int, default=300)
    p.add_argument("--out", type=Path, required=True)
    args = p.parse_args()
    if args.pid <= 0 or args.seconds < 0:
        p.error("pid must be positive and seconds nonnegative")
    collector = Path.home() / ".codex/bin/safe-processes"
    if not collector.is_file():
        raise SystemExit("The credential-safe process collector is required.")
    try:
        source = DarwinPIDSource()
    except (AttributeError, OSError):
        raise SystemExit("The PID-only process collector is unavailable.") from None
    args.out.mkdir(parents=True, exist_ok=True)
    metrics = args.out / "metrics"
    try:
        subprocess.run(["/usr/bin/clang", "-O2", str(Path(__file__).with_name("metrics.c")),
                        "-o", str(metrics)], check=True, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL)
    except (OSError, subprocess.SubprocessError):
        raise SystemExit("The numeric process metrics collector could not be built.") from None

    started = time.monotonic()
    root_start_ticks = None
    samples = args.out / "samples.jsonl"
    for tick in range(args.seconds + 1):
        try:
            selected = discover_owned_pids(args.pid, source.list_pids,
                                          source.responsible_pid, source.child_pids)
        except OSError:
            raise SystemExit("PID-only process enumeration failed; no partial sample recorded.") from None
        try:
            owned = collect_owned_metadata(selected, lambda pid: read_safe_metadata(collector, pid))
        except MetadataCollectionError:
            raise SystemExit("Safe process metadata collection failed; no partial sample recorded.") from None
        try:
            payload = subprocess.check_output([str(metrics), *map(str, sorted(owned))],
                                              text=True, stderr=subprocess.DEVNULL)
        except (OSError, subprocess.SubprocessError):
            raise SystemExit("The numeric process metrics collector failed.") from None
        processes = []
        for line in payload.splitlines():
            item = json.loads(line)
            # Exiting PIDs produce numeric errno records, not measured processes.
            if item.get("pid") not in owned or "error_errno" in item:
                continue
            item["executable"] = owned[item["pid"]]["executable"]
            processes.append(item)
        root = next((item for item in processes if item["pid"] == args.pid), None)
        if root is not None:
            if root_start_ticks is not None and root["start_ticks"] != root_start_ticks:
                break  # The PID was reused; it is no longer the measured app.
            root_start_ticks = root["start_ticks"]
        row = {"time": datetime.datetime.now().astimezone().isoformat(timespec="seconds"),
               "elapsed": round(time.monotonic()-started, 3), "processes": processes,
               "footprint": sum(x.get("footprint", 0) for x in processes)}
        phase = args.out / "phase.txt"
        if phase.is_file():
            row["phase"] = phase.read_text().strip()[:80]
        with samples.open("a") as out:
            out.write(json.dumps(row)+"\n")
        (args.out/"latest.json").write_text(json.dumps(row, indent=2)+"\n")
        if tick % 30 == 0:
            print(json.dumps({"elapsed": row["elapsed"], "total_MiB": round(row["footprint"]/1048576, 2),
                              "processes": len(processes)}), flush=True)
        if (args.out/"stop").exists() or root is None:
            break
        time.sleep(max(0, started + tick + 1 - time.monotonic()))


if __name__ == "__main__":
    main()
