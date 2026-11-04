#!/usr/bin/env python3
"""Scan Go source/tests and a CGO-free example; reject findings and coverage gaps.

Embedded QuickJS/WASI code is outside govulncheck's scope. The separate manual
engine review remains a release gate even when this command succeeds.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys


TOOL_VERSION = "v1.9.0"
DATABASE = "https://vuln.go.dev"
ROOT = Path(__file__).resolve().parent.parent
MODULES = (".", "javascript", "experiments/javascript")


class GateError(Exception):
    pass


def decode_stream(raw):
    decoder = json.JSONDecoder()
    messages = []
    remaining = raw
    try:
        while remaining.strip():
            remaining = remaining.lstrip()
            value, end = decoder.raw_decode(remaining)
            if not isinstance(value, dict):
                raise GateError("expected a JSON object")
            messages.append(value)
            remaining = remaining[end:]
    except (ValueError, TypeError) as exc:
        raise GateError(f"invalid JSON stream: {exc}") from exc
    return messages


def check_scan(result, output, mode, go_version):
    output.write_text(result.stdout)
    output.with_suffix(".stderr.txt").write_text(result.stderr)
    if result.returncode:
        raise GateError(f"scanner exited {result.returncode}: {result.stderr[-2000:]}")
    messages = decode_stream(result.stdout)
    allowed = {"config", "SBOM", "progress", "osv", "finding"}
    if any(len(m) != 1 or not set(m) <= allowed or not isinstance(next(iter(m.values())), dict) for m in messages):
        raise GateError("unexpected scanner protocol message")
    configs = [m["config"] for m in messages if "config" in m]
    sboms = [m["SBOM"] for m in messages if "SBOM" in m]
    if len(configs) != 1 or len(sboms) != 1:
        raise GateError("scan requires exactly one config and SBOM")
    config, sbom = configs[0], sboms[0]
    expected = {"protocol_version": "v1.0.0", "scanner_name": "govulncheck",
                "scanner_version": TOOL_VERSION, "db": DATABASE,
                "scan_mode": mode, "scan_level": "symbol"}
    if any(config.get(k) != v for k, v in expected.items()):
        raise GateError(f"unexpected scanner config: {config}")
    try:
        modified = datetime.datetime.fromisoformat(config["db_last_modified"].replace("Z", "+00:00"))
        if modified.tzinfo is None:
            raise ValueError("database timestamp has no timezone")
    except (KeyError, TypeError, ValueError, AttributeError) as exc:
        raise GateError("missing/invalid database timestamp") from exc
    if sbom.get("go_version") != go_version or config.get("go_version", go_version) != go_version:
        raise GateError("scanner/build toolchain version mismatch")
    if not isinstance(sbom.get("modules"), list) or not sbom["modules"]:
        raise GateError("missing module inventory in scanner SBOM")
    for module in sbom["modules"]:
        if not isinstance(module, dict) or not isinstance(module.get("path"), str) or not module["path"] or not isinstance(module.get("version", ""), str):
            raise GateError("invalid SBOM module")
    if not isinstance(sbom.get("roots"), list) or not sbom["roots"] or any(not isinstance(p, str) or not p for p in sbom["roots"]):
        raise GateError("missing scanner package roots")
    findings = [m["finding"] for m in messages if "finding" in m]
    if findings:
        ids = sorted({str(f.get("osv", "invalid finding")) for f in findings})
        raise GateError("vulnerability findings: " + ", ".join(ids))
    return {"config": config, "sbom": sbom, "findings": findings,
            "raw_sha256": hashlib.sha256(result.stdout.encode()).hexdigest(),
            "raw_file": output.name}


def normalize_modules(modules, aliases, own):
    result = set()
    for path, version in modules:
        if path in aliases:
            path = aliases[path]
        elif path.startswith(".") or Path(path).is_absolute():
            raise GateError(f"unrecognized local replacement: {path}")
        if path in own:
            version = ""
        elif path == "stdlib" and version.startswith("go"):
            # Binary mode uses go1.x; source mode uses v1.x (govulncheck v1.9).
            version = "v" + version.removeprefix("go")
        elif not version:
            raise GateError(f"external module has no version: {path}")
        result.add((path, version))
    return result


def require_coverage(required, observed, label):
    missing = required - observed
    if missing:
        names = [p + ("@" + v if v else "") for p, v in sorted(missing)]
        raise GateError(f"{label} coverage missing: " + ", ".join(names))


def run(command, cwd, env, check=True):
    try:
        result = subprocess.run(command, cwd=cwd, env=env, text=True,
                                capture_output=True, timeout=300)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise GateError(f"command failed: {command[0]}: {exc}") from exc
    if check and result.returncode:
        raise GateError(f"command exited {result.returncode}: {' '.join(command)}\n{result.stderr[-2000:]}")
    return result


def module_key(module, workspace, aliases):
    path, version = module["Path"], module.get("Version", "")
    replacement = module.get("Replace")
    if replacement:
        if replacement.get("Version"):
            path, version = replacement["Path"], replacement["Version"]
        else:
            directory = Path(replacement["Dir"]).resolve()
            if directory not in workspace:
                raise GateError(f"local replacement outside scanned workspace: {path}")
            path, version = workspace[directory], ""
            aliases[replacement["Path"]] = path
    if path in workspace.values():
        version = ""
    if not version and path not in workspace.values():
        raise GateError(f"unknown dependency version: {path}")
    return path, version


def inventory(directory, pattern, tests, workspace, env):
    selected = decode_stream(run(["go", "list", "-m", "-json", "all"], directory, env).stdout)
    aliases = {}
    selected_keys = {module_key(m, workspace, aliases) for m in selected}
    command = ["go", "list", "-deps", "-json"]
    if tests:
        command.append("-test")
    packages = decode_stream(run(command + [pattern], directory, env).stdout)
    imported, roots = set(), set()
    for package in packages:
        if package.get("Error") or package.get("DepsErrors") or package.get("Incomplete"):
            raise GateError("incomplete Go package inventory")
        if not package.get("DepOnly"):
            roots.add(package["ImportPath"].split(" [", 1)[0])
        if package.get("Module"):
            imported.add(module_key(package["Module"], workspace, aliases))
    return {"selected": selected_keys, "imported": imported, "roots": roots, "aliases": aliases}


def scan(scanner, directory, arguments, output, mode, version, env):
    # JSON mode exits zero even for findings. check_scan must always parse it.
    result = run([scanner, "-db", DATABASE, "-json"] + arguments, directory, env, check=False)
    return check_scan(result, output, mode, version)


def pairs(values):
    return [{"path": p, "version": v} for p, v in sorted(values)]


def execute(scanner, output):
    env = dict(os.environ, GOWORK="off")
    build = json.loads(run(["go", "env", "-json", "GOVERSION", "GOOS", "GOARCH"], ROOT, env).stdout)
    version = build["GOVERSION"]
    workspace = {}
    for relative in MODULES:
        directory = (ROOT / relative).resolve()
        main = json.loads(run(["go", "list", "-m", "-json"], directory, env).stdout)
        workspace[directory] = main["Path"]
    own = set(workspace.values())
    stdlib = {("stdlib", "v" + version.removeprefix("go"))}
    required, observed = set(stdlib), set()
    report = {"status": "running", "target": build, "tool_version": TOOL_VERSION,
              "source_scans": [], "embedded_C_review": "separate release gate"}
    try:
        for i, relative in enumerate(MODULES):
            directory = ROOT / relative
            inv = inventory(directory, "./...", True, workspace, env)
            record = scan(scanner, directory, ["-test", "./..."], output / f"source-{i}.json", "source", version, env)
            require_coverage(inv["roots"], set(record["sbom"]["roots"]), f"{relative} package roots")
            covered = normalize_modules({(m["path"], m.get("version", "")) for m in record["sbom"]["modules"]}, inv["aliases"], own)
            required |= inv["imported"]
            observed |= covered
            record.update(module=workspace[directory.resolve()], imported=pairs(inv["imported"]),
                          selected=pairs(inv["selected"]), selected_not_imported=pairs(inv["selected"] - inv["imported"]))
            report["source_scans"].append(record)
            print(f"{relative}: no reported findings", flush=True)
        require_coverage(required, observed, "combined source module/version")
        binary_env = dict(env, CGO_ENABLED="0")
        inv = inventory(ROOT / "javascript", "./examples/basic", False, workspace, binary_env)
        binary = output / ("public-example.exe" if build["GOOS"] == "windows" else "public-example")
        run(["go", "build", "-trimpath", "-o", str(binary), "./examples/basic"], ROOT / "javascript", binary_env)
        record = scan(scanner, ROOT / "javascript", ["-mode=binary", str(binary)], output / "binary.json", "binary", version, binary_env)
        covered = normalize_modules({(m["path"], m.get("version", "")) for m in record["sbom"]["modules"]}, inv["aliases"], own)
        require_coverage(inv["imported"] | stdlib, covered, "binary module/version")
        # Binary SBOM roots contain the main MODULE, not the source package.
        require_coverage({workspace[(ROOT / "javascript").resolve()]}, set(record["sbom"]["roots"]), "binary main module")
        record.update(imported=pairs(inv["imported"]), source_roots=sorted(inv["roots"]),
                      binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest())
        report["binary_scan"] = record
        report["status"] = "passed"
        print("Source and CGO-free binary findings/coverage: PASS", flush=True)
        return report
    except Exception as exc:
        report.update(status="failed", error=str(exc))
        raise
    finally:
        (output / "report.json").write_text(json.dumps(report, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--govulncheck", default="govulncheck")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    try:
        execute(args.govulncheck, args.output)
    except (GateError, KeyError, ValueError) as exc:
        print(f"Go security gate failed: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
