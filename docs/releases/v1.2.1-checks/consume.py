#!/usr/bin/env python3
"""Reproduce the pre-tag core checks through public module downloads.

Use a new output directory and absolute paths to Go 1.26.10 and Go 1.27.3.
This verifies the pinned merged implementation, not an unpublished v1.2.1 tag.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess

CORE = "github.com/mellowforge/jsontrans-go"
VERSION = "v1.2.1-0.20261108161751-949cd9b13a68"
HERE = Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go126", required=True, type=Path)
    parser.add_argument("--go127", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    (output / "empty").mkdir()
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="local", CGO_ENABLED="0",
               GOMODCACHE=str(output / "module-cache"),
               GOPROXY="https://proxy.golang.org", GOSUMDB="sum.golang.org",
               GOPRIVATE="", GONOPROXY="", GONOSUMDB="", GOFLAGS="",
               GOEXPERIMENT="")
    records = []

    def run(name, command, cwd):
        result = subprocess.run(list(map(str, command)), cwd=cwd, env=env,
                                text=True, capture_output=True, timeout=300)
        (output / (name + ".stdout")).write_text(result.stdout)
        (output / (name + ".stderr")).write_text(result.stderr)
        records.append(dict(name=name, command=list(map(str, command)),
                            exit_code=result.returncode))
        (output / "results.json").write_text(json.dumps(records, indent=2) + "\n")
        if result.returncode:
            raise SystemExit(result.stdout + result.stderr)
        return result.stdout

    toolchains = [("1.26.10", args.go126.resolve()), ("1.27.3", args.go127.resolve())]
    for version, go in toolchains:
        actual = run(version + "-version", [go, "env", "GOVERSION"], output).strip()
        if actual != "go" + version:
            raise SystemExit("unexpected toolchain: " + actual)
    for kind in ("native", "javascript"):
        work = output / kind
        work.mkdir()
        requirements = CORE + " " + VERSION + "\n"
        if kind == "javascript":
            requirements += CORE + "/javascript v1.0.1\n"
        (work / "go.mod").write_text("module example.com/" + kind +
                                    "\n\ngo 1.26.0\n\nrequire (\n" + requirements + ")\n")
        (work / "main.go").write_bytes((HERE / (kind + ".go.txt")).read_bytes())
        run(kind + "-tidy", [args.go126, "mod", "tidy"], work)
        stream = run(kind + "-modules", [args.go126, "list", "-m", "-json", "all"], work)
        modules = []
        decoder = json.JSONDecoder()
        while stream.strip():
            item, end = decoder.raw_decode(stream.lstrip())
            modules.append(item)
            stream = stream.lstrip()[end:]
        if any(item.get("Replace") for item in modules):
            raise SystemExit("consumer contains a replacement")
        versions = {item["Path"]: item.get("Version") for item in modules}
        if versions.get(CORE) != VERSION:
            raise SystemExit("consumer selected another core")
        if kind == "javascript" and versions.get(CORE + "/javascript") != "v1.0.1":
            raise SystemExit("consumer selected another JavaScript module")
        deps = run(kind + "-deps", [args.go126, "list", "-deps", "."], work).splitlines()
        forbidden = ["github.com/nats-io/", "github.com/testcontainers/", CORE + "/experiments/",
                     "github.com/dop251/goja", "github.com/fastschema/qjs"]
        forbidden += ([CORE + "/javascript", "github.com/tetratelabs/wazero"] if kind == "native"
                      else [CORE + "/jsonata", CORE + "/jsonpath"])
        if any(dep.startswith(prefix) for dep in deps for prefix in forbidden):
            raise SystemExit("production dependency boundary violated")
        if kind == "native" and CORE + "/javascript" in versions:
            raise SystemExit("native consumer selected JavaScript")
        run(kind + "-verify", [args.go126, "mod", "verify"], work)
        for version, go in toolchains:
            binary = output / (kind + "-" + version + (".exe" if os.name == "nt" else ""))
            run(kind + "-" + version + "-build", [go, "build", "-mod=readonly", "-o", binary, "."], work)
            print(run(kind + "-" + version + "-run", [binary], output / "empty").strip())
    print("Pinned core, released JavaScript, checksums and dependency boundaries: PASS")


if __name__ == "__main__":
    main()
