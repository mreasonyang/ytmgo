#!/usr/bin/env python3
"""Summarize the metadata-only samples produced by measure.py."""
import json
from pathlib import Path
import statistics
import sys

samples = Path(sys.argv[1])
rows = [json.loads(line) for line in samples.read_text().splitlines() if line]
groups = {}
for row in rows:
    groups.setdefault(row.get("phase", "unlabelled"), []).append(row)
result = {"samples": len(rows), "first": rows[0]["time"], "last": rows[-1]["time"],
          "peak_MiB": round(max(row["footprint"] for row in rows) / 1048576, 2),
          "webkit_observed": any("webkit" in proc["executable"].lower()
                                  for row in rows for proc in row["processes"]),
          "phases": {}}
for phase, subset in groups.items():
    values = [row["footprint"] / 1048576 for row in subset]
    result["phases"][phase] = {"samples": len(subset),
        "median_MiB": round(statistics.median(values), 2),
        "min_MiB": round(min(values), 2), "max_MiB": round(max(values), 2),
        "process_counts": sorted({len(row["processes"]) for row in subset})}
print(json.dumps(result, indent=2))
