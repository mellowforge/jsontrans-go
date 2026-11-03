# Security review evidence, 2026-11-03

Baseline: merged PR #10, commit `f5dd8ba6a3073cb5e0f00e82d824102aa1531827`.
These files describe existing pins, including unresolved findings. They are not
evidence that upgrades have already been applied.

- `scans.json`: scanner configs, SBOMs, every finding and raw-stream hashes.
- `*.json.gz`: complete govulncheck streams, gzip timestamp zero. For example,
  `gzip -dc core-source.json.gz` shows all four findings; JSON exit zero does not
  mean zero findings.
- `inventory.json`: Go modules, JS-only runtime imports, guest pins and notice
  fingerprints. Module roles differ from scanner coverage.
- `upstream.json`: all ten published QuickJS advisory metadata records, source
  target and SDK provenance. Empty advisory lists are limited evidence.
- `artifact.json`: raw Wasm section audit of all import kinds; verified
  byte-identical rebuild. Reproducibility does not imply memory safety.
- `probes.go.txt` / `probes.jsonl`: bounded public-API guest reproductions and
  results. No native exploit or upstream attachment was executed.

From the repository root, use the normal dependency cache or a task-local
GOCACHE/GOBIN if your environment restricts writes:

```sh
GOBIN=/tmp/jsontrans-security-tools go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
cd javascript
/tmp/jsontrans-security-tools/govulncheck -json -test ./... > /tmp/javascript-source.json
GOOS=linux GOARCH=amd64 /tmp/jsontrans-security-tools/govulncheck -json -test ./... > /tmp/javascript-linux.json
GOOS=windows GOARCH=amd64 /tmp/jsontrans-security-tools/govulncheck -json -test ./... > /tmp/javascript-windows.json
# Module-only mode accepts no package patterns.
/tmp/jsontrans-security-tools/govulncheck -json -scan=module > /tmp/javascript-modules.json
CGO_ENABLED=0 go build -trimpath -o /tmp/jsontrans-security-public-example ./examples/basic
/tmp/jsontrans-security-tools/govulncheck -json -mode=binary /tmp/jsontrans-security-public-example > /tmp/javascript-binary.json
go mod verify
cd ..
/tmp/jsontrans-security-tools/govulncheck -json -test ./... > /tmp/core-source.json
cd experiments/javascript
/tmp/jsontrans-security-tools/govulncheck -json -test ./... > /tmp/assessment-source.json
```

JSON mode preserves findings; it is not the gate. Use normal failing mode or
parse every finding and scanner failure explicitly. Scan the root independently:
nested local-replacement SBOMs omit transitive native dependencies and cannot
certify the complete selected module graph. Cross-target source runs do not
execute Windows/Linux binaries on macOS.

Reproduce the guest probe from the repository root:

```sh
cp javascript/testdata/security/2026-11-03/probes.go.txt /tmp/jsontrans-security-probe.go
go -C javascript run /tmp/jsontrans-security-probe.go
python3 javascript/internal/quickjswasm/guest/build.py --output /tmp/security-rebuilt.wasm
cmp javascript/internal/quickjswasm/quickjs.wasm /tmp/security-rebuilt.wasm
```

The Proxy fixture traps in the current guest and the next evaluation recovers.
The minimal array shrink/grow fixture returns `undefined`; it did not reproduce
the reported array issue. Neither observation establishes a host escape or a
blanket proof that all related source paths are safe.
