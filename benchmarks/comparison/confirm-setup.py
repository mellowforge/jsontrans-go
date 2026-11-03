#!/usr/bin/env python3
"""Confirm setup timings with alternating source order and fixed binaries."""
import csv
import importlib.util
import os
from pathlib import Path
import re
import statistics
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("matched_batches", HERE / "run-deadline.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
BEFORE = Path(os.environ.get("SETUP_BEFORE_SOURCE", "/private/tmp/jsontrans-before-setup"))
OUT = HERE / "testdata/results/setup/confirmation"
CASES = [("simple", 64), ("mixed", 16), ("unique", 64)]
SAMPLES = 5
# Benchmark sub-operations matched by the pattern below, per source version.
OPERATIONS = ["reused_doc/new_dst", "reused_doc/reused_dst", "prepare_and_batch/new_dst",
              "prepare_and_batch/reused_dst", "gnata_warmed"]
ROW = re.compile(r"^BenchmarkCompiledBatch/1024/(\w+)/N(\d+)/(.+)-\d+\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op$")


def run(cmd):
    result = subprocess.run(cmd, cwd=HERE, env=runner.ENV, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if result.returncode:
        raise RuntimeError(result.stdout)
    return result.stdout


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    hashes = runner.source_hashes(BEFORE)
    (OUT / "source-before.txt").write_text(str(hashes) + "\n")
    samples = {}
    with tempfile.TemporaryDirectory(prefix="jsontrans-setup-confirm-") as temp:
        temp = Path(temp)
        mod = runner.write_before_modfile(temp, BEFORE)
        binaries = {v: temp / f"{v}.test" for v in ("before", "current")}
        for version, binary in binaries.items():
            cmd = ["go", "test", "-c", "-tags", "prepared batch", "-o", str(binary)]
            if version == "before":
                cmd += [f"-modfile={mod}"]
            run(cmd)
        for kind, n in CASES:
            for sample in range(SAMPLES):
                order = ("before", "current") if sample % 2 == 0 else ("current", "before")
                for version in order:
                    print(f"{kind}/N{n} sample {sample + 1} {version}", flush=True)
                    pattern = (f"^BenchmarkCompiledBatch$/^1024$/^{kind}$/^N{n}$/"
                               "^(reused_doc|prepare_and_batch|gnata_warmed)$")
                    output = run([str(binaries[version]), "-test.run=^$", f"-test.bench={pattern}", "-test.benchtime=300ms", "-test.count=1"])
                    (OUT / f"{version}-{kind}-N{n}-{sample + 1}.txt").write_text(output)
                    for line in output.splitlines():
                        match = ROW.match(line)
                        if match:
                            workload, count, op, ns, byte_count, allocs = match.groups()
                            key = (version, workload, int(count), op)
                            samples.setdefault(key, []).append(tuple(map(float, (ns, byte_count, allocs))))
    runner.require(len(samples) == len(CASES) * 2 * len(OPERATIONS), f"unexpected operation count {len(samples)}")
    runner.require(all(len(rows) == SAMPLES for rows in samples.values()), "missing samples")
    after = runner.source_hashes(BEFORE)
    (OUT / "source-after.txt").write_text(str(after) + "\n")
    runner.require(hashes == after, "runtime source changed")
    with (OUT / "medians.csv").open("w", newline="") as f:
        writer = csv.writer(f, lineterminator="\n")
        writer.writerow(["source", "workload", "rules", "operation", "samples", "ns/op", "B/op", "allocs/op", "min-ns/op", "max-ns/op"])
        for key, rows in sorted(samples.items()):
            writer.writerow([*key, len(rows), *(statistics.median(c) for c in zip(*rows)), min(r[0] for r in rows), max(r[0] for r in rows)])
    lines = ["# Alternating-order setup confirmation", "",
             "Five 300ms samples per row; before/current order reverses each round.",
             "Same compiled harness binaries; 1 KiB fixtures, reused destination, production defaults.", "",
             "| Workload | Rules | Before prepare+batch µs | After prepare+batch µs | Warmed gnata µs | Before reuse µs | After reuse µs |",
             "| --- | ---: | ---: | ---: | ---: | ---: | ---: |"]
    def median(v, kind, n, op):
        return statistics.median(r[0] for r in samples[(v, kind, n, op)]) / 1000
    for kind, n in CASES:
        prep, reuse = "prepare_and_batch/reused_dst", "reused_doc/reused_dst"
        values = [median(v, kind, n, op) for v, op in [("before", prep), ("current", prep), ("current", "gnata_warmed"), ("before", reuse), ("current", reuse)]]
        lines.append(f"| {kind} | {n} | " + " | ".join(f"{v:.2f}" for v in values) + " |")
    (OUT / "summary.md").write_text("\n".join(lines) + "\n")
    print(f"Verified {sum(map(len, samples.values()))} samples; unchanged runtime hashes.", flush=True)


if __name__ == "__main__":
    main()
