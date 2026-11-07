#!/usr/bin/env python3
"""Run native byte-to-byte experiment profiles sequentially on Linux."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile

MODULE = Path(__file__).resolve().parent
ROOT = MODULE.parents[1]
PROFILES = {
    "native127": ("127", "^BenchmarkBytes$/(jsonata|jsonpath)/(scalar|object|array|fallback|bindings|limits|host|output)/(1024|65536)/N(1|4|16)$", "1", [8]),
    "sizes127": ("127", "^BenchmarkBytes$/(jsonata|jsonpath)/scalar/(512|8192)/N(1|4|16)$", "1", [8]),
    "large127": ("127", "^BenchmarkBytes$/(jsonata|jsonpath)/large-array/1048576/N1$", "1", [8]),
    "parallel127": ("127", "^BenchmarkParallelBytes$", "1,2,4,8", [0,2,4,6,8,10,12,14]),
    "javascript127": ("127", "^BenchmarkJavaScriptBytes$", "1", [8]),
    "control126": ("126", "^BenchmarkBytes$/jsonata/(scalar|object)/(1024|65536)/N(1|16)$", "1", [8]),
    "jsontext127": ("jsontext", "^BenchmarkBytes$/jsonata/(scalar|object)/(1024|65536)/N(1|16)$", "1", [8]),
}


def source_hash():
    digest = hashlib.sha256()
    for path in sorted(ROOT.rglob("*")):
        if not path.is_file() or ".git" in path.parts or ".superpowers" in path.parts:
            continue
        rel = path.relative_to(ROOT)
        # Other experiments and the later nested NATS module are separate artifacts.
        if rel.parts[0] == "experiments" and (rel.parts[:2] != ("experiments", "messages") or "nats" in rel.parts):
            continue
        if path.suffix not in {".go", ".wasm"} and path.name not in {"go.mod", "go.sum"}:
            continue
        digest.update(str(rel).encode() + b"\0" + path.read_bytes() + b"\0")
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go126", type=Path, required=True)
    parser.add_argument("--go127", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--rounds", type=int, default=6)
    parser.add_argument("--benchtime", default="75ms")
    parser.add_argument("--profiles", nargs="+", choices=list(PROFILES), default=list(PROFILES))
    args = parser.parse_args()
    if args.rounds < 1:
        parser.error("rounds must be positive")
    args.output.mkdir(parents=True, exist_ok=True)
    if any(args.output.iterdir()):
        parser.error("output must be empty")
    env = dict(os.environ, GOTOOLCHAIN="local", GOWORK="off", GOFLAGS="", GOEXPERIMENT="")
    metadata = {"platform": platform.platform(), "source_sha256": source_hash(), "profiles": {}, "rounds": args.rounds, "benchtime": args.benchtime,
                "commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
                "dirty": subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True), "commands": [], "requested_profiles": list(args.profiles), "completed": False, "final_source_sha256": None}
    if Path("/proc/cpuinfo").exists():
        metadata["cpu"] = next(line.split(":",1)[1].strip() for line in Path("/proc/cpuinfo").read_text().splitlines() if line.startswith("model name"))
    try:
        with tempfile.TemporaryDirectory(prefix="message-bench-") as tmp:
            binaries = {}
            for mode, go, tags in [("126", args.go126, []), ("127", args.go127, []), ("jsontext", args.go127, ["-tags=jsontext_experiment"])]:
                binary = str(Path(tmp) / (mode + ".test"))
                print("BUILD", mode, flush=True)
                subprocess.run([str(go), "test", "-c", *tags, "-o", binary, "."], cwd=MODULE, env=env, check=True)
                binaries[mode] = binary
                metadata[mode] = {"version": subprocess.check_output([str(go), "version"], env=env, text=True).strip(), "tags": tags}
            with (args.output / "inventory.json").open("w") as f:
                subprocess.run([str(args.go127), "run", "./cmd/inventory"], cwd=MODULE, env=env, stdout=f, check=True)
            for round_id in range(args.rounds):
                order = args.profiles[round_id % len(args.profiles):] + args.profiles[:round_id % len(args.profiles)]
                for profile in order:
                    mode, pattern, cpus, affinity = PROFILES[profile]
                    metadata["profiles"][profile] = {"mode": mode, "pattern": pattern, "cpu": cpus, "affinity": affinity}
                    if not set(affinity).issubset(os.sched_getaffinity(0)):
                        raise ValueError(f"required CPUs unavailable: {affinity}")
                    cmd = ["taskset", "-c", ",".join(map(str, affinity)), "/usr/bin/time", "-v", binaries[mode], "-test.run=^$", "-test.bench=" + pattern,
                           "-test.cpu=" + cpus, "-test.benchtime=" + args.benchtime, "-test.benchmem", "-test.count=1"]
                    bench_env = dict(env, GOMAXPROCS="1", MESSAGE_STRATEGY_ORDER=str(round_id % 3))
                    print(f"ROUND {round_id+1}/{args.rounds} {profile}", flush=True)
                    result = subprocess.run(cmd, cwd=MODULE, env=bench_env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                    with (args.output / (profile + ".txt")).open("a") as f:
                        f.write(f"\n# round {round_id+1}; strategy rotation {round_id%3}\n" + result.stdout)
                    with (args.output / (profile + "-rss.txt")).open("a") as f:
                        f.write(f"\n# round {round_id+1}\n" + result.stderr)
                    metadata["commands"].append({"profile": profile, "round": round_id+1, "strategy_rotation": round_id%3, "command": cmd, "exit_code": result.returncode})
                    result.check_returncode()
            metadata["final_source_sha256"] = source_hash()
            if metadata["final_source_sha256"] != metadata["source_sha256"]:
                raise ValueError("source changed during measurement")
            metadata["completed"] = True
    finally:
        (args.output / "metadata.json").write_text(json.dumps(metadata, indent=2)+"\n")


if __name__ == "__main__":
    main()
