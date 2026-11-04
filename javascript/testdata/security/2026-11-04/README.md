# Go update evidence, 2026-11-04

PR A is based on merged PR #11, `f5c53c4652151eda8c5c939c29fd74754b15edcc`.
These runs use the updated Go selections and the explicitly approved
assessment-only QJS compatibility copy. The historical 2026-11-03 audit is
unchanged. **The embedded QuickJS release blocker remains unresolved.**

- `core-before.txt`: normal-mode scanner exit 3 with GO-2026-5970.
- `core-after.txt`: same core scan after the update, exit 0 with no findings.
- `<target>-report.json`: complete source/test inventories, selected-but-unused
  requirements, scanner config/SBOMs, raw stream hashes and binary hash. Targets
  are darwin/arm64, linux/amd64 and windows/amd64. All gate invocations exited 0.
- `<target>-source-{0,1,2}.json.gz` and `<target>-binary.json.gz`: complete JSON
  scanner streams, compressed with gzip timestamp zero. The numbered source
  scans are core, public optional module and assessment. Decompress to verify
  the raw SHA-256 recorded in the corresponding report. Scanner stderr was
  empty in all successful retained runs. CI retains stderr even on failures.
- `verification.json` and `*-verification.txt.gz`: executed checks and captured
  module test logs. `fuzz-soak.txt.gz` contains the public bounded fuzz runs and
  400-recovery soak; `guest-build.txt.gz` is the successful build's empty log.
- `owned-probes.json`: all 21 owned gates passed with the updated runtime.

Use Go 1.26.10, the three checked-in module selections and scanner v1.9.0.
From the repository root:

```sh
GOBIN=/tmp/jsontrans-security-tools go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
python3 -m unittest discover -s scripts -p 'test_check_go_security.py'
CGO_ENABLED=0 python3 scripts/check_go_security.py --govulncheck /tmp/jsontrans-security-tools/govulncheck --output /tmp/go-security-current
# Keep the scanner host-native; these commands analyze cross-built binaries.
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 python3 scripts/check_go_security.py --govulncheck /tmp/jsontrans-security-tools/govulncheck --output /tmp/go-security-linux
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 python3 scripts/check_go_security.py --govulncheck /tmp/jsontrans-security-tools/govulncheck --output /tmp/go-security-windows
gzip -dc javascript/testdata/security/2026-11-04/linux-amd64-source-0.json.gz
```

The database revision is `2026-11-02T22:01:43Z` in these streams. The commands
contact the official database; later runs can produce different advisories.
The binary hash also includes build/VCS provenance and need not match a later
build from a different commit or dirty state. Cross-target scans ran on macOS;
native target execution is covered separately by CI. No clean Go result here
certifies copied or embedded C code, unused requirements, or untested build tags.
