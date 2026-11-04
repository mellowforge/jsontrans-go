#!/usr/bin/env python3
"""Validate the archived readiness matrix, raw evidence and current runtime identity."""
import gzip
import hashlib
import json
from pathlib import Path, PureWindowsPath

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[3]
SOURCE = "acbf01cb0271089c08de2f63ece849067a2137fb"
GUEST = "31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee"


def read(path):
    if path.exists():
        return path.read_bytes()
    return gzip.decompress(path.with_name(path.name + ".gz").read_bytes())


def load(path):
    return json.loads(read(path).decode("utf-8-sig"))


reports = {host: load(ROOT / host / "report.json") for host in ("mac", "linux", "windows")}
summary = {}
for host, report in reports.items():
    assert report["source_commit"] == SOURCE, host
    assert report["guest_sha256"] == GUEST, host
    assert report["complete"] is True and report["passed"] is True, host
    if host == "windows":
        steps = report["steps"]
        assert [s["name"] for s in steps] == ["vet", "test", "race", "module-verify", "example-build", "example-run"]
        for step in steps:
            if "stdout" in step:
                assert read(ROOT / host / PureWindowsPath(step["stdout"]).name) is not None
                assert read(ROOT / host / PureWindowsPath(step["stderr"]).name) is not None
        assert read(ROOT / host / "example.out").strip()
    else:
        assert all(g["state"] == "passed" for g in report["groups"].values()), host
        steps = [s for group in report["groups"].values() for s in group["steps"]]
        for step in steps:
            log = read(ROOT / host / step["log"])
            if step["name"] == "format":
                assert not log.strip(), host
        for group, version in (("native-126", "go1.26.10"), ("native-127", "go1.27.3")):
            native = report["groups"][group]["steps"]
            checks = [s for s in native if s["name"] != "format"]
            assert len(checks) == 12
            assert all(s["environment"]["GOTOOLCHAIN"] == version for s in checks)
    assert all(s["exit_code"] == 0 and "error" not in s for s in steps), host
    summary[host] = len(steps)
assert summary == {"mac": 25, "linux": 49, "windows": 6}, summary
fuzz = reports["linux"]["groups"]["fuzz"]["steps"]
assert len(fuzz) == 11 and all(any(a.startswith("-fuzztime=") and a.endswith("x") for a in s["command"]) for s in fuzz)
probes = load(ROOT / "linux" / "owned-probes.json")["cases"]
assert len(probes) == 21 and all(p["status"] == "PASS" for p in probes)
soak = json.loads(read(ROOT / "linux" / "probes-soak-recovery-soak.log"))
assert soak["verified_recoveries"] == 1200
assert sum(v["calls"] for v in soak["outcomes"].values()) == 1200
assert len(soak["construction_ns"]) == len(soak["close_ns"]) == 3
for target in ("linux-amd64", "windows-amd64", "darwin-arm64"):
    folder = ROOT / "linux" / ("security-" + target)
    report = load(folder / "report.json")
    assert report["status"] == "passed" and len(report["source_scans"]) == 3
    for scan in report["source_scans"] + [report["binary_scan"]]:
        assert not scan["findings"]
        assert hashlib.sha256(read(folder / scan["raw_file"])).hexdigest() == scan["raw_sha256"]
identity = load(ROOT / "runtime-identity.json")
assert identity["candidate_source"] == SOURCE and identity["all_identical"] is True
for name, digest in identity["identical_files"].items():
    assert hashlib.sha256((REPO / name).read_bytes()).hexdigest() == digest, name
manifest = dict(line.split("  ", 1)[::-1] for line in (ROOT / "SHA256SUMS").read_text().splitlines())
assert set(manifest) == {str(p.relative_to(ROOT)) for p in ROOT.rglob("*") if p.is_file() and p.name != "SHA256SUMS"}
for name, digest in manifest.items():
    assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
print("PASS:", sum(summary.values()), "steps; 21 probes; 1200 recovery pairs; 3 security targets;", len(manifest), "evidence hashes; unchanged runtime")
