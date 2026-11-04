#!/usr/bin/env python3
"""Validate recorded diagnostic runs and summarize first/subsequent GC samples."""
import json
import statistics
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def read_text(path):
    data = path.read_bytes()
    return data.decode("utf-16" if data.startswith((b"\xff\xfe", b"\xfe\xff")) else "utf-8-sig")


def read_json(path):
    return json.loads(read_text(path))


def analyze(name, cycles):
    folder = ROOT / name
    assert read_json(folder / "exit.json")["exit_code"] == 0, name
    assert "PASS" in read_text(folder / "test.log"), name
    rows = [json.loads(line) for line in (folder / "samples.jsonl").read_text().splitlines()]
    assert rows[-1]["cycles"] == cycles, name
    assert rows[-1]["recoveries"] == cycles * 25 * 2, name
    samples = rows[:-1]
    assert len(samples) == cycles * 3, name
    assert [(s["cycle"], s["gc"]) for s in samples] == [
        (cycle, gc) for cycle in range(1, cycles + 1) for gc in (1, 2, 3)
    ], name
    out = {"cycles": cycles, "verified_recoveries": rows[-1]["recoveries"], "gc_passes": {}}
    for gc in (1, 2, 3):
        data = [s["heap_alloc"] for s in samples if s["gc"] == gc]
        out["gc_passes"][str(gc)] = {
            "min_bytes": min(data), "median_bytes": statistics.median(data),
            "max_bytes": max(data), "over_1_mib": sum(x > 1 << 20 for x in data),
        }
    if "pre_gc_goroutines" in samples[0]:
        counts = Counter((s["pre_gc_goroutines"], s["heap_alloc"] > 1 << 20)
                         for s in samples if s["gc"] == 1)
        out["pre_gc_correlation"] = [
            {"goroutines": g, "over_1_mib": high, "cycles": n}
            for (g, high), n in sorted(counts.items())
        ]
    return out


result = {name: analyze(name, cycles) for name, cycles in
          (("probe2", 40), ("baseline80", 80), ("wait80", 80))}
for name in result:
    assert result[name]["gc_passes"]["2"]["over_1_mib"] == 0, name
    assert result[name]["gc_passes"]["3"]["over_1_mib"] == 0, name
assert result["baseline80"]["gc_passes"]["1"]["over_1_mib"] > 0
assert result["wait80"]["gc_passes"]["1"]["over_1_mib"] == 0
stacks = ROOT / "stacks10"
assert read_json(stacks / "exit.json")["exit_code"] == 0
files = sorted(stacks.glob("stacks-*.txt"))
assert len(files) == 10
result["stacks10"] = {"snapshots": len(files), "monitor_frames_per_snapshot": [
    p.read_text().count("CloseModuleOnCanceledOrTimeout.gowrap1()") for p in files
]}
assert all(result["stacks10"]["monitor_frames_per_snapshot"])
print(json.dumps(result, indent=2))
