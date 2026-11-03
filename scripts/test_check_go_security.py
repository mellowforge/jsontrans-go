import json
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock

import check_go_security as gate


def clean(mode="source", modules=None, roots=None):
    return [
        {"config": {"protocol_version": "v1.0.0", "scanner_name": "govulncheck",
                    "scanner_version": "v1.9.0", "db": "https://vuln.go.dev",
                    "db_last_modified": "2026-11-03T00:00:00Z",
                    "scan_mode": mode, "scan_level": "symbol"}},
        {"SBOM": {"go_version": "go1.26.10", "modules": modules or [
            {"path": "example.com/app"},
            {"path": "stdlib", "version": "v1.26.10"}],
            "roots": roots or ["example.com/app"]}},
        {"progress": {"message": "done"}},
    ]


class ScanTests(unittest.TestCase):
    def scan(self, messages, code=0, mode="source"):
        # Only the external scanner is replaced; parsing, process-status handling
        # and raw-evidence retention run through the real gate.
        raw = "\n".join(json.dumps(m) for m in messages)
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "scan.json"
            result = subprocess.CompletedProcess(["govulncheck"], code, raw, "network failure" if code else "")
            return gate.check_scan(result, path, mode, "go1.26.10")

    def test_zero_exit_with_any_finding_fails(self):
        for trace in [[{"module": "example.com/dep", "version": "v1.0.0"}],
                      [{"module": "example.com/dep", "package": "example.com/dep/pkg", "function": "Bad"}]]:
            with self.subTest(trace=trace), self.assertRaisesRegex(gate.GateError, "GO-2026-test"):
                self.scan(clean() + [{"finding": {"osv": "GO-2026-test", "trace": trace}}])

    def test_process_failure_cannot_pass_with_clean_json(self):
        with self.assertRaisesRegex(gate.GateError, "scanner exited 1"):
            self.scan(clean(), code=1)

    def test_scanner_process_failure_keeps_raw_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "scan.json"
            result = subprocess.CompletedProcess([], 1, "partial response", "network failure")
            with mock.patch.object(subprocess, "run", return_value=result):
                with self.assertRaisesRegex(gate.GateError, "scanner exited 1"):
                    gate.scan("govulncheck", directory, [], path, "source", "go1.26.10", {})
            self.assertEqual(path.read_text(), "partial response")
            self.assertEqual(path.with_suffix(".stderr.txt").read_text(), "network failure")

    def test_clean_stream_with_unaffected_osv_passes(self):
        record = self.scan(clean() + [{"osv": {"id": "GO-unaffected"}}])
        self.assertEqual(record["findings"], [])
        self.assertEqual(record["sbom"]["go_version"], "go1.26.10")

    def test_incomplete_or_wrong_protocol_fails(self):
        variants = [[], clean()[1:], clean()[:1], clean() + [clean()[0]],
                    [{"unknown": {}}] + clean(), clean("binary")]
        for messages in variants:
            with self.subTest(messages=messages), self.assertRaises(gate.GateError):
                self.scan(messages)
        for key, wrong in [("scanner_version", "v0.0.0"), ("protocol_version", "future"),
                           ("db", "file:///empty"), ("db_last_modified", ""), ("scan_level", "module")]:
            messages = clean()
            messages[0]["config"][key] = wrong
            with self.subTest(key=key), self.assertRaises(gate.GateError):
                self.scan(messages)

    def test_truncated_stream_fails_and_raw_output_is_kept(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "scan.json"
            result = subprocess.CompletedProcess([], 0, '{"config":', "")
            with self.assertRaises(gate.GateError):
                gate.check_scan(result, path, "source", "go1.26.10")
            self.assertEqual(path.read_text(), '{"config":')

    def test_wrong_toolchain_fails(self):
        messages = clean()
        messages[1]["SBOM"]["go_version"] = "go1.25.0"
        with self.assertRaises(gate.GateError):
            self.scan(messages)


class CoverageTests(unittest.TestCase):
    def test_binary_stdlib_version_is_normalized_without_hiding_mismatch(self):
        required = {("stdlib", "v1.26.10")}
        gate.require_coverage(required, gate.normalize_modules({("stdlib", "go1.26.10")}, {}, set()), "binary")
        with self.assertRaises(gate.GateError):
            gate.require_coverage(required, gate.normalize_modules({("stdlib", "go1.26.9")}, {}, set()), "binary")

    def test_sources_cover_local_replacement_dependencies_together(self):
        required = {("example.com/core", ""), ("example.com/js", ""),
                    ("golang.org/x/text", "v0.43.0"), ("stdlib", "v1.26.10")}
        core = {("example.com/core", ""), ("golang.org/x/text", "v0.43.0"), ("stdlib", "v1.26.10")}
        js = {("example.com/js", ""), ("..", ""), ("stdlib", "v1.26.10")}
        gate.require_coverage(required, core | gate.normalize_modules(js, {"..": "example.com/core"}, {"example.com/core", "example.com/js"}), "sources")

    def test_missing_or_stale_dependency_cannot_pass(self):
        required = {("golang.org/x/text", "v0.43.0")}
        for observed in [set(), {("golang.org/x/text", "v0.36.0")}]:
            with self.subTest(observed=observed), self.assertRaisesRegex(gate.GateError, "x/text@v0.43.0"):
                gate.require_coverage(required, observed, "sources")

    def test_binary_coverage_cannot_borrow_from_sources(self):
        with self.assertRaisesRegex(gate.GateError, "wazero"):
            gate.require_coverage({("github.com/tetratelabs/wazero", "v1.12.0")}, set(), "binary")

    def test_unknown_local_replacement_is_rejected(self):
        with self.assertRaises(gate.GateError):
            gate.normalize_modules({("../unknown", "")}, {}, {"example.com/app"})

    def test_own_module_versions_are_not_confused_with_external_versions(self):
        result = gate.normalize_modules({("example.com/app", "(devel)"),
                                         ("golang.org/x/sys", "v0.44.0")}, {}, {"example.com/app"})
        self.assertEqual(result, {("example.com/app", ""), ("golang.org/x/sys", "v0.44.0")})


if __name__ == "__main__":
    unittest.main()
