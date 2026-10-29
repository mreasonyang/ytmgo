#!/usr/bin/env python3
"""Summarize only coverage-complete metadata-only samples from measure.py."""
import argparse
import json
from pathlib import Path
import statistics


def coverage_counts(rows):
    return {"samples": len(rows),
            "complete_samples": sum(row.get("coverage_complete") is True for row in rows),
            "incomplete_samples": sum(row.get("coverage_complete") is False for row in rows),
            "coverage_unknown_samples": sum(type(row.get("coverage_complete")) is not bool for row in rows)}


def summarize(rows):
    groups = {}
    for row in rows:
        groups.setdefault(row.get("phase", "unlabelled"), []).append(row)
    complete = [row for row in rows if row.get("coverage_complete") is True]
    result = {**coverage_counts(rows),
              "first": rows[0]["time"] if rows else None,
              "last": rows[-1]["time"] if rows else None,
              "webkit_observed": any("webkit" in proc["executable"].lower()
                                      for row in rows for proc in row["processes"]),
              "phases": {}}
    if complete:
        result["peak_MiB"] = round(max(row["footprint"] for row in complete) / 1048576, 2)
    for phase, subset in groups.items():
        eligible = [row for row in subset if row.get("coverage_complete") is True]
        stats = {**coverage_counts(subset),
                 "process_counts": sorted({len(row["processes"]) for row in eligible})}
        if eligible:
            values = [row["footprint"] / 1048576 for row in eligible]
            stats.update(median_MiB=round(statistics.median(values), 2),
                         min_MiB=round(min(values), 2), max_MiB=round(max(values), 2))
        result["phases"][phase] = stats
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("samples", type=Path)
    args = parser.parse_args()
    rows = [json.loads(line) for line in args.samples.read_text().splitlines() if line]
    print(json.dumps(summarize(rows), indent=2))


if __name__ == "__main__":
    main()
