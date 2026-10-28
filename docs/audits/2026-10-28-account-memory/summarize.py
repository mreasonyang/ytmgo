#!/usr/bin/env python3
"""Emit fixed-field statistics from the three-state sampler's numeric records."""
import argparse
import json
from pathlib import Path
import statistics

PHASES = ("startup", "idle", "resolving", "playback_warmup", "playback_steady", "paused", "finished")
COMPONENTS = ("native_ui", "node", "ffmpeg", "system_helper", "other_helper")
MIB = 1048576


def statistics_for(selected):
    if not selected:
        return {"samples": 0}
    values = [r["footprint_bytes"] / MIB for r in selected]
    return {
        "samples": len(selected),
        "elapsed_window_seconds": [selected[0]["elapsed_seconds"], selected[-1]["elapsed_seconds"]],
        "duration_seconds": round(selected[-1]["elapsed_seconds"] - selected[0]["elapsed_seconds"], 3),
        "median_MiB": round(statistics.median(values), 2), "peak_MiB": round(max(values), 2),
        "min_MiB": round(min(values), 2),
        "components_median_MiB": {key: round(statistics.median(r["components_bytes"][key] / MIB for r in selected), 2) for key in COMPONENTS},
        "components_peak_MiB": {key: round(max(r["components_bytes"][key] / MIB for r in selected), 2) for key in COMPONENTS},
        "process_counts": sorted({len(r["processes"]) for r in selected}),
        "node_counts": sorted({sum(p["component"] == "node" for p in r["processes"]) for r in selected}),
    }


def summarize(directory, steady_seconds=120):
    metadata = json.loads((directory / "metadata.json").read_text())
    rows = [json.loads(line) for line in (directory / "samples.jsonl").read_text().splitlines() if line]
    valid = [r for r in rows if r.get("complete") is True]
    phases = {phase: statistics_for([r for r in valid if r.get("phase") == phase])
              for phase in PHASES if any(r.get("phase") == phase for r in valid)}
    # Anchor to the first sampled marker, including incomplete samples, so an
    # incomplete early sample cannot shift the compared interval forward.
    raw_steady = [r for r in rows if r.get("phase") == "playback_steady"]
    fixed_steady = []
    window = None
    raw_span = 0
    if raw_steady:
        begin = raw_steady[0]["elapsed_seconds"]
        end = begin + steady_seconds
        window = [begin, round(end, 3)]
        raw_span = raw_steady[-1]["elapsed_seconds"] - begin
        fixed_steady = [r for r in raw_steady if r.get("complete") is True
                        and begin <= r["elapsed_seconds"] < end]
    steady_complete = raw_span >= steady_seconds and len(fixed_steady) >= steady_seconds
    fixed = dict(statistics_for(fixed_steady),
                 requested_seconds=steady_seconds,
                 requested_elapsed_window_seconds=window,
                 relative_window_seconds=[0, steady_seconds],
                 window_bounds="inclusive_start_exclusive_end",
                 window_anchor="first_sample_marked_playback_steady",
                 raw_phase_span_seconds=round(raw_span, 3),
                 complete=steady_complete)
    return {
        "state": metadata["state"], "app_sha256": metadata["app_sha256"],
        "scope": metadata["scope"], "metric": metadata["metric"],
        "samples": len(rows), "complete_samples": len(valid), "incomplete_samples": len(rows) - len(valid),
        "steady_window_complete": steady_complete,
        "webkit_observed": any("webkit" in Path(p["executable"]).name.lower() for r in rows for p in r["processes"]),
        "all_complete_samples_peak_MiB": round(max((r["footprint_bytes"] / MIB for r in valid), default=0), 2),
        "all_complete_samples": statistics_for(valid),
        "fixed_playback_steady": fixed,
        "phases": phases,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directories", nargs="+", type=Path)
    parser.add_argument("--steady-seconds", type=int, default=120,
                        help="fixed playback_steady window, default 120 seconds")
    args = parser.parse_args()
    if args.steady_seconds < 120:
        parser.error("steady window must be at least 120 seconds")
    summaries = [summarize(d, args.steady_seconds) for d in args.directories]
    print(json.dumps({"same_app_binary": len({s["app_sha256"] for s in summaries}) == 1,
                      "all_steady_windows_complete": all(s["steady_window_complete"] for s in summaries),
                      "three_states_present": {s["state"] for s in summaries} == {"anonymous", "standard", "premium"},
                      "steady_window_seconds": args.steady_seconds,
                      "runs": summaries}, indent=2))
