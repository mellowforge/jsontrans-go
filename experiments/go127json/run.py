#!/usr/bin/env python3
"""Build once per configuration, then interleave native and JS benchmark runs.

Use installed toolchain binaries; GOTOOLCHAIN=local prevents implicit downloads.
Results are exploratory local measurements, not release performance promises.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
MODES = {
    "go126": ("go126", "", []),
    "go127-nojsonv2": ("go127", "nojsonv2", []),
    "go127": ("go127", "", []),
    "go127-jsontext": ("go127", "", ["-tags=jsontext_experiment"]),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go126", required=True, type=Path)
    parser.add_argument("--go127", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--count", type=int, default=6)
    parser.add_argument("--benchtime", default="150ms")
    parser.add_argument("--skip-javascript", action="store_true")
    parser.add_argument("--native-pattern", default="BenchmarkJSONAPI")
    args = parser.parse_args()
    if args.count < 1:
        parser.error("count must be positive")
    args.output.mkdir(parents=True, exist_ok=True)
    if any(args.output.iterdir()):
        parser.error("output directory must be empty (avoid mixing measurements)")
    env = dict(os.environ, GOTOOLCHAIN="local", GOWORK="off", GOMAXPROCS="1", GOFLAGS="")
    # Compilation/cache writes are outside timings and contained in a temp path.
    env.setdefault("GOCACHE", str(Path(tempfile.gettempdir()) / "jsontrans-go127-cache"))
    files = sorted(p for p in ROOT.rglob("*") if p.is_file() and (p.suffix == ".go" or p.name in {"go.mod", "go.sum"}) and ".git" not in p.parts and ".superpowers" not in p.parts)
    digest = hashlib.sha256()
    for path in files:
        digest.update(str(path.relative_to(ROOT)).encode() + b"\0" + path.read_bytes() + b"\0")
    metadata = {
        "platform": platform.platform(), "machine": platform.machine(),
        "commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
        "dirty": subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True),
        "go_source_sha256": digest.hexdigest(),
        "count": args.count, "benchtime": args.benchtime, "gomaxprocs": 1,
        "modes": {}, "commands": [], "rss": "High-water RSS for each whole benchmark process, excluding compilation; not per operation or retained document memory.",
    }
    if platform.system() == "Darwin":
        metadata["cpu"] = subprocess.check_output(["sysctl", "-n", "machdep.cpu.brand_string"], text=True).strip()
    suites = [("native", ROOT, args.native_pattern)]
    if not args.skip_javascript:
        suites.append(("javascript", ROOT / "javascript", "BenchmarkJavaScript/(Eval|EvalJSON|PreparedFallback)$"))
    try:
        with tempfile.TemporaryDirectory(prefix="jsontrans-go127-") as tmp:
            binaries = {}
            for mode, (tool, experiment, tags) in MODES.items():
                go = str(getattr(args, tool).resolve())
                mode_env = dict(env, GOEXPERIMENT=experiment)
                metadata["modes"][mode] = {
                    "version": subprocess.check_output([go, "version"], env=mode_env, text=True).strip(),
                    "env": json.loads(subprocess.check_output([go, "env", "-json", "GOOS", "GOARCH", "GOEXPERIMENT", "CGO_ENABLED"], env=mode_env, text=True)),
                    "tags": tags,
                }
                for suite, cwd, _ in suites:
                    binary = str(Path(tmp) / f"{mode}-{suite}.test")
                    cmd = [go, "test", "-c", *tags, "-o", binary, "."]
                    print("Building", mode, suite, flush=True)
                    subprocess.run(cmd, cwd=cwd, env=mode_env, check=True)
                    binaries[mode, suite] = binary
            names = list(MODES)
            for round_id in range(args.count):
                # Rotate configuration order to reduce correlation with drift.
                order = names[round_id % len(names):] + names[:round_id % len(names)]
                for mode in order:
                    mode_env = dict(env, GOEXPERIMENT=MODES[mode][1])
                    for suite, cwd, pattern in suites:
                        cmd = [binaries[mode, suite], "-test.run=^$", f"-test.bench=^{pattern}", f"-test.benchtime={args.benchtime}", "-test.benchmem", "-test.cpu=1", "-test.count=1"]
                        # Use the native platform time utility for a process-wide
                        # memory high-water mark; do not time the go build tool.
                        timed = cmd
                        if platform.system() == "Darwin":
                            timed = ["/usr/bin/time", "-l", *cmd]
                        elif platform.system() == "Linux":
                            timed = ["/usr/bin/time", "-v", *cmd]
                        metadata["commands"].append({"round": round_id + 1, "mode": mode, "suite": suite, "command": timed})
                        print(f"Round {round_id+1}/{args.count}: {mode} {suite}", flush=True)
                        result = subprocess.run(timed, cwd=cwd, env=mode_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                        with (args.output / f"{mode}-{suite}.txt").open("a") as out:
                            out.write(f"\n# Round {round_id+1}\n" + result.stdout)
                        with (args.output / f"{mode}-{suite}-rss.txt").open("a") as out:
                            out.write(f"\n# Round {round_id+1}\n" + result.stderr)
                        result.check_returncode()
    finally:
        (args.output / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")


if __name__ == "__main__":
    main()
