#!/usr/bin/env python3
"""Summarize matched batch samples without third-party dependencies."""

import csv
import pathlib
import re
import statistics
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "testdata/results/batch")
pattern = re.compile(
    r"^Benchmark(?:Prepared|CompiledBatch)/(\d+)/(simple|mixed|unique)/N(\d+)/"
    r"(.+)-\d+\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op$"
)
samples = {}
for source, filename in [
    ("before", "before-prepared.txt"),
    ("current", "current-batch.txt"),
    ("current", "current-prepare.txt"),
]:
    for line in (root / filename).read_text().splitlines():
        match = pattern.match(line.strip())
        if match:
            size, kind, count, operation, ns, size_bytes, allocs = match.groups()
            key = source, int(size), kind, int(count), operation
            samples.setdefault(key, []).append(tuple(map(float, (ns, size_bytes, allocs))))

if len(samples) != 270 or any(len(values) != 5 for values in samples.values()):
    raise ValueError("Expected the complete matrix: 270 rows with five samples each")

with (root / "medians.csv").open("w", newline="") as handle:
    writer = csv.writer(handle)
    writer.writerow(["source", "bytes", "workload", "rules", "operation", "samples", "ns/op", "B/op", "allocs/op", "min-ns/op", "max-ns/op"])
    for key, values in sorted(samples.items()):
        medians = [statistics.median(v[i] for v in values) for i in range(3)]
        writer.writerow([*key, len(values), *medians, min(v[0] for v in values), max(v[0] for v in values)])


def median(source, size, kind, count, operation):
    values = samples[source, size, kind, count, operation]
    if len(values) != 5:
        raise ValueError(f"Expected five samples: {(source, size, kind, count, operation)}")
    return tuple(statistics.median(v[i] for v in values) for i in range(3))


lines = [
    "# Compiled batch development measurements",
    "",
    "Five samples per row; medians. Times are microseconds per whole document/rule set.",
    "Preparation-inclusive current rows reuse the result destination. Safety defaults remain active.",
    "These are sequential development measurements on a shared machine, not an isolated release benchmark.",
    "",
    "| KiB | Rules | Workload | Old prepare + loop | New prepare + batch | New/old | Warmed gnata | New/gnata | Reused doc + dst | Fresh dst B / allocs | Reused dst B / allocs |",
    "| ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
]
for size in [1024, 8192, 65536]:
    for kind in ["simple", "mixed", "unique"]:
        for count in [4, 16, 64]:
            old = median("before", size, kind, count, "prepare_and_eval")
            new = median("current", size, kind, count, "prepare_and_batch/reused_dst")
            gnata = median("current", size, kind, count, "gnata_warmed")
            reuse = median("current", size, kind, count, "reused_doc/reused_dst")
            fresh = median("current", size, kind, count, "reused_doc/new_dst")
            lines.append(
                f"| {size // 1024} | {count} | {kind} | {old[0] / 1000:.2f} | {new[0] / 1000:.2f} | "
                f"{new[0] / old[0]:.2f}× | {gnata[0] / 1000:.2f} | {new[0] / gnata[0]:.2f}× | "
                f"{reuse[0] / 1000:.2f} | {fresh[1]:.0f} / {fresh[2]:.0f} | {reuse[1]:.0f} / {reuse[2]:.0f} |"
            )
lines += [
    "",
    "Preparation-only medians below use simple/N16; the input is identical for every rule set.",
    "",
    "| KiB | Old prepare µs | New prepare µs | Old B / allocs | New B / allocs |",
    "| ---: | ---: | ---: | ---: | ---: |",
]
for size in [1024, 8192, 65536]:
    old = median("before", size, "simple", 16, "prepare_only")
    new = median("current", size, "simple", 16, "prepare_only")
    lines.append(f"| {size // 1024} | {old[0] / 1000:.2f} | {new[0] / 1000:.2f} | {old[1]:.0f} / {old[2]:.0f} | {new[1]:.0f} / {new[2]:.0f} |")
(root / "summary.md").write_text("\n".join(lines) + "\n")
print(f"Summarized {sum(map(len, samples.values()))} samples into {root / 'summary.md'}")
