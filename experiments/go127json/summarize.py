#!/usr/bin/env python3
"""Validate complete benchmark runs and summarize their medians and ranges."""
import argparse
from collections import defaultdict
import csv
import json
from pathlib import Path
import re
import statistics

MODES = ["go126", "go127-nojsonv2", "go127", "go127-jsontext"]


def parse(text, count):
    if text.splitlines().count("PASS") != count:
        raise ValueError(f"expected {count} completed processes")
    rows = defaultdict(list)
    for line in text.splitlines():
        if not line.startswith("Benchmark"):
            continue
        fields = line.split()
        name = re.sub(r"-\d+$", "", fields[0])
        metrics = {}
        for unit in ["ns/op", "B/op", "allocs/op"]:
            pos = fields.index(unit)
            metrics[unit] = float(fields[pos - 1])
        rows[name].append(metrics)
    if not rows or any(len(samples) != count for samples in rows.values()):
        raise ValueError("missing or duplicate benchmark samples")
    return {
        name: {unit: {"median": statistics.median(s[unit] for s in samples),
                      "min": min(s[unit] for s in samples),
                      "max": max(s[unit] for s in samples)}
               for unit in ["ns/op", "B/op", "allocs/op"]}
        for name, samples in rows.items()
    }


def summarize(directory):
    metadata = json.loads((directory / "metadata.json").read_text())
    count = metadata["count"]
    summary, rss = {}, {}
    for mode in MODES:
        summary[mode], rss[mode] = {}, {}
        for suite in ["native", "javascript"]:
            path = directory / f"{mode}-{suite}.txt"
            if not path.exists():
                if suite == "native":
                    raise ValueError(f"missing {path}")
                continue
            summary[mode].update(parse(path.read_text(), count))
            memory = (directory / f"{mode}-{suite}-rss.txt").read_text()
            values = re.findall(r"(\d+)\s+maximum resident set size", memory)
            scale = 1
            if not values:
                values = re.findall(r"Maximum resident set size \(kbytes\):\s*(\d+)", memory)
                scale = 1024
            if values:
                if len(values) != count:
                    raise ValueError("incomplete RSS samples")
                rss[mode][suite] = [int(v) * scale for v in values]
        if set(summary[mode]) != set(summary[MODES[0]]):
            raise ValueError(f"benchmark set differs: {mode}")
    (directory / "summary.json").write_text(json.dumps({"benchmarks": summary, "peak_rss_bytes": rss}, indent=2) + "\n")
    with (directory / "summary.csv").open("w", newline="") as file:
        writer = csv.writer(file, lineterminator="\n")
        writer.writerow(["mode", "benchmark", "ns_median", "ns_min", "ns_max", "bytes_median", "allocs_median"])
        for mode, benchmarks in summary.items():
            for name, metrics in benchmarks.items():
                timing = metrics["ns/op"]
                writer.writerow([mode, name, timing["median"], timing["min"], timing["max"], metrics["B/op"]["median"], metrics["allocs/op"]["median"]])
    selected = [
        "1KiB/decode", "64KiB/decode", "array1MiB/decode",
        "64KiB/encode", "64KiB/eval_json/jsonata", "64KiB/eval_json/jsonpath",
        "array1MiB/eval_json/jsonata", "array1MiB/large_output",
        "64KiB/prepare", "64KiB/reuse/N1", "64KiB/reuse/N16", "64KiB/reuse/N64",
        "64KiB/prepare_batch/N16", "64KiB/reuse_mixed/N16",
    ]
    names = ["BenchmarkJSONAPI/" + n for n in selected if "BenchmarkJSONAPI/" + n in summary[MODES[0]]]
    names += [n for n in summary[MODES[0]] if n.startswith("BenchmarkJavaScript/")]
    lines = ["# Local benchmark summary", "", f"{count} samples per configuration; median microseconds per operation. Ranges and allocation counts are in summary.csv/summary.json. Descriptive results; no statistical significance claim.", "",
             "| Workload | Go 1.26 | Go 1.27, old JSON | Go 1.27 | Go 1.27 + prototype |",
             "|---|---:|---:|---:|---:|"]
    for name in names:
        times = [f'{summary[m][name]["ns/op"]["median"]/1000:,.2f}' for m in MODES]
        lines.append("| " + name.replace("BenchmarkJSONAPI/", "").replace("BenchmarkJavaScript/", "JS/") + " | " + " | ".join(times) + " |")
    decode_sizes = [size for size in ["1KiB", "64KiB", "array1MiB"] if f"BenchmarkJSONAPI/{size}/decode" in summary[MODES[0]]]
    if decode_sizes:
        lines += ["", "## Decode allocations", "", "| Workload | Go 1.26 B/op; allocs/op | Go 1.27 B/op; allocs/op | Prototype B/op; allocs/op |", "|---|---:|---:|---:|"]
    for size in decode_sizes:
        name = f"BenchmarkJSONAPI/{size}/decode"
        cells = [f'{summary[m][name]["B/op"]["median"]:,.0f}; {summary[m][name]["allocs/op"]["median"]:,.0f}' for m in ["go126", "go127", "go127-jsontext"]]
        lines.append("| " + size + " | " + " | ".join(cells) + " |")
    lines += ["", "## Whole-process peak RSS", "", "MiB minimum–maximum across benchmark processes. These processes run entire suites and warm the runtime; values are neither per-evaluation nor retained-document memory.", "", "| Configuration | Native suite | JavaScript suite |", "|---|---:|---:|"]
    for mode in MODES:
        cells = []
        for suite in ["native", "javascript"]:
            values = rss[mode].get(suite, [])
            cells.append(f"{min(values)/(1<<20):.1f}–{max(values)/(1<<20):.1f}" if values else "unavailable")
        lines.append("| " + mode + " | " + " | ".join(cells) + " |")
    (directory / "summary.md").write_text("\n".join(lines) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    summarize(parser.parse_args().directory)
