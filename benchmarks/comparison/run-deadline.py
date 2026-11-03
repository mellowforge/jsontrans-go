#!/usr/bin/env python3
"""Five-sample matched deadline-pass comparison; run in this directory."""
import csv
import hashlib
import os
from pathlib import Path
import re
import statistics
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
BEFORE = Path(os.environ.get("DEADLINE_BEFORE_SOURCE", "/private/tmp/jsontrans-before-deadline"))
OUT = HERE / "testdata/results/deadline"
CASES = [(1024, "simple", n) for n in (4, 16, 64)] + [
    (1024, "mixed", 16), (1024, "unique", 64),
    (8192, "mixed", 16), (65536, "simple", 64),
]
LABEL = "Deadline-pass"
ENV = dict(os.environ, GOTOOLCHAIN="local", GOCACHE="/tmp/jsontrans-go-build")


def source_hash(root):
    files = list(root.glob("*.go"))
    for folder in ("internal", "jsonata", "jsonpath"):
        files.extend((root / folder).rglob("*.go"))
    h = hashlib.sha256()
    for p in sorted(files):
        if p.name.endswith("_test.go"):
            continue
        h.update(p.relative_to(root).as_posix().encode() + b"\0" + p.read_bytes() + b"\0")
    return h.hexdigest()


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    hashes = {"before": source_hash(BEFORE), "current": source_hash(ROOT)}
    (OUT / "source-before.txt").write_text(str(hashes) + "\n")
    row_re = re.compile(r"^BenchmarkCompiledBatch/(\d+)/(\w+)/N(\d+)/(.+)-\d+\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op$")
    samples = {}
    with tempfile.TemporaryDirectory(prefix="jsontrans-deadline-mod-") as temp:
        mod = Path(temp) / "before.mod"
        mod.write_text((HERE / "go.mod").read_text().replace(
            "replace github.com/mellowforge/jsontrans-go => ../..",
            f"replace github.com/mellowforge/jsontrans-go => {BEFORE}"))
        mod.with_suffix(".sum").write_bytes((HERE / "go.sum").read_bytes())
        for size, kind, n in CASES:
            for version in ("before", "current"):
                cmd = ["go", "test"]
                if version == "before":
                    cmd += [f"-modfile={mod}"]
                cmd += ["-tags", "prepared batch", "-run", "^$", "-bench",
                        f"^BenchmarkCompiledBatch$/{size}/{kind}/N{n}$",
                        "-benchtime=200ms", "-count=5"]
                print(f"{version} {size}/{kind}/N{n}", flush=True)
                result = subprocess.run(cmd, cwd=HERE, env=ENV, text=True,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
                (OUT / f"{version}-{size}-{kind}-N{n}.txt").write_text(result.stdout)
                if result.returncode:
                    raise RuntimeError(result.stdout)
                for line in result.stdout.splitlines():
                    match = row_re.match(line)
                    if match:
                        a, k, count, op, ns, byte_count, allocs = match.groups()
                        key = (version, int(a), k, int(count), op)
                        samples.setdefault(key, []).append(tuple(map(float, (ns, byte_count, allocs))))
    assert len(samples) == len(CASES) * 2 * 6, len(samples)
    assert all(len(rows) == 5 for rows in samples.values())
    after = {"before": source_hash(BEFORE), "current": source_hash(ROOT)}
    (OUT / "source-after.txt").write_text(str(after) + "\n")
    assert hashes == after, "runtime source changed during measurement"
    with (OUT / "medians.csv").open("w", newline="") as f:
        writer = csv.writer(f, lineterminator="\n")
        writer.writerow(["source", "bytes", "workload", "rules", "operation", "samples",
                         "ns/op", "B/op", "allocs/op", "min-ns/op", "max-ns/op"])
        for key, rows in sorted(samples.items()):
            values = [statistics.median(column) for column in zip(*rows)]
            writer.writerow([*key, len(rows), *values, min(r[0] for r in rows), max(r[0] for r in rows)])
    lines = [f"# {LABEL} matched measurements", "",
             "Five 200 ms samples per row; medians in microseconds per document/batch.",
             "Preparation-inclusive rows reuse the destination; all safety defaults active.", "",
             "| KiB | Workload | Rules | Before prepare+batch | After prepare+batch | Warmed gnata | Before reused doc | After reused doc | After B/allocs (reused doc) |",
             "| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |"]
    def median(version, size, kind, n, op, column=0):
        return statistics.median(row[column] for row in samples[(version, size, kind, n, op)])
    for size, kind, n in CASES:
        prep = "prepare_and_batch/reused_dst"
        reuse = "reused_doc/reused_dst"
        values = [median(v, size, kind, n, op) / 1000 for v, op in
                  [("before", prep), ("current", prep), ("current", "gnata_warmed"),
                   ("before", reuse), ("current", reuse)]]
        b = median("current", size, kind, n, reuse, 1)
        a = median("current", size, kind, n, reuse, 2)
        lines.append(f"| {size // 1024} | {kind} | {n} | " + " | ".join(f"{x:.2f}" for x in values) + f" | {b:,.0f}/{a:.0f} |")
    (OUT / "summary.md").write_text("\n".join(lines) + "\n")
    print(f"Verified {sum(map(len, samples.values()))} samples; unchanged runtime hashes.", flush=True)


if __name__ == "__main__":
    main()
