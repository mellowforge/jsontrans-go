#!/usr/bin/env python3
"""Validate matched benchmark coverage and summarize medians, not profiler timings."""
import csv
from pathlib import Path
import re
from statistics import median
import sys

root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parent
pattern = re.compile(
    r"^(BenchmarkBytes/\S+)\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op",
    re.M,
)
for folder, toolchain, rounds, cases in [("paired", "go127", 5, 48), ("go126", "go126", 3, 24)]:
    versions = {}
    for version in ["baseline", "candidate"]:
        values = {}
        files = sorted((root / version / folder).glob("round-*.txt"))
        assert len(files) == rounds, (version, folder, len(files))
        for file in files:
            text = file.read_text()
            assert text.rstrip().endswith("PASS"), file
            matches = pattern.findall(text)
            assert len(matches) == cases and len({row[0] for row in matches}) == cases, file
            for name, ns, size, allocations in matches:
                values.setdefault(name, []).append((float(ns), int(size), int(allocations)))
        assert len(values) == cases and all(len(v) == rounds for v in values.values())
        versions[version] = values
    assert versions["baseline"].keys() == versions["candidate"].keys()
    with (root / f"medians-{toolchain}.csv").open("w", newline="") as stream:
        writer = csv.writer(stream, lineterminator="\n")
        writer.writerow([
            "benchmark", "baseline_ns", "candidate_ns", "change_pct",
            "baseline_B", "candidate_B", "baseline_allocs", "candidate_allocs",
            "baseline_ns_min", "baseline_ns_max", "candidate_ns_min", "candidate_ns_max",
        ])
        for name in sorted(versions["baseline"]):
            before = list(zip(*versions["baseline"][name]))
            after = list(zip(*versions["candidate"][name]))
            b, a = list(map(median, before)), list(map(median, after))
            writer.writerow([
                name, b[0], a[0], round(100 * (a[0] / b[0] - 1), 2), b[1], a[1], b[2], a[2],
                min(before[0]), max(before[0]), min(after[0]), max(after[0]),
            ])
    print(f"{toolchain}: validated {2 * rounds * cases} rows")
