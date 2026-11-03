#!/usr/bin/env python3
"""Summarize five benchmark samples; raw observations remain authoritative."""
import collections, json, pathlib, re, statistics
root = pathlib.Path(__file__).parent / 'testdata/results'
rows = collections.defaultdict(list)
for line in (root / 'bench.txt').read_text().splitlines():
    match = re.match(r'(Benchmark\S+)\s+\d+\s+([\d.]+) ns/op\s+(?:[\d.]+ MB/s\s+)?(\d+) B/op\s+(\d+) allocs/op', line)
    if match:
        name, ns, size, allocs = match.groups()
        rows[name].append((float(ns), int(size), int(allocs)))
summary = {name: dict(samples=len(samples), ns_op=statistics.median(x[0] for x in samples),
                     bytes_op=statistics.median(x[1] for x in samples),
                     allocs_op=statistics.median(x[2] for x in samples)) for name, samples in rows.items()}
(root / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
for name, row in summary.items():
    print(f'{name}: {row["ns_op"]/1000:.1f} us, {row["bytes_op"]} B/op, {row["allocs_op"]} allocs/op')
