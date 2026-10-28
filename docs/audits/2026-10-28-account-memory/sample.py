#!/usr/bin/env python3
"""Sample a known native app using PID responsibility + descendant discovery.

Only PID relationships, safe-processes metadata and numeric rusage are read.
No argv, environment, application logs or authentication stores are read.
Output is exclusive-create; phase.txt accepts only the fixed PHASES below.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

REPO = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPO / "scripts/diagnostics"))
from process_scope import DarwinPIDSource, collect_owned_metadata, discover_owned_pids
from measure import read_safe_metadata, MetadataCollectionError

PHASES = {"startup", "idle", "resolving", "playback_warmup", "playback_steady", "paused", "finished"}
MIB = 1048576


def write_json(path, value):
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "w") as f:
        json.dump(value, f, indent=2)
        f.write("\n")


def component(pid, root, executable):
    if pid == root:
        return "native_ui"
    name = Path(executable).name
    if name == "node":
        return "node"
    if name == "ffmpeg":
        return "ffmpeg"
    if executable.startswith("/System/") or executable.startswith("/usr/"):
        return "system_helper"
    return "other_helper"


def metrics_for(executable, pids):
    if not pids:
        return {}
    payload = subprocess.check_output([str(executable), *map(str, sorted(pids))],
                                      text=True, stderr=subprocess.DEVNULL, timeout=5)
    result = {}
    for line in payload.splitlines():
        raw = json.loads(line)
        pid = raw.get("pid")
        if type(pid) is not int or pid not in pids:
            raise ValueError("invalid_metrics")
        result[pid] = {key: raw[key] for key in
                       ("pid", "start_ticks", "footprint", "resident", "lifetime_peak", "cpu_ns", "threads", "error_errno")
                       if type(raw.get(key)) is int}
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--pid", required=True, type=int)
    p.add_argument("--app-executable", required=True, type=Path)
    p.add_argument("--state", required=True, choices=("anonymous", "standard", "premium"))
    p.add_argument("--out", required=True, type=Path)
    p.add_argument("--seconds", type=int, default=480)
    args = p.parse_args()
    if args.pid <= 0 or not 1 <= args.seconds <= 3600:
        p.error("invalid bounds")
    safe = Path.home() / ".codex/bin/safe-processes"
    expected_exe = args.app_executable.resolve()
    initial = read_safe_metadata(safe, args.pid)
    if len(initial) != 1 or Path(initial[0]["executable"]).resolve() != expected_exe:
        raise RuntimeError("root_executable_mismatch")
    args.out.mkdir(parents=True, exist_ok=False, mode=0o700)
    metrics = args.out / "metrics"
    subprocess.run(["/usr/bin/clang", "-O2", str(REPO / "scripts/diagnostics/metrics.c"), "-o", str(metrics)],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
    anchor = metrics_for(metrics, {args.pid}).get(args.pid)
    if not anchor or "start_ticks" not in anchor:
        raise RuntimeError("root_unavailable")
    source = DarwinPIDSource()
    write_json(args.out / "metadata.json", {
        "schema": "ytmgo-three-state-memory-v1", "state": args.state,
        "root_pid": args.pid, "root_start_ticks": anchor["start_ticks"],
        "app_sha256": hashlib.sha256(expected_exe.read_bytes()).hexdigest(),
        "metric": "proc_pid_rusage.ri_phys_footprint", "sampling_interval_seconds": 1,
        "scope": "native_root_and_responsibility_owned_processes_and_all_descendants",
        "browser_included": False, "expected_steady_min_seconds": 120,
    })
    (args.out / "phase.txt").write_text("startup\n")
    fd = os.open(args.out / "samples.jsonl", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    started = time.monotonic()
    reason, recorded = "duration_complete", 0
    with os.fdopen(fd, "w") as out:
        for tick in range(args.seconds + 1):
            time.sleep(max(0, started + tick - time.monotonic()))
            selected = discover_owned_pids(args.pid, source.list_pids, source.responsible_pid, source.child_pids)
            before = metrics_for(metrics, selected)
            root = before.get(args.pid, {})
            if root.get("start_ticks") != anchor["start_ticks"]:
                reason = "root_exited_or_reused"
                break
            owned = collect_owned_metadata(selected, lambda pid: read_safe_metadata(safe, pid))
            after = metrics_for(metrics, selected)
            root = after.get(args.pid, {})
            if root.get("start_ticks") != anchor["start_ticks"]:
                reason = "root_exited_or_reused"
                break
            phase = (args.out / "phase.txt").read_text().strip()
            if phase not in PHASES:
                raise RuntimeError("invalid_phase")
            processes, missing = [], []
            by_kind = {key: 0 for key in ("native_ui", "node", "ffmpeg", "system_helper", "other_helper")}
            for pid in sorted(selected):
                first, last, meta = before.get(pid, {}), after.get(pid, {}), owned.get(pid)
                if (not meta or "footprint" not in last or "start_ticks" not in first or
                        first["start_ticks"] != last.get("start_ticks")):
                    missing.append(pid)
                    continue
                kind = component(pid, args.pid, meta["executable"])
                item = dict(last, executable=meta["executable"], component=kind)
                processes.append(item)
                by_kind[kind] += last["footprint"]
            row = {"schema": "ytmgo-three-state-memory-v1",
                   "timestamp": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="milliseconds"),
                   "elapsed_seconds": round(time.monotonic() - started, 3),
                   "state": args.state, "phase": phase, "complete": not missing,
                   "missing_pids": missing, "processes": processes,
                   "footprint_bytes": sum(by_kind.values()), "components_bytes": by_kind}
            out.write(json.dumps(row) + "\n")
            out.flush()
            recorded += 1
            if tick % 30 == 0:
                print(json.dumps({"elapsed_seconds": row["elapsed_seconds"], "phase": phase,
                                  "total_MiB": round(row["footprint_bytes"] / MIB, 2),
                                  "process_count": len(processes), "complete": row["complete"]}), flush=True)
            if (args.out / "stop").exists():
                reason = "stop_requested"
                break
    write_json(args.out / "completion.json", {"reason": reason, "samples": recorded,
                                               "elapsed_seconds": round(time.monotonic() - started, 3)})


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, MetadataCollectionError):
        print("sampler_failed: metadata_or_metrics_unavailable", file=sys.stderr)
        sys.exit(1)
